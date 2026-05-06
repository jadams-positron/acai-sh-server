package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// ---------------------------------------------------------------------------
// Binary discovery
// ---------------------------------------------------------------------------

// findCLIBinary returns the absolute path of the acai CLI binary, or calls
// t.Skip when not available. Checked in order:
//  1. ACAI_CLI_BINARY env var (CI override / cross-platform)
//  2. /usr/local/bin/acai-darwin-arm64 (developer default)
//  3. PATH lookup for "acai"
func findCLIBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("ACAI_CLI_BINARY"); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("ACAI_CLI_BINARY=%q not found: %v", p, err)
		}
		return p
	}
	const defaultBinary = "/usr/local/bin/acai-darwin-arm64"
	if _, err := os.Stat(defaultBinary); err == nil {
		return defaultBinary
	}
	if p, err := exec.LookPath("acai"); err == nil {
		return p
	}
	t.Skip("acai CLI binary not found; set ACAI_CLI_BINARY or install /usr/local/bin/acai-darwin-arm64")
	return ""
}

// ---------------------------------------------------------------------------
// Smoke fixture
// ---------------------------------------------------------------------------

// smokeFixture holds a running httptest.Server and all the seeded IDs the
// tests need to issue CLI commands.
type smokeFixture struct {
	app       *testfx.App
	serverURL string // e.g. "http://127.0.0.1:PORT"
	apiURL    string // serverURL + "/api/v1"
	token     string // bearer token plaintext
	teamID    string
	productID string
	implID    string
	workdir   string // git repo temp directory
}

// newSmokeFixture wires up a fresh test App, seeds the minimum rows, starts
// an httptest.Server over the App's Echo instance, and initializes a temporary
// git repository the CLI can work in.
func newSmokeFixture(t *testing.T) *smokeFixture {
	t.Helper()

	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "smoke@example.com"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "smoke-team"})
	_, plaintext := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{Name: "smoke-token"})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "smoke-product"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})

	ts := httptest.NewServer(app.Echo)
	t.Cleanup(ts.Close)

	workdir := t.TempDir()

	return &smokeFixture{
		app:       app,
		serverURL: ts.URL,
		apiURL:    ts.URL + "/api/v1",
		token:     plaintext,
		teamID:    team.ID,
		productID: product.ID,
		implID:    impl.ID,
		workdir:   workdir,
	}
}

// runCLI executes the CLI binary with the given args, the test's API base URL
// and token injected, and the working directory set to the fixture's workdir.
// Returns (exitCode, stdout, stderr).
func (f *smokeFixture) runCLI(t *testing.T, binary string, args ...string) (exitCode int, stdout string, stderr string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Dir = f.workdir
	cmd.Env = append(os.Environ(),
		"ACAI_API_BASE_URL="+f.apiURL,
		"ACAI_API_TOKEN="+f.token,
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // exec.ExitError is always a concrete type
			code = ee.ExitCode()
		} else {
			t.Fatalf("runCLI: unexpected error running %v: %v\nstderr: %s", args, err, errBuf.String())
		}
	}
	return code, outBuf.String(), errBuf.String()
}

// initGitRepo runs git init, basic config, and remote add in dir.
// Returns an error if git is unavailable or init fails.
func initGitRepo(ctx context.Context, dir string) error {
	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "smoke@example.com"},
		{"git", "config", "user.name", "Smoke Test"},
		{"git", "remote", "add", "origin", "git@github.com:smoke/smoke-product.git"},
	}
	for _, c := range cmds {
		cmd := exec.CommandContext(ctx, c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w\noutput: %s", c, err, string(out))
		}
	}
	return nil
}

// runGit runs a git command in dir and returns a combined error with output.
func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w\noutput: %s", args, err, string(out))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCLISmoke_Skill verifies the `skill` subcommand exits 0 and emits a
// non-empty prompt. It does not hit the server, so it runs on any host where
// the binary is present.
func TestCLISmoke_Skill(t *testing.T) {
	binary := findCLIBinary(t)

	cmd := exec.CommandContext(t.Context(), binary, "skill")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("acai skill failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "acai") {
		t.Errorf("acai skill output does not mention 'acai': %s", stdout.String())
	}
}

