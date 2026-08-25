package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	natsbus "github.com/xsama/context-fabric/internal/adapters/nats"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/adapters/postgres"
	s3store "github.com/xsama/context-fabric/internal/adapters/s3"
	"github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/authn"
	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/config"
	"github.com/xsama/context-fabric/internal/connectors/chatwoot"
	"github.com/xsama/context-fabric/internal/connectors/synthetic"
	"github.com/xsama/context-fabric/internal/deletion"
	"github.com/xsama/context-fabric/internal/export"
	"github.com/xsama/context-fabric/internal/httpapi"
	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/migrate"
	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
	"github.com/xsama/context-fabric/internal/quota"
	"github.com/xsama/context-fabric/internal/retrieval"
)

var roles = map[string]bool{
	"serve": true, "worker": true, "connector": true,
	"migrate": true, "bootstrap": true, "doctor": true,
	"backup": true, "restore": true, "reconcile": true,
	"all": true,
}

// oneShotRoles always honor argv and ignore CONTEXT_FABRIC_ROLE.
var oneShotRoles = map[string]bool{
	"migrate": true, "bootstrap": true, "doctor": true,
	"backup": true, "restore": true, "reconcile": true, "connector": true,
}

func main() {
	role := resolveRole()
	if !roles[role] {
		fmt.Fprintf(os.Stderr, "unknown role %q; want serve|worker|connector|migrate|bootstrap|doctor|backup|restore|reconcile|all\n", role)
		os.Exit(2)
	}

	switch role {
	case "migrate":
		if err := runMigrate(); err != nil {
			fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("context-fabric migrate: ok")
		return
	case "doctor":
		if err := runDoctor(); err != nil {
			fmt.Fprintf(os.Stderr, "doctor: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("context-fabric doctor: ok")
		return
	case "bootstrap":
		if err := runBootstrap(); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("context-fabric bootstrap: ok")
		return
	case "backup", "restore", "reconcile":
		if err := runOps(role, os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", role, err)
			os.Exit(1)
		}
		fmt.Printf("context-fabric %s: ok\n", role)
		return
	case "connector":
		if err := runConnector(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "connector: %v\n", err)
			os.Exit(1)
		}
		return
	case "worker":
		runUntilSignal(role)
		return
	case "serve", "all":
		if err := runServe(role); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
			os.Exit(1)
		}
	}
}

// resolveRole: one-shot argv roles beat CONTEXT_FABRIC_ROLE; long-running
// serve/worker/all prefer env when set (k8s), else argv, else "serve".
func resolveRole() string {
	argv := ""
	if len(os.Args) > 1 {
		argv = strings.TrimSpace(os.Args[1])
	}
	env := strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_ROLE"))

	if argv != "" && oneShotRoles[argv] {
		return argv
	}
	if env != "" {
		return env
	}
	if argv != "" {
		return argv
	}
	return "serve"
}

func profileName() string {
	return strings.ToLower(firstEnv("PROFILE", "CONTEXT_FABRIC_PROFILE", "starter"))
}

func useMemory() bool {
	if strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "1") ||
		strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "true") {
		return true
	}
	// Explicit demo without Postgres → in-process memory ledger.
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	return dsn == "" && profileName() == "demo"
}

func allowMemoryAuth() bool {
	// Local forgeable tokens + memory OpenFGA only when explicitly requested.
	return useMemory() || profileName() == "demo"
}

func allowStubOps() bool {
	if strings.EqualFold(os.Getenv("CONTEXT_FABRIC_ALLOW_STUB_OPS"), "1") ||
		strings.EqualFold(os.Getenv("CONTEXT_FABRIC_ALLOW_STUB_OPS"), "true") {
		return true
	}
	return allowMemoryAuth()
}

