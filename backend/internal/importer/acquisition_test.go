package importer_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/importer"
)

type resolverStub struct{ values map[string][]net.IPAddr }

func (r resolverStub) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	if values := r.values[host]; values != nil {
		return values, nil
	}
	return nil, errors.New("not found")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func response(request *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}},
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}
}

func TestFetcher_SSRFRedirectSizeAndMIMEPolicy(t *testing.T) {
	resolver := resolverStub{values: map[string][]net.IPAddr{
		"public.example":  {{IP: net.ParseIP("93.184.216.34")}},
		"private.example": {{IP: net.ParseIP("10.0.0.8")}},
	}}
	policy := importer.DefaultURLPolicy()
	policy.MaxBytes = 80
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/redirect-private":
			result := response(request, http.StatusFound, "text/plain", "")
			result.Header.Set("Location", "http://169.254.169.254/latest/meta-data")
			return result, nil
		case "/large":
			return response(request, http.StatusOK, "text/plain", strings.Repeat("x", 81)), nil
		case "/fake-pdf":
			return response(request, http.StatusOK, "application/pdf", "not a pdf"), nil
		default:
			return response(request, http.StatusOK, "text/html", "<!doctype html><html><p>safe</p></html>"), nil
		}
	})
	fetcher := importer.NewFetcher(policy, resolver, transport)

	for _, target := range []string{
		"http://127.0.0.1/admin", "http://[::1]/", "http://private.example/",
		"file:///etc/passwd", "http://public.example:8080/",
	} {
		if _, err := fetcher.Fetch(context.Background(), target); !errors.Is(err, importer.ErrUnsafeURL) {
			t.Fatalf("target=%s err=%v", target, err)
		}
	}
	if _, err := fetcher.Fetch(context.Background(), "http://public.example/redirect-private"); !errors.Is(err, importer.ErrFetchFailed) {
		// Client 把 CheckRedirect 的 ErrUnsafeURL 包装为 url.Error，Fetcher 再统一包装 FetchFailed。
		t.Fatalf("redirect err=%v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), "https://public.example/large"); !errors.Is(err, importer.ErrSourceTooLarge) {
		t.Fatalf("large err=%v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), "https://public.example/fake-pdf"); !errors.Is(err, importer.ErrUnsupportedMIME) {
		t.Fatalf("mime err=%v", err)
	}
	got, err := fetcher.Fetch(context.Background(), "https://public.example/article")
	if err != nil || got.ContentHash == "" || got.MIMEType != "text/html" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestValidateUpload_MalwareAndLimitsBeforeStorage(t *testing.T) {
	policy := importer.DefaultURLPolicy()
	malicious := []byte("<!doctype html><html><script>document.cookie</script></html>")
	if _, err := importer.ValidateUpload(context.Background(), policy, nil, "../../evil.html",
		"text/html", malicious); !errors.Is(err, importer.ErrMalware) {
		t.Fatalf("malware err=%v", err)
	}
	valid, err := importer.ValidateUpload(context.Background(), policy, nil, "../../safe.pdf",
		"application/pdf", []byte("%PDF-1.4\n%%Page: 1\n(hello) Tj"))
	if err != nil || valid.Filename != "safe.pdf" {
		t.Fatalf("valid=%+v err=%v", valid, err)
	}
}
