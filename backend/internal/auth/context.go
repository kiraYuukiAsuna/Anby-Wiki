// Package auth owns external identity mapping, login transactions, server-side
// sessions, and the authenticated request context.
package auth

import (
	"context"

	"github.com/google/uuid"
)

type principalKey struct{}

// Principal is the authenticated identity for one request. Authorization data
// is deliberately absent so role and protection changes take effect immediately.
type Principal struct {
	ActorID     uuid.UUID
	ActorType   string
	DisplayName string
	Method      string
}

// WithPrincipal attaches an authenticated principal to a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// PrincipalFrom returns the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}
