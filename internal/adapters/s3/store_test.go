package s3store

import (
	"strings"
	"testing"

	"github.com/xsama/context-fabric/internal/platform"
)

func TestCopyCappedRejectsOversized(t *testing.T) {
	_, _, err := copyCapped(strings.NewReader("hello-world"), 4)
	if err == nil {
		t.Fatal("expected oversized error")
	}
	ae, ok := platform.AsAPIError(err)
	if !ok || ae.ReasonCode != "validation_failed" {
		t.Fatalf("want validation error, got %v", err)
	}
}

func TestCopyCappedAcceptsExactMax(t *testing.T) {
	data, hash, err := copyCapped(strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcd" {
		t.Fatalf("data=%q", data)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash=%s", hash)
	}
}
