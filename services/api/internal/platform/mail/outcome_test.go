package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSMTPAcceptanceSurvivesLostQuitAndLostDataAckIsUnknown(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			fmt.Fprint(conn, "220 local.test ESMTP\r\n")
			reader := bufio.NewReader(conn)
			for {
				line, e := reader.ReadString('\n')
				if e != nil {
					return
				}
				if strings.HasPrefix(line, "DATA") {
					fmt.Fprint(conn, "354 data\r\n")
					for {
						line, e = reader.ReadString('\n')
						if e != nil {
							return
						}
						if line == ".\r\n" {
							break
						}
					}
					if accepted {
						fmt.Fprint(conn, "250 accepted\r\n")
					}
					return
				}
				fmt.Fprint(conn, "250 local.test\r\n")
			}
		}()
		smtp, e := NewSMTP("test", listener.Addr().String(), "ascend@example.test", "", "")
		if e != nil {
			t.Fatal(e)
		}
		err := smtp.Send(context.Background(), Message{To: "person@example.test", Subject: "test", Text: "test"})
		listener.Close()
		<-done
		if accepted && err != nil || !accepted && !errors.Is(err, ErrDeliveryUnknown) {
			t.Fatal(accepted, err)
		}
	}
}
