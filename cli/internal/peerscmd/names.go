package peerscmd

// ValidRole reports whether role is one of the peer roles. A worker's task id is
// validated separately with taskid.Valid.
func ValidRole(role string) bool {
	switch role {
	case "orchestrator", "peer", "worker":
		return true
	}
	return false
}
