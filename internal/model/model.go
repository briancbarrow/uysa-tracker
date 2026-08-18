// Package model holds the data types shared by the scraper and the renderer.
package model

import "time"

// TournamentGUID is the 2026 Fall PL, SCL, IRL, XL season.
const TournamentGUID = "69C35A62-D325-418C-95D3-C61FA95030D3"

// FlightRef identifies one flight page to scrape.
type FlightRef struct {
	Slug string // stable short name, used for fixtures and JSON keys
	GUID string
}

// Flights is every flight we track, in display order.
var Flights = []FlightRef{
	{Slug: "premier", GUID: "66DD5A39-2C01-4E6B-B14A-77B55AC36027"},
	{Slug: "division3", GUID: "86FD18DF-E48F-4C70-B14F-A6535C269A1E"},
	{Slug: "metroa", GUID: "6170DA6C-22CD-4DDD-A666-7F899F31F723"},
	{Slug: "metrob", GUID: "788C877A-364A-4401-8166-FF2C034737AF"},
}

// Standing is one team's row in the flight table.
type Standing struct {
	Slot         string `json:"slot"` // "A1"
	Group        string `json:"group"`
	TeamCode     string `json:"teamCode"` // stable across the season
	Team         string `json:"team"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	Ties         int    `json:"ties"`
	PointsDed    int    `json:"pointsDed"`
	TotalPoints  int    `json:"totalPoints"`
	GoalDiff     int    `json:"goalDiff"`
	GoalsAgainst int    `json:"goalsAgainst"`
	GoalsFor     int    `json:"goalsFor"`
	Shutouts     int    `json:"shutouts"`
	Yellow       int    `json:"yellow"`
	Red          int    `json:"red"`
}

// Game is one scheduled match. Number is unique tournament-wide.
type Game struct {
	Number     string    `json:"number"`
	FlightSlug string    `json:"flightSlug"`
	FlightName string    `json:"flightName"`
	Kickoff    time.Time `json:"kickoff"`
	RawDate    string    `json:"rawDate"`
	RawTime    string    `json:"rawTime"`
	Venue      string    `json:"venue"`
	Field      string    `json:"field"`
	Matchup    string    `json:"matchup"` // "A1 vs A2"
	Home       string    `json:"home"`
	Away       string    `json:"away"`
	HomeScore  *int      `json:"homeScore"` // nil until a score is posted
	AwayScore  *int      `json:"awayScore"`
}

// Played reports whether both scores have been posted.
func (g Game) Played() bool { return g.HomeScore != nil && g.AwayScore != nil }

// Flight is one scraped page.
type Flight struct {
	Slug      string     `json:"slug"`
	GUID      string     `json:"guid"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Standings []Standing `json:"standings"`
	Games     []Game     `json:"games"`
}

// Site is the whole rendered dataset.
type Site struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Flights     []Flight  `json:"flights"`
}
