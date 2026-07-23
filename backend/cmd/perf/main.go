// Command perf generates isolated performance fixtures and benchmarks P0 paths.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type config struct {
	Profile    string
	Pages      int
	Revisions  int
	Workers    int
	Iterations int
	Output     string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "perf:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("perf", flag.ContinueOnError)
	cfg := config{}
	fs.StringVar(&cfg.Profile, "profile", "smoke", "smoke or full")
	fs.IntVar(&cfg.Pages, "pages", 0, "override page count")
	fs.IntVar(&cfg.Revisions, "revisions", 0, "revisions per page")
	fs.IntVar(&cfg.Workers, "workers", runtime.NumCPU(), "seed concurrency")
	fs.IntVar(&cfg.Iterations, "iterations", 0, "requests per benchmark")
	fs.StringVar(&cfg.Output, "output", "", "JSON report path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := applyProfile(&cfg); err != nil {
		return err
	}
	databaseURL := os.Getenv("PERF_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("PERF_DATABASE_URL is required; DATABASE_URL is intentionally ignored")
	}
	if os.Getenv("PERF_DATABASE_CONFIRM") != confirmValue {
		return fmt.Errorf("PERF_DATABASE_CONFIRM must equal %q", confirmValue)
	}
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	databaseName, err := verifyPerformanceDatabase(ctx, pool)
	if err != nil {
		return err
	}

	started := time.Now()
	fixtures, err := seed(ctx, pool, cfg)
	if err != nil {
		return err
	}
	m9Fixtures, err := seedM9(ctx, pool, fixtures)
	if err != nil {
		return err
	}
	seedDuration := time.Since(started)
	result, err := benchmark(ctx, pool, cfg, fixtures, m9Fixtures)
	if err != nil {
		return err
	}
	result.GeneratedAt = time.Now().UTC()
	result.Profile = cfg.Profile
	result.Database = databaseName
	result.Pages = len(fixtures)
	result.SeedSeconds = seedDuration.Seconds()
	result.Machine = machineInfo()
	if err := collectDatabaseStats(ctx, pool, &result); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if cfg.Output != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Output), 0o755); err != nil {
			return err
		}
		return os.WriteFile(cfg.Output, append(encoded, '\n'), 0o644)
	}
	return nil
}

func applyProfile(cfg *config) error {
	switch cfg.Profile {
	case "smoke":
		setDefaults(cfg, 100, 2, 30)
	case "full":
		setDefaults(cfg, 100000, 2, 1000)
	default:
		return fmt.Errorf("unknown profile %q", cfg.Profile)
	}
	if cfg.Pages < 1 || cfg.Revisions < 1 || cfg.Workers < 1 || cfg.Iterations < 1 {
		return errors.New("pages, revisions, workers and iterations must be positive")
	}
	return nil
}

func setDefaults(cfg *config, pages, revisions, iterations int) {
	if cfg.Pages == 0 {
		cfg.Pages = pages
	}
	if cfg.Revisions == 0 {
		cfg.Revisions = revisions
	}
	if cfg.Iterations == 0 {
		cfg.Iterations = iterations
	}
}
