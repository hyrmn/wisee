package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"wisee/internal/store"
)

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := New(st, "test")
	if err != nil {
		t.Fatal(err)
	}
	return srv, st
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

func TestIndex(t *testing.T) {
	srv, _ := testServer(t)
	code, body := get(t, srv.Handler(), "/")
	if code != 200 {
		t.Fatalf("want 200, got %d", code)
	}
	for _, want := range []string{"Wi-See", "htmx.min.js", `id="viewport"`} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestDevicesFragment(t *testing.T) {
	srv, st := testServer(t)
	now := time.Now()
	err := st.ApplyScan([]store.Observation{
		{IP: "10.0.0.1", MAC: "AA:BB:CC:00:00:01", Name: "gw", Kind: "router",
			Services: []store.ServiceObs{{Port: 53, Name: "dns"}, {Port: 80, Name: "http", Title: "Router"}}},
		{IP: "10.0.0.2", MAC: "AA:BB:CC:00:00:02", Name: "box", Kind: "server"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MergeHistorical(
		store.Observation{IP: "10.0.0.3", MAC: "AA:BB:CC:00:00:03", Name: "ghost", Kind: "phone"},
		now.Add(-48*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	code, body := get(t, srv.Handler(), "/map/devices")
	if code != 200 {
		t.Fatalf("want 200, got %d", code)
	}
	for _, want := range []string{
		`class="device online`, `class="device offline`,
		"gw", "ghost", "last seen", "svc-bubble", "hx-swap-oob",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment missing %q", want)
		}
	}
}

func TestDeviceDetailAndPosition(t *testing.T) {
	srv, st := testServer(t)
	st.ApplyScan([]store.Observation{
		{IP: "10.0.0.1", MAC: "AA:BB:CC:00:00:01", Name: "gw", Kind: "router",
			Services: []store.ServiceObs{{Port: 80, Name: "http", Title: "Router UI"}}},
	}, time.Now())
	devices, _ := st.Devices()
	id := devices[0].ID

	code, body := get(t, srv.Handler(), "/devices/1")
	if code != 200 || !strings.Contains(body, "Router UI") || !strings.Contains(body, "online now") {
		t.Errorf("bad detail: code=%d", code)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/devices/"+strconv.FormatInt(id, 10)+"/position",
		strings.NewReader("x=120&y=-40"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("position: want 204, got %d", rec.Code)
	}
	d, _ := st.Device(id)
	if !d.HasPos || d.X != 120 || d.Y != -40 {
		t.Errorf("position not stored: %+v", d)
	}

	// Unknown device must 404, not panic.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/devices/4242", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rec.Code)
	}
}

func TestScanEndpointWithoutScanner(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/scan", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "demo mode") {
		t.Errorf("want demo-mode note, got %d %q", rec.Code, rec.Body.String())
	}
}