func runServe(role string) error {
	svc, srv, bus, cleanup, err := wire(role)
	if err != nil {
		return err
	}
	defer cleanup()

	addr := firstEnv("LISTEN_ADDR", "CONTEXT_FABRIC_LISTEN_ADDR", ":8080")
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		srv.SetReady(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	if role == "all" {
		w := &app.Worker{Ledger: svc.Ledger, Bus: bus, Index: svc.Index, ChangeFeed: svc.ChangeFeed, Authz: asRelWriter(svc.Authz)}
		go app.RunWorker(ctx, w)
	}

	fmt.Printf("context-fabric %s: listening on %s (memory=%v)\n", role, addr, useMemory())
	return httpSrv.ListenAndServe()
}

func wire(role string) (*app.ApplicationService, *httpapi.Server, ports.EventBus, func(), error) {
	pol := policy.New()
	auditLog := audit.Logger(audit.NewMemory())
	q := quota.NewLimiter(quota.DefaultLimits())

	var (
		identity   ports.IdentityProvider
		ledger     ports.LedgerStore
		evidence   ports.EvidenceStore
		index      ports.IndexProvider
		authz      ports.AuthorizationProvider
		bus        ports.EventBus
		changesL   app.ChangeLister
		quotas     app.QuotaStore
		extras     app.ExtraStore
		snippets   retrieval.SnippetSource
		creds      ports.CredentialProvider
		exportJobs app.ExportJobStore
		holds      deletion.LegalHoldChecker
		feedStore  changes.FeedStore
		pgPool     *postgres.Pool
		cleanup    = func() {}
		modelID    string
	)

	bus = natsbus.NewNoop()
	creds = memory.NewCredentialStore()

	if useMemory() {
		identity = authn.NewLocal()
		store := memory.NewStore()
		idx := memory.NewIndex()
		ledger = store
		evidence = memory.NewEvidence()
		index = idx
		snippets = idx
		changesL = store
		quotas = store
		extras = store
		exportJobs = store
		holds = store
		feedStore = store
		memAuthz := openfga.NewMemory()
		authz = memAuthz
		modelID = memAuthz.ModelID
		// Memory profile: optional NATS; silent noop when unreachable.
		if u := strings.TrimSpace(os.Getenv("NATS_URL")); u != "" {
			if b, err := natsbus.Connect(u); err == nil {
				bus = b
				prev := cleanup
				cleanup = func() {
					b.Close()
					prev()
				}
			}
		}
	} else {
		cfg, err := config.Load()
		if err != nil {
			return nil, nil, nil, nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pool, err := postgres.Connect(ctx, cfg.Postgres.DSN)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		cleanup = pool.Close
		pgPool = pool
		pgStore := postgres.NewStore(pool)
		ledger = pgStore
		changesL = pgStore
		quotas = pgStore
		extras = pgStore
		exportJobs = pgStore
		feedStore = pgStore
		creds = postgres.NewCredentialStore(pool)
		auditLog = &audit.LedgerLogger{Writer: pgStore}
		evidence = memory.NewEvidence()
		if strings.TrimSpace(cfg.S3.Endpoint) != "" {
			s3e, err := s3store.New(s3store.Config{
				Endpoint: cfg.S3.Endpoint, Region: cfg.S3.Region,
				AccessKeyID: cfg.S3.AccessKeyID, SecretAccessKey: cfg.S3.SecretAccessKey,
				PathStyle: cfg.S3.PathStyle, Bucket: cfg.S3.BucketQuarantine,
			})
			if err != nil {
				if allowMemoryAuth() {
					fmt.Fprintf(os.Stderr, "s3 adapter warning: %v (demo: using memory evidence)\n", err)
				} else {
					return nil, nil, nil, nil, fmt.Errorf("s3 evidence required (fail-closed): %w", err)
				}
			} else {
				evidence = s3e
			}
		}

		indexBackend := strings.ToLower(strings.TrimSpace(os.Getenv("INDEX_BACKEND")))
		switch indexBackend {
		case "postgres", "pg", "search_documents":
			pgIdx := postgres.NewIndex(pool)
			index = pgIdx
			snippets = pgIdx
		default:
			// In-process memory index: fine for role=all (shared). Split serve/worker
			// cannot share it — fail closed unless INDEX_BACKEND=postgres.
			if role == "serve" || role == "worker" {
				return nil, nil, nil, nil, fmt.Errorf(
					"in-memory index cannot be shared across separate serve/worker processes; use CONTEXT_FABRIC_ROLE=all (or CMD all) or set INDEX_BACKEND=postgres",
				)
			}
			memIdx := memory.NewIndex()
			index = memIdx
			snippets = memIdx
		}

		if allowMemoryAuth() {
			// PROFILE=demo: Local identity + memory OpenFGA even with Postgres ledger.
			identity = authn.NewLocal()
			memAuthz := openfga.NewMemory()
			authz = memAuthz
			modelID = memAuthz.ModelID
		} else {
			if strings.TrimSpace(cfg.OIDC.Issuer) == "" {
				return nil, nil, nil, nil, fmt.Errorf("OIDC_ISSUER is required when not using memory/demo auth")
			}
			identity = authn.NewOIDC(authn.OIDCConfig{
				Issuer:       cfg.OIDC.Issuer,
				Audience:     cfg.OIDC.Audience,
				DiscoveryURL: cfg.OIDC.DiscoveryURL,
				JWKSURL:      cfg.OIDC.JWKSURL,
				ClaimSubject: cfg.OIDC.ClaimSubject,
				ClaimEmail:   cfg.OIDC.ClaimEmail,
				ClaimGroups:  cfg.OIDC.ClaimGroups,
				ClaimOrg:     cfg.OIDC.ClaimOrg,
				ClaimScopes:  cfg.OIDC.ClaimScopes,
			})
			fga, err := openfga.NewFromEnv()
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("openfga required (fail-closed): %w", err)
			}
			authz = fga
			modelID = fga.ModelID
		}

		natsURL := strings.TrimSpace(cfg.NATS.URL)
		if natsURL == "" {
			natsURL = strings.TrimSpace(os.Getenv("NATS_URL"))
		}
		if allowMemoryAuth() {
			// Demo+Postgres: prefer live NATS; fall back to in-process noop with warning.
			if natsURL == "" {
				fmt.Fprintln(os.Stderr, "nats: demo profile using in-process noop bus (set NATS_URL for JetStream)")
			} else if b, err := natsbus.Connect(natsURL); err == nil {
				bus = b
				prev := cleanup
				cleanup = func() {
					b.Close()
					prev()
				}
			} else {
				fmt.Fprintf(os.Stderr, "nats: demo profile falling back to noop: %v\n", err)
			}
		} else {
			if natsURL == "" {
				return nil, nil, nil, nil, fmt.Errorf("NATS_URL is required when not using memory/demo profile")
			}
			b, err := natsbus.Connect(natsURL)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("nats required (fail-closed): %w", err)
			}
			bus = b
			prev := cleanup
			cleanup = func() {
				b.Close()
				prev()
			}
		}
	}

	// Also wire S3 when endpoint is set in memory/demo mode.
	if ep := strings.TrimSpace(os.Getenv("S3_ENDPOINT")); ep != "" && useMemory() {
		if s3e, err := s3store.New(s3store.Config{
			Endpoint: ep, Region: firstEnv("S3_REGION", "us-east-1"),
			AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			PathStyle: true, Bucket: firstEnv("S3_BUCKET_QUARANTINE", "context-quarantine"),
		}); err == nil {
			evidence = s3e
		} else {
			fmt.Fprintf(os.Stderr, "s3 adapter warning: %v (memory profile: keeping memory evidence)\n", err)
		}
	}

	secret, err := requireWebhookSigningSecret()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	changeFeed := changes.New(feedStore, secret)
	if feedStore == nil && changesL != nil {
		changeFeed = changes.New(changesL, secret)
	}
	if pgPool != nil {
		changeFeed.Durable = postgres.NewWebhookStore(pgPool)
	}
	identity = authn.WithAPIKeys(identity, creds)
	delSvc := &deletion.Service{
		Ledger: ledger, Evidence: evidence, Index: index, Authz: authz,
		Audit: auditLog, Changes: changeFeed, Holds: holds,
	}
	expSvc := &export.Service{Ledger: ledger}

	pipe := &retrieval.Pipeline{
		Identity: identity,
		Authz:    authz,
		Policy:   pol,
		Ledger:   ledger,
		Index:    index,
		Audit:    auditLog,
		Snippets: snippets,
	}
	ing := &ingest.IntakeService{Ledger: ledger, Evidence: evidence}
	if w, ok := authz.(ports.RelationshipWriter); ok {
		ing.Authz = w
	}
	svc := &app.ApplicationService{
		Identity: identity,
		Authz:    authz,
		Policy:   pol,
		Ledger:   ledger,
		Evidence: evidence,
		Index:    index,
		Bus:      bus,
		Audit:    auditLog,
		Quota:    q,
		Retrieve: pipe,
		Ingest:   ing,
		Build: app.VersionInfo{
			ProductVersion: firstEnv("CONTEXT_FABRIC_VERSION", "0.0.0-dev"),
			AuthzModelID:   modelID,
		},
		Ready: func() bool { return true },
		ReadyDetail: func() (bool, map[string]any) {
			checks := map[string]any{}
			migOK := true
			migDetail := "memory profile (no schema_migrations)"
			if !useMemory() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
				if err := migrate.CheckApplied(ctx, dsn, migrate.MigrationsDir()); err != nil {
					migOK = false
					migDetail = err.Error()
				} else {
					migDetail = "all migrations applied (008+ outbox claim disambiguation, org-scoped revoke)"
				}
			}
			checks["migrations"] = map[string]any{"ok": migOK, "detail": migDetail}

			authzOK := strings.TrimSpace(modelID) != ""
			authzDetail := "authz model id present"
			if !authzOK {
				authzDetail = "authz model id missing"
			}
			checks["authz_model_pinned"] = map[string]any{"ok": authzOK, "detail": authzDetail}

			relWriter := asRelWriter(authz)
			tupleWriterOK := relWriter != nil
			tupleWriterDetail := "RelationshipWriter available"
			if !tupleWriterOK {
				tupleWriterDetail = "AuthorizationProvider does not implement RelationshipWriter"
			}
			checks["authz_tuple_writer"] = map[string]any{"ok": tupleWriterOK, "detail": tupleWriterDetail}

			pending, dead, outboxErr := ledger.CountAuthzTuplePending(context.Background(), "")
			maxPending := 500
			if v := strings.TrimSpace(os.Getenv("AUTHZ_OUTBOX_PENDING_MAX")); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n >= 0 {
					maxPending = n
				}
			}
			outboxOK := outboxErr == nil && dead == 0 && pending <= maxPending
			outboxDetail := fmt.Sprintf("pending=%d dead=%d max_pending=%d", pending, dead, maxPending)
			if outboxErr != nil {
				outboxOK = false
				outboxDetail = outboxErr.Error()
			} else if dead > 0 {
				outboxDetail = fmt.Sprintf("dead-letter AuthZ tuples present (%d); pending=%d", dead, pending)
			} else if pending > maxPending {
				outboxDetail = fmt.Sprintf("AuthZ outbox lag exceeds SLO: pending=%d max=%d", pending, maxPending)
			}
			checks["authz_outbox"] = map[string]any{"ok": outboxOK, "detail": outboxDetail, "pending": pending, "dead": dead, "max_pending": maxPending}

			ready := migOK && authzOK && tupleWriterOK && outboxOK
			return ready, checks
		},
		Changes:     changesL,
		Quotas:      quotas,
		Extras:      extras,
		Deletion:    delSvc,
		Export:      expSvc,
		ChangeFeed:  changeFeed,
		Credentials: creds,
		ExportJobs:  exportJobs,
	}
	if ms, ok := ledger.(app.MappingStore); ok {
		svc.Mappings = ms
		svc.MappingBySource = func(ctx context.Context, orgID, sourceID string) (mapping.Spec, error) {
			return ms.GetMappingSpec(ctx, orgID, sourceID)
		}
	}
	api := httpapi.New(svc)
	return svc, api, bus, cleanup, nil
}

