//go:build unix

package orcacmd

import (
	"strings"
	"testing"
)

func TestValidHandle(t *testing.T) {
	for _, h := range []string{"term_597df510-648e-4e9d-98dd-d789a6e8b607", "a:b.c_d-e"} {
		if !ValidHandle(h) {
			t.Errorf("ValidHandle(%q) = false", h)
		}
	}
	for _, h := range []string{"", "term x", "term;rm", "$(x)", strings.Repeat("a", 129), "a\nb"} {
		if ValidHandle(h) {
			t.Errorf("ValidHandle(%q) = true", h)
		}
	}
}
