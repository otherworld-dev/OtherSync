package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Deck #1128: the status line learns the server has gone from the requests
// themselves, without flapping on one blip.
func TestReachDebounce(t *testing.T) {
	t0 := time.Now()
	var r reach

	// One quick failure is a blip until the retry says otherwise.
	r.note(t0, false, t0.Add(100*time.Millisecond))
	if down, wait := r.state(t0.Add(time.Minute)); down || wait != 0 {
		t.Errorf("one quick failure: down=%v wait=%v", down, wait)
	}

	// A second with no answer between: down once the first is reachGrace old.
	r.note(t0.Add(time.Second), false, t0.Add(time.Second))
	if down, wait := r.state(t0.Add(4 * time.Second)); down || wait != reachGrace-4*time.Second {
		t.Errorf("two failures 4 s in: down=%v wait=%v", down, wait)
	}
	if down, _ := r.state(t0.Add(reachGrace)); !down {
		t.Error("two failures reachGrace in: not down")
	}

	// Any answer clears it.
	r.note(t0.Add(20*time.Second), true, t0.Add(20*time.Second))
	if down, wait := r.state(t0.Add(time.Minute)); down || wait != 0 {
		t.Errorf("after an answer: down=%v wait=%v", down, wait)
	}

	// A dial nobody answered for reachGrace is enough on its own.
	t1 := t0.Add(time.Hour)
	r.note(t1, false, t1.Add(21*time.Second))
	if down, _ := r.state(t1.Add(21 * time.Second)); !down {
		t.Error("one 21 s failure: not down")
	}
}

func TestReachCallsBackOnFailureAndRecovery(t *testing.T) {
	var calls int
	r := reach{onChange: func() { calls++ }}
	now := time.Now()
	r.note(now, true, now) // answered with nothing pending: no change
	r.note(now, false, now)
	r.note(now, false, now)
	r.note(now, true, now)
	if calls != 3 {
		t.Errorf("onChange calls = %d, want 3 (two failures and the recovery)", calls)
	}
}

func TestUnanswered(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		err  error
		want bool
	}{
		{"200", 200, nil, false},
		{"401", 401, nil, false},
		{"503 maintenance", 503, nil, false},
		{"502 from the proxy", 502, nil, true},
		{"504 from the proxy", 504, nil, true},
		{"refused", 0, &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"cancelled", 0, context.Canceled, false},
		{"pin mismatch", 0, &PinMismatchError{Want: "a", Got: "b"}, false},
	} {
		var resp *http.Response
		if tc.err == nil {
			resp = &http.Response{StatusCode: tc.code}
		}
		if got := unanswered(resp, tc.err); got != tc.want {
			t.Errorf("%s: unanswered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Through a real client: a server that has gone (connection refused) counts
// once the grace has passed, and its first answer afterwards clears it.
func TestClientUnreachableWhenServerRefuses(t *testing.T) {
	orig := reachGrace
	reachGrace = 200 * time.Millisecond
	t.Cleanup(func() { reachGrace = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	addr := srv.Listener.Addr().String()
	c := New(srv.URL, "u", "pw")
	var nudges atomic.Int32
	c.SetReachFunc(func() { nudges.Add(1) })

	get := func() {
		req, err := c.NewRequest(context.Background(), "GET", srv.URL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp, err := c.DoOnce(req); err == nil {
			resp.Body.Close()
		}
	}
	get()
	if down, _ := c.Unreachable(time.Now()); down {
		t.Fatal("down while the server answers")
	}

	srv.Close() // refused from now on
	get()
	get()
	if down, wait := c.Unreachable(time.Now()); down || wait <= 0 {
		t.Fatalf("two refusals just now: down=%v wait=%v, want pending", down, wait)
	}
	time.Sleep(reachGrace)
	if down, _ := c.Unreachable(time.Now()); !down {
		t.Fatal("two refusals reachGrace ago: not down")
	}
	if nudges.Load() < 2 {
		t.Errorf("nudges = %d, want one per failure", nudges.Load())
	}

	// The server comes back on the same address and answers.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("can't listen on %s again: %v", addr, err)
	}
	back := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})}
	go back.Serve(ln)
	t.Cleanup(func() { back.Close() })
	get()
	if down, wait := c.Unreachable(time.Now()); down || wait != 0 {
		t.Errorf("after the server answered again: down=%v wait=%v", down, wait)
	}
}

// Requests to some other host say nothing about the account's server.
func TestReachIgnoresOtherHosts(t *testing.T) {
	c := New("https://cloud.example.invalid", "u", "pw")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	other.Close()
	for i := 0; i < 3; i++ {
		req, _ := c.NewRequest(context.Background(), "GET", other.URL+"/", nil)
		if resp, err := c.DoOnce(req); err == nil {
			resp.Body.Close()
		}
	}
	c.router.reach.mu.Lock()
	n := c.router.reach.failures
	c.router.reach.mu.Unlock()
	if n != 0 {
		t.Errorf("failures = %d after requests to another host, want 0", n)
	}
}
