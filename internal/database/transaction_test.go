package database

import (
    "context"
    "strings"
    "testing"
)

func TestBeginTx_ReturnsValidTransaction(t *testing.T) {
    db := requirePostgres(t)

    tx, err := db.BeginTx(context.Background())
    if err != nil {
        t.Fatalf("unexpected error from BeginTx: %v", err)
    }
    defer tx.Rollback(context.Background()) //nolint:errcheck

    if tx == nil {
        t.Error("expected non-nil transaction")
    }
}

func TestBeginTx_CommitSucceeds(t *testing.T) {
    db := requirePostgres(t)

    tx, err := db.BeginTx(context.Background())
    if err != nil {
        t.Fatalf("BeginTx: %v", err)
    }
    defer tx.Rollback(context.Background()) //nolint:errcheck

    if err := tx.Commit(context.Background()); err != nil {
        t.Errorf("unexpected error on Commit: %v", err)
    }
}

func TestBeginTx_RollbackSucceeds(t *testing.T) {
    db := requirePostgres(t)

    tx, err := db.BeginTx(context.Background())
    if err != nil {
        t.Fatalf("BeginTx: %v", err)
    }

    if err := tx.Rollback(context.Background()); err != nil {
        t.Errorf("unexpected error on Rollback: %v", err)
    }
}

func TestBeginTx_RollbackAfterCommitIsHarmless(t *testing.T) {
    // pgx returns ErrTxClosed when rolling back an already-committed
    // transaction. This must not panic.
    db := requirePostgres(t)

    tx, err := db.BeginTx(context.Background())
    if err != nil {
        t.Fatalf("BeginTx: %v", err)
    }

    if err := tx.Commit(context.Background()); err != nil {
        t.Fatalf("Commit: %v", err)
    }

    _ = tx.Rollback(context.Background()) // ErrTxClosed expected; must not panic
}

func TestBeginTx_CancelledContextReturnsWrappedError(t *testing.T) {
    db := requirePostgres(t)

    ctx, cancel := context.WithCancel(context.Background())
    cancel()

    _, err := db.BeginTx(ctx)
    if err == nil {
        t.Fatal("expected error from BeginTx with cancelled context, got nil")
    }
    if !strings.Contains(err.Error(), "failed to begin transaction") {
        t.Errorf("expected wrapped error, got: %v", err)
    }
}