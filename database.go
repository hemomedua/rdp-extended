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
	ID               int
	Host             string
	Port             int
	Username         string
	Password         string
	Resolution       string // "1024x768", "1920x1080", "fullscreen", "custom"
	ClipboardEnabled bool
	DisksEnabled     bool
	DisksRedirect    string // "all", "none", "custom"
	DisksDynamic     bool   // redirection drives plugged in later
	ProxyMode        string // "direct", "global", "custom"
	ProxyAddress     string // host:port for custom
}

type GlobalSettings struct {
	GlobalProxyMode    string `json:"global_proxy_mode"`    // "disabled", "enabled"
	GlobalProxyAddress string `json:"global_proxy_address"` // host:port
}

func NewDatabase(dbPath string) (*Database, error) {
	// Create directory if not exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	database := &Database{
		dbPath:   dbPath,
		Profiles: []RDPProfile{},
		Settings: GlobalSettings{GlobalProxyMode: "disabled"},
		History:  []HistoryEntry{},
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

	if d.Settings.GlobalProxyMode == "" {
		d.Settings.GlobalProxyMode = "disabled"
	}
	return &d.Settings, nil
}

func (d *Database) SaveGlobalSettings(settings *GlobalSettings) error {
	d.mu.Lock()
	defer d.mu.Unlock()

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
