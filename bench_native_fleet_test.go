//go:build integration

// Fleet-construction helpers split out of bench_native_test.go (which owns
// resolving browser paths and launching raw kit browsers) to keep both files
// under the repo's 200-LOC guidance.
package automationfleet_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/multiprofilevn/automationfleet"
	"github.com/multiprofilevn/chromekit"
)

// newNativeChromeFleet registers browsers under ids "c0".."c{n-1}" onto a
// fresh, started Fleet with native capacity raised to len(browsers).
func newNativeChromeFleet(b *testing.B, browsers []*chromekit.Browser) (*automationfleet.Fleet, []string) {
	b.Helper()
	f := automationfleet.New(automationfleet.WithNativeCapacity(len(browsers)))
	ids := make([]string, 0, len(browsers))
	for i, br := range browsers {
		id := fmt.Sprintf("c%d", i)
		if err := f.RegisterChrome(id, br, automationfleet.ExpectNative()); err != nil {
			b.Fatalf("register chrome %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	f.Start()
	return f, ids
}

// newRemoteChromeFleet registers browsers on the default CDP (remote)
// backend, with the dispatcher's remote worker pool sized to workers.
func newRemoteChromeFleet(b *testing.B, browsers []*chromekit.Browser, workers int) (*automationfleet.Fleet, []string) {
	b.Helper()
	f := automationfleet.New(automationfleet.WithRemoteWorkers(workers))
	ids := make([]string, 0, len(browsers))
	for i, br := range browsers {
		id := fmt.Sprintf("r%d", i)
		if err := f.RegisterChrome(id, br, automationfleet.ExpectRemote()); err != nil {
			b.Fatalf("register remote chrome %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	f.Start()
	return f, ids
}

// submitAndWait submits one job and blocks for its result.
func submitAndWait(b *testing.B, f *automationfleet.Fleet, id string, act automationfleet.Action) automationfleet.JobResult {
	b.Helper()
	ch, err := f.Submit(automationfleet.Job{BrowserID: id, Action: act, Timeout: 15 * time.Second})
	if err != nil {
		b.Fatalf("submit: %v", err)
	}
	return <-ch
}

// navigateAll drives every registered id to url and fails the benchmark
// loudly on any non-Done result — a benchmark measuring a broken navigate is
// a benchmark measuring nothing.
func navigateAll(b *testing.B, f *automationfleet.Fleet, ids []string, url string) {
	b.Helper()
	for _, id := range ids {
		res := submitAndWait(b, f, id, automationfleet.NavigateAction{URL: url, Timeout: 15 * time.Second})
		if res.Status != automationfleet.StatusDone {
			b.Fatalf("navigate %s: status=%s err=%v", id, res.Status, res.Err)
		}
	}
}
