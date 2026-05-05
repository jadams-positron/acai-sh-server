package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// goldenPath returns the absolute path to testdata/openapi.golden.json,
// resolved relative to the repo root regardless of which package is running.
func goldenPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/api/openapi_golden_test.go -> repo root is two levels up.
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(repoRoot, "testdata", "openapi.golden.json")
}

func TestOpenAPIGolden_FileExistsAndIsValidJSON(t *testing.T) {
	path := goldenPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func TestOpenAPIGolden_HasExpectedPaths(t *testing.T) {
	path := goldenPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	want := []string{
		"/feature-context",
		"/feature-states",
		"/implementation-features",
		"/implementations",
		"/push",
	}

	got := make([]string, 0, len(doc.Paths))
	for k := range doc.Paths {
		got = append(got, k)
	}
	sort.Strings(got)

	for _, p := range want {
		if !contains(got, p) {
			t.Errorf("openapi.golden.json is missing required path %q (have %v)", p, got)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
