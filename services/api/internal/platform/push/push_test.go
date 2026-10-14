package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	webpush "github.com/SherClockHolmes/webpush-go"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
)

type testClient struct {
	code    int
	unknown bool
	request *http.Request
}

func (c *testClient) Do(r *http.Request) (*http.Response, error) {
	c.request = r
	if c.unknown {
		httptrace.ContextClientTrace(r.Context()).WroteHeaders()
		return nil, io.ErrUnexpectedEOF
	}
	return &http.Response{StatusCode: c.code, Body: io.NopCloser(strings.NewReader(""))}, nil
}
func keys(t *testing.T) (Subscription, string, string) {
	t.Helper()
	key, e := ecdh.P256().GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	priv, pub, e := webpush.GenerateVAPIDKeys()
	if e != nil {
		t.Fatal(e)
	}
	s := Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/test"}
	s.Keys.P256dh = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return s, pub, priv
}
func TestValidateRejectsUnsafeEndpointAndKeys(t *testing.T) {
	s, _, _ := keys(t)
	if Validate(s) != nil {
		t.Fatal("valid")
	}
	for _, endpoint := range []string{"http://fcm.googleapis.com/a", "https://127.0.0.1/a", "https://fcm.googleapis.com.evil.test/a", "https://user:pass@fcm.googleapis.com/a", "https://fcm.googleapis.com:443/a", "https://fcm.googleapis.com/a#secret"} {
		copy := s
		copy.Endpoint = endpoint
		if Validate(copy) == nil {
			t.Fatal(endpoint)
		}
	}
	s.Keys.Auth = "bad"
	if Validate(s) == nil {
		t.Fatal("keys")
	}
}
func TestContactUsesHTTPSOrMailbox(t *testing.T) {
	if Contact("https://ascend.test", "x@example.test") != "https://ascend.test" || Contact("http://localhost:3000", "ASCEND <x@example.test>") != "mailto:x@example.test" {
		t.Fatal("contact URI")
	}
}
func TestAdapterEncryptsAndPreservesOutcomes(t *testing.T) {
	s, pub, priv := keys(t)
	for _, tc := range []struct {
		code    int
		unknown bool
		want    error
	}{{201, false, nil}, {410, false, ErrGone}, {429, false, ErrRetry}, {503, false, ErrRetry}, {403, false, ErrRejected}, {0, true, ErrUnknown}} {
		c := &testClient{code: tc.code, unknown: tc.unknown}
		a := Adapter{PublicKey: pub, PrivateKey: priv, Subject: "https://ascend.test", Client: c}
		if e := a.Send(context.Background(), s, []byte("PRIVATE_SENTINEL")); e != tc.want {
			t.Fatal(tc, e)
		}
		body, _ := io.ReadAll(c.request.Body)
		if strings.Contains(string(body), "PRIVATE_SENTINEL") || c.request.Header.Get("Authorization") == "" || c.request.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Fatal("not encrypted/authenticated")
		}
	}
}
