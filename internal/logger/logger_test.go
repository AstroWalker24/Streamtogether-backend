package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// captureLogger returns a Logger that writes JSON to buf.
func captureLogger(buf *bytes.Buffer, opts ...Option) Logger {
	return New(append([]Option{WithWriter(buf)}, opts...)...)
}

// decodeJSON unmarshals the last non-empty line from buf.
func decodeJSON(t *testing.T, buf *bytes.Buffer) map[string]interface{} {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("failed to decode JSON: %v\nraw: %s", err, buf.String())
	}
	return m
}

// ── Output format ─────────────────────────────────────────────────────────────

func TestNew_DefaultOutputIsJSON(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)
	log.Info("hello")

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
}

func TestNew_PrettyOutputIsNotJSON(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf, WithPretty(true))
	log.Info("hello pretty")

	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Error("expected non-JSON pretty output, but got valid JSON")
	}
}

// ── Log levels ────────────────────────────────────────────────────────────────

func TestLevel_DebugNotWrittenAtInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf, WithLevel("info"))
	log.Debug("this should not appear")

	if buf.Len() > 0 {
		t.Errorf("expected no output at info level for debug message, got: %s", buf.String())
	}
}

func TestLevel_WarnWrittenAtWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf, WithLevel("warn"))
	log.Warn("warn message")

	if buf.Len() == 0 {
		t.Error("expected warn to be written at warn level, got no output")
	}
}

func TestLevel_DebugWrittenAtDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf, WithLevel("debug"))
	log.Debug("debug message")

	if buf.Len() == 0 {
		t.Error("expected debug output, got none")
	}
}

func TestLevel_AllLevelsPresent(t *testing.T) {
	cases := []struct {
		method func(Logger)
		level  string
	}{
		{func(l Logger) { l.Debug("m") }, "debug"},
		{func(l Logger) { l.Info("m") }, "info"},
		{func(l Logger) { l.Warn("m") }, "warn"},
		{func(l Logger) { l.Error("m") }, "error"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		log := captureLogger(&buf, WithLevel("debug"))
		tc.method(log)

		m := decodeJSON(t, &buf)
		if m["level"] != tc.level {
			t.Errorf("expected level=%s, got %v", tc.level, m["level"])
		}
	}
}

// ── Message ───────────────────────────────────────────────────────────────────

func TestInfo_MessageField(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("hello world")

	m := decodeJSON(t, &buf)
	if m["message"] != "hello world" {
		t.Errorf("expected message=hello world, got %v", m["message"])
	}
}

// ── Field constructors ────────────────────────────────────────────────────────

func TestFields_String(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", String("key", "value"))
	assertField(t, &buf, "key", "value")
}

func TestFields_Int(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", Int("port", 8080))
	m := decodeJSON(t, &buf)
	if m["port"] != float64(8080) {
		t.Errorf("expected port=8080, got %v", m["port"])
	}
}

func TestFields_Int64(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", Int64("big", 9999999999))
	m := decodeJSON(t, &buf)
	if m["big"] != float64(9999999999) {
		t.Errorf("expected big=9999999999, got %v", m["big"])
	}
}

func TestFields_Bool(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", Bool("flag", true))
	m := decodeJSON(t, &buf)
	if m["flag"] != true {
		t.Errorf("expected flag=true, got %v", m["flag"])
	}
}

func TestFields_Duration(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", Duration("latency", 5*time.Millisecond))
	m := decodeJSON(t, &buf)
	if m["latency"] == nil {
		t.Error("expected latency field, got nil")
	}
}

func TestFields_Time(t *testing.T) {
	var buf bytes.Buffer
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	captureLogger(&buf).Info("msg", Time("at", ts))
	m := decodeJSON(t, &buf)
	if m["at"] == nil {
		t.Error("expected at field, got nil")
	}
}

func TestFields_Any(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Info("msg", Any("data", map[string]int{"x": 1}))
	m := decodeJSON(t, &buf)
	if m["data"] == nil {
		t.Error("expected data field, got nil")
	}
}

func TestFields_Err(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf).Error("failed", Err(errors.New("something broke")))
	m := decodeJSON(t, &buf)
	if m["error"] != "something broke" {
		t.Errorf("expected error=something broke, got %v", m["error"])
	}
}

func TestFields_ErrNil(t *testing.T) {
	// Err(nil) must not panic and must not add an error field.
	var buf bytes.Buffer
	captureLogger(&buf).Error("msg", Err(nil))
	m := decodeJSON(t, &buf)
	if _, ok := m["error"]; ok {
		t.Error("expected no error field for Err(nil)")
	}
}

// ── With ──────────────────────────────────────────────────────────────────────

func TestWith_ChildInheritsFields(t *testing.T) {
	var buf bytes.Buffer
	parent := captureLogger(&buf)
	child := parent.With(String("component", "auth"))
	child.Info("login attempt")

	assertField(t, &buf, "component", "auth")
}

