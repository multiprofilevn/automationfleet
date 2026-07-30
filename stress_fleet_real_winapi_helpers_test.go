//go:build integration && windows

package automationfleet

// Raw Win32 helpers for the tier-2 real stress suite — split from
// stress_fleet_real_helpers_test.go (200-line rule). Lazy procs rather than
// x/sys wrappers where the wrapper does not exist (SetWindowPos, SetCursorPos,
// GetKeyboardLayout, GetProcessHandleCount).

import (
	"os"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ------------------------------------------------------- raw OS helpers ----

var (
	user32Stress        = syscall.NewLazyDLL("user32.dll")
	kernel32Stress      = syscall.NewLazyDLL("kernel32.dll")
	procSetWindowPos    = user32Stress.NewProc("SetWindowPos")
	procSetCursorPos    = user32Stress.NewProc("SetCursorPos")
	procGetKbdLayout    = user32Stress.NewProc("GetKeyboardLayout")
	procProcHandleCount = kernel32Stress.NewProc("GetProcessHandleCount")
)

// moveWindowTo repositions a real top-level window (SWP_NOSIZE keeps its size).
func moveWindowTo(t *testing.T, hwnd uintptr, x, y int) {
	t.Helper()
	const swpNoSize, swpNoZOrder, swpNoActivate = 0x0001, 0x0004, 0x0010
	r, _, err := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate)
	if r == 0 {
		t.Fatalf("SetWindowPos(0x%x → %d,%d): %v", hwnd, x, y, err)
	}
}

// overlayWindowOn places hwnd topmost exactly over the given rect.
func overlayWindowOn(t *testing.T, hwnd uintptr, r Rect) {
	t.Helper()
	const hwndTopmost = ^uintptr(0) // (HWND)-1
	ret, _, err := procSetWindowPos.Call(hwnd, hwndTopmost,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0)
	if ret == 0 {
		t.Fatalf("SetWindowPos topmost overlay: %v", err)
	}
}

func setCursorTo(x, y int) { procSetCursorPos.Call(uintptr(x), uintptr(y)) } //nolint:errcheck

// keyboardLayoutOf returns the HKL of one thread — GetKeyboardLayout is
// THREAD-scoped, which is the entire point of TestStressReal_KeyboardLayoutThreadRestored.
func keyboardLayoutOf(tid uint32) uintptr {
	hkl, _, _ := procGetKbdLayout.Call(uintptr(tid))
	return hkl
}

func isENUS(hkl uintptr) bool { return uint16(hkl&0xFFFF) == 0x0409 }

// processThreadLayouts snapshots tid → keyboard layout for every thread of
// THIS process.
func processThreadLayouts(t *testing.T) map[uint32]uintptr {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		t.Fatalf("thread snapshot: %v", err)
	}
	defer windows.CloseHandle(snap)
	pid := uint32(os.Getpid())
	out := make(map[uint32]uintptr)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID == pid {
			out[te.ThreadID] = keyboardLayoutOf(te.ThreadID)
		}
	}
	return out
}

// ownProcessHandleCount reports the Windows handle count of this process —
// the tier-2 leak metric the fake tier cannot measure.
func ownProcessHandleCount(t *testing.T) uint32 {
	t.Helper()
	var n uint32
	r, _, err := procProcHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return n
}

// processAlive reports whether pid is still running (orphan detection).
func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
