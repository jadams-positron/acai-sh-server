package auth

import (
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

// Scope is the auth carrier threaded through the request lifecycle. nil User
// means anonymous; non-nil means authenticated.
type Scope struct {
	User *accounts.User
}

// IsAuthenticated reports whether the scope has a user attached.
func (s *Scope) IsAuthenticated() bool { return s != nil && s.User != nil }
