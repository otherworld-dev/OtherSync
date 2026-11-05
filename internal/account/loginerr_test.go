package account

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
)

// Each common way a sign-in can fail to start reads as a short plain sentence
// naming the server, never the raw Go error, and still unwraps to the original
// so logs and errors.Is keep the detail (Deck d907).
func TestExplainLoginStart(t *testing.T) {
	wrap := func(err error) error {
		return &url.Error{Op: "Post", URL: "https://cloud.example.com/index.php/login/v2", Err: err}
	}
	op := func(err error) error { return wrap(&net.OpError{Op: "dial", Net: "tcp", Err: err}) }
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"timeout", wrap(context.DeadlineExceeded), "didn't answer in time"},
		{"dns", op(&net.DNSError{Err: "no such host", Name: "cloud.example.com", IsNotFound: true}), "Can't find cloud.example.com"},
		{"refused", op(os.NewSyscallError("connect", syscall.ECONNREFUSED)), "turned the connection down"},
		{"refused-windows", op(os.NewSyscallError("connectex", syscall.Errno(10061))), "turned the connection down"},
		{"unknown-ca", wrap(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), "certificate"},
		{"hostname", wrap(x509.HostnameError{Host: "cloud.example.com"}), "certificate"},
		{"plain-http", wrap(errors.New("http: server gave HTTP response to HTTPS client")), "http://"},
		{"other-net", op(errors.New("network is unreachable")), "Can't reach cloud.example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := explainLoginStart("https://cloud.example.com", c.err)
			if !strings.Contains(got.Error(), c.want) {
				t.Errorf("message %q, want it to contain %q", got.Error(), c.want)
			}
			if strings.Contains(got.Error(), "login/v2") || strings.Contains(got.Error(), "Post ") {
				t.Errorf("message %q leaks the raw error", got.Error())
			}
			if !errors.Is(got, c.err) {
				t.Errorf("message does not unwrap to the original error")
			}
		})
	}
}

// A caller cancelling the attempt is not a failure to explain.
func TestExplainLoginStartKeepsCancel(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://x/index.php/login/v2", Err: context.Canceled}
	if got := explainLoginStart("https://x", err); got != err {
		t.Errorf("got %v, want the original cancel error", got)
	}
}

// A server that isn't Nextcloud (or the wrong path) answers 404: say so plainly
// instead of dumping its HTML error page.
func TestInitLoginNotFoundIsPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html><body>Not here</body></html>", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := InitLogin(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "No Nextcloud sign-in") || strings.Contains(err.Error(), "<html>") {
		t.Errorf("got %q", err.Error())
	}
}

func TestInitLoginServerErrorIsPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "stack trace…", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := InitLogin(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "stack trace") {
		t.Errorf("got %v", err)
	}
}

func TestInitLoginNotNextcloudIsPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>welcome to my blog</html>"))
	}))
	defer srv.Close()
	_, err := InitLogin(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "not like a Nextcloud server") {
		t.Errorf("got %v", err)
	}
}
