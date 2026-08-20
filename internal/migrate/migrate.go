// Package migrate applies forward SQL migrations from a directory or embedded FS.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

const grantPrivilegesSQL = `
DO $$
DECLARE
  t TEXT;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'context_gateway') THEN
    RAISE NOTICE 'role context_gateway does not exist; skip grants';
    RETURN;
  END IF;
  GRANT USAGE ON SCHEMA public TO context_gateway;
  FOR t IN
    SELECT tablename FROM pg_tables WHERE schemaname = 'public'
  LOOP
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE %I TO context_gateway', t);
  END LOOP;
  GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO context_gateway;
END
$$;
`

// Options controls migration application.
type Options struct {
	// DSN is the Postgres connection string (prefer admin for DDL).
	DSN string
	// Dir is an optional filesystem directory of *.sql files (e.g. /migrations).
	// When empty or unreadable, embedded migrations are used.
	Dir string
}

// AdminDSN returns POSTGRES_ADMIN_DSN if set, else POSTGRES_DSN.
func AdminDSN() string {
	if v := strings.TrimSpace(os.Getenv("POSTGRES_ADMIN_DSN")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
}

// MigrationsDir returns MIGRATIONS_DIR or "migrations".
func MigrationsDir() string {
	if v := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); v != "" {
		return v
	}
	return "migrations"
}

// Run applies all pending *.sql migrations in sorted order and grants privileges.
func Run(ctx context.Context, opt Options) error {
	if strings.TrimSpace(opt.DSN) == "" {
		return fmt.Errorf("POSTGRES_ADMIN_DSN or POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, opt.DSN)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	files, err := loadMigrations(opt.Dir)
	if err != nil {
		return err
	}
	if opt.Dir != "" {
		fmt.Printf("migrate: loaded %d file(s)\n", len(files))
	} else {
		fmt.Println("migrate: using embedded migrations")
	}
	for _, f := range files {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, f.Version,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			fmt.Printf("skip %s (already applied)\n", f.Version)
			continue
		}
		if _, err := pool.Exec(ctx, f.SQL); err != nil {
			return fmt.Errorf("%s: %w", f.Version, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, f.Version,
		); err != nil {
			return fmt.Errorf("record %s: %w", f.Version, err)
		}
		fmt.Printf("applied %s\n", f.Version)
	}

	if _, err := pool.Exec(ctx, grantPrivilegesSQL); err != nil {
		return fmt.Errorf("grant privileges: %w", err)
	}
	fmt.Println("grants: context_gateway privileges ensured")
	return nil
}

// CheckApplied verifies the database is reachable and every known migration version
// is recorded in schema_migrations. Used by readiness probes.
func CheckApplied(ctx context.Context, dsn, dir string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	files, err := loadMigrations(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, f.Version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("schema_migrations query: %w", err)
		}
		if !exists {
			return fmt.Errorf("migration %s not applied", f.Version)
		}
	}
	return nil
}

// Bootstrap verifies core tables exist and re-applies grants (thin ops bootstrap).
func Bootstrap(ctx context.Context, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("POSTGRES_ADMIN_DSN or POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	required := []string{
		"organizations", "resources", "records", "outbox", "schema_migrations",
	}
	for _, t := range required {
		var ok bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(
			   SELECT 1 FROM information_schema.tables
			   WHERE table_schema = 'public' AND table_name = $1
			 )`, t,
		).Scan(&ok)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("required table %q missing; run migrate first", t)
		}
		fmt.Printf("bootstrap: table %s ok\n", t)
	}
	if _, err := pool.Exec(ctx, grantPrivilegesSQL); err != nil {
		return fmt.Errorf("grant privileges: %w", err)
	}
	fmt.Println("bootstrap: grants ok")
	return nil
}

type migrationFile struct {
	Version string
	SQL     string
}

func loadMigrations(dir string) ([]migrationFile, error) {
	if dir != "" {
		if entries, err := os.ReadDir(dir); err == nil {
			var out []migrationFile
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
					continue
				}
				b, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					return nil, err
				}
				out = append(out, migrationFile{Version: e.Name(), SQL: string(b)})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	return loadEmbedded()
}

func loadEmbedded() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	var out []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migrationFile{Version: e.Name(), SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	if len(out) == 0 {
		return nil, fmt.Errorf("no migration SQL found (dir or embed)")
	}
	return out, nil
}
