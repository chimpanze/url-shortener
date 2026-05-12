package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/store"
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
	fmt.Fprintln(os.Stderr, "usage: ffsbz <serve|set-password|migrate> [flags]")
}

func openStore(dbPath string) (*store.Store, error) {
	if dbPath == "" {
		return nil, errors.New("--db is required")
	}
	return store.Open(dbPath)
}

func cmdMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := fs.String("db", "ffsbz.db", "path to SQLite database")
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
	dbPath := fs.String("db", "ffsbz.db", "path to SQLite database")
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
	// Stub - implemented in next task.
	fmt.Fprintln(os.Stderr, "serve not implemented yet")
	return 1
}
