// Package envfile parses .env files in the restricted format used by LETS:
// KEY=VALUE per line, # comments above keys (no inline), no multi-line values,
// no quote stripping, no variable expansion. Mirrors the whitelist parser in
// plugins/lets-workflow/hooks/session-start.sh.
package envfile

import (
	"bufio"
	"io"
	"strings"
)

// MaxValueLen caps individual values to defend against bloated context injection.
// Mirrors `head -c 200` in the bash whitelist parser.
const MaxValueLen = 200

// Parse reads .env-format key=value pairs from r and returns them as a map.
//
// Rules:
//   - Lines starting with `#` (after trim) are comments - skipped
//   - Blank lines are skipped
//   - Other lines split on first `=`; key is trimmed, value is raw (after `=`)
//   - `\r` stripped from values (CRLF defense)
//   - Values longer than MaxValueLen bytes are truncated
//   - Lines without `=` are skipped (malformed)
//   - Duplicate keys: last value wins
//
// Quote stripping and inline comments are intentionally NOT supported - this
// matches the bash whitelist parser exactly.
func Parse(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.ReplaceAll(line[eq+1:], "\r", "")
		if len(val) > MaxValueLen {
			val = val[:MaxValueLen]
		}
		out[key] = val
	}
	return out, scanner.Err()
}