func runUntilSignal(role string) {
	fmt.Printf("context-fabric %s: running\n", role)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if role == "worker" {
		svc, _, bus, cleanup, err := wire(role)
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker wire: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()
		w := &app.Worker{Ledger: svc.Ledger, Bus: bus, Index: svc.Index, ChangeFeed: svc.ChangeFeed, Authz: asRelWriter(svc.Authz)}
		app.RunWorker(ctx, w)
		fmt.Printf("context-fabric %s: shutdown\n", role)
		return
	}
	<-ctx.Done()
	fmt.Printf("context-fabric %s: shutdown\n", role)
}

func runConnector(args []string) error {
	name := "chatwoot"
	rest := args
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
		rest = args[1:]
	}
	switch name {
	case "chatwoot":
		return chatwoot.RunCLI(rest)
	case "synthetic":
		return synthetic.RunCLI(rest)
	default:
		return fmt.Errorf("unknown connector %q (want chatwoot|synthetic)", name)
	}
}

func runMigrate() error {
	dsn := migrate.AdminDSN()
	if dsn == "" {
		if useMemory() || profileName() == "demo" {
			fmt.Println("migrate: no DSN; skipping (memory/demo profile)")
			return nil
		}
		return fmt.Errorf("POSTGRES_ADMIN_DSN or POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	return migrate.Run(ctx, migrate.Options{
		DSN: dsn,
		Dir: migrate.MigrationsDir(),
	})
}

