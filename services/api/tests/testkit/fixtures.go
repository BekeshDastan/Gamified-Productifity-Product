package testkit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
)

type User struct{ ID, Email, Password string }

func NewUser() User {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("test random source unavailable")
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	id := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	return User{ID: id, Email: h + "@example.test", Password: "test-only-strong-password-42"}
}

type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewClock(now time.Time) *Clock { return &Clock{now: now.UTC()} }
func (c *Clock) Now() time.Time     { c.mu.RLock(); defer c.mu.RUnlock(); return c.now }
func (c *Clock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

type FakeMailer struct {
	mu       sync.Mutex
	messages []mail.Message
	Failure  error
}

func (m *FakeMailer) Send(_ context.Context, message mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Failure != nil {
		return m.Failure
	}
	m.messages = append(m.messages, message)
	return nil
}

var _ mail.Mailer = (*FakeMailer)(nil)

func (m *FakeMailer) Messages() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message(nil), m.messages...)
}

type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func NewClient(t *testing.T, baseURL string) Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return Client{HTTP: &http.Client{Jar: jar, Timeout: 5 * time.Second}, BaseURL: baseURL}
}
func (c Client) JSON(t *testing.T, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}
