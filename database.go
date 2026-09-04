package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Database struct {
	Profiles []RDPProfile   `json:"profiles"`
	Settings GlobalSettings `json:"settings"`
	History  []HistoryEntry `json:"history"`
	mu       sync.RWMutex
	dbPath   string
}

type HistoryEntry struct {
	Host        string `json:"host"`
	Username    string `json:"username"`
	ConnectedAt string `json:"connected_at"`
}

type RDPProfile struct {
	ID           int
	Host         string
	Port         int
	Username     string
	Password     string
	Resolution   string // "" = inherit global default; else "1024x768"/"1920x1080"/.../"fullscreen"/"fit"/custom "WxH"
	ColorDepth   string // "" = inherit; else "15"/"16"/"24"/"32"
	ClipboardMode string // "" = inherit; "on"; "off"
	DisksRedirect string // "" = inherit; "none"; "all"; "dynamic" (only drives attached after the session starts)
	ProxyMode    string // "direct", "global", "custom"
	ProxyAddress string // host:port for custom
}

type GlobalSettings struct {
	GlobalProxyMode    string `json:"global_proxy_mode"`    // "disabled", "enabled"
	GlobalProxyAddress string `json:"global_proxy_address"` // host:port

	DefaultResolution    string `json:"default_resolution"`     // concrete value, e.g. "1920x1080", "fullscreen", "fit"
	DefaultColorDepth    string `json:"default_color_depth"`    // "15"/"16"/"24"/"32"
	DefaultClipboard     bool   `json:"default_clipboard"`
	DefaultDisksRedirect string `json:"default_disks_redirect"` // "none"/"all"/"dynamic"

	// SettingsVersion lets us tell a freshly-created settings object (which
	// has correct built-in defaults) apart from one loaded from an older
	// database file that predates these fields (which would otherwise read
	// back as zero-valued/false). See GetGlobalSettings.
	SettingsVersion int `json:"settings_version"`
}

const currentSettingsVersion = 2

func NewDatabase(dbPath string) (*Database, error) {
	// Create directory if not exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	database := &Database{
		dbPath:   dbPath,
		Profiles: []RDPProfile{},
		Settings: GlobalSettings{
			GlobalProxyMode:      "disabled",
			DefaultResolution:    "1920x1080",
			DefaultColorDepth:    "32",
			DefaultClipboard:     true,
			DefaultDisksRedirect: "all",
			SettingsVersion:      currentSettingsVersion,
		},
		History: []HistoryEntry{},
	}

	// Load existing database if it exists
	if err := database.load(); err != nil {
		// If file doesn't exist, that's ok - we'll create it on first save
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return database, nil
}

func (d *Database) load() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := os.ReadFile(d.dbPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, d)
}

// saveLocked writes the database to disk. Callers MUST already hold d.mu (write lock).
func (d *Database) saveLocked() error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(d.dbPath, data, 0644)
}

func (d *Database) AddProfile(profile *RDPProfile) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Find and update existing, or add new
	for i, p := range d.Profiles {
		if p.Host == profile.Host && p.Username == profile.Username {
			d.Profiles[i] = *profile
			return d.saveLocked()
		}
	}

	// Add new profile
	d.Profiles = append(d.Profiles, *profile)
	return d.saveLocked()
}

func (d *Database) GetProfilesByHost(host string) ([]RDPProfile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var profiles []RDPProfile
	for _, p := range d.Profiles {
		if p.Host == host {
			profiles = append(profiles, p)
		}
	}

	// Sort by username
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Username < profiles[j].Username
	})

	return profiles, nil
}

func (d *Database) GetAllHosts() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	hostMap := make(map[string]bool)
	for _, p := range d.Profiles {
		hostMap[p.Host] = true
	}

	var hosts []string
	for h := range hostMap {
		hosts = append(hosts, h)
	}

	sort.Strings(hosts)
	return hosts, nil
}

func (d *Database) GetProfile(host, username string) (*RDPProfile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, p := range d.Profiles {
		if p.Host == host && p.Username == username {
			return &p, nil
		}
	}

	return nil, nil
}

func (d *Database) DeleteProfile(host, username string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, p := range d.Profiles {
		if p.Host == host && p.Username == username {
			// Remove from slice
			d.Profiles = append(d.Profiles[:i], d.Profiles[i+1:]...)
			return d.saveLocked()
		}
	}

	return fmt.Errorf("profile not found")
}

func (d *Database) GetGlobalSettings() (*GlobalSettings, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	s := d.Settings
	if s.GlobalProxyMode == "" {
		s.GlobalProxyMode = "disabled"
	}

	// A database saved before these fields existed will have them at their
	// Go zero values (empty string / false) after loading, which would be
	// misread as "disabled"/"none" rather than "never configured". Backfill
	// sensible defaults on every read until the user actually opens Settings
	// and saves - at which point SaveGlobalSettings stamps the current
	// version and their explicit choices (including any false/none) stick.
	if s.SettingsVersion < currentSettingsVersion {
		if s.DefaultResolution == "" {
			s.DefaultResolution = "1920x1080"
		}
		if s.DefaultColorDepth == "" {
			s.DefaultColorDepth = "32"
		}
		if s.DefaultDisksRedirect == "" {
			s.DefaultDisksRedirect = "all"
		}
		s.DefaultClipboard = true
	}

	return &s, nil
}

func (d *Database) SaveGlobalSettings(settings *GlobalSettings) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	settings.SettingsVersion = currentSettingsVersion
	d.Settings = *settings
	return d.saveLocked()
}

func (d *Database) RecordConnection(host, username string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := HistoryEntry{
		Host:        host,
		Username:    username,
		ConnectedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	d.History = append(d.History, entry)

	// Keep only last 100 entries
	if len(d.History) > 100 {
		d.History = d.History[len(d.History)-100:]
	}

	return d.saveLocked()
}

// GetHistory returns a copy of recorded connection history, most recent last.
func (d *Database) GetHistory() []HistoryEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]HistoryEntry, len(d.History))
	copy(out, d.History)
	return out
}

func (d *Database) Close() error {
	return nil // JSON database doesn't need explicit close
}
