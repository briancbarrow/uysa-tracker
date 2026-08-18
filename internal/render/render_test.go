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
		Flights: []model.Flight{{
			Slug: "premier", Name: "Boys 13U Premier",
			Standings: []model.Standing{
				{Team: "Low Team", TotalPoints: 3, GoalDiff: 0},
				{Team: "Top Team", TotalPoints: 9, GoalDiff: 5},
				{Team: "Mid Team", TotalPoints: 9, GoalDiff: 1},
			},
			Games: []model.Game{
				{Number: "1", FlightName: "Boys 13U Premier", Kickoff: base.Add(-48 * time.Hour),
					Home: "Winner FC", Away: "Loser FC", HomeScore: ptr(3), AwayScore: ptr(1)},
				{Number: "2", FlightName: "Boys 13U Premier", Kickoff: base.Add(2 * time.Hour),
					Home: "Today A", Away: "Today B"},
				{Number: "3", FlightName: "Boys 13U Premier", Kickoff: base.Add(72 * time.Hour),
					Home: "Future A", Away: "Future B"},
				{Number: "4", FlightName: "Boys 13U Premier", Kickoff: base.Add(-30 * 24 * time.Hour),
					Home: "Ancient A", Away: "Ancient B", HomeScore: ptr(0), AwayScore: ptr(0)},
			},
		}},
	}
}

func TestBuildViewBuckets(t *testing.T) {
	v := BuildView(sample(), now())
	if len(v.Today) != 1 || v.Today[0].Number != "2" {
		t.Errorf("Today = %+v, want just game 2", v.Today)
	}
	if len(v.Recent) != 1 || v.Recent[0].Number != "1" {
		t.Errorf("Recent = %+v, want just game 1", v.Recent)
	}
	if len(v.Upcoming) != 1 || v.Upcoming[0].Number != "3" {
		t.Errorf("Upcoming = %+v, want just game 3", v.Upcoming)
	}
	if v.SeasonSoon {
		t.Error("SeasonSoon should be false once results exist")
	}
}

func TestStandingsSortedByPointsThenGD(t *testing.T) {
	v := BuildView(sample(), now())
	got := []string{}
	for _, s := range v.Flights[0].Standings {
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
	if !strings.Contains(out, "Today A") {
		t.Error("today's game missing from page")
	}
}

// A draw has no winner to highlight.
func TestDrawHasNoWinner(t *testing.T) {
	g := model.Game{HomeScore: ptr(2), AwayScore: ptr(2)}
	if funcs["winner"].(func(model.Game, string) bool)(g, "home") {
		t.Error("a 2-2 draw should not mark a winner")
	}
}
