package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"url-shortener/internal/auth"
	"url-shortener/internal/clicklog"
	"url-shortener/internal/shortener"
	"url-shortener/internal/store"
	"url-shortener/internal/web"
)

var stdinReader = bufio.NewReader(os.Stdin)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "migrate":
		os.Exit(cmdMigrate(args))
	case "set-password":
		os.Exit(cmdSetPassword(args))
	case "serve":
		os.Exit(cmdServe(args))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: url-shortener <serve|set-password|migrate> [flags]")
}

func openStore(dbPath string) (*store.Store, error) {
	if dbPath == "" {
		return nil, errors.New("--db is required")
	}
	return store.Open(dbPath)
}

func cmdMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := fs.String("db", "url-shortener.db", "path to SQLite database")
	_ = fs.Parse(args)

	s, err := openStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}
	defer s.Close()
	fmt.Println("migrations applied")
	return 0
}

func cmdSetPassword(args []string) int {
	fs := flag.NewFlagSet("set-password", flag.ExitOnError)
	dbPath := fs.String("db", "url-shortener.db", "path to SQLite database")
	_ = fs.Parse(args)

	s, err := openStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "set-password:", err)
		return 1
	}
	defer s.Close()

	pw, err := readPassword("New admin password: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	confirm, err := readPassword("Confirm: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if pw != confirm {
		fmt.Fprintln(os.Stderr, "passwords do not match")
		return 1
	}
	if len(pw) < 8 {
		fmt.Fprintln(os.Stderr, "password too short (min 8)")
		return 1
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := s.SetAdminPasswordHash(context.Background(), hash); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("admin password set")
	return 0
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return string(b), err
	}
	line, err := stdinReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line, nil
}

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	dbPath := fs.String("db", "url-shortener.db", "path to SQLite database")
	secureCookies := fs.Bool("secure-cookies", false, "set Secure flag on session cookie")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	s, err := openStore(*dbPath)
	if err != nil {
		slog.Error("open store", "err", err)
		return 1
	}
	defer s.Close()

	if _, err := s.GetAdminPasswordHash(context.Background()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.Error("no admin password set; run `url-shortener set-password` first")
			return 1
		}
		slog.Error("read admin password", "err", err)
		return 1
	}

	sessions := auth.NewSessionManager(s, auth.SessionConfig{
		CookieName:    "urlshortener_session",
		TTL:           7 * 24 * time.Hour,
		SecureCookies: *secureCookies,
		LoginPath:     "/admin/login",
	})
	clicks := clicklog.New(s, clicklog.DefaultConfig())
	clicks.Start()

	stopCleanup := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := s.DeleteExpiredSessions(ctx); err != nil {
					slog.Warn("delete expired sessions", "err", err)
				}
				cancel()
			case <-stopCleanup:
				return
			}
		}
	}()

	svr := web.NewServer(web.Deps{
		Store:     s,
		Shortener: shortener.New(s),
		Sessions:  sessions,
		Clicks:    clicks,
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           svr.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", *addr)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		clicks.Shutdown(shutdownCtx)
		close(stopCleanup)
	}

	select {
	case err := <-errCh:
		shutdown()
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			return 1
		}
	case sig := <-sigCh:
		slog.Info("shutting down", "signal", sig.String())
		shutdown()
	}
	return 0
}
