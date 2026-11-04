package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"ydiskspanel/internal/auth"
	"ydiskspanel/internal/imgproc"
	"ydiskspanel/internal/store"
)

var (
	iconNameRe = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg)$`)
	lucideRe   = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	colorRe    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// ---------- dashboard ----------

type svcView struct {
	store.Service
	Status string `json:"status"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, u *store.User) {
	groups, err := s.st.ListGroups()
	if err != nil {
		serverErr(w, err)
		return
	}
	all, err := s.st.ListServices()
	if err != nil {
		serverErr(w, err)
		return
	}
	svcs := []svcView{}
	has := map[int64]bool{}
	for i := range all {
		if !store.VisibleTo(&all[i], u) {
			continue
		}
		v := svcView{Service: all[i], Status: s.health.Status(all[i].ID)}
		if u.Role != "admin" {
			v.Teams = []int64{} // don't leak ACL structure
		}
		has[v.GroupID] = true
		svcs = append(svcs, v)
	}
	if u.Role != "admin" {
		kept := groups[:0]
		for _, g := range groups {
			if has[g.ID] {
				kept = append(kept, g)
			}
		}
		groups = kept
	}
	writeJSON(w, 200, map[string]any{"groups": groups, "services": svcs})
}

// ---------- services ----------

func validURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 500 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validIcon(i string) bool {
	switch {
	case i == "":
		return true
	case strings.HasPrefix(i, "http://") || strings.HasPrefix(i, "https://"):
		return validURL(i)
	case strings.HasPrefix(i, "/icons/"):
		return iconNameRe.MatchString(strings.TrimPrefix(i, "/icons/"))
	case lucideRe.MatchString(i):
		return true
	}
	// a short emoji / symbol
	return utf8.RuneCountInString(i) <= 8 && !strings.ContainsAny(i, "<>\"'`/\\ ")
}

func (s *Server) handleSaveService(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in store.Service
	if !decode(w, r, &in) {
		return
	}
	if r.Method == http.MethodPut {
		id, ok := pathID(r)
		if !ok {
			writeErr(w, 400, "invalid id")
			return
		}
		in.ID = id
	} else {
		in.ID = 0
	}
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if in.IconFit == "" {
		in.IconFit = "contain"
	}
	in.LanURL, in.WanURL, in.Icon = strings.TrimSpace(in.LanURL), strings.TrimSpace(in.WanURL), strings.TrimSpace(in.Icon)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 60:
		writeErr(w, 400, "name must be 1-60 characters")
	case utf8.RuneCountInString(in.Description) > 200:
		writeErr(w, 400, "description too long")
	case in.LanURL == "" && in.WanURL == "":
		writeErr(w, 400, "provide at least one of the LAN / WAN URLs")
	case !validURL(in.LanURL) || !validURL(in.WanURL):
		writeErr(w, 400, "URLs must be http(s)")
	case !validIcon(in.Icon):
		writeErr(w, 400, "invalid icon")
	case in.IconFit != "" && in.IconFit != "contain" && in.IconFit != "cover":
		writeErr(w, 400, "iconFit must be contain or cover")
	case in.Color != "" && !colorRe.MatchString(in.Color):
		writeErr(w, 400, "color must be like #1a2b3c")
	case in.GroupID == 0:
		writeErr(w, 400, "group is required")
	default:
		if err := s.st.SaveService(&in); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 400, "invalid group or team")
			}
			return
		}
		s.audit(r, u, "service_saved", in.Name)
		saved, _ := s.st.GetService(in.ID)
		writeJSON(w, 200, saved)
	}
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if err := s.st.DeleteService(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, u, "service_deleted", strconv.FormatInt(id, 10))
	w.WriteHeader(204)
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.Is(err, store.ErrConflict):
		writeErr(w, 409, "conflict: still in use or already exists")
	default:
		serverErr(w, err)
	}
}

// handleLayout persists drag-and-drop results: group id -> ordered service ids.
func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Layout map[string][]int64 `json:"layout"`
	}
	if !decode(w, r, &in) {
		return
	}
	m := map[int64][]int64{}
	for k, ids := range in.Layout {
		gid, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid layout")
			return
		}
		m[gid] = ids
	}
	if err := s.st.MoveServices(m); err != nil {
		writeErr(w, 400, "invalid layout")
		return
	}
	w.WriteHeader(204)
}

