package platform

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxy accepts only a single address overwritten by the configured edge.
// Forwarded/X-Forwarded-For supplied by browsers never establish client identity.
func TrustedProxy(cidrs string) (func(http.Handler) http.Handler, error) {
	var prefixes []netip.Prefix
	if strings.TrimSpace(cidrs) != "" {
		for _, value := range strings.Split(cidrs, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
			if err != nil || prefix.Bits() == 0 {
				return nil, errors.New("TRUSTED_PROXY_CIDRS must contain explicit non-default CIDRs")
			}
			prefixes = append(prefixes, prefix)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, _, _ := net.SplitHostPort(r.RemoteAddr)
			address, err := netip.ParseAddr(peer)
			if err == nil {
				for _, prefix := range prefixes {
					if prefix.Contains(address.Unmap()) {
						client, err := netip.ParseAddr(r.Header.Get("X-Real-IP"))
						if err != nil || client.Zone() != "" {
							WriteError(w, r, problem(400, "INVALID_PROXY_CLIENT", "Некорректный адрес клиента"))
							return
						}
						clone := r.Clone(r.Context())
						clone.RemoteAddr = net.JoinHostPort(client.Unmap().String(), "0")
						r = clone
						break
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
