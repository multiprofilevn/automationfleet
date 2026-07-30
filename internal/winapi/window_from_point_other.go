//go:build !windows

package winapi

// WindowFromPoint is unsupported off Windows — there is no HWND to compare
// against, so the click guard degrades to a no-op.
func WindowFromPoint(_, _ int) (uintptr, error) { return 0, errUnsupported }

// RootWindow is the identity function off Windows.
func RootWindow(hwnd uintptr) uintptr { return hwnd }

// CurrentThreadID returns 0 off Windows.
func CurrentThreadID() uint32 { return 0 }
