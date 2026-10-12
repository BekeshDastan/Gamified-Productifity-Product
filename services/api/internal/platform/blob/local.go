package blob

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrNotFound = errors.New("blob not found")
var ErrTooLarge = errors.New("blob exceeds size limit")

type Object struct {
	Key, SHA256 string
	Size        int64
}
type BlobStore interface {
	Put(context.Context, io.Reader, int64) (Object, error)
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Local struct{ root *os.Root }

var keyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NewLocal(directory string) (*Local, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	for _, component := range strings.Split(strings.ToLower(filepath.ToSlash(abs)), "/") {
		if component == "public" || component == ".next" || component == "node_modules" {
			return nil, errors.New("blob directory must be outside web roots")
		}
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}
func (s *Local) Close() error { return s.root.Close() }

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

type contextReadCloser struct {
	contextReader
	closer io.Closer
}

func (r contextReadCloser) Close() error { return r.closer.Close() }
func (s *Local) Put(ctx context.Context, source io.Reader, max int64) (Object, error) {
	if max < 1 || max > 100<<20 {
		return Object{}, errors.New("blob limit must be 1..100 MiB")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Object{}, err
	}
	key := hex.EncodeToString(random[:])
	file, err := s.root.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Object{}, err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = s.root.Remove(key)
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{ctx: ctx, reader: source}, max+1))
	if err != nil {
		return Object{}, err
	}
	if size > max {
		return Object{}, ErrTooLarge
	}
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	if err := file.Sync(); err != nil {
		return Object{}, err
	}
	if err := file.Close(); err != nil {
		return Object{}, err
	}
	committed = true
	return Object{Key: key, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size}, nil
}
func (s *Local) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !keyPattern.MatchString(key) {
		return nil, ErrNotFound
	}
	file, err := s.root.Open(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return contextReadCloser{contextReader: contextReader{ctx: ctx, reader: file}, closer: file}, nil
}
func (s *Local) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !keyPattern.MatchString(key) {
		return ErrNotFound
	}
	err := s.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
