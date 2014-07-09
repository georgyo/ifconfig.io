package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)


// Logger is a simple log handler, out puts in the standard of apache access log common
// http://httpd.apache.org/docs/2.2/logs.html#accesslog
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		ip, err := net.ResolveTCPAddr("tcp", c.Req.RemoteAddr)
		if err != nil {
			c.Abort(500)
		}

		// before request
		c.Next()
		// after request

		user := "-"
		if c.Req.URL.User != nil {
			user = c.Req.URL.User.Username()
		}

		latency := time.Since(t)

		// This is the format of Apache Log Common, with an additional field of latency
		fmt.Printf("%v - %v [%v] \"%v %v %v\" %v %v %v\n",
			ip.IP, user, t.Format(time.RFC3339), c.Req.Method, c.Req.URL.Path,
			c.Req.Proto, c.Writer.Status(), c.Req.ContentLength, latency)
	}
}

func mainHandler(c *gin.Context) {
	fields := strings.Split(c.Params.ByName("field"), ".")
	ip, err := net.ResolveTCPAddr("tcp", c.Req.RemoteAddr)
	if err != nil {
		c.Abort(500)
	}
	c.Set("ip", ip.IP.String())
	c.Set("port", ip.Port)
	c.Set("ua", c.Req.UserAgent())
	c.Set("lang", c.Req.Header.Get("Accept-Language"))
	c.Set("encoding", c.Req.Header.Get("Accept-Encoding"))

	hostnames, err := net.LookupAddr(ip.IP.String())
	if err != nil {
		c.Set("host", "")
	} else {
		c.Set("host", hostnames[0])
	}

	wantsJSON := false
	if len(fields) >= 2 && fields[1] == "json" {
		wantsJSON = true
	}

	ua := strings.Split(c.Req.UserAgent(), "/")
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
		c.JSON(200, c.Req)
		return
	case "all":
		if wantsJSON {
			c.JSON(200, c.Keys)
		} else {
			c.String(200, "%v", c.Keys)
		}
		return
	}

	fieldResult, err := c.Get(fields[0])
	if err != nil {
		c.String(404, "Not Found")
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
		http.ServeFile(c.Writer, c.Req, path.Join(root, path.Clean(file)))
	}
}

func main() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(Logger())
	r.LoadHTMLTemplates("templates/*")

	r.GET("/:field", mainHandler)
	r.GET("/", mainHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}
