package products

import (
	"strings"
	"testing"
)

func TestValidateProductName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"alpha", "acme", false},
		{"alphanumeric", "acme123", false},
		{"hyphen", "acme-corp", false},
		{"underscore", "acme_corp", false},
		{"period", "acme.io", false},
		{"period multiple", "github.com.acme", false},
		{"all allowed mixed", "Acme_Corp-v1.2", false},
		{"single char", "a", false},
		{"max length", strings.Repeat("a", 64), false},

		{"empty", "", true},
		{"too long", strings.Repeat("a", 65), true},
		{"space", "acme corp", true},
		{"slash", "acme/corp", true},
		{"at sign", "acme@corp", true},
		{"unicode emoji", "acme✨", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateProductName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("validateProductName(%q): want error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateProductName(%q): want nil, got %v", tc.input, err)
			}
		})
	}
}
