// Package apierror implements the Acai API error envelope shapes.
//
// Two response forms are emitted, matching the Phoenix open_api_spex behavior
// the existing CLI parses:
//
//  1. App-level errors (auth, size, rate, business-logic):
//     {"errors":{"detail":"...","status":"UNAUTHORIZED"}}
//
//  2. Validation errors (request body / query failed schema validation):
//     {"errors":[{"title":"...","source":{"pointer":"/specs/0/feature/name"},...}]}
//
// Use WriteAppError for the first form; WriteValidationError for the second.
package apierror

import (
	"encoding/json"
	"net/http"
)

// AppError is the single-object error envelope.
type AppError struct {
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// ValidationError is one entry in the validation-error array.
type ValidationError struct {
	Title  string         `json:"title"`
	Source map[string]any `json:"source,omitempty"`
}

// WriteAppError writes the single-object envelope at the given HTTP status.
// status defaults to a SCREAMING_SNAKE_CASE label derived from code if empty.
func WriteAppError(w http.ResponseWriter, code int, detail, status string) {
	if status == "" {
		status = StatusFromCode(code)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": AppError{Detail: detail, Status: status},
	})
}

// WriteValidationError writes the array-of-validations envelope at the given
// HTTP status (typically 400 or 422).
func WriteValidationError(w http.ResponseWriter, code int, items []ValidationError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": items,
	})
}

// StatusFromCode returns the SCREAMING_SNAKE_CASE label Phoenix uses.
func StatusFromCode(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusUnprocessableEntity:
		return "UNPROCESSABLE_ENTITY"
	case http.StatusTooManyRequests:
		return "TOO_MANY_REQUESTS"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusInternalServerError:
		return "INTERNAL_SERVER_ERROR"
	default:
		return "ERROR"
	}
}