func runBootstrap() error {
	if allowStubOps() && migrate.AdminDSN() == "" {
		fmt.Println("bootstrap: memory/demo stub — no DB; OpenFGA store seeding skipped")
		return nil
	}
	dsn := migrate.AdminDSN()
	if dsn == "" {
		return fmt.Errorf("NOT IMPLEMENTED without DSN; set POSTGRES_ADMIN_DSN or CONTEXT_FABRIC_ALLOW_STUB_OPS=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := migrate.Bootstrap(ctx, dsn); err != nil {
		return err
	}
	return bootstrapOpenFGA(ctx)
}

// runOps executes scripts/{backup,restore,reconcile}.sh when bash is available.
// Falls back to an acknowledged stub only when CONTEXT_FABRIC_ALLOW_STUB_OPS is set.
func runOps(role string, args []string) error {
	script, err := locateOpsScript(role)
	if err != nil {
		if allowStubOps() {
			fmt.Printf("%s: script missing; stub allowed (CONTEXT_FABRIC_ALLOW_STUB_OPS or memory/demo)\n", role)
			return nil
		}
		return err
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		if allowStubOps() {
			fmt.Printf("%s: bash not found; stub allowed\n", role)
			return nil
		}
		return fmt.Errorf("bash required to run %s (or set CONTEXT_FABRIC_ALLOW_STUB_OPS=1): %w", script, err)
	}
	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command(bash, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", script, err)
	}
	return nil
}

func locateOpsScript(role string) (string, error) {
	name := "scripts/" + role + ".sh"
	candidates := []string{name}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "..", name),
			filepath.Join(filepath.Dir(exe), name),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, name))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", fmt.Errorf("NOT IMPLEMENTED: missing %s; set CONTEXT_FABRIC_ALLOW_STUB_OPS=1 to bypass", name)
}

