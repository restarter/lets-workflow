package envfile_test

import (
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "empty",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "comments and blanks only",
			input: "# comment\n\n  # indented comment\n",
			want:  map[string]string{},
		},
		{
			name:  "single key value",
			input: "KEY=value\n",
			want:  map[string]string{"KEY": "value"},
		},
		{
			name:  "multiple keys",
			input: "A=1\nB=2\nC=3\n",
			want:  map[string]string{"A": "1", "B": "2", "C": "3"},
		},
		{
			name:  "comment above key (canonical format)",
			input: "# Response language\nLETS_LANGUAGE=English\n",
			want:  map[string]string{"LETS_LANGUAGE": "English"},
		},
		{
			name:  "inline hash is part of value (not a comment)",
			input: "KEY=value # not a comment\n",
			want:  map[string]string{"KEY": "value # not a comment"},
		},
		{
			name:  "CRLF stripped from value",
			input: "KEY=value\r\n",
			want:  map[string]string{"KEY": "value"},
		},
		{
			name:  "key whitespace trimmed",
			input: "  KEY  =value\n",
			want:  map[string]string{"KEY": "value"},
		},
		{
			name:  "value preserves trailing whitespace except CR",
			input: "KEY=value  \n",
			want:  map[string]string{"KEY": "value  "},
		},
		{
			name:  "duplicate key - last wins",
			input: "KEY=first\nKEY=second\n",
			want:  map[string]string{"KEY": "second"},
		},
		{
			name:  "malformed line skipped",
			input: "KEY=value\nbadline_no_equals\nOTHER=ok\n",
			want:  map[string]string{"KEY": "value", "OTHER": "ok"},
		},
		{
			name:  "empty value preserved",
			input: "KEY=\nOTHER=ok\n",
			want:  map[string]string{"KEY": "", "OTHER": "ok"},
		},
		{
			name:  "value with equals sign",
			input: "KEY=a=b=c\n",
			want:  map[string]string{"KEY": "a=b=c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := envfile.Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("len(got)=%d want %d; got=%v want=%v", len(got), len(tt.want), got, tt.want)
			}
			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestParse_TruncatesLongValues(t *testing.T) {
	long := strings.Repeat("x", envfile.MaxValueLen+50)
	input := "KEY=" + long + "\n"
	got, err := envfile.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if l := len(got["KEY"]); l != envfile.MaxValueLen {
		t.Errorf("value length = %d, want %d (truncated)", l, envfile.MaxValueLen)
	}
}
