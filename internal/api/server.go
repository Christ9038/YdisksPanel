// Package api wires HTTP routes, middleware and handlers.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ydiskspanel/internal/auth"
	"ydiskspanel/internal/config"
	"ydiskspanel/internal/health"
	"ydiskspanel/internal/netdetect"
	"ydiskspanel/internal/store"
)

const cookieName = "ydiskspanel_session"

type Server struct {
	cfg    *config.Config
	st     *store.Store
	box    *auth.Box
	det    *netdetect.Detector
	health *health.Checker
	static fs.FS

	loginIP   *auth.Limiter
	loginUser *auth.Limiter
	loginPair *auth.Limiter
	totpLim   *auth.Limiter
	setupLim  *auth.Limiter

	setupMu    sync.Mutex
	setupToken string
}

func New(cfg *config.Config, st *store.Store, box *auth.Box, h *health.Checker, static fs.FS) (*Server, error) {
	s := &Server{
		cfg: cfg, st: st, box: box, health: h, static: static,
		det:       &netdetect.Detector{Trusted: cfg.TrustedProxies, LAN: cfg.LANCIDRs},
		loginIP:   auth.NewLimiter(20, 30*time.Second, 30*time.Minute),
		loginUser: auth.NewLimiter(10, 30*time.Second, 15*time.Minute),
		loginPair: auth.NewLimiter(5, 30*time.Second, 30*time.Minute),
		totpLim:   auth.NewLimiter(5, 30*time.Second, 30*time.Minute),
		setupLim:  auth.NewLimiter(5, time.Minute, time.Hour),
	}
	n, err := st.CountUsers()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		b := make([]byte, 12)
		_, _ = rand.Read(b)
		s.setupToken = hex.EncodeToString(b)
		log.Printf("==================================================")
		log.Printf(" No users yet. Open the panel and use this one-time")
		log.Printf(" setup token to create the administrator:")
		log.Printf("   %s", s.setupToken)
		log.Printf("==================================================")
	}
	return s, nil
}

// Maintain runs periodic cleanup until ctx ends.
func (s *Server) Maintain(ctx context.Context) {
	_, _ = s.st.PurgeOrphanBlobs(time.Hour)
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.st.PurgeExpired()
			_, _ = s.st.PurgeOrphanBlobs(time.Hour)
			for _, l := range []*auth.Limiter{s.loginIP, s.loginUser, s.loginPair, s.totpLim, s.setupLim} {
				l.Sweep()
			}
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	pub := func(p string, h http.HandlerFunc) { mux.HandleFunc(p, h) }
	usr := func(p string, h authedHandler) { mux.HandleFunc(p, s.requireUser(h)) }
	adm := func(p string, h authedHandler) { mux.HandleFunc(p, s.requireAdmin(h)) }

	pub("GET /api/bootstrap", s.handleBootstrap)
	pub("POST /api/setup", s.handleSetup)
	pub("POST /api/auth/login", s.handleLogin)
	pub("POST /api/auth/totp", s.handleLoginTOTP)
	pub("GET /api/network", s.handleNetwork)
	usr("POST /api/auth/logout", s.handleLogout)

	usr("GET /api/me", s.handleMe)
	usr("PATCH /api/me", s.handleUpdateMe)
	usr("PUT /api/me/password", s.handleChangePassword)
	usr("POST /api/me/totp/enroll", s.handleTOTPEnroll)
	usr("POST /api/me/totp/enable", s.handleTOTPEnable)
	usr("POST /api/me/totp/disable", s.handleTOTPDisable)
	usr("GET /api/me/sessions", s.handleListSessions)
	usr("DELETE /api/me/sessions/{id}", s.handleDeleteSession)

	usr("GET /api/dashboard", s.handleDashboard)
	usr("GET /api/services/{id}/favicon", s.handleFavicon)

	adm("POST /api/services", s.handleSaveService)
	adm("PUT /api/services/{id}", s.handleSaveService)
	adm("DELETE /api/services/{id}", s.handleDeleteService)
	adm("PUT /api/layout", s.handleLayout)
	adm("POST /api/groups", s.handleCreateGroup)
	adm("PATCH /api/groups/{id}", s.handleRenameGroup)
	adm("DELETE /api/groups/{id}", s.handleDeleteGroup)
	adm("PUT /api/groups/order", s.handleReorderGroups)
	adm("GET /api/teams", s.handleListTeams)
	adm("POST /api/teams", s.handleCreateTeam)
	adm("DELETE /api/teams/{id}", s.handleDeleteTeam)
	adm("GET /api/users", s.handleListUsers)
	adm("POST /api/users", s.handleCreateUser)
	adm("PATCH /api/users/{id}", s.handleUpdateUser)
	adm("DELETE /api/users/{id}", s.handleDeleteUser)
	adm("GET /api/settings", s.handleGetSettings)
	adm("PUT /api/settings", s.handlePutSettings)
	adm("GET /api/audit", s.handleAudit)
	adm("POST /api/uploads/icon", s.handleUploadIcon)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "not found") })
	mux.Handle("GET /icons/", s.iconHandler())
	mux.Handle("/", s.spaHandler())

	return s.securityHeaders(s.csrf(mux))
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeErrCode(w http.ResponseWriter, code int, msg, c string) {
	writeJSON(w, code, map[string]string{"error": msg, "code": c})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "invalid request body")
		return false
	}
	return true
}

