package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ydiskspanel/internal/store"
)

func pngBytes(w, h int) []byte {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y += 7 {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.NRGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}

func (c *client) upload(name string, data []byte, fields map[string]string) (int, map[string]any) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	r := httptest.NewRequest("POST", "/api/uploads/icon", &body)
	r.RemoteAddr = c.remote
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-Requested-With", "ydiskspanel")
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func get(h http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestImageUploadFlow(t *testing.T) {
	s, h := newSrv(t)
	adm := &client{t, h, nil, "192.168.1.2:1"}
	adm.do("POST", "/api/setup", map[string]string{"token": s.setupToken, "username": "admin", "password": "supersecret1"}, nil)

	// unauthenticated and non-admin are refused
	if code, _ := (&client{t, h, nil, "9.9.9.9:1"}).upload("a.png", pngBytes(10, 10), nil); code != 401 {
		t.Fatalf("anon upload: %d", code)
	}

	// oversized image is centre-cropped to a square, downscaled to <=256 and stored in the DB
	code, v := adm.upload("../../evil.png", pngBytes(1000, 600), nil)
	if code != 200 {
		t.Fatalf("upload: %d %v", code, v)
	}
	url := v["url"].(string)
	if !strings.HasPrefix(url, "/icons/") || !strings.HasSuffix(url, ".png") || v["width"].(float64) != 256 || v["height"].(float64) != 256 {
		t.Fatalf("unexpected result: %v", v)
	}

	// served publicly (login page needs it), as a real <=256px PNG, immutable + ETag
	w := get(h, url, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("ETag") == "" ||
		!strings.Contains(w.Header().Get("Cache-Control"), "immutable") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("serve: %d %v", w.Code, w.Header())
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || cfg.Width > 256 || cfg.Height > 256 {
		t.Fatalf("stored image: %v %+v", err, cfg)
	}
	if w := get(h, url, map[string]string{"If-None-Match": w.Header().Get("ETag")}); w.Code != 304 {
		t.Fatalf("etag: %d", w.Code)
	}
	if w := get(h, strings.TrimSuffix(url, ".png")+".jpg", nil); w.Code != 404 {
		t.Fatalf("wrong extension must 404: %d", w.Code)
	}
	if w := get(h, "/icons/"+strings.Repeat("0", 32)+".png", nil); w.Code != 404 {
		t.Fatalf("unknown id: %d", w.Code)
	}
	// nothing is written to disk any more
	if _, err := os.Stat(filepath.Join(s.cfg.DataDir, "icons")); !os.IsNotExist(err) {
		t.Fatalf("icons dir should not exist: %v", err)
	}

	// explicit crop from the UI; bad crops are refused
	if code, v := adm.upload("c.png", pngBytes(400, 200), map[string]string{"cropX": "0", "cropY": "0", "cropSize": "200"}); code != 200 || v["width"].(float64) != 200 || v["height"].(float64) != 200 {
		t.Fatalf("explicit crop: %d %v", code, v)
	}
	if code, _ := adm.upload("c.png", pngBytes(400, 200), map[string]string{"cropX": "0", "cropY": "0", "cropSize": "3"}); code != 400 {
		t.Fatalf("tiny crop must be rejected: %d", code)
	}
	if code, _ := adm.upload("c.png", pngBytes(400, 200), map[string]string{"cropX": "x", "cropY": "0", "cropSize": "100"}); code != 400 {
		t.Fatalf("non-numeric crop must be rejected: %d", code)
	}

	// a body beyond the hard cap gets a clear 413, not a vague "file required"
	if code, v := adm.upload("huge.png", append(pngBytes(10, 10), make([]byte, 14<<20)...), nil); code != 413 {
		t.Fatalf("oversized body: %d %v", code, v)
	}

	// rejects svg/html and non-images
	for name, data := range map[string]string{"x.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`, "x.png": `<html>hi</html>`} {
		if code, _ := adm.upload(name, []byte(data), nil); code != 400 {
			t.Fatalf("%s should be rejected: %d", name, code)
		}
	}

	// wallpaper gets the larger canvas and becomes a JPEG
	code, wv := adm.upload("bg.png", pngBytes(2400, 1200), map[string]string{"kind": "wallpaper"})
	if code != 200 || wv["width"].(float64) != 1920 || !strings.HasSuffix(wv["url"].(string), ".jpg") {
		t.Fatalf("wallpaper: %d %v", code, wv)
	}

	// a service can reference the stored icon; unreferenced blobs are purged, referenced ones kept
	_, g := adm.do("POST", "/api/groups", map[string]string{"name": "G"}, nil)
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": g["id"], "name": "S", "wanUrl": "https://a.example", "icon": url}, nil); code != 200 {
		t.Fatalf("service with stored icon: %d", code)
	}
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": g["id"], "name": "S2", "wanUrl": "https://a.example", "icon": "/icons/../../etc/passwd"}, nil); code != 400 {
		t.Fatalf("path traversal icon must be rejected: %d", code)
	}
	n, err := s.st.PurgeOrphanBlobs(-time.Second)
	if err != nil || n < 1 { // the unreferenced wallpaper (and other test uploads) go
		t.Fatalf("purge: %d %v", n, err)
	}
	if _, err := s.st.GetBlob(strings.TrimSuffix(strings.TrimPrefix(url, "/icons/"), ".png")); err != nil {
		t.Fatalf("referenced icon must survive: %v", err)
	}
	if _, err := s.st.GetBlob(strings.TrimSuffix(strings.TrimPrefix(wv["url"].(string), "/icons/"), ".jpg")); err != store.ErrNotFound {
		t.Fatalf("orphan must be gone: %v", err)
	}

	// iconFit: defaults to contain, accepts cover, rejects anything else, and persists
	_, sv := adm.do("POST", "/api/services", map[string]any{"groupId": g["id"], "name": "F", "wanUrl": "https://a.example"}, nil)
	if sv["iconFit"] != "contain" {
		t.Fatalf("default iconFit: %v", sv["iconFit"])
	}
	if code, v := adm.do("PUT", "/api/services/"+itoa(sv["id"].(float64)), map[string]any{"groupId": g["id"], "name": "F", "wanUrl": "https://a.example", "iconFit": "cover"}, nil); code != 200 || v["iconFit"] != "cover" {
		t.Fatalf("cover: %d %v", code, v)
	}
	if code, _ := adm.do("POST", "/api/services", map[string]any{"groupId": g["id"], "name": "F2", "wanUrl": "https://a.example", "iconFit": "stretch"}, nil); code != 400 {
		t.Fatalf("bad iconFit: %d", code)
	}

	// grace period protects fresh uploads
	_, fresh := adm.upload("n.png", pngBytes(50, 50), nil)
	if n, _ := s.st.PurgeOrphanBlobs(time.Hour); n != 0 {
		t.Fatalf("fresh upload purged: %v %d", fresh, n)
	}
}
