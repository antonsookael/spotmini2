package app

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.10", "v1.9", true},   // the lexical trap
		{"v1.9", "v1.10", false},  // and its reverse
		{"v1.10", "v1.10", false}, // same build
		{"v1.10.1", "v1.10", true},
		{"v1.10", "v1.10.0", false},
		{"v1.10", "v1.10.1", false},
		{"v2.0", "v1.99.99", true},
		{"v1.6.1", "v1.6", true},
		{"v1.7", "v1.6.1", true},
	}

	for _, tc := range cases {
		got, err := isNewer(tc.candidate, tc.current)
		if err != nil {
			t.Errorf("isNewer(%q, %q) errored: %v", tc.candidate, tc.current, err)
			continue
		}
		if got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}

	// A malformed tag must error rather than quietly compare as zero,
	// which would read as "no update" forever.
	for _, bad := range []string{"", "v", "dev", "v1.x", "latest"} {
		if _, err := isNewer(bad, "v1.10"); err == nil {
			t.Errorf("isNewer(%q, ...) should have errored", bad)
		}
	}
}
