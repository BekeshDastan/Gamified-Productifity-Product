package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestSMTPRejectsHeaderInjectionAndPlaintextProduction(t *testing.T) {
	mailer, err := NewSMTP("test", "127.0.0.1:1", "ascend@example.test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.Send(context.Background(), Message{To: "person@example.test", Subject: "hello\r\nBcc: other@example.test"}); err == nil {
		t.Fatal("header injection accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprint(conn, "220 local.test ESMTP\r\n")
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		_, _ = fmt.Fprint(conn, "250 local.test\r\n")
	}()
	production, err := NewSMTP("production", listener.Addr().String(), "ascend@example.test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := production.Send(context.Background(), Message{To: "person@example.test", Subject: "test", Text: "test"}); !errors.Is(err, ErrTLSRequired) {
		t.Fatal("plaintext production accepted", err)
	}
	<-done
}
