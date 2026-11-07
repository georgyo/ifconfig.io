package main

import (
	"net/netip"
	"testing"
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
