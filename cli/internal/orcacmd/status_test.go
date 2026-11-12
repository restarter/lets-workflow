//go:build unix

package orcacmd

import (
	"context"
	"testing"
)

// lets-52jph: every status path reports running=true, or running=false with a
// non-empty reason.
func TestStatus_FalseAlwaysHasReason(t *testing.T) {
	cases := []struct {
		name, stdout, stderr string
		fail                 bool
		want                 string
	}{
		{"running", `{"ok":true,"result":{"app":{"running":true},"runtime":{"appVersion":"1.4.241"}}}`, "", false, ""},
		{"app not running", `{"ok":true,"result":{"app":{"running":false}}}`, "", false, ReasonAppNotRunning},
		{"unrecognized", `{"ok":true,"result":{}}`, "", false, ReasonStatusUnrecognized},
		{"verb failed", "", "boom", true, ReasonError},
		{"ok:false envelope", `{"ok":false,"error":{"code":"x","message":"y"}}`, "", false, ReasonError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFakeOrca(t, func([]string) (string, string, bool) { return tc.stdout, tc.stderr, tc.fail })
			res, err := Status(context.Background())
			if err != nil || res.Status == nil {
				t.Fatalf("Status: %+v, %v", res, err)
			}
			if res.Status.Running != (tc.want == "") || res.Status.Reason != tc.want {
				t.Errorf("running=%v reason=%q, want reason %q", res.Status.Running, res.Status.Reason, tc.want)
			}
		})
	}
	t.Run("orca absent", func(t *testing.T) {
		old := lookOrca
		lookOrca = func() (string, bool) { return "", false }
		defer func() { lookOrca = old }()
		res, _ := Status(context.Background())
		if res.Status == nil || res.Status.Running || res.Status.Reason != ReasonNotFound {
			t.Errorf("absent: %+v", res.Status)
		}
	})
}
