package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestOf(t *testing.T) {
	SetTrusted([]string{"192.168.94.54", "10.0.0.0/8", "nonsense"})
	defer SetTrusted(nil)
	cases := []struct {
		name, peer, xff, realIP, want string
	}{
		{"direct", "192.168.94.20:5000", "", "", "192.168.94.20"},
		{"direct client faking the header", "192.168.94.20:5000", "1.2.3.4", "5.6.7.8", "192.168.94.20"},
		{"through nginx", "192.168.94.54:5000", "203.0.113.7", "", "203.0.113.7"},
		{"client's own header before nginx's", "192.168.94.54:5000", "1.2.3.4, 203.0.113.7", "", "203.0.113.7"},
		{"two trusted proxies", "192.168.94.54:5000", "203.0.113.7, 10.1.2.3", "", "203.0.113.7"},
		{"only trusted hops", "192.168.94.54:5000", "10.1.2.3", "", "10.1.2.3"},
		{"X-Real-IP only", "192.168.94.54:5000", "", "203.0.113.9", "203.0.113.9"},
		{"garbage", "192.168.94.54:5000", "not-an-ip", "", "192.168.94.54"},
		{"no headers from nginx", "192.168.94.54:5000", "", "", "192.168.94.54"},
		{"IPv6 client", "192.168.94.54:5000", "2001:db8::1", "", "2001:db8::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if got := Of(r); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
		if got := Peer(r); got != c.peer[:len(c.peer)-5] {
			t.Errorf("%s: peer %s", c.name, got)
		}
	}
}
