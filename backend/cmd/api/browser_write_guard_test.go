package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserWriteGuard_SourceRules(t *testing.T) {
	var calls int
	handler := BrowserWriteGuard("anby_session", []string{"https://wiki.example.com"})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	tests := []struct {
		name    string
		method  string
		cookie  bool
		origin  string
		referer string
		want    int
	}{
		{name: "safe method", method: http.MethodGet, cookie: true, want: http.StatusNoContent},
		{name: "development header flow", method: http.MethodPost, want: http.StatusNoContent},
		{name: "trusted origin", method: http.MethodPost, cookie: true, origin: "https://wiki.example.com", want: http.StatusNoContent},
		{name: "trusted referer fallback", method: http.MethodPatch, cookie: true, referer: "https://wiki.example.com/edit/1", want: http.StatusNoContent},
		{name: "missing source", method: http.MethodPost, cookie: true, want: http.StatusForbidden},
		{name: "hostile origin", method: http.MethodPut, cookie: true, origin: "https://evil.example", want: http.StatusForbidden},
		{name: "origin takes precedence", method: http.MethodDelete, cookie: true, origin: "https://evil.example", referer: "https://wiki.example.com/edit", want: http.StatusForbidden},
		{name: "origin path rejected", method: http.MethodPost, cookie: true, origin: "https://wiki.example.com/evil", want: http.StatusForbidden},
		{name: "referer userinfo rejected", method: http.MethodPost, cookie: true, referer: "https://evil@wiki.example.com/edit", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := calls
			request := httptest.NewRequest(tt.method, "/api/v1/resource", nil)
			if tt.cookie {
				request.AddCookie(&http.Cookie{Name: "anby_session", Value: "token"})
			}
			request.Header.Set("Origin", tt.origin)
			request.Header.Set("Referer", tt.referer)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tt.want, recorder.Body.String())
			}
			if tt.want == http.StatusForbidden && calls != before {
				t.Fatal("被拒绝的请求不应调用下游 handler")
			}
		})
	}
}
