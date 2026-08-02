package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

func TestWriteErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "no identity", err: auth.ErrNoIdentity, wantStatus: http.StatusUnauthorized},
		{name: "not the owner", err: auth.ErrNotOwner, wantStatus: http.StatusNotFound},
		{name: "wrapped not the owner", err: errors.Join(errors.New("loading workflow"), auth.ErrNotOwner), wantStatus: http.StatusNotFound},
		{name: "anything else", err: errors.New("database is on fire"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeError(recorder, httptest.NewRequest(http.MethodGet, "/api/whatever", nil), tc.err)

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
		})
	}
}

func TestNotOwnerIsIndistinguishableFromMissing(t *testing.T) {
	// A 403 would confirm the resource exists, letting any account probe for
	// other users' ids. The two responses must look identical.
	request := httptest.NewRequest(http.MethodGet, "/api/workflows/abc", nil)

	notOwner := httptest.NewRecorder()
	writeError(notOwner, request, auth.ErrNotOwner)

	if notOwner.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", notOwner.Code)
	}
	if body := notOwner.Body.String(); !strings.Contains(body, "not found") {
		t.Errorf("body = %q, want it to say only 'not found'", body)
	}
	if body := notOwner.Body.String(); strings.Contains(body, "own") || strings.Contains(body, "forbid") {
		t.Errorf("body = %q leaks that the resource exists", body)
	}
}

func TestInternalErrorIsLoggedNotReturned(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	request := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	request = request.WithContext(logger.WithContext(request.Context()))

	recorder := httptest.NewRecorder()
	writeError(recorder, request, errors.New("connection refused to secret-host:5432"))

	// The operator gets the cause.
	if !strings.Contains(buf.String(), "connection refused") {
		t.Errorf("cause was not logged: %s", buf.String())
	}
	// The client does not.
	if body := recorder.Body.String(); strings.Contains(body, "connection refused") || strings.Contains(body, "secret-host") {
		t.Errorf("response leaked the cause: %s", body)
	}
}

func TestRequestLoggerPutsLoggerOnContext(t *testing.T) {
	// writeError relies on this: without it a 500 would be logged nowhere.
	var buf bytes.Buffer
	var found bool

	handler := RequestLogger(zerolog.New(&buf))(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		found = zerolog.Ctx(r.Context()).GetLevel() != zerolog.Disabled
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/me", nil))

	if !found {
		t.Error("handler found no logger on the request context")
	}
}