func serverErr(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeErr(w, 500, "internal error")
}

func (s *Server) ip(r *http.Request) string { return s.det.ClientIP(r).String() }

func (s *Server) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.fromTrustedPeer(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// fromTrustedPeer reports whether the direct TCP peer is a configured proxy.
func (s *Server) fromTrustedPeer(r *http.Request) bool {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	a := ap.Addr().Unmap()
	for _, p := range s.cfg.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (s *Server) secureCookie(r *http.Request) bool {
	switch s.cfg.CookieSecure {
	case "always":
		return true
	case "never":
		return false
	}
	return s.isHTTPS(r)
}

// ---------- middleware ----------

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: http: https:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if s.isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// csrf protects state-changing requests: a custom header (which cross-site
// forms cannot send) plus same-origin Origin / Sec-Fetch-Site checks.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Requested-With") != "ydiskspanel" {
			writeErr(w, 403, "missing CSRF header")
			return
		}
		if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
			writeErr(w, 403, "cross-site request blocked")
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || !s.hostAllowed(r, u.Host) {
				writeErr(w, 403, "origin not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(r *http.Request, host string) bool {
	if strings.EqualFold(host, r.Host) {
		return true
	}
	if s.fromTrustedPeer(r) {
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" && strings.EqualFold(host, strings.TrimSpace(strings.Split(fh, ",")[0])) {
			return true
		}
	}
	return false
}

type ctxKey int

const (
	userKey ctxKey = iota
	sessKey
)

type authedHandler func(w http.ResponseWriter, r *http.Request, u *store.User)

func (s *Server) currentUser(r *http.Request) (*store.User, *store.Session) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	sess, err := s.st.GetSession(auth.HashToken(c.Value))
	if err != nil {
		return nil, nil
	}
	if sess.ExpiresAt < time.Now().Unix() {
		_ = s.st.DeleteSession(sess.ID)
		return nil, nil
	}
	u, err := s.st.GetUser(sess.UserID)
	if err != nil || u.Disabled {
		return nil, nil
	}
	// Slide expiry at most once a minute to avoid a write per request.
	if time.Now().Unix()-sess.LastSeen > 60 {
		_ = s.st.TouchSession(sess)
	}
	return u, sess
}

func (s *Server) requireUser(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, sess := s.currentUser(r)
		if u == nil {
			writeErrCode(w, 401, "authentication required", "unauthenticated")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessKey, sess))
		// Enforce mandatory 2FA server-side, not just in the UI.
		if s.st.GetSetting("require2fa", "0") == "1" && !u.TOTPEnabled &&
			!strings.HasPrefix(r.URL.Path, "/api/me") && r.URL.Path != "/api/auth/logout" {
			writeErrCode(w, 403, "two-factor enrollment required", "2fa_enrollment_required")
			return
		}
		h(w, r, u)
	}
}

func (s *Server) requireAdmin(h authedHandler) http.HandlerFunc {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if u.Role != "admin" {
			writeErr(w, 403, "administrator only")
			return
		}
		h(w, r, u)
	})
}

func sessionFrom(r *http.Request) *store.Session {
	x, _ := r.Context().Value(sessKey).(*store.Session)
	return x
}

func (s *Server) audit(r *http.Request, u *store.User, action, detail string) {
	var id *int64
	name := ""
	if u != nil {
		id, name = &u.ID, u.Username
	}
	s.st.Audit(id, name, action, detail, s.ip(r))
}

// ---------- static ----------

func (s *Server) spaHandler() http.Handler {
	files := http.FS(s.static)
	fileServer := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, 405, "method not allowed")
			return
		}
		p := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")
		if p != "" {
			if f, err := s.static.Open(p); err == nil {
				st, _ := f.Stat()
				f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(p, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}
		// A missing *file* (e.g. /favicon.svg) must 404, not return the HTML shell: browsers would
		// otherwise cache the page as the site icon.
		if ext := filepath.Ext(p); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// iconHandler serves stored images. They're public (the login page shows the
// wallpaper) but unguessable, immutable and sandboxed.
func (s *Server) iconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Base(r.URL.Path)
		m := iconNameRe.FindStringSubmatch(name)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		id := name[:32]
		etag := `"` + id + `"`
		h := w.Header()
		h.Set("ETag", etag)
		h.Set("Cache-Control", "public, max-age=31536000, immutable") // content never changes for a given id
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		b, err := s.st.GetBlob(id)
		if err != nil || b.Ext != m[1] {
			h.Del("Cache-Control")
			http.NotFound(w, r)
			return
		}
		h.Set("Content-Type", b.ContentType)
		h.Set("Content-Length", strconv.Itoa(len(b.Data)))
		_, _ = w.Write(b.Data)
	})
}

func eqConst(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
