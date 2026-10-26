package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const (
	DevActorHeader    = "X-Actor-ID"
	WikiContextCookie = "anby_wiki_id"
	WikiContextHeader = "X-Wiki-ID"
)

// Authenticator resolves a session cookie, or an explicitly enabled
// development/test header. It never derives authorization permissions.
type Authenticator struct {
	service        *Service
	sessionCookie  string
	allowDevHeader bool
}

func NewAuthenticator(service *Service, sessionCookie string, allowDevHeader bool) *Authenticator {
	return &Authenticator{
		service:        service,
		sessionCookie:  sessionCookie,
		allowDevHeader: allowDevHeader,
	}
}

// Authenticate returns ok=false for an absent or invalid credential. Storage
// errors are returned so callers do not accidentally downgrade an outage to
// anonymous access.
func (a *Authenticator) Authenticate(ctx context.Context, request *http.Request) (principal Principal, ok bool, err error) {
	requestedWikiID, err := requestedWikiIDFrom(request)
	if err != nil {
		return Principal{}, false, nil
	}
	if cookie, cookieErr := request.Cookie(a.sessionCookie); cookieErr == nil {
		principal, err = a.service.ResolveSession(ctx, cookie.Value, requestedWikiID)
		if errors.Is(err, ErrUnauthenticated) {
			return Principal{}, false, nil
		}
		return principal, err == nil, err
	}
	if authorization := strings.TrimSpace(request.Header.Get("Authorization")); authorization != "" {
		scheme, token, found := strings.Cut(authorization, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") ||
			strings.TrimSpace(token) == "" {
			return Principal{}, false, nil
		}
		principal, err = a.service.ResolveCLIToken(ctx, strings.TrimSpace(token), requestedWikiID)
		if errors.Is(err, ErrUnauthenticated) {
			return Principal{}, false, nil
		}
		return principal, err == nil, err
	}
	if !a.allowDevHeader {
		return Principal{}, false, nil
	}
	rawActorID := request.Header.Get(DevActorHeader)
	if rawActorID == "" {
		return Principal{}, false, nil
	}
	actorID, parseErr := uuid.Parse(rawActorID)
	if parseErr != nil {
		return Principal{}, false, nil
	}
	principal, err = a.service.ResolveDevActor(ctx, actorID, requestedWikiID)
	if errors.Is(err, ErrUnauthenticated) {
		return Principal{}, false, nil
	}
	return principal, err == nil, err
}

func requestedWikiIDFrom(request *http.Request) (uuid.UUID, error) {
	raw := strings.TrimSpace(request.Header.Get(WikiContextHeader))
	if raw == "" {
		if cookie, err := request.Cookie(WikiContextCookie); err == nil {
			raw = strings.TrimSpace(cookie.Value)
		}
	}
	if raw == "" {
		return uuid.Nil, nil
	}
	wikiID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, err
	}
	return wikiID, nil
}
