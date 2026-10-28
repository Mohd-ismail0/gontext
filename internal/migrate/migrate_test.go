package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestEmbeddedMigrationsLoadedAndSorted(t *testing.T) {
	files, err := loadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no embedded migrations found")
	}
	if !sort.SliceIsSorted(files, func(i, j int) bool { return files[i].Version < files[j].Version }) {
		t.Fatal("embedded migrations are not sorted by version")
	}
	var found bool
	for _, f := range files {
		if f.Version != "009_legal_holds.sql" {
			continue
		}
		found = true
		// Regression guard for the tenant-isolation fix: RLS on legal_holds must
		// be enforced unconditionally, not gated behind the app role existing.
		for _, want := range []string{
			"ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE legal_holds FORCE ROW LEVEL SECURITY",
		} {
			if !strings.Contains(f.SQL, want) {
				t.Fatalf("009 must contain %q", want)
			}
		}
	}
	if !found {
		t.Fatal("009_legal_holds.sql missing from embedded migrations")
	}
}

// TestEmbeddedMigrationsMatchTopLevel mirrors the CI `diff -qr` gate: the
// embedded copy the binary ships must match the top-level migrations/ directory.
func TestEmbeddedMigrationsMatchTopLevel(t *testing.T) {
	files, err := loadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("top-level migrations dir not available: %v", err)
	}
	onDisk := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		onDisk[e.Name()] = string(b)
	}
	if len(onDisk) != len(files) {
		t.Fatalf("embedded=%d top-level=%d migrations", len(files), len(onDisk))
	}
	for _, f := range files {
		want, ok := onDisk[f.Version]
		if !ok {
			t.Fatalf("embedded migration %s missing from top-level dir", f.Version)
		}
		if normalizeNewlines(want) != normalizeNewlines(f.SQL) {
			t.Fatalf("embedded %s differs from top-level copy", f.Version)
		}
	}
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
