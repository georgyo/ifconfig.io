package main

import (
	"html/template"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// field identifies what a request asked for.
type field uint8

const (
	fieldRoot field = iota
	fieldIP
	fieldUA
	fieldPort
	fieldLang
	fieldEncoding
	fieldMethod
	fieldMime
	fieldReferer
	fieldForwarded
	fieldCountryCode
	fieldHost
	fieldAll
	fieldHeaders
)

func parseField(name string) (field, bool) {
	switch name {
	case "ip":
		return fieldIP, true
	case "ua":
		return fieldUA, true
	case "port":
		return fieldPort, true
	case "lang":
		return fieldLang, true
	case "encoding":
		return fieldEncoding, true
	case "method":
		return fieldMethod, true
	case "mime":
		return fieldMime, true
	case "referer":
		return fieldReferer, true
	case "forwarded":
		return fieldForwarded, true
	case "country_code":
		return fieldCountryCode, true
	case "host":
		return fieldHost, true
	case "all":
		return fieldAll, true
	case "headers":
		return fieldHeaders, true
	}
	return 0, false
}

type format uint8

const (
	formatText format = iota
	formatJSON
	formatJS
)

// parseRoute splits "/name[.json|.js]" into its parts.
func parseRoute(path string) (f field, name string, fm format, ok bool) {
	if path == "/" {
		return fieldRoot, "", formatText, true
	}
	if len(path) < 2 || path[0] != '/' {
		return 0, "", 0, false
	}
	name = path[1:]
	if base, ext, found := strings.Cut(name, "."); found {
		switch ext {
		case "json":
			fm = formatJSON
		case "js":
			fm = formatJS
		default:
			return 0, "", 0, false
		}
		name = base
	}
	f, ok = parseField(name)
	return f, name, fm, ok
}

// Shared header values. Assigning these directly into the response header
// map avoids allocating a new slice per response; net/http never mutates them.
var (
	ctText      = []string{"text/plain; charset=utf-8"}
	ctPlain     = []string{"text/plain"}
	ctJSON      = []string{"application/json; charset=utf-8"}
	ctJS        = []string{"application/javascript"}
	ctHTML      = []string{"text/html; charset=utf-8"}
	notFoundMsg = []byte("404 page not found")
)

type buffer struct{ b []byte }

func (b *buffer) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}

var bufPool = sync.Pool{New: func() any { return &buffer{b: make([]byte, 0, 1024)} }}

func getBuffer() *buffer { return bufPool.Get().(*buffer) }

func putBuffer(b *buffer) {
	if cap(b.b) > 64<<10 {
		return
	}
	b.b = b.b[:0]
	bufPool.Put(b)
}

// server is the HTTP handler for every endpoint.
type server struct {
	tmpl  *template.Template
	hosts *rdnsCache
}

// pageData is what the HTML template renders.
type pageData struct {
	Hostname    string
	CmdHostname string
	IP          string
	Host        string
	CountryCode string
	UA          string
	Port        uint16
	Lang        string
	Referer     string
	Method      string
	Encoding    string
	Mime        string
	Forwarded   string
}

