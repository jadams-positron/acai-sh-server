package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// fsResp is a loose shape for decoding feature-states JSON responses.
type fsResp struct {
	Data struct {
		FeatureName        string   `json:"feature_name"`
		ImplementationId   string   `json:"implementation_id"` //nolint:staticcheck // ST1003: matches generated JSON tag
		ImplementationName string   `json:"implementation_name"`
		ProductName        string   `json:"product_name"`
		StatesWritten      int      `json:"states_written"`
		Warnings           []string `json:"warnings"`
	} `json:"data"`
}

// readStates reads back the raw states JSON map from the DB for (implID, featureName).
// It queries the DB directly so the returned map uses the on-disk JSON keys (lowercase).
func readStates(t *testing.T, db *store.DB, implID, featureName string) map[string]any {
	t.Helper()
	var rawStates string
	err := db.Read.QueryRowContext(
		context.Background(),
		"SELECT states FROM feature_impl_states WHERE implementation_id = ? AND feature_name = ?",
		implID, featureName,
	).Scan(&rawStates)
	if err != nil {
		t.Fatalf("readStates query: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(rawStates), &out); err != nil {
		t.Fatalf("readStates unmarshal: %v", err)
	}
	return out
}

// stateEntry aliases the anonymous struct from spec.FeatureStatesRequest.States.
type stateEntry = struct {
	Comment *string                                `json:"comment,omitempty"`
	Status  *spec.FeatureStatesRequestStatesStatus `json:"status"`
}

func TestFeatureStates_HappyPath_NewRow(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acme"})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})

	statusCompleted := spec.FeatureStatesRequestStatesStatusCompleted
	comment := "done"
	body := spec.FeatureStatesRequest{
		FeatureName:        "auth-feature",
		ImplementationName: "production",
		ProductName:        "myapp",
		States: map[string]stateEntry{
			"auth-feature.AUTH.1": {
				Status:  &statusCompleted,
				Comment: &comment,
			},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusOK)

	var doc fsResp
	resp.JSON(&doc)

	if doc.Data.StatesWritten != 1 {
		t.Errorf("states_written = %d, want 1", doc.Data.StatesWritten)
	}
	if doc.Data.FeatureName != "auth-feature" {
		t.Errorf("feature_name = %q, want auth-feature", doc.Data.FeatureName)
	}
	if doc.Data.ProductName != "myapp" {
		t.Errorf("product_name = %q, want myapp", doc.Data.ProductName)
	}
	if doc.Data.ImplementationName != "production" {
		t.Errorf("implementation_name = %q, want production", doc.Data.ImplementationName)
	}
	if doc.Data.ImplementationId == "" {
		t.Error("implementation_id should be non-empty")
	}
	if doc.Data.Warnings == nil {
		t.Error("warnings should be non-nil (empty slice)")
	}

	// Verify DB row.
	states := readStates(t, app.DB, impl.ID, "auth-feature")
	acid, ok := states["auth-feature.AUTH.1"]
	if !ok {
		t.Fatalf("acid auth-feature.AUTH.1 not found in DB states")
	}
	acidMap, _ := acid.(map[string]any)
	if acidMap["status"] != "completed" {
		t.Errorf("DB status = %v, want completed", acidMap["status"])
	}
	if acidMap["comment"] != "done" {
		t.Errorf("DB comment = %v, want done", acidMap["comment"])
	}
}

func TestFeatureStates_HappyPath_MergeExisting(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "mergeapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})

	// Seed existing states.
	testfx.SeedFeatureImplState(t, app.DB, impl, "feat", map[string]any{
		"feat.A.1": map[string]any{"status": "blocked"},
		"feat.A.2": map[string]any{"status": "completed"},
	})

	statusAccepted := spec.FeatureStatesRequestStatesStatusAccepted
	statusIncomplete := spec.FeatureStatesRequestStatesStatusIncomplete
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "mergeapp",
		States: map[string]stateEntry{
			"feat.A.2": {Status: &statusAccepted},
			"feat.A.3": {Status: &statusIncomplete},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusOK)

	var doc fsResp
	resp.JSON(&doc)
	if doc.Data.StatesWritten != 2 {
		t.Errorf("states_written = %d, want 2", doc.Data.StatesWritten)
	}

	// Verify merge: A.1 unchanged, A.2 updated, A.3 added.
	states := readStates(t, app.DB, impl.ID, "feat")

	a1, ok := states["feat.A.1"]
	if !ok {
		t.Fatal("feat.A.1 should still exist after merge")
	}
	if m, _ := a1.(map[string]any); m["status"] != "blocked" {
		t.Errorf("feat.A.1 status = %v, want blocked", m["status"])
	}

	a2, ok := states["feat.A.2"]
	if !ok {
		t.Fatal("feat.A.2 should exist after merge")
	}
	if m, _ := a2.(map[string]any); m["status"] != "accepted" {
		t.Errorf("feat.A.2 status = %v, want accepted", m["status"])
	}

	a3, ok := states["feat.A.3"]
	if !ok {
		t.Fatal("feat.A.3 should have been added")
	}
	if m, _ := a3.(map[string]any); m["status"] != "incomplete" {
		t.Errorf("feat.A.3 status = %v, want incomplete", m["status"])
	}
}

func TestFeatureStates_ClearACID_BothNil(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "clearapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})

	// Seed existing states with feat.A.1 = completed.
	testfx.SeedFeatureImplState(t, app.DB, impl, "feat", map[string]any{
		"feat.A.1": map[string]any{"status": "completed"},
	})

	// PATCH with both status and comment nil → clear feat.A.1.
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "clearapp",
		States: map[string]stateEntry{
			"feat.A.1": {Status: nil, Comment: nil},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusOK)

	var doc fsResp
	resp.JSON(&doc)
	if doc.Data.StatesWritten != 1 {
		t.Errorf("states_written = %d, want 1", doc.Data.StatesWritten)
	}

	// Verify feat.A.1 is gone from the DB.
	states := readStates(t, app.DB, impl.ID, "feat")
	if _, ok := states["feat.A.1"]; ok {
		t.Error("feat.A.1 should have been cleared from DB states")
	}
}

func TestFeatureStates_InvalidStatus_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "app"})

	bogus := spec.FeatureStatesRequestStatesStatus("bogus")
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "app",
		States: map[string]stateEntry{
			"feat.A.1": {Status: &bogus},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

func TestFeatureStates_TooManyStates_422(t *testing.T) {
	t.Setenv("API_FEATURE_STATES_MAX_STATES", "2")

	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "app"})

	statusCompleted := spec.FeatureStatesRequestStatesStatusCompleted
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "app",
		States: map[string]stateEntry{
			"feat.A.1": {Status: &statusCompleted},
			"feat.A.2": {Status: &statusCompleted},
			"feat.A.3": {Status: &statusCompleted},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

func TestFeatureStates_CommentTooLong_422(t *testing.T) {
	t.Setenv("API_FEATURE_STATES_MAX_COMMENT_LENGTH", "10")

	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "app"})

	statusCompleted := spec.FeatureStatesRequestStatesStatusCompleted
	longComment := "this comment is longer than 10 chars"
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "app",
		States: map[string]stateEntry{
			"feat.A.1": {
				Status:  &statusCompleted,
				Comment: &longComment,
			},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

func TestFeatureStates_ProductNotFound_404(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})

	statusCompleted := spec.FeatureStatesRequestStatesStatusCompleted
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "nonexistent",
		States: map[string]stateEntry{
			"feat.A.1": {Status: &statusCompleted},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusNotFound)
}

func TestFeatureStates_ImplementationNotFound_404(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "existingprod"})

	statusCompleted := spec.FeatureStatesRequestStatesStatusCompleted
	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "nonexistent",
		ProductName:        "existingprod",
		States: map[string]stateEntry{
			"feat.A.1": {Status: &statusCompleted},
		},
	}

	resp := app.Client().WithBearer(token).PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusNotFound)
}

func TestFeatureStates_NoBearer_401(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	body := spec.FeatureStatesRequest{
		FeatureName:        "feat",
		ImplementationName: "main",
		ProductName:        "myapp",
		States:             map[string]stateEntry{},
	}

	resp := app.Client().PATCHJSON("/api/v1/feature-states", body)
	resp.AssertStatus(http.StatusUnauthorized)
}

func TestFeatureStates_BadJSON_400(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})

	// Send raw non-JSON body.
	resp := app.Client().WithBearer(token).PATCHRaw(
		"/api/v1/feature-states",
		"application/json",
		strings.NewReader("not-valid-json{{{"),
	)
	resp.AssertStatus(http.StatusBadRequest)
}
