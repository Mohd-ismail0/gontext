package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEscapeHatchesRejectsOutsideDemo(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_MEMORY", "1")
	if err := ValidateEscapeHatches("starter"); err == nil {
		t.Fatal("expected error for CONTEXT_FABRIC_MEMORY outside demo")
	}
	t.Setenv("CONTEXT_FABRIC_MEMORY", "")
	t.Setenv("CONTEXT_FABRIC_ALLOW_STUB_OPS", "true")
	if err := ValidateEscapeHatches("xsama"); err == nil {
		t.Fatal("expected error for CONTEXT_FABRIC_ALLOW_STUB_OPS outside demo")
	}
}

func TestValidateEscapeHatchesAllowsDemo(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_MEMORY", "1")
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "1")
	if err := ValidateEscapeHatches("demo"); err != nil {
		t.Fatalf("demo should allow escape hatches: %v", err)
	}
}

func TestSecretValueFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBHOOK_SIGNING_SECRET", "")
	t.Setenv("WEBHOOK_SIGNING_SECRET_FILE", path)
	got, err := secretValue("WEBHOOK_SIGNING_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretValueMissingFileFallsBackToEnv(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.txt")
	t.Setenv("OPENFGA_STORE_ID", "replace-after-bootstrap")
	t.Setenv("OPENFGA_STORE_ID_FILE", missing)
	got, err := secretValue("OPENFGA_STORE_ID")
	if err != nil {
		t.Fatal(err)
	}
	if got != "replace-after-bootstrap" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretValuePrefersExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfga_store_id")
	if err := os.WriteFile(path, []byte("store-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENFGA_STORE_ID", "replace-after-bootstrap")
	t.Setenv("OPENFGA_STORE_ID_FILE", path)
	got, err := secretValue("OPENFGA_STORE_ID")
	if err != nil {
		t.Fatal(err)
	}
	if got != "store-from-file" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadBaseSkipsRuntimeDeps(t *testing.T) {
	t.Setenv("PROFILE", "starter")
	t.Setenv("CONTEXT_FABRIC_MEMORY", "")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@localhost/db")
	t.Setenv("POSTGRES_ADMIN_DSN", "postgres://admin:p@localhost/db")
	t.Setenv("WEBHOOK_SIGNING_SECRET", "")
	t.Setenv("DELETION_SIGNING_SECRET", "")

	cfg, err := LoadBase()
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if cfg.Profile != "starter" {
		t.Fatalf("profile=%q", cfg.Profile)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected full Load to require runtime deps")
	}
}

func TestLoadRejectsPlaceholderSecrets(t *testing.T) {
	t.Setenv("PROFILE", "starter")
	t.Setenv("CONTEXT_FABRIC_MEMORY", "")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@localhost/db")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_ACCESS_KEY_ID", "key")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	t.Setenv("OPENFGA_API_URL", "http://localhost:8080")
	t.Setenv("OPENFGA_MODEL_ID", "model-1")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_DISCOVERY_URL", "https://issuer.example/.well-known/openid-configuration")
	t.Setenv("WEBHOOK_SIGNING_SECRET", "change-me")
	t.Setenv("DELETION_SIGNING_SECRET", "real-secret-value")
	_, err := Load()
	if err == nil {
		t.Fatal("expected placeholder rejection")
	}
}

func TestConfigOverlayValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: context-fabric.io/v1\nkind: DeploymentConfig\nprofile: starter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigOverlay(path); err != nil {
		t.Fatal(err)
	}
}

func TestUseMemoryDemoWithoutDSN(t *testing.T) {
	t.Setenv("PROFILE", "demo")
	t.Setenv("CONTEXT_FABRIC_MEMORY", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_ACCESS_KEY_ID", "key")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UseMemory() {
		t.Fatal("expected memory mode for demo without DSN")
	}
}
