//go:build unix

package orcacmd

import "context"

// Status reports whether Orca is usable. Never hard-fails.
func Status(ctx context.Context) (*StatusResult, error) {
	res := &StatusResult{Envelope: Envelope{SchemaVersion: SchemaVersion, OK: true, Subcommand: "status", Steps: []Step{}}}
	c, f := NewClient()
	if f != nil {
		res.Status = &StatusInfo{Reason: f.Reason}
		return res, nil
	}
	info, f := c.Status(ctx)
	if f != nil {
		info.Reason = f.Reason
	}
	res.Status = &info
	return res, nil
}
