package masterthesis

import (
	"bytes"
	"encoding/binary"
)

type Header interface {
	PacketNumber() uint32
	PacketType() uint8
	ConnectionId() uint64
	encode() []byte
}
func ReadHeader(buffer *bytes.Reader, conn *Connection) Header {
	var h Header
	typeByte, _ := buffer.ReadByte()
	buffer.UnreadByte()
	if typeByte & 0x80 == 0x80 {
		h = ReadLongHeader(buffer)
	} else {
		h = ReadShortHeader(buffer, conn)
	}
	return h
}

type LongHeader struct {
	packetType   LongPacketType
	connectionId uint64
	Version      uint32
	packetNumber uint32
}
func (h LongHeader) encode() []byte {
	buffer := new(bytes.Buffer)
	typeByte := uint8(0x80)
	typeByte |= uint8(h.packetType)
	binary.Write(buffer, binary.BigEndian, typeByte)
	binary.Write(buffer, binary.BigEndian, h.connectionId)
	binary.Write(buffer, binary.BigEndian, h.Version)
	binary.Write(buffer, binary.BigEndian, h.packetNumber)
	return buffer.Bytes()
}
func (h LongHeader) PacketNumber() uint32 {
	return h.packetNumber
}
func (h LongHeader) PacketType() uint8 {
	return uint8(h.packetType)
}
func (h LongHeader) ConnectionId() uint64 {
	return h.connectionId
}
func ReadLongHeader(buffer *bytes.Reader) *LongHeader {
	h := new(LongHeader)
	typeByte, _ := buffer.ReadByte()
	h.packetType = LongPacketType(typeByte - 0x80)
	binary.Read(buffer, binary.BigEndian, &h.connectionId)
	binary.Read(buffer, binary.BigEndian, &h.Version)
	binary.Read(buffer, binary.BigEndian, &h.packetNumber)
	return h
}
func NewLongHeader(packetType LongPacketType, conn *Connection) *LongHeader {
	h := new(LongHeader)
	h.packetType = packetType
	h.connectionId = conn.ConnectionId
	h.packetNumber = uint32(conn.nextPacketNumber())
	h.Version = conn.Version
	return h
}

type LongPacketType uint8

const (
	Initial          LongPacketType = 0x7f
	Retry            LongPacketType = 0x7e
	Handshake        LongPacketType = 0x7d
	ZeroRTTProtected LongPacketType = 0x7c
)

type ShortHeader struct {
	omitConnectionIdFlag bool
	keyPhase             KeyPhaseBit
	packetType           ShortHeaderPacketType
	connectionId         uint64
	packetNumber         uint32
}
func (h ShortHeader) encode() []byte {
	buffer := new(bytes.Buffer)
	var typeByte uint8
	if h.omitConnectionIdFlag {
		typeByte |= 0x40
	}
	if h.keyPhase == KeyPhaseOne {
		typeByte |= 0x20
	}
	typeByte |= uint8(h.packetType)
	binary.Write(buffer, binary.BigEndian, typeByte)
	if !h.omitConnectionIdFlag {
		binary.Write(buffer, binary.BigEndian, h.connectionId)
	}
	switch h.packetType {
	case OneBytePacketNumber:
		binary.Write(buffer, binary.BigEndian, uint8(h.packetNumber))
	case TwoBytesPacketNumber:
		binary.Write(buffer, binary.BigEndian, uint16(h.packetNumber))
	case FourBytesPacketNumber:
		binary.Write(buffer, binary.BigEndian, h.packetNumber)
	}
	return buffer.Bytes()
}
func (h ShortHeader) PacketNumber() uint32 {
	return h.packetNumber
}
func (h ShortHeader) PacketType() uint8 {
	return uint8(h.packetType)
}
func (h ShortHeader) ConnectionId() uint64 {
	return h.connectionId
}
func ReadShortHeader(buffer *bytes.Reader, conn *Connection) *ShortHeader {
	h := new(ShortHeader)
	typeByte, _ := buffer.ReadByte()
	h.omitConnectionIdFlag = (typeByte & 0x40) == 0x40
	h.keyPhase = (typeByte & 0x20) == 0x20
	h.packetType = ShortHeaderPacketType(typeByte & 0x1F)
	if !h.omitConnectionIdFlag {
		binary.Read(buffer, binary.BigEndian, &h.connectionId)
	}
	switch h.packetType {
	case OneBytePacketNumber:
		var number uint8
		binary.Read(buffer, binary.BigEndian, &number)
		h.packetNumber = (uint32(conn.expectedPacketNumber) & 0xffffff00) | uint32(number)
	case TwoBytesPacketNumber:
		var number uint16
		binary.Read(buffer, binary.BigEndian, &number)
		h.packetNumber = (uint32(conn.expectedPacketNumber) & 0xffff0000) | uint32(number)
	case FourBytesPacketNumber:
		binary.Read(buffer, binary.BigEndian, &h.packetNumber)
	}
	return h
}
func NewShortHeader(packetType ShortHeaderPacketType, keyPhase KeyPhaseBit, conn *Connection) *ShortHeader {
	h := new(ShortHeader)
	h.omitConnectionIdFlag = !conn.omitConnectionId
	h.keyPhase = keyPhase
	h.packetType = packetType
	h.connectionId = conn.ConnectionId
	h.packetNumber = uint32(conn.nextPacketNumber())
	return h
}

type KeyPhaseBit bool
const KeyPhaseZero KeyPhaseBit = false
const KeyPhaseOne KeyPhaseBit = true

type ShortHeaderPacketType uint8
const OneBytePacketNumber ShortHeaderPacketType = 0x1f
const TwoBytesPacketNumber ShortHeaderPacketType = 0x1e
const FourBytesPacketNumber ShortHeaderPacketType = 0x1d