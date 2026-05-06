package testfx_test

import (
	"os"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestAssertJSONGolden_MatchesExistingFile(t *testing.T) {
	// testdata/golden/testfx/sample.json contains {"hello":"world"}.
	testfx.AssertJSONGolden(t, "testfx/sample.json", map[string]string{"hello": "world"})
}

func TestAssertJSONGolden_MissingFile_Fails(t *testing.T) {
	// Use AssertJSONGoldenWith with a test double that captures Fatalf.
	fc := &failCapture{}
	testfx.AssertJSONGoldenWith(fc, "testfx/does-not-exist-file.json", map[string]string{"x": "y"})
	if !fc.failed {
		t.Error("expected AssertJSONGoldenWith to call Fatalf for a missing golden file, but it did not")
	}
}

func TestAssertJSONGolden_UpdateGolden_WritesAndPasses(t *testing.T) {
	// Write a temp golden file via UPDATE_GOLDEN=1, then verify it reads back.
	path := "testfx/update-test-golden.json"

	// Resolve the absolute path for cleanup.
	absPath := testfx.GoldenFilePathExported(path)
	t.Cleanup(func() { _ = os.Remove(absPath) })

	t.Setenv("UPDATE_GOLDEN", "1")
	payload := map[string]any{"answer": 42}
	testfx.AssertJSONGolden(t, path, payload) // should write without failing

	// Now verify the file was written and a subsequent call passes.
	t.Setenv("UPDATE_GOLDEN", "")
	testfx.AssertJSONGolden(t, path, payload) // should pass
}

// failCapture implements testfx.GoldenTB so we can verify that
// AssertJSONGoldenWith calls Fatalf when the golden file is missing.
type failCapture struct{ failed bool }

func (f *failCapture) Helper()                   {}
func (f *failCapture) Fatalf(_ string, _ ...any) { f.failed = true }
func (f *failCapture) Fatal(_ ...any)            { f.failed = true }
func (f *failCapture) Logf(_ string, _ ...any)   {}
func (f *failCapture) Errorf(_ string, _ ...any) {}
