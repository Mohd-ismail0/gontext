package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/xsama/context-fabric/internal/adapters/memory"
	natsbus "github.com/xsama/context-fabric/internal/adapters/nats"
	"github.com/xsama/context-fabric/internal/adapters/openfga"
	"github.com/xsama/context-fabric/internal/adapters/postgres"
	s3store "github.com/xsama/context-fabric/internal/adapters/s3"
	"github.com/xsama/context-fabric/internal/app"
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

func main() {
	role := "serve"
	if len(os.Args) > 1 {
		role = strings.TrimSpace(os.Args[1])
	}
	if env := strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_ROLE")); env != "" {
		role = env
	}
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
	case "bootstrap", "backup", "restore", "reconcile":
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

func useMemory() bool {
	if strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "1") ||
		strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "true") {
		return true
	}
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	profile := strings.ToLower(firstEnv("PROFILE", "CONTEXT_FABRIC_PROFILE", "demo"))
	return dsn == "" && (profile == "demo" || profile == "")
}

func runServe(role string) error {
	svc, srv, bus, cleanup, err := wire()
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
		w := &app.Worker{Ledger: svc.Ledger, Bus: bus, Index: svc.Index}
		go app.RunWorker(ctx, w)
	}

	fmt.Printf("context-fabric %s: listening on %s (memory=%v)\n", role, addr, useMemory())
	return httpSrv.ListenAndServe()
}

func wire() (*app.ApplicationService, *httpapi.Server, ports.EventBus, func(), error) {
	identity := authn.NewLocal()
	pol := policy.New()
	auditLog := audit.NewMemory()
	q := quota.NewLimiter(quota.DefaultLimits())

	var (
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
		cleanup    = func() {}
		modelID    string
	)

	bus = natsbus.ConnectOrNoop(strings.TrimSpace(os.Getenv("NATS_URL")))
	creds = memory.NewCredentialStore()

	if useMemory() {
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
		ledger = postgres.NewStore(pool)
		evidence = memory.NewEvidence()
		if strings.TrimSpace(cfg.S3.Endpoint) != "" {
			if s3e, err := s3store.New(s3store.Config{
				Endpoint: cfg.S3.Endpoint, Region: cfg.S3.Region,
				AccessKeyID: cfg.S3.AccessKeyID, SecretAccessKey: cfg.S3.SecretAccessKey,
				PathStyle: cfg.S3.PathStyle, Bucket: cfg.S3.BucketQuarantine,
			}); err == nil {
				evidence = s3e
			} else {
				fmt.Fprintf(os.Stderr, "s3 adapter warning: %v (using memory evidence)\n", err)
			}
		}
		index = memory.NewIndex()
		if fga, err := openfga.NewFromEnv(); err == nil {
			authz = fga
			modelID = fga.ModelID
		} else {
			authz = openfga.NewMemory()
		}
		if b, err := natsbus.Connect(cfg.NATS.URL); err == nil {
			bus = b
			prev := cleanup
			cleanup = func() {
				b.Close()
				prev()
			}
		}
	}

	// Also wire S3 when endpoint is set in memory/demo mode.
	if ep := strings.TrimSpace(os.Getenv("S3_ENDPOINT")); ep != "" {
		if s3e, err := s3store.New(s3store.Config{
			Endpoint: ep, Region: firstEnv("S3_REGION", "us-east-1"),
			AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			PathStyle: true, Bucket: firstEnv("S3_BUCKET_QUARANTINE", "context-quarantine"),
		}); err == nil {
			evidence = s3e
		}
	}

	changeFeed := changes.New(feedStore, nil)
	if feedStore == nil && changesL != nil {
		changeFeed = changes.New(changesL, nil)
	}
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
		Ready:       func() bool { return true },
		Changes:     changesL,
		Quotas:      quotas,
		Extras:      extras,
		Deletion:    delSvc,
		Export:      expSvc,
		ChangeFeed:  changeFeed,
		Credentials: creds,
		ExportJobs:  exportJobs,
	}
	api := httpapi.New(svc)
	return svc, api, bus, cleanup, nil
}

func runUntilSignal(role string) {
	fmt.Printf("context-fabric %s: running\n", role)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if role == "worker" {
		svc, _, bus, cleanup, err := wire()
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker wire: %v\n", err)
			return
		}
		defer cleanup()
		w := &app.Worker{Ledger: svc.Ledger, Bus: bus, Index: svc.Index}
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
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		fmt.Println("migrate: POSTGRES_DSN empty; skipping (memory profile)")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	dir := firstEnv("MIGRATIONS_DIR", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		fmt.Printf("applied %s\n", e.Name())
	}
	return nil
}

func runDoctor() error {
	if useMemory() {
		fmt.Println("doctor: memory profile ok")
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, cfg.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	pool.Close()
	if _, err := openfga.NewFromEnv(); err != nil {
		fmt.Printf("doctor: openfga warning: %v\n", err)
	}
	fmt.Println("doctor: connectivity ok")
	return nil
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
