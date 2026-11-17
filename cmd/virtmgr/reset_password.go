package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"virtmgr/internal/domain"
	authapp "virtmgr/internal/modules/auth/app"
	"virtmgr/internal/platform/config"
	"virtmgr/internal/platform/sqlite"
)

const (
	recoveryMinPasswordLength = 12
	recoveryMaxPasswordLength = 256
)

func runResetPassword(args []string) error {
	flags := flag.NewFlagSet("virtmgr reset-password", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := config.DefaultPath
	username := ""
	passwordFromStdin := false
	flags.StringVar(&configPath, "config", config.DefaultPath, "path to server.yaml")
	flags.StringVar(&configPath, "c", config.DefaultPath, "path to server.yaml")
	flags.StringVar(&username, "username", "", "local account username")
	flags.BoolVar(&passwordFromStdin, "password-stdin", false, "read the new password from standard input")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("--username is required")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	password, err := readRecoveryPassword(passwordFromStdin)
	if err != nil {
		return err
	}
	if !validRecoveryPassword(password) {
		return fmt.Errorf("password must contain %d to %d characters and must not include NUL", recoveryMinPasswordLength, recoveryMaxPasswordLength)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.Migrate(ctx, db); err != nil {
		return err
	}
	store := sqlite.NewStore(db)
	if err := resetLocalPassword(ctx, store, username, password); err != nil {
		return err
	}
	fmt.Printf("Password reset for %q; all sessions were revoked.\n", username)
	return nil
}

func readRecoveryPassword(fromStdin bool) (string, error) {
	if fromStdin {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(string(contents), "\r\n"), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("standard input is not a terminal; use --password-stdin")
	}
	fmt.Fprint(os.Stderr, "New password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read new password: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm new password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password confirmation: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("password confirmation does not match")
	}
	return string(first), nil
}

func validRecoveryPassword(password string) bool {
	length := utf8.RuneCountInString(password)
	return length >= recoveryMinPasswordLength && length <= recoveryMaxPasswordLength && !strings.ContainsRune(password, '\x00')
}

type localPasswordStore interface {
	UserByUsername(ctx context.Context, username string) (domain.User, error)
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	DeleteOtherSessionsByUser(ctx context.Context, userID int64, keepTokenHash string) error
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

func resetLocalPassword(ctx context.Context, store localPasswordStore, username, password string) error {
	user, err := store.UserByUsername(ctx, username)
	if err != nil {
		return err
	}
	passwordHash, err := authapp.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash user password: %w", err)
	}
	if err := store.UpdatePassword(ctx, user.ID, passwordHash); err != nil {
		return err
	}
	if err := store.DeleteOtherSessionsByUser(ctx, user.ID, ""); err != nil {
		return err
	}
	if err := store.RecordAudit(ctx, domain.AuditEvent{
		RequestID:  fmt.Sprintf("cli-reset-password-%d", time.Now().UTC().UnixNano()),
		Actor:      "local-cli",
		Role:       domain.RoleAdmin,
		Action:     "user.password.recovery",
		TargetKind: "user",
		TargetID:   strconv.FormatInt(user.ID, 10),
		SourceIP:   "local-cli",
		Result:     "success",
		Detail:     map[string]any{"username": user.Username, "sessionsRevoked": true},
	}); err != nil {
		return fmt.Errorf("record password recovery audit: %w", err)
	}
	return nil
}
