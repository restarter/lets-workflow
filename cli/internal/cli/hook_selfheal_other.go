//go:build !unix

package cli

// selfHealFn is a no-op off unix: `lets worktree adopt` needs flock and POSIX
// symlinks (the worktree.go / worktree_stub.go split).
var selfHealFn = func(root, rulesPath string) string { return "" }
