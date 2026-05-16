package input

import (
	"encoding/binary"
	"testing"
)

func TestParseMove(t *testing.T) {
	b := make([]byte, 5)
	b[0] = EvMouseMove
	var x int16 = -42
	binary.LittleEndian.PutUint16(b[1:], uint16(x))
	var y int16 = 1337
	binary.LittleEndian.PutUint16(b[3:], uint16(y))
	e, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != EvMouseMove || e.X != -42 || e.Y != 1337 {
		t.Fatalf("got %+v", e)
	}
}

func TestParseButton(t *testing.T) {
	for _, down := range []byte{0, 1} {
		b := []byte{EvMouseButton, 2, down}
		e, err := Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		if e.Button != 2 {
			t.Fatalf("button: %d", e.Button)
		}
		if e.Down != (down == 1) {
			t.Fatalf("down: %v", e.Down)
		}
	}
}

func TestParseScrollSignedAxes(t *testing.T) {
	b := make([]byte, 5)
	b[0] = EvScroll
	var dx int16 = -1
	var dy int16 = -1000
	binary.LittleEndian.PutUint16(b[1:], uint16(dx))
	binary.LittleEndian.PutUint16(b[3:], uint16(dy))
	e, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if e.X != -1 || e.Y != -1000 {
		t.Fatalf("got %+v", e)
	}
}

func TestParseKey(t *testing.T) {
	b := make([]byte, 4)
	b[0] = EvKey
	binary.LittleEndian.PutUint16(b[1:], 65) // 'A'
	b[3] = 1
	e, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if e.Key != 65 || !e.Down {
		t.Fatalf("got %+v", e)
	}
}

func TestParseRejectsShortAndUnknown(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{EvMouseMove, 1, 2},         // short
		{EvMouseButton, 0},          // short
		{EvScroll, 1, 2, 3},         // short
		{EvKey, 1, 2},               // short
		{0xFF, 1, 2, 3, 4, 5, 6, 7}, // unknown type
	}
	for i, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Fatalf("case %d: expected error, got nil for % x", i, c)
		}
	}
}
