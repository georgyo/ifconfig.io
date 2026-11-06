package main

import (
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

type Configuration struct {
	hostname       string // Displayed Hostname
	cmd_hostname   string // Displayed Hostname for CMD section
	host           string // Listened Host
	port           string // HTTP Port
	proxy_listener string // Proxy Protocol Listener
	ipheader       string // Header to overwrite the remote IP
	countryheader  string // Header to find country code associated to remote IP
	tls            bool   // TLS enabled
	tlscert        string // TLS Cert Path
	tlskey         string // TLS Cert Key Path
	tlsport        string // HTTPS Port

	rdnsTimeout   time.Duration // How long a request waits for reverse DNS
	rdnsCacheSize int           // Number of reverse DNS results to cache

	trustedProxies prefixSet // Peers allowed to set ipheader
	proxyTrusted   prefixSet // Peers allowed to send PROXY headers (nil: library default)
}

var configuration = Configuration{}

func init() {
	hostname := getEnvWithDefault("HOSTNAME", "ifconfig.io")
	protocol := getEnvWithDefault("CMD_PROTOCOL", "")

	cmd_hostname := protocol + hostname

	host := getEnvWithDefault("HOST", "")
	port := getEnvWithDefault("PORT", "8080")
	proxy_listener := getEnvWithDefault("PROXY_PROTOCOL_ADDR", "")

	// Most common alternative would be X-Forwarded-For
	ipheader := getEnvWithDefault("FORWARD_IP_HEADER", "CF-Connecting-IP")

	countryheader := getEnvWithDefault("COUNTRY_CODE_HEADER", "CF-IPCountry")

	tlsenabled := getEnvWithDefault("TLS", "0")
	tlsport := getEnvWithDefault("TLSPORT", "8443")
	tlscert := getEnvWithDefault("TLSCERT", "/opt/ifconfig/.cf/ifconfig.io.crt")
	tlskey := getEnvWithDefault("TLSKEY", "/opt/ifconfig/.cf/ifconfig.io.key")

	rdnsTimeout, err := time.ParseDuration(getEnvWithDefault("REVERSE_DNS_TIMEOUT", "300ms"))
	if err != nil {
		panic(fmt.Sprintf("invalid REVERSE_DNS_TIMEOUT: %v", err))
	}
	rdnsCacheSize, err := strconv.Atoi(getEnvWithDefault("REVERSE_DNS_CACHE_SIZE", "100000"))
	if err != nil {
		panic(fmt.Sprintf("invalid REVERSE_DNS_CACHE_SIZE: %v", err))
	}

	trustedProxies, err := parsePrefixSet(getEnvWithDefault("TRUSTED_PROXIES", "cloudflare,private"))
	if err != nil {
		panic(fmt.Sprintf("invalid TRUSTED_PROXIES: %v", err))
	}
	var proxyTrusted prefixSet
	if spec := os.Getenv("PROXY_PROTOCOL_TRUSTED"); spec != "" {
		if proxyTrusted, err = parsePrefixSet(spec); err != nil {
			panic(fmt.Sprintf("invalid PROXY_PROTOCOL_TRUSTED: %v", err))
		}
	}

	configuration = Configuration{
		hostname:       hostname,
		cmd_hostname:   cmd_hostname,
		host:           host,
		port:           port,
		proxy_listener: proxy_listener,
		ipheader:       textproto.CanonicalMIMEHeaderKey(ipheader),
		countryheader:  textproto.CanonicalMIMEHeaderKey(countryheader),
		tls:            tlsenabled == "1",
		tlscert:        tlscert,
		tlskey:         tlskey,
		tlsport:        tlsport,

		rdnsTimeout:   rdnsTimeout,
		rdnsCacheSize: rdnsCacheSize,

		trustedProxies: trustedProxies,
		proxyTrusted:   proxyTrusted,
	}
}

func getEnvWithDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func main() {
	tuneRuntime()

	r := newRouter()

	errc := make(chan error)

	go func() {
		errc <- newServer(net.JoinHostPort(configuration.host, configuration.port), r).ListenAndServe()
	}()

	if configuration.tls {
		go func() {
			srv := newServer(net.JoinHostPort(configuration.host, configuration.tlsport), r)
			errc <- srv.ListenAndServeTLS(configuration.tlscert, configuration.tlskey)
		}()
	}

	if configuration.proxy_listener != "" {
		go func() {
			list, err := net.Listen("tcp", configuration.proxy_listener)
			if err != nil {
				errc <- err
				return
			}
			proxyListener := &proxyproto.Listener{
				Listener:          list,
				ReadHeaderTimeout: 5 * time.Second,
			}
			if configuration.proxyTrusted != nil {
				proxyListener.ConnPolicy = proxyProtocolPolicy(configuration.proxyTrusted)
			}
			defer proxyListener.Close()
			errc <- newServer(configuration.proxy_listener, r).Serve(proxyListener)
		}()
	}

	panic(<-errc)
}

// newServer returns an http.Server with timeouts, so slow or idle clients
// cannot hold connections (and file descriptors) open indefinitely.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}
