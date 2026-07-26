package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripSpoofableAuthHeaders(t *testing.T) {
	echoActor := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Seen-Actor", r.Header.Get("X-Actor-ID"))
		w.Header().Set("Seen-Remote-User", r.Header.Get("X-Remote-User"))
		w.WriteHeader(http.StatusNoContent)
	})

	for _, test := range []struct {
		name       string
		allowActor bool
		wantActor  string
	}{
		{name: "production strips actor", allowActor: false},
		{name: "explicit dev mode preserves actor", allowActor: true, wantActor: "actor-id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
			request.Header.Set("X-Actor-ID", "actor-id")
			request.Header.Set("X-Remote-User", "spoofed")
			recorder := httptest.NewRecorder()

			StripSpoofableAuthHeaders(test.allowActor)(echoActor).ServeHTTP(recorder, request)

			if got := recorder.Header().Get("Seen-Actor"); got != test.wantActor {
				t.Fatalf("Seen-Actor=%q want %q", got, test.wantActor)
			}
			if got := recorder.Header().Get("Seen-Remote-User"); got != "" {
				t.Fatalf("Seen-Remote-User=%q want empty", got)
			}
		})
	}
}

func TestRequestBodyLimitRejectsKnownOversizeBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("oversize request reached handler")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pages",
		bytes.NewReader(make([]byte, defaultRequestBodyLimit+1)))
	recorder := httptest.NewRecorder()

	RequestBodyLimit(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRequestBodyLimitCapsUnknownLengthStream(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		var maxErr *http.MaxBytesError
		if !errors.As(err, &maxErr) {
			t.Fatalf("read error=%v want MaxBytesError", err)
		}
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pages",
		bytes.NewReader(make([]byte, defaultRequestBodyLimit+1)))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()

	RequestBodyLimit(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRequestBodyLimitAllowsLargerUploadEnvelope(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), int(defaultRequestBodyLimit+1))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("upload body read: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/import-jobs/uploads",
		bytes.NewReader(payload))
	recorder := httptest.NewRecorder()

	RequestBodyLimit(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d want %d", recorder.Code, http.StatusNoContent)
	}
}
