package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrIdempotencyMismatch = errors.New("IDEMPOTENCY_MISMATCH")
var ErrOperationInProgress = errors.New("OPERATION_IN_PROGRESS")
var ErrInvalidScope = errors.New("invalid idempotency scope")
var validKey = regexp.MustCompile(`^[A-Za-z0-9._-]{16,128}$`)
var validRoute = regexp.MustCompile(`^/api/v1/[A-Za-z0-9/_-]+$`)

type Scope struct{ UserID, Method, RouteKey, Key string }
type Result struct {
	Status   int
	Body     []byte
	Replayed bool
}
type IdempotencyStore struct {
	DB  Beginner
	Now func() time.Time
}

// Execute holds a row lock and runs both the operation and its response in one transaction.
// RouteKey includes concrete resource IDs; it never comes from a client-provided header.
// Retention cleanup is intentionally absent: records are retained for at least 24 hours.
func (s IdempotencyStore) Execute(ctx context.Context, scope Scope, body []byte, fn func(pgx.Tx) (Result, error)) (Result, error) {
	var userID pgtype.UUID
	if userID.Scan(scope.UserID) != nil || !validKey.MatchString(scope.Key) || len(scope.RouteKey) > 512 || !validRoute.MatchString(scope.RouteKey) ||
		(scope.Method != "POST" && scope.Method != "PUT" && scope.Method != "PATCH" && scope.Method != "DELETE") {
		return Result{}, ErrInvalidScope
	}
	if s.Now == nil || fn == nil {
		return Result{}, ErrInvalidScope
	}
	hash := sha256.Sum256(body)
	now := s.Now().UTC()
	var result Result
	err := WithinTx(ctx, s.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records (user_id,method,route_key,idempotency_key,body_hash,created_at,expires_at)
            VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, scope.UserID, scope.Method, scope.RouteKey, scope.Key, hash[:], now, now.Add(24*time.Hour))
		if err != nil {
			return err
		}
		var storedHash, response []byte
		var status *int
		err = tx.QueryRow(ctx, `SELECT body_hash,status_code,response_body FROM idempotency_records
            WHERE user_id=$1 AND method=$2 AND route_key=$3 AND idempotency_key=$4 FOR UPDATE`, scope.UserID, scope.Method, scope.RouteKey, scope.Key).Scan(&storedHash, &status, &response)
		if err != nil {
			return err
		}
		if !bytes.Equal(storedHash, hash[:]) {
			return ErrIdempotencyMismatch
		}
		if status != nil {
			result = Result{Status: *status, Body: response, Replayed: true}
			return nil
		}
		if tag.RowsAffected() == 0 {
			return ErrOperationInProgress
		}
		result, err = fn(tx)
		if err != nil {
			return err
		}
		if result.Status < 200 || result.Status > 299 || len(result.Body) > 1<<20 || !json.Valid(result.Body) {
			return errors.New("invalid idempotent response")
		}
		_, err = tx.Exec(ctx, `UPDATE idempotency_records SET status_code=$5,response_body=$6
            WHERE user_id=$1 AND method=$2 AND route_key=$3 AND idempotency_key=$4`, scope.UserID, scope.Method, scope.RouteKey, scope.Key, result.Status, result.Body)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}
