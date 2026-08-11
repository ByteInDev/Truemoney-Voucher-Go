package httpx

import (
	"encoding/binary"
	"io"
)

// HTTP/2 frame types.
const (
	frameData         = 0x0
	frameHeaders      = 0x1
	frameRSTStream    = 0x3
	frameSettings     = 0x4
	framePing         = 0x6
	frameGoaway       = 0x7
	frameWindowUpdate = 0x8
	frameContinuation = 0x9
)

// HTTP/2 frame flags.
const (
	flagAck        = 0x1
	flagEndHeaders = 0x4
	flagEndStream  = 0x1
)

// HTTP/2 SETTINGS identifiers.
const (
	settingHeaderTableSize      = 0x1
	settingEnablePush           = 0x2
	settingMaxConcurrentStreams = 0x3
	settingInitialWindowSize    = 0x4
	settingMaxFrameSize         = 0x5
	// settingMaxHeaderListSize = 0x6 // Firefox does not send this one.
)

// Firefox SETTINGS values as observed on the wire for Firefox 148, kept
// consistent with the User-Agent and TLS fingerprint. Mixing Firefox and
// Chrome values here is detectable via HTTP/2 fingerprinting.
const (
	firefoxHeaderTableSize      uint32 = 65536
	firefoxEnablePush           uint32 = 0
	firefoxMaxConcurrentStreams uint32 = 100
	firefoxInitialWindowSize    uint32 = 131072
	firefoxMaxFrameSize         uint32 = 16384
)

const http2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// h2frameHeader is a parsed HTTP/2 frame header.
type h2frameHeader struct {
	Length   int
	Type     byte
	Flags    byte
	StreamID uint32
}

// writeFrameHeader writes a 9-byte HTTP/2 frame header.
func writeFrameHeader(w io.Writer, length int, ftype, flags byte, streamID uint32) error {
	var hdr [9]byte
	hdr[0] = byte(length >> 16)
	hdr[1] = byte(length >> 8)
	hdr[2] = byte(length)
	hdr[3] = ftype
	hdr[4] = flags
	binary.BigEndian.PutUint32(hdr[5:], streamID)
	_, err := w.Write(hdr[:])
	return err
}

// readFrameHeader reads a 9-byte HTTP/2 frame header from r.
func readFrameHeader(r io.Reader) (h2frameHeader, error) {
	var buf [9]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return h2frameHeader{}, err
	}
	return h2frameHeader{
		Length:   int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2]),
		Type:     buf[3],
		Flags:    buf[4],
		StreamID: binary.BigEndian.Uint32(buf[5:9]) & 0x7FFFFFFF,
	}, nil
}

// writeSettings writes a SETTINGS frame with the given id/value pairs.
func writeSettings(w io.Writer, settings ...[2]uint32) error {
	payloadLen := len(settings) * 6
	if err := writeFrameHeader(w, payloadLen, frameSettings, 0, 0); err != nil {
		return err
	}
	var pair [6]byte
	for _, s := range settings {
		binary.BigEndian.PutUint16(pair[0:2], uint16(s[0]))
		binary.BigEndian.PutUint32(pair[2:6], s[1])
		if _, err := w.Write(pair[:]); err != nil {
			return err
		}
	}
	return nil
}