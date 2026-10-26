package update

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false},
		{"0.10.0", "0.9.3", true}, {"1.0.0", "v0.9.0", true}, {"0.2.0", "dev", false}, {"0.2.0", "edge", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func checker(t *testing.T, body string, status int) *Checker {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials sent")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.Client(), "0.1.0")
	c.url = srv.URL
	c.Configure(true)
	return c
}

func TestCheck(t *testing.T) {
	c := checker(t, `{"tag_name":"v0.2.0","html_url":"https://github.com/Pyzyi/netzwart/releases/tag/v0.2.0","body":"<script>"}`, 200)
	c.Check(context.Background())
	st := c.Status()
	if !st.Available || st.Latest != "0.2.0" || st.URL != "https://github.com/Pyzyi/netzwart/releases/tag/v0.2.0" || st.Error != "" {
		t.Fatalf("unexpected %+v", st)
	}
	c.Configure(false)
	if st := c.Status(); st.Enabled || st.Latest != "" {
		t.Fatalf("disabling keeps state: %+v", st)
	}
}

func TestCheckRejectsForeignLinks(t *testing.T) {
	for _, body := range []string{
		`{"tag_name":"v0.2.0","html_url":"https://evil.example/releases/v0.2.0"}`,
		`{"tag_name":"v0.2.0; rm -rf","html_url":"https://github.com/Pyzyi/netzwart/releases/tag/x"}`,
		`not json`,
	} {
		c := checker(t, body, 200)
		c.Check(context.Background())
		if st := c.Status(); st.Available || st.URL != "" || st.Error == "" {
			t.Fatalf("accepted %s: %+v", body, st)
		}
	}
	c := checker(t, `{}`, 404)
	c.Check(context.Background())
	if st := c.Status(); st.Error == "" {
		t.Fatal("404 not reported")
	}
}
