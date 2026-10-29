package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"unicode/utf8"
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

const (
	DefaultMaxSourceBytes = int64(300 << 20)
	MIMEMarkdown          = "text/markdown"
)

func DefaultURLPolicy() URLPolicy {
	return URLPolicy{
		MaxBytes: DefaultMaxSourceBytes, MaxRedirects: 3,
		AllowedMIMEs: map[string]bool{
			"text/html":        true,
			"text/plain":       true,
			MIMEMarkdown:       true,
			"text/csv":         true,
			"application/pdf":  true,
			"application/json": true,
			"image/png":        true,
			"image/jpeg":       true,
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
	request.Header.Set(
		"Accept",
		"text/html,"+MIMEMarkdown+",application/pdf,text/plain,text/csv,application/json,image/png,image/jpeg",
	)
	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &sourceHTTPError{status: response.StatusCode}
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
	filename := filepath.Base(response.Request.URL.Path)
	mimeType := normalizedMIME(response.Header.Get("Content-Type"))
	mimeType = refineMIME(filename, mimeType, content)
	if !f.policy.AllowedMIMEs[mimeType] || !magicMatches(mimeType, content) {
		return nil, ErrUnsupportedMIME
	}
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "source" + extensionForMIME(mimeType)
	}
	sum := sha256.Sum256(content)
	return &AcquiredSource{URL: response.Request.URL.String(), Filename: filename,
		MIMEType: mimeType, Content: content, ContentHash: hex.EncodeToString(sum[:])}, nil
}

func normalizedMIME(value string) string {
	parsed, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed) {
	case "image/jpg", "image/pjpeg":
		return "image/jpeg"
	case "application/csv":
		return "text/csv"
	case "text/json":
		return "application/json"
	case "text/x-markdown":
		return MIMEMarkdown
	default:
		return strings.ToLower(parsed)
	}
}

