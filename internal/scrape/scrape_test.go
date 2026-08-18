package scrape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/briancbarrow/uysa-standings/internal/model"
)

// Counts captured from the fixtures on 2026-08-18, before the season opened.
// Team counts are fixed for the season; game counts only grow if the league
// adds fixtures, so a mismatch is a signal worth looking at, not a silent pass.
var fixtures = []struct {
	slug  string
	name  string
	teams int
	games int
}{
	{"premier", "Boys 13U Premier", 9, 36},
	{"division3", "Boys 13U Division 3", 11, 55},
	{"metroa", "Boys 13U Metro A", 9, 36},
	{"metrob", "Boys 13U Metro B", 10, 45},
}

func load(t *testing.T, slug string) model.Flight {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", slug+".html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	ref := model.FlightRef{Slug: slug, GUID: "TEST-GUID"}
	for _, f := range model.Flights {
		if f.Slug == slug {
			ref = f
		}
	}
	fl, err := Parse(ref, b)
	if err != nil {
		t.Fatalf("parse %s: %v", slug, err)
	}
	return fl
}

func TestParseFixtures(t *testing.T) {
	for _, fx := range fixtures {
		t.Run(fx.slug, func(t *testing.T) {
			f := load(t, fx.slug)
			if f.Name != fx.name {
				t.Errorf("name = %q, want %q", f.Name, fx.name)
			}
			if got := len(f.Standings); got != fx.teams {
				t.Errorf("standings rows = %d, want %d", got, fx.teams)
			}
			if got := len(f.Games); got != fx.games {
				t.Errorf("games = %d, want %d", got, fx.games)
			}
			for _, s := range f.Standings {
				if s.Team == "" || s.TeamCode == "" || s.Slot == "" {
					t.Errorf("incomplete standing: %+v", s)
				}
			}
			for _, g := range f.Games {
				if g.Number == "" || g.Home == "" || g.Away == "" {
					t.Errorf("incomplete game: %+v", g)
				}
				if g.Kickoff.IsZero() {
					t.Errorf("game %s has zero kickoff", g.Number)
				}
			}
		})
	}
}

// TestGameNumbersUnique guards the assumption that game number is a usable
// primary key across every flight.
func TestGameNumbersUnique(t *testing.T) {
	seen := map[string]string{}
	for _, fx := range fixtures {
		for _, g := range load(t, fx.slug).Games {
			if prev, dup := seen[g.Number]; dup {
				t.Errorf("game %s appears in both %s and %s", g.Number, prev, fx.slug)
			}
			seen[g.Number] = fx.slug
		}
	}
	if len(seen) != 36+55+36+45 {
		t.Errorf("total games = %d, want 172", len(seen))
	}
}

func TestKnownGame(t *testing.T) {
	var got *model.Game
	for _, g := range load(t, "premier").Games {
		if g.Number == "672226" {
			got = &g
			break
		}
	}
	if got == nil {
		t.Fatal("game 672226 not found")
	}
	if got.Venue != "Orem JH" {
		t.Errorf("venue = %q, want %q", got.Venue, "Orem JH")
	}
	if got.Field != "5262" {
		t.Errorf("field = %q, want %q", got.Field, "5262")
	}
	if got.Home != "Utah Rush B13 LM" {
		t.Errorf("home = %q", got.Home)
	}
	if got.Away != "La Roca U13B- C Santos ECNL RL" {
		t.Errorf("away = %q", got.Away)
	}
	if got.Matchup != "A1 vs A2" {
		t.Errorf("matchup = %q", got.Matchup)
	}
	if y, m, d := got.Kickoff.Date(); y != 2026 || m != 8 || d != 22 {
		t.Errorf("kickoff date = %v, want 2026-08-22", got.Kickoff)
	}
	if h, min := got.Kickoff.Hour(), got.Kickoff.Minute(); h != 8 || min != 30 {
		t.Errorf("kickoff clock = %02d:%02d, want 08:30", h, min)
	}
	if got.Played() {
		t.Errorf("game should be unplayed, got %v-%v", got.HomeScore, got.AwayScore)
	}
}

// TestUnplayedScoresAreNil pins the nil-vs-zero distinction: pre-season every
// score is blank, and blank must not decay into 0-0.
func TestUnplayedScoresAreNil(t *testing.T) {
	for _, fx := range fixtures {
		for _, g := range load(t, fx.slug).Games {
			if g.HomeScore != nil || g.AwayScore != nil {
				t.Errorf("%s game %s: expected nil scores pre-season, got %v-%v",
					fx.slug, g.Number, g.HomeScore, g.AwayScore)
			}
		}
	}
}

func TestStandingsZeroedPreSeason(t *testing.T) {
	for _, s := range load(t, "premier").Standings {
		if s.Wins != 0 || s.Losses != 0 || s.Ties != 0 || s.TotalPoints != 0 {
			t.Errorf("expected zeroed pre-season row, got %+v", s)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	_, err := Parse(model.FlightRef{Slug: "x"}, []byte("<html><body><p>nope</p></body></html>"))
	if err == nil {
		t.Fatal("expected an error parsing a page with no flight title")
	}
}
