package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrUnsafeURL       = errors.New("importer: URL 被 SSRF 策略拒绝")
	ErrSourceTooLarge  = errors.New("importer: 来源超过大小限制")
	ErrUnsupportedMIME = errors.New("importer: 来源 MIME 不受支持")
	ErrMalware         = errors.New("importer: 来源被恶意内容扫描隔离")
	ErrFetchFailed     = errors.New("importer: 来源获取失败")
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type URLPolicy struct {
	MaxBytes     int64
	MaxRedirects int
	AllowedMIMEs map[string]bool
	AllowedPorts map[string]bool
}

func DefaultURLPolicy() URLPolicy {
	return URLPolicy{
		MaxBytes: 10 << 20, MaxRedirects: 3,
		AllowedMIMEs: map[string]bool{
			"text/html": true, "text/plain": true, "application/pdf": true,
		},
		AllowedPorts: map[string]bool{"": true, "80": true, "443": true},
	}
}

type Fetcher struct {
	policy   URLPolicy
	resolver Resolver
	client   *http.Client
}

func NewFetcher(policy URLPolicy, resolver Resolver, transport http.RoundTripper) *Fetcher {
	if policy.MaxBytes <= 0 {
		policy = DefaultURLPolicy()
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	f := &Fetcher{policy: policy, resolver: resolver}
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 nil,
			DialContext:           (&safeDialer{resolver: resolver, timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		}
	}
	f.client = &http.Client{Transport: transport, Timeout: 30 * time.Second}
	f.client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > policy.MaxRedirects {
			return fmt.Errorf("%w: redirect 超限", ErrUnsafeURL)
		}
		return f.ValidateURL(request.Context(), request.URL)
	}
	return f
}

func (f *Fetcher) ValidateURL(ctx context.Context, target *url.URL) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") ||
		target.Hostname() == "" || target.User != nil || !f.policy.AllowedPorts[target.Port()] {
		return ErrUnsafeURL
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrUnsafeURL
	}
	addresses, err := f.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("%w: DNS", ErrUnsafeURL)
	}
	for _, address := range addresses {
		if !publicIP(address.IP) {
			return fmt.Errorf("%w: private address", ErrUnsafeURL)
		}
	}
	return nil
}

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// Carrier-grade NAT、"this network" 与 benchmark 网段也不可作为外部抓取目标。
		if v4[0] == 0 || (v4[0] == 100 && v4[1]&0xc0 == 64) ||
			(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) {
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
	return nil, ErrFetchFailed
}

type AcquiredSource struct {
	URL         string
	Filename    string
	MIMEType    string
	Content     []byte
	ContentHash string
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*AcquiredSource, error) {
	target, err := url.Parse(rawURL)
	if err != nil || f.ValidateURL(ctx, target) != nil {
		return nil, ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, ErrUnsafeURL
	}
	request.Header.Set("User-Agent", "AnbyWiki-Importer/1.0")
	request.Header.Set("Accept", "text/html,application/pdf,text/plain")
	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrFetchFailed, response.StatusCode)
	}
	if response.ContentLength > f.policy.MaxBytes {
		return nil, ErrSourceTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, f.policy.MaxBytes+1))
	if err != nil {
		return nil, ErrFetchFailed
	}
	if int64(len(content)) > f.policy.MaxBytes {
		return nil, ErrSourceTooLarge
	}
	mimeType := normalizedMIME(response.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = normalizedMIME(http.DetectContentType(content))
	}
	if !f.policy.AllowedMIMEs[mimeType] || !magicMatches(mimeType, content) {
		return nil, ErrUnsupportedMIME
	}
	sum := sha256.Sum256(content)
	return &AcquiredSource{URL: response.Request.URL.String(), Filename: filepath.Base(response.Request.URL.Path),
		MIMEType: mimeType, Content: content, ContentHash: hex.EncodeToString(sum[:])}, nil
}

func normalizedMIME(value string) string {
	parsed, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed)
}

func magicMatches(mimeType string, content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	switch mimeType {
	case "application/pdf":
		return bytes.HasPrefix(trimmed, []byte("%PDF-"))
	case "text/html":
		lower := bytes.ToLower(trimmed)
		return bytes.Contains(lower[:min(len(lower), 512)], []byte("<html")) ||
			bytes.Contains(lower[:min(len(lower), 512)], []byte("<!doctype html"))
	case "text/plain":
		return !bytes.Contains(content, []byte{0})
	default:
		return false
	}
}

type MalwareScanner interface {
	Scan(ctx context.Context, content []byte) error
}

type SignatureScanner struct{}

func (SignatureScanner) Scan(_ context.Context, content []byte) error {
	upper := bytes.ToUpper(content)
	if bytes.Contains(upper, []byte("EICAR-STANDARD-ANTIVIRUS-TEST-FILE")) ||
		bytes.Contains(upper, []byte("<SCRIPT>DOCUMENT.COOKIE")) {
		return ErrMalware
	}
	return nil
}

func ValidateUpload(ctx context.Context, policy URLPolicy, scanner MalwareScanner, filename, mimeType string, content []byte) (*AcquiredSource, error) {
	if policy.MaxBytes <= 0 {
		policy = DefaultURLPolicy()
	}
	if int64(len(content)) > policy.MaxBytes {
		return nil, ErrSourceTooLarge
	}
	mimeType = normalizedMIME(mimeType)
	if !policy.AllowedMIMEs[mimeType] || !magicMatches(mimeType, content) {
		return nil, ErrUnsupportedMIME
	}
	if scanner == nil {
		scanner = SignatureScanner{}
	}
	if err := scanner.Scan(ctx, content); err != nil {
		return nil, ErrMalware
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == "" {
		filename = "source"
	}
	sum := sha256.Sum256(content)
	return &AcquiredSource{Filename: filename, MIMEType: mimeType, Content: content,
		ContentHash: hex.EncodeToString(sum[:])}, nil
}
