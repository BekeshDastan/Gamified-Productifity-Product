package blob

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// ClamAV is restricted to a local, explicitly configured daemon. No file paths
// or user-supplied commands enter the clamd protocol.
type ClamAV struct {
	store   BlobStore
	address string
}

func ConfigureScanner(store BlobStore, mode, address string) (Scanner, error) {
	switch mode {
	case "", "unavailable":
		return UnavailableScanner{}, nil
	case "clamav":
		if address == "" {
			address = "127.0.0.1:3310"
		}
		return NewClamAV(store, address)
	default:
		return nil, errors.New("UPLOAD_SCAN_MODE must be unavailable or clamav")
	}
}

func NewClamAV(store BlobStore, address string) (*ClamAV, error) {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	p, e := strconv.Atoi(port)
	if err != nil || e != nil || p < 1 || p > 65535 || ip == nil || !ip.IsLoopback() {
		return nil, errors.New("CLAMAV_ADDRESS must be a loopback IP and port")
	}
	return &ClamAV{store: store, address: address}, nil
}
func (s *ClamAV) Scan(ctx context.Context, object Object) (Verdict, error) {
	if object.Size < 1 || object.Size > 10<<20 {
		return Pending, ErrScannerUnavailable
	}
	reader, err := s.store.Get(ctx, object.Key)
	if err != nil {
		return Pending, ErrScannerUnavailable
	}
	defer reader.Close()
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return Pending, ErrScannerUnavailable
	}
	defer conn.Close()
	deadline := time.Now().Add(12 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return Pending, ErrScannerUnavailable
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := io.WriteString(conn, "zINSTREAM\x00"); err != nil {
		return Pending, ErrScannerUnavailable
	}
	buf := make([]byte, 64<<10)
	var size int64
	for {
		n, e := reader.Read(buf)
		if n > 0 {
			size += int64(n)
			if size > object.Size {
				return Pending, ErrScannerUnavailable
			}
			var head [4]byte
			binary.BigEndian.PutUint32(head[:], uint32(n))
			if _, err := conn.Write(head[:]); err != nil {
				return Pending, ErrScannerUnavailable
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return Pending, ErrScannerUnavailable
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return Pending, ErrScannerUnavailable
		}
	}
	if size != object.Size {
		return Pending, ErrScannerUnavailable
	}
	if _, err := conn.Write(make([]byte, 4)); err != nil {
		return Pending, ErrScannerUnavailable
	}
	reply, err := bufio.NewReader(io.LimitReader(conn, 4096)).ReadString(0)
	if err != nil {
		return Pending, ErrScannerUnavailable
	}
	if reply == "stream: OK\x00" {
		return Clean, nil
	}
	if strings.HasPrefix(reply, "stream: ") && strings.HasSuffix(reply, " FOUND\x00") {
		return Rejected, nil
	}
	return Pending, ErrScannerUnavailable
}
