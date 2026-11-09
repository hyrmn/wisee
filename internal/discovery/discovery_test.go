package discovery

import (
	"bufio"
	"strings"
	"testing"
)

func TestParsePortsSpec(t *testing.T) {
	ports, err := ParsePortsSpec("8000, 9000-9002")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{8000, 9000, 9001, 9002}
	if len(ports) != len(want) {
		t.Fatalf("got %v", ports)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("got %v, want %v", ports, want)
		}
	}
	if _, err := ParsePortsSpec("abc"); err == nil {
		t.Error("want error for bad port")
	}
	if _, err := ParsePortsSpec("9000-8000"); err == nil {
		t.Error("want error for reversed range")
	}
	if _, err := ParsePortsSpec("70000"); err == nil {
		t.Error("want error for out-of-range port")
	}
}

func TestNormalizeMAC(t *testing.T) {
	for in, want := range map[string]string{
		"3c:22:fb:aa:bb:cc": "3C:22:FB:AA:BB:CC",
		"3C-22-FB-AA-BB-CC": "3C:22:FB:AA:BB:CC",
		"0:1:2:3:4:5":       "00:01:02:03:04:05",
		"not-a-mac":         "",
		"":                  "",
	} {
		if got := normalizeMAC(in); got != want {
			t.Errorf("normalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseARPLinux(t *testing.T) {
	sample := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         50:c7:bf:11:22:33     *        wlp3s0
192.168.1.5      0x1         0x0         00:00:00:00:00:00     *        wlp3s0
192.168.1.9      0x1         0x2         3c:22:fb:aa:bb:cc     *        wlp3s0
`
	out := map[string]string{}
	parseARPLinux(bufio.NewScanner(strings.NewReader(sample)), out)
	if len(out) != 2 {
		t.Fatalf("want 2 entries, got %v", out)
	}
	if out["192.168.1.1"] != "50:C7:BF:11:22:33" {
		t.Errorf("bad mac: %q", out["192.168.1.1"])
	}
	if _, ok := out["192.168.1.5"]; ok {
		t.Error("incomplete ARP entry must be skipped")
	}
}

func TestVendor(t *testing.T) {
	if got := Vendor("B8:27:EB:11:22:33"); got != "Raspberry Pi" {
		t.Errorf("want Raspberry Pi, got %q", got)
	}
	if got := Vendor("FF:FF:FF:11:22:33"); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name             string
		vendor, hostname string
		ip               string
		gateway, self    bool
		ports            []int
		want             string
	}{
		{name: "gateway", gateway: true, want: "router"},
		{name: "self", self: true, want: "computer"},
		{name: "iphone", vendor: "Apple", hostname: "juno-iphone", want: "phone"},
		{name: "tasmota", vendor: "Espressif", hostname: "tasmota-kettle", ports: []int{80}, want: "iot"},
		{name: "printer by ports", ports: []int{9100}, want: "printer"},
		{name: "plex", ports: []int{32400}, want: "media"},
		{name: "chromecast", vendor: "Google", ports: []int{8008}, want: "media"},
		{name: "synology", vendor: "Synology", ports: []int{445}, want: "nas"},
		{name: "router on .1", ip: "10.0.0.1", ports: []int{53, 80}, want: "router"},
		{name: "pi", vendor: "Raspberry Pi", ports: []int{22}, want: "server"},
		{name: "ssh host", ports: []int{22}, want: "server"},
		{name: "smb host", ports: []int{445}, want: "computer"},
		{name: "home assistant", ports: []int{8123}, want: "iot"},
		{name: "mystery", want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.vendor, tc.hostname, tc.ip, tc.gateway, tc.self, tc.ports)
			if got != tc.want {
				t.Errorf("Classify() = %q, want %q", got, tc.want)
			}
		})
	}
}
