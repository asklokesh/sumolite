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
	ev, err := Parse(b)
	if err != nil {
		return err
	}
	switch ev.Kind {
	case EvMouseMove:
		d.lastX, d.lastY = ev.X, ev.Y
		C.postMouseMove(C.double(ev.X), C.double(ev.Y))
	case EvMouseButton:
		down := 0
		if ev.Down {
			down = 1
		}
		C.postMouseButton(C.double(d.lastX), C.double(d.lastY), C.int(ev.Button), C.int(down))
	case EvScroll:
		C.postScroll(C.int(ev.X), C.int(ev.Y))
	case EvKey:
		down := 0
		if ev.Down {
			down = 1
		}
		C.postKey(C.int(ev.Key), C.int(down))
	}
	return nil
}

func (d *darwinInjector) Close() error { return nil }
