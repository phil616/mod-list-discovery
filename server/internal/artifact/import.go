package artifact

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

func ImportClient(allowPrivate bool) *http.Client {
	validate := func(u *url.URL) error {
		if u.User != nil || u.Host == "" || u.Fragment != "" || (u.Scheme != "https" && !(allowPrivate && u.Scheme == "http")) {
			return fmt.Errorf("import URL rejected")
		}
		return nil
	}
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses")
		}
		for _, ip := range ips {
			if !allowPrivate && !publicIP(ip.Unmap()) {
				return nil, fmt.Errorf("private import address rejected")
			}
		}
		d := net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	return &http.Client{Timeout: 5 * time.Minute, Transport: roundTripper{tr, validate}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("too many redirects")
		}
		if via[0].URL.Scheme == "https" && r.URL.Scheme != "https" {
			return fmt.Errorf("HTTPS downgrade rejected")
		}
		return validate(r.URL)
	}}
}

type roundTripper struct {
	base     http.RoundTripper
	validate func(*url.URL) error
}

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if e := r.validate(req.URL); e != nil {
		return nil, e
	}
	return r.base.RoundTrip(req)
}
func publicIP(ip netip.Addr) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(cidr).Contains(ip) {
			return false
		}
	}
	return true
}
