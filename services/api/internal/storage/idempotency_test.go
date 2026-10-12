package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestInvalidMethodRejectedBeforeDatabase(t *testing.T) {
	store := IdempotencyStore{Now: time.Now}
	for _, method := range []string{"", "GET", "post", "POST|PUT"} {
		scope := Scope{UserID: "00000000-0000-4000-8000-000000000001", Method: method, RouteKey: "/api/v1/tasks/complete", Key: "operation-key-0001"}
		_, err := store.Execute(context.Background(), scope, nil, func(pgx.Tx) (Result, error) { t.Fatal("invalid method reached operation"); return Result{}, nil })
		if !errors.Is(err, ErrInvalidScope) {
			t.Fatalf("method %q not rejected: %v", method, err)
		}
	}
}
