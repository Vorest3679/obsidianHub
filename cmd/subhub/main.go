package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"obsidianhub/internal/archive"
	"obsidianhub/internal/config"
	"obsidianhub/internal/fetcher"
	"obsidianhub/internal/service"
	"obsidianhub/internal/store"
)

func main() {
	configPath := flag.String("config", "./config.json", "path to config JSON")
	once := flag.Bool("once", false, "sync once and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	svc := &service.Service{
		Config:  cfg,
		Fetcher: fetcher.New(cfg.UserAgent),
		Store:   st,
		Writer:  archive.New(cfg.VaultPath),
		Logger:  log.New(os.Stdout, "obsidianhub ", log.LstdFlags),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sync := func() {
		if err := svc.SyncAll(ctx); err != nil {
			svc.Logger.Printf("sync completed with errors: %v", err)
		}
	}
	sync()
	if *once {
		return
	}

	ticker := time.NewTicker(time.Duration(cfg.IntervalSecs) * time.Second)
	defer ticker.Stop()
	svc.Logger.Printf("running; interval=%s, vault=%s", tickerDuration(cfg.IntervalSecs), cfg.VaultPath)
	for {
		select {
		case <-ctx.Done():
			svc.Logger.Println("stopped")
			return
		case <-ticker.C:
			sync()
		}
	}
}

func tickerDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
