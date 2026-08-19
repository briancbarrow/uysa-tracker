package render

import (
	"strings"
	"testing"
	"time"

	"github.com/briancbarrow/uysa-standings/internal/model"
)

func ptr(n int) *int { return &n }

func now() time.Time { return time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC) }

func sample() model.Site {
	base := now()
	return model.Site{
		GeneratedAt: base,
		Flights: []model.Flight{
			{
				Slug: "premier", Name: "Boys 13U Premier",
				Standings: []model.Standing{
					{Team: "Low Team", TotalPoints: 3, GoalDiff: 0},
					{Team: "Top Team", TotalPoints: 9, GoalDiff: 5},
					{Team: "Mid Team", TotalPoints: 9, GoalDiff: 1},
				},
				Games: []model.Game{
					{Number: "1", FlightSlug: "premier", Kickoff: base.Add(-48 * time.Hour),
						Home: "Winner FC", Away: "Loser FC", HomeScore: ptr(3), AwayScore: ptr(1)},
					{Number: "2", FlightSlug: "premier", Kickoff: base.Add(2 * time.Hour),
						Home: "Premier Today A", Away: "Premier Today B"},
					{Number: "3", FlightSlug: "premier", Kickoff: base.Add(72 * time.Hour),
						Home: "Future A", Away: "Future B"},
					{Number: "4", FlightSlug: "premier", Kickoff: base.Add(-30 * 24 * time.Hour),
						Home: "Ancient A", Away: "Ancient B", HomeScore: ptr(0), AwayScore: ptr(0)},
				},
			},
			{
				Slug: "metroa", Name: "Boys 13U Metro A",
				Standings: []model.Standing{{Team: "Metro Team", TotalPoints: 4}},
				Games: []model.Game{
					{Number: "9", FlightSlug: "metroa", Kickoff: base.Add(3 * time.Hour),
						Home: "Metro Today A", Away: "Metro Today B"},
				},
			},
		},
	}
}

func flight(v View, slug string) FlightView {
	for _, f := range v.Flights {
		if f.Slug == slug {
			return f
		}
	}
	panic("flight not in view: " + slug)
}

func TestBucketsArePerFlight(t *testing.T) {
	v := BuildView(sample(), now())

	p := flight(v, "premier")
	if len(p.Today) != 1 || p.Today[0].Number != "2" {
		t.Errorf("premier Today = %+v, want just game 2", p.Today)
	}
	if len(p.Recent) != 1 || p.Recent[0].Number != "1" {
		t.Errorf("premier Recent = %+v, want just game 1", p.Recent)
	}
	if len(p.Upcoming) != 1 || p.Upcoming[0].Number != "3" {
		t.Errorf("premier Upcoming = %+v, want just game 3", p.Upcoming)
	}

	m := flight(v, "metroa")
	if len(m.Today) != 1 || m.Today[0].Number != "9" {
		t.Errorf("metroa Today = %+v, want just game 9", m.Today)
	}
	if len(m.Recent) != 0 || len(m.Upcoming) != 0 {
		t.Errorf("metroa should have no recent/upcoming, got %+v / %+v", m.Recent, m.Upcoming)
	}
	if v.SeasonSoon {
		t.Error("SeasonSoon should be false once results exist")
	}
}

// TestGamesStayInTheirFlight is the point of the per-flight layout: a game must
// never be rendered under a flight it does not belong to.
func TestGamesStayInTheirFlight(t *testing.T) {
	v := BuildView(sample(), now())
	for _, f := range v.Flights {
		for _, bucket := range [][]model.Game{f.Today, f.Recent, f.Upcoming} {
			for _, g := range bucket {
				if g.FlightSlug != f.Slug {
					t.Errorf("game %s (flight %s) leaked into flight %s", g.Number, g.FlightSlug, f.Slug)
				}
			}
		}
	}
}

func TestMetroADefaultsOpen(t *testing.T) {
	v := BuildView(sample(), now())
	for _, f := range v.Flights {
		want := f.Slug == DefaultOpen
		if f.Open != want {
			t.Errorf("flight %s Open = %v, want %v", f.Slug, f.Open, want)
		}
	}

	var sb strings.Builder
	if err := Page(&sb, v); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if n := strings.Count(out, "<details open>"); n != 1 {
		t.Errorf("found %d open <details>, want exactly 1", n)
	}
	openAt := strings.Index(out, "<details open>")
	if metro := strings.Index(out, "Boys 13U Metro A"); metro < openAt {
		t.Error("the open <details> is not the Metro A card")
	}
}

// TestNoScheduleOutsideCards guards the requirement that no schedule shows
// unless a flight card is opened: every game card must sit inside a <details>.
func TestNoScheduleOutsideCards(t *testing.T) {
	var sb strings.Builder
	if err := Page(&sb, BuildView(sample(), now())); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	for _, team := range []string{"Premier Today A", "Metro Today A", "Winner FC", "Future A"} {
		at := strings.Index(out, team)
		if at < 0 {
			t.Errorf("%q missing from page", team)
			continue
		}
		before := out[:at]
		if strings.Count(before, "<details") <= strings.Count(before, "</details>") {
			t.Errorf("%q renders outside any <details> card", team)
		}
	}
}

func TestStandingsSortedByPointsThenGD(t *testing.T) {
	got := []string{}
	for _, s := range flight(BuildView(sample(), now()), "premier").Standings {
		got = append(got, s.Team)
	}
	want := []string{"Top Team", "Mid Team", "Low Team"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("standings order = %v, want %v", got, want)
		}
	}
}

// TestPageRendersScores covers the path the pre-season fixtures cannot: a game
// with a posted score must show the score and mark the winner.
func TestPageRendersScores(t *testing.T) {
	var sb strings.Builder
	if err := Page(&sb, BuildView(sample(), now())); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "3 – 1") {
		t.Error("rendered page is missing the 3–1 score")
	}
	if !strings.Contains(out, `class="team win">Winner FC`) {
		t.Error("winning team is not marked with .win")
	}
	if strings.Contains(out, `class="team win">Loser FC`) {
		t.Error("losing team should not be marked as winner")
	}
}

func TestDrawHasNoWinner(t *testing.T) {
	g := model.Game{HomeScore: ptr(2), AwayScore: ptr(2)}
	if funcs["winner"].(func(model.Game, string) bool)(g, "home") {
		t.Error("a 2-2 draw should not mark a winner")
	}
}
