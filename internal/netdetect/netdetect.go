// Package netdetect determines the real client IP and whether it is on the LAN.
package netdetect

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type Detector struct {
	Trusted []netip.Prefix
	LAN     []netip.Prefix
}

func contains(ps []netip.Prefix, a netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	a, err := netip.ParseAddr(strings.Trim(s, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}

// ClientIP returns the client's address. Forwarded headers are honoured only
// when the direct peer is a trusted proxy; the XFF chain is walked right to
// left, skipping trusted hops, so a client cannot spoof its own address.
func (d *Detector) ClientIP(r *http.Request) netip.Addr {
	peer, ok := parseAddr(r.RemoteAddr)
	if !ok {
		return netip.Addr{}
	}
	if !contains(d.Trusted, peer) {
		return peer
	}
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		parts := strings.Split(strings.Join(xff, ","), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			a, ok := parseAddr(parts[i])
			if !ok {
				return peer // malformed chain: do not trust it
			}
			if !contains(d.Trusted, a) {
				return a
			}
		}
	}
	if a, ok := parseAddr(r.Header.Get("X-Real-IP")); ok {
		return a
	}
	return peer
}

func (d *Detector) IsLAN(a netip.Addr) bool {
	return a.IsValid() && contains(d.LAN, a)
}

// Zone returns "lan" or "wan" for the request.
func (d *Detector) Zone(r *http.Request) (netip.Addr, string) {
	ip := d.ClientIP(r)
	if d.IsLAN(ip) {
		return ip, "lan"
	}
	return ip, "wan"
}
