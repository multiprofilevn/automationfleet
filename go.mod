module github.com/tuwibu/automationfleet

go 1.26.2

require (
	github.com/tuwibu/chromekit v0.6.1
	github.com/tuwibu/firefoxkit v0.1.0
	go.uber.org/goleak v1.3.0
	golang.org/x/sys v0.42.0
)

// Local development against the sibling kit repos. Removed at the end of
// phase 8, when both kits are tagged and this points at real versions again.
replace github.com/tuwibu/chromekit => ../chromekit

replace github.com/tuwibu/firefoxkit => ../firefoxkit

require (
	github.com/chromedp/cdproto v0.0.0-20260427013145-5737772c319b // indirect
	github.com/chromedp/chromedp v0.15.1 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/go-json-experiment/json v0.0.0-20260214004413-d219187c3433 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
)
