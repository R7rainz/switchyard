package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// testLogger discards output, for the tests that only care about behaviour.
func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

// captureLogs returns a logger writing JSON into buf, so assertions can read
// individual fields rather than grep a formatted string.
func captureLogs(buf *bytes.Buffer, level zerolog.Level) zerolog.Logger {
	return zerolog.New(buf).Level(level)
}

// decodeLines parses the buffer as one JSON object per line.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line %q is not JSON: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}

func TestRequestLoggerRecordsTheRequest(t *testing.T) {
	var buf bytes.Buffer
	router := testRouter(&stubVerifier{accept: "good"}, captureLogs(&buf, zerolog.InfoLevel))

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer good")
	router.ServeHTTP(httptest.NewRecorder(), request)

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("logged %d lines, want 1: %s", len(lines), buf.String())
	}

	line := lines[0]
	for field, want := range map[string]any{
		"message": "request",
		"level":   "info",
		"method":  "GET",
		"path":    "/api/me",
		"status":  float64(http.StatusOK),
	} {
		if line[field] != want {
			t.Errorf("%s = %v, want %v", field, line[field], want)
		}
	}
	if id, _ := line["request_id"].(string); id == "" {
		t.Error("log line carried no request_id")
	}
	if _, ok := line["duration"]; !ok {
		t.Error("log line carried no duration")
	}
}

func TestHealthzLogsAtDebug(t *testing.T) {
	// A load balancer polls this every few seconds; at info it would drown
	// everything else.
	var buf bytes.Buffer
	router := testRouter(&stubVerifier{accept: "good"}, captureLogs(&buf, zerolog.InfoLevel))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if buf.Len() != 0 {
		t.Errorf("healthz logged at info: %s", buf.String())
	}

	buf.Reset()
	router = testRouter(&stubVerifier{accept: "good"}, captureLogs(&buf, zerolog.DebugLevel))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if buf.Len() == 0 {
		t.Error("healthz logged nothing even at debug")
	}
}

func TestRejectedTokenIsLoggedButNotReturned(t *testing.T) {
	var buf bytes.Buffer
	router := testRouter(&stubVerifier{accept: "good"}, captureLogs(&buf, zerolog.InfoLevel))

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer forged")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// The operator gets the reason.
	var reason string
	for _, line := range decodeLines(t, &buf) {
		if line["message"] == "rejected token" {
			reason, _ = line["error"].(string)
		}
	}
	if reason == "" {
		t.Fatalf("rejection was not logged: %s", buf.String())
	}
	if !strings.Contains(reason, "signature does not verify") {
		t.Errorf("logged reason = %q, want the verifier's error", reason)
	}

	// The client does not.
	if body := recorder.Body.String(); strings.Contains(body, "signature") {
		t.Errorf("response body leaked the reason: %s", body)
	}
}

func TestLogPathRedactsInviteTokensForBothAPIPaths(t *testing.T) {
	for _, path := range []string{
		"/api/invites/secret-token/accept",
		"/api/v1/invites/secret-token/accept",
	} {
		if got := logPath(path); strings.Contains(got, "secret-token") {
			t.Errorf("logPath(%q) = %q, leaked the invite token", path, got)
		}
	}
}
