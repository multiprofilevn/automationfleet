package main

import (
	"fmt"
	stdlog "log"
	"sync"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/automationfleet/examples/bingsearch"
	"github.com/multiprofilevn/chromekit"
)

// runQueries submits one Bing search per instance concurrently — remote
// (CDP) input is parallel-safe, so all 25 searches compete for the
// remoteWorkers pool instead of queueing behind a single serialized worker
// like the native examples do.
func runQueries(fleet *automationfleet.Fleet, browsers []*chromekit.Browser, ids []string) error {
	stdlog.Printf("=== running %d remote Bing searches across %d workers ===", len(ids), remoteWorkers)
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			page := browsers[idx].Current()
			results[idx] = bingsearch.RunQuery(fleet, id, page,
				func() error { return page.Keyboard().Press("Enter") }, idx+1, jobTimeout)
		}(i, id)
	}
	wg.Wait()
	fleet.Wait()

	failed := 0
	for i, id := range ids {
		if err := results[i]; err != nil {
			stdlog.Printf("FAIL %s: %v", id, err)
			failed++
			continue
		}
		stdlog.Printf("PASS %s", id)
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d instances failed", failed, len(ids))
	}
	stdlog.Printf("ALL PASS — %d remote Chrome instances searched Bing and clicked their first result", len(ids))
	return nil
}
