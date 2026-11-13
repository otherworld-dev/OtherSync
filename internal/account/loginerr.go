package account

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// loginError is what InitLogin returns when it can't start a sign-in: a short
// sentence a user can act on, wrapping the transport or server error so logs
// and errors.Is keep the detail. The sign-in screens show Error() as it is, and
// the raw Go error ("start login flow: Post …: context deadline exceeded")
// told nobody anything (Deck d907).
type loginError struct {
	msg string
	err error
}

func (e *loginError) Error() string { return e.msg }
func (e *loginError) Unwrap() error { return e.err }

// hostOf names the server the way the user typed it, without the scheme.
func hostOf(serverURL string) string {
	if u, err := url.Parse(serverURL); err == nil && u.Host != "" {
		return u.Host + strings.TrimRight(u.Path, "/")
	}
	return serverURL
}

// wsaConnRefused is WSAECONNREFUSED: Windows reports a refused connection with
// its own code, which syscall.ECONNREFUSED doesn't match there.
const wsaConnRefused = syscall.Errno(10061)

// explainLoginStart turns a failed request to start the login flow into a
// plain message. A cancelled attempt is passed through untouched: nothing went
// wrong that the user needs telling about.
func explainLoginStart(serverURL string, err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	host := hostOf(serverURL)
	var (
		certErr     *tls.CertificateVerificationError
		unknownCA   x509.UnknownAuthorityError
		badHost     x509.HostnameError
		invalidCert x509.CertificateInvalidError
		dnsErr      *net.DNSError
		netErr      net.Error
	)
	var msg string
	switch {
	case errors.As(err, &certErr), errors.As(err, &unknownCA), errors.As(err, &badHost), errors.As(err, &invalidCert):
		msg = fmt.Sprintf("Can't connect securely to %s: its certificate isn't trusted. An expired or self-signed certificate is the usual cause.", host)
	case strings.Contains(err.Error(), "server gave HTTP response to HTTPS client"):
		msg = fmt.Sprintf("%s doesn't answer on https. If your server really has no https, type the address starting with http://", host)
	case errors.As(err, &dnsErr):
		msg = fmt.Sprintf("Can't find %s. Check the address, and that this device is online.", host)
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, wsaConnRefused):
		msg = fmt.Sprintf("%s turned the connection down. Check the address (and the port, if it has one), then try again.", host)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		msg = fmt.Sprintf("%s didn't answer in time. Check your connection and the address, then try again.", host)
	default:
		msg = fmt.Sprintf("Can't reach %s. Check your connection and the address, then try again.", host)
	}
	return &loginError{msg: msg, err: err}
}

// explainLoginStatus is the plain message for a server that answered the
// login-flow request with something other than 200. The body (often an HTML
// error page) only goes in the wrapped error.
func explainLoginStatus(serverURL string, status int, statusText, body string) error {
	host := hostOf(serverURL)
	raw := fmt.Errorf("start login flow: server returned %s: %s", statusText, body)
	var msg string
	switch {
	case status == 404:
		msg = fmt.Sprintf("No Nextcloud sign-in found at %s. Check the address; if Nextcloud isn't at the top of the site, include its folder, e.g. example.com/nextcloud.", host)
	case status >= 500:
		msg = fmt.Sprintf("%s had a problem starting the sign-in (%s). Try again in a moment.", host, statusText)
	default:
		msg = fmt.Sprintf("%s wouldn't start a sign-in (%s).", host, statusText)
	}
	return &loginError{msg: msg, err: raw}
}

// notNextcloud is the plain message for a 200 answer that isn't a login flow.
func notNextcloud(serverURL string, err error) error {
	return &loginError{
		msg: fmt.Sprintf("%s answered, but not like a Nextcloud server. Check the address.", hostOf(serverURL)),
		err: err,
	}
}
