package api

// HTTP transport policy: response compression + Cache-Control.
//
// Caching here is deliberately asymmetric, because the two frontends have
// opposite requirements:
//
//   - /admin/assets/* are CONTENT-HASHED by Vite, so a given URL's bytes can
//     never change → cache them for a year as `immutable`. A rebuild produces a
//     new filename, so there is no staleness risk and repeat visits cost nothing.
//
//   - web/ (the alert client) is NOT content-hashed — it is served under stable
//     names like /app.js and /verify.js. With no explicit Cache-Control a browser
//     falls back to HEURISTIC freshness (commonly ~10% of the time since
//     Last-Modified), which could serve a WEEKS-STALE alert client that never
//     revalidates. For a life-safety channel that is unacceptable: a fix to
//     verify.js or the accept gate must reach devices on the next load. So we set
//     an explicit `no-cache`, which still allows a cheap 304 via Last-Modified but
//     forbids using the cached copy without asking first.
//
// Anything carrying secrets or per-request state (every JSON API response, and
// /pubkey, which hands out the trust anchor plus the broker password) is
// `no-store` so it never lands on disk.

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

const (
	// cacheImmutable is for content-hashed assets: safe to keep for a year.
	cacheImmutable = "public, max-age=31536000, immutable"
	// cacheRevalidate lets the browser keep a copy but never use it without
	// revalidating (a 304 keeps that cheap). Used for the un-hashed alert client.
	cacheRevalidate = "no-cache"
	// cacheNoStore forbids storing the response at all (secrets / trust anchor).
	cacheNoStore = "no-store"

	// gzipMinBytes skips compression for tiny bodies, where the ~20-byte gzip
	// envelope and the CPU cost buy nothing.
	gzipMinBytes = 1024
)

// compressibleType reports whether a Content-Type is worth gzipping. Fonts and
// images are already compressed; re-compressing them wastes CPU and can grow them.
func compressibleType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	switch ct {
	case "application/javascript", "text/javascript", "application/json",
		"text/html", "text/css", "text/plain", "image/svg+xml",
		"application/manifest+json", "application/samlmetadata+xml", "text/xml", "application/xml":
		return true
	}
	// Prometheus exposition and any other text/* payload.
	return strings.HasPrefix(ct, "text/")
}

var gzipPool = sync.Pool{
	New: func() any { return gzip.NewWriter(nil) },
}

// gzipWriter defers BOTH the compress/passthrough decision and the actual
// WriteHeader until it has seen enough of the body.
//
// Deferring WriteHeader is the crux: http.ServeContent (and most handlers) set
// Content-Type/Content-Length and call WriteHeader *before* the first Write. If we
// forwarded that immediately, the response head would already be on the wire by
// the time we learned the body was compressible, and our Content-Encoding /
// Content-Length edits would be silently dropped. So we hold the status until
// decide() has fixed up the headers.
type gzipWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	decided     bool
	wroteHeader bool // the wrapped handler has chosen a status
	status      int
	buf         []byte // holds the first writes until we reach gzipMinBytes
}

func (w *gzipWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return // mirror net/http: the first WriteHeader wins
	}
	w.wroteHeader = true
	w.status = status
	// Deliberately NOT forwarded here — decide() sends it.
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = http.StatusOK // implicit 200, same as net/http
	}
	if w.decided {
		if w.gz != nil {
			return w.gz.Write(b)
		}
		return w.ResponseWriter.Write(b)
	}
	// Buffer until we can tell whether the body clears the size threshold.
	w.buf = append(w.buf, b...)
	if len(w.buf) < gzipMinBytes {
		return len(b), nil
	}
	w.decide()
	return len(b), nil
}

// decide picks compression or passthrough, sends the (possibly rewritten) header,
// and flushes whatever was buffered.
func (w *gzipWriter) decide() {
	w.decided = true
	body := w.buf
	w.buf = nil
	// Only 2xx bodies are worth compressing; 204/304 have none, and an encoding
	// header on an empty body would be a protocol error.
	compress := len(body) >= gzipMinBytes &&
		w.status >= 200 && w.status < 300 &&
		compressibleType(w.Header().Get("Content-Type"))
	if compress {
		w.Header().Del("Content-Length") // the compressed length differs
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.ResponseWriter.WriteHeader(w.status) // now the headers are final
	if compress {
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w.ResponseWriter)
		w.gz = gz
	}
	if len(body) > 0 {
		if w.gz != nil {
			_, _ = w.gz.Write(body)
		} else {
			_, _ = w.ResponseWriter.Write(body)
		}
	}
}

// close flushes any buffered/compressed bytes and returns the writer to the pool.
func (w *gzipWriter) close() {
	if !w.decided {
		w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Close()
		gzipPool.Put(w.gz)
		w.gz = nil
	}
}

// Flush supports streaming handlers; it settles the decision first so a flush
// never races the buffered start-up bytes.
func (w *gzipWriter) Flush() {
	if !w.decided {
		w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withGzip compresses eligible responses. It bows out for clients that did not
// offer gzip and for Range requests, whose byte offsets refer to the identity
// representation (http.ServeContent advertises Accept-Ranges for static files).
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vary regardless of this request's outcome: the same URL yields different
		// bytes per Accept-Encoding, and shared caches must key on that.
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, status: http.StatusOK}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// withCacheControl stamps a Cache-Control value before delegating, unless the
// handler already set one.
func withCacheControl(value string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", value)
		}
		next.ServeHTTP(w, r)
	})
}
