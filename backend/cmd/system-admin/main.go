// Command system-admin explicitly grants/revokes operator authority without
// granting access to private Wiki content or resetting account credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/config"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func main() {
	username := flag.String("username", "", "Existing local account username")
	revoke := flag.Bool("revoke", false, "Revoke system administrator authority")
	flag.Parse()
	if *username == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: system-admin -username USER [-revoke]")
		os.Exit(2)
	}
	if err := run(*username, !*revoke); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("System administrator authority updated; audit recorded.")
}

func run(username string, grant bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	service := auth.NewService(pool, db.NewTxManager(pool), id.NewGenerator(), cfg.SessionTTL)
	return service.SetSystemAdminByUsername(ctx, username, grant)
}
