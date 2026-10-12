package blob

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClamAVStreamingVerdictsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		reply string
		want  Verdict
	}{{"stream: OK\x00", Clean}, {"stream: Test-Signature FOUND\x00", Rejected}, {"INSTREAM size limit exceeded. ERROR\x00", Pending}, {"stream: OK", Pending}} {
		t.Run(string(tc.want)+tc.reply, func(t *testing.T) {
			store, err := NewLocal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			object, err := store.Put(context.Background(), strings.NewReader("private content"), 100)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				command, err := bufio.NewReader(io.LimitReader(conn, 10)).ReadString(0)
				if err != nil || command != "zINSTREAM\x00" {
					done <- io.ErrUnexpectedEOF
					return
				}
				var all bytes.Buffer
				for {
					var header [4]byte
					if _, err := io.ReadFull(conn, header[:]); err != nil {
						done <- err
						return
					}
					size := binary.BigEndian.Uint32(header[:])
					if size == 0 {
						break
					}
					if size > 65536 {
						done <- io.ErrShortBuffer
						return
					}
					if _, err := io.CopyN(&all, conn, int64(size)); err != nil {
						done <- err
						return
					}
				}
				if all.String() != "private content" {
					done <- io.ErrUnexpectedEOF
					return
				}
				_, err = io.WriteString(conn, tc.reply)
				done <- err
			}()
			scanner, err := NewClamAV(store, listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			verdict, err := scanner.Scan(context.Background(), object)
			if verdict != tc.want || (tc.want == Pending && err == nil) {
				t.Fatal("unsafe verdict", verdict, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := NewClamAV(store, "192.0.2.1:3310"); err == nil {
		t.Fatal("non-local scanner")
	}
	if _, err := ConfigureScanner(store, "allow-all", ""); err == nil {
		t.Fatal("unsafe scanner mode")
	}
}
