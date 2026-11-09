package web

import (
	"fmt"
	"html/template"
	"strings"
)

// iconPaths holds compact 24x24 SVG path data per icon key.
var iconPaths = map[string]string{
	// wifi arcs — routers and the Wi-See mark
	"router": "M12 17.5a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6zm0-4.7c-1.9 0-3.6.74-4.9 1.95l1.7 1.7A4.4 4.4 0 0 1 12 15.3c1.24 0 2.36.5 3.2 1.35l1.7-1.7A6.4 6.4 0 0 0 12 12.8zm0-4.7c-3.9 0-7.5 1.55-10.1 4.1l1.7 1.7A11.2 11.2 0 0 1 12 10.6c3.2 0 6.1 1.3 8.4 3.4l1.7-1.7A13.2 13.2 0 0 0 12 8.1z",
	// stacked server
	"server": "M4 3h16a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zm0 8h16a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-4a1 1 0 0 1 1-1zM6.5 5.2v1.6h2V5.2h-2zm0 8v1.6h2v-1.6h-2z",
	// monitor — computers and remote-desktop services
	"computer": "M4 4h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-6v2h3v2H7v-2h3v-2H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z",
	// handset
	"phone": "M8 2h8a1 1 0 0 1 1 1v18a1 1 0 0 1-1 1H8a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1zm3.2 17.2h1.6v1.6h-1.6v-1.6z",
	// printer
	"printer": "M7 3h10v4H7V3zM4 8h16a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-2v4H6v-4H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1zm4 7.5V19h8v-3.5H8z",
	// tv with play — media devices and services
	"media": "M3 5h18a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1h-7v2h2v2H8v-2h2v-2H3a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zm7 3.2v6.6l5.5-3.3L10 8.2z",
	// lightbulb — IoT
	"iot": "M12 2a6 6 0 0 0-3.6 10.8c.8.7 1.6 1.7 1.6 2.8v.4h4v-.4c0-1.1.8-2.1 1.6-2.8A6 6 0 0 0 12 2zm-2.5 16h5v.5a1.5 1.5 0 0 1-1.5 1.5h-2a1.5 1.5 0 0 1-1.5-1.5V18z",
	// database cylinder — NAS and databases
	"nas": "M12 3C7.6 3 5 4.6 5 6v12c0 1.4 2.6 3 7 3s7-1.6 7-3V6c0-1.4-2.6-3-7-3zm0 2.2c3.2 0 4.8.85 4.8.8s-1.6.8-4.8.8-4.8-.85-4.8-.8 1.6-.8 4.8-.8zM7 8.6c1.1.8 3 1.2 5 1.2s3.9-.4 5-1.2V12c0 .05-1.6.8-4.8.8S7.2 12.05 7.2 12V8.6zM7 14.6c1.1.8 3 1.2 5 1.2s3.9-.4 5-1.2V18c0 .05-1.6.8-4.8.8S7.2 18.05 7.2 18v-3.4z",
	// chip — unknown devices and generic services
	"unknown": "M9 2h2v2.5h2V2h2v2.5h2a1 1 0 0 1 1 1V7h2.5v2H19v2h2.5v2H19v2h2.5v2H19v1.5a1 1 0 0 1-1 1H15.5V22h-2v-2.5h-2V22h-2v-2.5H7a1 1 0 0 1-1-1V17H3.5v-2H6v-2H3.5v-2H6V9H3.5V7H6V5.5a1 1 0 0 1 1-1h2V2zm.5 6.5v7h5v-7h-5z",
	// globe — http and dns
	"globe": "M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm6.9 6h-2.4a15.4 15.4 0 0 0-1.2-3.2A8 8 0 0 1 18.9 8zM12 4c.5.6 1.3 2 1.9 4h-3.8c.6-2 1.4-3.4 1.9-4zM5.1 8a8 8 0 0 1 3.2-3.2A15.4 15.4 0 0 0 7.5 8H5.1zM7.1 10a16 16 0 0 0 0 4H4.1a8.2 8.2 0 0 1 0-4h3zm-2 6h2.4c.3 1.2.7 2.3 1.2 3.2A8 8 0 0 1 5.1 16zm6.9 4c-.5-.6-1.3-2-1.9-4h3.8c-.6 2-1.4 3.4-1.9 4zm3.3-.8a15.4 15.4 0 0 0 1.2-3.2h2.4a8 8 0 0 1-3.6 3.2zM16.9 14a16 16 0 0 0 0-4h3a8.2 8.2 0 0 1 0 4h-3z",
	// padlock — https and tls
	"lock": "M12 2a4 4 0 0 0-4 4v3H7a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V10a1 1 0 0 0-1-1h-1V6a4 4 0 0 0-4-4zm-2 7V6a2 2 0 1 1 4 0v3h-4zm2 4.5a1.8 1.8 0 0 1 .9 3.3V18h-1.8v-1.2a1.8 1.8 0 0 1 .9-3.3z",
	// terminal prompt — ssh
	"terminal": "M3 4h18a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1zm3.3 4.2L10.5 12l-4.2 3.8 1.4 1.4L12.8 12 7.7 6.8l-1.4 1.4zM12.5 15H18v2h-5.5v-2z",
	// folder — file services: smb, afp, ftp, rsync
	"folder": "M3 5h6.2l2 2H21a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1z",
	// envelope — mail services
	"mail": "M2 5h20a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H2a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zm10 6.6L4 7v1.8l8 4.8 8-4.8V7l-8 4.6z",
	// broadcast — mqtt and discovery services
	"radio": "M5.1 15.4a5 5 0 0 1 0-7l1.4 1.4a3 3 0 0 0 0 4.2l-1.4 1.4zm13.8 0l-1.4-1.4a3 3 0 0 0 0-4.2l1.4-1.4a5 5 0 0 1 0 7zM2.7 18a9 9 0 0 1 0-12.2l1.4 1.4a7 7 0 0 0 0 9.4L2.7 18zm18.6 0l-1.4-1.4a7 7 0 0 0 0-9.4l1.4-1.4a9 9 0 0 1 0 12.2zM10.8 11.6a1.4 1.4 0 1 1 2.8 0 1.4 1.4 0 0 1-2.8 0zm.5 2.4h1.8l.9 7h-3.6l.9-7z",
	// house — home automation hubs
	"home": "M12 3l9 7.5h-2.5V20h-4.2v-5.5h-4.6V20H5.5V10.5H3l9-7.5z",
	// play triangle
	"play": "M7 4l13 8-13 8V4z",
	// single dot — fallback
	"dot": "M12 8.5a3.5 3.5 0 1 1 0 7 3.5 3.5 0 0 1 0-7z",
}

