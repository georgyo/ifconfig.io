package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/brandfolder/gin-gorelic"
	"github.com/coreos/go-systemd/activation"
	"github.com/gin-gonic/gin"
)

// Logger is a simple log handler, out puts in the standard of apache access log common.
// See http://httpd.apache.org/docs/2.2/logs.html#accesslog
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		ip, err := net.ResolveTCPAddr("tcp", c.Request.RemoteAddr)
		if err != nil {
			c.Abort()
		}

		// before request
		c.Next()
		// after request

		user := "-"
		if c.Request.URL.User != nil {
			user = c.Request.URL.User.Username()
		}

		latency := time.Since(t)

		// This is the format of Apache Log Common, with an additional field of latency
		fmt.Printf("%v - %v [%v] \"%v %v %v\" %v %v %v\n",
			ip.IP, user, t.Format(time.RFC3339), c.Request.Method, c.Request.URL.Path,
			c.Request.Proto, c.Writer.Status(), c.Request.ContentLength, latency)
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
	fields := strings.Split(c.Params.ByName("field"), ".")
	ip, err := net.ResolveTCPAddr("tcp", c.Request.RemoteAddr)
	if err != nil {
		c.Abort()
	}

	cfIP := net.ParseIP(c.Request.Header.Get("CF-Connecting-IP"))
	if cfIP != nil {
		ip.IP = cfIP
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
	if stringInSlice(fields[0], []string{"all", "host"}) || (fields[0] == "" && ua[0] != "curl") {
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
	}

	fieldResult, exists := c.Get(fields[0])
	if !exists {
		c.String(404, "Not Found")
		return
	}
	c.String(200, fmt.Sprintln(fieldResult))

}

// FileServer is a basic file serve handler, this is just here as an example.
// gin.Static() should be used instead
func FileServer(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file := c.Params.ByName("file")
		if !strings.HasPrefix(file, "/") {
			file = "/" + file
		}
		http.ServeFile(c.Writer, c.Request, path.Join(root, path.Clean(file)))
	}
}

func main() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(Logger())
	r.LoadHTMLGlob("templates/*")

	if NEWRELIC_LICENSE_KEY := os.Getenv("NEWRELIC_LICENSE_KEY"); NEWRELIC_LICENSE_KEY != "" {
		var NEWRELIC_APPLICATION_NAME string
		if NEWRELIC_APPLICATION_NAME = os.Getenv("NEWRELIC_APPLICATION_NAME"); NEWRELIC_APPLICATION_NAME == "" {
			NEWRELIC_APPLICATION_NAME = "ifconfig.io"
		}
		gorelic.InitNewrelicAgent(NEWRELIC_LICENSE_KEY, NEWRELIC_APPLICATION_NAME, true)
		r.Use(gorelic.Handler)
	}

	r.GET("/:field", mainHandler)
	r.GET("/", mainHandler)

	// Create a listener for FCGI
	fcgi_listen, err := net.Listen("tcp", "127.0.0.1:4000")
	if err != nil {
		panic(err)
	}
	errc := make(chan error)
	go func(errc chan error) {
		for err := range errc {
			panic(err)
		}
	}(errc)

	go func(errc chan error) {
		errc <- fcgi.Serve(fcgi_listen, r)
	}(errc)

	// Listen on whatever systemd tells us to.
	listeners, err := activation.Listeners(true)
	if err != nil {
		fmt.Printf("Could not get systemd listerns with err %q", err)
	}
	for _, listener := range listeners {
		go func(errc chan error) {
			errc <- http.Serve(listener, r)
		}(errc)
	}

	port := os.Getenv("PORT")
	host := os.Getenv("HOST")
	if port == "" {
		port = "8080"
	}
	errc <- r.Run(host + ":" + port)
}
