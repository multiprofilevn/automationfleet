package main

import (
	"context"
	"fmt"
	stdlog "log"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/automationfleet/examples/bingsearch"
	"github.com/multiprofilevn/chromekit"
)

// runQueries submits one Bing search per instance. Native input is
// inherently serial — one real cursor, one real keyboard — so the
// dispatcher's single native worker processes these one at a time
// regardless of submission order; looping sequentially here just mirrors
// that reality instead of pretending concurrency helps. ctx is checked
// between instances so Ctrl+C stops queuing further searches promptly
// instead of running all of them first.
func runQueries(ctx context.Context, fleet *automationfleet.Fleet, browsers []*chromekit.Browser, ids []string) error {
	stdlog.Printf("=== running %d native Bing searches (serialized through the one native worker) ===", len(ids))
	failed := 0
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("interrupted after %d/%d searches: %w", i, len(ids), err)
		}
		page := browsers[i].Current()
		if err := bingsearch.RunQuery(fleet, id, page,
			func() error { return page.Keyboard().Press("Enter") }, i+1, jobTimeout); err != nil {
			stdlog.Printf("FAIL %s: %v", id, err)
			failed++
			continue
		}
		stdlog.Printf("PASS %s", id)
	}
	fleet.Wait()
	if failed > 0 {
		return fmt.Errorf("%d/%d instances failed", failed, len(ids))
	}
	stdlog.Printf("ALL PASS — %d native Chrome instances searched Bing and clicked their first result", len(ids))
	return nil
}