func refineMIME(filename, mimeType string, content []byte) string {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if (mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain") &&
		(extension == ".md" || extension == ".markdown") {
		return MIMEMarkdown
	}
	if (mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain") &&
		json.Valid(bytes.TrimSpace(content)) {
		return "application/json"
	}
	if (mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain") &&
		extension == ".csv" {
		return "text/csv"
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		return normalizedMIME(http.DetectContentType(content))
	}
	return mimeType
}

func extensionForMIME(mimeType string) string {
	switch mimeType {
	case "text/html":
		return ".html"
	case "text/plain":
		return ".txt"
	case MIMEMarkdown:
		return ".md"
	case "text/csv":
		return ".csv"
	case "application/pdf":
		return ".pdf"
	case "application/json":
		return ".json"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ""
	}
}

func magicMatches(mimeType string, content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	switch mimeType {
	case "application/pdf":
		return bytes.HasPrefix(trimmed, []byte("%PDF-"))
	case "text/html":
		lower := bytes.ToLower(trimmed)
		prefix := lower[:min(len(lower), 512)]
		for _, marker := range [][]byte{
			[]byte("<!doctype html"),
			[]byte("<html"),
			[]byte("<head"),
			[]byte("<body"),
			[]byte("<main"),
			[]byte("<article"),
			[]byte("<section"),
			[]byte("<div"),
			[]byte("<p"),
			[]byte("<pre"),
		} {
			if bytes.Contains(prefix, marker) {
				return true
			}
		}
		return false
	case "text/plain", MIMEMarkdown:
		return utf8.Valid(content)
	case "text/csv":
		return utf8.Valid(content) && !bytes.Contains(content, []byte{0})
	case "application/json":
		return json.Valid(trimmed)
	case "image/png":
		return len(content) >= 8 &&
			bytes.Equal(content[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	case "image/jpeg":
		return len(content) >= 3 &&
			content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff
	default:
		return false
	}
}

const (
	uploadSniffBytes       = 4096
	signatureScanChunkSize = 64 << 10
)

var blockedUploadSignatures = [][]byte{
	[]byte("EICAR-STANDARD-ANTIVIRUS-TEST-FILE"),
	[]byte("<SCRIPT>DOCUMENT.COOKIE"),
}

type MalwareScanner interface {
	Scan(ctx context.Context, content io.Reader) error
}

type SignatureScanner struct{}

func (SignatureScanner) Scan(ctx context.Context, content io.Reader) error {
	maxSignatureBytes := 0
	for _, signature := range blockedUploadSignatures {
		maxSignatureBytes = max(maxSignatureBytes, len(signature))
	}
	overlapBytes := maxSignatureBytes - 1
	buffer := make([]byte, signatureScanChunkSize+overlapBytes)
	overlap := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, err := content.Read(buffer[overlap : overlap+signatureScanChunkSize])
		total := overlap + read
		for index := overlap; index < total; index++ {
			if buffer[index] >= 'a' && buffer[index] <= 'z' {
				buffer[index] -= 'a' - 'A'
			}
		}
		for _, signature := range blockedUploadSignatures {
			if bytes.Contains(buffer[:total], signature) {
				return ErrMalware
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: 扫描上传内容失败: %v", ErrFetchFailed, err)
		}
		overlap = min(overlapBytes, total)
		copy(buffer[:overlap], buffer[total-overlap:total])
	}
}

type ValidatedUpload struct {
	Filename    string
	MIMEType    string
	ContentHash string
	SizeBytes   int64
}

func ValidateUploadReader(
	ctx context.Context,
	policy URLPolicy,
	scanner MalwareScanner,
	filename, mimeType string,
	content io.ReadSeeker,
) (*ValidatedUpload, error) {
	if policy.MaxBytes <= 0 {
		policy = DefaultURLPolicy()
	}
	if content == nil {
		return nil, ErrFetchFailed
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, ErrFetchFailed
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(content, policy.MaxBytes+1))
	if err != nil {
		return nil, ErrFetchFailed
	}
	if size > policy.MaxBytes {
		return nil, ErrSourceTooLarge
	}
	mimeType, err = refineMIMEReader(filename, normalizedMIME(mimeType), content)
	if err != nil {
		return nil, ErrFetchFailed
	}
	matches, err := magicMatchesReader(mimeType, content)
	if err != nil {
		return nil, ErrFetchFailed
	}
	if !policy.AllowedMIMEs[mimeType] || !matches {
		return nil, ErrUnsupportedMIME
	}
	if scanner == nil {
		scanner = SignatureScanner{}
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, ErrFetchFailed
	}
	if err := scanner.Scan(ctx, content); err != nil {
		if errors.Is(err, ErrFetchFailed) || errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrMalware
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == "" {
		filename = "source"
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, ErrFetchFailed
	}
	return &ValidatedUpload{
		Filename: filename, MIMEType: mimeType,
		ContentHash: hex.EncodeToString(hash.Sum(nil)), SizeBytes: size,
	}, nil
}

func ValidateUpload(
	ctx context.Context,
	policy URLPolicy,
	scanner MalwareScanner,
	filename, mimeType string,
	content []byte,
) (*AcquiredSource, error) {
	validated, err := ValidateUploadReader(
		ctx, policy, scanner, filename, mimeType, bytes.NewReader(content),
	)
	if err != nil {
		return nil, err
	}
	return &AcquiredSource{
		Filename: validated.Filename, MIMEType: validated.MIMEType,
		Content: content, ContentHash: validated.ContentHash,
	}, nil
}

func refineMIMEReader(filename, mimeType string, content io.ReadSeeker) (string, error) {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if (mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain") &&
		(extension == ".md" || extension == ".markdown") {
		return MIMEMarkdown, nil
	}
	if mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain" {
		valid, err := validJSONReader(content)
		if err != nil {
			return "", err
		}
		if valid {
			return "application/json", nil
		}
	}
	if (mimeType == "" || mimeType == "application/octet-stream" || mimeType == "text/plain") &&
		extension == ".csv" {
		return "text/csv", nil
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		prefix, err := uploadPrefix(content)
		if err != nil {
			return "", err
		}
		return normalizedMIME(http.DetectContentType(prefix)), nil
	}
	return mimeType, nil
}

func magicMatchesReader(mimeType string, content io.ReadSeeker) (bool, error) {
	prefix, err := uploadPrefix(content)
	if err != nil {
		return false, err
	}
	trimmed := bytes.TrimSpace(prefix)
	switch mimeType {
	case "application/pdf":
		return bytes.HasPrefix(trimmed, []byte("%PDF-")), nil
	case "text/html":
		lower := bytes.ToLower(trimmed)
		prefix := lower[:min(len(lower), 512)]
		for _, marker := range [][]byte{
			[]byte("<!doctype html"),
			[]byte("<html"),
			[]byte("<head"),
			[]byte("<body"),
			[]byte("<main"),
			[]byte("<article"),
			[]byte("<section"),
			[]byte("<div"),
			[]byte("<p"),
			[]byte("<pre"),
		} {
			if bytes.Contains(prefix, marker) {
				return true, nil
			}
		}
		return false, nil
	case "text/plain", MIMEMarkdown:
		return validUTF8Reader(content)
	case "text/csv":
		contains, err := readerContainsByte(content, 0)
		if err != nil || contains {
			return false, err
		}
		return validUTF8Reader(content)
	case "application/json":
		return validJSONReader(content)
	case "image/png":
		return len(prefix) >= 8 &&
			bytes.Equal(prefix[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}), nil
	case "image/jpeg":
		return len(prefix) >= 3 &&
			prefix[0] == 0xff && prefix[1] == 0xd8 && prefix[2] == 0xff, nil
	default:
		return false, nil
	}
}

func uploadPrefix(content io.ReadSeeker) ([]byte, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	prefix := make([]byte, uploadSniffBytes)
	read, err := io.ReadFull(content, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return prefix[:read], nil
}

func readerContainsByte(content io.ReadSeeker, target byte) (bool, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	buffer := make([]byte, signatureScanChunkSize)
	for {
		read, err := content.Read(buffer)
		if bytes.IndexByte(buffer[:read], target) >= 0 {
			return true, nil
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func validUTF8Reader(content io.ReadSeeker) (bool, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	buffer := make([]byte, signatureScanChunkSize+utf8.UTFMax)
	pending := 0
	for {
		read, err := content.Read(buffer[pending:])
		total := pending + read
		complete := total
		if err == nil && !utf8.Valid(buffer[:complete]) {
			minimum := max(0, complete-(utf8.UTFMax-1))
			for complete > minimum && !utf8.Valid(buffer[:complete]) {
				complete--
			}
		}
		if !utf8.Valid(buffer[:complete]) {
			return false, nil
		}
		pending = total - complete
		copy(buffer[:pending], buffer[complete:total])
		if errors.Is(err, io.EOF) {
			return pending == 0, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func validJSONReader(content io.ReadSeeker) (bool, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	decoder := json.NewDecoder(content)
	decoder.UseNumber()
	token, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil || !consumeJSONToken(decoder, token) {
		return false, nil
	}
	_, err = decoder.Token()
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, nil
}

func consumeJSONToken(decoder *json.Decoder, token json.Token) bool {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return false
			}
			if _, ok := key.(string); !ok {
				return false
			}
			value, err := decoder.Token()
			if err != nil || !consumeJSONToken(decoder, value) {
				return false
			}
		}
		closeToken, err := decoder.Token()
		return err == nil && closeToken == json.Delim('}')
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil || !consumeJSONToken(decoder, value) {
				return false
			}
		}
		closeToken, err := decoder.Token()
		return err == nil && closeToken == json.Delim(']')
	default:
		return false
	}
}
