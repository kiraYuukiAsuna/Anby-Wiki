package linkhealth_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/linkhealth"
)

type resolverStub struct {
	values map[string][]net.IPAddr
}

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

func response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
	}
}

func TestHTTPProberBlocksSSRFAndTracksRedirectCanonicalAndStatus(t *testing.T) {
	resolver := resolverStub{values: map[string][]net.IPAddr{
		"public.example": {{IP: net.ParseIP("93.184.216.34")}},
		"other.example":  {{IP: net.ParseIP("93.184.216.35")}},
		"private.test":   {{IP: net.ParseIP("10.0.0.8")}},
	}}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/redirect":
			result := response(request, http.StatusMovedPermanently, "")
			result.Header.Set("Location", "https://other.example/final")
			return result, nil
		case "/redirect-missing":
			result := response(request, http.StatusMovedPermanently, "")
			result.Header.Set("Location", "https://other.example/missing")
			return result, nil
		case "/private":
			result := response(request, http.StatusFound, "")
			result.Header.Set("Location", "http://169.254.169.254/latest/meta-data")
			return result, nil
		case "/missing":
			return response(request, http.StatusNotFound, "missing"), nil
		case "/final":
			return response(request, http.StatusOK, "redirected"), nil
		case "/unsafe-canonical":
			result := response(request, http.StatusOK, "healthy")
			result.Header.Set("Link", `<http://10.0.0.8/admin>; rel="canonical"`)
			return result, nil
		default:
			result := response(request, http.StatusOK, "healthy")
			result.Header.Set("Link", `</canonical?utm_source=probe>; rel="canonical"`)
			return result, nil
		}
	})
	prober := linkhealth.NewHTTPProber(resolver, transport)

	for _, rawURL := range []string{
		"http://127.0.0.1/admin",
		"http://private.test/",
		"http://192.0.2.1/documentation",
		"http://198.51.100.2/documentation",
		"http://203.0.113.3/documentation",
		"http://240.0.0.1/reserved",
		"http://[64:ff9b::a00:1]/translated-private",
		"http://[2001:db8::1]/documentation",
		"http://[2002:0a00:0001::]/six-to-four-private",
		"http://[fec0::1]/deprecated-site-local",
		"file:///etc/passwd",
		"https://public.example:8443/",
		"https://user:secret@public.example/",
	} {
		if _, err := prober.Probe(context.Background(), rawURL); !errors.Is(err, linkhealth.ErrUnsafeURL) {
			t.Fatalf("Probe(%q) err=%v, want ErrUnsafeURL", rawURL, err)
		}
	}
	if _, err := prober.Probe(context.Background(), "https://public.example/private"); !errors.Is(err, linkhealth.ErrUnsafeURL) {
		t.Fatalf("unsafe redirect err=%v", err)
	}

	redirect, err := prober.Probe(context.Background(), "https://public.example/redirect")
	if err != nil || redirect.Status != evidence.ExternalResourceStatusRedirect ||
		redirect.TargetURL == nil || *redirect.TargetURL != "https://other.example/final" ||
		redirect.HTTPStatus == nil || *redirect.HTTPStatus != http.StatusOK {
		t.Fatalf("redirect=%+v err=%v", redirect, err)
	}
	ok, err := prober.Probe(context.Background(), "https://public.example/article")
	if err != nil || ok.Status != evidence.ExternalResourceStatusOK || ok.ContentHash == nil ||
		ok.CanonicalURL == nil || *ok.CanonicalURL != "https://public.example/canonical" ||
		ok.TargetURL == nil || *ok.TargetURL != "https://public.example/canonical" {
		t.Fatalf("ok=%+v err=%v", ok, err)
	}
	broken, err := prober.Probe(context.Background(), "https://public.example/missing")
	if err != nil || broken.Status != evidence.ExternalResourceStatusBroken ||
		broken.HTTPStatus == nil || *broken.HTTPStatus != http.StatusNotFound {
		t.Fatalf("broken=%+v err=%v", broken, err)
	}
	redirectMissing, err := prober.Probe(context.Background(), "https://public.example/redirect-missing")
	if err != nil || redirectMissing.Status != evidence.ExternalResourceStatusBroken ||
		redirectMissing.TargetURL != nil {
		t.Fatalf("redirect missing=%+v err=%v", redirectMissing, err)
	}
	unsafeCanonical, err := prober.Probe(context.Background(), "https://public.example/unsafe-canonical")
	if err != nil || unsafeCanonical.Status != evidence.ExternalResourceStatusOK ||
		unsafeCanonical.CanonicalURL != nil || unsafeCanonical.TargetURL != nil {
		t.Fatalf("unsafe canonical=%+v err=%v", unsafeCanonical, err)
	}
}
