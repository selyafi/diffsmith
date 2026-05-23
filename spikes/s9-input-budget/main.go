// Spike S9: input budget calibration.
//
// Given a list of public GitHub PR URLs, prints per-PR diff and prompt
// sizes plus a distribution summary. Used once during v1 release prep to
// pick a defensible default for DefaultInputBudgetBytes (currently
// 256*1024 in claudecli/adapter.go and codexcli/adapter.go).
//
// Per docs/dev-plan/spikes.md, spike artifacts don't ship product code —
// this binary lives here so anyone can re-run the measurement when models
// change or the prompt scaffold grows.
//
// Usage:
//
//	go run ./spikes/s9-input-budget \
//	  https://github.com/owner/repo/pull/123 \
//	  https://github.com/owner/repo/pull/456
//
// Requires `gh` on PATH with `gh auth status` succeeding.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/provider/githubgh"
)

// The current adapter budget. Imported as a literal to keep the spike
// independent of which adapter we're calibrating against (they're equal
// in v1).
const currentBudgetBytes = 256 * 1024

type row struct {
	url         string
	files       int
	rawBytes    int
	promptBytes int
	estTokens   int
}

func main() {
	urls := os.Args[1:]
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "usage: measure-budget <github-pr-url>...")
		os.Exit(2)
	}

	ctx := context.Background()
	adapter := githubgh.New(nil)
	if err := adapter.Preflight(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "gh preflight: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-80s\t%5s\t%10s\t%12s\t%10s\t%6s\n",
		"URL", "files", "raw_bytes", "prompt_bytes", "est_tokens", "%budget")

	var rows []row
	for _, url := range urls {
		input, err := adapter.Fetch(ctx, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch %s: %v\n", url, err)
			continue
		}
		prompt := model.BuildPrompt(input)
		r := row{
			url:         url,
			files:       len(input.Files),
			rawBytes:    len(input.RawDiff),
			promptBytes: len(prompt),
			estTokens:   len(prompt) / 4,
		}
		rows = append(rows, r)
		pct := 100.0 * float64(r.promptBytes) / float64(currentBudgetBytes)
		fmt.Printf("%-80s\t%5d\t%10d\t%12d\t%10d\t%5.1f%%\n",
			r.url, r.files, r.rawBytes, r.promptBytes, r.estTokens, pct)
	}

	if len(rows) == 0 {
		os.Exit(1)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].promptBytes < rows[j].promptBytes })

	var sum int
	for _, r := range rows {
		sum += r.promptBytes
	}

	min := rows[0].promptBytes
	max := rows[len(rows)-1].promptBytes
	median := rows[len(rows)/2].promptBytes
	mean := sum / len(rows)

	fmt.Printf("\nSummary over %d PR(s):\n", len(rows))
	fmt.Printf("  prompt_bytes min:    %s\n", fmtSize(min))
	fmt.Printf("  prompt_bytes median: %s\n", fmtSize(median))
	fmt.Printf("  prompt_bytes mean:   %s\n", fmtSize(mean))
	fmt.Printf("  prompt_bytes max:    %s\n", fmtSize(max))
	fmt.Printf("  current budget:      %s (%d bytes)\n", fmtSize(currentBudgetBytes), currentBudgetBytes)
	overBudget := 0
	for _, r := range rows {
		if r.promptBytes > currentBudgetBytes {
			overBudget++
		}
	}
	fmt.Printf("  PRs over budget:     %d / %d\n", overBudget, len(rows))
}

func fmtSize(b int) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f KB (%d B)", float64(b)/1024, b)
}
