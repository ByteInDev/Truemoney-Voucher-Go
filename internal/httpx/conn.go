package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2/hpack"

	utls "github.com/refraction-networking/utls"
)

// connWindowReclaim is how much connection-level flow-control window may
// be consumed before a WINDOW_UPDATE refills it: half of the 65535 bytes
// every connection starts with (RFC 9113 §5.2.1). Reclaiming at half keeps
// the pipe full without flooding tiny updates.
const connWindowReclaim = 32768

// h2conn is a single HTTP/2 connection that mimics Firefox at the frame level.
type h2conn struct {
	conn net.Conn
	bw   *bufio.Writer
	br   *bufio.Reader

	// Persistent HPACK state: the dynamic table must survive across HEADERS
	// blocks on the same connection (RFC 9113 §4.3), so both encoder and
	// decoder live here and are reused for the life of the connection.
	hbuf bytes.Buffer
	enc  *hpack.Encoder
	dec  *hpack.Decoder

	// Flow-control accounting for received DATA. The connection window
	// starts at 65535 and is only refilled by WINDOW_UPDATE; each stream's
	// window is the INITIAL_WINDOW_SIZE we advertise. Without sending
	// WINDOW_UPDATE, any response larger than the remaining window would
	// deadlock against the server.
	connConsumed   uint32
	streamConsumed uint32
	streamWindow   uint32

	// writeErr is the first write error from a control frame (SETTINGS ACK,
	// PING ACK, WINDOW_UPDATE). It is sticky: once a write fails the
	// connection is unusable, and the error surfaces from do().
	writeErr error

	// created/lastUsed support the idle pool's age and idle-time limits.
	created  time.Time
	lastUsed time.Time

	nextStreamID uint32
	mu           sync.Mutex
}

// dialH2 opens a TCP+TLS connection and performs the HTTP/2 handshake.
func dialH2(ctx context.Context, hostport string, dialer *net.Dialer,
	tlsCfg *utls.Config) (*h2conn, error) {

	// 1. TCP dial.
	raw, err := dialer.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}

	// 2. TLS with a browser fingerprint. The ClientHelloID makes uTLS
	// generate a fresh spec per connection (HelloFirefox_148 includes the
	// h2 ALPN), so specs are never shared or mutated across connections.
	cfg := &utls.Config{ServerName: hostportToSNI(hostport)}
	if tlsCfg != nil {
		cfg = tlsCfg.Clone()
		cfg.ServerName = hostportToSNI(hostport)
	}
	uconn := utls.UClient(raw, cfg, browserFingerprint)
	if deadline, ok := ctx.Deadline(); ok {
		uconn.SetDeadline(deadline)
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	if uconn.ConnectionState().NegotiatedProtocol != "h2" {
		uconn.Close()
		return nil, fmt.Errorf("server did not negotiate h2, got %q",
			uconn.ConnectionState().NegotiatedProtocol)
	}

	c := &h2conn{
		conn:         uconn,
		bw:           bufio.NewWriter(uconn),
		br:           bufio.NewReader(uconn),
		streamWindow: firefoxInitialWindowSize,
		nextStreamID: 1,
		created:      time.Now(),
		lastUsed:     time.Now(),
	}
	c.enc = hpack.NewEncoder(&c.hbuf)
	c.dec = hpack.NewDecoder(firefoxHeaderTableSize, nil)

	// 3. HTTP/2 connection preface + Firefox SETTINGS.
	if _, err := io.WriteString(c.bw, http2Preface); err != nil {
		uconn.Close()
		return nil, err
	}
	if err := writeSettings(c.bw,
		[2]uint32{settingHeaderTableSize, firefoxHeaderTableSize},
		[2]uint32{settingEnablePush, firefoxEnablePush},
		[2]uint32{settingMaxConcurrentStreams, firefoxMaxConcurrentStreams},
		[2]uint32{settingInitialWindowSize, firefoxInitialWindowSize},
		[2]uint32{settingMaxFrameSize, firefoxMaxFrameSize},
	); err != nil {
		uconn.Close()
		return nil, err
	}
	if err := c.bw.Flush(); err != nil {
		uconn.Close()
		return nil, err
	}

	// 4. Read server preface, ACK its SETTINGS and any early PINGs.
	if err := c.readServerPreface(); err != nil {
		uconn.Close()
		return nil, err
	}

	return c, nil
}

