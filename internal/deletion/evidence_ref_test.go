package deletion

import "testing"

func TestSplitEvidenceRef(t *testing.T) {
	cases := []struct{ in, key, ver string }{
		{"org/quarantine/abc@v1", "org/quarantine/abc", "v1"},
		{"ev/key#v1", "ev/key", "v1"},
		{"keyonly", "keyonly", ""},
		{"a@b#c", "a", "b#c"},
	}
	for _, c := range cases {
		k, v := splitEvidenceRef(c.in)
		if k != c.key || v != c.ver {
			t.Errorf("splitEvidenceRef(%q) = (%q,%q); want (%q,%q)", c.in, k, v, c.key, c.ver)
		}
	}
}
