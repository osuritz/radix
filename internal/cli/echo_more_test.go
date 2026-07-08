package cli

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osuritz/radix/internal/config"
	"github.com/spf13/cobra"
)

func TestWriteJSONStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONStatus(rec, "ok")

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want %q", body["status"], "ok")
	}
}

// newEchoFlagsCmd builds a fresh command carrying the echo flag set bound to
// the same package globals, so tests can mark flags Changed without mutating
// the shared echoCmd. Registering the flags resets the globals to defaults;
// callers must save/restore any globals they care about.
func newEchoFlagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "echo"}
	c.Flags().IntVarP(&echoStatus, "status", "s", 200, "")
	c.Flags().DurationVar(&echoDelay, "delay", 0, "")
	c.Flags().DurationVar(&echoDelayJitter, "delay-jitter", 0, "")
	c.Flags().StringVar(&echoBody, "body", "", "")
	c.Flags().StringVar(&echoContentType, "content-type", "application/json", "")
	c.Flags().StringArrayVar(&echoHeaders, "header", nil, "")
	c.Flags().BoolVar(&echoEchoBody, "echo-body", true, "")
	c.Flags().BoolVar(&echoEchoHeaders, "echo-headers", true, "")
	c.Flags().BoolVar(&echoEchoQuery, "echo-query", true, "")
	c.Flags().IntVar(&echoBodyLimit, "body-limit", 1048576, "")
	c.Flags().BoolVar(&echoPretty, "pretty", true, "")
	c.Flags().BoolVar(&echoStatusFromPath, "status-from-path", false, "")
	c.Flags().BoolVar(&echoDelayFromPath, "delay-from-path", false, "")
	c.Flags().BoolVar(&echoCORS, "cors", false, "")
	return c
}

// restoreEchoFlagGlobals snapshots all echo flag globals and restores them on
// test cleanup.
func restoreEchoFlagGlobals(t *testing.T) {
	t.Helper()
	oldStatus, oldDelay, oldJitter := echoStatus, echoDelay, echoDelayJitter
	oldBody, oldCT, oldHeaders := echoBody, echoContentType, echoHeaders
	oldEB, oldEH, oldEQ := echoEchoBody, echoEchoHeaders, echoEchoQuery
	oldLimit, oldPretty := echoBodyLimit, echoPretty
	oldSFP, oldDFP, oldCORS := echoStatusFromPath, echoDelayFromPath, echoCORS
	t.Cleanup(func() {
		echoStatus, echoDelay, echoDelayJitter = oldStatus, oldDelay, oldJitter
		echoBody, echoContentType, echoHeaders = oldBody, oldCT, oldHeaders
		echoEchoBody, echoEchoHeaders, echoEchoQuery = oldEB, oldEH, oldEQ
		echoBodyLimit, echoPretty = oldLimit, oldPretty
		echoStatusFromPath, echoDelayFromPath, echoCORS = oldSFP, oldDFP, oldCORS
	})
}

func TestRunEcho_FlagOverridesApplied(t *testing.T) {
	restoreEchoFlagGlobals(t)
	cmd := newEchoFlagsCmd()

	flagValues := map[string]string{
		"status":           "201",
		"delay":            "1ms",
		"delay-jitter":     "1ms",
		"body":             `{"msg":"hi"}`,
		"content-type":     "text/plain",
		"header":           "X-Test: yes",
		"echo-body":        "false",
		"echo-headers":     "false",
		"echo-query":       "false",
		"body-limit":       "1024",
		"pretty":           "false",
		"status-from-path": "true",
		"delay-from-path":  "true",
		"cors":             "true",
	}
	for name, value := range flagValues {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	// Occupy the app port so runEcho executes its full body (handler chain,
	// server construction) and then fails at bind, without a live server.
	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Echo:    config.EchoConfig{Status: 200, ContentType: "application/json"},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runEcho(cmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	assertBindFailure(t, err)

	// All Changed flags must have overridden the config.
	e := cfg.Echo
	if e.Status != 201 {
		t.Errorf("Status = %d, want 201", e.Status)
	}
	if e.Delay != time.Millisecond {
		t.Errorf("Delay = %v, want 1ms", e.Delay)
	}
	if e.DelayJitter != time.Millisecond {
		t.Errorf("DelayJitter = %v, want 1ms", e.DelayJitter)
	}
	if e.Body != `{"msg":"hi"}` {
		t.Errorf("Body = %q, want the fixed body", e.Body)
	}
	if e.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", e.ContentType)
	}
	if len(e.Headers) != 1 || e.Headers[0] != "X-Test: yes" {
		t.Errorf("Headers = %v, want [X-Test: yes]", e.Headers)
	}
	if e.EchoBody || e.EchoHeaders || e.EchoQuery {
		t.Errorf("EchoBody/EchoHeaders/EchoQuery = %v/%v/%v, want all false", e.EchoBody, e.EchoHeaders, e.EchoQuery)
	}
	if e.BodyLimit != 1024 {
		t.Errorf("BodyLimit = %d, want 1024", e.BodyLimit)
	}
	if e.Pretty {
		t.Error("Pretty = true, want false")
	}
	if !e.StatusFromPath || !e.DelayFromPath {
		t.Errorf("StatusFromPath/DelayFromPath = %v/%v, want both true", e.StatusFromPath, e.DelayFromPath)
	}
	if !e.CORS {
		t.Error("CORS = false, want true")
	}
}

func TestRunEcho_FullServerPathBindFailure(t *testing.T) {
	// Metrics enabled + verbose + CORS make runEcho walk its collector,
	// logging, and middleware construction lines; only the bind error is
	// observed here, so this covers line paths, not middleware behavior.
	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Verbose: true,
		Echo:    config.EchoConfig{Status: 200, ContentType: "application/json", CORS: true},
		Metrics: config.MetricsConfig{Enabled: true, Port: freePort(t), Path: "/_metrics", Format: "json"},
	})

	err := runEcho(echoCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	assertBindFailure(t, err)
}

func TestRunEcho_TLSConfigError(t *testing.T) {
	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Echo: config.EchoConfig{Status: 200, ContentType: "application/json"},
		TLS: config.TLSConfig{
			Enabled: true,
			Cert:    "/nonexistent/cert.pem",
			Key:     "/nonexistent/key.pem",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runEcho(echoCmd, nil)
	if err == nil {
		t.Fatal("expected TLS configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("error = %v, want a TLS configuration error", err)
	}
}

func TestRunEcho_TLSSuccessPathBindFailure(t *testing.T) {
	certPath, keyPath, _ := genTestCerts(t)

	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Echo: config.EchoConfig{Status: 200, ContentType: "application/json"},
		TLS: config.TLSConfig{
			Enabled:    true,
			Cert:       certPath,
			Key:        keyPath,
			MinVersion: "1.3",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runEcho(echoCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("valid certs should pass TLS setup; got %v", err)
	}
}