func hostportToSNI(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

// readServerPreface consumes frames until the server's SETTINGS arrive,
// acknowledging them and any PINGs sent beforehand.
func (c *h2conn) readServerPreface() error {
	for {
		hdr, err := readFrameHeader(c.br)
		if err != nil {
			return fmt.Errorf("read preface frame: %w", err)
		}
		if hdr.Length > int(firefoxMaxFrameSize) {
			return fmt.Errorf("preface frame of %d bytes exceeds max frame size %d", hdr.Length, firefoxMaxFrameSize)
		}

		switch hdr.Type {
		case frameSettings:
			if hdr.Flags&flagAck != 0 {
				continue
			}
			if _, err := io.CopyN(io.Discard, c.br, int64(hdr.Length)); err != nil {
				return err
			}
			// Server SETTINGS are deliberately not parsed: connections are
			// single-use with tiny request bodies, so server-side window
			// values never bind us. Receive-side windows are governed by
			// OUR advertised SETTINGS and can only change via our own
			// WINDOW_UPDATEs, so hardcoding firefoxInitialWindowSize and
			// firefoxMaxFrameSize for the receive path is correct.
			// ACK the server SETTINGS.
			c.mu.Lock()
			err := c.writeLocked(frameSettings, flagAck, 0, nil)
			c.mu.Unlock()
			return err

		case framePing:
			payload := make([]byte, hdr.Length)
			if _, err := io.ReadFull(c.br, payload); err != nil {
				return err
			}
			if hdr.Flags&flagAck == 0 {
				c.mu.Lock()
				err := c.writeLocked(framePing, flagAck, 0, payload)
				c.mu.Unlock()
				if err != nil {
					return err
				}
			}

		default:
			if _, err := io.CopyN(io.Discard, c.br, int64(hdr.Length)); err != nil {
				return err
			}
		}
	}
}

// do sends one HTTP request and reads the response. When connections are
// reused, only one request is in flight per connection: the pool hands out
// a connection exclusively, so frames never interleave across streams.
func (c *h2conn) do(req *http.Request, headerOrder []string) (*http.Response, error) {
	c.mu.Lock()
	if c.writeErr != nil {
		c.mu.Unlock()
		return nil, c.writeErr
	}
	// Stream-level flow-control accounting is per stream; a reused
	// connection starts a fresh stream here.
	c.streamConsumed = 0
	streamID := c.nextStreamID
	c.nextStreamID += 2
	c.mu.Unlock()

	headerBlock, err := c.encodeHeaders(req, headerOrder)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	flags := byte(flagEndHeaders)
	if req.Body == nil || req.ContentLength == 0 {
		flags |= flagEndStream
	}

	// HEADERS frame.
	if err := writeFrameHeader(c.bw, len(headerBlock), frameHeaders, flags, streamID); err != nil {
		c.writeErr = err
		c.mu.Unlock()
		return nil, err
	}
	if _, err := c.bw.Write(headerBlock); err != nil {
		c.writeErr = err
		c.mu.Unlock()
		return nil, err
	}

	// DATA frame (when a body is present).
	if req.Body != nil && req.ContentLength > 0 {
		bodyBytes, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			c.writeErr = err
			c.mu.Unlock()
			return nil, fmt.Errorf("read body: %w", err)
		}
		if err := writeFrameHeader(c.bw, len(bodyBytes), frameData, flagEndStream, streamID); err != nil {
			c.writeErr = err
			c.mu.Unlock()
			return nil, err
		}
		if _, err := c.bw.Write(bodyBytes); err != nil {
			c.writeErr = err
			c.mu.Unlock()
			return nil, err
		}
	}

	if err := c.bw.Flush(); err != nil {
		c.writeErr = err
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	resp, err := c.readResponse(streamID, req)
	if err != nil {
		return nil, err
	}
	if c.writeErr != nil {
		return nil, c.writeErr
	}
	return resp, nil
}

// readResponse reads HEADERS + DATA frames for streamID and builds
// *http.Response, refilling the flow-control windows as DATA is consumed.
func (c *h2conn) readResponse(streamID uint32, req *http.Request) (*http.Response, error) {
	var (
		headerBlock []byte
		bodyBuf     bytes.Buffer
		done        bool
	)

	for !done {
		hdr, err := readFrameHeader(c.br)
		if err != nil {
			return nil, fmt.Errorf("read response frame: %w", err)
		}
		if hdr.Length > int(firefoxMaxFrameSize) {
			return nil, fmt.Errorf("frame of %d bytes exceeds max frame size %d", hdr.Length, firefoxMaxFrameSize)
		}

		payload := make([]byte, hdr.Length)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return nil, fmt.Errorf("read frame payload: %w", err)
		}

		switch {
		case (hdr.Type == frameHeaders || hdr.Type == frameContinuation) && hdr.StreamID == streamID:
			headerBlock = append(headerBlock, payload...)
			// A HEADERS frame without END_HEADERS must be followed by
			// CONTINUATION frames (RFC 9113 §6.10): only finish once the
			// full block is in hand, even when END_STREAM is already set.
			if hdr.Flags&flagEndHeaders != 0 {
				if hdr.Flags&flagEndStream != 0 {
					done = true
				}
			}

		case hdr.Type == frameData && hdr.StreamID == streamID:
			bodyBuf.Write(payload)
			c.consume(streamID, uint32(hdr.Length))
			if hdr.Flags&flagEndStream != 0 {
				done = true
			}

		case hdr.Type == frameRSTStream && hdr.StreamID == streamID:
			return nil, fmt.Errorf("server reset stream")

		case hdr.Type == frameSettings && hdr.Flags&flagAck == 0:
			// ACK server settings; abort immediately if the write fails.
			c.mu.Lock()
			if err := c.writeLocked(frameSettings, flagAck, 0, nil); err != nil {
				c.mu.Unlock()
				return nil, err
			}
			c.mu.Unlock()

		case hdr.Type == framePing && hdr.Flags&flagAck == 0:
			// Reply to pings.
			c.mu.Lock()
			if err := c.writeLocked(framePing, flagAck, 0, payload); err != nil {
				c.mu.Unlock()
				return nil, err
			}
			c.mu.Unlock()

		case hdr.Type == frameGoaway:
			return nil, fmt.Errorf("server sent GOAWAY")

		case hdr.Type == frameWindowUpdate:
			// Server-side flow control for our request bodies; they are
			// tiny, so nothing to track.
		}
	}

	return c.decodeResponse(headerBlock, req, &bodyBuf)
}

