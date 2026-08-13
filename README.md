# automationfleet

Orchestrator for driving N browser instances — **Chrome** (via [chromekit](https://github.com/tuwibu/chromekit)) and **Firefox** (via [firefoxkit](https://github.com/tuwibu/firefoxkit)) — through one fleet, with dual-path routing: native input (mouse + keyboard) under a serialized critical section, or parallel human-like remote actions (bezier cursor, TypeHuman, ClearInput) per per-handle `Native` flag.

**Driver abstraction.** The fleet talks to a `Driver` interface, not a concrete kit. Two thin adapters (`WrapChrome`, `WrapFirefox`) bridge chromekit/firefoxkit into the fleet; the kits stay unmodified and independently versioned. The dispatcher, queue, focus arbitration, and hotkey controls are kit-agnostic.

## Architecture

```
┌────────────────┐    Submit(Job)    ┌──────────────────────────────┐
│  Your code     │──────────────────▶│  Fleet                       │
└────────────────┘                   │   ├─ priorityQueue           │
                                     │   ├─ nativeWorker (if any)   │
                                     │   ├─ cdpWorkers pool (if any)│
                                     │   └─ hotkeyListener          │
                                     │      (Pause/Resume enabled   │
                                     │       by default; Stop opt-in)
                                     └──────────────────────────────┘
                    ▲                 ▼
              Per-handle Native flag determines routing:
              
  Native=true (native critical section):      Native=false (parallel CDP):
  ├─ 1× nativeWorker (serial)                 ├─ N× cdpWorkers (parallel)
  ├─ Focus, scroll, bbox, drift-guard        ├─ Mouse.Click (bezier glide)
  ├─ Click / Type / Navigate                  ├─ Mouse.FocusElement
  ├─ Retry on drift (default 3)               ├─ Keyboard.ClearInput (optional)
  └─ IME guard (Windows)                      └─ Keyboard.TypeHuman
                                                 (80–220ms/char, anti-bot safe)
```

**Native critical section** (single worker, one NativeHandle at a time):

```
Browser.Focus → page.ScrollIntoView → page.BoundingBox → MouseMove → drift-guard → Click [+ IME-guard → Type]
```

Cannot be split — typing without a focused target risks keystrokes leaking into other windows.

**CDP parallel path** (multiple browsers concurrently):

```
Mouse.Click (bezier glide) + FocusElement + optional ClearInput + TypeHuman (human-like 80–220ms/char, 5% typo)
```

Each browser runs independently on a CDP worker; human-like behavior avoids bot detection.

## Quick start

```go
fleet := automationfleet.New(
    automationfleet.WithLogger(myLogger),
    automationfleet.WithDefaultTimeout(10*time.Second),
)
fleet.Start()
defer fleet.Stop()

b1, _ := chromekit.Connect(9222,
    chromekit.WithInputBackend(chromekit.BackendNative),
    chromekit.WithNativeWindow(0, 0, 1.0))
// The input mode comes from the kit; ExpectNative() asserts what you believe
// you launched, and a mismatch is an error rather than a silent downgrade.
fleet.RegisterChrome("b1", b1, automationfleet.ExpectNative())

ch, _ := fleet.Submit(automationfleet.Job{
    BrowserID: "b1",
    Action:    automationfleet.TypeAction{Selector: "input[name=q]", Text: "hello"},
    Priority:  5,
})
result := <-ch
```

## Hotkey controls

**Pause (Ctrl+F10) and Resume (Ctrl+F11):** Enabled by default. Pause gracefully blocks after the current job completes; Resume unblocks immediately.

**Stop (Ctrl+Alt+Shift+S):** DISABLED by default (opt-in via `WithStopHotkey`). Destructive — no resume after stop.

Customize stop hotkey:

```go
hk, _ := automationfleet.ParseHotkey("Ctrl+Shift+Q")
fleet := automationfleet.New(automationfleet.WithStopHotkey(hk))
```

Or programmatically: `fleet.Pause("reason")` / `fleet.Resume("reason")` / `fleet.Stop()`.

`Stop()` is a full drain: it requests shutdown once, cancels pending work, and
returns only after the hotkey listener, dispatcher workers, and in-flight jobs
have exited. To bound the caller's wait when a driver operation does not honor
cancellation, use `StopContext`; shutdown keeps draining after the deadline:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := fleet.StopContext(ctx); err != nil {
    log.Printf("fleet is still draining: %v", err)
}
```

## Foreground lease

Native input is not addressed to a window: keystrokes reach whatever owns the OS foreground, and clicks reach whatever sits under the cursor. So anything that *takes* the foreground — above all opening another browser — must not happen while the fleet is mid-typing, or the tail of the string lands in the new window.

Borrow the foreground before opening a browser, and give it back once the window is up:

```go
lease, err := fleet.AcquireForeground(ctx, "open-profile-42")
if err != nil {
    return err // ctx cancelled, or the fleet has stopped
}
defer lease.Release() // ALWAYS defer — a leaked lease stalls every native job

openBrowser()
waitUntilReady()
```

`AcquireForeground` blocks new native jobs and waits for the one already running (across every drift retry) to finish. Remote (CDP/BiDi) jobs keep flowing throughout — they never touch the OS cursor.

A lease held past `WithLeaseTimeout` (default 60s) does **not** auto-release: dropping it on a timer would break exclusivity silently during a cold start that legitimately runs long. It fires `OnLeaseStuck` instead so the owner can surface the stall:

```go
fleet := automationfleet.New(
    automationfleet.WithLeaseTimeout(90*time.Second),
    automationfleet.OnLeaseStuck(func(reason string, heldFor time.Duration) {
        log.Printf("foreground lease %q stuck for %s", reason, heldFor)
    }),
)
```

Inside a job the kits guard the emitters themselves: every ~5 characters they confirm the browser still owns the foreground, and abort with `ErrForegroundLost` when it does not. A `TypeAction` interrupted that way is repaired by reading the field back over the protocol and comparing it to the target value — never by retyping blindly, which is what duplicates content.

## Drivers

Register either kit through a typed helper — both wrap the browser in a `Driver` and route through the same dispatcher:

```go
fleet.RegisterChrome("chrome-1", chromeBrowser, automationfleet.ExpectNative())
fleet.RegisterFirefox("firefox-1", ffBrowser, automationfleet.ExpectRemote())
```

The input mode is **derived** from the kit (`Driver.InputBackend()`), never declared: no call site can put the fleet and the browser into disagreement. `ExpectNative()` / `ExpectRemote()` are assertions — a browser launched remote but registered with `ExpectNative()` is **rejected**, because quietly accepting it as remote strips the anti-bot guarantees it was launched for and nothing downstream would say so. The window origin and DPI scale are no longer parameters either; the kit measures the coordinate frame live.

See [`examples/mixed_fleet`](examples/mixed_fleet/main.go) for a Chrome + Firefox fleet.

**Driver surface.** `Driver`/`Page`/`Mouse`/`Keyboard` are widened to cover
navigation, element query, wait, eval, screenshot, upload and dialog handling
on `Page`, plus the full `Mouse` (12) and `Keyboard` (8) primitive sets — every
method both chromekit and firefoxkit expose under an identical name and a
signature that doesn't diverge by kit. `parity_test.go`'s
`TestConformance_Adapter*` tests enumerate both kits reflectively and fail the
build the moment an adapter drifts from either kit's real surface.

| Interface | Covers | Deliberately NOT wrapped |
|---|---|---|
| `Driver` | `Current`, `Focus`, `InputBackend`, `ContentOffset`, `HWND`, `WindowScale`, `InvalidateNativeWindow`, `WindowRect` | — |
| `Mouse` | All 12: `MoveTo`, `ClickAt`, `Click`, `RightClick`, `MiddleClick`, `DoubleClick`, `Hover`, `Scroll`, `Drag`, `DragAndDrop`, `Tap`, `FocusElement` | — |
| `Keyboard` | All 8: `ClearInput`, `Down`, `Up`, `Press`, `Type`, `TypeFast`, `TypeHuman`, `TypeSelector` | — |
| `Page` | Navigation (`Navigate`/`Reload`/`GoBack`/`GoForward`/`GetURL`/`GetTitle`/`GetOrigin`/`URL`/`Title`), query (`GetText`/`GetAttribute`/`ScrollIntoView`/`Focus`/`ElementCenter`/`ReadValue`), wait (`WaitForSelector`/`WaitForSelectorGone`/`WaitForChangeURL`/`WaitForFunction`/`WaitForReady`), eval (`Evaluate`/`EvalSelector`/`EvalSelectorAll`/`EvaluateCtx`), screenshot (`Screenshot`/`ScreenshotClip`/`ScreenshotElement`/`FullScreenshot`), upload (`SetInputFiles`), dialog (`OnDialog`) | `Cookies`, `Downloads`, `LocalStorage`/`SessionStorage`, WebExtension install, frame-scoped (`Frames`/`AllFrames`/`MainFrame`/`FrameByURL`/`FrameByName`), page lifecycle (`ID`/`Ctx`/`Index`/`Close`/`BringToFront`), node-handle queries (`QuerySelector`/`QuerySelectorAll`/`BoundingBox` — abstracted instead by `ElementCenter`), and Page-level shortcuts that duplicate a `Mouse`/`Keyboard` primitive under a second name (`Click`/`ClickElement`/`Type`/`ClickAt`/`HoverAt`/`Hover`/`Wait`) — full accounting with one reason per method in `driver_page.go`'s `unwrappedPageMethods` |

No `Action` type was added for this widened surface — the three the fleet
constructs today (`ClickAction`/`TypeAction`/`NavigateAction`) and the two that
exist but are never constructed (`ScrollAction`/`WaitAction`) cover every real
caller; a new `Action` is cheap to add once one exists, and each must decide
native-vs-remote routing at construction time — see `action.go`'s doc comment
for the classification rule.

**Firefox native input caveat:** firefoxkit builds its native input window without the content offset, so CSS (0,0) maps to the window top-left (tabs/omnibox) rather than the content origin. Launch firefox with the BiDi/Remote backend and register it with `ExpectRemote()` — fully supported — until firefoxkit wires `MeasureContentOffset` into its native window. Chrome native input is unaffected.

## Capacity and worker pool

Native handles are capped machine-wide — the limit is one mouse and one keyboard, not nine Chrome windows:

```go
fleet := automationfleet.New(
    automationfleet.WithNativeCapacity(9),  // default 9, counted across every kit
    automationfleet.WithRemoteWorkers(4),   // initial remote (CDP/BiDi) pool size
)

used, max := fleet.NativeCapacity()        // 0, 9
mode, err := fleet.InputMode("chrome-1")   // InputNative / InputRemote
```

Registering past the cap returns `ErrNativeCapacityExceeded` **at `Register`**, and registers nothing. `Unregister` frees the slot immediately.

The remote pool resizes while the fleet runs:

```go
err := fleet.SetRemoteWorkers(12) // grow: spawns now
err = fleet.SetRemoteWorkers(2)   // shrink: surplus workers leave AFTER their current job
```

Shrinking never cancels work in flight. After `Stop` / `AbortAll` it returns `ErrFleetStopped` and spawns nothing. `fleet.Snapshot()` reports `NativeUsed`, `NativeMax` and the live `RemoteWorkers`.

## Dependencies

- `github.com/tuwibu/chromekit` v0.6.1 — per-browser Chrome library (native input + CDP).
- `github.com/tuwibu/firefoxkit` v0.1.0 — per-browser Firefox library (native input + BiDi).

Both are published modules, but `go.mod` currently carries `replace` directives
pointing both at sibling checkouts (`../chromekit`, `../firefoxkit`) for local
development against the widened Driver surface above. `go get
github.com/tuwibu/automationfleet` does **not** yet work standalone — the
remaining step is tagging a new chromekit/firefoxkit release that includes the
widened `MouseAPI`/`KeyboardAPI`/`Page` surface, then dropping both `replace`
lines and repointing `go.mod` at the tagged versions (see
`TestGoMod_NoReplaceDirective` in `parity_test.go`, currently skipped pending
that release).

## Status

| Phase | Status |
|------|--------|
| 01 — chromekit prep (mutex, HWND, Focus) | ✅ done |
| 02 — automationfleet bootstrap (types, skeleton) | ✅ done |
| 03 — dispatcher (queue, critical section) | ✅ done |
| 04 — hotkey listener | ✅ done |
| 05 — PoC (2-browser, stress 9, hotkey demo) | ✅ done |
| 06 — testing & stability (>80% coverage) | ⏳ in progress |
| 07 — hardening (timeouts, retry, logging) | ⏳ planned |
| 08 — documentation + examples | ⏳ planned |

See [`docs/development-roadmap.md`](docs/development-roadmap.md) for detailed phase breakdown.

## Stress suite (two tiers)

The stress suite is split by what it can touch:

**Tier 1 — fake driver, CI-safe.** Races, goroutine/heap leaks, queue priority,
lease semantics, pool resize, abort/pause churn. Runs as part of the normal
suite; must survive the race detector at `-count=10` without flaking:

```bash
go test -race -count=10 ./...
```

**Tier 2 — real browsers, real mouse and keyboard.** Behind the `integration`
build tag AND the `FLEET_ALLOW_REAL_INPUT=1` env var (tests self-skip without
it). It types and clicks on the live desktop — never run it on a machine with
sensitive windows open, and never wire it into CI:

```bash
# compile-only check (what CI does):
go vet -tags=integration ./...

# deliberate manual run on an idle desktop:
FLEET_ALLOW_REAL_INPUT=1 go test -tags=integration -run 'TestStressReal' -timeout=30m ./...
```

Tier 1 green does NOT prove foreground correctness: HWND contention, real
cursor drift, IME, and cross-window focus theft are only exercised by tier 2.
See `plans/260729-2027-native-foreground-and-kit-parity/stress-report.md` for
the full coverage map and the explicit list of what tier 1 cannot catch.

## Platform support

- **Windows**: full native input + hotkey listener.
- **Linux / macOS**: CDP fallback (no native cursor / hotkey). Fleet stays functional but loses the anti-bot benefits.
