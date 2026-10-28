package authn

import (
	"context"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

// TestOIDCRejectsMissingExp verifies a signed token without an `exp` claim is
// rejected (fail closed) rather than being treated as never-expiring.
func TestOIDCRejectsMissingExp(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)

	p := NewOIDC(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "context-fabric", JWKSURL: srv.URL,
	})
	token := signRS256(t, priv, kid, map[string]any{
		"iss": "https://issuer.example", "aud": "context-fabric", "sub": "u", "org_id": "o",
	})
	if _, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: token}); err == nil {
		t.Fatal("expected token without exp to be rejected")
	}
}

// TestOIDCAcceptsValidExp is the positive control for the check above.
func TestOIDCAcceptsValidExp(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)

	p := NewOIDC(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "context-fabric", JWKSURL: srv.URL,
	})
	token := signRS256(t, priv, kid, map[string]any{
		"iss": "https://issuer.example", "aud": "context-fabric", "sub": "u", "org_id": "o",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: token}); err != nil {
		t.Fatalf("expected valid token to authenticate: %v", err)
	}
}