// consume accounts n received DATA bytes against the flow-control windows
// and sends WINDOW_UPDATE once half of either window is consumed. Without
// this, responses larger than the connection window deadlock.
func (c *h2conn) consume(streamID uint32, n uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connConsumed += n
	if c.connConsumed >= connWindowReclaim {
		c.writeLocked(frameWindowUpdate, 0, 0, windowUpdatePayload(c.connConsumed))
		c.connConsumed = 0
	}

	c.streamConsumed += n
	if c.streamWindow > 0 && c.streamConsumed >= c.streamWindow/2 {
		c.writeLocked(frameWindowUpdate, 0, streamID, windowUpdatePayload(c.streamConsumed))
		c.streamConsumed = 0
	}
}

// windowUpdatePayload encodes a WINDOW_UPDATE increment (RFC 9113 §6.9).
func windowUpdatePayload(increment uint32) []byte {
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], increment)
	return p[:]
}

// writeLocked writes a control frame and flushes, recording the first
// error as the sticky c.writeErr. The caller must hold c.mu.
func (c *h2conn) writeLocked(ftype, flags byte, streamID uint32, payload []byte) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	if err := writeFrameHeader(c.bw, len(payload), ftype, flags, streamID); err != nil {
		c.writeErr = err
		return err
	}
	if len(payload) > 0 {
		if _, err := c.bw.Write(payload); err != nil {
			c.writeErr = err
			return err
		}
	}
	if err := c.bw.Flush(); err != nil {
		c.writeErr = err
		return err
	}
	return nil
}

// Close closes the underlying connection.
func (c *h2conn) Close() error {
	return c.conn.Close()
}