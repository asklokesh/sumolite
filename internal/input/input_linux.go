//go:build linux

package input

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// Linux backend uses /dev/uinput directly. Works on both X11 and Wayland
// because uinput is below the display server.
//
// Requires either root, or the user to be in the `input` group, or a
// udev rule chmod'ing /dev/uinput. The install script sets up the
// udev rule and group membership.

// Selected constants from <linux/uinput.h>, <linux/input-event-codes.h>.
// We hard-code these to avoid a cgo dependency on kernel headers.
const (
	uiSetEvBit   = 0x40045564 // _IOW(UINPUT_IOCTL_BASE, 100, int)
	uiSetKeyBit  = 0x40045565
	uiSetRelBit  = 0x40045566
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
	// UI_DEV_SETUP = _IOW('U', 3, struct uinput_setup) -> needs runtime sizeof
	uiAbsSetup = 0
)

const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02

	synReport = 0x00

	relX      = 0x00
	relY      = 0x01
	relWheel  = 0x08
	relHWheel = 0x06

	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	btnSide   = 0x113
	btnExtra  = 0x114

	// We expose KEY_RESERVED..KEY_MAX (~0x2FF) as a flat range when the
	// device is registered, so any keycode the browser sends through is
	// valid.
	keyMax = 0x2FF
)

// uiDevSetup mirrors `struct uinput_setup` from <linux/uinput.h>.
//
//	struct input_id id;
//	char name[UINPUT_MAX_NAME_SIZE]; // 80
//	__u32 ff_effects_max;
//
// struct input_id is 8 bytes (bustype, vendor, product, version u16).
type uiDevSetup struct {
	BusType uint16
	Vendor  uint16
	Product uint16
	Version uint16
	Name    [80]byte
	FFMax   uint32
}

// inputEvent mirrors the kernel struct input_event with timeval.
// On 64-bit systems timeval is 16 bytes (sec int64, usec int64).
type inputEvent struct {
	TVSec  int64
	TVUSec int64
	Type   uint16
	Code   uint16
	Value  int32
}

const uiDevSetupReq = 0x405c5503 // _IOW('U', 3, struct uinput_setup) — sizeof = 92

type linuxInjector struct {
	f            *os.File
	lastX, lastY int16
}

func platformInjector() (Injector, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("/dev/uinput: %w (run install.sh to set up the udev rule)", err)
	}
	li := &linuxInjector{f: f}
	if err := li.setup(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return li, nil
}

func (l *linuxInjector) setup() error {
	fd := l.f.Fd()

	// Enable event types.
	if err := ioctl(fd, uiSetEvBit, evKey); err != nil {
		return fmt.Errorf("UI_SET_EVBIT EV_KEY: %w", err)
	}
	if err := ioctl(fd, uiSetEvBit, evRel); err != nil {
		return fmt.Errorf("UI_SET_EVBIT EV_REL: %w", err)
	}

	// Enable relative axes.
	for _, axis := range []uintptr{relX, relY, relWheel, relHWheel} {
		if err := ioctl(fd, uiSetRelBit, axis); err != nil {
			return fmt.Errorf("UI_SET_RELBIT %d: %w", axis, err)
		}
	}

	// Enable mouse buttons.
	for _, b := range []uintptr{btnLeft, btnRight, btnMiddle, btnSide, btnExtra} {
		if err := ioctl(fd, uiSetKeyBit, b); err != nil {
			return fmt.Errorf("UI_SET_KEYBIT %#x: %w", b, err)
		}
	}

	// Enable the keyboard key range. Browser keycodes are remapped to
	// Linux keycodes by the (future) layout layer; for now we expose the
	// full keyboard codepoint range.
	for k := uintptr(1); k <= keyMax; k++ {
		_ = ioctl(fd, uiSetKeyBit, k)
	}

	// Register the device.
	setup := uiDevSetup{
		BusType: 0x03, // BUS_USB
		Vendor:  0x5311,
		Product: 0x534c, // "SL"
		Version: 1,
	}
	copy(setup.Name[:], "sumolite-virtual-input")

	if err := ioctlPtr(fd, uiDevSetupReq, unsafe.Pointer(&setup)); err != nil {
		return fmt.Errorf("UI_DEV_SETUP: %w", err)
	}
	if err := ioctl(fd, uiDevCreate, 0); err != nil {
		return fmt.Errorf("UI_DEV_CREATE: %w", err)
	}

	// The kernel takes ~50ms to wire up the new device on the input
	// subsystem; events sent before that are dropped on the floor.
	time.Sleep(80 * time.Millisecond)
	return nil
}

func (l *linuxInjector) Handle(b []byte) error {
	ev, err := Parse(b)
	if err != nil {
		return err
	}
	// Track the last position so a MOUSE_BUTTON event has somewhere to land.
	// We currently emit relative motion (REL_X/REL_Y) deltas; the browser
	// could equally well send absolute positions if we registered ABS axes
	// with the screen resolution. Relative is simpler and works under
	// portal-mediated input capture on Wayland.
	switch ev.Kind {
	case EvMouseMove:
		dx, dy := int32(ev.X)-int32(l.lastX), int32(ev.Y)-int32(l.lastY)
		l.lastX, l.lastY = ev.X, ev.Y
		if dx != 0 {
			l.emit(evRel, relX, dx)
		}
		if dy != 0 {
			l.emit(evRel, relY, dy)
		}
		l.emit(evSyn, synReport, 0)
	case EvMouseButton:
		code := uint16(btnLeft + uint16(ev.Button))
		switch ev.Button {
		case 0:
			code = btnLeft
		case 1:
			code = btnMiddle // browser button=1 == middle; right is button=2
		case 2:
			code = btnRight
		case 3:
			code = btnSide
		case 4:
			code = btnExtra
		}
		val := int32(0)
		if ev.Down {
			val = 1
		}
		l.emit(evKey, code, val)
		l.emit(evSyn, synReport, 0)
	case EvScroll:
		if ev.X != 0 {
			l.emit(evRel, relHWheel, int32(ev.X))
		}
		if ev.Y != 0 {
			// Browser wheel deltas are pixels and positive == down;
			// Linux REL_WHEEL is "notches" positive == up. Invert and
			// clamp to one notch per ~120px to feel reasonable.
			notches := -int32(ev.Y) / 40
			if notches == 0 {
				if ev.Y > 0 {
					notches = -1
				} else {
					notches = 1
				}
			}
			l.emit(evRel, relWheel, notches)
		}
		l.emit(evSyn, synReport, 0)
	case EvKey:
		val := int32(0)
		if ev.Down {
			val = 1
		}
		l.emit(evKey, ev.Key, val)
		l.emit(evSyn, synReport, 0)
	}
	return nil
}

func (l *linuxInjector) emit(typ, code uint16, val int32) {
	ev := inputEvent{Type: typ, Code: code, Value: val}
	var buf [24]byte
	binary.LittleEndian.PutUint64(buf[0:], uint64(ev.TVSec))
	binary.LittleEndian.PutUint64(buf[8:], uint64(ev.TVUSec))
	binary.LittleEndian.PutUint16(buf[16:], ev.Type)
	binary.LittleEndian.PutUint16(buf[18:], ev.Code)
	binary.LittleEndian.PutUint32(buf[20:], uint32(ev.Value))
	_, _ = l.f.Write(buf[:])
}

func (l *linuxInjector) Close() error {
	_ = ioctl(l.f.Fd(), uiDevDestroy, 0)
	return l.f.Close()
}

func ioctl(fd, req, arg uintptr) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	if e != 0 {
		return e
	}
	return nil
}

func ioctlPtr(fd, req uintptr, p unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}
