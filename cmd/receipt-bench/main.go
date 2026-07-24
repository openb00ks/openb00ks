// Command receipt-bench replays a labeled corpus of receipts through the decomposed pipeline and
// prints per-stage accuracy. It uses the SAME request builders as production (internal/pipeline), so a
// prompt/model change surfaces regressions before shipping. See docs/receipt-pipeline.md.
//
// Usage:
//
//	OPENAI_API_KEY=... OPENAI_MODEL=... receipt-bench cases.jsonl
//
// cases.jsonl is one JSON eval.Case per line (blank lines and #-comments ignored).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openb00ks/openb00ks/internal/config"
	"github.com/openb00ks/openb00ks/internal/eval"
	"github.com/openb00ks/openb00ks/internal/suggest"
	aipkg "github.com/spectrum-labs-tech/ai"
)

// completer adapts the shared provider to pipeline.Completer at temperature 0 (matches the worker).
type completer struct{ p aipkg.Provider }

func (c completer) Complete(ctx context.Context, system, user, schema string) (string, error) {
	zero := 0.0
	return c.p.Complete(ctx, system, user, schema, aipkg.Options{Temperature: &zero})
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: receipt-bench <cases.jsonl>")
		os.Exit(2)
	}
	cases, err := loadCases(os.Args[1])
	if err != nil {
		fatal(err)
	}
	if len(cases) == 0 {
		fatal(fmt.Errorf("no cases in %s", os.Args[1]))
	}

	cfg := config.Load()
	if cfg.OpenAIAPIKey == "" {
		fatal(fmt.Errorf("OPENAI_API_KEY is required"))
	}
	provider, err := suggest.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	if err != nil {
		fatal(err)
	}
	defer func() { _ = provider.Close() }()
	ai := completer{p: provider}

	ctx := context.Background()
	scores := make([]eval.CaseScore, 0, len(cases))
	for _, c := range cases {
		_, s, _ := eval.RunCase(ctx, ai, c)
		scores = append(scores, s)
		fmt.Printf("%-32s %s\n", truncate(c.Name, 32), summary(s))
	}

	rep := eval.Aggregate(scores)
	fmt.Println("\n=== per-dimension accuracy ===")
	line := func(name string, a eval.Accuracy) {
		if a.Scored > 0 {
			fmt.Printf("  %-9s %6.1f%%  (%d/%d)\n", name, a.Pct(), a.Correct, a.Scored)
		}
	}
	line("total", rep.Total)
	line("vendor", rep.Vendor)
	line("account", rep.Account)
	line("ready", rep.Ready)
	fmt.Printf("  cases=%d  errors=%d\n", rep.Cases, rep.Errors)
	if rep.Errors > 0 {
		os.Exit(1)
	}
}

func loadCases(path string) ([]eval.Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var cases []eval.Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20) // allow long lines (OCR text)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c eval.Case
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("parse case: %w", err)
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

func summary(s eval.CaseScore) string {
	if s.Err != nil {
		return "ERROR: " + s.Err.Error()
	}
	mark := func(b *bool) string {
		switch {
		case b == nil:
			return "·"
		case *b:
			return "✓"
		default:
			return "✗"
		}
	}
	out := fmt.Sprintf("total:%s vendor:%s account:%s ready:%s", mark(s.Total), mark(s.Vendor), mark(s.Account), mark(s.Ready))
	if s.FailedStage != "" {
		out += "  parked@" + s.FailedStage
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "receipt-bench:", err)
	os.Exit(1)
}
