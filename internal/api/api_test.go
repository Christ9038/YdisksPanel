package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"ydiskspanel/internal/auth"
	"ydiskspanel/internal/config"
	"ydiskspanel/internal/health"
	"ydiskspanel/internal/netdetect"
	"ydiskspanel/internal/store"
)

type client struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
	remote string
}

func (c *client) do(method, path string, body any, hdr map[string]string) (int, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = c.remote
	r.Header.Set("X-Requested-With", "ydiskspanel")
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	for _, ck := range w.Result().Cookies() {
		if ck.Name == cookieName && ck.MaxAge > 0 {
			c.cookie = ck
		}
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func newSrv(t *testing.T) (*Server, http.Handler) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, _ := auth.OpenBox(dir)
	cfg, _ := config.Load()
	cfg.DataDir = dir
	hc := health.New(st.ListServices, 0)
	s, err := New(cfg, st, box, hc, fstest.MapFS{"index.html": {Data: []byte("<html></html>")}})
	if err != nil {
		t.Fatal(err)
	}
	s.det = &netdetect.Detector{LAN: cfg.LANCIDRs}
	return s, s.Handler()
}

func TestFlow(t *testing.T) {
	s, h := newSrv(t)
	adm := &client{t, h, nil, "192.168.1.2:1"}

	// setup requires the token
	if code, _ := adm.do("POST", "/api/setup", map[string]string{"token": "nope", "username": "admin", "password": "supersecret1"}, nil); code != 403 {
		t.Fatalf("bad token: %d", code)
	}
	if code, _ := adm.do("POST", "/api/setup", map[string]string{"token": s.setupToken, "username": "admin", "password": "supersecret1"}, nil); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code, _ := adm.do("POST", "/api/setup", map[string]string{"token": "x", "username": "b", "password": "supersecret1"}, nil); code != 409 {
		t.Fatalf("setup must be one-shot: %d", code)
	}

	// CSRF
	if code, _ := adm.do("POST", "/api/groups", map[string]string{"name": "g"}, map[string]string{"X-Requested-With": ""}); code != 403 {
		t.Fatalf("csrf header: %d", code)
	}
	if code, _ := adm.do("POST", "/api/groups", map[string]string{"name": "g"}, map[string]string{"Origin": "http://evil.example"}); code != 403 {
		t.Fatalf("csrf origin: %d", code)
	}
	if code, _ := adm.do("POST", "/api/groups", map[string]string{"name": "g"}, map[string]string{"Sec-Fetch-Site": "cross-site"}); code != 403 {
		t.Fatalf("csrf sec-fetch: %d", code)
	}

	// content
	_, g := adm.do("POST", "/api/groups", map[string]string{"name": "Media"}, nil)
	gid := g["id"].(float64)
	_, tm := adm.do("POST", "/api/teams", map[string]string{"name": "family"}, nil)
	tid := tm["id"].(float64)
	svc := func(name string, teams []float64, lan, wan string) float64 {
		code, v := adm.do("POST", "/api/services", map[string]any{"groupId": gid, "name": name, "lanUrl": lan, "wanUrl": wan, "teams": teams}, nil)
		if code != 200 {
			t.Fatalf("save %s: %d %v", name, code, v)
		}
		return v["id"].(float64)
	}
	svc("public", nil, "http://192.168.1.10", "")
	secret := svc("secret", []float64{tid}, "", "https://x.example.com")
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": gid, "name": "xss", "wanUrl": "javascript:alert(1)"}, nil); code != 400 {
		t.Fatalf("javascript: url must be rejected: %d", code)
	}
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": gid, "name": "badcolor", "wanUrl": "https://a.example", "color": "red;background:url(x)"}, nil); code != 400 {
		t.Fatalf("bad color must be rejected: %d", code)
	}
	if code, v := adm.do("POST", "/api/services", map[string]any{"groupId": gid, "name": "colored", "wanUrl": "https://a.example", "color": "#1A2b3C"}, nil); code != 200 || v["color"] != "#1A2b3C" {
		t.Fatalf("color should persist: %d %v", code, v)
	}
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": gid, "name": "none"}, nil); code != 400 {
		t.Fatalf("needs a url: %d", code)
	}
	if code, _ := adm.do("DELETE", "/api/teams/"+itoa(tid), nil, nil); code != 409 {
		t.Fatalf("team in use must not be deletable: %d", code)
	}

	// regular user
	if code, _ := adm.do("POST", "/api/users", map[string]any{"username": "bob", "password": "bobspassword1", "role": "user"}, nil); code != 200 {
		t.Fatalf("create user: %d", code)
	}
	bob := &client{t, h, nil, "8.8.8.8:1"}
	if code, _ := bob.do("POST", "/api/auth/login", map[string]any{"username": "bob", "password": "bobspassword1"}, nil); code != 200 {
		t.Fatalf("bob login: %d", code)
	}
	_, d := bob.do("GET", "/api/dashboard", nil, nil)
	if n := len(d["services"].([]any)); n != 2 {
		t.Fatalf("bob should see 2 services, got %d", n)
	}
	if code, _ := bob.do("GET", "/api/services/"+itoa(secret)+"/favicon", nil, nil); code != 404 {
		t.Fatalf("favicon of a restricted service must be hidden: %d", code)
	}
	if code, _ := bob.do("GET", "/api/users", nil, nil); code != 403 {
		t.Fatalf("bob must not list users: %d", code)
	}
	if code, _ := bob.do("DELETE", "/api/services/"+itoa(secret), nil, nil); code != 403 {
		t.Fatalf("bob must not delete: %d", code)
	}
	if _, nw := bob.do("GET", "/api/network", nil, nil); nw["zone"] != "wan" {
		t.Fatalf("zone: %v", nw)
	}
	if code, _ := (&client{t, h, nil, "1.1.1.1:1"}).do("GET", "/api/dashboard", nil, nil); code != 401 {
		t.Fatalf("anon dashboard: %d", code)
	}

	// last admin protections
	_, me := adm.do("GET", "/api/me", nil, nil)
	aid := me["user"].(map[string]any)["id"].(float64)
	if code, _ := adm.do("PATCH", "/api/users/"+itoa(aid), map[string]any{"role": "user"}, nil); code != 409 {
		t.Fatalf("demote last admin: %d", code)
	}
	if code, _ := adm.do("DELETE", "/api/users/"+itoa(aid), nil, nil); code != 409 {
		t.Fatalf("delete self: %d", code)
	}

	// disabling bob revokes his session
	_, bu := adm.do("GET", "/api/users", nil, nil)
	_ = bu
	if code, _ := adm.do("PATCH", "/api/users/2", map[string]any{"disabled": true}, nil); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	if code, _ := bob.do("GET", "/api/dashboard", nil, nil); code != 401 {
		t.Fatalf("disabled user's session must die: %d", code)
	}
}

func itoa(f float64) string { b, _ := json.Marshal(int64(f)); return string(b) }

func TestLoginLockout(t *testing.T) {
	_, h := newSrv(t)
	c := &client{t, h, nil, "9.9.9.9:1"}
	var last int
	for i := 0; i < 8; i++ {
		last, _ = c.do("POST", "/api/auth/login", map[string]any{"username": "ghost", "password": "whatever-pass"}, nil)
	}
	if last != 429 {
		t.Fatalf("expected lockout, got %d", last)
	}
	_ = time.Second
}

func TestMissingAssetIs404NotShell(t *testing.T) {
	_, h := newSrv(t)
	for path, want := range map[string]int{"/favicon.svg": 404, "/assets/x.js": 404, "/settings/security": 200, "/": 200} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Errorf("%s: got %d want %d", path, w.Code, want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, h := newSrv(t)
	r := httptest.NewRequest("GET", "/some/spa/route", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' data: blob:") {
		t.Fatalf("CSP must allow blob: images for the crop preview: %q", csp)
	}
	if w.Code != 200 || w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("headers/spa: %d %v", w.Code, w.Header())
	}
}
