// Package input decodes the browser's input events from the data channel
// and replays them as platform input. The wire format is a tiny binary
// protocol (versioned) so we don't pay JSON parse on every mouse move.
package input

import (
	"encoding/binary"
	"fmt"
)

const (
	EvMouseMove  = 1 // x int16, y int16
	EvMouseButton = 2 // button uint8, down uint8
	EvScroll     = 3 // dx int16, dy int16
	EvKey        = 4 // keycode uint16, down uint8
)

type Injector interface {
	Handle(b []byte) error
	Close() error
}

func New() (Injector, error) {
	return platformInjector()
}

// decode is shared by all platform backends.
func decode(b []byte) (kind byte, p []byte, err error) {
	if len(b) < 1 {
		return 0, nil, fmt.Errorf("short event")
	}
	return b[0], b[1:], nil
}

func i16(p []byte, off int) int16 { return int16(binary.LittleEndian.Uint16(p[off:])) }
func u16(p []byte, off int) uint16 { return binary.LittleEndian.Uint16(p[off:]) }
