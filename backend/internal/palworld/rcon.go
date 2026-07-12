package palworld

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	rconResponseValue = 0
	rconExecCommand   = 2
	rconAuth          = 3
	rconAuthResponse  = 2
)

// RCONPacket is one Source RCON wire packet.
type RCONPacket struct {
	ID   int32
	Type int32
	Body string
}

// EncodeRCONPacket encodes a packet including its little-endian length prefix.
func EncodeRCONPacket(p RCONPacket) ([]byte, error) {
	if strings.IndexByte(p.Body, 0) >= 0 {
		return nil, errors.New("RCON body contains NUL")
	}
	n := 4 + 4 + len(p.Body) + 2
	b := make([]byte, 4+n)
	binary.LittleEndian.PutUint32(b, uint32(n))
	binary.LittleEndian.PutUint32(b[4:], uint32(p.ID))
	binary.LittleEndian.PutUint32(b[8:], uint32(p.Type))
	copy(b[12:], p.Body)
	return b, nil
}

// DecodeRCONPacket reads and validates a packet.
func DecodeRCONPacket(r io.Reader) (RCONPacket, error) {
	var p RCONPacket
	var n int32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return p, err
	}
	if n < 10 || n > 4*1024*1024 {
		return p, fmt.Errorf("invalid RCON packet length %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return p, err
	}
	p.ID = int32(binary.LittleEndian.Uint32(b))
	p.Type = int32(binary.LittleEndian.Uint32(b[4:]))
	if b[len(b)-2] != 0 || b[len(b)-1] != 0 {
		return p, errors.New("invalid RCON terminator")
	}
	p.Body = string(b[8 : len(b)-2])
	return p, nil
}

// RCONClient executes commands over a new authenticated connection.
type RCONClient struct {
	Addr, Password string
	Timeout        time.Duration
}

// NewRCONClient creates an RCON client.
func NewRCONClient(addr, password string) *RCONClient {
	return &RCONClient{Addr: addr, Password: password, Timeout: 5 * time.Second}
}

// Exec authenticates and executes command, combining short multi-packet responses.
func (c *RCONClient) Exec(ctx context.Context, command string) (string, error) {
	if c.Addr == "" {
		return "", errors.New("PALWORLD_RCON_ADDR is unset")
	}
	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.Timeout))
	write := func(p RCONPacket) error {
		b, e := EncodeRCONPacket(p)
		if e != nil {
			return e
		}
		_, e = conn.Write(b)
		return e
	}
	if err = write(RCONPacket{ID: 1, Type: rconAuth, Body: c.Password}); err != nil {
		return "", err
	}
	for {
		p, e := DecodeRCONPacket(conn)
		if e != nil {
			return "", e
		}
		if p.Type == rconAuthResponse {
			if p.ID == -1 {
				return "", errors.New("RCON authentication failed")
			}
			break
		}
	}
	if err = write(RCONPacket{ID: 2, Type: rconExecCommand, Body: command}); err != nil {
		return "", err
	}
	// Palworld's RCON server is non-conformant two ways (verified against the live
	// v1.0.0.100427 server): it never echoes the trailing RESPONSE_VALUE packet the
	// standard Source multi-packet trick relies on, and its exec responses carry
	// ID 0 rather than the request ID. Read the first response with the full
	// deadline, keep any RESPONSE_VALUE body regardless of ID, then drain briefly.
	var out bytes.Buffer
	p, e := DecodeRCONPacket(conn)
	if e != nil {
		return "", e
	}
	if p.Type == rconResponseValue {
		out.WriteString(p.Body)
	}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		p, e = DecodeRCONPacket(conn)
		if e != nil {
			break // deadline: no more packets
		}
		if p.Type == rconResponseValue {
			out.WriteString(p.Body)
		}
	}
	return strings.TrimRight(out.String(), "\x00\r\n"), nil
}
