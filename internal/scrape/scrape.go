// Package scrape fetches and parses UYSA sportsaffinity flight pages.
//
// The pages are legacy ASP table soup: no ids, no classes on data cells, and
// some rows are malformed HTML. net/html repairs them consistently, so the
// parsers below key off structure and header text rather than markup details.
// Every parser is strict: a layout change must fail loudly, because silently
// emitting an empty table looks exactly like "no games have been played yet".
package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata" // so the America/Denver lookup works on bare runners

	"github.com/PuerkitoBio/goquery"

	"github.com/briancbarrow/uysa-standings/internal/model"
)

const baseURL = "https://uysa.sportsaffinity.com/tour/public/info/schedule_results2.asp"

// Mountain is the tournament's local time zone; all listed kickoff times are in it.
var Mountain = func() *time.Location {
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		panic("America/Denver unavailable: " + err.Error())
	}
	return loc
}()

// URLFor builds the public page URL for a flight.
func URLFor(guid string) string {
	return fmt.Sprintf("%s?sessionguid=&flightguid=%s&tournamentguid=%s", baseURL, guid, model.TournamentGUID)
}

// Fetch retrieves one flight page.
func Fetch(ctx context.Context, client *http.Client, guid string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URLFor(guid), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "uysa-standings/1.0 (+https://github.com/briancbarrow/uysa-standings)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flight %s: HTTP %d", guid, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

var (
	reTeamCode = regexp.MustCompile(`teamcode=([^&"]*)`)
	reGroup    = regexp.MustCompile(`groupcode=([^&"]*)`)
	reRecord   = regexp.MustCompile(`W-(\d+)\s*L-(\d+)\s*T-(\d+)`)
	reBracket  = regexp.MustCompile(`^Bracket\s*-\s*(.+)$`)
)

// Parse turns one flight page into a Flight.
func Parse(ref model.FlightRef, html []byte) (model.Flight, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(html)))
	if err != nil {
		return model.Flight{}, fmt.Errorf("%s: %w", ref.Slug, err)
	}

	f := model.Flight{Slug: ref.Slug, GUID: ref.GUID, URL: URLFor(ref.GUID)}

	title := norm(doc.Find("span.title").First().Text())
	name := strings.TrimSpace(strings.TrimPrefix(title, "Team Schedules -"))
	if name == "" || name == title {
		return f, fmt.Errorf("%s: flight name not found (span.title was %q)", ref.Slug, title)
	}
	f.Name = name

	if f.Standings, err = parseStandings(doc); err != nil {
		return f, fmt.Errorf("%s: %w", ref.Slug, err)
	}
	if f.Games, err = parseSchedule(doc, ref.Slug, name); err != nil {
		return f, fmt.Errorf("%s: %w", ref.Slug, err)
	}
	return f, nil
}

// parseStandings reads the group table.
//
// Column count varies by flight: the per-round columns scale with team count,
// so the leading columns are fixed but the trailing ones are not addressable
// from the left. Everything we want except team and record sits in the last
// nine cells, so those are indexed from the end.
func parseStandings(doc *goquery.Document) ([]model.Standing, error) {
	rows := doc.Find("a[href*='teamcode=']")
	if rows.Length() == 0 {
		return nil, fmt.Errorf("no standings rows found")
	}

	var out []model.Standing
	var parseErr error
	rows.Each(func(_ int, a *goquery.Selection) {
		if parseErr != nil {
			return
		}
		href, _ := a.Attr("href")
		cells := a.Closest("tr").ChildrenFiltered("td")

		// team, record, >=1 round column, then the 9 trailing stat columns.
		const trailing = 9
		if n := cells.Length(); n < trailing+3 {
			parseErr = fmt.Errorf("standings row %q has %d cells, want at least %d", norm(a.Text()), n, trailing+3)
			return
		}
		last := cells.Length() - 1
		at := func(fromEnd int) string { return norm(cells.Eq(last - fromEnd).Text()) }

		slot, team, _ := strings.Cut(norm(a.Text()), " : ")
		w, l, t, err := parseRecord(norm(cells.Eq(1).Text()))
		if err != nil {
			parseErr = fmt.Errorf("team %q: %w", team, err)
			return
		}

		out = append(out, model.Standing{
			Slot:     strings.TrimSpace(slot),
			Group:    firstSubmatch(reGroup, href),
			TeamCode: firstSubmatch(reTeamCode, href),
			Team:     strings.TrimSpace(team),
			Wins:     w,
			Losses:   l,
			Ties:     t,
			// index 0 from the end is the card-detail icon cell, which has no text.
			Red:          atoiOr0(at(1)),
			Yellow:       atoiOr0(at(2)),
			Shutouts:     atoiOr0(at(3)),
			GoalsFor:     atoiOr0(at(4)),
			GoalsAgainst: atoiOr0(at(5)),
			GoalDiff:     atoiOr0(at(6)),
			TotalPoints:  atoiOr0(at(7)),
			PointsDed:    atoiOr0(at(8)),
		})
	})
	return out, parseErr
}

