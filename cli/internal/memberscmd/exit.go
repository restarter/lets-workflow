//go:build unix

package memberscmd

// Exit codes for `lets members` (the 40-49 range; worktreecmd owns 27-34,
// integratecmd 50-59).
const (
	ExitOK                  = 0
	ExitGeneric             = 1
	ExitUsage               = 2
	ExitInvalidScope        = 40 // --scope is neither a team name nor run-<RUN>
	ExitNameInvalid         = 41 // --name fails the grammar, or names no member of the scope
	ExitNameLive            = 42 // a member of that name is live, rotated or unknown
	ExitRoleNotAllowed      = 43 // --role is not a shipped lets:* agent (actor excluded)
	ExitLeadHeld            = 44 // another session holds the lead and is live, rotated or unknown
	ExitNoLead              = 45 // the scope has no recorded lead
	ExitLockBusy            = 46 // members-<scope>.lock stayed busy past the deadline
	ExitRegistryUnavailable = 47 // no session id, no registry, or this session is not in it
	ExitIOError             = 49 // the registry file could not be read, parsed or written
)
