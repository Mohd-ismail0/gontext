package changes

import (
	"testing"
)

func TestValidateWebhookTargetHTTPSRequired(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_PROFILE", "starter")
	if err := ValidateWebhookTarget("http://example.com/hook"); err == nil {
		t.Fatal("expected https required outside demo")
	}
	if err := ValidateWebhookTarget("https://example.com/hook"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateWebhookTargetBlocksLoopback(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_PROFILE", "demo")
	if err := ValidateWebhookTarget("http://127.0.0.1/hook"); err == nil {
		t.Fatal("expected loopback blocked")
	}
}

func TestValidateWebhookTargetBlocksMetadata(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_PROFILE", "demo")
	if err := ValidateWebhookTarget("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Fatal("expected metadata IP blocked")
	}
}
