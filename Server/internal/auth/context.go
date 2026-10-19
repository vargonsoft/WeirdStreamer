package auth

import (
	"context"

	"github.com/weirdstreamer/server/internal/models"
)

type contextKey string

const (
	claimsKey contextKey = "claims"
	userKey   contextKey = "user"
)

// WithClaims adds JWT claims to the context
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// GetClaims retrieves JWT claims from the context
func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}

// WithUser adds the authenticated user, loaded once per request, to the context
func WithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// GetUser retrieves the authenticated user from the context
func GetUser(ctx context.Context) *models.User {
	user, _ := ctx.Value(userKey).(*models.User)
	return user
}
