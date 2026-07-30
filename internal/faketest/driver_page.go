package faketest

import (
	"context"
	"time"
)

// ------------------------------------------------------------------ page ----

// Evaluate mirrors Page.Evaluate.
func (d *Driver) Evaluate(expr string, _ any) error {
	sleep(d.record("evaluate", expr, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.evalErr
}

// Navigate mirrors Page.Navigate.
func (d *Driver) Navigate(url string, _ time.Duration) error {
	sleep(d.record("navigate", url, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.navErr
}

// WaitForSelector mirrors Page.WaitForSelector.
func (d *Driver) WaitForSelector(sel string, _ time.Duration) error {
	sleep(d.record("waitforselector", sel, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.waitErr
}

// ElementCenter mirrors Page.ElementCenter.
func (d *Driver) ElementCenter(sel string) (float64, float64, error) {
	sleep(d.record("elementcenter", sel, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.centerX, d.centerY, d.centerErr
}

// ReadValue mirrors Page.ReadValue — reading through the protocol never needs
// the keyboard, which is what makes verify-then-repair safe.
func (d *Driver) ReadValue(sel string) (string, error) {
	sleep(d.record("readvalue", sel, 0, 0))
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readErr != nil {
		return "", d.readErr
	}
	return d.value, nil
}

// -------------------------------------------------- page (widened, phase 8) --
//
// Every method below records its op and returns the generic error injected
// via SetErr(op, err) (see extraErr in driver_transcript.go) — no test needs
// per-op tuning beyond "fail this call" yet, so a dedicated field per method
// would only be unused surface.

func (d *Driver) EvalSelector(sel, fnBody string, _ any) error {
	sleep(d.record("evalselector", sel, 0, 0))
	return d.extraErr("evalselector")
}

func (d *Driver) EvalSelectorAll(sel, jsFn string, _ any) error {
	sleep(d.record("evalselectorall", sel, 0, 0))
	return d.extraErr("evalselectorall")
}

func (d *Driver) EvaluateCtx(_ context.Context, expression string, _ any) error {
	sleep(d.record("evaluatectx", expression, 0, 0))
	return d.extraErr("evaluatectx")
}

func (d *Driver) Reload(_ time.Duration) error {
	sleep(d.record("reload", "", 0, 0))
	return d.extraErr("reload")
}

func (d *Driver) GoBack(_ time.Duration) error {
	sleep(d.record("goback", "", 0, 0))
	return d.extraErr("goback")
}

func (d *Driver) GoForward(_ time.Duration) error {
	sleep(d.record("goforward", "", 0, 0))
	return d.extraErr("goforward")
}

func (d *Driver) GetURL() (string, error) {
	sleep(d.record("geturl", "", 0, 0))
	return "", d.extraErr("geturl")
}

func (d *Driver) GetTitle() (string, error) {
	sleep(d.record("gettitle", "", 0, 0))
	return "", d.extraErr("gettitle")
}

func (d *Driver) GetOrigin() (string, error) {
	sleep(d.record("getorigin", "", 0, 0))
	return "", d.extraErr("getorigin")
}

func (d *Driver) URL() string {
	sleep(d.record("url", "", 0, 0))
	return ""
}

func (d *Driver) Title() string {
	sleep(d.record("title", "", 0, 0))
	return ""
}

func (d *Driver) GetText(sel string) (string, error) {
	sleep(d.record("gettext", sel, 0, 0))
	return "", d.extraErr("gettext")
}

func (d *Driver) GetAttribute(sel, attribute string) (string, error) {
	sleep(d.record("getattribute", sel+"/"+attribute, 0, 0))
	return "", d.extraErr("getattribute")
}

func (d *Driver) ScrollIntoView(sel string) error {
	sleep(d.record("scrollintoview", sel, 0, 0))
	return d.extraErr("scrollintoview")
}

// FocusSelector mirrors Page.Focus(selector) — named distinctly from Focus
// (driver_core.go, Driver.Focus(ctx)) because Go forbids two methods sharing
// a name on the same receiver even with different signatures. The fleet-side
// wrapper (fakePage.Focus) still exposes the correct Page.Focus signature.
func (d *Driver) FocusSelector(sel string) error {
	sleep(d.record("focusselector", sel, 0, 0))
	return d.extraErr("focusselector")
}

func (d *Driver) WaitForReady(_ time.Duration) error {
	sleep(d.record("waitforready", "", 0, 0))
	return d.extraErr("waitforready")
}

func (d *Driver) WaitForChangeURL(_ time.Duration) error {
	sleep(d.record("waitforchangeurl", "", 0, 0))
	return d.extraErr("waitforchangeurl")
}

func (d *Driver) WaitForFunction(expr string, _ time.Duration) error {
	sleep(d.record("waitforfunction", expr, 0, 0))
	return d.extraErr("waitforfunction")
}

func (d *Driver) WaitForSelectorGone(sel string, _ time.Duration) error {
	sleep(d.record("waitforselectorgone", sel, 0, 0))
	return d.extraErr("waitforselectorgone")
}

func (d *Driver) Screenshot() ([]byte, error) {
	sleep(d.record("screenshot", "", 0, 0))
	return nil, d.extraErr("screenshot")
}

func (d *Driver) ScreenshotClip(x, y, w, h float64) ([]byte, error) {
	sleep(d.record("screenshotclip", "", x, y))
	return nil, d.extraErr("screenshotclip")
}

func (d *Driver) ScreenshotElement(sel string) ([]byte, error) {
	sleep(d.record("screenshotelement", sel, 0, 0))
	return nil, d.extraErr("screenshotelement")
}

func (d *Driver) FullScreenshot(_ int) ([]byte, error) {
	sleep(d.record("fullscreenshot", "", 0, 0))
	return nil, d.extraErr("fullscreenshot")
}

func (d *Driver) SetInputFiles(sel string, paths ...string) error {
	sleep(d.record("setinputfiles", sel, 0, 0))
	return d.extraErr("setinputfiles")
}

// OnDialog mirrors Page.OnDialog. Stores the handler; nothing invokes it yet
// (no test simulates a JS dialog), consistent with the rest of this file
// recording-but-not-driving new behavior.
func (d *Driver) OnDialog(handler func(DialogEvent) DialogAction) {
	sleep(d.record("ondialog", "", 0, 0))
	d.mu.Lock()
	d.dialogHandler = handler
	d.mu.Unlock()
}
