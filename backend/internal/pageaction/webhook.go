// Package pageaction executes explicitly confirmed side effects declared by
// page button blocks. Authoritative writes stay in their owning domain
// services; this package only owns the restricted outbound webhook channel.
package pageaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
)

var (
	ErrButtonNotFound  = errors.New("pageaction: button not found")
	ErrActionNotFound  = errors.New("pageaction: action not found")
	ErrInvalidWebhook  = errors.New("pageaction: invalid webhook")
	ErrWebhookRejected = errors.New("pageaction: webhook rejected")
)

const maxWebhookPayloadBytes = 64 << 10

type pageReader interface {
	CurrentContent(context.Context, uuid.UUID) (*page.Revision, *page.ContentSnapshot, error)
}

type WebhookResult struct {
	ExecutionID uuid.UUID `json:"execution_id"`
	StatusCode  int       `json:"status_code"`
}

type Service struct {
	pages    pageReader
	resolver *net.Resolver
}

func NewService(pages pageReader) *Service {
	return &Service{pages: pages, resolver: net.DefaultResolver}
}

func (s *Service) ExecuteWebhook(
	ctx context.Context,
	pageID uuid.UUID,
	blockID string,
	actionIndex int,
	executionID uuid.UUID,
) (*WebhookResult, error) {
	if s == nil || s.pages == nil || pageID == uuid.Nil || blockID == "" ||
		actionIndex < 0 || executionID == uuid.Nil {
		return nil, ErrInvalidWebhook
	}
	_, snapshot, err := s.pages.CurrentContent(ctx, pageID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrButtonNotFound
	}
	document, err := ast.Parse(snapshot.AST)
	if err != nil {
		return nil, err
	}
	location, ok := ast.FindBlock(document, blockID)
	if !ok || location.Block.Type != ast.BlockButton {
		return nil, ErrButtonNotFound
	}
	if actionIndex >= len(location.Block.Actions) {
		return nil, ErrActionNotFound
	}
	action := location.Block.Actions[actionIndex]
	if action.Type != ast.ButtonActionWebhook ||
		len(action.Payload) == 0 ||
		len(action.Payload) > maxWebhookPayloadBytes {
		return nil, ErrInvalidWebhook
	}
	target, err := url.Parse(action.URL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" ||
		target.User != nil {
		return nil, ErrInvalidWebhook
	}

	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, target.String(), bytes.NewReader(action.Payload),
	)
	if err != nil {
		return nil, ErrInvalidWebhook
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", executionID.String())
	client := s.publicHTTPSClient()
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebhookRejected, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status=%d", ErrWebhookRejected, response.StatusCode)
	}
	return &WebhookResult{ExecutionID: executionID, StatusCode: response.StatusCode}, nil
}

func (s *Service) publicHTTPSClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = s.dialPublicAddress
	return &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || request.URL.Scheme != "https" ||
				request.URL.Hostname() == "" || request.URL.User != nil {
				return ErrInvalidWebhook
			}
			return nil
		},
	}
}

func (s *Service) dialPublicAddress(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidWebhook
	}
	addresses, err := s.lookupHost(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, ErrInvalidWebhook
	}
	for _, ip := range addresses {
		if !isPublicIP(ip) {
			return nil, ErrInvalidWebhook
		}
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}

func (s *Service) lookupHost(ctx context.Context, host string) ([]net.IP, error) {
	if parsed := net.ParseIP(strings.Trim(host, "[]")); parsed != nil {
		return []net.IP{parsed}, nil
	}
	resolved, err := s.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	result := make([]net.IP, 0, len(resolved))
	for _, address := range resolved {
		result = append(result, address.IP)
	}
	return result, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil &&
		!ip.IsUnspecified() &&
		!ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast()
}
