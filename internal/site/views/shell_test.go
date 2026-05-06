package views_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// renderShell renders a Shell with no children to a string for substring assertions.
func renderShell(t *testing.T, p views.ShellProps) string {
	t.Helper()
	var buf bytes.Buffer
	if err := views.Shell(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Shell render: %v", err)
	}
	return buf.String()
}

func TestShell_TopbarShowsUserChipAndTeamSwitcher(t *testing.T) {
	t.Parallel()
	t1 := &teams.Team{ID: "t1", Name: "acme"}
	t2 := &teams.Team{ID: "t2", Name: "globex"}

	out := renderShell(t, views.ShellProps{
		Title:           "Page",
		UserEmail:       "user@example.com",
		Teams:           []*teams.Team{t1, t2},
		ActiveTeam:      t1,
		LogoutCSRFToken: "csrf-token-abc",
	})

	for _, want := range []string{
		`<title>Page — Acai</title>`,
		`user@example.com`,
		`<form method="post" action="/users/log-out">`,
		`name="gorilla.csrf.Token" value="csrf-token-abc"`,
		`<select`,
		`value="/t/acme" selected>acme`,
		`value="/t/globex">globex`,
		`value="/teams">— All teams —`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Shell output missing %q; got:\n%s", want, out)
		}
	}
}

func TestShell_NoActiveTeam_ShowsOnlyYourTeams(t *testing.T) {
	t.Parallel()
	out := renderShell(t, views.ShellProps{
		Title:      "Teams",
		UserEmail:  "user@example.com",
		Teams:      []*teams.Team{{ID: "t1", Name: "acme"}},
		ActiveTeam: nil,
	})

	if !strings.Contains(out, `class="nav-item is-active" href="/teams">Your teams`) {
		t.Errorf(`expected highlighted "Your teams" nav-item; got:\n%s`, out)
	}
	for _, notWanted := range []string{
		`Settings`,
		`Tokens`,
	} {
		if strings.Contains(out, notWanted) {
			t.Errorf("Shell with no ActiveTeam should not render %q; got:\n%s", notWanted, out)
		}
	}
}

func TestShell_ActiveTeam_SidebarHasFullNav(t *testing.T) {
	t.Parallel()
	team := &teams.Team{ID: "t1", Name: "acme"}
	out := renderShell(t, views.ShellProps{
		Title:         "Acme",
		Teams:         []*teams.Team{team},
		ActiveTeam:    team,
		ActiveSection: "tokens",
	})

	for _, want := range []string{
		`href="/t/acme">Overview`,
		`href="/t/acme/features">Features`,
		`href="/t/acme/implementations">Implementations`,
		`href="/t/acme/settings">Settings`,
		`is-active" href="/t/acme/tokens">Tokens`, // active highlight
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Shell sidebar missing %q; got:\n%s", want, out)
		}
	}
}

func TestShell_ImplementationsSectionHighlights(t *testing.T) {
	t.Parallel()
	team := &teams.Team{ID: "t1", Name: "acme"}
	out := renderShell(t, views.ShellProps{
		Title:         "Implementations",
		Teams:         []*teams.Team{team},
		ActiveTeam:    team,
		ActiveSection: "implementations",
	})

	if !strings.Contains(out, `is-active" href="/t/acme/implementations">Implementations`) {
		t.Errorf("expected Implementations nav-item to be active; got:\n%s", out)
	}
}

func TestShell_FeaturesSectionHighlights(t *testing.T) {
	t.Parallel()
	team := &teams.Team{ID: "t1", Name: "acme"}
	out := renderShell(t, views.ShellProps{
		Title:         "Features",
		Teams:         []*teams.Team{team},
		ActiveTeam:    team,
		ActiveSection: "features",
	})

	if !strings.Contains(out, `is-active" href="/t/acme/features">Features`) {
		t.Errorf("expected Features nav-item to be active; got:\n%s", out)
	}
}

func TestShell_NoTeams_ShowsCreateTeamCTA(t *testing.T) {
	t.Parallel()
	out := renderShell(t, views.ShellProps{
		Title:     "Teams",
		UserEmail: "user@example.com",
		Teams:     nil,
	})

	if !strings.Contains(out, `href="/teams"`) {
		t.Errorf("expected link to /teams; got:\n%s", out)
	}
	if !strings.Contains(out, `+ Create team`) {
		t.Errorf("expected create-team CTA; got:\n%s", out)
	}
}

func TestBreadcrumbs_RenderTrail(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := views.Breadcrumbs([]views.Crumb{
		{Label: "Teams", HRef: "/teams"},
		{Label: "acme", HRef: "/t/acme"},
		{Label: "ledger"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Breadcrumbs render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<a href="/teams">Teams</a>`,
		`<a href="/t/acme">acme</a>`,
		`<span class="breadcrumbs-current">ledger</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Breadcrumbs missing %q; got:\n%s", want, out)
		}
	}
	// Two separators expected (between three crumbs).
	if got := strings.Count(out, `class="breadcrumbs-sep"`); got != 2 {
		t.Errorf("expected 2 separators, got %d; out:\n%s", got, out)
	}
}

func TestBreadcrumbs_LastIsAlwaysCurrentEvenIfHrefSet(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := views.Breadcrumbs([]views.Crumb{
		{Label: "A", HRef: "/a"},
		{Label: "B", HRef: "/b"}, // last crumb — HRef should be ignored
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Breadcrumbs render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `<span class="breadcrumbs-current">B</span>`) {
		t.Errorf("expected last crumb rendered as current span; got:\n%s", out)
	}
	if strings.Contains(out, `<a href="/b">B</a>`) {
		t.Errorf("last crumb should not be a link; got:\n%s", out)
	}
}
