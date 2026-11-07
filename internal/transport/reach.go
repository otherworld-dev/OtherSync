package transport

import (
	"crypto/tls"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Whether the account's server answers at all (Deck #1128). The status line
// used to learn the server had gone only from a failed listing of the root,
// which on a connection that just goes quiet is four dials of about 21 s each:
// some 90 s of "Uploading X" before it said so. Every request passes through
// the router, so it notes there whether the server answered.
//
// One failure is a blip until shown otherwise: the callers retry within a
// second or two, and an answer clears it. The server counts as unreachable
// once there have been two failures in a row with no answer between them and
// the first of them came reachGrace ago, so nothing shows unless the server
// has been failing for that long. A request that went unanswered for
// reachGrace before it failed (a dial nobody answered, an upload whose
// connection went quiet) counts as two: it has already waited that long.
//
// The clock starts when the first failure comes back, not when its request
// was sent. On the VM a 30 MB upload that had been running for a minute
// failed 20 s after the network went; counted from when it was sent, a single
// reset of a long upload would have flashed the line even when the retry
// went straight through.

// reachGrace is how long the server must have gone unanswered before it
// counts as unreachable.
var reachGrace = 10 * time.Second

type reach struct {
	mu       sync.Mutex
	failures int       // connection failures in a row, no answer between them (a slow one counts twice)
	since    time.Time // when the first of them came back
	onChange func()
}

// note records one request to the server, sent at sent: answered, or failed at
// the connection.
func (r *reach) note(sent time.Time, answered bool, now time.Time) {
	r.mu.Lock()
	changed := true
	if answered {
		changed = r.failures > 0
		r.failures = 0
	} else {
		if r.failures == 0 {
			r.since = now
		}
		r.failures++
		if now.Sub(sent) >= reachGrace {
			r.failures++
		}
	}
	f := r.onChange
	r.mu.Unlock()
	if changed && f != nil {
		f()
	}
}

// state reports whether the server is unreachable and, while a failure is not
// yet old enough to count, how long until it would be with no answer
// meanwhile (0 = nothing pending).
func (r *reach) state(now time.Time) (down bool, wait time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures < 2 {
		return false, 0
	}
	if age := now.Sub(r.since); age < reachGrace {
		return false, reachGrace - age
	}
	return true, 0
}

// unanswered reports whether a request's outcome means the server didn't
// answer: the connection failed, or a proxy in front of it said the server
// behind it didn't (502, 504). A certificate the server presents is an answer
// (a wrong one is a different problem), as is every other status: a 503 is
// maintenance or a server asking for a breather, which the pacer handles.
func unanswered(resp *http.Response, err error) bool {
	if err == nil {
		return resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout
	}
	var (
		pm  *PinMismatchError
		cve *tls.CertificateVerificationError
		rhe tls.RecordHeaderError
	)
	if errors.As(err, &pm) || errors.As(err, &cve) || errors.As(err, &rhe) {
		return false
	}
	return connectionError(err)
}

// Unreachable reports whether the server has stopped answering (see reach),
// and, when it would count as unreachable soon if nothing answers meanwhile,
// how long until then, so a caller can look again.
func (c *Client) Unreachable(now time.Time) (bool, time.Duration) {
	return c.router.reach.state(now)
}

// SetReachFunc sets f to be called (on the request's goroutine, so it must not
// block) whenever a request to the server fails, or one is answered after a
// failure.
func (c *Client) SetReachFunc(f func()) {
	c.router.reach.mu.Lock()
	c.router.reach.onChange = f
	c.router.reach.mu.Unlock()
}
