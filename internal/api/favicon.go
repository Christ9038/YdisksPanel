package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"ydiskspanel/internal/store"
)

const (
	faviconMax      = 256 << 10
	faviconPageMax  = 512 << 10
	faviconTTL      = 7 * 24 * time.Hour
	faviconMissTTL  = time.Hour
	faviconFetchTMO = 5 * time.Second
)

var (
	faviconSem    = make(chan struct{}, 4) // bound concurrent cold fetches
	faviconFlight = struct {
		sync.Mutex
		m map[string]*flight
	}{m: map[string]*flight{}}
	faviconClient = &http.Client{
		Timeout: faviconFetchTMO,
		// LAN services commonly use self-signed certs; this only downloads a public icon.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
)

type flight struct {
	done chan struct{}
	data []byte
}

// fetchOnce collapses concurrent requests for the same key into one upstream fetch.
func fetchOnce(key, base string) []byte {
	faviconFlight.Lock()
	if f, ok := faviconFlight.m[key]; ok {
		faviconFlight.Unlock()
		<-f.done
		return f.data
	}
	f := &flight{done: make(chan struct{})}
	faviconFlight.m[key] = f
	faviconFlight.Unlock()

	faviconSem <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	f.data = fetchFavicon(ctx, base)
	cancel()
	<-faviconSem

	faviconFlight.Lock()
	delete(faviconFlight.m, key)
	faviconFlight.Unlock()
	close(f.done)
	return f.data
}

// handleFavicon serves the cached favicon of a service's website, fetching it
// server-side (the panel can reach LAN hosts that remote visitors cannot).
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	svc, err := s.st.GetService(id)
	if err != nil || !store.VisibleTo(svc, u) {
		http.NotFound(w, r)
		return
	}
	base := svc.LanURL
	if base == "" {
		base = svc.WanURL
	}
	sum := sha256.Sum256([]byte(base))
	dir := filepath.Join(s.cfg.DataDir, "favicons")
	key := strconv.FormatInt(id, 10) + "-" + hex.EncodeToString(sum[:6])
	hit, miss := filepath.Join(dir, key+".img"), filepath.Join(dir, key+".none")

	serve := func(data []byte) {
		ct := sniffImage(data)
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox") // SVGs must never run script
		w.Header().Set("Cache-Control", "private, max-age=86400")
		_, _ = w.Write(data)
	}
	if st, err := os.Stat(hit); err == nil && time.Since(st.ModTime()) < faviconTTL {
		if data, err := os.ReadFile(hit); err == nil {
			serve(data)
			return
		}
	}
	if st, err := os.Stat(miss); err == nil && time.Since(st.ModTime()) < faviconMissTTL {
		http.NotFound(w, r)
		return
	}

	data := fetchOnce(key, base)
	_ = os.MkdirAll(dir, 0o700)
	if data == nil {
		_ = os.WriteFile(miss, nil, 0o600)
		http.NotFound(w, r)
		return
	}
	_ = os.Remove(miss)
	_ = os.WriteFile(hit, data, 0o600)
	serve(data)
}

func sniffImage(b []byte) string {
	if bytes.Contains(b[:min(len(b), 512)], []byte("<svg")) {
		return "image/svg+xml"
	}
	if len(b) > 4 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0 {
		return "image/x-icon"
	}
	return http.DetectContentType(b)
}

func isAllowedImage(b []byte) bool {
	switch sniffImage(b) {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/vnd.microsoft.icon", "image/svg+xml":
		return true
	}
	return false
}

func fetchFavicon(ctx context.Context, rawBase string) []byte {
	base, err := url.Parse(rawBase)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return nil
	}
	var candidates []string
	page, final, err := fetchPage(ctx, base.String())
	if err != nil {
		return nil // host unreachable: /favicon.ico would only time out again
	}
	if page != nil {
		candidates = iconLinks(page, final)
	}
	root := &url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/favicon.ico"}
	candidates = append(candidates, root.String())
	for _, c := range candidates {
		u, err := url.Parse(c)
		// Only same-host icons: a page must not be able to steer the panel at other internal hosts.
		if err != nil || u.Hostname() != base.Hostname() || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		if data := fetchBytes(ctx, c, faviconMax); data != nil && isAllowedImage(data) {
			return data
		}
	}
	return nil
}

func fetchBytes(ctx context.Context, u string, limit int64) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "YdisksPanel-Favicon/1.0")
	resp, err := faviconClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil
	}
	return data
}

func fetchPage(ctx context.Context, u string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "YdisksPanel-Favicon/1.0")
	req.Header.Set("Accept", "text/html")
	resp, err := faviconClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, faviconPageMax))
	return data, resp.Request.URL, nil
}

// iconLinks returns <link rel="icon"…> targets, best (largest) first.
func iconLinks(page []byte, base *url.URL) []string {
	type cand struct {
		href  string
		score int
	}
	var out []cand
	z := html.NewTokenizer(bytes.NewReader(page))
	for {
		switch z.Next() {
		case html.ErrorToken:
			sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
			res := make([]string, 0, len(out))
			for _, c := range out {
				res = append(res, c.href)
			}
			return res
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "link" || !hasAttr {
				continue
			}
			var rel, href, sizes string
			for {
				k, v, more := z.TagAttr()
				switch string(k) {
				case "rel":
					rel = strings.ToLower(string(v))
				case "href":
					href = strings.TrimSpace(string(v))
				case "sizes":
					sizes = strings.ToLower(string(v))
				}
				if !more {
					break
				}
			}
			if href == "" || !strings.Contains(rel, "icon") || strings.Contains(rel, "mask-icon") {
				continue
			}
			ref, err := base.Parse(href)
			if err != nil {
				continue
			}
			score := 16
			if sizes == "any" || strings.HasSuffix(strings.ToLower(ref.Path), ".svg") {
				score = 128
			} else if i := strings.Index(sizes, "x"); i > 0 {
				if n, err := strconv.Atoi(sizes[:i]); err == nil {
					score = min(n, 256)
				}
			}
			if strings.Contains(rel, "apple-touch-icon") {
				score += 1 // slight preference: high-res, opaque
			}
			out = append(out, cand{ref.String(), score})
		}
	}
}
