package s3store

import "testing"

func TestEvidenceTier(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"org1/quarantine/sha256:abc", "quarantine"},
		{"org1/raw/doc.pdf", "raw"},
		{"org1/derived/chunk-1", "derived"},
		{"org1/unknown/x", "quarantine"},
		{"bare-key", "quarantine"},
	}
	for _, tc := range tests {
		if got := evidenceTier(tc.key); got != tc.want {
			t.Errorf("evidenceTier(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}
