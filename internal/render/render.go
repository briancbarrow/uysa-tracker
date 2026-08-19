// Package render turns scraped flights into the static page.
package render

import (
	"embed"
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/briancbarrow/uysa-standings/internal/model"
)

//go:embed templates/*.tmpl
var files embed.FS

// Window is how far forward and back each flight's schedule reaches.
const Window = 7 * 24 * time.Hour

// RefreshURL points at the scrape workflow's run page. Triggering it needs
// write access to the repo, so the link is a no-op for everyone else: GitHub
// only renders the "Run workflow" button for users who can push.
const RefreshURL = "https://github.com/briancbarrow/uysa-tracker/actions/workflows/scrape.yml"

// FavoriteTeamCode is the team highlighted throughout the page: Copper
// Mountain 7 ZP/ZN in Metro A. Matching on the team code rather than the name
// matters here, because the same flight also fields "Copper Mountain 7 BH" and
// three more Copper Mountain sides play in other flights.
const FavoriteTeamCode = "0213-01CB13-1128"

// DefaultOpen is the one flight expanded on load, so the page is useful
// without a tap. Everything else stays collapsed to keep the page short.
const DefaultOpen = "metroa"

// GameCard is one game plus whether the favorite team is playing in it.
// Games carry team names but not codes, so the flag is resolved from the
// favorite's exact name, looked up once from the standings.
type GameCard struct {
	model.Game
	Fav bool
}

// StandingRow is one standings row plus whether it is the favorite team.
type StandingRow struct {
	model.Standing
	Fav bool
}

// FlightView is one collapsible flight card: its standings and its own
// schedule. Games never appear outside the flight they belong to.
type FlightView struct {
	Slug      string
	Name      string
	URL       string
	Open      bool
	Standings []StandingRow
	Today     []GameCard
	Recent    []GameCard
	Upcoming  []GameCard
}

// HasSchedule reports whether any windowed section has games to show.
func (f FlightView) HasSchedule() bool {
	return len(f.Today)+len(f.Recent)+len(f.Upcoming) > 0
}

// View is the shape the template consumes.
type View struct {
	GeneratedAt  time.Time
	RefreshURL   string
	FavoriteName string
	Flights      []FlightView
	SeasonSoon   bool // true before any result exists, to explain empty sections
}

// BuildView slices each flight into its own glance sections, relative to now.
func BuildView(site model.Site, now time.Time) View {
	loc := now.Location()
	y, m, d := now.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	v := View{GeneratedAt: site.GeneratedAt, RefreshURL: RefreshURL}

	// Resolve the favorite's display name once; the schedule only has names.
	for _, f := range site.Flights {
		for _, s := range f.Standings {
			if s.TeamCode == FavoriteTeamCode {
				v.FavoriteName = s.Team
			}
		}
	}
	isFav := func(g model.Game) bool {
		return v.FavoriteName != "" && (g.Home == v.FavoriteName || g.Away == v.FavoriteName)
	}
	played := 0

	for _, f := range site.Flights {
		fv := FlightView{
			Slug: f.Slug,
			Name: f.Name,
			URL:  f.URL,
			Open: f.Slug == DefaultOpen,
		}

		// The source page lists teams by slot (A1, A2, ...); rank them instead.
		for _, st := range f.Standings {
			fv.Standings = append(fv.Standings, StandingRow{
				Standing: st,
				Fav:      st.TeamCode == FavoriteTeamCode,
			})
		}
		sort.SliceStable(fv.Standings, func(a, b int) bool {
			x, y := fv.Standings[a], fv.Standings[b]
			if x.TotalPoints != y.TotalPoints {
				return x.TotalPoints > y.TotalPoints
			}
			if x.GoalDiff != y.GoalDiff {
				return x.GoalDiff > y.GoalDiff
			}
			return x.GoalsFor > y.GoalsFor
		})

		games := append([]model.Game(nil), f.Games...)
		sort.Slice(games, func(i, j int) bool {
			if games[i].Kickoff.Equal(games[j].Kickoff) {
				return games[i].Number < games[j].Number
			}
			return games[i].Kickoff.Before(games[j].Kickoff)
		})

		for _, g := range games {
			c := GameCard{Game: g, Fav: isFav(g)}
			switch {
			case !g.Kickoff.Before(dayStart) && g.Kickoff.Before(dayEnd):
				fv.Today = append(fv.Today, c)
			case g.Kickoff.Before(dayStart) && g.Kickoff.After(now.Add(-Window)):
				fv.Recent = append(fv.Recent, c)
			case !g.Kickoff.Before(dayEnd) && g.Kickoff.Before(now.Add(Window)):
				fv.Upcoming = append(fv.Upcoming, c)
			}
			if g.Played() {
				played++
			}
		}
		// Most recent first reads better for results.
		for i, j := 0, len(fv.Recent)-1; i < j; i, j = i+1, j-1 {
			fv.Recent[i], fv.Recent[j] = fv.Recent[j], fv.Recent[i]
		}

		v.Flights = append(v.Flights, fv)
	}

	v.SeasonSoon = played == 0
	return v
}

// cardCtx is what the gamecard sub-template receives: inside a {{define}} the
// "$" root is rebound to the argument, so the favorite name has to travel with
// the game rather than being read off the top-level view.
type cardCtx struct {
	GameCard
	FavName string
}

var funcs = template.FuncMap{
	"card": func(g GameCard, favName string) cardCtx {
		return cardCtx{GameCard: g, FavName: favName}
	},
	"clock": func(t time.Time) string { return t.Format("3:04 PM") },
	"day":   func(t time.Time) string { return t.Format("Mon Jan 2") },
	"stamp": func(t time.Time) string { return t.Format("Mon Jan 2, 3:04 PM MST") },
	"score": func(g model.Game) string {
		if !g.Played() {
			return ""
		}
		return itoa(*g.HomeScore) + " – " + itoa(*g.AwayScore)
	},
	"winner": func(g model.Game, side string) bool {
		if !g.Played() || *g.HomeScore == *g.AwayScore {
			return false
		}
		if side == "home" {
			return *g.HomeScore > *g.AwayScore
		}
		return *g.AwayScore > *g.HomeScore
	},
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Page writes the static HTML page.
func Page(w io.Writer, v View) error {
	t, err := template.New("index.html.tmpl").Funcs(funcs).ParseFS(files, "templates/*.tmpl")
	if err != nil {
		return err
	}
	return t.Execute(w, v)
}
