package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/term"
	"log/slog"
	"mod-list-discovery/server/internal/app"
	"mod-list-discovery/server/internal/auth"
	"mod-list-discovery/server/internal/config"
	"mod-list-discovery/server/internal/database"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func password() (string, error) {
	if s := os.Getenv("MLD_ADMIN_PASSWORD"); s != "" {
		return s, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("set MLD_ADMIN_PASSWORD or use an interactive terminal")
	}
	fmt.Fprint(os.Stderr, "Admin password: ")
	b, e := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), e
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: init | serve | version | admin set-password [--config path]")
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	if cmd == "version" {
		fmt.Println(app.Version)
		return nil
	}
	if cmd == "admin" {
		if len(args) == 0 || args[0] != "set-password" {
			return fmt.Errorf("expected admin set-password")
		}
		cmd = "set-password"
		args = args[1:]
	}
	flags := flag.NewFlagSet(cmd, flag.ContinueOnError)
	file := flags.String("config", "./data/config.yaml", "YAML configuration file")
	if e := flags.Parse(args); e != nil {
		return e
	}
	c, e := config.Load(*file)
	if e != nil {
		return e
	}
	switch cmd {
	case "init":
		p, e := password()
		if e != nil {
			return e
		}
		return app.Init(c, p)
	case "set-password":
		p, e := password()
		if e != nil {
			return e
		}
		hash, e := auth.Password(p)
		if e != nil {
			return e
		}
		if _, e = os.Stat(c.Database.Path); e != nil {
			return e
		}
		db, e := database.Open(c.Database.Path)
		if e != nil {
			return e
		}
		defer db.Close()
		tx, e := db.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		res, e := tx.Exec("UPDATE admins SET password_hash=?,updated_at=? WHERE id=1", hash, app.Now())
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return fmt.Errorf("administrator not initialized")
		}
		if _, e = tx.Exec("DELETE FROM admin_sessions"); e != nil {
			return e
		}
		if _, e = tx.Exec("INSERT INTO audit_logs(actor_type,action,created_at) VALUES('local','admin.password.change',?)", app.Now()); e != nil {
			return e
		}
		return tx.Commit()
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a, e := app.Open(ctx, c)
		if e != nil {
			return e
		}
		defer a.DB.Close()
		srv := &http.Server{Addr: c.Server.Listen, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 6 * time.Minute, WriteTimeout: 6 * time.Minute, IdleTimeout: 60 * time.Second}
		done := make(chan error, 1)
		go func() {
			slog.Info("listening", "address", c.Server.Listen, "version", app.Version)
			done <- srv.ListenAndServe()
		}()
		select {
		case e := <-done:
			if e != http.ErrServerClosed {
				return e
			}
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return srv.Shutdown(shutdown)
		}
		return nil
	default:
		return fmt.Errorf("unknown command")
	}
}
func main() {
	if e := run(); e != nil {
		slog.Error("command failed", "error", e)
		os.Exit(1)
	}
}
