package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <math.h>

static bool r7_rect(AXUIElementRef win, CGRect *rect) {
	CFTypeRef position = NULL, size = NULL;
	bool ok = AXUIElementCopyAttributeValue(win, kAXPositionAttribute, &position) == kAXErrorSuccess
		&& AXUIElementCopyAttributeValue(win, kAXSizeAttribute, &size) == kAXErrorSuccess
		&& position && size && CFGetTypeID(position) == AXValueGetTypeID() && CFGetTypeID(size) == AXValueGetTypeID()
		&& AXValueGetValue(position, kAXValueCGPointType, &rect->origin)
		&& AXValueGetValue(size, kAXValueCGSizeType, &rect->size);
	if (position) CFRelease(position);
	if (size) CFRelease(size);
	return ok;
}

static bool r7_same_rect(CGRect a, CGRect b) {
	return fabs(a.origin.x-b.origin.x) < 1 && fabs(a.origin.y-b.origin.y) < 1
		&& fabs(a.size.width-b.size.width) < 1 && fabs(a.size.height-b.size.height) < 1;
}

typedef struct { int pid; unsigned int window, display; CGRect bounds, desktop; size_t pw, ph; char title[1024]; } R7Scene;

static int r7_scene(R7Scene *out) {
	AXUIElementRef system = AXUIElementCreateSystemWide();
	CFTypeRef app = NULL, win = NULL, title = NULL;
	int status = 1;
	if (!system || AXUIElementCopyAttributeValue(system, kAXFocusedApplicationAttribute, &app) != kAXErrorSuccess || !app) goto done;
	if (AXUIElementGetPid((AXUIElementRef)app, &out->pid) != kAXErrorSuccess
		|| AXUIElementCopyAttributeValue((AXUIElementRef)app, kAXFocusedWindowAttribute, &win) != kAXErrorSuccess || !win
		|| !r7_rect((AXUIElementRef)win, &out->bounds)) goto done;
	if (AXUIElementCopyAttributeValue((AXUIElementRef)win, kAXTitleAttribute, &title) != kAXErrorSuccess
		|| !title || CFGetTypeID(title) != CFStringGetTypeID()
		|| !CFStringGetCString((CFStringRef)title, out->title, sizeof(out->title), kCFStringEncodingUTF8)) goto done;
	// Public CG window numbers distinguish same-title windows; all AX copies are released.
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (!list) goto done;
	for (CFIndex i=0; i<CFArrayGetCount(list); i++) {
		CFDictionaryRef entry = (CFDictionaryRef)CFArrayGetValueAtIndex(list,i);
		int pid=0, layer=0; CGRect rect;
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowOwnerPID),kCFNumberIntType,&pid);
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowLayer),kCFNumberIntType,&layer);
		if (pid != out->pid || layer != 0 || !CGRectMakeWithDictionaryRepresentation(CFDictionaryGetValue(entry,kCGWindowBounds),&rect)
			|| !r7_same_rect(rect,out->bounds)) continue;
		if (out->window) { out->window=0; break; }
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowNumber),kCFNumberIntType,&out->window);
	}
	CFRelease(list);
	if (!out->window) goto done;
	CGDirectDisplayID displays[32]; uint32_t count=0;
	if (CGGetActiveDisplayList(32,displays,&count) != kCGErrorSuccess || count == 32) goto done;
	for (uint32_t i=0;i<count;i++) {
		CGRect desktop = CGDisplayBounds(displays[i]);
		if (!CGRectContainsRect(desktop,out->bounds)) continue;
		if (out->display) { out->display=0; break; }
		out->display=displays[i]; out->desktop=desktop;
		out->pw=CGDisplayPixelsWide(displays[i]); out->ph=CGDisplayPixelsHigh(displays[i]);
	}
	if (out->display && out->pw && out->ph) status=0;
done:
	if (title) CFRelease(title);
	if (win) CFRelease(win);
	if (app) CFRelease(app);
	if (system) CFRelease(system);
	return status;
}

static bool r7_visible(unsigned int window, double x, double y) {
	bool visible=false;
	CFArrayRef list=CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,kCGNullWindowID);
	if (!list) return false;
	for (CFIndex i=0;i<CFArrayGetCount(list);i++) {
		CFDictionaryRef entry=(CFDictionaryRef)CFArrayGetValueAtIndex(list,i);
		CGRect bounds; double alpha=0; unsigned int id=0;
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowAlpha),kCFNumberDoubleType,&alpha);
		if (alpha <= 0 || !CGRectMakeWithDictionaryRepresentation(CFDictionaryGetValue(entry,kCGWindowBounds),&bounds)
			|| !CGRectContainsPoint(bounds,CGPointMake(x,y))) continue;
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowNumber),kCFNumberIntType,&id);
		visible=id==window; break;
	}
	CFRelease(list); return visible;
}

