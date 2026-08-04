package database

import (
    "context"
    "strings"
    "testing"
)

func TestHealth_ReturnsNilWhenConnected(t *testing.T) {
    db := requirePostgres(t)

    if err := db.Health(context.Background()); err != nil {
        t.Errorf("expected nil from Health on live connection, got: %v", err)
    }
}

func TestHealth_CancelledContextReturnsDescriptiveError(t *testing.T) {
    db := requirePostgres(t)

    ctx, cancel := context.WithCancel(context.Background())
    cancel()

    err := db.Health(ctx)
    if err == nil {
        t.Fatal("expected error with cancelled context, got nil")
    }
    if !strings.Contains(err.Error(), "health check failed") {
        t.Errorf("expected wrapped error message, got: %v", err)
    }
}

func TestHealth_ClosedPoolReturnsError(t *testing.T) {
    db := requirePostgres(t)
    db.Close()

    err := db.Health(context.Background())
    if err == nil {
        t.Error("expected error from Health after Close(), got nil")
    }
    if !strings.Contains(err.Error(), "health check failed") {
        t.Errorf("expected wrapped error message, got: %v", err)
    }
}

func TestHealth_TimeoutContextReturnsError(t *testing.T) {
    db := requirePostgres(t)

    // A 1ns timeout will expire before any ping can complete.
    ctx, cancel := context.WithTimeout(context.Background(), 1)
    defer cancel()

    err := db.Health(ctx)
    if err == nil {
        t.Error("expected error with expired timeout context, got nil")
    }
}