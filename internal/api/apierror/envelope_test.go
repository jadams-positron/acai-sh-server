package apierror_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

func TestWriteAppError_ShapeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	apierror.WriteAppError(rec, http.StatusUnauthorized, "Token revoked", "")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}

	var doc struct {
		Errors apierror.AppError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Errors.Detail != "Token revoked" {
		t.Errorf("Detail = %q", doc.Errors.Detail)
	}
	if doc.Errors.Status != "UNAUTHORIZED" {
		t.Errorf("Status = %q, want UNAUTHORIZED (auto-derived)", doc.Errors.Status)
	}
}

func TestWriteValidationError_ArrayShape(t *testing.T) {
	rec := httptest.NewRecorder()
	apierror.WriteValidationError(rec, http.StatusUnprocessableEntity, []apierror.ValidationError{
		{Title: "Invalid value", Source: map[string]any{"pointer": "/specs/0/feature/name"}},
		{Title: "Required", Source: map[string]any{"pointer": "/repo_uri"}},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}

	var doc struct {
		Errors []apierror.ValidationError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Errors) != 2 {
		t.Errorf("got %d errors, want 2", len(doc.Errors))
	}
	if doc.Errors[0].Source["pointer"] != "/specs/0/feature/name" {
		t.Errorf("pointer = %v", doc.Errors[0].Source["pointer"])
	}
}

func TestStatusFromCode_KnownCodes(t *testing.T) {
	cases := map[int]string{
		400: "BAD_REQUEST",
		401: "UNAUTHORIZED",
		403: "FORBIDDEN",
		413: "PAYLOAD_TOO_LARGE",
		422: "UNPROCESSABLE_ENTITY",
		429: "TOO_MANY_REQUESTS",
	}
	for code, want := range cases {
		if got := apierror.StatusFromCode(code); got != want {
			t.Errorf("StatusFromCode(%d) = %q, want %q", code, got, want)
		}
	}
}
