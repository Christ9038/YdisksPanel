// Package health periodically probes service URLs and caches up/down state.
package health

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"ydiskspanel/internal/store"
)

type Checker struct {
	list     func() ([]store.Service, error)
	interval time.Duration
	client   *http.Client

	mu     sync.RWMutex
	status map[int64]string
}

func New(list func() ([]store.Service, error), interval time.Duration) *Checker {
	return &Checker{
		list: list, interval: interval, status: map[int64]string{},
		client: &http.Client{
			Timeout: 3 * time.Second,
			// Probe only: self-signed LAN certs are normal, and no secrets are sent.
			Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Status returns "up", "down" or "unknown".
func (c *Checker) Status(id int64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if s, ok := c.status[id]; ok {
		return s
	}
	return "unknown"
}

func (c *Checker) Run(ctx context.Context) {
	if c.interval <= 0 {
		return
	}
	t := time.NewTicker(c.interval)
	defer t.Stop()
	c.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.sweep(ctx)
		}
	}
}

func (c *Checker) sweep(ctx context.Context) {
	svcs, err := c.list()
	if err != nil {
		return
	}
	live := make(map[int64]bool, len(svcs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, s := range svcs {
		if !s.HealthCheck {
			continue
		}
		live[s.ID] = true
		u := s.LanURL // the server sits on the LAN, so prefer the internal address
		if u == "" {
			u = s.WanURL
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id int64, u string) {
			defer wg.Done()
			defer func() { <-sem }()
			st := c.probe(ctx, u)
			// Publish as each probe finishes so slow/unreachable hosts don't delay the rest.
			c.mu.Lock()
			c.status[id] = st
			c.mu.Unlock()
		}(s.ID, u)
	}
	wg.Wait()
	// Drop entries for deleted services or ones that stopped being checked.
	c.mu.Lock()
	for id := range c.status {
		if !live[id] {
			delete(c.status, id)
		}
	}
	c.mu.Unlock()
}

func (c *Checker) probe(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "down"
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "down"
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "down"
	}
	return "up"
}
