package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// recordingRT counts the RoundTrips it sees and delegates to base (falling
// back to http.DefaultTransport when base is nil), mirroring the shape of a
// real instrumentation wrapper like telemetry.Reporter.Transport.
type recordingRT struct {
	base  http.RoundTripper
	calls int
}

func (rt *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls++
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func loginHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OTP string `json:"otp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OTP != "123456" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	return mux
}

// TestWrapTransportIdentityKeepsBehavior pins that an inert wrap (returns
// its argument untouched) leaves the client working exactly as before —
// app startup relies on being able to apply WrapTransport unconditionally.
func TestWrapTransportIdentityKeepsBehavior(t *testing.T) {
	c := newClient(t, loginHandler())

	c.WrapTransport(func(rt http.RoundTripper) http.RoundTripper { return rt })

	if err := c.Login(t.Context(), "123456"); err != nil {
		t.Fatalf("Login after identity wrap = %v, want nil", err)
	}
	if c.hc.Transport != nil {
		t.Errorf("identity wrap on a fresh client must leave Transport nil, got %v", c.hc.Transport)
	}
}

// TestWrapTransportInterceptsRequests pins that the installed transport
// actually sees outgoing traffic — not just that it's stored.
func TestWrapTransportInterceptsRequests(t *testing.T) {
	c := newClient(t, loginHandler())

	rec := &recordingRT{}
	c.WrapTransport(func(rt http.RoundTripper) http.RoundTripper {
		rec.base = rt
		return rec
	})

	if err := c.Login(t.Context(), "123456"); err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if rec.calls == 0 {
		t.Error("recording transport saw 0 calls, want > 0 — WrapTransport must actually intercept requests")
	}
}

// TestWrapTransportReceivesCurrentTransport pins the wrap contract itself:
// it is invoked exactly once per call, with the transport that was
// installed at call time (nil on a fresh client), and its return value
// becomes the client's new transport.
func TestWrapTransportReceivesCurrentTransport(t *testing.T) {
	c := newClient(t, loginHandler())

	var calls int
	var got http.RoundTripper
	sentinel := &recordingRT{}
	c.WrapTransport(func(rt http.RoundTripper) http.RoundTripper {
		calls++
		got = rt
		return sentinel
	})

	if calls != 1 {
		t.Fatalf("wrap invoked %d times, want exactly 1", calls)
	}
	if got != nil {
		t.Fatalf("wrap received %v as the prior transport, want nil on a fresh client", got)
	}
	if c.hc.Transport != http.RoundTripper(sentinel) {
		t.Fatalf("client transport = %v, want the value returned by wrap", c.hc.Transport)
	}

	// A second wrap must observe the transport the first wrap installed,
	// proving WrapTransport always hands wrap the CURRENT transport, not
	// some cached/original value.
	var got2 http.RoundTripper
	c.WrapTransport(func(rt http.RoundTripper) http.RoundTripper {
		got2 = rt
		return rt
	})
	if got2 != http.RoundTripper(sentinel) {
		t.Fatalf("second wrap received %v, want the transport installed by the first wrap", got2)
	}
}
