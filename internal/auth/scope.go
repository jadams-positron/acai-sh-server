package auth

import (
	"context"

	"github.com/acai-sh/server/internal/domain/accounts"
)

// Scope is the auth carrier threaded through the request lifecycle. nil User
// means anonymous; non-nil means authenticated.
type Scope struct {
	User *accounts.User
}

// IsAuthenticated reports whether the scope has a user attached.
func (s *Scope) IsAuthenticated() bool { return s != nil && s.User != nil }

type scopeKey struct{}

// WithScope returns a derived ctx carrying scope.
func WithScope(ctx context.Context, scope *Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFrom returns the scope from ctx, or an anonymous scope.
func ScopeFrom(ctx context.Context) *Scope {
	if s, ok := ctx.Value(scopeKey{}).(*Scope); ok && s != nil {
		return s
	}
	return &Scope{}
}
