package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestIdleConnectionsAreNotClosed checks that a keep-alive connection stays
// usable while idle for longer than the read timeouts.
func TestIdleConnectionsAreNotClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer("", newRouter())
	// Shrink the request timeouts so the test is fast; the idle timeout is
	// left as configured.
	srv.ReadHeaderTimeout = 50 * time.Millisecond
	srv.ReadTimeout = 50 * time.Millisecond
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	br := bufio.NewReader(c)
	get := func() error {
		fmt.Fprint(c, "GET /ip HTTP/1.1\r\nHost: x\r\nUser-Agent: curl/8\r\n\r\n")
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return err
	}
	if err := get(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // well past ReadTimeout
	if err := get(); err != nil {
		t.Fatalf("idle connection was closed: %v", err)
	}
}