// svcIcons maps service names to icon keys.
var svcIcons = map[string]string{
	"http": "globe", "http-alt": "globe", "http-dev": "globe", "dns": "globe",
	"mdns": "globe", "upnp": "radio",
	"https": "lock", "https-alt": "lock",
	"ssh": "terminal", "telnet": "terminal", "cockpit": "terminal",
	"smb": "folder", "netbios": "folder", "afp": "folder", "ftp": "folder",
	"rsync": "folder", "nfs": "folder",
	"mysql": "nas", "postgres": "nas", "redis": "nas",
	"smtp": "mail", "pop3": "mail", "pop3s": "mail", "imap": "mail", "imaps": "mail",
	"mqtt": "radio", "mqtts": "radio",
	"home-assistant": "home",
	"plex":           "play", "jellyfin": "play", "rtsp": "play", "chromecast": "play",
	"vnc": "computer", "rdp": "computer", "iphone-sync": "phone",
	"ipp": "printer", "pdl-print": "printer",
}

// icon renders an inline SVG icon at the given pixel size.
func icon(key string, size int) template.HTML {
	path, ok := iconPaths[key]
	if !ok {
		path = iconPaths["unknown"]
	}
	return template.HTML(fmt.Sprintf(
		`<svg viewBox="0 0 24 24" width="%d" height="%d" fill="currentColor" aria-hidden="true"><path d="%s"/></svg>`,
		size, size, path))
}

// serviceIcon renders the icon for a service name.
func serviceIcon(name string, size int) template.HTML {
	key, ok := svcIcons[strings.ToLower(name)]
	if !ok {
		key = "dot"
	}
	return icon(key, size)
}
