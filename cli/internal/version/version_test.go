package version_test

import (
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

func TestIsDev(t *testing.T) {
	origVersion := version.Version
	t.Cleanup(func() { version.Version = origVersion })

	cases := []struct {
		v    string
		want bool
	}{
		// dev sentinels (positive)
		{"dev", true},
		{"dev-feat", true},
		{"dev-feat-abc1234", true},
		{"dev-feat-abc1234-dirty", true},
		{"dev-x", true}, // minimal non-empty suffix

		// negative — must NOT be treated as dev
		{"", false},
		{"dev-", false},        // empty suffix rejected by len guard
		{"deviation", false},   // no dash separator
		{"vdev", false},        // wrong prefix
		{"0.5.4", false},       // semver
		{"0.6.0-rc.1", false},  // release rc tag (no dev- prefix after v-strip)
		{"0.6.0-dev.1", false}, // dot, not dash after dev
	}
	for _, c := range cases {
		c := c
		t.Run(c.v, func(t *testing.T) {
			version.Version = c.v
			if got := version.IsDev(); got != c.want {
				t.Errorf("IsDev() for Version=%q: got %v, want %v", c.v, got, c.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "legacy"},
		{"dev", "dev"},
		{"dev-feat", "dev-feat"},
		{"dev-feat-abc1234", "dev-feat-abc1234"},
		{"dev-feat-abc1234-dirty", "dev-feat-abc1234-dirty"},
		{"dev-", "vdev-"},           // not a valid dev suffix → v-prefixed
		{"deviation", "vdeviation"}, // not dev- prefix → v-prefixed
		{"0.5.4", "v0.5.4"},
		{"0.6.0-rc.1", "v0.6.0-rc.1"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			if got := version.Format(c.in); got != c.want {
				t.Errorf("Format(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
