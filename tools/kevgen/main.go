// Command kevgen builds the offline snapshot of known exploited
// vulnerabilities embedded by internal/vuln (docs/09, F6): the CISA KEV
// catalogue plus, for each entry of a product Netward can recognise, the
// affected version ranges from the CVE List v5 record.
//
// Both sources are read at fixed commits, so the same command always
// produces the same file and CI can check that the committed file is
// genuine:
//
//	go run ./tools/kevgen -kev <commit> -cve <commit> -out internal/vuln/kev.json
//	go run ./tools/kevgen -verify internal/vuln/kev.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	kevURL = "https://raw.githubusercontent.com/cisagov/kev-data/%s/known_exploited_vulnerabilities.json"
	cveURL = "https://raw.githubusercontent.com/CVEProject/cvelistV5/%s/cves/%s/%sxxx/%s.json"
)

// products maps CVE product names to Netward's product ids.
var products = map[string][]string{
	"ios and ipados": {"ios", "ipados"},
	"ios":            {"ios"},
	"ipados":         {"ipados"},
	"macos":          {"macos"},
	"tvos":           {"tvos"},
}

// Snapshot is the file format (shared with internal/vuln).
type Snapshot struct {
	KEVCatalogVersion string  `json:"kevCatalogVersion"`
	KEVCommit         string  `json:"kevCommit"`
	CVECommit         string  `json:"cveCommit"`
	Entries           []Entry `json:"entries"`
}

// Entry is one exploited vulnerability with the versions it affects.
type Entry struct {
	CVE      string             `json:"cve"`
	Name     string             `json:"name"`
	Added    string             `json:"added"`
	URL      string             `json:"url,omitempty"`
	Products map[string][]Range `json:"products"`
}

// Range: versions from From (inclusive, "0" = any) up to Fixed (exclusive).
type Range struct {
	From  string `json:"from"`
	Fixed string `json:"fixed"`
}

var (
	commitRE  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	cveRE     = regexp.MustCompile(`^CVE-(\d{4})-(\d{4,7})$`)
	versionRE = regexp.MustCompile(`^\d+(\.\d+){0,3}$`)
	client    = &http.Client{Timeout: 30 * time.Second}
)

func get(u string) ([]byte, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func main() {
	kevCommit := flag.String("kev", "", "commit of github.com/cisagov/kev-data")
	cveCommit := flag.String("cve", "", "commit of github.com/CVEProject/cvelistV5")
	out := flag.String("out", "", "file to write")
	verify := flag.String("verify", "", "regenerate the given file from its own commits and compare")
	flag.Parse()
	if *verify != "" {
		old, err := os.ReadFile(*verify)
		if err != nil {
			log.Fatal(err)
		}
		var s Snapshot
		if err := json.Unmarshal(old, &s); err != nil {
			log.Fatal(err)
		}
		fresh, err := build(s.KEVCommit, s.CVECommit)
		if err != nil {
			log.Fatal(err)
		}
		if !bytes.Equal(old, fresh) {
			log.Fatalf("%s does not match its sources (kev %s, cve %s)", *verify, s.KEVCommit, s.CVECommit)
		}
		fmt.Println("ok:", *verify, "matches its sources")
		return
	}
	if !commitRE.MatchString(*kevCommit) || !commitRE.MatchString(*cveCommit) || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	data, err := build(*kevCommit, *cveCommit)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}
}

func build(kevCommit, cveCommit string) ([]byte, error) {
	if !commitRE.MatchString(kevCommit) || !commitRE.MatchString(cveCommit) {
		return nil, fmt.Errorf("invalid commit")
	}
	raw, err := get(fmt.Sprintf(kevURL, kevCommit))
	if err != nil {
		return nil, err
	}
	var kev struct {
		CatalogVersion  string `json:"catalogVersion"`
		Vulnerabilities []struct {
			CVE    string `json:"cveID"`
			Vendor string `json:"vendorProject"`
			Name   string `json:"vulnerabilityName"`
			Added  string `json:"dateAdded"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(raw, &kev); err != nil {
		return nil, err
	}
	s := Snapshot{KEVCatalogVersion: kev.CatalogVersion, KEVCommit: kevCommit, CVECommit: cveCommit, Entries: []Entry{}}
	for _, v := range kev.Vulnerabilities {
		if !strings.EqualFold(v.Vendor, "Apple") {
			continue
		}
		m := cveRE.FindStringSubmatch(v.CVE)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		rec, err := get(fmt.Sprintf(cveURL, cveCommit, m[1], strconv.Itoa(n/1000), v.CVE))
		if err != nil {
			return nil, err
		}
		e, ok := entry(rec)
		if !ok {
			continue
		}
		e.CVE, e.Name, e.Added = v.CVE, v.Name, v.Added
		s.Entries = append(s.Entries, e)
	}
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].CVE < s.Entries[j].CVE })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// entry extracts the affected ranges of the products Netward knows.
func entry(rec []byte) (Entry, bool) {
	var r struct {
		Containers struct {
			CNA struct {
				Affected []struct {
					Vendor   string `json:"vendor"`
					Product  string `json:"product"`
					Versions []struct {
						Version  string `json:"version"`
						Status   string `json:"status"`
						LessThan string `json:"lessThan"`
					} `json:"versions"`
				} `json:"affected"`
				References []struct {
					URL string `json:"url"`
				} `json:"references"`
			} `json:"cna"`
		} `json:"containers"`
	}
	if err := json.Unmarshal(rec, &r); err != nil {
		return Entry{}, false
	}
	e := Entry{Products: map[string][]Range{}}
	for _, a := range r.Containers.CNA.Affected {
		ids := products[strings.ToLower(strings.TrimSpace(a.Product))]
		if !strings.EqualFold(a.Vendor, "Apple") || ids == nil {
			continue
		}
		for _, v := range a.Versions {
			if v.Status != "affected" || !versionRE.MatchString(v.LessThan) {
				continue
			}
			from := v.Version
			if !versionRE.MatchString(from) {
				from = "0" // "unspecified": every earlier version
			}
			for _, id := range ids {
				e.Products[id] = append(e.Products[id], Range{From: from, Fixed: v.LessThan})
			}
		}
	}
	if len(e.Products) == 0 {
		return Entry{}, false
	}
	for id, rs := range e.Products {
		sort.Slice(rs, func(i, j int) bool { return rs[i].Fixed < rs[j].Fixed })
		e.Products[id] = dedupe(rs)
	}
	// The vendor's advisory: only https links to Apple's support site.
	for _, ref := range r.Containers.CNA.References {
		if u, err := url.Parse(ref.URL); err == nil && u.Scheme == "https" && u.Host == "support.apple.com" {
			e.URL = u.String()
			break
		}
	}
	return e, true
}

func dedupe(rs []Range) []Range {
	out := rs[:0]
	for i, r := range rs {
		if i == 0 || r != rs[i-1] {
			out = append(out, r)
		}
	}
	return out
}
