package accounts

import "time"

// EmailToken is the in-memory representation of a one-time authentication
// token stored in the email_tokens table.
type EmailToken struct {
	ID         string
	UserID     string
	TokenHash  []byte
	Context    string
	SentTo     string
	InsertedAt time.Time
}
