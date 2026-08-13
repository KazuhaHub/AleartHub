package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bigBody is comfortably over gzipMinBytes and compresses well.
var bigBody = strings.Repeat("alerthub compressible payload ", 200)

func gzipTestHandler(contentType, body string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		// Mimic http.ServeContent: set Content-Length and call WriteHeader BEFORE
		// writing. This is exactly the ordering that breaks a naive gzip wrapper.
		w.Header().Set("Content-Length", itoaLen(len(body)))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func itoaLen(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func doGzipReq(h http.Handler, acceptEncoding string, hdrs map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	for k, v := range hdrs {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	withGzip(h).ServeHTTP(w, r)
	return w
}

// TestGzip_CompressesLargeCompressibleBody is the core case, and specifically
// guards the deferred-WriteHeader behaviour: the handler sets Content-Length and
// calls WriteHeader before writing, so a wrapper that forwards WriteHeader eagerly
// would never manage to set Content-Encoding.
func TestGzip_CompressesLargeCompressibleBody(t *testing.T) {
	w := doGzipReq(gzipTestHandler("application/javascript", bigBody, http.StatusOK), "gzip", nil)

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length must be dropped when compressing, got %q", got)
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary must include Accept-Encoding, got %q", w.Header().Get("Vary"))
	}
	if w.Body.Len() >= len(bigBody) {
		t.Errorf("compressed body (%d) should be smaller than the original (%d)", w.Body.Len(), len(bigBody))
	}
	// It must actually be valid gzip that round-trips to the original bytes.
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	if string(got) != bigBody {
		t.Fatal("decompressed body does not match the original")
	}
}

func TestGzip_SkipsWhenClientDoesNotAccept(t *testing.T) {
	w := doGzipReq(gzipTestHandler("application/javascript", bigBody, http.StatusOK), "identity", nil)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("must not compress for a client that did not offer gzip")
	}
	if w.Body.String() != bigBody {
		t.Fatal("identity response must be the original bytes")
	}
}

func TestGzip_SkipsRangeRequests(t *testing.T) {
	w := doGzipReq(gzipTestHandler("application/javascript", bigBody, http.StatusOK),
		"gzip", map[string]string{"Range": "bytes=0-99"})
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("must not compress a Range request — byte offsets refer to the identity representation")
	}
}

func TestGzip_SkipsSmallBodies(t *testing.T) {
	w := doGzipReq(gzipTestHandler("application/json", `{"ok":true}`, http.StatusOK), "gzip", nil)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("must not compress a body below the size threshold")
	}
	if w.Body.String() != `{"ok":true}` {
		t.Fatalf("small body must pass through unchanged, got %q", w.Body.String())
	}
}

func TestGzip_SkipsIncompressibleTypes(t *testing.T) {
	for _, ct := range []string{"font/woff2", "image/png", "application/octet-stream"} {
		w := doGzipReq(gzipTestHandler(ct, bigBody, http.StatusOK), "gzip", nil)
		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: already-compressed type must not be gzipped", ct)
		}
	}
}

// TestGzip_NoEncodingOnBodylessStatuses guards a protocol error: a 304/204 has no
// body, so it must never advertise Content-Encoding.
func TestGzip_NoEncodingOnBodylessStatuses(t *testing.T) {
	for _, status := range []int{http.StatusNotModified, http.StatusNoContent} {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript")
			w.WriteHeader(status)
		})
		w := doGzipReq(h, "gzip", nil)
		if w.Code != status {
			t.Errorf("status = %d, want %d", w.Code, status)
		}
		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("status %d must not carry Content-Encoding", status)
		}
		if w.Body.Len() != 0 {
			t.Errorf("status %d must have an empty body, got %d bytes", status, w.Body.Len())
		}
	}
}

func TestGzip_PreservesStatusCode(t *testing.T) {
	w := doGzipReq(gzipTestHandler("text/html", bigBody, http.StatusNotFound), "gzip", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 preserved through the wrapper", w.Code)
	}
}

func TestCompressibleType(t *testing.T) {
	yes := []string{"application/javascript", "text/css", "application/json; charset=utf-8",
		"text/html; charset=utf-8", "image/svg+xml", "text/plain"}
	no := []string{"font/woff2", "image/png", "application/octet-stream", "video/mp4", ""}
	for _, ct := range yes {
		if !compressibleType(ct) {
			t.Errorf("%q should be compressible", ct)
		}
	}
	for _, ct := range no {
		if compressibleType(ct) {
			t.Errorf("%q should NOT be compressible", ct)
		}
	}
}

func TestWithCacheControl_DoesNotOverrideHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	})
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	withCacheControl(cacheImmutable, inner).ServeHTTP(w, r)
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want the handler's own no-store to win", got)
	}
}

// --- end-to-end policy over the real mux -----------------------------------

// TestCachePolicy_EndToEnd pins the per-surface Cache-Control contract described
// in httpcache.go, through the actual routing.
func TestCachePolicy_EndToEnd(t *testing.T) {
	ts := newTestServer(t)

	cases := []struct {
		name, path, wantCacheControl string
	}{
		{"admin index must revalidate", "/admin/", cacheRevalidate},
		{"pubkey is a secret bootstrap", "/pubkey", cacheNoStore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			ts.handler.ServeHTTP(w, r)
			if got := w.Header().Get("Cache-Control"); got != tc.wantCacheControl {
				t.Fatalf("%s: Cache-Control = %q, want %q", tc.path, got, tc.wantCacheControl)
			}
		})
	}
}

// TestCachePolicy_APIResponsesAreNoStore ensures authenticated JSON never lands in
// a cache — writeJSON is the single choke point for every API response.
func TestCachePolicy_APIResponsesAreNoStore(t *testing.T) {
	ts := newTestServer(t)
	w := ts.req(t, http.MethodGet, "/api/history", nil, adminHdr())
	if w.Code != http.StatusOK {
		t.Fatalf("history = %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Fatalf("API Cache-Control = %q, want %q", got, cacheNoStore)
	}
}

// TestCachePolicy_HashedAssetsAreImmutable proves the big win: Vite's
// content-hashed chunks are cacheable for a year, so repeat visits cost nothing.
func TestCachePolicy_HashedAssetsAreImmutable(t *testing.T) {
	ts := newTestServer(t)
	// Discover a real hashed asset from the embedded index.html.
	idx := httptest.NewRecorder()
	ts.handler.ServeHTTP(idx, httptest.NewRequest(http.MethodGet, "/admin/", nil))
	asset := findAssetPath(idx.Body.String())
	if asset == "" {
		t.Skip("no hashed asset in the embedded index.html (dist not built)")
	}

	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, asset, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", asset, w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Fatalf("%s: Cache-Control = %q, want %q", asset, got, cacheImmutable)
	}
}

// findAssetPath pulls the first /admin/assets/... URL out of the index HTML.
func findAssetPath(html string) string {
	const marker = "/admin/assets/"
	i := strings.Index(html, marker)
	if i < 0 {
		return ""
	}
	rest := html[i:]
	j := strings.IndexAny(rest, `"'`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
