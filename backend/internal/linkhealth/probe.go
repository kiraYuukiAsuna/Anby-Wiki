// Package linkhealth safely probes external resources and composes proposals
// when their effective target changes.
package linkhealth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/anby/wiki/backend/internal/evidence"
)

var (
	ErrUnsafeURL = errors.New("linkhealth: URL rejected by SSRF policy")
	ErrProbe     = errors.New("linkhealth: probe failed")
)

const (
	maxProbeBytes = 64 << 10
	maxRedirects  = 5
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type ProbeResult struct {
	Status       string
	HTTPStatus   *int32
	ContentHash  *string
	CanonicalURL *string
	TargetURL    *string
}

type Prober interface {
	Probe(context.Context, string) (ProbeResult, error)
}

type HTTPProber struct {
	resolver Resolver
	client   *http.Client
}

func NewHTTPProber(resolver Resolver, transport http.RoundTripper) *HTTPProber {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 nil,
			DialContext:           (&safeDialer{resolver: resolver, timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		}
	}
	prober := &HTTPProber{resolver: resolver}
	prober.client = &http.Client{Transport: transport, Timeout: 30 * time.Second}
	prober.client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("%w: redirect limit", ErrUnsafeURL)
		}
		return prober.validateURL(request.Context(), request.URL)
	}
	return prober
}

func (p *HTTPProber) Probe(ctx context.Context, rawURL string) (ProbeResult, error) {
	target, err := url.Parse(rawURL)
	if err != nil || p.validateURL(ctx, target) != nil {
		return ProbeResult{}, ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return ProbeResult{}, ErrUnsafeURL
	}
	request.Header.Set("User-Agent", "AnbyWiki-LinkHealth/1.0")
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,*/*;q=0.1")
	request.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxProbeBytes-1))

	response, err := p.client.Do(request)
	if err != nil {
		if errors.Is(err, ErrUnsafeURL) {
			return ProbeResult{}, ErrUnsafeURL
		}
		return ProbeResult{}, fmt.Errorf("%w: %v", ErrProbe, err)
	}
	defer response.Body.Close()

	code := int32(response.StatusCode)
	result := ProbeResult{HTTPStatus: &code}
	finalURL, err := evidence.NormalizeURL(response.Request.URL.String())
	if err != nil {
		return ProbeResult{}, ErrUnsafeURL
	}
	originalURL, _ := evidence.NormalizeURL(rawURL)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Status = evidence.ExternalResourceStatusBroken
		return result, nil
	}
	if finalURL != originalURL {
		result.Status = evidence.ExternalResourceStatusRedirect
		result.TargetURL = &finalURL
	} else {
		result.Status = evidence.ExternalResourceStatusOK
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxProbeBytes))
	if err != nil {
		return ProbeResult{}, fmt.Errorf("%w: read body: %v", ErrProbe, err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	result.ContentHash = &hash

	if canonical, ok := canonicalLink(response.Request.URL, response.Header.Values("Link")); ok {
		// Canonical metadata is untrusted. Unsafe targets are ignored rather
		// than changing the status of an otherwise healthy original resource.
		if p.validateURL(ctx, canonical) == nil {
			normalized, err := evidence.NormalizeURL(canonical.String())
			if err == nil {
				result.CanonicalURL = &normalized
				if normalized != originalURL {
					result.TargetURL = &normalized
				}
			}
		}
	}
	return result, nil
}

func (p *HTTPProber) validateURL(ctx context.Context, target *url.URL) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") ||
		target.Hostname() == "" || target.User != nil ||
		(target.Port() != "" && target.Port() != "80" && target.Port() != "443") {
		return ErrUnsafeURL
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrUnsafeURL
	}
	addresses, err := p.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("%w: DNS", ErrUnsafeURL)
	}
	for _, address := range addresses {
		if !publicIP(address.IP) {
			return fmt.Errorf("%w: non-public address", ErrUnsafeURL)
		}
	}
	return nil
}

func canonicalLink(base *url.URL, values []string) (*url.URL, bool) {
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			parts := strings.Split(item, ";")
			if len(parts) < 2 {
				continue
			}
			rawTarget := strings.TrimSpace(parts[0])
			if len(rawTarget) < 2 || rawTarget[0] != '<' || rawTarget[len(rawTarget)-1] != '>' {
				continue
			}
			canonical := false
			for _, parameter := range parts[1:] {
				key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if ok && strings.EqualFold(key, "rel") &&
					strings.EqualFold(strings.Trim(value, `"'`), "canonical") {
					canonical = true
					break
				}
			}
			if !canonical {
				continue
			}
			parsed, err := url.Parse(rawTarget[1 : len(rawTarget)-1])
			if err == nil {
				return base.ResolveReference(parsed), true
			}
		}
	}
	return nil, false
}

var rejectedIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func publicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range rejectedIPPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

type safeDialer struct {
	resolver Resolver
	timeout  time.Duration
}

func (d *safeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrUnsafeURL
	}
	addresses, err := d.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: d.timeout}
	for _, candidate := range addresses {
		if !publicIP(candidate.IP) {
			return nil, ErrUnsafeURL
		}
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	return nil, ErrProbe
}
