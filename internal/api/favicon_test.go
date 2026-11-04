package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

var png1x1 = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\x00IEND\xaeB`\x82")

func TestIconLinksPicksLargest(t *testing.T) {
	page := []byte(`<html><head>
	<link rel="icon" href="/small.png" sizes="16x16">
	<link rel="shortcut icon" href="fav.ico">
	<link rel="apple-touch-icon" href="/touch.png" sizes="180x180">
	<link rel="mask-icon" href="/mask.svg">
	<link rel="stylesheet" href="/x.css"></head></html>`)
	base, _ := url.Parse("http://nas.local/app/")
	got := iconLinks(page, base)
	if len(got) != 3 || got[0] != "http://nas.local/touch.png" || got[2] != "http://nas.local/small.png" && got[1] != "http://nas.local/small.png" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestFetchFavicon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<link rel="icon" href="/logo.png"><link rel="icon" href="http://169.254.169.254/evil.png" sizes="512x512">`))
	})
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) { w.Write(png1x1) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if d := fetchFavicon(context.Background(), srv.URL); sniffImage(d) != "image/png" {
		t.Fatalf("expected png via same-host link, got %q", sniffImage(d))
	}

	// Falls back to /favicon.ico; rejects non-image content.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>not an image</html>")) })
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	if d := fetchFavicon(context.Background(), srv2.URL); d != nil {
		t.Fatal("html must not be accepted as an icon")
	}
	if fetchFavicon(context.Background(), "javascript:alert(1)") != nil || fetchFavicon(context.Background(), "ftp://x") != nil {
		t.Fatal("non-http scheme must be refused")
	}
}

func TestSVGDetected(t *testing.T) {
	if sniffImage([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`)) != "image/svg+xml" {
		t.Fatal("svg")
	}
}
