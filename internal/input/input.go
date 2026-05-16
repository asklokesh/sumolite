// Package input decodes the browser's input events from the data channel
// and replays them as platform input. The wire format is a tiny binary
// protocol (versioned by the event-type byte) so we don't pay JSON parse
// on every mouse move.
package input

import "encoding/binary"

const (
	EvMouseMove   byte = 1
	EvMouseButton byte = 2
	EvScroll      byte = 3
	EvKey         byte = 4
)

// Injector replays decoded events as platform input. Implementations are
// platform-specific (CGEvent on darwin, /dev/uinput on linux).
type Injector interface {
	Handle(b []byte) error
	Close() error
}

func New() (Injector, error) { return platformInjector() }

func i16(p []byte, off int) int16  { return int16(binary.LittleEndian.Uint16(p[off:])) }
func u16(p []byte, off int) uint16 { return binary.LittleEndian.Uint16(p[off:]) }