func TestWith_ParentUnchanged(t *testing.T) {
	var buf bytes.Buffer
	parent := captureLogger(&buf)
	_ = parent.With(String("component", "auth"))
	parent.Info("parent log")

	m := decodeJSON(t, &buf)
	if _, ok := m["component"]; ok {
		t.Error("parent logger should not have component field from child With()")
	}
}

func TestWith_Chaining(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf).
		With(String("service", "api")).
		With(String("version", "1.0"))

	log.Info("chained")

	m := decodeJSON(t, &buf)
	if m["service"] != "api" {
		t.Errorf("expected service=api, got %v", m["service"])
	}
	if m["version"] != "1.0" {
		t.Errorf("expected version=1.0, got %v", m["version"])
	}
}

func TestWith_MultipleFields(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf).With(
		String("a", "1"),
		String("b", "2"),
		String("c", "3"),
	)
	log.Info("multi")

	m := decodeJSON(t, &buf)
	for _, key := range []string{"a", "b", "c"} {
		if m[key] == nil {
			t.Errorf("expected field %q, not found in: %v", key, m)
		}
	}
}

// ── Nop ───────────────────────────────────────────────────────────────────────

func TestNop_ProducesNoOutput(t *testing.T) {
	// Nop() should silently discard everything including Fatal (no os.Exit).
	nop := Nop()
	nop.Debug("debug")
	nop.Info("info")
	nop.Warn("warn")
	nop.Error("error")
	// Fatal on a Nop logger does not call os.Exit — zerolog disables all events.
}

func TestNop_WithReturnsNop(t *testing.T) {
	child := Nop().With(String("k", "v"))
	// Should not panic and should still discard.
	child.Info("silent")
}

// ── Caller ────────────────────────────────────────────────────────────────────

func TestWithCaller_AttachesCallerField(t *testing.T) {
	var buf bytes.Buffer
	captureLogger(&buf, WithCaller()).Info("with caller")

	m := decodeJSON(t, &buf)
	if m["caller"] == nil {
		t.Error("expected caller field when WithCaller() is set")
	}
}

// ── Fatal (subprocess test) ──────────────────────────────────────────────────
//
// Calling Fatal triggers os.Exit(1) inside zerolog, so it cannot be tested in
// the main process. The standard Go pattern is to re-invoke the test binary as
// a subprocess (identified by the BE_CRASHER env var), let it exit naturally,
// and validate the captured stdout from the parent.

func TestFatal_WritesCorrectLevel(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		// Running inside the subprocess: write directly to os.Stdout so the
		// parent can capture it, then let zerolog call os.Exit(1).
		New(WithWriter(os.Stdout)).Fatal("unrecoverable")
		return // unreachable; here only to satisfy the compiler
	}

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")

	out, err := cmd.Output() // captures stdout; waits for exit
	if err == nil {
		t.Fatal("expected subprocess to exit with non-zero code, but it exited cleanly")
	}

	out = bytes.TrimSpace(out)
	var m map[string]interface{}
	if jsonErr := json.Unmarshal(out, &m); jsonErr != nil {
		t.Fatalf("subprocess output is not valid JSON: %v\nraw output: %s", jsonErr, out)
	}
	if m["level"] != "fatal" {
		t.Errorf("expected level=fatal, got %v", m["level"])
	}
	if m["message"] != "unrecoverable" {
		t.Errorf("expected message=unrecoverable, got %v", m["message"])
	}
}

// ── context helpers ───────────────────────────────────────────────────────────

func TestFromContext_ReturnsNopWhenAbsent(t *testing.T) {
	ctx := context.Background()
	log := FromContext(ctx)
	if log == nil {
		t.Fatal("FromContext returned nil, expected Nop logger")
	}
	// Must not panic.
	log.Info("silent")
}

func TestWithContext_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	original := captureLogger(&buf)

	ctx := WithContext(context.Background(), original)
	retrieved := FromContext(ctx)

	retrieved.Info("round trip")
	if buf.Len() == 0 {
		t.Error("expected retrieved logger to write output")
	}
}

func TestWithContext_DoesNotModifyParentContext(t *testing.T) {
	parent := context.Background()
	var buf bytes.Buffer
	ctx := WithContext(parent, captureLogger(&buf))
	_ = ctx

	// Parent context should still return Nop.
	log := FromContext(parent)
	log.Info("this goes to nop")
	if buf.Len() > 0 {
		t.Error("parent context should not have a logger after WithContext on derived context")
	}
}

// ── helper ────────────────────────────────────────────────────────────────────

func assertField(t *testing.T, buf *bytes.Buffer, key, expected string) {
	t.Helper()
	m := decodeJSON(t, buf)
	if m[key] != expected {
		t.Errorf("expected %s=%s, got %v", key, expected, m[key])
	}
}
