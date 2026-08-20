package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/pavlomaksymov/bank-account-api/internal/api"
	"github.com/pavlomaksymov/bank-account-api/internal/bank"
	"github.com/pavlomaksymov/bank-account-api/internal/config"
	"github.com/pavlomaksymov/bank-account-api/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	gdb, err := gorm.Open(gormpg.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	if err := migrations.Up(sqlDB); err != nil {
		return err
	}

	svc := bank.NewAccountService(bank.NewRepository(gdb))
	router := api.NewRouter(api.NewHandler(svc))
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	_ = sqlDB.Close()
	return nil
}
