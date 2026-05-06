package testfx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// goldenTB is the minimal testing interface that AssertJSONGoldenWith requires.
// *testing.T satisfies it; tests can substitute a lightweight double.
type goldenTB interface {
	Helper()
	Fatalf(format string, args ...any)
	Fatal(args ...any)
	Logf(format string, args ...any)
	Errorf(format string, args ...any)
}

// AssertJSONGoldenWith is the interface-based variant of AssertJSONGolden.
// It accepts any value implementing goldenTB (including *testing.T and test
// doubles). UPDATE_GOLDEN is read directly from os.Getenv.
func AssertJSONGoldenWith(t goldenTB, relPath string, got any) {
	t.Helper()
	gotJSON, err := canonicalJSON(got)
	if err != nil {
		t.Fatalf("testfx.AssertJSONGoldenWith: marshal got: %v", err)
		return
	}

	path := goldenFilePathFrom(relPath)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("testfx.AssertJSONGoldenWith: mkdir: %v", err)
			return
		}
		if err := os.WriteFile(path, append(gotJSON, '\n'), 0o644); err != nil {
			t.Fatalf("testfx.AssertJSONGoldenWith: write: %v", err)
			return
		}
		t.Logf("testfx: golden updated at %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from compile-time Caller(0) + caller-supplied relPath, not user input
	if err != nil {
		t.Fatalf("testfx.AssertJSONGoldenWith: read %s: %v (set UPDATE_GOLDEN=1 to create)", path, err)
		return
	}
	want = bytes.TrimRight(want, "\n")
	if !bytes.Equal(gotJSON, want) {
		t.Errorf("testfx.AssertJSONGoldenWith: %s mismatch\n--- want ---\n%s\n--- got ---\n%s", path, want, gotJSON)
	}
}

// AssertJSONGolden compares got (must be JSON-marshallable) against the
// canonical JSON file at testdata/golden/<relPath>. The file is overwritten
// when UPDATE_GOLDEN=1 in the env, making it easy to refresh expectations
// after a deliberate change.
//
// Comparison is byte-equal after json.Marshal+json.Indent normalization, so
// whitespace/key-order differences don't cause noise.
func AssertJSONGolden(t *testing.T, relPath string, got any) {
	t.Helper()
	gotJSON, err := canonicalJSON(got)
	if err != nil {
		t.Fatalf("testfx.AssertJSONGolden: marshal got: %v", err)
	}

	path := goldenFilePath(t, relPath)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("testfx.AssertJSONGolden: mkdir: %v", err)
		}
		if err := os.WriteFile(path, append(gotJSON, '\n'), 0o644); err != nil {
			t.Fatalf("testfx.AssertJSONGolden: write: %v", err)
		}
		t.Logf("testfx: golden updated at %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from compile-time Caller(0) + caller-supplied relPath, not user input
	if err != nil {
		t.Fatalf("testfx.AssertJSONGolden: read %s: %v (set UPDATE_GOLDEN=1 to create)", path, err)
	}
	want = bytes.TrimRight(want, "\n")
	if !bytes.Equal(gotJSON, want) {
		t.Errorf("testfx.AssertJSONGolden: %s mismatch\n--- want ---\n%s\n--- got ---\n%s", path, want, gotJSON)
	}
}

func canonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var canon any
	if err := json.Unmarshal(raw, &canon); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(canon, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// goldenFilePath resolves <repo-root>/testdata/golden/<relPath>.
// Named goldenFilePath to avoid collision with the api_test package's
// local goldenPath helper.
func goldenFilePath(t *testing.T, relPath string) string {
	t.Helper()
	return goldenFilePathFrom(relPath)
}

// GoldenFilePathExported is the exported version of goldenFilePathFrom for use
// in test cleanup helpers that need the absolute path.
func GoldenFilePathExported(relPath string) string { return goldenFilePathFrom(relPath) }

// goldenFilePathFrom resolves <repo-root>/testdata/golden/<relPath> without
// requiring a *testing.T. Used by AssertJSONGoldenWith.
func goldenFilePathFrom(relPath string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testfx.goldenFilePathFrom: runtime.Caller failed")
	}
	// internal/testfx/golden.go → repo root is two levels up.
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(repoRoot, "testdata", "golden", relPath)
}
