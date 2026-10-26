package vuln

import (
	"encoding/json"
	"testing"
)

func fake(t *testing.T, js string) {
	t.Helper()
	var s snapshot
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		t.Fatal(err)
	}
	old := load
	load = func() *snapshot { return &s }
	t.Cleanup(func() { load = old })
}

const sample = `{"kevCatalogVersion":"2026.10.23","entries":[
 {"cve":"CVE-2025-1","name":"A","added":"2025-01-29","url":"https://support.apple.com/x","products":{
   "ios":[{"from":"0","fixed":"18.3"},{"from":"0","fixed":"17.7.4"}],
   "macos":[{"from":"0","fixed":"15.3"},{"from":"0","fixed":"14.7.3"},{"from":"0","fixed":"10.15.7"}]}},
 {"cve":"CVE-2025-2","name":"B","added":"2025-03-11","products":{"ios":[{"from":"0","fixed":"18.3.2"}]}},
 {"cve":"CVE-2025-3","name":"C","added":"2025-04-01","products":{"tvos":[{"from":"18.0","fixed":"18.4"}]}}
]}`

func TestCheck(t *testing.T) {
	fake(t, sample)
	for _, c := range []struct {
		product, version, verdict, fixed string
		cves                             int
	}{
		{"ios", "18.2.1", Affected, "18.3.2", 2},
		{"ios", "18.3", Uncertain, "18.3.2", 0},  // "18.3" means 18.3.x: CVE-1 is fixed in every 18.3, CVE-2 maybe
		{"ios", "18.3.1", Affected, "18.3.2", 1}, // exact: CVE-2 still open
		{"ios", "18.3.2", None, "", 0},
		{"ios", "18.4", None, "", 0},
		{"ios", "17.7.3", Affected, "17.7.4", 1}, // older branch has its own fix
		{"ios", "17.7", Uncertain, "17.7.4", 0},  // depends on the patch level
		{"ios", "16.7.10", None, "", 0},          // branch not listed: no verdict either way
		{"ios", "18", Unknown, "", 0},            // major only
		{"macos", "14.7.2", Affected, "14.7.3", 1},
		{"macos", "15.0", Affected, "15.3", 1},
		{"macos", "10.15.6", Affected, "10.15.7", 1},
		{"macos", "10.14.6", None, "", 0}, // 10.14 is its own branch
		{"tvos", "17.5", None, "", 0},     // below the range's lower bound
		{"tvos", "18.1", Affected, "18.4", 1},
		{"windows", "10.0.19045", Unknown, "", 0},
		{"ios", "18.x", Unknown, "", 0},
	} {
		r := Check(c.product, c.version)
		if r.Verdict != c.verdict || r.Fixed != c.fixed || len(r.CVEs) != c.cves {
			t.Errorf("%s %s: got %+v, want %s fixed %q with %d CVEs", c.product, c.version, r, c.verdict, c.fixed, c.cves)
		}
	}
	if r := Check("ios", "18.2"); r.CVEs[0].ID != "CVE-2025-2" {
		t.Errorf("newest first: %+v", r.CVEs)
	}
	if DataAsOf() != "2026-10-23" {
		t.Error(DataAsOf())
	}
}

// The embedded snapshot loads and knows the iOS 18.3 fix of CVE-2025-24085.
func TestEmbeddedSnapshot(t *testing.T) {
	s := load()
	if len(s.Entries) < 50 || s.KEVCatalogVersion == "" {
		t.Fatalf("snapshot has %d entries", len(s.Entries))
	}
	r := Check("ios", "18.2.1")
	found := false
	for _, c := range r.CVEs {
		found = found || c.ID == "CVE-2025-24085"
	}
	if r.Verdict != Affected || !found {
		t.Fatalf("iOS 18.2.1: %+v", r)
	}
	if r := Check("ios", "99.0.0"); r.Verdict != None {
		t.Fatalf("future version: %+v", r)
	}
}
