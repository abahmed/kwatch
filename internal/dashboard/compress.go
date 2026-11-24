package dashboard

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// gzipMinBytes is the smallest body worth compressing: below it the
// gzip header costs about what it saves.
const gzipMinBytes = 512

// gzipTagSuffix marks the entity tag of a compressed body: the same
// resource in another encoding is another representation, so its tag
// differs, and a tag sent back in If-None-Match is read without it.
const gzipTagSuffix = "-gz"

var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return w
}}

// acceptsGzip reports whether the client takes a gzip body.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"),
		",") {
		name, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			q = strings.ReplaceAll(q, " ", "")
			return q != "q=0" && q != "q=0.0" && q != "q=0.00" &&
				q != "q=0.000"
		}
	}
	return false
}

// compressible reports whether a content type is text worth
// compressing. Live streams never are: each event must leave at once.
func compressible(contentType string) bool {
	ct, _, _ := strings.Cut(contentType, ";")
	switch strings.TrimSpace(ct) {
	case "text/html", "text/css", "text/javascript", "image/svg+xml",
		"application/json", "application/manifest+json":
		return true
	}
	return false
}

// compress gzips text responses (pages, scripts, styles, JSON) for a
// client that accepts it. Responses vary on Accept-Encoding; entity tags
// of compressed bodies carry gzipTagSuffix, and If-None-Match is read
// without it, so a poll still gets its 304. The live stream (/api/events)
// passes through untouched.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/events" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			r.Header.Set("If-None-Match",
				strings.ReplaceAll(inm, gzipTagSuffix+`"`, `"`))
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

// gzipWriter holds the start of the body until it knows whether to
// compress: only a 200 of a compressible type, not already encoded and
// at least gzipMinBytes long.
type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
	status  int
	pending []byte
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.decided || g.status != 0 {
		return
	}
	g.status = code
	if code == http.StatusNotModified {
		g.tagGzip()
	}
	if code != http.StatusOK {
		g.decide(false)
	}
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if g.status == 0 {
		g.status = http.StatusOK
	}
	if !g.decided {
		g.pending = append(g.pending, b...)
		if len(g.pending) >= gzipMinBytes {
			g.decide(g.wantsGzip())
		}
		return len(b), nil
	}
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// wantsGzip reports whether the response is one to compress.
func (g *gzipWriter) wantsGzip() bool {
	h := g.ResponseWriter.Header()
	return g.status == http.StatusOK && h.Get("Content-Encoding") == "" &&
		compressible(h.Get("Content-Type"))
}

// decide sends the header, compressed or not, and what was held back.
func (g *gzipWriter) decide(zip bool) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.ResponseWriter.Header()
	if zip {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.tagGzip()
		g.gz = gzipPool.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
	}
	if g.status == 0 {
		g.status = http.StatusOK
	}
	g.ResponseWriter.WriteHeader(g.status)
	if len(g.pending) > 0 {
		_, _ = g.Write(g.pending[:len(g.pending):len(g.pending)])
	}
	g.pending = nil
}

// tagGzip marks the entity tag as the compressed body's. A 304 to a
// client that takes gzip carries the same tag as its 200 did.
func (g *gzipWriter) tagGzip() {
	h := g.ResponseWriter.Header()
	if tag := h.Get("ETag"); strings.HasSuffix(tag, `"`) &&
		!strings.HasSuffix(tag, gzipTagSuffix+`"`) {
		h.Set("ETag", strings.TrimSuffix(tag, `"`)+gzipTagSuffix+`"`)
	}
}

// Flush sends what is written so far.
func (g *gzipWriter) Flush() {
	g.decide(g.wantsGzip())
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection.
func (g *gzipWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

// finish ends the response: a short body goes out as it is.
func (g *gzipWriter) finish() {
	if !g.decided {
		if g.status == 0 && len(g.pending) == 0 {
			return
		}
		g.decide(false)
	}
	if g.gz != nil {
		_ = g.gz.Close()
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}
