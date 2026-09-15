package fsutil

import "path/filepath"

// SameDir compares two paths after symlink resolution + Abs normalization.
// macOS /var/folders vs /private/var/folders mismatch (initRepo uses
// realTempDir to dodge this, but porcelain output is raw git data).
func SameDir(a, b string) bool {
	resolve := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	return resolve(a) == resolve(b)
}
