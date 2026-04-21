package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/codercollo/lignin/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate up | down | version")
	}

	ctx := context.Background()
	logger := slog.Default()

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		log.Fatal("DATABASE_DSN is required")
	}

	switch os.Args[1] {
	case "up":
		if err := store.MigrateUp(ctx, dsn, logger); err != nil {
			log.Fatal(err)
		}

	case "down":
		if err := store.MigrateDown(ctx, dsn, 1, logger); err != nil {
			log.Fatal(err)
		}

	case "version":
		v, dirty, err := store.MigrateVersion(dsn)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("version=%d dirty=%v", v, dirty)

	default:
		log.Fatal("unknown command")
	}
}
