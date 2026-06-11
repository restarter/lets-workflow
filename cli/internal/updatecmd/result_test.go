package updatecmd

import (
	"encoding/json"
	"testing"
)

func TestResult_AddCounters(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate})
	r.Add(Artifact{Name: "rules", Status: StatusUpdated})
	r.Add(Artifact{Name: "binary", Status: StatusOutdated})
	r.Add(Artifact{Name: "plugin", Status: StatusUnknown})
	got := r.Summary
	want := Summary{UpToDate: 1, Updated: 1, ActionNeeded: 1, Unknown: 1}
	if got != want {
		t.Fatalf("Summary = %+v, want %+v", got, want)
	}
	if len(r.Artifacts) != 4 {
		t.Fatalf("len(Artifacts) = %d, want 4", len(r.Artifacts))
	}
}

func TestResult_InSyncCountsAsUpToDate(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Add(Artifact{Name: ".env", Status: StatusInSync})
	r.Add(Artifact{Name: "binary", Status: StatusUpToDate})
	// Both fold into the "in sync" bucket (Summary.UpToDate).
	if r.Summary.UpToDate != 2 {
		t.Fatalf("UpToDate = %d, want 2 (in-sync + up-to-date)", r.Summary.UpToDate)
	}
}

func TestResult_AddCounters_NotInitializedCountsAsAction(t *testing.T) {
	r := NewResult("/p", "/plug")
	r.Add(Artifact{Name: ".env", Status: StatusNotInitialized})
	r.Add(Artifact{Name: "binary", Status: StatusDev})
	r.Add(Artifact{Name: "plugin", Status: StatusAhead})
	if r.Summary.ActionNeeded != 1 {
		t.Errorf("ActionNeeded = %d, want 1", r.Summary.ActionNeeded)
	}
	if r.Summary.Unknown != 2 {
		t.Errorf("Unknown = %d, want 2 (dev + ahead)", r.Summary.Unknown)
	}
}

// TestResult_SchemaContract pins the JSON shape. Bumping SchemaVersion or
// renaming/removing a field must touch this test on purpose (mirrors
// initcmd.TestResult_SchemaContract).
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 2 {
		t.Fatalf("SchemaVersion changed to %d - update consumers (commands/update.md) and this test", SchemaVersion)
	}
	r := NewResult("/p", "/plug")
	r.OK = true
	r.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: "0.6.0"})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema_version", "ok", "project_root", "plugin_root", "artifacts", "consistent", "summary"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing top-level JSON key %q", k)
		}
	}
	arts, ok := m["artifacts"].([]any)
	if !ok || len(arts) != 1 {
		t.Fatalf("artifacts shape = %#v", m["artifacts"])
	}
	a0, _ := arts[0].(map[string]any)
	for _, k := range []string{"name", "status", "current_version"} {
		if _, ok := a0[k]; !ok {
			t.Errorf("missing artifact JSON key %q", k)
		}
	}
}

// Bucket-sum invariant: every status increments exactly one Summary bucket.
// Ranges over allStatuses (result.go, adjacent to the consts) so a NEW status
// added there automatically enters the invariant - an uncounted status would
// desync Summary from len(Artifacts).
func TestResult_AddBucketSum(t *testing.T) {
	var r Result
	for _, s := range allStatuses {
		r.Add(Artifact{Name: "x", Status: s})
	}
	sum := r.Summary.UpToDate + r.Summary.Updated + r.Summary.ActionNeeded + r.Summary.Unknown
	if sum != len(r.Artifacts) {
		t.Fatalf("summary buckets sum %d != %d artifacts - a status is uncounted in Add()", sum, len(r.Artifacts))
	}
}
