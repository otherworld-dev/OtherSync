package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A whole-tree listing that fails is not sent again. On a big folder the
// server builds the entire answer in memory, and a 500 there means it ran out
// doing so: four tries in a row was four minutes of a PHP worker at its memory
// limit, every few minutes, until the server's host killed PHP (Deck #1004).
// The caller lists the folder another way instead.
func TestPropFindRecursiveTriesOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL, "adam", "app-password")
	_, err := c.PropFindRecursive(context.Background(), "Projects")
	if err == nil {
		t.Fatal("a 500 to a whole-tree listing reported no error")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("whole-tree listing sent %d times, want once", n)
	}
	if got := StatusCode(err); got != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", got)
	}
}

// A request that kept getting a server error says which one, so the sync loop
// can tell a server that answered (and failed) from one it could not reach.
// The message reads as it always has.
func TestRetriedServerErrorKeepsItsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL, "adam", "app-password")
	_, err := c.PropFind(context.Background(), "Projects", 1)
	if !errors.Is(err, ErrRetriesExhausted) {
		t.Fatalf("err = %v, want retries exhausted", err)
	}
	if got := StatusCode(err); got != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", got)
	}
	if want := "request failed after 4 attempts: server returned 500 Internal Server Error"; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}
