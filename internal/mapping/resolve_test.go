package mapping

import "testing"

// TestResolveSingleQuoteDoesNotPanic covers a client-supplied mapping spec whose
// expression is a lone quote character, which previously panicked on expr[1:0].
func TestResolveSingleQuoteDoesNotPanic(t *testing.T) {
	for _, in := range []string{`"`, `'`} {
		if _, err := resolve(in, map[string]any{}); err != nil {
			t.Fatalf("resolve(%q) unexpected error: %v", in, err)
		}
	}
}

func TestResolveQuotedLiteral(t *testing.T) {
	got, err := resolve(`"hello"`, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %v, want hello", got)
	}
}