func runDoctor() error {
	if useMemory() {
		fmt.Println("doctor: memory profile ok")
		return nil
	}

	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if dsn == "" {
		dsn = migrate.AdminDSN()
	}
	if dsn == "" {
		return fmt.Errorf("postgres: POSTGRES_DSN (or POSTGRES_ADMIN_DSN) required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()
	fmt.Println("doctor: postgres ping ok")

	var migOK bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM information_schema.tables
		   WHERE table_schema = 'public' AND table_name = 'schema_migrations'
		 )`,
	).Scan(&migOK)
	if err != nil {
		return fmt.Errorf("migrations table check: %w", err)
	}
	if !migOK {
		return fmt.Errorf("schema_migrations missing; run migrate")
	}
	var migCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migCount); err != nil {
		fmt.Printf("doctor: schema_migrations count warning: %v\n", err)
	} else {
		fmt.Printf("doctor: schema_migrations count=%d (hint: compare to migrations/ files)\n", migCount)
		if migCount == 0 {
			return fmt.Errorf("schema_migrations empty; run migrate")
		}
	}
	fmt.Println("doctor: schema_migrations ok")

	var ftsOK bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM information_schema.columns
		   WHERE table_schema = 'public' AND table_name = 'search_documents' AND column_name = 'search_tsv'
		 )`,
	).Scan(&ftsOK)
	if err != nil {
		fmt.Printf("doctor: FTS column check warning: %v\n", err)
	} else if ftsOK {
		fmt.Println("doctor: search_documents.search_tsv present")
	} else {
		return fmt.Errorf("search_documents.search_tsv missing; apply migration 006+")
	}

	if url := strings.TrimSpace(os.Getenv("NATS_URL")); url != "" {
		if b, err := natsbus.Connect(url); err != nil {
			fmt.Printf("doctor: nats warning: %v\n", err)
		} else {
			b.Close()
			fmt.Println("doctor: nats ok")
		}
	} else {
		fmt.Println("doctor: nats skipped (NATS_URL unset)")
	}

	if ep := strings.TrimSpace(os.Getenv("S3_ENDPOINT")); ep != "" {
		if _, err := s3store.New(s3store.Config{
			Endpoint: ep, Region: firstEnv("S3_REGION", "us-east-1"),
			AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			PathStyle: true, Bucket: firstEnv("S3_BUCKET_QUARANTINE", "context-quarantine"),
		}); err != nil {
			fmt.Printf("doctor: s3 warning: %v\n", err)
		} else {
			fmt.Println("doctor: s3 config ok")
		}
	} else {
		fmt.Println("doctor: s3 skipped (S3_ENDPOINT unset)")
	}

	if allowMemoryAuth() {
		fmt.Println("doctor: openfga skipped (memory/demo auth)")
	} else {
		if err := validateOpenFGAStoreID(); err != nil {
			return err
		}
		modelID := strings.TrimSpace(firstEnv("OPENFGA_MODEL_ID", "AUTHZ_MODEL_ID", ""))
		if modelID == "" || modelID == "demo-model-v1" {
			if path := strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID_FILE")); path != "" {
				if b, err := os.ReadFile(path); err == nil {
					modelID = strings.TrimSpace(string(b))
				}
			}
		}
		if modelID == "" || modelID == "demo-model-v1" {
			return fmt.Errorf("OPENFGA_MODEL_ID unset or placeholder; run bootstrap to write and pin a model")
		}
		fmt.Printf("doctor: openfga model pin=%s\n", modelID)
		if _, err := openfga.NewFromEnv(); err != nil {
			return fmt.Errorf("openfga: %w", err)
		}
		fmt.Println("doctor: openfga config ok")
	}

	if allowMemoryAuth() || useMemory() {
		fmt.Println("doctor: WEBHOOK_SIGNING_SECRET skipped (memory/demo)")
	} else if strings.TrimSpace(os.Getenv("WEBHOOK_SIGNING_SECRET")) == "" {
		return fmt.Errorf("WEBHOOK_SIGNING_SECRET required for non-demo profiles")
	} else {
		fmt.Println("doctor: WEBHOOK_SIGNING_SECRET set")
	}

	fmt.Println("doctor: connectivity ok")
	return nil
}

