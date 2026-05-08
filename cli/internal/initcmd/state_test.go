package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStatuslineSh(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want StatuslineState
	}{
		{"absent", nil, StatuslineAbsent},
		{"current shim", embeddedStatuslineShim, StatuslineCurrentShim},
		{"legacy bash", makeLegacyBash(), StatuslineLegacyBash},
		{"foreign script", []byte("#!/bin/bash\necho custom\n"), StatuslineForeign},
		{"foreign large no markers", make([]byte, 6000), StatuslineForeign},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "statusline.sh")
			if tt.body != nil {
				if err := os.WriteFile(path, tt.body, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectStatuslineSh(path); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func makeLegacyBash() []byte {
	body := "#!/bin/sh\n# Legacy 209-line bash statusline\n"
	body += "_fetch_usage() { :; }\n"
	body += "compute_delta() { :; }\n"
	pad := make([]byte, 5500)
	for i := range pad {
		pad[i] = '#'
	}
	body += string(pad) + "\n"
	return []byte(body)
}

func TestDetectStatusLineField(t *testing.T) {
	tests := []struct {
		name string
		json string
		want StatusLineFieldState
	}{
		{"absent", `{}`, StatusLineAbsent},
		{"empty command", `{"statusLine":{"type":"command","command":""}}`, StatusLineAbsent},
		{"managed", `{"_letsManaged":{"statusLine":true},"statusLine":{"command":"lets statusline"}}`, StatusLineLetsManaged},
		{"managed marker but command tampered", `{"_letsManaged":{"statusLine":true},"statusLine":{"command":"/etc/passwd"}}`, StatusLineForeign},
		{"managed marker but command missing", `{"_letsManaged":{"statusLine":true}}`, StatusLineForeign},
		{"direct unmanaged", `{"statusLine":{"command":"lets statusline"}}`, StatusLineLetsDirect},
		{"bash wrapper", `{"statusLine":{"command":"bash -c 'cat | bash $(git rev-parse --show-toplevel)/.lets/statusline.sh 2>/dev/null'"}}`, StatusLineLetsBashWrapper},
		{"foreign", `{"statusLine":{"command":"/usr/local/bin/my-status.sh"}}`, StatusLineForeign},
		{"managed marker false treated as foreign-or-absent", `{"_letsManaged":{"statusLine":false},"statusLine":{"command":"lets statusline"}}`, StatusLineLetsDirect},
		{"managed marker non-bool", `{"_letsManaged":{"statusLine":"yes"},"statusLine":{"command":"lets statusline"}}`, StatusLineLetsDirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
				t.Fatal(err)
			}
			if got := detectStatusLineField(m); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
