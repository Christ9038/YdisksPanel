package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ydiskspanel/internal/api"
	"ydiskspanel/internal/auth"
	"ydiskspanel/internal/config"
	"ydiskspanel/internal/health"
	"ydiskspanel/internal/store"
	"ydiskspanel/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	box, err := auth.OpenBox(cfg.DataDir)
	if err != nil {
		log.Fatalf("secret key: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hc := health.New(st.ListServices, cfg.HealthInterval)
	go hc.Run(ctx)

	srv, err := api.New(cfg, st, box, hc, web.FS())
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	go srv.Maintain(ctx)

	hs := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = hs.Shutdown(sctx)
	}()
	log.Printf("YdisksPanel listening on %s", cfg.Listen)
	if cfg.TLSCert != "" {
		err = hs.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		err = hs.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
