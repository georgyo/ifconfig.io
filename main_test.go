package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testRemoteIP   = "203.0.113.7"
	testRemoteAddr = testRemoteIP + ":4321"
	testUA         = `curl/8.0 "<x>" & ü`
)

func stubLookup(t testing.TB, names []string, err error) *int {
	t.Helper()
	calls := 0
	orig := lookupAddr
	lookupAddr = func(string) ([]string, error) {
		calls++
		return names, err
	}
	t.Cleanup(func() { lookupAddr = orig })
	return &calls
}

func newTestRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = testRemoteAddr
	req.Header.Set("User-Agent", testUA)
	req.Header.Set("Accept-Language", "")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("CF-IPCountry", "US")
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	req.Header.Set("Referer", "http://a: b#c")
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

const (
	ctText = "text/plain; charset=utf-8"
	ctJSON = "application/json; charset=utf-8"
	ctJS   = "application/javascript"
)

func TestEndpoints(t *testing.T) {
	stubLookup(t, []string{"host.example."}, nil)
	h := newRouter()

	allJSON := `{"country_code":"US","encoding":"","forwarded":"1.1.1.1, 2.2.2.2","host":"203.0.113.7","ifconfig_cmd_hostname":"ifconfig.io","ifconfig_hostname":"ifconfig.io","ip":"203.0.113.7","lang":"","method":"GET","mime":"*/*","port":4321,"referer":"http://a: b#c","ua":"curl/8.0 \"\u003cx\u003e\" \u0026 ü"}`
	allYAML := `country_code: US
encoding: ""
forwarded: 1.1.1.1, 2.2.2.2
host: 203.0.113.7
ifconfig_cmd_hostname: ifconfig.io
ifconfig_hostname: ifconfig.io
ip: 203.0.113.7
lang: ""
method: GET
mime: "*/*"
port: 4321
referer: "http://a: b#c"
ua: curl/8.0 "<x>" & ü
`
	headersJSON := `{"Accept":["*/*"],"Accept-Language":[""],"Cf-Ipcountry":["US"],"Referer":["http://a: b#c"],"User-Agent":["curl/8.0 \"\u003cx\u003e\" \u0026 ü"],"X-Forwarded-For":["1.1.1.1, 2.2.2.2"]}`

	tests := []struct {
		path string
		code int
		ct   string
		body string
	}{
		{"/ip", 200, ctText, "203.0.113.7\n"},
		{"/ip.json", 200, ctJSON, `"203.0.113.7"`},
		{"/ip.js", 200, ctJS, "ip = \"203.0.113.7\"\n"},
		{"/port", 200, ctText, "4321\n"},
		{"/port.json", 200, ctJSON, "4321"},
		{"/port.js", 200, ctJS, "port = 4321\n"},
		{"/host", 200, ctText, "host.example.\n"},
		{"/host.json", 200, ctJSON, `"host.example."`},
		{"/host.js", 200, ctJS, "host = \"host.example.\"\n"},
		{"/ua", 200, ctText, testUA + "\n"},
		{"/ua.json", 200, ctJSON, `"curl/8.0 \"\u003cx\u003e\" \u0026 ü"`},
		{"/ua.js", 200, ctJS, "ua = \"curl/8.0 \\\"\\u003cx\\u003e\\\" \\u0026 ü\"\n"},
		{"/lang", 200, ctText, "\n"},
		{"/encoding", 200, ctText, "\n"},
		{"/method", 200, ctText, "GET\n"},
		{"/mime", 200, ctText, "*/*\n"},
		{"/referer", 200, ctText, "http://a: b#c\n"},
		{"/forwarded", 200, ctText, "1.1.1.1, 2.2.2.2\n"},
		{"/country_code", 200, ctText, "US\n"},
		{"/country_code.json", 200, ctJSON, `"US"`},
		{"/all", 200, ctText, allYAML},
		{"/all.json", 200, ctJSON, allJSON},
		{"/headers", 200, ctJSON, headersJSON},
		{"/headers.json", 200, ctJSON, headersJSON},
		{"/nope", 404, "text/plain", "404 page not found"},
		{"/all.yaml", 404, "text/plain", "404 page not found"},
		{"/all.xml", 404, "text/plain", "404 page not found"},
		{"/request", 404, "text/plain", "404 page not found"},
		{"/porttest", 404, "text/plain", "404 page not found"},
		{"/porttest.80", 404, "text/plain", "404 page not found"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := serve(h, newTestRequest("GET", tt.path))
			if w.Code != tt.code {
				t.Errorf("code = %d, want %d", w.Code, tt.code)
			}
			if got := w.Header().Get("Content-Type"); got != tt.ct {
				t.Errorf("content-type = %q, want %q", got, tt.ct)
			}
			if got := w.Body.String(); got != tt.body {
				t.Errorf("body = %q, want %q", got, tt.body)
			}
		})
	}

	// The .js variants of all/headers wrap the JSON document. Key order is
	// not guaranteed, so compare the decoded values.
	for path, want := range map[string]string{"/all.js": allJSON, "/headers.js": headersJSON} {
		t.Run(path, func(t *testing.T) {
			w := serve(h, newTestRequest("GET", path))
			if w.Code != 200 || w.Header().Get("Content-Type") != ctJS {
				t.Fatalf("code=%d ct=%q", w.Code, w.Header().Get("Content-Type"))
			}
			body, ok := strings.CutPrefix(w.Body.String(), "ifconfig_io = ")
			if !ok || !strings.HasSuffix(body, "\n") {
				t.Fatalf("unexpected body %q", w.Body.String())
			}
			var got, exp any
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(want), &exp); err != nil {
				t.Fatal(err)
			}
			gb, _ := json.Marshal(got)
			eb, _ := json.Marshal(exp)
			if string(gb) != string(eb) {
				t.Errorf("got %s, want %s", gb, eb)
			}
		})
	}
}

