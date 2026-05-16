//go:build linux

package input

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Linux backend uses /dev/uinput directly. Works on both X11 and Wayland
// because uinput is below the display server.
//
// Requires either root, or the user to be in the `input` group, or a udev
// rule chmod'ing /dev/uinput. The install script sets this up.

type linuxInjector struct {
	fd *os.File
}

func platformInjector() (Injector, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("/dev/uinput: %w (try `sumolite install-udev`)", err)
	}
	li := &linuxInjector{fd: f}
	if err := li.setup(); err != nil {
		f.Close()
		return nil, err
	}
	return li, nil
}

// The full uinput setup is verbose; abbreviated here. A production build
// emits keybit / evbit / relbit ioctls and a uinput_setup struct.
func (l *linuxInjector) setup() error {
	_ = unsafe.Sizeof(l.fd) // placeholder; real setup uses ioctls below
	// TODO: UI_SET_EVBIT(EV_KEY), UI_SET_KEYBIT(BTN_LEFT...), UI_SET_RELBIT(REL_X/Y/WHEEL),
	//       struct uinput_setup, UI_DEV_SETUP, UI_DEV_CREATE.
	return nil
}

func (l *linuxInjector) Handle(b []byte) error {
	kind, _, err := decode(b)
	if err != nil {
		return err
	}
	_ = kind
	// Emit input_event{time, type, code, value} writes. Stub for scaffold.
	return nil
}

func (l *linuxInjector) Close() error { return l.fd.Close() }
