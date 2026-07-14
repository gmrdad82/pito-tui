package telemetry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/config"
	"github.com/gmrdad82/pito-tui/internal/version"
)

// setVersion stamps version.Version for the duration of the test and
// restores it on cleanup, so tests can flip between "dev" and a release
// build without leaking state onto other tests in the package.
func setVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// tracesCollector stands in for the AppSignal OTLP collector: it 200s every
// request and atomically counts POSTs that land on /v1/traces, so tests can
// assert on export traffic without ever reaching a real network.
func tracesCollector(t *testing.T) (srv *httptest.Server, count *atomic.Int64) {
	t.Helper()
	count = new(atomic.Int64)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/traces" {
			count.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, count
}

func TestInitGateOffMatrix(t *testing.T) {
	cases := []struct {
		name    string
		version string
		cfg     config.Telemetry
	}{
		{
			name:    "dev build with fully configured cfg",
			version: "dev",
			cfg:     config.Telemetry{Endpoint: "collector.example.com", Key: "k", Enabled: true},
		},
		{
			name:    "release build missing key",
			version: "1.0.0",
			cfg:     config.Telemetry{Endpoint: "collector.example.com", Key: "", Enabled: true},
		},
		{
			name:    "release build missing endpoint",
			version: "1.0.0",
			cfg:     config.Telemetry{Endpoint: "", Key: "k", Enabled: true},
		},
		{
			name:    "release build enabled false",
			version: "1.0.0",
			cfg:     config.Telemetry{Endpoint: "collector.example.com", Key: "k", Enabled: false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setVersion(t, tc.version)

			r := Init(tc.cfg)
			if r.Active() {
				t.Fatalf("Active() = true, want false for cfg=%+v version=%q", tc.cfg, tc.version)
			}

			base := http.DefaultTransport
			if got := r.Transport(base); got != base {
				t.Error("Transport(base) returned a wrapped transport, want the identical base untouched")
			}
		})
	}
}

func TestInitGateOnExportsSpansAndShutdownFlushes(t *testing.T) {
	setVersion(t, "1.0.0")
	collector, count := tracesCollector(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	r := Init(config.Telemetry{Endpoint: collector.URL, Key: "k", Enabled: true})
	if !r.Active() {
		t.Fatal("Active() = false, want true for a release build with a full config")
	}

	client := &http.Client{Transport: r.Transport(http.DefaultTransport)}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	r.Shutdown()

	if got := count.Load(); got < 1 {
		t.Errorf("collector saw %d POSTs to /v1/traces after Shutdown, want >= 1", got)
	}
}

func TestInertReporterSendsNoRequests(t *testing.T) {
	setVersion(t, "dev")
	collector, count := tracesCollector(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	r := Init(config.Telemetry{Endpoint: collector.URL, Key: "k", Enabled: true})
	if r.Active() {
		t.Fatal("Active() = true, want false on a dev build")
	}

	client := &http.Client{Transport: r.Transport(http.DefaultTransport)}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	r.Shutdown()

	if got := count.Load(); got != 0 {
		t.Errorf("collector saw %d POSTs, want 0 — an inert reporter must never dial out", got)
	}
}

func TestReportPanicFlushesSynchronouslyWithoutShutdown(t *testing.T) {
	setVersion(t, "1.0.0")
	collector, count := tracesCollector(t)

	r := Init(config.Telemetry{Endpoint: collector.URL, Key: "k", Enabled: true})
	t.Cleanup(r.Shutdown)
	if !r.Active() {
		t.Fatal("Active() = false, want true for a release build with a full config")
	}

	r.ReportPanic("boom", []byte("stack"))

	if got := count.Load(); got < 1 {
		t.Errorf("collector saw %d POSTs after ReportPanic (no Shutdown call), want >= 1 — the panic span must flush synchronously", got)
	}
}

// TestInitNormalizesBareHostEndpoint pins the unexported tracesURL behavior
// indirectly through Init: a scheme-less endpoint must still normalize to a
// usable collector URL (https:// + /v1/traces appended) rather than falling
// back to the inert Reporter. Constructing the exporter never dials, so this
// stays offline.
func TestInitNormalizesBareHostEndpoint(t *testing.T) {
	setVersion(t, "1.0.0")

	r := Init(config.Telemetry{Endpoint: "example.com", Key: "k", Enabled: true})
	t.Cleanup(r.Shutdown)
	if !r.Active() {
		t.Error("Active() = false, want true — a bare host must normalize, not go inert")
	}
}

// TestInitEmptyEndpointStaysInert covers the other half of tracesURL's
// contract: an empty endpoint never even reaches normalization because
// cfg.Active() gates it first.
func TestInitEmptyEndpointStaysInert(t *testing.T) {
	setVersion(t, "1.0.0")

	r := Init(config.Telemetry{Endpoint: "", Key: "k", Enabled: true})
	if r.Active() {
		t.Error("Active() = true, want false — cfg.Active() requires a non-empty endpoint")
	}
}
