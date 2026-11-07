package main

import (
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"time"
)

type Configuration struct {
	hostname      string // Displayed Hostname
	cmd_hostname  string // Displayed Hostname for CMD section
	host          string // Listened Host
	port          string // HTTP Port
	ipheader      string // Header to overwrite the remote IP
	countryheader string // Header to find country code associated to remote IP
	tls           bool   // TLS enabled
	tlscert       string // TLS Cert Path
	tlskey        string // TLS Cert Key Path
	tlsport       string // HTTPS Port

	rdnsTimeout time.Duration // How long a request waits for reverse DNS

	trustedProxies prefixSet // Peers allowed to set ipheader
}

var configuration = Configuration{}

func init() {
	hostname := getEnvWithDefault("HOSTNAME", "ifconfig.io")
	protocol := getEnvWithDefault("CMD_PROTOCOL", "")

	cmd_hostname := protocol + hostname

	host := getEnvWithDefault("HOST", "")
	port := getEnvWithDefault("PORT", "8080")

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

	trustedProxies, err := parsePrefixSet(getEnvWithDefault("TRUSTED_PROXIES", "cloudflare,private"))
	if err != nil {
		panic(fmt.Sprintf("invalid TRUSTED_PROXIES: %v", err))
	}

	configuration = Configuration{
		hostname:      hostname,
		cmd_hostname:  cmd_hostname,
		host:          host,
		port:          port,
		ipheader:      textproto.CanonicalMIMEHeaderKey(ipheader),
		countryheader: textproto.CanonicalMIMEHeaderKey(countryheader),
		tls:           tlsenabled == "1",
		tlscert:       tlscert,
		tlskey:        tlskey,
		tlsport:       tlsport,

		rdnsTimeout: rdnsTimeout,

		trustedProxies: trustedProxies,
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
