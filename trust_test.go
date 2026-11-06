package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

func TestParsePrefixSet(t *testing.T) {
	s, err := parsePrefixSet(" cloudflare , 192.0.2.0/24,2001:db8::/32, 198.51.100.7 ")
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]bool{
		"162.158.1.1":          true,
		"::ffff:162.158.1.1":   true,
		"2606:4700::1":         true,
		"192.0.2.200":          true,
		"2001:db8::5":          true,
		"198.51.100.7":         true,
		"198.51.100.8":         false,
		"10.0.0.1":             false,
		"8.8.8.8":              false,
		"2001:4860:4860::8888": false,
	} {
		if got := s.Contains(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}

	all, err := parsePrefixSet("*")
	if err != nil || !all.Contains(netip.MustParseAddr("8.8.8.8")) || !all.Contains(netip.MustParseAddr("2001:db8::1")) {
		t.Errorf("* should match everything (err=%v)", err)
	}

	empty, err := parsePrefixSet("")
	if err != nil || empty.Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Errorf("empty set should match nothing (err=%v)", err)
	}

	for _, bad := range []string{"nope", "10.0.0.0/33", "cloudflare,1.2.3"} {
		if _, err := parsePrefixSet(bad); err == nil {
			t.Errorf("parsePrefixSet(%q) succeeded", bad)
		}
	}
}

// startProxyServer serves h behind a PROXY protocol listener that only
// trusts the given networks.
func startProxyServer(t *testing.T, trusted string, h http.Handler) string {
	t.Helper()
	set, err := parsePrefixSet(trusted)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pl := &proxyproto.Listener{Listener: ln, ConnPolicy: proxyProtocolPolicy(set), ReadHeaderTimeout: time.Second}
	srv := newServer("", h)
	go srv.Serve(pl)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func proxyGet(t *testing.T, addr, path string) (string, error) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	hdr := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 1111},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 80},
	}
	if _, err := hdr.WriteTo(conn); err != nil {
		return "", err
	}
	io.WriteString(conn, "GET "+path+" HTTP/1.1\r\nHost: x\r\nUser-Agent: curl/8\r\nConnection: close\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func TestProxyProtocolTrusted(t *testing.T) {
	addr := startProxyServer(t, "private", newRouter())
	body, err := proxyGet(t, addr, "/ip")
	if err != nil {
		t.Fatal(err)
	}
	if body != "198.51.100.1\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestProxyProtocolUntrustedRejected(t *testing.T) {
	addr := startProxyServer(t, "192.0.2.0/24", newRouter())
	body, err := proxyGet(t, addr, "/ip")
	if err == nil && strings.Contains(body, "198.51.100.1") {
		t.Fatalf("untrusted peer was able to spoof its address: %q", body)
	}
}
