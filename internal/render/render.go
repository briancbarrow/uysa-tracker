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

// Window is how far forward and back the summary sections reach.
const Window = 7 * 24 * time.Hour

// View is the shape the template consumes.
type View struct {
	GeneratedAt time.Time
	Today       []model.Game
	Recent      []model.Game
	Upcoming    []model.Game
	Flights     []model.Flight
	SeasonSoon  bool // true before any result exists, to explain the empty sections
}

// BuildView slices the dataset into the glance sections, relative to now.
func BuildView(site model.Site, now time.Time) View {
	loc := now.Location()
	y, m, d := now.Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	v := View{GeneratedAt: site.GeneratedAt, Flights: site.Flights}
	// The source page lists teams by slot (A1, A2, ...); rank them instead.
	for i := range v.Flights {
		rows := append([]model.Standing(nil), v.Flights[i].Standings...)
		sort.SliceStable(rows, func(a, b int) bool {
			x, y := rows[a], rows[b]
			if x.TotalPoints != y.TotalPoints {
				return x.TotalPoints > y.TotalPoints
			}
			if x.GoalDiff != y.GoalDiff {
				return x.GoalDiff > y.GoalDiff
			}
			return x.GoalsFor > y.GoalsFor
		})
		v.Flights[i].Standings = rows
	}
	var all []model.Game
	for _, f := range site.Flights {
		all = append(all, f.Games...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Kickoff.Equal(all[j].Kickoff) {
			return all[i].Number < all[j].Number
		}
		return all[i].Kickoff.Before(all[j].Kickoff)
	})

	played := 0
	for _, g := range all {
		switch {
		case !g.Kickoff.Before(dayStart) && g.Kickoff.Before(dayEnd):
			v.Today = append(v.Today, g)
		case g.Kickoff.Before(dayStart) && g.Kickoff.After(now.Add(-Window)):
			v.Recent = append(v.Recent, g)
		case !g.Kickoff.Before(dayEnd) && g.Kickoff.Before(now.Add(Window)):
			v.Upcoming = append(v.Upcoming, g)
		}
		if g.Played() {
			played++
		}
	}
	// Most recent first reads better for results.
	for i, j := 0, len(v.Recent)-1; i < j; i, j = i+1, j-1 {
		v.Recent[i], v.Recent[j] = v.Recent[j], v.Recent[i]
	}
	v.SeasonSoon = played == 0
	return v
}

var funcs = template.FuncMap{
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
