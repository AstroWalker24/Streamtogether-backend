package database

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5"
)


func (db *Database) BeginTx(ctx context.Context) (pgx.Tx, error) {
    tx, err := db.pool.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }
    return tx, nil
}