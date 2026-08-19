# uysa-standings

Scrapes Boys 13U scores and standings from UYSA / SportsAffinity and renders a
single static page you can glance at on a phone.

Four flights, 2026 Fall season (`tournamentguid=69C35A62-…`):

| Flight | slug | teams | games |
|---|---|---|---|
| Boys 13U Premier | `premier` | 9 | 36 |
| Boys 13U Division 3 | `division3` | 11 | 55 |
| Boys 13U Metro A | `metroa` | 9 | 36 |
| Boys 13U Metro B | `metrob` | 10 | 45 |

Season runs **Aug 22 – Oct 31, 2026**.

## How it works

GitHub Actions runs on a cron, fetches the four flight pages concurrently,
parses them, writes `docs/index.html` + `docs/data.json`, and commits only if
something changed. GitHub Pages serves `docs/`. No server, no database — the
git history *is* the change log, so `git log -p docs/data.json` shows exactly
which scores posted and when.

## Run it locally

```sh
go run ./cmd/scrape -out docs                 # live fetch
go run ./cmd/scrape -offline testdata -out /tmp/site   # parse saved fixtures
go test ./...
```

Then open `docs/index.html`.

## Setup

1. Push to GitHub as a **public** repo. Public repos get unlimited Actions
   minutes and free Pages, so this costs nothing to run.
2. Settings → Pages → Source: *Deploy from a branch*, branch `main`, folder
   `/docs`.
3. Settings → Actions → General → Workflow permissions: *Read and write*
   (the scrape job commits its output).
4. Run the `scrape` workflow once by hand to seed it.

Private repo instead? Pages needs a Pro plan. The minutes are a non-issue at
one run a day (each job is billed as a full minute even though it takes ~20s).

## Polling schedule

One scheduled run a day, at **21:30 Mountain** (`30 3 * * *` UTC — the season
is entirely MDT, UTC-6). That is late enough to pick up results entered after
the evening fixtures.

Scheduled runs are gated on `SEASON_OPENS` (2026-08-22) in the workflow and do
nothing before the first fixture, since there is no data to collect yet. The
gate only applies to the cron: a manual run always scrapes, whatever the date.
Because 03:30 UTC is the previous evening in Mountain time, the first automatic
scrape lands the evening of opening Saturday.

For anything sooner, the page footer links straight to this workflow's run
page; `workflow_dispatch` lets anyone with write access trigger a run by hand.
Dispatching requires push permission, so the link is inert for the public.

Scraping more often would not help much anyway: the real lag is clubs entering
results into SportsAffinity, which takes hours to days.

At ~30 runs/month this is far inside the free tier even on a private repo, and
free outright on a public one. If you ever widen the schedule again, two things
to not change casually:

- **Keep it one job.** A `strategy.matrix` over the four flights would be four
  billed minutes per run instead of one, for no gain — the fetches already run
  concurrently inside a single Go process (~0.6s total).
- **Keep the windows narrow.** Flat `*/15` around the clock is ~2,880 runs/month.

## Parsing notes

The source pages are legacy ASP table soup: no ids, no classes on data cells,
and some rows are malformed HTML (unclosed `<nobr>`, a `<td>` nested inside
another `<td>`). Go's `net/html` repairs them consistently, so the parsers key
off structure and header text.

- **`game number` is the primary key** — unique across all four flights
  (verified by `TestGameNumbersUnique`), stable across reschedules.
- **Standings columns are indexed from the right.** The per-round columns scale
  with team count (a 9-team flight has 19 cells, an 11-team flight has 21), so
  the trailing stat columns are not addressable from the left.
- **`teamcode` in each row's link** (e.g. `0571-01CB13-0019`) is a stable team
  identifier, better than matching on display name.
- **An unposted score parses to `nil`, never `0`** — otherwise every unplayed
  game would look like a 0-0 draw.
- **Parsers fail loudly.** A layout change errors the run and turns the Actions
  job red, rather than quietly publishing an empty table that looks just like
  "no games played yet".

Fixtures in `testdata/` were captured 2026-08-18. Refresh them with
`scripts/refresh-fixtures.sh` if the site changes, then update the expected
counts in `internal/scrape/scrape_test.go`.

### Caveats

- Fixtures were captured **before the season opened**, so every score cell was
  blank. Score parsing is written defensively but is unverified against real
  posted scores until after the first Saturday (Aug 22).
- GitHub disables scheduled workflows after 60 days of repo inactivity. The
  bot's own commits should keep it alive; the last-updated stamp in the page
  footer is the tell if it ever stops.

## Possible next steps

- Push notification (ntfy/Pushover) when `data.json` gains a score.
- Pebble watchapp fed by `docs/data.json`.
