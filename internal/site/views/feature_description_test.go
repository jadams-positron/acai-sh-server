package views_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// renderFeature renders FeatureShow with no impl cards so the body output is
// short enough for substring assertions about the description block.
func renderFeature(t *testing.T, description string) string {
	t.Helper()
	var buf bytes.Buffer
	team := &teams.Team{ID: "t1", Name: "acme"}
	props := views.FeatureShowProps{
		Shell:              views.ShellProps{Title: "feat", Teams: []*teams.Team{team}, ActiveTeam: team},
		Team:               team,
		FeatureName:        "feat",
		FeatureDescription: description,
	}
	if err := views.FeatureShow(props).Render(context.Background(), &buf); err != nil {
		t.Fatalf("FeatureShow render: %v", err)
	}
	return buf.String()
}

func TestFeatureShow_ShortDescription_RendersPlain(t *testing.T) {
	t.Parallel()
	short := "A pithy 30-char feature blurb."
	out := renderFeature(t, short)

	if !strings.Contains(out, `class="subtitle desc-plain">`+short+`</p>`) {
		t.Errorf("expected plain subtitle paragraph for short description; got:\n%.800s", out)
	}
	if strings.Contains(out, `<details class="desc-collapse"`) {
		t.Errorf("short description should not be wrapped in <details>; got:\n%.800s", out)
	}
}

func TestFeatureShow_LongDescription_CollapsesIntoDetails(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("This is a very long technical description that wraps repeatedly. ", 6) // ~390 chars
	out := renderFeature(t, long)

	for _, want := range []string{
		`<details class="desc-collapse">`,
		`<summary class="desc-collapse-summary">`,
		`class="subtitle desc-collapse-text">`,
		long, // the description text is rendered exactly once inside the summary
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%.800s", want, out)
		}
	}
	// Description appears exactly once — single source of truth.
	if got := strings.Count(out, long); got != 1 {
		t.Errorf("expected description to appear exactly once, got %d copies", got)
	}
}

func TestFeatureShow_NoDescription_RendersNeither(t *testing.T) {
	t.Parallel()
	out := renderFeature(t, "")

	for _, notWanted := range []string{
		`desc-plain`,
		`desc-collapse`,
	} {
		if strings.Contains(out, notWanted) {
			t.Errorf("empty description should render neither plain nor collapse; got %q in:\n%.800s", notWanted, out)
		}
	}
}
