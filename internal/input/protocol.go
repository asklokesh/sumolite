package input

import "fmt"

// Event is the platform-neutral decoded form of a wire event. Backends
// translate this into platform calls; tests assert on it without invoking
// real input injection.
type Event struct {
	Kind   byte
	X, Y   int16  // mouse / scroll
	Button uint8  // mouse button index
	Down   bool   // button / key state
	Key    uint16 // keycode
}

// Parse decodes one wire frame.
//
// Wire format (little-endian, byte-packed):
//
//	type=1 MOVE   : [1][x:int16][y:int16]                 5 bytes
//	type=2 BUTTON : [2][button:u8][down:u8]               3 bytes
//	type=3 SCROLL : [3][dx:int16][dy:int16]               5 bytes
//	type=4 KEY    : [4][keycode:u16][down:u8]             4 bytes
func Parse(b []byte) (Event, error) {
	if len(b) < 1 {
		return Event{}, fmt.Errorf("short event")
	}
	e := Event{Kind: b[0]}
	switch b[0] {
	case EvMouseMove:
		if len(b) < 5 {
			return e, fmt.Errorf("short move")
		}
		e.X, e.Y = i16(b, 1), i16(b, 3)
	case EvMouseButton:
		if len(b) < 3 {
			return e, fmt.Errorf("short button")
		}
		e.Button = b[1]
		e.Down = b[2] != 0
	case EvScroll:
		if len(b) < 5 {
			return e, fmt.Errorf("short scroll")
		}
		e.X, e.Y = i16(b, 1), i16(b, 3)
	case EvKey:
		if len(b) < 4 {
			return e, fmt.Errorf("short key")
		}
		e.Key = u16(b, 1)
		e.Down = b[3] != 0
	default:
		return e, fmt.Errorf("unknown event type 0x%02x", b[0])
	}
	return e, nil
}
