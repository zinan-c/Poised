package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	URL      string
	MaxConns int32
}

type DB struct {
	pool *pgxpool.Pool
}

type CheckResult struct {
	Connected     bool     `json:"connected"`
	Initialized   bool     `json:"initialized"`
	MissingTables []string `json:"missing_tables"`
}

func Open(ctx context.Context, config Config) (*DB, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("database url is required")
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if config.MaxConns > 0 {
		poolConfig.MaxConns = config.MaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}

	return &DB{pool: pool}, nil
}

func (db *DB) Close() {
	if db == nil || db.pool == nil {
		return
	}
	db.pool.Close()
}

func (db *DB) Pool() *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.pool
}

func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return fmt.Errorf("database is not open")
	}
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (db *DB) Initialize(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return fmt.Errorf("database is not open")
	}
	if _, err := db.pool.Exec(ctx, SchemaSQL); err != nil {
		return fmt.Errorf("initialize database schema: %w", err)
	}
	return nil
}

func (db *DB) Check(ctx context.Context) (CheckResult, error) {
	result := CheckResult{}
	if err := db.Ping(ctx); err != nil {
		return result, err
	}

	result.Connected = true
	for _, table := range RequiredTables {
		exists, err := db.tableExists(ctx, table)
		if err != nil {
			return result, err
		}
		if !exists {
			result.MissingTables = append(result.MissingTables, table)
		}
	}

	result.Initialized = len(result.MissingTables) == 0
	return result, nil
}

func (db *DB) tableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	if err := db.pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil {
		return false, fmt.Errorf("check table %q: %w", table, err)
	}
	return exists, nil
}
