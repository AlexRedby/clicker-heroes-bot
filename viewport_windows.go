//go:build windows

package main

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/go-vgo/robotgo"
	"golang.org/x/sys/windows"
)

// The Win32 calls live here so the windowed path has no capture/input backend
// of its own. Coordinates are physical pixels because the process is PMv2.
var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	setDPI              = user32.NewProc("SetProcessDpiAwarenessContext")
	getThreadDPIContext = user32.NewProc("GetThreadDpiAwarenessContext")
	dpiContextsEqual    = user32.NewProc("AreDpiAwarenessContextsEqual")
	getForeground       = user32.NewProc("GetForegroundWindow")
	getWindowText       = user32.NewProc("GetWindowTextW")
	getClientRect       = user32.NewProc("GetClientRect")
	clientToScreen      = user32.NewProc("ClientToScreen")
	monitorFromWindow   = user32.NewProc("MonitorFromWindow")
	getMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	getDpiForWindow     = user32.NewProc("GetDpiForWindow")
	windowFromPoint     = user32.NewProc("WindowFromPoint")
	getAncestor         = user32.NewProc("GetAncestor")
	setForeground       = user32.NewProc("SetForegroundWindow")
	getCursorPos        = user32.NewProc("GetCursorPos")
)

type winRect struct{ left, top, right, bottom int32 }
type winPoint struct{ x, y int32 }
type monitorInfo struct {
	cbSize            uint32
	rcMonitor, rcWork winRect
	flags             uint32
}

const (
	dpiAwarenessContextPMv2 = ^uintptr(3) // (HANDLE)-4
	monitorDefaultToNearest = 2
	gaRoot                  = 2
)

func winErr(name string, err error) error { return fmt.Errorf("%s: %w", name, err) }

func windowKey(hwnd windows.HWND, pid uint32) string {
	return strconv.FormatUint(uint64(pid), 10) + ":" + strconv.FormatUint(uint64(hwnd), 10)
}

func parseWindowKey(key string) (windows.HWND, uint32, error) {
	a, b, ok := strings.Cut(key, ":")
	if !ok || a == "" || b == "" {
		return 0, 0, fmt.Errorf("invalid window identity %q", key)
	}
	pid, err := strconv.ParseUint(a, 10, 32)
	if err != nil || pid == 0 {
		return 0, 0, fmt.Errorf("invalid process id in %q", key)
	}
	h, err := strconv.ParseUint(b, 10, strconv.IntSize)
	if err != nil || h == 0 {
		return 0, 0, fmt.Errorf("invalid window handle in %q", key)
	}
	return windows.HWND(h), uint32(pid), nil
}

func windowTitle(hwnd windows.HWND) (string, error) {
	buf := make([]uint16, 512)
	n, _, err := getWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 && err != windows.ERROR_SUCCESS {
		return "", winErr("GetWindowTextW", err)
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func windowedPreflight() error {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("-windowed requires 64-bit Windows")
	}
	for _, proc := range []*windows.LazyProc{setDPI, getThreadDPIContext, dpiContextsEqual} {
		if err := proc.Find(); err != nil {
			return winErr("DPI awareness API unavailable", err)
		}
	}
	r, _, err := setDPI.Call(dpiAwarenessContextPMv2)
	if r != 0 {
		return nil
	}
	// The process may already have selected its awareness. Confirm the exact
	// PMv2 context rather than accepting a merely DPI-aware process.
	current, _, contextErr := getThreadDPIContext.Call()
	if current != 0 {
		equal, _, equalErr := dpiContextsEqual.Call(current, dpiAwarenessContextPMv2)
		if equal != 0 {
			return nil
		}
		if equalErr != windows.ERROR_SUCCESS {
			contextErr = equalErr
		}
	}
	if err == windows.ERROR_SUCCESS {
		err = contextErr
	}
	if err == windows.ERROR_SUCCESS {
		err = windows.ERROR_CALL_NOT_IMPLEMENTED
	}
	return winErr("SetProcessDpiAwarenessContext", err)
}

func readNativeScene() (nativeScene, error) {
	var scene nativeScene
	h, _, _ := getForeground.Call()
	if h == 0 {
		return scene, fmt.Errorf("foreground window unavailable")
	}
	hwnd := windows.HWND(h)
	title, err := windowTitle(hwnd)
	if err != nil {
		return scene, err
	}
	if !isGameTitle(title) {
		return scene, fmt.Errorf("foreground window %q is not the game", title)
	}
	vis, _, _ := user32.NewProc("IsWindowVisible").Call(h)
	iconic, _, _ := user32.NewProc("IsIconic").Call(h)
	if vis == 0 || iconic != 0 {
		return scene, fmt.Errorf("game window is hidden or minimized")
	}
	var pid uint32
	if _, e := windows.GetWindowThreadProcessId(hwnd, &pid); e != nil {
		return scene, winErr("GetWindowThreadProcessId", e)
	}
	if pid == 0 {
		return scene, fmt.Errorf("GetWindowThreadProcessId returned no process")
	}
	var client winRect
	if r, _, e := getClientRect.Call(h, uintptr(unsafe.Pointer(&client))); r == 0 {
		return scene, winErr("GetClientRect", e)
	}
	tl, br := winPoint{client.left, client.top}, winPoint{client.right, client.bottom}
	if r, _, e := clientToScreen.Call(h, uintptr(unsafe.Pointer(&tl))); r == 0 {
		return scene, winErr("ClientToScreen", e)
	}
	if r, _, e := clientToScreen.Call(h, uintptr(unsafe.Pointer(&br))); r == 0 {
		return scene, winErr("ClientToScreen", e)
	}
	window := image.Rect(int(tl.x), int(tl.y), int(br.x), int(br.y))
	if window.Empty() {
		return scene, fmt.Errorf("game client rectangle is empty")
	}
	mon, _, e := monitorFromWindow.Call(h, monitorDefaultToNearest)
	if mon == 0 {
		return scene, winErr("MonitorFromWindow", e)
	}
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, e := getMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return scene, winErr("GetMonitorInfoW", e)
	}
	desktop := image.Rect(int(mi.rcMonitor.left), int(mi.rcMonitor.top), int(mi.rcMonitor.right), int(mi.rcMonitor.bottom))
	if !window.In(desktop) || !image.Pt(window.Max.X-1, window.Max.Y-1).In(desktop) {
		return scene, fmt.Errorf("game client spans multiple monitors")
	}
	dpi, _, e := getDpiForWindow.Call(h)
	if dpi == 0 {
		return scene, winErr("GetDpiForWindow", e)
	}
	scene.Window = windowKey(hwnd, pid)
	scene.Display = int(mon)
	scene.Bounds = window
	scene.Desktop = desktop
	scene.Pixels = image.Pt(desktop.Dx(), desktop.Dy())
	scene.DPI = uint32(dpi)
	return scene, nil
}

