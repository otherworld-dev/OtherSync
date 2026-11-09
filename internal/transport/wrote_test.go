package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPutReportsTheFileItWrote checks a finished PUT tells the wrote hook the
// path and the server's file id, which is how the engine knows the push notice
// that follows is for its own upload (Deck d1103).
func TestPutReportsTheFileItWrote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("OC-FileId", "00004213ocrt2fp5q2bq")
		w.Header().Set("OC-ETag", `"abc"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	c := New(srv.URL, "alice", "pw")
	var gotPath, gotID string
	c.SetWroteFunc(func(p, id string) { gotPath, gotID = p, id })

	body := func() (io.Reader, error) { return bytes.NewReader([]byte("hi")), nil }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := c.PutWithChecksum(ctx, "Docs/a.txt", body, 2, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "Docs/a.txt" || gotID != "00004213ocrt2fp5q2bq" {
		t.Fatalf("wrote hook got %q %q", gotPath, gotID)
	}
}

func TestNumericFileID(t *testing.T) {
	for in, want := range map[string]uint64{
		"4213":                 4213,
		"00004213ocrt2fp5q2bq": 4213,
		"00000001oc1":          1,
	} {
		if got, ok := NumericFileID(in); !ok || got != want {
			t.Errorf("NumericFileID(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "ocrt2fp5q2bq", "x12"} {
		if got, ok := NumericFileID(in); ok {
			t.Errorf("NumericFileID(%q) = %d, true; want false", in, got)
		}
	}
}
