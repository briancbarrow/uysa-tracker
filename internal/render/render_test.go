package render

import (
	"strings"
	"testing"
	"time"

	"github.com/briancbarrow/uysa-standings/internal/model"
)

func ptr(n int) *int { return &n }

// favName is the display name behind FavoriteTeamCode.
const favName = "Copper Mountain 7 ZP/ZN"

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
				Standings: []model.Standing{
					{Team: "Metro Team", TotalPoints: 4},
					// The favorite, and a decoy that shares its name prefix.
					{Team: favName, TeamCode: FavoriteTeamCode, TotalPoints: 7},
					{Team: "Copper Mountain 7 BH", TeamCode: "0213-01CB13-1134", TotalPoints: 6},
				},
				Games: []model.Game{
					{Number: "9", FlightSlug: "metroa", Kickoff: base.Add(3 * time.Hour),
						Home: "Metro Today A", Away: "Metro Today B"},
					{Number: "10", FlightSlug: "metroa", Kickoff: base.Add(4 * time.Hour),
						Home: favName, Away: "Copper Mountain 7 BH"},
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
	if len(m.Today) != 2 {
		t.Errorf("metroa Today = %+v, want 2 games", m.Today)
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
		for _, bucket := range [][]GameCard{f.Today, f.Recent, f.Upcoming} {
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

// TestRefreshLinkRendered pins the manual-trigger link. Dispatching the
// workflow needs write access, so publishing this URL exposes nothing.
func TestRefreshLinkRendered(t *testing.T) {
	var sb strings.Builder
	if err := Page(&sb, BuildView(sample(), now())); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, RefreshURL) {
		t.Errorf("page is missing the refresh link %q", RefreshURL)
	}
	if strings.Contains(out, "workflow_dispatch") || strings.Contains(out, "ghp_") {
		t.Error("page must not embed API calls or tokens")
	}
}

// TestFavoriteTeamHighlighted covers the highlight end to end. The decoy
// matters: "Copper Mountain 7 BH" plays in the same flight, so a name-prefix
// match would light up the wrong row.
func TestFavoriteTeamHighlighted(t *testing.T) {
	v := BuildView(sample(), now())
	if v.FavoriteName != favName {
		t.Fatalf("FavoriteName = %q, want %q", v.FavoriteName, favName)
	}

	favRows := 0
	for _, f := range v.Flights {
		for _, s := range f.Standings {
			if s.Fav {
				favRows++
				if s.Team != favName {
					t.Errorf("highlighted the wrong team: %q", s.Team)
				}
			}
			if s.Team == "Copper Mountain 7 BH" && s.Fav {
				t.Error("decoy Copper Mountain 7 BH must not be highlighted")
			}
		}
	}
	if favRows != 1 {
		t.Errorf("highlighted %d standings rows, want exactly 1", favRows)
	}

	var sb strings.Builder
	if err := Page(&sb, v); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()
	if n := strings.Count(out, `<tr class="favrow">`); n != 1 {
		t.Errorf("rendered %d favrow rows, want 1", n)
	}
	if n := strings.Count(out, "card game favgame"); n != 1 {
		t.Errorf("rendered %d highlighted game cards, want 1", n)
	}
	if !strings.Contains(out, `class="team fav">`+favName) {
		t.Error("favorite side of the game card is not marked")
	}
	if strings.Contains(out, `class="team fav">Copper Mountain 7 BH`) {
		t.Error("decoy team marked as favorite inside a game card")
	}
}

// TestFavoriteDoesNotReorderStandings: highlighting must not move the team out
// of its real league position.
func TestFavoriteDoesNotReorderStandings(t *testing.T) {
	got := []string{}
	for _, s := range flight(BuildView(sample(), now()), "metroa").Standings {
		got = append(got, s.Team)
	}
	want := []string{favName, "Copper Mountain 7 BH", "Metro Team"} // 7, 6, 4 points
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("standings order = %v, want %v", got, want)
		}
	}
}

func TestDrawHasNoWinner(t *testing.T) {
	g := model.Game{HomeScore: ptr(2), AwayScore: ptr(2)}
	if funcs["winner"].(func(model.Game, string) bool)(g, "home") {
		t.Error("a 2-2 draw should not mark a winner")
	}
}
