// Package demo seeds the database with a sample network so the UI can be
// explored without scanning anything.
package demo

import (
	"time"

	"wisee/internal/store"
)

// Seed loads a sample network: nine devices online now, three faded into
// history. Idempotent: repeated calls update the same rows.
func Seed(st *store.Store) error {
	now := time.Now()
	online := []store.Observation{
		{
			IP: "192.168.1.1", MAC: "50:C7:BF:11:22:33", Name: "gateway.home",
			Vendor: "TP-Link", Kind: "router",
			Services: []store.ServiceObs{
				{Port: 53, Name: "dns"},
				{Port: 80, Name: "http", Title: "Archer C7"},
			},
		},
		{
			IP: "192.168.1.10", MAC: "8C:8D:28:AA:BB:CC", Name: "fenrir",
			Vendor: "Intel", Kind: "computer", Self: true,
			Services: []store.ServiceObs{
				{Port: 22, Name: "ssh", Title: "SSH-2.0-OpenSSH_9.6"},
				{Port: 8080, Name: "http-alt", Title: "Wi-See"},
			},
		},
		{
			IP: "192.168.1.20", MAC: "00:11:32:44:55:66", Name: "helios-nas",
			Vendor: "Synology", Kind: "nas",
			Services: []store.ServiceObs{
				{Port: 445, Name: "smb"},
				{Port: 5000, Name: "upnp", Title: "Synology DiskStation"},
				{Port: 22, Name: "ssh", Title: "SSH-2.0-OpenSSH_8.4"},
			},
		},
		{
			IP: "192.168.1.30", MAC: "3C:52:82:77:88:99", Name: "office-printer",
			Vendor: "HP", Kind: "printer",
			Services: []store.ServiceObs{
				{Port: 631, Name: "ipp"},
				{Port: 9100, Name: "pdl-print"},
				{Port: 80, Name: "http", Title: "HP LaserJet Pro"},
			},
		},
		{
			IP: "192.168.1.40", MAC: "3C:22:FB:DE:AD:01", Name: "rowan-laptop",
			Vendor: "Apple", Kind: "computer",
			Services: []store.ServiceObs{
				{Port: 22, Name: "ssh", Title: "SSH-2.0-OpenSSH_9.9"},
			},
		},
		{
			IP: "192.168.1.50", MAC: "1C:F2:9A:BE:EF:02", Name: "pixel-8",
			Vendor: "Google", Kind: "phone",
		},
		{
			IP: "192.168.1.60", MAC: "9C:3D:CF:00:11:22", Name: "plexbox",
			Vendor: "Netgear", Kind: "media",
			Services: []store.ServiceObs{
				{Port: 32400, Name: "plex", Title: "Plex Media Server"},
				{Port: 22, Name: "ssh"},
			},
		},
		{
			IP: "192.168.1.70", MAC: "B8:27:EB:33:44:55", Name: "home-assistant",
			Vendor: "Raspberry Pi", Kind: "server",
			Services: []store.ServiceObs{
				{Port: 8123, Name: "home-assistant", Title: "Home Assistant"},
				{Port: 22, Name: "ssh"},
				{Port: 1883, Name: "mqtt"},
			},
		},
		{
			IP: "192.168.1.100", MAC: "24:6F:28:66:77:88", Name: "tasmota-kettle",
			Vendor: "Espressif", Kind: "iot",
			Services: []store.ServiceObs{
				{Port: 80, Name: "http", Title: "Tasmota"},
			},
		},
	}
	if err := st.ApplyScan(online, now); err != nil {
		return err
	}

	history := []struct {
		obs       store.Observation
		firstSeen time.Time
		lastSeen  time.Time
	}{
		{
			obs: store.Observation{
				IP: "192.168.1.80", MAC: "54:60:09:99:AA:BB", Name: "living-room-tv",
				Vendor: "Google", Kind: "media",
				Services: []store.ServiceObs{
					{Port: 8008, Name: "chromecast"},
					{Port: 8009, Name: "chromecast"},
				},
			},
			firstSeen: now.Add(-90 * 24 * time.Hour),
			lastSeen:  now.Add(-3 * time.Hour),
		},
		{
			obs: store.Observation{
				IP: "192.168.1.90", MAC: "A4:83:E7:CC:DD:EE", Name: "juno-iphone",
				Vendor: "Apple", Kind: "phone",
			},
			firstSeen: now.Add(-200 * 24 * time.Hour),
			lastSeen:  now.Add(-2 * 24 * time.Hour),
		},
		{
			obs: store.Observation{
				IP: "192.168.1.101", MAC: "30:AE:A4:12:34:56", Name: "shelly-lamp",
				Vendor: "Espressif", Kind: "iot",
				Services: []store.ServiceObs{
					{Port: 80, Name: "http", Title: "Shelly 1PM"},
				},
			},
			firstSeen: now.Add(-30 * 24 * time.Hour),
			lastSeen:  now.Add(-26 * time.Minute),
		},
	}
	for _, h := range history {
		if err := st.MergeHistorical(h.obs, h.firstSeen, h.lastSeen); err != nil {
			return err
		}
	}
	return nil
}
