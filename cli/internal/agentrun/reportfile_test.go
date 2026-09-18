//go:build unix

package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func reportRequest(t *testing.T) AwaitRequest {
	t.Helper()
	return AwaitRequest{Marker: "unused", Since: time.Now().Add(-time.Minute), OutBase: filepath.Join(t.TempDir(), "b"), Timeout: 50 * time.Millisecond}
}

func TestReportFile_Complete(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	write(t, AgentReport(r.OutBase), "FINDINGS "+fakeToken+" see https://user:pw@example.com/x \x1b[2J")
	write(t, AgentDone(r.OutBase), "")
	res := reportFile{name: "antigravity"}.Await(context.Background(), r)
	if !res.Complete || res.Provider != "antigravity" || !res.Ran {
		t.Fatalf("result: %+v", res)
	}
	b, _ := os.ReadFile(res.ReportPath)
	if !strings.Contains(string(b), "FINDINGS") || strings.Contains(string(b), fakeToken) || strings.Contains(string(b), "user:pw@") || strings.ContainsRune(string(b), '\x1b') {
		t.Errorf("report not redacted: %q", b)
	}
}

func TestReportFile_SymlinkRefused(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	write(t, secret, "TOP SECRET")
	if err := os.Symlink(secret, AgentReport(r.OutBase)); err != nil {
		t.Fatal(err)
	}
	write(t, AgentDone(r.OutBase), "")
	res := reportFile{name: "antigravity"}.Await(context.Background(), r)
	if res.Complete || res.Reason != ReasonReportUnreadable || strings.Contains(res.StderrTail, "TOP SECRET") {
		t.Errorf("result: %+v", res)
	}
	if _, err := os.Lstat(res.ReportPath); err == nil {
		t.Error("no report may be written from a symlink")
	}
}

// A FIFO in place of the report must not hang the wait: the open does not block
// and anything but a regular file is refused (review of lets-w5tm5, C4).
func TestReportFile_FIFORefused(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	if err := syscall.Mkfifo(AgentReport(r.OutBase), 0o600); err != nil {
		t.Fatal(err)
	}
	write(t, AgentDone(r.OutBase), "")
	done := make(chan Result, 1)
	go func() { done <- reportFile{name: "antigravity"}.Await(context.Background(), r) }()
	select {
	case res := <-done:
		if res.Complete || res.Reason != ReasonReportUnreadable {
			t.Errorf("result: %+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the wait blocked on a FIFO")
	}
}

func TestReportFile_DoneBeforeSendIgnored(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	write(t, AgentReport(r.OutBase), "stale")
	write(t, AgentDone(r.OutBase), "")
	old := r.Since.Add(-time.Hour)
	if err := os.Chtimes(AgentDone(r.OutBase), old, old); err != nil {
		t.Fatal(err)
	}
	res := reportFile{name: "antigravity"}.Await(context.Background(), r)
	if res.Complete || res.Reason != ReasonTimeout || len(res.Warnings) != 1 {
		t.Errorf("result: %+v", res)
	}
}

func TestReportFile_Empty(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	write(t, AgentReport(r.OutBase), "  \n")
	write(t, AgentDone(r.OutBase), "")
	if res := (reportFile{name: "antigravity"}).Await(context.Background(), r); res.Reason != ReasonReportEmpty {
		t.Errorf("result: %+v", res)
	}
}

func TestReportFile_Huge(t *testing.T) {
	useClock(t)
	r := reportRequest(t)
	write(t, AgentReport(r.OutBase), strings.Repeat("x", 1<<20))
	write(t, AgentDone(r.OutBase), "")
	res := reportFile{name: "antigravity"}.Await(context.Background(), r)
	b, _ := os.ReadFile(res.ReportPath)
	if !res.Complete || len(b) > ReportCap+64 || !strings.Contains(string(b), "…[truncated") {
		t.Errorf("result %+v, %d bytes", res, len(b))
	}
}

func TestReportFile_RunUnsupported(t *testing.T) {
	res := reportFile{name: "antigravity"}.Run(context.Background(), Request{})
	if res.Ran || res.Reason != ReasonHeadlessUnsupported || res.Provider != "antigravity" {
		t.Errorf("result: %+v", res)
	}
}
