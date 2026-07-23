package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

const DevActorHeader = "X-Actor-ID"

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
	if cookie, cookieErr := request.Cookie(a.sessionCookie); cookieErr == nil {
		principal, err = a.service.ResolveSession(ctx, cookie.Value)
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
	principal, err = a.service.ResolveDevActor(ctx, actorID)
	if errors.Is(err, ErrUnauthenticated) {
		return Principal{}, false, nil
	}
	return principal, err == nil, err
}
