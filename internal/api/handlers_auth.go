package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ydiskspanel/internal/auth"
	"ydiskspanel/internal/store"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

const (
	sessionTTL  = 7 * 24 * time.Hour
	rememberTTL = 30 * 24 * time.Hour
)

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	s.setupMu.Lock()
	needs := s.setupToken != ""
	s.setupMu.Unlock()
	writeJSON(w, 200, map[string]any{
		"needsSetup": needs,
		"siteName":   s.st.GetSetting("siteName", "YdisksPanel"),
		"wallpaper":  s.st.GetSetting("wallpaper", ""),
	})
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	ip, zone := s.det.Zone(r)
	writeJSON(w, 200, map[string]string{"ip": ip.String(), "zone": zone})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token, Username, Password string }
	if !decode(w, r, &in) {
		return
	}
	ipk := s.ip(r)
	if ok, d := s.setupLim.Allow(ipk); !ok {
		tooMany(w, d)
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupToken == "" {
		writeErr(w, 409, "setup already completed")
		return
	}
	if !eqConst(strings.TrimSpace(in.Token), s.setupToken) {
		s.setupLim.Fail(ipk)
		writeErr(w, 403, "invalid setup token")
		return
	}
	if !usernameRe.MatchString(in.Username) {
		writeErr(w, 400, "username must be 3-32 chars: letters, digits, _ . -")
		return
	}
	if err := auth.CheckPasswordPolicy(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		serverErr(w, err)
		return
	}
	u, err := s.st.CreateUser(in.Username, in.Username, hash, "admin")
	if err != nil {
		serverErr(w, err)
		return
	}
	s.setupToken = ""
	s.audit(r, u, "setup", "administrator created")
	if err := s.startSession(w, r, u, false); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

func tooMany(w http.ResponseWriter, d time.Duration) {
	secs := int(d.Seconds()) + 1
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeErrCode(w, 429, fmt.Sprintf("too many attempts, retry in %ds", secs), "rate_limited")
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User, remember bool) error {
	token, stored, err := auth.NewToken()
	if err != nil {
		return err
	}
	ttl := sessionTTL
	if remember {
		ttl = rememberTTL
	}
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	if err := s.st.CreateSession(stored, u.ID, ttl, s.ip(r), ua); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: s.secureCookie(r), MaxAge: int(ttl.Seconds()),
	})
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username, Password string
		Remember           bool
	}
	if !decode(w, r, &in) {
		return
	}
	ip := s.ip(r)
	uname := strings.ToLower(strings.TrimSpace(in.Username))
	keys := []struct {
		l *auth.Limiter
		k string
	}{{s.loginIP, ip}, {s.loginUser, uname}, {s.loginPair, ip + "|" + uname}}
	for _, k := range keys {
		if ok, d := k.l.Allow(k.k); !ok {
			tooMany(w, d)
			return
		}
	}
	fail := func() {
		for _, k := range keys {
			k.l.Fail(k.k)
		}
		s.st.Audit(nil, uname, "login_failed", "", ip)
		writeErr(w, 401, "invalid username or password")
	}
	if len(in.Password) > auth.MaxPasswordLen {
		fail()
		return
	}
	u, err := s.st.GetUserByName(uname)
	if err != nil {
		auth.BurnPasswordCheck(in.Password)
		fail()
		return
	}
	if !auth.VerifyPassword(in.Password, u.PasswordHash) || u.Disabled {
		fail()
		return
	}
	if u.TOTPEnabled {
		ch, stored, err := auth.NewToken()
		if err != nil {
			serverErr(w, err)
			return
		}
		if err := s.st.CreateChallenge(stored, u.ID, in.Remember, 5*time.Minute); err != nil {
			serverErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"totpRequired": true, "challenge": ch})
		return
	}
	s.finishLogin(w, r, u, in.Remember, keys[0].k, keys[1].k, keys[2].k)
}