func newRouter() http.Handler {
	return &server{
		tmpl:  template.Must(template.ParseGlob("templates/*")),
		hosts: newRDNSCache(configuration.rdnsCacheSize),
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		respond(w, http.StatusNotFound, ctPlain, notFoundMsg)
		return
	}

	f, name, fm, ok := parseRoute(r.URL.Path)
	if !ok {
		s.notFound(w, r)
		return
	}

	ip, port, ok := clientAddr(r)
	if !ok {
		respond(w, http.StatusInternalServerError, ctText, []byte("Unable to determine remote address"))
		return
	}

	buf := getBuffer()
	defer putBuffer(buf)
	b := buf.b

	switch f {
	case fieldRoot:
		// Command line clients like curl/HTTPie just get the IP; browsers
		// get the home page.
		if isReqFromCmdLine(header(r, "User-Agent")) {
			b = ip.AppendTo(b)
			b = append(b, '\n')
			respond(w, http.StatusOK, ctText, b)
			return
		}
		buf.b = b
		s.renderPage(w, r, ip, port, buf)
		return

	case fieldAll:
		switch fm {
		case formatJSON:
			respond(w, http.StatusOK, ctJSON, appendAllJSON(b, r, ip, port))
		case formatJS:
			b = append(b, "ifconfig_io = "...)
			b = appendAllJSON(b, r, ip, port)
			respond(w, http.StatusOK, ctJS, append(b, '\n'))
		default:
			respond(w, http.StatusOK, ctText, appendAllYAML(b, r, ip, port))
		}
		return

	case fieldHeaders:
		keys := make([]string, 0, len(r.Header))
		for k := range r.Header {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		if fm == formatJS {
			b = append(b, "ifconfig_io = "...)
			b = appendJSONHeaders(b, keys, r.Header)
			respond(w, http.StatusOK, ctJS, append(b, '\n'))
		} else {
			respond(w, http.StatusOK, ctJSON, appendJSONHeaders(b, keys, r.Header))
		}
		return

	case fieldPort:
		switch fm {
		case formatJSON:
			respond(w, http.StatusOK, ctJSON, strconv.AppendUint(b, uint64(port), 10))
		case formatJS:
			b = append(b, "port = "...)
			b = strconv.AppendUint(b, uint64(port), 10)
			respond(w, http.StatusOK, ctJS, append(b, '\n'))
		default:
			b = strconv.AppendUint(b, uint64(port), 10)
			respond(w, http.StatusOK, ctText, append(b, '\n'))
		}
		return

	case fieldIP:
		// IP addresses never need escaping, so skip the string conversion.
		switch fm {
		case formatJSON:
			b = append(b, '"')
			b = ip.AppendTo(b)
			respond(w, http.StatusOK, ctJSON, append(b, '"'))
		case formatJS:
			b = append(b, `ip = "`...)
			b = ip.AppendTo(b)
			respond(w, http.StatusOK, ctJS, append(b, '"', '\n'))
		default:
			b = ip.AppendTo(b)
			respond(w, http.StatusOK, ctText, append(b, '\n'))
		}
		return
	}

	var v string
	switch f {
	case fieldHost:
		v = s.hostname(ip)
	case fieldUA:
		v = header(r, "User-Agent")
	case fieldLang:
		v = header(r, "Accept-Language")
	case fieldEncoding:
		v = header(r, "Accept-Encoding")
	case fieldMethod:
		v = r.Method
	case fieldMime:
		v = header(r, "Accept")
	case fieldReferer:
		v = header(r, "Referer")
	case fieldForwarded:
		v = header(r, "X-Forwarded-For")
	case fieldCountryCode:
		v = header(r, configuration.countryheader)
	}

	switch fm {
	case formatJSON:
		respond(w, http.StatusOK, ctJSON, appendJSONString(b, v))
	case formatJS:
		b = append(b, name...)
		b = append(b, " = "...)
		b = appendJSONString(b, v)
		respond(w, http.StatusOK, ctJS, append(b, '\n'))
	default:
		b = append(b, v...)
		respond(w, http.StatusOK, ctText, append(b, '\n'))
	}
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	// Redirect "/ip/" to "/ip", as gin used to.
	p := r.URL.Path
	if len(p) > 1 && p[len(p)-1] == '/' {
		if _, _, _, ok := parseRoute(p[:len(p)-1]); ok {
			u := *r.URL
			u.Path = p[:len(p)-1]
			u.RawPath = ""
			code := http.StatusMovedPermanently
			if r.Method != http.MethodGet {
				code = http.StatusTemporaryRedirect
			}
			http.Redirect(w, r, u.String(), code)
			return
		}
	}
	respond(w, http.StatusNotFound, ctPlain, notFoundMsg)
}

func (s *server) renderPage(w http.ResponseWriter, r *http.Request, ip netip.Addr, port uint16, buf *buffer) {
	ipStr := ip.String()
	data := pageData{
		Hostname:    configuration.hostname,
		CmdHostname: configuration.cmd_hostname,
		IP:          ipStr,
		Host:        s.hostname(ip),
		CountryCode: header(r, configuration.countryheader),
		UA:          header(r, "User-Agent"),
		Port:        port,
		Lang:        header(r, "Accept-Language"),
		Referer:     header(r, "Referer"),
		Method:      r.Method,
		Encoding:    header(r, "Accept-Encoding"),
		Mime:        header(r, "Accept"),
		Forwarded:   header(r, "X-Forwarded-For"),
	}
	if err := s.tmpl.ExecuteTemplate(buf, "index.html", &data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header()["Content-Length"] = []string{strconv.Itoa(len(buf.b))}
	respond(w, http.StatusOK, ctHTML, buf.b)
}

// hostname returns the reverse DNS name for ip, or the IP itself if there is
// none (or it is too slow to find).
func (s *server) hostname(ip netip.Addr) string {
	ipStr := ip.String()
	if name, ok := s.hosts.Lookup(ipStr, configuration.rdnsTimeout); ok {
		return name
	}
	return ipStr
}

func respond(w http.ResponseWriter, code int, contentType []string, body []byte) {
	w.Header()["Content-Type"] = contentType
	w.WriteHeader(code)
	w.Write(body)
}

// header returns the first value of a header. key must already be in
// canonical form, which lets us skip the canonicalisation Header.Get does.
func header(r *http.Request, key string) string {
	if v := r.Header[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// clientAddr returns the client's address and port. If the connection came
// from a trusted proxy, the address is taken from the configured header.
func clientAddr(r *http.Request) (netip.Addr, uint16, bool) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, 0, false
	}
	ip := ap.Addr().Unmap().WithZone("")
	if configuration.trustedProxies.Contains(ip) {
		if v := header(r, configuration.ipheader); v != "" {
			first, _, _ := strings.Cut(v, ",")
			if h, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil && h.Zone() == "" {
				ip = h.Unmap()
			}
		}
	}
	return ip, ap.Port(), true
}

// appendAllJSON appends every field as a JSON object, keys sorted.
func appendAllJSON(b []byte, r *http.Request, ip netip.Addr, port uint16) []byte {
	b = append(b, `{"country_code":`...)
	b = appendJSONString(b, header(r, configuration.countryheader))
	b = append(b, `,"encoding":`...)
	b = appendJSONString(b, header(r, "Accept-Encoding"))
	b = append(b, `,"forwarded":`...)
	b = appendJSONString(b, header(r, "X-Forwarded-For"))
	b = append(b, `,"host":"`...)
	b = ip.AppendTo(b)
	b = append(b, `","ifconfig_cmd_hostname":`...)
	b = appendJSONString(b, configuration.cmd_hostname)
	b = append(b, `,"ifconfig_hostname":`...)
	b = appendJSONString(b, configuration.hostname)
	b = append(b, `,"ip":"`...)
	b = ip.AppendTo(b)
	b = append(b, `","lang":`...)
	b = appendJSONString(b, header(r, "Accept-Language"))
	b = append(b, `,"method":`...)
	b = appendJSONString(b, r.Method)
	b = append(b, `,"mime":`...)
	b = appendJSONString(b, header(r, "Accept"))
	b = append(b, `,"port":`...)
	b = strconv.AppendUint(b, uint64(port), 10)
	b = append(b, `,"referer":`...)
	b = appendJSONString(b, header(r, "Referer"))
	b = append(b, `,"ua":`...)
	b = appendJSONString(b, header(r, "User-Agent"))
	return append(b, '}')
}

// appendAllYAML appends every field as a YAML mapping, keys sorted.
func appendAllYAML(b []byte, r *http.Request, ip netip.Addr, port uint16) []byte {
	b = append(b, "country_code: "...)
	b = appendYAMLString(b, header(r, configuration.countryheader))
	b = append(b, "\nencoding: "...)
	b = appendYAMLString(b, header(r, "Accept-Encoding"))
	b = append(b, "\nforwarded: "...)
	b = appendYAMLString(b, header(r, "X-Forwarded-For"))
	b = append(b, "\nhost: "...)
	b = appendYAMLIP(b, ip)
	b = append(b, "\nifconfig_cmd_hostname: "...)
	b = appendYAMLString(b, configuration.cmd_hostname)
	b = append(b, "\nifconfig_hostname: "...)
	b = appendYAMLString(b, configuration.hostname)
	b = append(b, "\nip: "...)
	b = appendYAMLIP(b, ip)
	b = append(b, "\nlang: "...)
	b = appendYAMLString(b, header(r, "Accept-Language"))
	b = append(b, "\nmethod: "...)
	b = appendYAMLString(b, r.Method)
	b = append(b, "\nmime: "...)
	b = appendYAMLString(b, header(r, "Accept"))
	b = append(b, "\nport: "...)
	b = strconv.AppendUint(b, uint64(port), 10)
	b = append(b, "\nreferer: "...)
	b = appendYAMLString(b, header(r, "Referer"))
	b = append(b, "\nua: "...)
	b = appendYAMLString(b, header(r, "User-Agent"))
	return append(b, '\n')
}

// appendYAMLIP writes an IP address as a YAML scalar. IPv4 addresses and most
// IPv6 addresses are safe unquoted; the rare IPv6 address that ends in ':'
// ("2001:db8::") must be quoted.
func appendYAMLIP(b []byte, ip netip.Addr) []byte {
	start := len(b)
	b = ip.AppendTo(b)
	if s := b[start:]; s[len(s)-1] == ':' {
		q := strconv.AppendQuote(nil, string(s))
		b = append(b[:start], q...)
	}
	return b
}

func isReqFromCmdLine(ua string) bool {
	// Example User Agents
	// curl/7.83.1
	// Mozilla/5.0 (Windows NT 10.0; Microsoft Windows 10.0.19044; en-US) PowerShell/7.2.4
	//
	// In the case of PowerShell we have to look at only the last word. A
	// trailing space is not treated as a word separator.
	if len(ua) > 1 {
		if i := strings.LastIndexByte(ua[:len(ua)-1], ' '); i >= 0 {
			ua = ua[i+1:]
		}
	}
	name, _, _ := strings.Cut(ua, "/")
	switch name {
	case "curl", "HTTPie", "httpie-go", "Wget", "fetch libfetch", "Go", "Go-http-client", "ddclient", "Mikrotik", "xh", "WindowsPowerShell", "PowerShell":
		return true
	}
	return false
}
