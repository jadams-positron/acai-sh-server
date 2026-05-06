package testfx_test

import (
	"context"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestSeedUser_InsertsRow(t *testing.T) {
	db := testfx.NewDB(t)
	u := testfx.SeedUser(t, db, testfx.SeedUserOpts{Email: "alice@example.com"})
	if u == nil {
		t.Fatal("SeedUser returned nil")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", u.Email)
	}
	if u.ID == "" {
		t.Error("ID is empty")
	}
	// Confirm the row is in the DB.
	var count int
	if err := db.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM users WHERE id = ?", u.ID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}
}

func TestSeedTeam_InsertsRow(t *testing.T) {
	db := testfx.NewDB(t)
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{Name: "myteam"})
	if team == nil {
		t.Fatal("SeedTeam returned nil")
	}
	if team.Name != "myteam" {
		t.Errorf("Name = %q, want myteam", team.Name)
	}
}

func TestSeedAccessToken_ReturnsPlaintext(t *testing.T) {
	db := testfx.NewDB(t)
	user := testfx.SeedUser(t, db, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{})
	tok, plaintext := testfx.SeedAccessToken(t, db, user, team, testfx.SeedAccessTokenOpts{})
	if tok == nil {
		t.Fatal("SeedAccessToken returned nil token")
	}
	if plaintext == "" {
		t.Fatal("SeedAccessToken returned empty plaintext")
	}
	if tok.TeamID != team.ID {
		t.Errorf("TeamID = %q, want %q", tok.TeamID, team.ID)
	}
}

func TestSeedProduct_InsertsRow(t *testing.T) {
	db := testfx.NewDB(t)
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{})
	prod := testfx.SeedProduct(t, db, team, testfx.SeedProductOpts{Name: "widget"})
	if prod == nil {
		t.Fatal("SeedProduct returned nil")
	}
	if prod.Name != "widget" {
		t.Errorf("Name = %q, want widget", prod.Name)
	}
	var count int
	if err := db.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM products WHERE id = ?", prod.ID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}
}

func TestSeedImplementation_InsertsRow(t *testing.T) {
	db := testfx.NewDB(t)
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{})
	prod := testfx.SeedProduct(t, db, team, testfx.SeedProductOpts{})
	impl := testfx.SeedImplementation(t, db, prod, testfx.SeedImplementationOpts{Name: "production"})
	if impl == nil {
		t.Fatal("SeedImplementation returned nil")
	}
	if impl.Name != "production" {
		t.Errorf("Name = %q, want production", impl.Name)
	}
	var count int
	if err := db.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM implementations WHERE id = ?", impl.ID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}
}

func TestSeedBranchAndTrackedBranch(t *testing.T) {
	db := testfx.NewDB(t)
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{})
	prod := testfx.SeedProduct(t, db, team, testfx.SeedProductOpts{})
	impl := testfx.SeedImplementation(t, db, prod, testfx.SeedImplementationOpts{})
	branch := testfx.SeedBranch(t, db, team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/test/repo",
		BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, db, impl, branch)

	var count int
	if err := db.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM tracked_branches WHERE implementation_id = ? AND branch_id = ?",
		impl.ID, branch.ID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("tracked_branches row count = %d, want 1", count)
	}
}
