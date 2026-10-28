// Package update looks up the latest Netward release on GitHub when the
// user enabled it (off by default, N-04 / T-34). It never installs anything:
// a container that updates itself would need the Docker socket.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// URL is fixed: the release feed of this project and nothing else.
const URL = "https://api.github.com/repos/Pyzyi/Netward/releases/latest"

const releasePrefix = "https://github.com/Pyzyi/Netward/releases/"

// Status is shown in the settings and the app shell.
type Status struct {
	Enabled   bool   `json:"enabled"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"` // "0.2.0"
	URL       string `json:"url,omitempty"`    // release notes
	Available bool   `json:"available"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Checker asks GitHub once a day while enabled.
type Checker struct {
	client  *http.Client
	url     string
	current string

	mu      sync.Mutex
	status  Status
	trigger chan struct{}
}

// New creates a disabled checker; client must be an egress client.
func New(client *http.Client, current string) *Checker {
	return &Checker{client: client, url: URL, current: current,
		status: Status{Current: current}, trigger: make(chan struct{}, 1)}
}

// Configure switches the check on or off; switching on checks right away.
func (c *Checker) Configure(enabled bool) {
	c.mu.Lock()
	was := c.status.Enabled
	c.status.Enabled = enabled
	if !enabled {
		c.status = Status{Current: c.current}
	}
	c.mu.Unlock()
	if enabled && !was {
		select {
		case c.trigger <- struct{}{}:
		default:
		}
	}
}

// Status returns the last result.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Run checks daily while enabled until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	t := time.NewTimer(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-c.trigger:
		}
		if c.Status().Enabled {
			c.Check(ctx)
		}
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(24 * time.Hour)
	}
}

var tagRe = regexp.MustCompile(`^v?([0-9]{1,4})\.([0-9]{1,4})\.([0-9]{1,4})$`)

// Check asks GitHub now.
func (c *Checker) Check(ctx context.Context) {
	latest, page, err := c.fetch(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.status.Enabled {
		return
	}
	c.status.CheckedAt = time.Now().UnixMilli()
	if err != nil {
		c.status.Error = err.Error()
		return
	}
	c.status.Error = ""
	c.status.Latest, c.status.URL = latest, page
	c.status.Available = Newer(latest, c.current)
}

var errResponse = errors.New("unexpected response")

func (c *Checker) fetch(ctx context.Context) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Netward")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", errors.New("unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%w: %d", errResponse, resp.StatusCode)
	}
	var r struct {
		Tag  string `json:"tag_name"`
		Page string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&r); err != nil {
		return "", "", errResponse
	}
	m := tagRe.FindStringSubmatch(r.Tag)
	if m == nil || !strings.HasPrefix(r.Page, releasePrefix) || len(r.Page) > 200 {
		return "", "", errResponse
	}
	return m[1] + "." + m[2] + "." + m[3], r.Page, nil
}

// Newer reports whether release a is newer than the running version b.
// Development builds ("dev", "edge", "main") are never told to update.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	m := tagRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return out, false
	}
	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out, true
}
