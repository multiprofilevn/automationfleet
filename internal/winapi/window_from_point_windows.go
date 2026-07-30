//go:build windows

package winapi

var (
	procWindowFromPoint = user32.NewProc("WindowFromPoint")
	procGetAncestor     = user32.NewProc("GetAncestor")
)

// gaRoot is GetAncestor's GA_ROOT: walk up to the top-level window, stopping
// before owner windows.
const gaRoot = 2

// WindowFromPoint returns the HWND under the given screen coordinates, i.e.
// the window a synthetic mouse click would actually land in. This is a
// different target from the foreground window (where keystrokes go) — guarding
// the foreground does not guard a click.
//
// Returns 0 with an error when no window sits at that point.
//
// POINT is an 8-byte struct passed by value, which the Windows x64 / arm64
// calling convention packs into a single register — hence the packed uintptr.
func WindowFromPoint(x, y int) (uintptr, error) {
	packed := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32
	hwnd, _, errno := procWindowFromPoint.Call(packed)
	if hwnd == 0 {
		return 0, errno
	}
	return hwnd, nil
}

// RootWindow maps an HWND to its top-level ancestor. Chrome renders page
// content in a child window, so a legitimate click reports the child — the
// click guard compares roots, not raw handles. Returns hwnd unchanged when the
// call fails, so a guard built on it degrades to "accept" rather than
// rejecting valid clicks.
func RootWindow(hwnd uintptr) uintptr {
	if hwnd == 0 {
		return 0
	}
	root, _, _ := procGetAncestor.Call(hwnd, gaRoot)
	if root == 0 {
		return hwnd
	}
	return root
}

// CurrentThreadID returns the calling OS thread's id. Used by tests to prove
// the native critical section stays pinned to one thread (runtime.LockOSThread),
// which is what makes the thread-scoped keyboard layout switch meaningful.
func CurrentThreadID() uint32 {
	id, _, _ := procGetCurrentThreadId.Call()
	return uint32(id)
}
