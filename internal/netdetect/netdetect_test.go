package netdetect

import (
	"net/http"
	"net/netip"
	"testing"
)

func pfx(s ...string) []netip.Prefix {
	var o []netip.Prefix
	for _, x := range s {
		o = append(o, netip.MustParsePrefix(x))
	}
	return o
}

func req(remote string, h map[string]string) *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return r
}

func TestZone(t *testing.T) {
	d := &Detector{Trusted: pfx("172.18.0.0/16"), LAN: pfx("192.168.0.0/16", "127.0.0.0/8", "fc00::/7")}
	cases := []struct {
		name, remote string
		h            map[string]string
		ip, zone     string
	}{
		{"direct lan", "192.168.1.5:1234", nil, "192.168.1.5", "lan"},
		{"direct wan", "8.8.8.8:1", nil, "8.8.8.8", "wan"},
		{"spoofed xff from untrusted peer", "8.8.8.8:1", map[string]string{"X-Forwarded-For": "192.168.1.1"}, "8.8.8.8", "wan"},
		{"trusted proxy real wan", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "1.2.3.4", "wan"},
		{"trusted proxy client spoofs lan prefix", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "192.168.1.1, 1.2.3.4"}, "1.2.3.4", "wan"},
		{"trusted proxy lan client", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "192.168.1.9"}, "192.168.1.9", "lan"},
		{"malformed xff", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "garbage"}, "172.18.0.2", "wan"},
		{"x-real-ip", "172.18.0.2:1", map[string]string{"X-Real-IP": "5.6.7.8"}, "5.6.7.8", "wan"},
		{"ipv4-mapped", "[::ffff:192.168.1.5]:1", nil, "192.168.1.5", "lan"},
		{"ipv6 ula", "[fd00::1]:1", nil, "fd00::1", "lan"},
		{"ipv6 global", "[2001:db8::1]:1", nil, "2001:db8::1", "wan"},
	}
	for _, c := range cases {
		ip, z := d.Zone(req(c.remote, c.h))
		if ip.String() != c.ip || z != c.zone {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, ip, z, c.ip, c.zone)
		}
	}
}
