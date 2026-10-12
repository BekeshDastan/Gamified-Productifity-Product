package blob

import (
	"context"
	"errors"
)

type Verdict string

const (
	Pending  Verdict = "pending"
	Clean    Verdict = "clean"
	Rejected Verdict = "rejected"
)

type Scanner interface {
	Scan(context.Context, Object) (Verdict, error)
}

var ErrScannerUnavailable = errors.New("scanner unavailable; object remains quarantined")

type UnavailableScanner struct{}

func (UnavailableScanner) Scan(context.Context, Object) (Verdict, error) {
	return Pending, ErrScannerUnavailable
}
