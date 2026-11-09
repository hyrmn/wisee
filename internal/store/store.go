// Package store keeps the device inventory in SQLite. Devices are never
// deleted: anything not seen in the latest scan becomes history.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

// Device is one host on the network.
type Device struct {
	ID        int64
	MAC       string
	Name      string
	IP        string
	Vendor    string
	Kind      string
	Self      bool
	Online    bool
	FirstSeen time.Time
	LastSeen  time.Time
	X         float64
	Y         float64
	HasPos    bool
	Services  []Service
}

// Service is one open TCP port on a device.
type Service struct {
	ID        int64
	DeviceID  int64
	Port      int
	Name      string
	Title     string
	Online    bool
	FirstSeen time.Time
	LastSeen  time.Time
}

// Observation is what one scan learned about one host.
type Observation struct {
	IP       string
	MAC      string
	Name     string
	Vendor   string
	Kind     string
	Self     bool
	Services []ServiceObs
}

// ServiceObs is what one scan learned about one open port.
type ServiceObs struct {
	Port  int
	Name  string
	Title string
}

// Stats summarizes the inventory.
type Stats struct {
	Total    int
	Online   int
	Services int
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (and creates if needed) the database at path.
// Use ":memory:" for an in-memory database.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; a single connection keeps the
	// in-memory variant consistent too.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS devices (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  mac        TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT '',
  vendor     TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'unknown',
  self       INTEGER NOT NULL DEFAULT 0,
  online     INTEGER NOT NULL DEFAULT 0,
  first_seen INTEGER NOT NULL,
  last_seen  INTEGER NOT NULL,
  x          REAL NOT NULL DEFAULT 0,
  y          REAL NOT NULL DEFAULT 0,
  has_pos    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_devices_mac ON devices(mac);
CREATE INDEX IF NOT EXISTS idx_devices_ip ON devices(ip);
CREATE TABLE IF NOT EXISTS services (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id  INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  port       INTEGER NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  online     INTEGER NOT NULL DEFAULT 0,
  first_seen INTEGER NOT NULL,
  last_seen  INTEGER NOT NULL,
  UNIQUE(device_id, port)
);
CREATE INDEX IF NOT EXISTS idx_services_device ON services(device_id);
`)
	return err
}

// ApplyScan records the result of one scan: every previously known device and
// service is marked offline first, then each observation marks itself online
// and updates last_seen.
func (s *Store) ApplyScan(obs []Observation, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE devices SET online=0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE services SET online=0`); err != nil {
		return err
	}
	for _, o := range obs {
		if _, err := mergeObservation(tx, o, now, true); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MergeHistorical records a device as seen at the given time but currently
// offline. Used for seeding sample data.
func (s *Store) MergeHistorical(o Observation, firstSeen, lastSeen time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, err := mergeObservation(tx, o, lastSeen, false)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE devices SET first_seen=? WHERE id=? AND first_seen>?`,
		firstSeen.Unix(), id, firstSeen.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeObservation(tx *sql.Tx, o Observation, t time.Time, online bool) (int64, error) {
	var id int64
	found := false
	if o.MAC != "" {
		err := tx.QueryRow(`SELECT id FROM devices WHERE mac=?`, o.MAC).Scan(&id)
		if err == nil {
			found = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	if !found && o.IP != "" {
		// Match by IP only for devices we never learned a MAC for.
		err := tx.QueryRow(`SELECT id FROM devices WHERE mac='' AND ip=?`, o.IP).Scan(&id)
		if err == nil {
			found = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	if found {
		_, err := tx.Exec(`UPDATE devices SET
			ip=?,
			name=CASE WHEN ?='' THEN name ELSE ? END,
			vendor=CASE WHEN ?='' THEN vendor ELSE ? END,
			kind=CASE WHEN ?='unknown' OR ?='' THEN kind ELSE ? END,
			self=self|?,
			online=?,
			last_seen=?
		  WHERE id=?`,
			o.IP, o.Name, o.Name, o.Vendor, o.Vendor, o.Kind, o.Kind, o.Kind,
			boolInt(o.Self), boolInt(online), t.Unix(), id)
		if err != nil {
			return 0, err
		}
	} else {
		res, err := tx.Exec(`INSERT INTO devices
			(mac, name, ip, vendor, kind, self, online, first_seen, last_seen)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			o.MAC, o.Name, o.IP, o.Vendor, o.Kind, boolInt(o.Self), boolInt(online),
			t.Unix(), t.Unix())
		if err != nil {
			return 0, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	}
	for _, svc := range o.Services {
		_, err := tx.Exec(`INSERT INTO services (device_id, port, name, title, online, first_seen, last_seen)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(device_id, port) DO UPDATE SET
			name=CASE WHEN excluded.name='' THEN services.name ELSE excluded.name END,
			title=CASE WHEN excluded.title='' THEN services.title ELSE excluded.title END,
			online=excluded.online,
			last_seen=excluded.last_seen`,
			id, svc.Port, svc.Name, svc.Title, boolInt(online), t.Unix(), t.Unix())
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

// Devices returns every known device with its services, ordered by first
// appearance so map layout is stable between scans.
func (s *Store) Devices() ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, mac, name, ip, vendor, kind, self, online,
		first_seen, last_seen, x, y, has_pos FROM devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	byID := map[int64]*Device{}
	for rows.Next() {
		var d Device
		var self, online, hasPos int
		var first, last int64
		if err := rows.Scan(&d.ID, &d.MAC, &d.Name, &d.IP, &d.Vendor, &d.Kind,
			&self, &online, &first, &last, &d.X, &d.Y, &hasPos); err != nil {
			return nil, err
		}
		d.Self = self != 0
		d.Online = online != 0
		d.HasPos = hasPos != 0
		d.FirstSeen = time.Unix(first, 0)
		d.LastSeen = time.Unix(last, 0)
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range devices {
		byID[devices[i].ID] = &devices[i]
	}
	svcRows, err := s.db.Query(`SELECT id, device_id, port, name, title, online, first_seen, last_seen
		FROM services ORDER BY port`)
	if err != nil {
		return nil, err
	}
	defer svcRows.Close()
	for svcRows.Next() {
		var svc Service
		var online int
		var first, last int64
		if err := svcRows.Scan(&svc.ID, &svc.DeviceID, &svc.Port, &svc.Name, &svc.Title,
			&online, &first, &last); err != nil {
			return nil, err
		}
		svc.Online = online != 0
		svc.FirstSeen = time.Unix(first, 0)
		svc.LastSeen = time.Unix(last, 0)
		if d, ok := byID[svc.DeviceID]; ok {
			d.Services = append(d.Services, svc)
		}
	}
	return devices, svcRows.Err()
}

// Device returns one device with its services.
func (s *Store) Device(id int64) (*Device, error) {
	devices, err := s.Devices()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].ID == id {
			return &devices[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

// SetPosition saves a device's map position.
func (s *Store) SetPosition(id int64, x, y float64) error {
	res, err := s.db.Exec(`UPDATE devices SET x=?, y=?, has_pos=1 WHERE id=?`, x, y, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Stats summarizes the inventory.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	err := s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM devices),
		(SELECT COUNT(*) FROM devices WHERE online=1),
		(SELECT COUNT(*) FROM services WHERE online=1)`).
		Scan(&st.Total, &st.Online, &st.Services)
	return st, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SortByIP orders devices numerically by IPv4 address; devices without an IP
// sort last, by ID.
func SortByIP(devices []Device) {
	sort.SliceStable(devices, func(i, j int) bool {
		a, b := devices[i], devices[j]
		if a.IP == "" || b.IP == "" {
			return a.ID < b.ID
		}
		return ipKey(a.IP) < ipKey(b.IP)
	})
}

func ipKey(ip string) string {
	var a, b, c, d int
	if _, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil {
		return "~~~" + ip
	}
	return fmt.Sprintf("%03d.%03d.%03d.%03d", a, b, c, d)
}
