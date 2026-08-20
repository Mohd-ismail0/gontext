package httpapi

import "testing"

func TestSkipHMACRejectedByDefault(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "")
	t.Setenv("CONTEXT_FABRIC_PROFILE", "demo")
	t.Setenv("PROFILE", "")
	if allowSkipHMAC() {
		t.Fatal("skip HMAC must be disabled by default")
	}
}

func TestSkipHMACRejectedOutsideDemo(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "1")
	t.Setenv("CONTEXT_FABRIC_PROFILE", "xsama")
	t.Setenv("PROFILE", "")
	if allowSkipHMAC() {
		t.Fatal("skip HMAC must not apply outside demo/memory")
	}
}

func TestSkipHMACAllowedDemoWithEnv(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "1")
	t.Setenv("CONTEXT_FABRIC_PROFILE", "demo")
	t.Setenv("PROFILE", "")
	if !allowSkipHMAC() {
		t.Fatal("expected allow when env=1 and profile=demo")
	}
}

func TestSkipHMACAllowedMemoryWithEnv(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "1")
	t.Setenv("CONTEXT_FABRIC_PROFILE", "memory")
	t.Setenv("PROFILE", "")
	if !allowSkipHMAC() {
		t.Fatal("expected allow when env=1 and profile=memory")
	}
}

func TestSkipHMACHeaderIgnoredWithoutAllow(t *testing.T) {
	t.Setenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC", "")
	t.Setenv("CONTEXT_FABRIC_PROFILE", "demo")
	headerSet := true // X-Context-Fabric-Skip-HMAC=1
	if headerSet && allowSkipHMAC() {
		t.Fatal("header must not enable SkipHMAC without CONTEXT_FABRIC_ALLOW_SKIP_HMAC=1")
	}
}