// ---------- groups ----------

func cleanName(w http.ResponseWriter, n string) (string, bool) {
	n = strings.TrimSpace(n)
	if n == "" || utf8.RuneCountInString(n) > 40 {
		writeErr(w, 400, "name must be 1-40 characters")
		return "", false
	}
	return n, true
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ Name string }
	if !decode(w, r, &in) {
		return
	}
	n, ok := cleanName(w, in.Name)
	if !ok {
		return
	}
	g, err := s.st.CreateGroup(n)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) handleRenameGroup(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, ok := pathID(r)
	var in struct{ Name string }
	if !ok || !decode(w, r, &in) {
		return
	}
	n, ok := cleanName(w, in.Name)
	if !ok {
		return
	}
	if err := s.st.RenameGroup(id, n); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if err := s.st.DeleteGroup(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, u, "group_deleted", strconv.FormatInt(id, 10))
	w.WriteHeader(204)
}

func (s *Server) handleReorderGroups(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ IDs []int64 }
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.ReorderGroups(in.IDs); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(204)
}

// ---------- teams ----------

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request, u *store.User) {
	t, err := s.st.ListTeams()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct{ Name string }
	if !decode(w, r, &in) {
		return
	}
	n, ok := cleanName(w, in.Name)
	if !ok {
		return
	}
	t, err := s.st.CreateTeam(n)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if err := s.st.DeleteTeam(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(204)
}

// ---------- users ----------

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.st.ListUsers()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

type userIn struct {
	Username    string
	DisplayName string
	Password    string
	Role        string
	Disabled    bool
	Teams       []int64
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, admin *store.User) {
	var in userIn
	if !decode(w, r, &in) {
		return
	}
	if !usernameRe.MatchString(in.Username) {
		writeErr(w, 400, "username must be 3-32 chars: letters, digits, _ . -")
		return
	}
	if in.Role != "admin" && in.Role != "user" {
		in.Role = "user"
	}
	if err := auth.CheckPasswordPolicy(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	dn := strings.TrimSpace(in.DisplayName)
	if dn == "" {
		dn = in.Username
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		serverErr(w, err)
		return
	}
	u, err := s.st.CreateUser(strings.ToLower(in.Username), dn, hash, in.Role)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = s.st.SetUserTeams(u.ID, in.Teams)
	s.audit(r, admin, "user_created", u.Username)
	u, _ = s.st.GetUser(u.ID)
	writeJSON(w, 200, u)
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, admin *store.User) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	var in userIn
	if !decode(w, r, &in) {
		return
	}
	u, err := s.st.GetUser(id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	role := in.Role
	if role != "admin" && role != "user" {
		role = u.Role
	}
	dn := strings.TrimSpace(in.DisplayName)
	if dn == "" {
		dn = u.DisplayName
	}
	// Never allow the last active administrator to be demoted or disabled.
	if u.Role == "admin" && !u.Disabled && (role != "admin" || in.Disabled) {
		if n, _ := s.st.CountActiveAdmins(); n <= 1 {
			writeErr(w, 409, "cannot demote or disable the last administrator")
			return
		}
	}
	if err := s.st.UpdateUser(id, dn, role, in.Disabled); err != nil {
		serverErr(w, err)
		return
	}
	if in.Teams != nil {
		_ = s.st.SetUserTeams(id, in.Teams)
	}
	revoke := in.Disabled && !u.Disabled || role != u.Role
	if in.Password != "" {
		if err := auth.CheckPasswordPolicy(in.Password); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			serverErr(w, err)
			return
		}
		_ = s.st.SetPassword(id, hash)
		revoke = true
	}
	if revoke {
		keep := ""
		if id == admin.ID {
			if sess := sessionFrom(r); sess != nil {
				keep = sess.ID
			}
		}
		_ = s.st.DeleteUserSessions(id, keep)
	}
	s.audit(r, admin, "user_updated", u.Username)
	u, _ = s.st.GetUser(id)
	writeJSON(w, 200, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, admin *store.User) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 400, "invalid id")
		return
	}
	if id == admin.ID {
		writeErr(w, 409, "you cannot delete your own account")
		return
	}
	u, err := s.st.GetUser(id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if u.Role == "admin" && !u.Disabled {
		if n, _ := s.st.CountActiveAdmins(); n <= 1 {
			writeErr(w, 409, "cannot delete the last administrator")
			return
		}
	}
	if err := s.st.DeleteUser(id); err != nil {
		serverErr(w, err)
		return
	}
	s.audit(r, admin, "user_deleted", u.Username)
	w.WriteHeader(204)
}

