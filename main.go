package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	proxyproto "github.com/pires/go-proxyproto"
)

type Configuration struct {
	hostname       string // Displayed Hostname
	host           string // Listened Host
	port           string // HTTP Port
	proxy_listener string // Proxy Protocol Listener
	ipheader       string // Header to overwrite the remote IP
	tls            bool   // TLS enabled
	tlscert        string // TLS Cert Path
	tlskey         string // TLS Cert Key Path
	tlsport        string // HTTPS Port
}

var configuration = Configuration{}

func init() {
	hostname := getEnvWithDefault("HOSTNAME", "ifconfig.io")

	host := getEnvWithDefault("HOST", "")
	port := getEnvWithDefault("PORT", "8080")
	proxy_listener := getEnvWithDefault("PROXY_PROTOCOL_ADDR", "")

	// Most common alternative would be X-Forwarded-For
	ipheader := getEnvWithDefault("FORWARD_IP_HEADER", "CF-Connecting-IP")

	tlsenabled := getEnvWithDefault("TLS", "0")
	tlsport := getEnvWithDefault("TLSPORT", "8443")
	tlscert := getEnvWithDefault("TLSCERT", "/opt/ifconfig/.cf/ifconfig.io.crt")
	tlskey := getEnvWithDefault("TLSKEY", "/opt/ifconfig/.cf/ifconfig.io.key")

	configuration = Configuration{
		hostname:       hostname,
		host:           host,
		port:           port,
		proxy_listener: proxy_listener,
		ipheader:       ipheader,
		tls:            tlsenabled == "1",
		tlscert:        tlscert,
		tlskey:         tlskey,
		tlsport:        tlsport,
	}
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func testRemoteTCPPort(address string) bool {
	_, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return false
	}
	return true
}

func mainHandler(c *gin.Context) {
	// fields := strings.Split(c.Params.ByName("field"), ".")
	URLFields := strings.Split(strings.Trim(c.Request.URL.EscapedPath(), "/"), "/")
	fields := strings.Split(URLFields[0], ".")
	ip, err := net.ResolveTCPAddr("tcp", c.Request.RemoteAddr)
	if err != nil {
		c.Abort()
	}

	header_ip := net.ParseIP(strings.Split(c.Request.Header.Get(configuration.ipheader), ",")[0])
	if header_ip != nil {
		ip.IP = header_ip
	}

	if fields[0] == "porttest" {
		if len(fields) >= 2 {
			if port, err := strconv.Atoi(fields[1]); err == nil && port > 0 && port <= 65535 {
				c.String(200, fmt.Sprintln(testRemoteTCPPort(ip.IP.String()+":"+fields[1])))
			} else {
				c.String(400, "Invalid Port Number")
			}
		} else {
			c.String(400, "Need Port")
		}
		return
	}

	//if strings.HasPrefix(fields[0], ".well-known/") {
	//	http.ServeFile(c.Writer, c.Request)
	//	return
	//}

	c.Set("ifconfig_hostname", configuration.hostname)

	c.Set("ip", ip.IP.String())
	c.Set("port", ip.Port)
	c.Set("ua", c.Request.UserAgent())
	c.Set("lang", c.Request.Header.Get("Accept-Language"))
	c.Set("encoding", c.Request.Header.Get("Accept-Encoding"))
	c.Set("method", c.Request.Method)
	c.Set("mime", c.Request.Header.Get("Accept"))
	c.Set("referer", c.Request.Header.Get("Referer"))
	c.Set("forwarded", c.Request.Header.Get("X-Forwarded-For"))
	c.Set("country_code", c.Request.Header.Get("CF-IPCountry"))

	ua := strings.Split(c.Request.UserAgent(), "/")

	// Only lookup hostname if the results are going to need it.
	// if stringInSlice(fields[0], []string{"all", "host"}) || (fields[0] == "" && ua[0] != "curl") {
	if stringInSlice(fields[0], []string{"host"}) || (fields[0] == "" && ua[0] != "curl") {
		hostnames, err := net.LookupAddr(ip.IP.String())
		if err != nil {
			c.Set("host", "")
		} else {
			c.Set("host", hostnames[0])
		}
	}

	wantsJSON := false
	if len(fields) >= 2 && fields[1] == "json" {
		wantsJSON = true
	}

	switch fields[0] {
	case "":
		//If the user is using curl, then we should just return the IP, else we show the home page.
		if ua[0] == "curl" {
			c.String(200, fmt.Sprintln(ip.IP))
		} else {
			c.HTML(200, "index.html", c.Keys)
		}
		return
	case "request":
		c.JSON(200, c.Request)
		return
	case "all":
		if wantsJSON {
			c.JSON(200, c.Keys)
		} else {
			c.String(200, "%v", c.Keys)
		}
		return
	case "headers":
		c.JSON(200, c.Request.Header)
		return
	}

	fieldResult, exists := c.Get(fields[0])
	if !exists {
		c.String(404, "Not Found")
		return
	}
	if wantsJSON {
		c.JSON(200, fieldResult)
	} else {
		c.String(200, fmt.Sprintln(fieldResult))
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
	r := gin.New()
	r.Use(gin.Recovery())
	r.LoadHTMLGlob("templates/*")

	for _, route := range []string{
		"ip", "ua", "port", "lang", "encoding", "method",
		"mime", "referer", "forwarded", "country_code",
		"all", "headers", "porttest",
	} {
		r.GET(fmt.Sprintf("/%s", route), mainHandler)
		r.GET(fmt.Sprintf("/%s.json", route), mainHandler)
	}
	r.GET("/", mainHandler)

	errc := make(chan error)
	go func(errc chan error) {
		for err := range errc {
			panic(err)
		}
	}(errc)

	go func(errc chan error) {
		errc <- r.Run(fmt.Sprintf("%s:%s", configuration.host, configuration.port))
	}(errc)

	if configuration.tls {
		go func(errc chan error) {
			errc <- r.RunTLS(
				fmt.Sprintf("%s:%s", configuration.host, configuration.tlsport),
				configuration.tlscert, configuration.tlskey)
		}(errc)
	}

	if configuration.proxy_listener != "" {
		go func(errc chan error) {
			list, err := net.Listen("tcp", configuration.proxy_listener)
			if err != nil {
				errc <- err
				return
			}
			proxyListener := &proxyproto.Listener{Listener: list}
			defer proxyListener.Close()
			errc <- r.RunListener(proxyListener)
		}(errc)
	}

	fmt.Println(<-errc)
}
