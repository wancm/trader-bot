package repository

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wancm/trader-bot/app_shared"
)

var PG_Pool *pgxpool.Pool

func Init(ctx context.Context) {
	var err error
	PG_Pool, err = NewPgPool(ctx)
	if err != nil {
		app_shared.AppLogger.Error(err.Error())
		os.Exit(1)
	}
}

func NewPgPool(ctx context.Context) (*pgxpool.Pool, error) {
	pgConnString := os.Getenv("DB_CONN") // 例如 "postgres://user:pass@localhost/traderdb?sslmode=disable"
	if pgConnString == "" {
		app_shared.AppLogger.Error("DB_CONN environment variable not set")
		return nil, fmt.Errorf("missing DB_CONN environment variable")
	}

	pool, err := pgxpool.New(ctx, pgConnString)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return pool, nil
}
