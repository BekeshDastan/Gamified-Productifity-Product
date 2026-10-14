// Package push sends encrypted, generic Web Push messages to known push providers.
package push

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	webpush "github.com/SherClockHolmes/webpush-go"
	"net/http"
	"net/http/httptrace"
	"net/mail"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

var ErrUnknown = errors.New("push outcome unknown")
var ErrGone = errors.New("push subscription expired")
var ErrUnavailable = errors.New("push unavailable")
var ErrRejected = errors.New("push rejected")
var ErrRetry = errors.New("push temporarily unavailable")

type Subscription = webpush.Subscription
type Sender interface {
	Send(context.Context, Subscription, []byte) error
}
type Adapter struct {
	PublicKey, PrivateKey, Subject string
	Client                         webpush.HTTPClient
}

func Contact(origin, from string) string {
	u, e := url.Parse(origin)
	if e == nil && u.Scheme == "https" && u.Host != "" {
		return origin
	}
	a, e := mail.ParseAddress(from)
	if e == nil {
		return "mailto:" + a.Address
	}
	return "mailto:notifications@localhost"
}

func Validate(s Subscription) error {
	u, e := url.Parse(s.Endpoint)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" || len(s.Endpoint) > 2048 {
		return ErrRejected
	}
	h := strings.ToLower(u.Hostname())
	if h != "fcm.googleapis.com" && h != "push.services.mozilla.com" && h != "updates.push.services.mozilla.com" && !strings.HasSuffix(h, ".push.apple.com") {
		return ErrRejected
	}
	pub, e := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if e != nil || len(pub) != 65 {
		return ErrRejected
	}
	if _, e = ecdh.P256().NewPublicKey(pub); e != nil {
		return ErrRejected
	}
	auth, e := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	if e != nil || len(auth) != 16 {
		return ErrRejected
	}
	return nil
}
func (a Adapter) Send(ctx context.Context, s Subscription, payload []byte) error {
	if a.PublicKey == "" || a.PrivateKey == "" {
		return ErrUnavailable
	}
	if e := Validate(s); e != nil {
		return e
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	var wrote atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteHeaders: func() { wrote.Store(true) }})
	response, e := webpush.SendNotificationWithContext(ctx, payload, &s, &webpush.Options{HTTPClient: client, Subscriber: a.Subject, VAPIDPublicKey: a.PublicKey, VAPIDPrivateKey: a.PrivateKey, TTL: 300, Urgency: webpush.UrgencyNormal})
	if e != nil {
		if wrote.Load() {
			return ErrUnknown
		}
		return ErrRetry
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == 404 || response.StatusCode == 410:
		return ErrGone
	case response.StatusCode == 429 || response.StatusCode >= 500:
		return ErrRetry
	default:
		return ErrRejected
	}
}
