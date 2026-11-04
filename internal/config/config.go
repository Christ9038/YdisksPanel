// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"
)

type Config struct {
	Listen         string
	DataDir        string
	TrustedProxies []netip.Prefix
	LANCIDRs       []netip.Prefix
	TLSCert        string
	TLSKey         string
	CookieSecure   string // auto | always | never
	HealthInterval time.Duration
}

var defaultLAN = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10",
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func ParseCIDRs(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR/IP %q", part)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func Load() (*Config, error) {
	c := &Config{
		Listen:       env("YDP_LISTEN", ":8080"),
		DataDir:      env("YDP_DATA_DIR", "./data"),
		TLSCert:      env("YDP_TLS_CERT", ""),
		TLSKey:       env("YDP_TLS_KEY", ""),
		CookieSecure: env("YDP_COOKIE_SECURE", "auto"),
	}
	var err error
	if c.TrustedProxies, err = ParseCIDRs(env("YDP_TRUSTED_PROXIES", "")); err != nil {
		return nil, fmt.Errorf("YDP_TRUSTED_PROXIES: %w", err)
	}
	if c.LANCIDRs, err = ParseCIDRs(env("YDP_LAN_CIDRS", strings.Join(defaultLAN, ","))); err != nil {
		return nil, fmt.Errorf("YDP_LAN_CIDRS: %w", err)
	}
	if c.HealthInterval, err = time.ParseDuration(env("YDP_HEALTH_INTERVAL", "60s")); err != nil {
		return nil, fmt.Errorf("YDP_HEALTH_INTERVAL: %w", err)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return nil, fmt.Errorf("YDP_TLS_CERT and YDP_TLS_KEY must be set together")
	}
	return c, nil
}