func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, u *store.User, remember bool, ip, uname, pair string) {
	s.loginIP.Reset(ip)
	s.loginUser.Reset(uname)
	s.loginPair.Reset(pair)
	if err := s.startSession(w, r, u, remember); err != nil {
		serverErr(w, err)
		return
	}
	s.audit(r, u, "login", "")
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) handleLoginTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct{ Challenge, Code string }
	if !decode(w, r, &in) {
		return
	}
	id := auth.HashToken(in.Challenge)
	if ok, d := s.totpLim.Allow(s.ip(r)); !ok {
		tooMany(w, d)
		return
	}
	ch, err := s.st.GetChallenge(id)
	if err != nil || ch.Attempts >= 5 {
		_ = s.st.DeleteChallenge(id)
		writeErr(w, 401, "login expired, please sign in again")
		return
	}
	u, err := s.st.GetUser(ch.UserID)
	if err != nil || u.Disabled || !u.TOTPEnabled {
		writeErr(w, 401, "login expired, please sign in again")
		return
	}
	_ = s.st.BumpChallenge(id)
	good := false
	code := strings.TrimSpace(in.Code)
	if secret, err := s.box.Open(u.TOTPSecret); err == nil && len(code) == 6 {
		if step, ok := auth.ValidateTOTP(code, secret); ok {
			// Reject replay of a code that was already accepted.
			if claimed, _ := s.st.ClaimTOTPStep(u.ID, step); claimed {
				good = true
			}
		}
	} else if len(code) > 6 {
		good, _ = s.st.ConsumeRecoveryCode(u.ID, auth.HashToken(auth.NormalizeRecovery(code)))
		if good {
			s.audit(r, u, "recovery_code_used", "")
		}
	}
	if !good {
		s.totpLim.Fail(s.ip(r))
		s.st.Audit(&u.ID, u.Username, "totp_failed", "", s.ip(r))
		writeErr(w, 401, "invalid verification code")
		return
	}
	_ = s.st.DeleteChallenge(id)
	s.finishLogin(w, r, u, ch.Remember, s.ip(r), strings.ToLower(u.Username), s.ip(r)+"|"+strings.ToLower(u.Username))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, u *store.User) {
	if sess := sessionFrom(r); sess != nil {
		_ = s.st.DeleteSession(sess.ID)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(204)
}

// ---------- /api/me ----------

func (s *Server) meView(u *store.User) map[string]any {
	return map[string]any{
		"user":          u,
		"mustEnroll2fa": s.st.GetSetting("require2fa", "0") == "1" && !u.TOTPEnabled,
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, u *store.User) {
	writeJSON(w, 200, s.meView(u))
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ DisplayName string }
	if !decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.DisplayName)
	if len([]rune(name)) == 0 || len([]rune(name)) > 40 {
		writeErr(w, 400, "display name must be 1-40 characters")
		return
	}
	if err := s.st.UpdateUser(u.ID, name, u.Role, u.Disabled); err != nil {
		serverErr(w, err)
		return
	}
	u.DisplayName = name
	writeJSON(w, 200, s.meView(u))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ Current, New string }
	if !decode(w, r, &in) {
		return
	}
	key := "pw:" + fmt.Sprint(u.ID)
	if ok, d := s.totpLim.Allow(key); !ok {
		tooMany(w, d)
		return
	}
	if !auth.VerifyPassword(in.Current, u.PasswordHash) {
		s.totpLim.Fail(key)
		writeErr(w, 403, "current password is incorrect")
		return
	}
	if err := auth.CheckPasswordPolicy(in.New); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := s.st.SetPassword(u.ID, hash); err != nil {
		serverErr(w, err)
		return
	}
	keep := ""
	if sess := sessionFrom(r); sess != nil {
		keep = sess.ID
	}
	_ = s.st.DeleteUserSessions(u.ID, keep)
	s.audit(r, u, "password_changed", "")
	w.WriteHeader(204)
}

func (s *Server) handleTOTPEnroll(w http.ResponseWriter, r *http.Request, u *store.User) {
	if u.TOTPEnabled {
		writeErr(w, 409, "two-factor is already enabled")
		return
	}
	key, err := auth.GenerateTOTP(s.st.GetSetting("siteName", "YdisksPanel"), u.Username)
	if err != nil {
		serverErr(w, err)
		return
	}
	enc, err := s.box.Seal(key.Secret())
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := s.st.SetTOTPSecret(u.ID, enc); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"secret": key.Secret(), "uri": key.URL()})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ Code string }
	if !decode(w, r, &in) {
		return
	}
	if u.TOTPEnabled || u.TOTPSecret == "" {
		writeErr(w, 409, "start enrollment first")
		return
	}
	secret, err := s.box.Open(u.TOTPSecret)
	if err != nil {
		serverErr(w, err)
		return
	}
	step, ok := auth.ValidateTOTP(in.Code, secret)
	if !ok {
		writeErr(w, 400, "invalid verification code")
		return
	}
	_, _ = s.st.ClaimTOTPStep(u.ID, step)
	codes, hashes, err := auth.NewRecoveryCodes(10)
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := s.st.EnableTOTP(u.ID, hashes); err != nil {
		serverErr(w, err)
		return
	}
	s.audit(r, u, "totp_enabled", "")
	writeJSON(w, 200, map[string]any{"recoveryCodes": codes})
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ Password string }
	if !decode(w, r, &in) {
		return
	}
	if s.st.GetSetting("require2fa", "0") == "1" {
		writeErr(w, 403, "two-factor is required by the administrator")
		return
	}
	if !auth.VerifyPassword(in.Password, u.PasswordHash) {
		writeErr(w, 403, "password is incorrect")
		return
	}
	if err := s.st.DisableTOTP(u.ID); err != nil {
		serverErr(w, err)
		return
	}
	s.audit(r, u, "totp_disabled", "")
	w.WriteHeader(204)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.st.ListSessions(u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	cur := ""
	if sess := sessionFrom(r); sess != nil {
		cur = sess.ID
	}
	for i := range list {
		list[i].Current = list[i].ID == cur
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.st.DeleteUserSession(u.ID, r.PathValue("id")); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(204)
}
