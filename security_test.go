package main

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct{ remote, xff, want string }{
		{"127.0.0.1:5000", "203.0.113.9", "203.0.113.9"},          // behind the local proxy
		{"127.0.0.1:5000", "6.6.6.6, 203.0.113.9", "203.0.113.9"}, // client-supplied hop ignored
		{"192.168.1.50:5000", "6.6.6.6", "192.168.1.50"},          // direct LAN client can't spoof
		{"[::1]:5000", "", "::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("remote=%s xff=%q -> %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
}

func TestClip(t *testing.T) {
	if got := clip("  àbcdef  ", 3); got != "àbc" {
		t.Errorf("clip = %q", got)
	}
}
