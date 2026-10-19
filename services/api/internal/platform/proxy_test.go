package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyBoundary(t *testing.T) {
	proxy, err := TrustedProxy("192.0.2.10/32")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		peer, header, want string
		status             int
	}{
		{"192.0.2.10:443", "203.0.113.7", "203.0.113.7:0", 200},
		{"192.0.2.11:443", "203.0.113.7", "192.0.2.11:443", 200},
		{"192.0.2.10:443", "203.0.113.7, 1.2.3.4", "", 400},
		{"192.0.2.10:443", "", "", 400},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Real-IP", tc.header)
		r.Header.Set("X-Forwarded-For", "6.6.6.6")
		w := httptest.NewRecorder()
		proxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.RemoteAddr != tc.want {
				t.Errorf("address %q want %q", r.RemoteAddr, tc.want)
			}
			w.WriteHeader(200)
		})).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("status %d want %d", w.Code, tc.status)
		}
	}
	for _, value := range []string{"0.0.0.0/0", "::/0", "secret", "127.0.0.1"} {
		if _, err := TrustedProxy(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}
