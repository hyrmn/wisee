// Package web serves the Wi-See UI: an HTMX-driven map of device bubbles,
// a detail panel per device, and a small JSON API for positions and scans.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wisee/internal/store"
)

//go:embed templates static
var content embed.FS

// Server serves the UI.
type Server struct {
	st       *store.Store
	version  string
	tpl      *template.Template
	static   http.Handler
	RunScan  func() // nil in demo mode
	scanning atomic.Bool
	lastScan atomic.Int64 // unix nanos of last completed scan
}

// New parses templates and builds the server.
func New(st *store.Store, version string) (*Server, error) {
	funcs := template.FuncMap{
		"fnum":    fnum,
		"human":   human,
		"icon":    icon,
		"svcicon": serviceIcon,
		"label":   label,
	}
	tpl, err := template.New("").Funcs(funcs).ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(content, "static")
	if err != nil {
		return nil, err
	}
	return &Server{
		st:      st,
		version: version,
		tpl:     tpl,
		static:  http.FileServerFS(staticFS),
	}, nil
}

// MarkScanned records a completed scan for the header status line.
func (s *Server) MarkScanned() { s.lastScan.Store(time.Now().UnixNano()) }

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /map/devices", s.handleDevices)
	mux.HandleFunc("GET /devices/{id}", s.handleDevice)
	mux.HandleFunc("POST /devices/{id}/position", s.handlePosition)
	mux.HandleFunc("POST /scan", s.handleScan)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	stats, err := s.st.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "index", map[string]any{
		"Version": s.version,
		"Stats":   stats,
	})
}

// handleDevices renders the device/service bubble layer. Polled by HTMX.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.st.Devices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stats, _ := s.st.Stats()
	s.render(w, "devices", map[string]any{
		"Devices":   layout(devices),
		"Stats":     stats,
		"ScanState": s.scanState(),
	})
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	dev, err := s.st.Device(id)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	s.render(w, "detail", deviceVM{Device: *dev, Services: orbit(dev.Services)})
}

func (s *Server) handlePosition(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	x, errX := strconv.ParseFloat(r.FormValue("x"), 64)
	y, errY := strconv.ParseFloat(r.FormValue("y"), 64)
	if errX != nil || errY != nil || math.Abs(x) > 1e6 || math.Abs(y) > 1e6 {
		http.Error(w, "bad coordinates", http.StatusBadRequest)
		return
	}
	if err := s.st.SetPosition(id, x, y); err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if s.RunScan == nil {
		s.render(w, "scan-state", "demo mode: scanning disabled")
		return
	}
	if s.scanning.CompareAndSwap(false, true) {
		go func() {
			defer s.scanning.Store(false)
			s.RunScan()
			s.lastScan.Store(time.Now().UnixNano())
		}()
		s.render(w, "scan-state", "scanning…")
		return
	}
	s.render(w, "scan-state", "already scanning…")
}

func (s *Server) scanState() string {
	if s.scanning.Load() {
		return "scanning…"
	}
	if t := s.lastScan.Load(); t > 0 {
		return "last scan " + human(time.Unix(0, t))
	}
	return "waiting for first scan…"
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// deviceVM is a device plus everything the map template needs.
type deviceVM struct {
	store.Device
	X, Y     float64
	IsHub    bool
	Edge     *edgeVM // line to the hub, nil for the hub itself
	Services []serviceVM
}

type edgeVM struct {
	X1, Y1 float64
	Len    float64
	Deg    float64
}

type serviceVM struct {
	store.Service
	X, Y float64
}

// layout assigns positions: saved positions win, everything else lands on a
// phyllotaxis spiral around the hub (the router, else this host).
func layout(devices []store.Device) []deviceVM {
	sort.SliceStable(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })

	hubIdx := -1
	for i, d := range devices {
		if d.Kind == "router" {
			hubIdx = i
			break
		}
	}
	if hubIdx < 0 {
		for i, d := range devices {
			if d.Self {
				hubIdx = i
				break
			}
		}
	}

	vms := make([]deviceVM, len(devices))
	spiral := 0
	for i, d := range devices {
		vm := deviceVM{Device: d}
		if d.HasPos {
			vm.X, vm.Y = d.X, d.Y
		} else if i == hubIdx {
			vm.X, vm.Y = 0, 0
		} else {
			// Phyllotaxis: even, organic spread; deterministic per device set.
			spiral++
			angle := float64(spiral) * 137.508 * math.Pi / 180
			radius := 170 * math.Sqrt(float64(spiral))
			vm.X = radius * math.Cos(angle)
			vm.Y = radius * math.Sin(angle)
		}
		vm.Services = orbit(d.Services)
		vms[i] = vm
	}

	if hubIdx >= 0 {
		vms[hubIdx].IsHub = true
		hub := vms[hubIdx]
		for i := range vms {
			if i == hubIdx {
				continue
			}
			dx := vms[i].X - hub.X
			dy := vms[i].Y - hub.Y
			vms[i].Edge = &edgeVM{
				X1:  hub.X,
				Y1:  hub.Y,
				Len: math.Hypot(dx, dy),
				Deg: math.Atan2(dy, dx) * 180 / math.Pi,
			}
		}
	}
	return vms
}

// orbit arranges services in rings around their device bubble.
func orbit(services []store.Service) []serviceVM {
	out := make([]serviceVM, len(services))
	for i, svc := range services {
		ring := i / 8
		posInRing := i % 8
		ringCount := len(services) - ring*8
		if ringCount > 8 {
			ringCount = 8
		}
		angle := -math.Pi/2 + 2*math.Pi*float64(posInRing)/float64(ringCount)
		radius := 52 + float64(ring)*26
		out[i] = serviceVM{
			Service: svc,
			X:       radius * math.Cos(angle),
			Y:       radius * math.Sin(angle),
		}
	}
	return out
}

// label picks the display name for a device.
func label(d store.Device) string {
	name := d.Name
	name = strings.TrimSuffix(name, ".")
	for _, suffix := range []string{".local", ".lan", ".home", ".internal"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" {
		name = d.IP
	}
	return name
}

// fnum formats a coordinate for CSS.
func fnum(f float64) string {
	return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64)
}

// human renders a time as a relative "ago" string.
func human(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