// ---------- settings / audit ----------

func (s *Server) settingsView() map[string]any {
	var lan []string
	for _, p := range s.cfg.LANCIDRs {
		lan = append(lan, p.String())
	}
	return map[string]any{
		"siteName":   s.st.GetSetting("siteName", "YdisksPanel"),
		"wallpaper":  s.st.GetSetting("wallpaper", ""),
		"require2fa": s.st.GetSetting("require2fa", "0") == "1",
		"lanCidrs":   lan,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	writeJSON(w, 200, s.settingsView())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		SiteName   string `json:"siteName"`
		Wallpaper  string `json:"wallpaper"`
		Require2FA bool   `json:"require2fa"`
	}
	if !decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.SiteName)
	if name == "" || utf8.RuneCountInString(name) > 40 {
		writeErr(w, 400, "site name must be 1-40 characters")
		return
	}
	if in.Wallpaper != "" && !validURL(in.Wallpaper) && !strings.HasPrefix(in.Wallpaper, "/icons/") {
		writeErr(w, 400, "invalid wallpaper URL")
		return
	}
	if in.Wallpaper != "" && strings.HasPrefix(in.Wallpaper, "/icons/") && !iconNameRe.MatchString(strings.TrimPrefix(in.Wallpaper, "/icons/")) {
		writeErr(w, 400, "invalid wallpaper")
		return
	}
	b := "0"
	if in.Require2FA {
		b = "1"
	}
	for k, v := range map[string]string{"siteName": name, "wallpaper": in.Wallpaper, "require2fa": b} {
		if err := s.st.SetSetting(k, v); err != nil {
			serverErr(w, err)
			return
		}
	}
	s.audit(r, u, "settings_changed", "")
	writeJSON(w, 200, s.settingsView())
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, u *store.User) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	list, err := s.st.ListAudit(limit, offset)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

// ---------- image upload ----------

const (
	iconUploadLimit      = 5 << 20 // raw bytes accepted; the stored result is far smaller
	wallpaperUploadLimit = 12 << 20
)

// handleUploadIcon accepts an image, normalises it (decode, downscale, re-encode)
// and stores the result in the database. kind=wallpaper allows a larger canvas.
func (s *Server) handleUploadIcon(w http.ResponseWriter, r *http.Request, u *store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, wallpaperUploadLimit+(1<<20))
	f, _, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, 413, fmt.Sprintf("file too large (max %d MB)", wallpaperUploadLimit>>20))
			return
		}
		writeErr(w, 400, "file required")
		return
	}
	defer f.Close()

	kind, limit := imgproc.Icon, int64(iconUploadLimit)
	if r.FormValue("kind") == "wallpaper" {
		kind, limit = imgproc.Wallpaper, wallpaperUploadLimit
	}
	var crop *imgproc.Crop
	if kind == imgproc.Icon && r.FormValue("cropSize") != "" {
		x, e1 := strconv.Atoi(r.FormValue("cropX"))
		y, e2 := strconv.Atoi(r.FormValue("cropY"))
		sz, e3 := strconv.Atoi(r.FormValue("cropSize"))
		if e1 != nil || e2 != nil || e3 != nil {
			writeErr(w, 400, "invalid crop area")
			return
		}
		crop = &imgproc.Crop{X: x, Y: y, Size: sz}
	}
	res, err := imgproc.Process(f, limit, kind, crop)
	switch {
	case errors.Is(err, imgproc.ErrFormat), errors.Is(err, imgproc.ErrDecode), errors.Is(err, imgproc.ErrCrop):
		writeErr(w, 400, err.Error())
		return
	case errors.Is(err, imgproc.ErrTooLarge):
		writeErr(w, 413, fmt.Sprintf("image too large (max %d MB, 10000 px per side)", limit>>20))
		return
	case err != nil:
		serverErr(w, err)
		return
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	if err := s.st.PutBlob(id, res.Ext, res.ContentType, res.Data); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"url": "/icons/" + id + "." + res.Ext, "width": res.Width, "height": res.Height, "bytes": len(res.Data)})
}
