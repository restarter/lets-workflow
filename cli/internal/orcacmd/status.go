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
	// lets-52jph: running=false always names its reason - a consumer (/lets:hub)
	// must never see a false negative with nothing to report.
	if !info.Running && info.Reason == "" {
		info.Reason = ReasonError
	}
	res.Status = &info
	return res, nil
}