func TestTrailingSlashRedirect(t *testing.T) {
	w := serve(newRouter(), newTestRequest("GET", "/ip/"))
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/ip" {
		t.Fatalf("code=%d location=%q", w.Code, w.Header().Get("Location"))
	}
}

func TestRootCommandLine(t *testing.T) {
	calls := stubLookup(t, []string{"host.example."}, nil)
	h := newRouter()
	for _, ua := range []string{
		"curl/8.4.0",
		"Wget/1.21",
		"HTTPie/3.2.1",
		"Mozilla/5.0 (Windows NT 10.0; Microsoft Windows 10.0.19044; en-US) PowerShell/7.2.4",
	} {
		req := newTestRequest("GET", "/")
		req.Header.Set("User-Agent", ua)
		w := serve(h, req)
		if w.Code != 200 || w.Body.String() != testRemoteIP+"\n" {
			t.Errorf("ua %q: code=%d body=%q", ua, w.Code, w.Body.String())
		}
	}
	if *calls != 0 {
		t.Errorf("reverse DNS performed %d times for command line clients", *calls)
	}
}

func TestBadRemoteAddr(t *testing.T) {
	req := newTestRequest("GET", "/ip")
	req.RemoteAddr = "garbage"
	w := serve(newRouter(), req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestRootBrowser(t *testing.T) {
	calls := stubLookup(t, []string{"host.example."}, nil)
	req := newTestRequest("GET", "/")
	req.Header.Set("User-Agent", "Mozilla/5.0 <script>")
	w := serve(newRouter(), req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("code=%d ct=%q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	for _, want := range []string{
		"<title>ifconfig.io</title>",
		`<div class="col-sm-9">203.0.113.7</div>`,
		`<div class="col-sm-9">host.example.</div>`,
		`<div class="col-sm-9">Mozilla/5.0 &lt;script&gt;</div>`,
		`<div class="col-sm-9">4321</div>`,
		`<div class="col-sm-9">US</div>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	for _, unwanted := range []string{"all.xml", "no value"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body contains %q", unwanted)
		}
	}
	if *calls != 1 {
		t.Errorf("reverse DNS performed %d times, want 1", *calls)
	}
}

func TestReverseDNSOnlyWhenNeeded(t *testing.T) {
	calls := stubLookup(t, []string{"host.example."}, nil)
	h := newRouter()
	for _, p := range []string{"/ip", "/all", "/all.json", "/ua", "/headers"} {
		serve(h, newTestRequest("GET", p))
	}
	if *calls != 0 {
		t.Errorf("reverse DNS performed %d times, want 0", *calls)
	}
}

func TestHostFallsBackToIP(t *testing.T) {
	stubLookup(t, nil, errors.New("lookup failed"))
	w := serve(newRouter(), newTestRequest("GET", "/host"))
	if w.Body.String() != testRemoteIP+"\n" {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHostEmptyLookupResult(t *testing.T) {
	stubLookup(t, []string{}, nil)
	w := serve(newRouter(), newTestRequest("GET", "/host"))
	if w.Code != 200 || w.Body.String() != testRemoteIP+"\n" {
		t.Fatalf("code = %d body = %q", w.Code, w.Body.String())
	}
}

func TestIPHeader(t *testing.T) {
	h := newRouter()
	for _, tt := range []struct{ header, want string }{
		{"198.51.100.9", "198.51.100.9"},
		{"198.51.100.9, 10.0.0.1", "198.51.100.9"},
		{"2001:db8::1", "2001:db8::1"},
		{"not-an-ip", testRemoteIP},
		{"", testRemoteIP},
	} {
		req := newTestRequest("GET", "/ip")
		if tt.header != "" {
			req.Header.Set("CF-Connecting-IP", tt.header)
		}
		w := serve(h, req)
		if w.Body.String() != tt.want+"\n" {
			t.Errorf("header %q: body = %q, want %q", tt.header, w.Body.String(), tt.want)
		}
	}
}

func TestIsReqFromCmdLine(t *testing.T) {
	for ua, want := range map[string]bool{
		"curl/7.83.1":        true,
		"Wget/1.21.4":        true,
		"xh/0.20.0":          true,
		"Go-http-client/1.1": true,
		"Mozilla/5.0 (Windows NT 10.0; Microsoft Windows 10.0.19044; en-US) PowerShell/7.2.4": true,
		"Mozilla/5.0 (X11; Linux x86_64) Firefox/120.0":                                       false,
		"":       false,
		"curl ":  false,
		" ":      false,
		"curl":   true,
		"Safari": false,
	} {
		if got := isReqFromCmdLine(ua); got != want {
			t.Errorf("isReqFromCmdLine(%q) = %v, want %v", ua, got, want)
		}
	}
}

func benchmarkPath(b *testing.B, path, ua string) {
	stubLookup(b, []string{"host.example."}, nil)
	h := newRouter()
	req := newTestRequest("GET", path)
	req.Header.Set("User-Agent", ua)
	w := &discardWriter{h: http.Header{}}
	b.ReportAllocs()
	for b.Loop() {
		clear(w.h)
		h.ServeHTTP(w, req)
	}
}

// discardWriter is a minimal ResponseWriter so benchmarks measure the handler
// rather than httptest.ResponseRecorder.
type discardWriter struct{ h http.Header }

func (w *discardWriter) Header() http.Header         { return w.h }
func (w *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardWriter) WriteHeader(int)             {}

func BenchmarkRootCurl(b *testing.B) { benchmarkPath(b, "/", "curl/8.4.0") }
func BenchmarkIP(b *testing.B)       { benchmarkPath(b, "/ip", "curl/8.4.0") }
func BenchmarkIPJSON(b *testing.B)   { benchmarkPath(b, "/ip.json", "curl/8.4.0") }
func BenchmarkAll(b *testing.B)      { benchmarkPath(b, "/all", "curl/8.4.0") }
func BenchmarkAllJSON(b *testing.B)  { benchmarkPath(b, "/all.json", "curl/8.4.0") }
func BenchmarkAllJS(b *testing.B)    { benchmarkPath(b, "/all.js", "curl/8.4.0") }
func BenchmarkHeaders(b *testing.B)  { benchmarkPath(b, "/headers", "curl/8.4.0") }
func BenchmarkRootBrowser(b *testing.B) {
	benchmarkPath(b, "/", "Mozilla/5.0 (X11; Linux x86_64) Firefox/120.0")
}