// parseSchedule reads every "Bracket - <date>" block and the table after it.
func parseSchedule(doc *goquery.Document, slug, flightName string) ([]model.Game, error) {
	blocks := doc.Find("center.txtM")
	if blocks.Length() == 0 {
		return nil, fmt.Errorf("no schedule date blocks found")
	}

	var out []model.Game
	var parseErr error
	blocks.Each(func(_ int, c *goquery.Selection) {
		if parseErr != nil {
			return
		}
		m := reBracket.FindStringSubmatch(norm(c.Text()))
		if m == nil {
			return // not a date header
		}
		rawDate := strings.TrimSpace(m[1])
		day, err := time.ParseInLocation("Monday, January 2, 2006", rawDate, Mountain)
		if err != nil {
			parseErr = fmt.Errorf("date header %q: %w", rawDate, err)
			return
		}

		table := c.Next()
		if !table.Is("table") {
			parseErr = fmt.Errorf("date header %q is not followed by a table", rawDate)
			return
		}
		if header := norm(table.Find("tr").First().Text()); !strings.Contains(header, "Home Team") {
			parseErr = fmt.Errorf("date header %q: schedule header row changed, got %q", rawDate, header)
			return
		}

		table.Find("tr").Each(func(j int, tr *goquery.Selection) {
			if j == 0 || parseErr != nil {
				return // header
			}
			cells := tr.ChildrenFiltered("td")
			const wantCells = 10
			if cells.Length() != wantCells {
				parseErr = fmt.Errorf("date %q row %d: %d cells, want %d", rawDate, j, cells.Length(), wantCells)
				return
			}
			cell := func(i int) string { return norm(cells.Eq(i).Text()) }

			rawTime := cell(2)
			kickoff := day
			if clock, err := time.ParseInLocation("03:04 PM", rawTime, Mountain); err == nil {
				kickoff = time.Date(day.Year(), day.Month(), day.Day(),
					clock.Hour(), clock.Minute(), 0, 0, Mountain)
			}

			out = append(out, model.Game{
				Number:     cell(0),
				FlightSlug: slug,
				FlightName: flightName,
				Kickoff:    kickoff,
				RawDate:    rawDate,
				RawTime:    rawTime,
				Venue:      cell(1),
				Field:      cell(3),
				Matchup:    cell(4),
				Home:       cell(5),
				HomeScore:  parseScore(cell(6)),
				Away:       cell(8),
				AwayScore:  parseScore(cell(9)),
			})
		})
	})
	if parseErr != nil {
		return nil, parseErr
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("schedule blocks present but no games parsed")
	}
	return out, nil
}

// parseScore returns nil for an unplayed game rather than 0, so "0-0 draw" and
// "not played yet" stay distinguishable.
func parseScore(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func parseRecord(s string) (w, l, t int, err error) {
	m := reRecord.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("unparseable W-L-T record %q", s)
	}
	return atoiOr0(m[1]), atoiOr0(m[2]), atoiOr0(m[3]), nil
}

func firstSubmatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// norm collapses non-breaking spaces and runs of whitespace into single spaces.
func norm(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}
