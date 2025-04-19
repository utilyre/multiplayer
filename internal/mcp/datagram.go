package mcp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrShortDatagram = errors.New("short datagram")

const (
	headerVersionSize = 1
	headerFlagsSize   = 2
	headerSize        = headerVersionSize + headerFlagsSize
)

type Datagram struct {
	Version byte
	Flags   uint16
	Data    []byte
}

func (dg Datagram) String() string {
	return fmt.Sprintf("Datagram(v%d:%08b:%x)", dg.Version, dg.Flags, dg.Data)
}

func (dg Datagram) MarshalBinary() ([]byte, error) {
	data := make([]byte, headerSize+len(dg.Data))
	data[0] = dg.Version
	binary.BigEndian.PutUint16(data[headerVersionSize:], dg.Flags)
	copy(data[headerSize:], dg.Data)
	return data, nil
}

func (dg *Datagram) UnmarshalBinary(data []byte) error {
	if l := len(data); l < headerSize {
		return fmt.Errorf("len data %d less than expected %d: %w",
			l, headerSize, ErrShortDatagram)
	}

	dg.Version = data[0]
	dg.Flags = binary.BigEndian.Uint16(data[headerVersionSize:])
	dg.Data = make([]byte, len(data[headerSize:]))
	copy(dg.Data, data[headerSize:])
	return nil
}
