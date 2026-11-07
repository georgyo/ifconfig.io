package main

import (
	"fmt"
	"net/netip"
	"strings"
)

// Published at https://www.cloudflare.com/ips/
var cloudflarePrefixes = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

// Loopback, RFC 1918, CGNAT, link-local and unique local addresses.
var privatePrefixes = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// prefixSet is a list of networks. It is small and scanned linearly, which is
// faster than anything cleverer at this size.
type prefixSet []netip.Prefix

// parsePrefixSet parses a comma separated list of CIDRs, bare IPs, or the
// keywords "cloudflare", "private" and "*" (everything).
func parsePrefixSet(spec string) (prefixSet, error) {
	var s prefixSet
	add := func(cidrs ...string) error {
		for _, c := range cidrs {
			p, err := netip.ParsePrefix(c)
			if err != nil {
				a, aerr := netip.ParseAddr(c)
				if aerr != nil {
					return fmt.Errorf("invalid network %q: %w", c, err)
				}
				p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
			}
			s = append(s, p.Masked())
		}
		return nil
	}
	for item := range strings.SplitSeq(spec, ",") {
		item = strings.TrimSpace(item)
		var err error
		switch strings.ToLower(item) {
		case "":
		case "cloudflare":
			err = add(cloudflarePrefixes...)
		case "private":
			err = add(privatePrefixes...)
		case "*":
			err = add("0.0.0.0/0", "::/0")
		default:
			err = add(item)
		}
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s prefixSet) Contains(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range s {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