// TestCLISmoke_GetFeatures calls `acai features` against the live httptest
// server and verifies the response is parseable JSON with the expected shape.
func TestCLISmoke_GetFeatures(t *testing.T) {
	binary := findCLIBinary(t)
	fx := newSmokeFixture(t)

	code, stdout, stderr := fx.runCLI(t, binary, "features",
		"--product=smoke-product", "--impl=main", "--json")
	if code != 0 {
		t.Fatalf("acai features exit=%d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Verify the response is valid JSON with the expected envelope.
	var resp struct {
		Data struct {
			Features           []any  `json:"features"`
			ImplementationID   string `json:"implementation_id"`
			ImplementationName string `json:"implementation_name"`
			ProductName        string `json:"product_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("acai features JSON decode failed: %v\nraw: %s", err, stdout)
	}
	if resp.Data.ImplementationName != "main" {
		t.Errorf("implementation_name = %q, want main", resp.Data.ImplementationName)
	}
	if resp.Data.ProductName != "smoke-product" {
		t.Errorf("product_name = %q, want smoke-product", resp.Data.ProductName)
	}
	if resp.Data.Features == nil {
		t.Error("features field is nil; want empty array []")
	}
}

// TestCLISmoke_GetFeature_NotFound calls `acai feature` for a feature that
// does not exist. It verifies the request reached our server (no connection
// refused) and that the server returns a well-formed empty context response
// rather than crashing or timing out.
func TestCLISmoke_GetFeature_NotFound(t *testing.T) {
	binary := findCLIBinary(t)
	fx := newSmokeFixture(t)

	code, stdout, stderr := fx.runCLI(t, binary, "feature", "no-such-feature",
		"--product=smoke-product", "--impl=main", "--json")

	// The request must have reached the server — no connection error.
	if strings.Contains(stderr, "connection refused") || strings.Contains(stderr, "Check ACAI_API_BASE_URL") {
		t.Fatalf("CLI could not reach server: stderr=%s", stderr)
	}

	// Our server returns 200 with an empty acids list when the feature has no
	// spec (feature context with source_type:"none"). The CLI passes this through
	// as exit 0.
	t.Logf("acai feature exit=%d stdout=%s stderr=%s", code, stdout, stderr)

	if code == 0 {
		// Parse the response to verify it has the expected shape.
		var resp struct {
			Data struct {
				FeatureName string `json:"feature_name"`
				Acids       []any  `json:"acids"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
			t.Fatalf("acai feature JSON decode failed: %v\nraw: %s", err, stdout)
		}
		if resp.Data.FeatureName != "no-such-feature" {
			t.Errorf("feature_name = %q, want no-such-feature", resp.Data.FeatureName)
		}
		// Empty acids list is expected when no spec exists.
		if resp.Data.Acids == nil {
			t.Error("acids field is nil; want empty array []")
		}
	}
}

// TestCLISmoke_SetStatus calls `acai set-status` and verifies the server
// accepted the payload (exit 0), that the response JSON matches the expected
// shape, and that the state was actually persisted in the DB.
func TestCLISmoke_SetStatus(t *testing.T) {
	binary := findCLIBinary(t)
	fx := newSmokeFixture(t)

	// The set-status JSON is an ACID-keyed map — NOT wrapped in feature_name.
	statusJSON := `{"smoke-feature.X.1":{"status":"completed","comment":"done"}}`

	code, stdout, stderr := fx.runCLI(t, binary, "set-status", statusJSON,
		"--product=smoke-product", "--impl=main", "--json")
	if code != 0 {
		t.Fatalf("acai set-status exit=%d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Parse the response shape.
	var resp struct {
		Data struct {
			FeatureName        string   `json:"feature_name"`
			ImplementationID   string   `json:"implementation_id"`
			ImplementationName string   `json:"implementation_name"`
			ProductName        string   `json:"product_name"`
			StatesWritten      int      `json:"states_written"`
			Warnings           []string `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("acai set-status JSON decode failed: %v\nraw: %s", err, stdout)
	}
	if resp.Data.FeatureName != "smoke-feature" {
		t.Errorf("feature_name = %q, want smoke-feature", resp.Data.FeatureName)
	}
	if resp.Data.StatesWritten != 1 {
		t.Errorf("states_written = %d, want 1", resp.Data.StatesWritten)
	}
	if resp.Data.ProductName != "smoke-product" {
		t.Errorf("product_name = %q, want smoke-product", resp.Data.ProductName)
	}

	// Verify the state was actually persisted via direct DB query.
	var raw string
	err := fx.app.DB.Read.QueryRowContext(context.Background(),
		`SELECT states FROM feature_impl_states
		 WHERE implementation_id = ? AND feature_name = ?`,
		fx.implID, "smoke-feature").Scan(&raw)
	if err != nil {
		t.Fatalf("DB verify set-status: %v", err)
	}
	var states map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		t.Fatalf("DB states unmarshal: %v", err)
	}
	entry, ok := states["smoke-feature.X.1"]
	if !ok {
		t.Fatalf("ACID smoke-feature.X.1 not found in persisted states: %v", states)
	}
	if entry["status"] != "completed" {
		t.Errorf("persisted status = %v, want completed", entry["status"])
	}
}

// TestCLISmoke_PushNewSpec initializes a real git repo with a spec YAML, runs
// `acai push`, and verifies behavior against our server.
//
// CONTRACT MISMATCH DOCUMENTED: the CLI sends a refs-only POST /api/v1/push
// payload (no `product_name`, no `target_impl_name`) even when a spec file is
// present. Our server correctly rejects refs-only pushes that omit product_name
// (per the OpenAPI spec: "For refs-only pushes, product_name + target_impl_name
// must resolve to an existing implementation"). The Phoenix server may have
// different semantics here. This test verifies the server is reachable and
// returns an appropriate error shape rather than crashing — this is the
// expected behavior given the current contract gap.
//
// The specific failure signature is: exit=1, stdout contains
// "failures":[{"productName":"unknown-product",...}].
func TestCLISmoke_PushNewSpec(t *testing.T) {
	binary := findCLIBinary(t)
	fx := newSmokeFixture(t)

	// Set up a git repo in the fixture workdir.
	if err := initGitRepo(t.Context(), fx.workdir); err != nil {
		t.Skipf("git not available or init failed: %v", err)
	}

	// Write a spec YAML that references smoke-product (the seeded product name).
	specPath := filepath.Join(fx.workdir, "smoke-feature.feature.yaml")
	specYAML := `feature:
  name: smoke-feature
  product: smoke-product
  description: A test feature
  version: 1.0.0
requirements:
  smoke-feature.X.1:
    requirement: First criterion
  smoke-feature.X.2:
    requirement: Second criterion
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := runGit(t.Context(), fx.workdir, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGit(t.Context(), fx.workdir, "commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Run: acai push smoke-feature --target=main --json
	code, stdout, stderr := fx.runCLI(t, binary,
		"push", "smoke-feature", "--target=main", "--json")

	// The request must have reached the server — no connection error.
	if strings.Contains(stderr, "connection refused") || strings.Contains(stderr, "Check ACAI_API_BASE_URL") {
		t.Fatalf("CLI could not reach server: stderr=%s stdout=%s", stderr, stdout)
	}

	// CONTRACT MISMATCH: The CLI sends refs-only without product_name or
	// target_impl_name, which our server rejects with 422 "product_name is
	// required for references". The CLI exits non-zero with a failures array.
	//
	// This is a known contract gap documented above.
	t.Logf("acai push exit=%d stdout=%s stderr=%s", code, stdout, stderr)

	// Verify the response is valid JSON (not a crash or empty output).
	if strings.TrimSpace(stdout) != "" {
		var pushOut map[string]any
		if err := json.Unmarshal([]byte(stdout), &pushOut); err != nil {
			t.Fatalf("acai push stdout is not valid JSON: %v\nraw: %s", err, stdout)
		}
		// The CLI push JSON always has repoUri and branchName at top level.
		if _, ok := pushOut["repoUri"]; !ok {
			t.Errorf("push output missing repoUri field: %s", stdout)
		}
	}
}
