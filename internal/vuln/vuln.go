// Package vuln matches detected system versions against an offline
// snapshot of known exploited vulnerabilities (docs/09, F6): the CISA KEV
// catalogue with the affected version ranges from the CVE records,
// generated per release by tools/kevgen. There is no online lookup.
//
// A verdict needs an exact version: at least major and minor ("tvOS 26.4").
// Versions are compared within their branch only, because older branches
// get their own fixes ("fixed in 15.3, 14.7.3 and 13.7.3"). Nothing here
// ever claims a device is free of vulnerabilities; it only knows the
// exploited ones in the snapshot.
package vuln

import (
	_ "embed"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed kev.json
var raw []byte

// Verdicts.
const (
	Affected  = "affected"  // a known exploited hole is open in this version
	Uncertain = "uncertain" // depends on the patch level, which is not known ("17.1")
	None      = "none"      // none of the known exploited holes applies; not a promise of safety
	Unknown   = "unknown"   // version too vague or product not covered
)

// CVE is one known exploited vulnerability.
type CVE struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Added string `json:"added"`
	URL   string `json:"url,omitempty"`
}

// Result is the outcome for one product and version.
type Result struct {
	Verdict string `json:"verdict"`
	Fixed   string `json:"fixed,omitempty"` // version that closes every matching hole in this branch
	CVEs    []CVE  `json:"cves,omitempty"`
}

type rng struct {
	From  string `json:"from"`
	Fixed string `json:"fixed"`
}

type snapshot struct {
	KEVCatalogVersion string `json:"kevCatalogVersion"`
	Entries           []struct {
		CVE      string           `json:"cve"`
		Name     string           `json:"name"`
		Added    string           `json:"added"`
		URL      string           `json:"url"`
		Products map[string][]rng `json:"products"`
	} `json:"entries"`
}

var load = sync.OnceValue(func() *snapshot {
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		panic("vuln: embedded snapshot is invalid: " + err.Error())
	}
	return &s
})

// DataAsOf returns the date of the KEV catalogue in the snapshot (YYYY-MM-DD).
func DataAsOf() string {
	return strings.ReplaceAll(load().KEVCatalogVersion, ".", "-")
}

// Products lists the product ids the snapshot covers.
var Products = []string{"ios", "ipados", "macos", "tvos"}

// parse splits "18.6.2" into numbers; nil if it is not a plain version.
func parse(s string) []int {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) > 4 {
		return nil
	}
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || len(p) > 6 {
			return nil
		}
		out[i] = n
	}
	return out
}

// cmp compares versions; missing parts count as 0.
func cmp(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// branch identifies the release line: the major version, and for macOS 10
// also the minor one (10.14, 10.15 were separate systems).
func branch(product string, v []int) [2]int {
	if product == "macos" && v[0] == 10 && len(v) > 1 {
		return [2]int{10, v[1]}
	}
	return [2]int{v[0], -1}
}

// Check matches product and version against the snapshot.
func Check(product, version string) Result {
	v := parse(version)
	if !slices.Contains(Products, product) || len(v) < 2 {
		return Result{Verdict: Unknown}
	}
	exact := len(v) >= 3
	res := Result{Verdict: None}
	var fixed []int
	uncertain := false
	for _, e := range load().Entries {
		hit, maybe := false, false
		var fix []int
		for _, r := range e.Products[product] {
			f, from := parse(r.Fixed), parse(r.From)
			if f == nil || from == nil || branch(product, f) != branch(product, v) || cmp(v, from) < 0 {
				continue
			}
			switch {
			case cmp(v, f) >= 0:
				// fixed in this version
			case exact || cmp(v[:2], f[:min(2, len(f))]) < 0:
				hit = true // even the highest patch level of "17.1" is below the fix
			default:
				maybe = true // "17.1" vs fix 17.1.2: depends on the patch level
			}
			if fix == nil || cmp(f, fix) > 0 {
				fix = f
			}
		}
		switch {
		case hit:
			res.CVEs = append(res.CVEs, CVE{ID: e.CVE, Name: e.Name, Added: e.Added, URL: e.URL})
			if fixed == nil || cmp(fix, fixed) > 0 {
				fixed = fix
			}
		case maybe:
			uncertain = true
			if fixed == nil || cmp(fix, fixed) > 0 {
				fixed = fix
			}
		}
	}
	switch {
	case len(res.CVEs) > 0:
		res.Verdict = Affected
	case uncertain:
		res.Verdict = Uncertain
	}
	if res.Verdict != None {
		parts := make([]string, len(fixed))
		for i, n := range fixed {
			parts[i] = strconv.Itoa(n)
		}
		res.Fixed = strings.Join(parts, ".")
	}
	// Newest first: the most recent ones matter most.
	slices.SortFunc(res.CVEs, func(a, b CVE) int { return strings.Compare(b.Added+b.ID, a.Added+a.ID) })
	return res
}