func validateOpenFGAStoreID() error {
	store := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID"))
	if store == "" || store == "replace-after-bootstrap" {
		if path := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID_FILE")); path != "" {
			if b, err := os.ReadFile(path); err == nil {
				store = strings.TrimSpace(string(b))
			}
		}
	}
	if store == "" || store == "replace-after-bootstrap" {
		return fmt.Errorf(
			"OPENFGA_STORE_ID is unset or still %q; run bootstrap with OPENFGA_API_URL set (or create a store and pin the ID)",
			"replace-after-bootstrap",
		)
	}
	return nil
}

// bootstrapOpenFGA creates an OpenFGA store when possible, writes the JSON
// authorization model when OPENFGA_MODEL_ID is empty, and validates the pin.
func bootstrapOpenFGA(ctx context.Context) error {
	api := strings.TrimSpace(firstEnv("OPENFGA_API_URL", "OPENFGA_URL", ""))
	if api == "" {
		fmt.Println("bootstrap: OPENFGA_API_URL unset; skipping OpenFGA store bootstrap")
		return nil
	}
	store := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID"))
	modelPath := firstNonEmpty(
		os.Getenv("OPENFGA_MODEL_PATH"),
		"contracts/openfga/model.fga",
	)
	modelJSONPath := firstNonEmpty(
		os.Getenv("OPENFGA_MODEL_JSON"),
		strings.TrimSuffix(modelPath, filepath.Ext(modelPath))+".json",
		"contracts/openfga/model.json",
	)
	if _, err := os.Stat(modelPath); err != nil {
		fmt.Printf("bootstrap: openfga DSL model missing at %s (%v)\n", modelPath, err)
	} else {
		fmt.Printf("bootstrap: openfga DSL model present: %s\n", modelPath)
	}
	if _, err := os.Stat(modelJSONPath); err != nil {
		fmt.Printf("bootstrap: openfga JSON model missing at %s (%v)\n", modelJSONPath, err)
	} else {
		fmt.Printf("bootstrap: openfga JSON model present: %s\n", modelJSONPath)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	if store == "" || store == "replace-after-bootstrap" {
		created, err := openfgaCreateStore(ctx, client, api, "context-fabric")
		if err != nil {
			return fmt.Errorf(
				"OPENFGA_STORE_ID is %q and auto-create failed: %w; create a store manually and set OPENFGA_STORE_ID",
				"replace-after-bootstrap", err,
			)
		}
		fmt.Printf("bootstrap: created OpenFGA store id=%s — set OPENFGA_STORE_ID=%s\n", created, created)
		store = created
		if path := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID_FILE")); path != "" {
			if err := os.WriteFile(path, []byte(store+"\n"), 0o644); err != nil {
				return fmt.Errorf("write OPENFGA_STORE_ID_FILE: %w", err)
			}
			fmt.Printf("bootstrap: wrote store id to %s\n", path)
		}
	} else {
		if err := openfgaPingStore(ctx, client, api, store); err != nil {
			return fmt.Errorf("OPENFGA_STORE_ID %q not reachable: %w", store, err)
		}
		fmt.Printf("bootstrap: OpenFGA store %s ok\n", store)
	}

	modelID := strings.TrimSpace(firstEnv("OPENFGA_MODEL_ID", "AUTHZ_MODEL_ID", ""))
	if modelID == "" || modelID == "demo-model-v1" {
		written, err := openfgaWriteAuthorizationModel(ctx, client, api, store, modelJSONPath)
		if err != nil {
			return fmt.Errorf("write authorization model from %s: %w", modelJSONPath, err)
		}
		modelID = written
		fmt.Printf("bootstrap: wrote authorization model id=%s — pin OPENFGA_MODEL_ID=%s\n", modelID, modelID)
		if path := strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID_FILE")); path != "" {
			if err := os.WriteFile(path, []byte(modelID+"\n"), 0o644); err != nil {
				return fmt.Errorf("write OPENFGA_MODEL_ID_FILE: %w", err)
			}
			fmt.Printf("bootstrap: wrote model id to %s\n", path)
		}
	} else {
		fmt.Printf("bootstrap: OPENFGA_MODEL_ID=%s (ensure it matches a written model)\n", modelID)
	}
	return nil
}

