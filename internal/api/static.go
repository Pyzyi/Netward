package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type asset struct {
	data, gz    []byte
	contentType string
	immutable   bool
}

// staticFiles serves the embedded single-page app. Hashed assets are cached
// forever; everything else falls back to index.html for client-side routing.
type staticFiles struct {
	fsys  fs.FS
	mu    sync.Mutex
	cache map[string]*asset
}

func newStaticFiles(fsys fs.FS) *staticFiles {
	return &staticFiles{fsys: fsys, cache: map[string]*asset{}}
}

func (s *staticFiles) load(name string) *asset {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.cache[name]; ok {
		return a
	}
	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	if strings.HasSuffix(name, ".webmanifest") {
		ct = "application/manifest+json"
	}
	a := &asset{data: data, contentType: ct, immutable: strings.HasPrefix(name, "assets/")}
	if len(data) > 1024 && (strings.HasPrefix(ct, "text/") || strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") || strings.Contains(ct, "svg")) {
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		_, _ = zw.Write(data)
		_ = zw.Close()
		a.gz = b.Bytes()
	}
	s.cache[name] = a
	return a
}

func (s *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	a := s.load(name)
	if a == nil {
		if path.Ext(name) != "" && !strings.HasSuffix(name, ".html") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		a = s.load(name)
		if a == nil {
			http.NotFound(w, r)
			return
		}
	}
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	if a.immutable {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	h.Add("Vary", "Accept-Encoding")
	body := a.data
	if a.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		body = a.gz
	}
	http.ServeContent(w, r, name, time.Time{}, bytesReader(body))
}

func bytesReader(b []byte) io.ReadSeeker { return bytes.NewReader(b) }