func captureNativeDisplay(scene nativeScene) (image.Image, error) {
	if scene.Desktop.Empty() {
		return nil, fmt.Errorf("display bounds are empty")
	}
	r := scene.Desktop
	img, err := robotgo.CaptureImg(r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	if err != nil {
		return nil, winErr("CaptureImg", err)
	}
	if img == nil {
		return nil, fmt.Errorf("CaptureImg returned no image")
	}
	return img, nil
}

func nativeTargetVisible(scene nativeScene, point image.Point) bool {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return false
	}
	hwnd, _, err := parseWindowKey(scene.Window)
	if err != nil || !point.In(scene.Desktop) {
		return false
	}
	var p winPoint = winPoint{int32(point.X), int32(point.Y)}
	got, _, _ := windowFromPoint.Call(*(*uintptr)(unsafe.Pointer(&p)))
	root, _, _ := getAncestor.Call(got, gaRoot)
	target, _, _ := getAncestor.Call(uintptr(hwnd), gaRoot)
	return got != 0 && root == target
}

func focusNativeWindow(key string) error {
	hwnd, pid, err := parseWindowKey(key)
	if err != nil {
		return err
	}
	var actual uint32
	if _, err = windows.GetWindowThreadProcessId(hwnd, &actual); err != nil || actual != pid {
		return fmt.Errorf("window identity changed")
	}
	if r, _, _ := user32.NewProc("IsWindow").Call(uintptr(hwnd)); r == 0 {
		return fmt.Errorf("window is no longer valid")
	}
	if r, _, e := setForeground.Call(uintptr(hwnd)); r == 0 {
		return winErr("SetForegroundWindow", e)
	}
	fg, _, _ := getForeground.Call()
	if fg != uintptr(hwnd) {
		return fmt.Errorf("SetForegroundWindow was denied")
	}
	return nil
}

func nativeMousePosition() (image.Point, error) {
	var p winPoint
	if r, _, e := getCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return image.Point{}, winErr("GetCursorPos", e)
	}
	return image.Pt(int(p.x), int(p.y)), nil
}

func nativeMouseArgument(p image.Point) (image.Point, error) {
	scale := robotgo.ScaleF()
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return image.Point{}, fmt.Errorf("invalid RobotGo scale %v", scale)
	}
	round := func(v int) int {
		f := float64(v) * scale
		if v < 0 {
			return int(math.Floor(f))
		}
		return int(math.Ceil(f))
	}
	arg := image.Pt(round(p.X), round(p.Y))
	gotX, gotY := robotgo.MoveScale(arg.X, arg.Y)
	if absInt(gotX-p.X) > 1 || absInt(gotY-p.Y) > 1 {
		return image.Point{}, fmt.Errorf("RobotGo scale roundtrip drift: input %v, got %v", p, image.Pt(gotX, gotY))
	}
	return arg, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func windowedHookPreflight() error { return nil }
