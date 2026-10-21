package linkhealth

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type previewResolver struct {
	ip net.IP
}

func (r previewResolver) LookupIPAddr(
	context.Context,
	string,
) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: r.ip}}, nil
}

type previewRoundTripper func(*http.Request) (*http.Response, error)

func (f previewRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

func TestResolvePreviewExtractsMetadata(t *testing.T) {
	prober := NewHTTPProber(
		previewResolver{ip: net.ParseIP("8.8.8.8")},
		previewRoundTripper(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/html; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader(`
					<html><head>
					<title>Fallback title</title>
					<meta property="og:title" content="  Preview   title ">
					<meta property="og:description" content="Useful description">
					<meta property="og:site_name" content="Example">
					<script>window.parent.hacked = true</script>
					</head></html>`)),
				Request: request,
			}, nil
		}),
	)
	result, err := prober.ResolvePreview(
		context.Background(), "https://example.com/article",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://example.com/article" ||
		result.Title != "Preview title" ||
		result.Description != "Useful description" ||
		result.SiteName != "Example" {
		t.Fatalf("unexpected preview: %#v", result)
	}
}

func TestResolvePreviewRejectsPrivateAddress(t *testing.T) {
	prober := NewHTTPProber(
		previewResolver{ip: net.ParseIP("127.0.0.1")},
		previewRoundTripper(func(*http.Request) (*http.Response, error) {
			t.Fatal("unsafe URL reached transport")
			return nil, nil
		}),
	)
	if _, err := prober.ResolvePreview(
		context.Background(), "https://internal.example/",
	); err != ErrUnsafeURL {
		t.Fatalf("expected ErrUnsafeURL, got %v", err)
	}
}
