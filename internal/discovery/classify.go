package discovery

import "strings"

// oui maps curated MAC prefixes to vendors. It is intentionally small: common
// home-network makers only, not the full IEEE registry.
var oui = map[string]string{
	"3C:22:FB": "Apple", "A4:83:E7": "Apple", "F0:18:98": "Apple",
	"8C:85:90": "Apple", "78:4F:43": "Apple", "14:7D:DA": "Apple",
	"1C:F2:9A": "Google", "54:60:09": "Google", "F4:F5:D8": "Google",
	"94:EB:2C": "Google", "3C:5A:B4": "Google",
	"44:65:0D": "Amazon", "68:37:E9": "Amazon", "FC:A1:83": "Amazon",
	"74:C2:46": "Amazon", "A0:02:DC": "Amazon",
	"E8:50:8B": "Samsung", "8C:F5:A3": "Samsung", "34:23:BA": "Samsung",
	"B8:27:EB": "Raspberry Pi", "DC:A6:32": "Raspberry Pi", "E4:5F:01": "Raspberry Pi",
	"D8:3A:DD": "Raspberry Pi",
	"24:6F:28": "Espressif", "30:AE:A4": "Espressif", "A4:CF:12": "Espressif",
	"84:F7:03": "Espressif", "10:D5:61": "Tuya",
	"4C:79:6E": "Intel", "8C:8D:28": "Intel", "94:C6:91": "Intel",
	"50:C7:BF": "TP-Link", "98:DA:C4": "TP-Link", "60:32:B1": "TP-Link",
	"A0:40:A0": "Netgear", "9C:3D:CF": "Netgear", "28:C6:8E": "Netgear",
	"04:D9:F5": "ASUS", "2C:56:DC": "ASUS", "AC:22:0B": "ASUS",
	"00:11:32": "Synology", "24:5E:BE": "QNAP", "00:08:9B": "QNAP",
	"00:17:88": "Philips", "EC:B5:FA": "Philips Hue",
	"48:A6:B8": "Sonos", "5C:AA:FD": "Sonos", "94:9F:3E": "Sonos", "B8:E9:37": "Sonos",
	"64:CC:2E": "Xiaomi", "78:11:DC": "Xiaomi",
	"F0:9F:C2": "Ubiquiti", "78:8A:20": "Ubiquiti", "24:5A:4C": "Ubiquiti", "E0:63:DA": "Ubiquiti",
	"D4:01:C3": "MikroTik", "48:8F:5A": "MikroTik", "B8:69:F4": "MikroTik",
	"AC:3A:7A": "Roku", "B0:A7:37": "Roku",
	"3C:52:82": "HP", "FC:3F:DB": "HP",
	"28:18:78": "Microsoft", "7C:1E:52": "Microsoft",
	"AC:E2:15": "Huawei", "34:6B:46": "Huawei",
}

// Vendor returns the vendor for a MAC address, or "" when unknown.
func Vendor(mac string) string {
	if len(mac) < 8 {
		return ""
	}
	return oui[strings.ToUpper(mac[:8])]
}

// Classify guesses the device kind from vendor, hostname, address and
// observed services. One of: router, server, computer, phone, printer,
// media, nas, iot, unknown.
func Classify(vendor, hostname, ip string, isGateway, isSelf bool, ports []int) string {
	h := strings.ToLower(hostname)
	v := strings.ToLower(vendor)
	has := func(want ...int) bool {
		for _, w := range want {
			for _, p := range ports {
				if p == w {
					return true
				}
			}
		}
		return false
	}
	contains := func(s string, needles ...string) bool {
		for _, n := range needles {
			if strings.Contains(s, n) {
				return true
			}
		}
		return false
	}

	switch {
	case isGateway:
		return "router"
	case isSelf:
		return "computer"
	case contains(h, "iphone", "ipad", "pixel", "galaxy", "phone"):
		return "phone"
	case contains(h, "tasmota", "shelly", "wled") || contains(v, "espressif", "tuya"):
		return "iot"
	case contains(v, "philips", "sonos", "roku") || has(8008, 8009):
		return "media"
	case has(9100, 631):
		return "printer"
	case has(32400, 8096, 554):
		return "media"
	case contains(v, "synology", "qnap") || has(548):
		return "nas"
	case strings.HasSuffix(ip, ".1") && has(53, 80):
		return "router"
	case contains(v, "raspberry pi"):
		return "server"
	case contains(v, "apple"):
		return "computer"
	case has(8123, 1883, 8883):
		return "iot"
	case has(22):
		return "server"
	case has(3389, 5900, 445, 139):
		return "computer"
	case contains(v, "google", "amazon", "xiaomi"):
		return "iot"
	default:
		return "unknown"
	}
}
