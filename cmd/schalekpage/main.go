// Command schalekpage serves the SchalekPage EduPage dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KuboHA/SchalekPage/internal/web"
	"golang.org/x/crypto/acme/autocert"
)

// version is the build version, stamped by the build script via
// -ldflags "-X main.version=...". It is "dev" for an unstamped `go build`.
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("schalekpage exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", ":5000", "address to listen on for plain HTTP (used when -domain is unset, or with -no-cert)")
	httpsAddr := flag.String("https-addr", ":443", "address to listen on for HTTPS when automatic TLS is enabled")
	domain := flag.String("domain", "", "public domain name to serve; enables automatic TLS (Let's Encrypt) unless -no-cert is set")
	noCert := flag.Bool("no-cert", false, "serve plain HTTP even when -domain is set, for deployments behind a reverse proxy (e.g. Caddy) that terminates TLS itself")
	certCacheDir := flag.String("cert-cache", "certs", "directory used to cache automatically obtained TLS certificates")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("schalekpage", version)
		return nil
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	templates, err := web.ParseTemplates()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	srv := web.NewServer(templates, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv.Sessions().StartReaper(ctx, 10*time.Minute)
	srv.LoginRateLimiter().StartReaper(ctx.Done(), 10*time.Minute)

	autoTLS := *domain != "" && !*noCert

	var servers []*http.Server
	if autoTLS {
		manager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(*domain),
			Cache:      autocert.DirCache(*certCacheDir),
		}

		httpsServer := &http.Server{
			Addr:              *httpsAddr,
			Handler:           srv.Routes(),
			ReadHeaderTimeout: 10 * time.Second,
			TLSConfig:         manager.TLSConfig(),
		}
		// The ACME HTTP-01 challenge is served over plain HTTP; everything
		// else on this port is redirected to HTTPS.
		httpServer := &http.Server{
			Addr:              *addr,
			Handler:           manager.HTTPHandler(nil),
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, httpsServer, httpServer)
	} else {
		if *domain != "" {
			logger.Info("automatic TLS disabled (-no-cert); serving plain HTTP", "domain", *domain)
		}
		servers = append(servers, &http.Server{
			Addr:              *addr,
			Handler:           srv.Routes(),
			ReadHeaderTimeout: 10 * time.Second,
		})
	}

	serveErr := make(chan error, len(servers))
	for _, s := range servers {
		s := s
		go func() {
			if autoTLS && s.TLSConfig != nil {
				logger.Info("listening", "addr", s.Addr, "tls", true, "domain", *domain)
				serveErr <- s.ListenAndServeTLS("", "")
				return
			}
			logger.Info("listening", "addr", s.Addr, "tls", false)
			serveErr <- s.ListenAndServe()
		}()
	}

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, s := range servers {
			if err := s.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("graceful shutdown: %w", err)
			}
		}
		return nil
	}
}
