package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.2.1", "v0.2.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0.0", "v0.99.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.1.9", "v0.2.0", false},
		{"v0.2.0", "dev", false}, // a dev build is never offered an "update"
		{"v0.2.0-rc1", "v0.1.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
