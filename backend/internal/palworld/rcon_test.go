package palworld

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestRCONPacketRoundTrip(t *testing.T) {
	want := RCONPacket{ID: 42, Type: rconExecCommand, Body: "ShowPlayers"}
	b, err := EncodeRCONPacket(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(b); got != uint32(len(b)-4) {
		t.Fatalf("length=%d", got)
	}
	got, err := DecodeRCONPacket(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestRCONExecAcceptsPalworldNonConformantResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		auth, err := DecodeRCONPacket(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if auth.ID != 1 || auth.Type != rconAuth || auth.Body != "secret" {
			serverErr <- fmt.Errorf("unexpected auth packet: %#v", auth)
			return
		}
		if err := writeTestRCONPacket(conn, RCONPacket{ID: auth.ID, Type: rconAuthResponse}); err != nil {
			serverErr <- err
			return
		}
		exec, err := DecodeRCONPacket(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if exec.ID != 2 || exec.Type != rconExecCommand || exec.Body != "ShowPlayers" {
			serverErr <- fmt.Errorf("unexpected exec packet: %#v", exec)
			return
		}
		// Palworld replies with ID 0 and never echoes the usual trailing empty
		// RESPONSE_VALUE marker. Keep the connection open long enough for the
		// client's short drain deadline to prove it does not wait for that marker.
		if err := writeTestRCONPacket(conn, RCONPacket{ID: 0, Type: rconResponseValue, Body: "name,playeruid\r\nHunter,abc\r\n"}); err != nil {
			serverErr <- err
			return
		}
		time.Sleep(250 * time.Millisecond)
		serverErr <- nil
	}()

	client := NewRCONClient(listener.Addr().String(), "secret")
	client.Timeout = time.Second
	got, err := client.Exec(context.Background(), "ShowPlayers")
	if err != nil {
		t.Fatal(err)
	}
	if want := "name,playeruid\r\nHunter,abc"; got != want {
		t.Fatalf("response %q, want %q", got, want)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func writeTestRCONPacket(conn net.Conn, packet RCONPacket) error {
	b, err := EncodeRCONPacket(packet)
	if err != nil {
		return err
	}
	_, err = conn.Write(b)
	return err
}

func TestRCONPacketRejectsBadTerminator(t *testing.T) {
	b, _ := EncodeRCONPacket(RCONPacket{ID: 1, Body: "x"})
	b[len(b)-1] = 1
	if _, err := DecodeRCONPacket(bytes.NewReader(b)); err == nil {
		t.Fatal("expected error")
	}
}
