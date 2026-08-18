#!/usr/bin/env bash
# Re-download the parser fixtures. Run this if the site's layout changes and
# the tests start failing, then update the expected counts in scrape_test.go.
set -euo pipefail
cd "$(dirname "$0")/.."

T=69C35A62-D325-418C-95D3-C61FA95030D3
declare -A F=(
  [premier]=66DD5A39-2C01-4E6B-B14A-77B55AC36027
  [division3]=86FD18DF-E48F-4C70-B14F-A6535C269A1E
  [metroa]=6170DA6C-22CD-4DDD-A666-7F899F31F723
  [metrob]=788C877A-364A-4401-8166-FF2C034737AF
)
for slug in "${!F[@]}"; do
  url="https://uysa.sportsaffinity.com/tour/public/info/schedule_results2.asp?sessionguid=&flightguid=${F[$slug]}&tournamentguid=$T"
  curl -sS -L --max-time 45 -A "uysa-standings/1.0" "$url" -o "testdata/$slug.html"
  echo "$slug -> testdata/$slug.html ($(wc -c < "testdata/$slug.html") bytes)"
done