static bool r7_raise(int pid, unsigned int window) {
	CGRect target=CGRectZero; bool found=false;
	CFArrayRef list=CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,kCGNullWindowID);
	if (!list) return false;
	for (CFIndex i=0;i<CFArrayGetCount(list);i++) {
		CFDictionaryRef entry=(CFDictionaryRef)CFArrayGetValueAtIndex(list,i);
		int owner=0; unsigned int id=0;
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowOwnerPID),kCFNumberIntType,&owner);
		CFNumberGetValue(CFDictionaryGetValue(entry,kCGWindowNumber),kCFNumberIntType,&id);
		if (owner==pid && id==window) { found=CGRectMakeWithDictionaryRepresentation(CFDictionaryGetValue(entry,kCGWindowBounds),&target); break; }
	}
	CFRelease(list);
	if (!found) return false;
	AXUIElementRef app=AXUIElementCreateApplication(pid);
	CFTypeRef windows=NULL; AXUIElementRef selected=NULL;
	bool ok=false;
	if (AXUIElementCopyAttributeValue(app,kAXWindowsAttribute,&windows)==kAXErrorSuccess && windows && CFGetTypeID(windows)==CFArrayGetTypeID()) {
		for (CFIndex i=0;i<CFArrayGetCount((CFArrayRef)windows);i++) {
			AXUIElementRef win=(AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)windows,i); CGRect rect;
			if (!r7_rect(win,&rect) || !r7_same_rect(rect,target)) continue;
			if (selected) { selected=NULL; break; }
			selected=win;
		}
		ok=selected && AXUIElementPerformAction(selected,kAXRaiseAction)==kAXErrorSuccess
			&& AXUIElementSetAttributeValue(app,kAXFocusedWindowAttribute,selected)==kAXErrorSuccess;
	}
	if (windows) CFRelease(windows); CFRelease(app); return ok;
}

static CGPoint r7_cursor(void) {
	CGEventRef event=CGEventCreate(NULL);
	CGPoint p=CGPointMake(NAN,NAN);
	if (event) { p=CGEventGetLocation(event); CFRelease(event); }
	return p;
}
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-vgo/robotgo"
)

func windowedPreflight() error {
	if C.AXIsProcessTrusted() == 0 {
		return fmt.Errorf("-windowed requires Accessibility permission for the app running the bot")
	}
	if !bool(C.CGPreflightScreenCaptureAccess()) {
		return fmt.Errorf("-windowed requires Screen Recording permission for the app running the bot")
	}
	return nil
}

func cgRectangle(r C.CGRect) image.Rectangle {
	return image.Rect(int(math.Floor(float64(r.origin.x))), int(math.Floor(float64(r.origin.y))),
		int(math.Ceil(float64(r.origin.x+r.size.width))), int(math.Ceil(float64(r.origin.y+r.size.height))))
}

func windowedHookPreflight() error {
	if !bool(C.CGPreflightListenEventAccess()) {
		return fmt.Errorf("-windowed run requires Input Monitoring permission for the global F8 hook")
	}
	return nil
}

func readNativeScene() (nativeScene, error) {
	var s C.R7Scene
	if C.r7_scene(&s) != 0 {
		return nativeScene{}, fmt.Errorf("focused window geometry unavailable, clipped or spanning displays")
	}
	if !isGameTitle(C.GoString(&s.title[0])) {
		return nativeScene{}, fmt.Errorf("focus the Clicker Heroes window and press F8")
	}
	return nativeScene{Window: fmt.Sprintf("%d:%d", int(s.pid), uint32(s.window)), Display: int(s.display),
		Bounds: cgRectangle(s.bounds), Desktop: cgRectangle(s.desktop), Pixels: image.Pt(int(s.pw), int(s.ph))}, nil
}

func captureNativeDisplay(s nativeScene) (image.Image, error) {
	return robotgo.CaptureImg(0, 0, s.Desktop.Dx(), s.Desktop.Dy(), s.Display)
}

func nativeTargetVisible(s nativeScene, p image.Point) bool {
	_, id, ok := strings.Cut(s.Window, ":")
	window, err := strconv.ParseUint(id, 10, 32)
	return ok && err == nil && bool(C.r7_visible(C.uint(window), C.double(p.X), C.double(p.Y)))
}

func focusNativeWindow(key string) error {
	pidText, id, ok := strings.Cut(key, ":")
	pid, e1 := strconv.Atoi(pidText)
	window, e2 := strconv.ParseUint(id, 10, 32)
	if !ok || e1 != nil || e2 != nil || pid <= 0 || window == 0 {
		return fmt.Errorf("invalid native window identity")
	}
	if !bool(C.r7_raise(C.int(pid), C.uint(window))) {
		return fmt.Errorf("cannot restore the original game window")
	}
	if err := robotgo.ActivePid(pid); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	scene, err := readNativeScene()
	if err != nil || scene.Window != key {
		return fmt.Errorf("game focus was not restored; focus it manually and press F8")
	}
	return nil
}

func nativeMousePosition() (image.Point, error) {
	p := C.r7_cursor()
	x, y := float64(p.x), float64(p.y)
	if math.IsNaN(x) || math.IsNaN(y) {
		return image.Point{}, fmt.Errorf("native cursor position unavailable")
	}
	return image.Pt(int(math.Round(x)), int(math.Round(y))), nil
}
func nativeMouseArgument(p image.Point) (image.Point, error) { return p, nil }