func openfgaWriteAuthorizationModel(ctx context.Context, client *http.Client, apiURL, storeID, modelJSONPath string) (string, error) {
	raw, err := os.ReadFile(modelJSONPath)
	if err != nil {
		return "", err
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("invalid model JSON: %w", err)
	}
	if _, ok := probe["type_definitions"]; !ok {
		return "", fmt.Errorf("model JSON missing type_definitions")
	}
	apiURL = strings.TrimRight(apiURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/stores/"+storeID+"/authorization-models", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(os.Getenv("OPENFGA_API_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}
	var out struct {
		AuthorizationModelID string `json:"authorization_model_id"`
		ID                   string `json:"id"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	id := firstNonEmpty(out.AuthorizationModelID, out.ID)
	if id == "" {
		return "", fmt.Errorf("empty model id in response: %s", string(b))
	}
	return id, nil
}

func openfgaCreateStore(ctx context.Context, client *http.Client, apiURL, name string) (string, error) {
	apiURL = strings.TrimRight(apiURL, "/")
	body := strings.NewReader(fmt.Sprintf(`{"name":%q}`, name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/stores", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(os.Getenv("OPENFGA_API_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", fmt.Errorf("empty store id in response: %s", string(b))
	}
	return out.ID, nil
}

func openfgaPingStore(ctx context.Context, client *http.Client, apiURL, storeID string) error {
	apiURL = strings.TrimRight(apiURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/stores/"+storeID, nil)
	if err != nil {
		return err
	}
	if tok := strings.TrimSpace(os.Getenv("OPENFGA_API_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func requireWebhookSigningSecret() ([]byte, error) {
	s := strings.TrimSpace(os.Getenv("WEBHOOK_SIGNING_SECRET"))
	if s != "" {
		return []byte(s), nil
	}
	if allowMemoryAuth() || useMemory() {
		// Demo/memory only: ephemeral secret for local tests.
		return []byte("demo-webhook-signing-secret"), nil
	}
	return nil, fmt.Errorf("WEBHOOK_SIGNING_SECRET is required for production profiles")
}

func webhookSigningSecret() []byte {
	s, _ := requireWebhookSigningSecret()
	return s
}

func firstEnv(keys ...string) string {
	def := ""
	for i, k := range keys {
		if i == len(keys)-1 && (strings.HasPrefix(k, ":") || !strings.Contains(k, "_") && strings.ToUpper(k) != k) {
			def = k
			continue
		}
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	if len(keys) > 0 {
		last := keys[len(keys)-1]
		if v := strings.TrimSpace(os.Getenv(last)); v != "" {
			return v
		}
	}
	return def
}

func asRelWriter(authz ports.AuthorizationProvider) ports.RelationshipWriter {
	if w, ok := authz.(ports.RelationshipWriter); ok {
		return w
	}
	return nil
}
