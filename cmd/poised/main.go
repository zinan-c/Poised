package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/adapters/echo"
	"github.com/zinan-c/Poised/internal/adapters/httpcheck"
	"github.com/zinan-c/Poised/internal/api"
	"github.com/zinan-c/Poised/internal/config"
	"github.com/zinan-c/Poised/internal/runner"
	"github.com/zinan-c/Poised/internal/scheduler"
	"github.com/zinan-c/Poised/internal/store"
)

func main() {
	configPath := flag.String("config", "configs/poised.example.json", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	appConfig, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	registry := adapters.NewRegistry()
	mustRegister(logger, registry, echo.New())
	mustRegister(logger, registry, httpcheck.New())

	runStore := store.NewMemoryRunStore()
	jobRunner := runner.New(registry, runStore, logger)
	jobScheduler := scheduler.New(appConfig.Jobs, jobRunner, logger, appConfig.Scheduler.RunOnStart)
	apiServer := api.NewServer(appConfig.Jobs, registry, jobRunner, runStore, logger)

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobScheduler.Start(rootContext)

	httpServer := &http.Server{
		Addr:              appConfig.HTTP.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("poised api listening", "addr", appConfig.HTTP.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-rootContext.Done()
	logger.Info("shutdown requested")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("http shutdown failed", "error", err)
	}
	jobScheduler.Wait()
	logger.Info("poised stopped")
}

func mustRegister(logger *slog.Logger, registry *adapters.Registry, adapter adapters.Adapter) {
	if err := registry.Register(adapter); err != nil {
		logger.Error("register adapter failed", "adapter", adapter.Name(), "error", err)
		os.Exit(1)
	}
}
