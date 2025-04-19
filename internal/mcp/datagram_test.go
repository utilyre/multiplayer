package mcp_test

import (
	"bytes"
	"multiplayer/internal/mcp"
	"testing"
)

func FuzzDatagram(f *testing.F) {
	tests := [...]struct {
		version byte
		flags   uint16
		data    []byte
	}{
		{2, 0b0100000100010000, nil},
		{24, 0b0110010000101000, []byte{}},
		{192, 0b0011001100000000, []byte{1}},
		{24, 0b0010001000001000, []byte("Hello, world")},
		{255, 0b0100000010000011, []byte("👋")},
	}

	for _, test := range tests {
		f.Add(test.version, test.flags, test.data)
	}
	f.Fuzz(func(t *testing.T, version byte, flags uint16, data []byte) {
		orig := mcp.Datagram{
			Version: version,
			Flags:   flags,
			Data:    data,
		}

		data, err := orig.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		var parsed mcp.Datagram
		err = parsed.UnmarshalBinary(data)
		if err != nil {
			t.Fatal(err)
		}

		if orig.Version != parsed.Version ||
			orig.Flags != parsed.Flags ||
			!bytes.Equal(orig.Data, parsed.Data) {
			t.Errorf("expected datagram %q; actual datagram %q", orig, parsed)
		}
	})
}
