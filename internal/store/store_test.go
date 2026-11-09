package store

import (
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func obs(ip, mac, name string, ports ...int) Observation {
	o := Observation{IP: ip, MAC: mac, Name: name, Kind: "unknown"}
	for _, p := range ports {
		o.Services = append(o.Services, ServiceObs{Port: p, Name: "svc"})
	}
	return o
}

func TestApplyScanMarksOffline(t *testing.T) {
	st := testStore(t)
	now := time.Now()

	if err := st.ApplyScan([]Observation{obs("10.0.0.1", "AA:BB:CC:00:00:01", "one", 22, 80)}, now); err != nil {
		t.Fatal(err)
	}
	devices, err := st.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || !devices[0].Online {
		t.Fatalf("want 1 online device, got %+v", devices)
	}
	if len(devices[0].Services) != 2 {
		t.Fatalf("want 2 services, got %d", len(devices[0].Services))
	}

	// Second scan sees nothing: the device must go dim, not disappear.
	if err := st.ApplyScan(nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	devices, _ = st.Devices()
	if len(devices) != 1 {
		t.Fatalf("device vanished: %d devices", len(devices))
	}
	if devices[0].Online {
		t.Fatal("device should be offline after missing a scan")
	}
	for _, s := range devices[0].Services {
		if s.Online {
			t.Fatalf("service %d should be offline", s.Port)
		}
	}
}

func TestMergeByMACKeepsIdentityAcrossIPChanges(t *testing.T) {
	st := testStore(t)
	now := time.Now()

	st.ApplyScan([]Observation{obs("10.0.0.5", "AA:BB:CC:00:00:05", "old-name", 22)}, now)
	st.ApplyScan([]Observation{obs("10.0.0.77", "AA:BB:CC:00:00:05", "new-name", 22, 443)}, now.Add(time.Minute))

	devices, _ := st.Devices()
	if len(devices) != 1 {
		t.Fatalf("want 1 device, got %d", len(devices))
	}
	d := devices[0]
	if d.IP != "10.0.0.77" {
		t.Errorf("ip not updated: %s", d.IP)
	}
	if d.Name != "new-name" {
		t.Errorf("name not updated: %s", d.Name)
	}
	if !d.FirstSeen.Before(d.LastSeen) {
		t.Errorf("first_seen %v should precede last_seen %v", d.FirstSeen, d.LastSeen)
	}
	if len(d.Services) != 2 {
		t.Errorf("want 2 services, got %d", len(d.Services))
	}
}

func TestSetPosition(t *testing.T) {
	st := testStore(t)
	now := time.Now()
	st.ApplyScan([]Observation{obs("10.0.0.9", "AA:BB:CC:00:00:09", "nine")}, now)

	devices, _ := st.Devices()
	if devices[0].HasPos {
		t.Fatal("device should start without a saved position")
	}
	if err := st.SetPosition(devices[0].ID, 123.5, -45.25); err != nil {
		t.Fatal(err)
	}
	d, err := st.Device(devices[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasPos || d.X != 123.5 || d.Y != -45.25 {
		t.Errorf("position not saved: %+v", d)
	}
	if err := st.SetPosition(9999, 0, 0); err == nil {
		t.Error("want error for unknown device")
	}
}

func TestStats(t *testing.T) {
	st := testStore(t)
	now := time.Now()
	st.ApplyScan([]Observation{
		obs("10.0.0.1", "AA:BB:CC:00:00:01", "one", 22),
		obs("10.0.0.2", "AA:BB:CC:00:00:02", "two", 80, 443),
	}, now)

	stats, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Online != 2 || stats.Services != 3 {
		t.Errorf("bad stats: %+v", stats)
	}
}

func TestMergeHistorical(t *testing.T) {
	st := testStore(t)
	now := time.Now()
	old := obs("10.0.0.50", "AA:BB:CC:00:00:50", "ghost", 80)
	if err := st.MergeHistorical(old, now.Add(-72*time.Hour), now.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	devices, _ := st.Devices()
	if len(devices) != 1 {
		t.Fatalf("want 1 device, got %d", len(devices))
	}
	d := devices[0]
	if d.Online {
		t.Error("historical device must be offline")
	}
	if got := time.Since(d.LastSeen).Hours(); got < 4.9 {
		t.Errorf("last_seen should be ~5h ago, got %.1fh", got)
	}
}
