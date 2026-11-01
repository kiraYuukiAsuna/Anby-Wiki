package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustBoundary(t *testing.T) {
	proxies := TrustedProxySet([]string{" 172.25.0.1 ", "172.25.0.9", "2001:0db8::1", "invalid", "172.25.0.0/16"})
	for _, test := range []struct {
		name      string
		peer      string
		forwarded []string
		want      string
	}{
		{"direct spoof ignored", "198.51.100.5:1234", []string{"203.0.113.7"}, "198.51.100.5"},
		{"host cleaned header", "172.25.0.1:1234", []string{"203.0.113.7"}, "203.0.113.7"},
		{"web chain", "172.25.0.9:1234", []string{"203.0.113.7, 172.25.0.1"}, "203.0.113.7"},
		{"nearest untrusted hop", "172.25.0.9:1234", []string{"192.0.2.99, 203.0.113.7, 172.25.0.1"}, "203.0.113.7"},
		{"multiple header fields", "172.25.0.9:1234", []string{"192.0.2.99, 203.0.113.7", "172.25.0.1"}, "203.0.113.7"},
		{"malformed boundary", "172.25.0.9:1234", []string{"203.0.113.7, unknown, 172.25.0.1"}, "172.25.0.9"},
		{"empty boundary", "172.25.0.9:1234", []string{"203.0.113.7, , 172.25.0.1"}, "172.25.0.9"},
		{"port in header rejected", "172.25.0.9:1234", []string{"203.0.113.7:1234"}, "172.25.0.9"},
		{"no header", "172.25.0.9:1234", nil, "172.25.0.9"},
		{"only proxies", "172.25.0.9:1234", []string{"172.25.0.1"}, "172.25.0.9"},
		{"IPv6 canonical peer", "[2001:db8:0:0::1]:1234", []string{"2001:0db8:0:0::7"}, "2001:db8::7"},
		{"IPv4 mapped peer", "[::ffff:172.25.0.9]:1234", []string{"::ffff:203.0.113.7, ::ffff:172.25.0.1"}, "203.0.113.7"},
		{"subnet not trusted", "172.25.0.10:1234", []string{"203.0.113.7"}, "172.25.0.10"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = test.peer
			for _, value := range test.forwarded {
				r.Header.Add("X-Forwarded-For", value)
			}
			if got := ClientIP(r, proxies); got != test.want {
				t.Fatalf("ClientIP=%q want=%q", got, test.want)
			}
		})
	}
}
