package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/uptrace/bun/migrate"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/config"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/database"
	"github.com/trencetech/bayse-orderbook-snapshot/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to load configuration: %w", err))
	}

	db := database.New(cfg.DSN())
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(fmt.Errorf("failed to connect to database: %w", err))
	}

	migrator := migrate.NewMigrator(db, migrations.Migrations, migrate.WithMarkAppliedOnSuccess(true))

	ctx := context.Background()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		if err := migrator.Init(ctx); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Migration table created successfully")

	case "migrate", "up":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatal(err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Migrate(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if group.IsZero() {
			fmt.Println("No new migrations to run")
		} else {
			fmt.Printf("Migrated to %s\n", group)
		}

	case "rollback", "down":
		if err := migrator.Lock(ctx); err != nil {
			log.Fatal(err)
		}
		defer migrator.Unlock(ctx)

		group, err := migrator.Rollback(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if group.IsZero() {
			fmt.Println("No migrations to rollback")
		} else {
			fmt.Printf("Rolled back %s\n", group)
		}

	case "status":
		ms, err := migrator.MigrationsWithStatus(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Migrations: %s\n", ms)
		fmt.Printf("Unapplied migrations: %s\n", ms.Unapplied())
		fmt.Printf("Last migration group: %s\n", ms.LastGroup())

	case "create":
		if len(os.Args) < 3 {
			fmt.Println("Error: migration name is required")
			fmt.Println("Usage: migrate create <name>")
			os.Exit(1)
		}

		name := strings.Join(os.Args[2:], "_")
		files, err := migrator.CreateTxSQLMigrations(ctx, name)
		if err != nil {
			log.Fatal(err)
		}
		for _, file := range files {
			fmt.Printf("Created migration file: %s\n", file.Path)
		}

	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Migration Tool")
	fmt.Println("\nUsage:")
	fmt.Println("  migrate <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  init              Create migration table")
	fmt.Println("  migrate, up       Run all pending migrations")
	fmt.Println("  rollback, down    Rollback the last migration group")
	fmt.Println("  status            Show migration status")
	fmt.Println("  create <name>     Create a new migration")
}
