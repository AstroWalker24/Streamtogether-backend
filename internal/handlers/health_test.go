package handlers

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthHandler_Live_StatusOK(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
    w := httptest.NewRecorder()

    h.Live(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}

func TestHealthHandler_Live_ContentType(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
    w := httptest.NewRecorder()

    h.Live(w, req)

    ct := w.Header().Get("Content-Type")
    if ct != "application/json" {
        t.Errorf("expected Content-Type application/json, got %s", ct)
    }
}

func TestHealthHandler_Live_Body(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
    w := httptest.NewRecorder()

    h.Live(w, req)

    var body map[string]string
    if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
        t.Fatalf("response body is not valid JSON: %v", err)
    }
    if body["status"] != "ok" {
        t.Errorf("expected status=ok, got %s", body["status"])
    }
}

func TestHealthHandler_Ready_StatusOK(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
    w := httptest.NewRecorder()

    h.Ready(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}

func TestHealthHandler_Ready_Body(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
    w := httptest.NewRecorder()

    h.Ready(w, req)

    var body map[string]string
    if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
        t.Fatalf("response body is not valid JSON: %v", err)
    }
    if body["status"] != "ready" {
        t.Errorf("expected status=ready, got %s", body["status"])
    }
}

// Edge case: wrong method — Go 1.22+ ServeMux returns 405 automatically,
// but the handler itself does not enforce method. Test that it still responds
// when called directly (bypassing the mux).
func TestHealthHandler_Live_PostMethodStillResponds(t *testing.T) {
    h := NewHealthHandler()
    req := httptest.NewRequest(http.MethodPost, "/health/live", nil)
    w := httptest.NewRecorder()

    h.Live(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200 from direct handler call regardless of method, got %d", w.Code)
    }
}