// Command scrape fetches every tracked UYSA flight, parses it, and writes the
// static site into an output directory (docs/ by default, for GitHub Pages).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/briancbarrow/uysa-standings/internal/model"
	"github.com/briancbarrow/uysa-standings/internal/render"
	"github.com/briancbarrow/uysa-standings/internal/scrape"
)

func main() {
	out := flag.String("out", "docs", "output directory for the generated site")
	offline := flag.String("offline", "", "parse fixtures from this directory instead of fetching")
	timeout := flag.Duration("timeout", 90*time.Second, "overall deadline")
	flag.Parse()

	if err := run(*out, *offline, *timeout); err != nil {
		log.Fatalf("scrape: %v", err)
	}
}

func run(out, offline string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pages, err := gather(ctx, offline)
	if err != nil {
		return err
	}

	site := model.Site{GeneratedAt: time.Now().In(scrape.Mountain)}
	for _, ref := range model.Flights {
		flight, err := scrape.Parse(ref, pages[ref.Slug])
		if err != nil {
			// Fail the whole run: a half-built page is worse than a stale one,
			// because it looks current.
			return err
		}
		site.Flights = append(site.Flights, flight)
		log.Printf("%-10s %-22s %2d teams %3d games", ref.Slug, flight.Name,
			len(flight.Standings), len(flight.Games))
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(out, "data.json"), site); err != nil {
		return err
	}
	if err := writeHTML(filepath.Join(out, "index.html"), site); err != nil {
		return err
	}
	log.Printf("wrote %s/index.html and %s/data.json", out, out)
	return nil
}

// gather loads every flight page, concurrently when fetching over the network.
func gather(ctx context.Context, offline string) (map[string][]byte, error) {
	pages := make(map[string][]byte, len(model.Flights))

	if offline != "" {
		for _, ref := range model.Flights {
			b, err := os.ReadFile(filepath.Join(offline, ref.Slug+".html"))
			if err != nil {
				return nil, err
			}
			pages[ref.Slug] = b
		}
		return pages, nil
	}

	client := &http.Client{Timeout: 45 * time.Second}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		errs []error
	)
	for _, ref := range model.Flights {
		wg.Add(1)
		go func(ref model.FlightRef) {
			defer wg.Done()
			b, err := scrape.Fetch(ctx, client, ref.GUID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", ref.Slug, err))
				return
			}
			pages[ref.Slug] = b
		}(ref)
	}
	wg.Wait()
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return pages, nil
}

func writeJSON(path string, site model.Site) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(site)
}

func writeHTML(path string, site model.Site) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return render.Page(f, render.BuildView(site, time.Now().In(scrape.Mountain)))
}
