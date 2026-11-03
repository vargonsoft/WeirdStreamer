// Package clientip finds the address a request really came from. Behind a reverse proxy (nginx)
// every connection comes from the proxy; the proxy names the client in X-Forwarded-For (or
// X-Real-IP). Anyone can send those headers, so they're only believed from the proxies set in
// config.yml's server.trustedProxies.
package clientip

import (
	"log"
	"net"
	"net/http"
	"strings"
)

var trusted []*net.IPNet

// SetTrusted sets the proxies whose forwarding headers are believed: addresses ("192.168.94.54")
// or networks ("10.0.0.0/8"). Ones that don't parse are logged and left out.
func SetTrusted(proxies []string) {
	trusted = nil
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if !strings.Contains(p, "/") {
			if ip := net.ParseIP(p); ip != nil {
				bits := 128
				if ip.To4() != nil {
					ip, bits = ip.To4(), 32
				}
				trusted = append(trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
				continue
			}
		} else if _, n, err := net.ParseCIDR(p); err == nil {
			trusted = append(trusted, n)
			continue
		}
		log.Printf("Warning: server.trustedProxies: %q isn't an address or network; ignored", p)
	}
}

// Peer is the address of the connection itself (the proxy, behind one).
func Peer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Of is the client's address: from a trusted proxy, the last address in X-Forwarded-For that
// isn't itself a trusted proxy (each proxy adds the one it got the request from, so those before
// it are whatever the client sent), else X-Real-IP; otherwise the connection's.
func Of(r *http.Request) string {
	peer := Peer(r)
	if !isTrusted(peer) {
		return peer
	}
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		for _, a := range strings.Split(h, ",") {
			if a = strings.TrimSpace(a); a != "" {
				hops = append(hops, a)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(hops[i])
		if ip == nil {
			break // not an address: nothing before it can be believed
		}
		if !isTrusted(hops[i]) || i == 0 {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	return peer
}

func isTrusted(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
