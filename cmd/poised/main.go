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
	"github.com/zinan-c/Poised/internal/adapters/airfare/ceair"
	"github.com/zinan-c/Poised/internal/adapters/airfare/csair"
	"github.com/zinan-c/Poised/internal/adapters/airfare/springair"
	"github.com/zinan-c/Poised/internal/adapters/examples/echo"
	"github.com/zinan-c/Poised/internal/adapters/httpcheck"
	"github.com/zinan-c/Poised/internal/api"
	"github.com/zinan-c/Poised/internal/config"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/database"
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
	mustRegister(logger, registry, ceair.New())
	mustRegister(logger, registry, csair.New())
	mustRegister(logger, registry, springair.New())
	ensureConfiguredAdapters(logger, registry, appConfig.Jobs)

	databaseInstance := openDatabase(logger, appConfig.Database)
	defer databaseInstance.Close()

	runStore := newRunStore(logger, databaseInstance, appConfig.Jobs)
	jobRunner := runner.New(registry, runStore, logger)
	jobScheduler := scheduler.New(runStore, jobRunner, logger, appConfig.Scheduler.RunOnStart)
	apiServer := api.NewServer(registry, jobRunner, runStore, databaseInstance, logger)

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobScheduler.Start(rootContext)

	httpServer := &http.Server{
		Addr:              appConfig.HTTP.Addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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

func ensureConfiguredAdapters(logger *slog.Logger, registry *adapters.Registry, jobs []core.JobSpec) {
	for _, job := range jobs {
		if _, exists := registry.Get(job.Adapter); !exists {
			logger.Error("job references unregistered adapter", "job_id", job.ID, "adapter", job.Adapter)
			os.Exit(1)
		}
	}
}

func openDatabase(logger *slog.Logger, config config.DatabaseConfig) *database.DB {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, database.Config{
		URL:      config.URL,
		MaxConns: config.MaxConns,
	})
	if err != nil {
		logger.Error("open database failed", "error", err)
		os.Exit(1)
	}

	if config.AutoMigrate {
		if err := db.Initialize(ctx); err != nil {
			logger.Error("initialize database failed", "error", err)
			os.Exit(1)
		}
	}

	checkResult, err := db.Check(ctx)
	if err != nil {
		logger.Error("check database failed", "error", err)
		os.Exit(1)
	}
	if !checkResult.Initialized {
		logger.Error("database schema is incomplete", "missing_tables", checkResult.MissingTables)
		os.Exit(1)
	}

	logger.Info("database ready", "max_conns", config.MaxConns, "auto_migrate", config.AutoMigrate)
	return db
}

func newRunStore(logger *slog.Logger, db *database.DB, jobs []core.JobSpec) *store.PostgresStore {
	postgresStore := store.NewPostgresStore(db.Pool())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, job := range jobs {
		if _, err := postgresStore.UpsertTask(ctx, job); err != nil {
			logger.Error("sync monitor task failed", "job_id", job.ID, "error", err)
			os.Exit(1)
		}
	}

	logger.Info("monitor tasks synced", "count", len(jobs))
	return postgresStore
}
