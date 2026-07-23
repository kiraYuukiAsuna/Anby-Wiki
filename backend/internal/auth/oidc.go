package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var ErrIdentityVerification = errors.New("auth: identity verification failed")

// OIDCConfig configures a standards-based Authorization Code + PKCE client.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// OIDCProvider abstracts protocol operations so handlers can be tested without
// an external identity provider.
type OIDCProvider interface {
	AuthorizationURL(state, nonce, codeChallenge string) string
	ExchangeAndVerify(ctx context.Context, code, codeVerifier, nonce string) (Identity, error)
}

type provider struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewOIDCProvider performs discovery and creates a generic OIDC client.
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig) (OIDCProvider, error) {
	discovered, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth: OIDC discovery failed: %w", err)
	}
	scopes := append([]string{oidc.ScopeOpenID}, cfg.Scopes...)
	return &provider{
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     discovered.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       uniqueStrings(scopes),
		},
		verifier: discovered.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

func (p *provider) AuthorizationURL(state, nonce, codeChallenge string) string {
	return p.oauth.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func (p *provider) ExchangeAndVerify(ctx context.Context, code, codeVerifier, nonce string) (Identity, error) {
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: token exchange", ErrIdentityVerification)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Identity{}, fmt.Errorf("%w: missing id_token", ErrIdentityVerification)
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: id_token", ErrIdentityVerification)
	}
	if idToken.Nonce != nonce {
		return Identity{}, fmt.Errorf("%w: nonce", ErrIdentityVerification)
	}
	var claims struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: claims", ErrIdentityVerification)
	}
	displayName := firstNonEmpty(claims.Name, claims.PreferredUsername, claims.Email)
	return Identity{
		Issuer:      idToken.Issuer,
		Subject:     idToken.Subject,
		DisplayName: displayName,
	}, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
