//go:build darwin

package input

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>

static void postMouseMove(double x, double y) {
    CGEventRef e = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
static void postMouseButton(double x, double y, int button, int down) {
    CGEventType t;
    CGMouseButton b = (CGMouseButton)button;
    if (button == 0)      t = down ? kCGEventLeftMouseDown   : kCGEventLeftMouseUp;
    else if (button == 1) t = down ? kCGEventRightMouseDown  : kCGEventRightMouseUp;
    else                  t = down ? kCGEventOtherMouseDown  : kCGEventOtherMouseUp;
    CGEventRef e = CGEventCreateMouseEvent(NULL, t, CGPointMake(x, y), b);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
static void postScroll(int dx, int dy) {
    CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 2, dy, dx);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
static void postKey(int keycode, int down) {
    CGEventRef e = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
*/
import "C"

type darwinInjector struct {
	lastX, lastY int16
}

func platformInjector() (Injector, error) {
	// macOS requires Accessibility / Screen Recording permission; the user
	// will see a system prompt on first event. We don't block on that.
	return &darwinInjector{}, nil
}

func (d *darwinInjector) Handle(b []byte) error {
	kind, p, err := decode(b)
	if err != nil {
		return err
	}
	switch kind {
	case EvMouseMove:
		if len(p) < 4 {
			return nil
		}
		x, y := i16(p, 0), i16(p, 2)
		d.lastX, d.lastY = x, y
		C.postMouseMove(C.double(x), C.double(y))
	case EvMouseButton:
		if len(p) < 2 {
			return nil
		}
		C.postMouseButton(C.double(d.lastX), C.double(d.lastY), C.int(p[0]), C.int(p[1]))
	case EvScroll:
		if len(p) < 4 {
			return nil
		}
		C.postScroll(C.int(i16(p, 0)), C.int(i16(p, 2)))
	case EvKey:
		if len(p) < 3 {
			return nil
		}
		C.postKey(C.int(u16(p, 0)), C.int(p[2]))
	}
	return nil
}

func (d *darwinInjector) Close() error { return nil }
