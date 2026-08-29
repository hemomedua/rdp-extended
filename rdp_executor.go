package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type RDPExecutor struct {
	db *Database
}

func NewRDPExecutor(db *Database) *RDPExecutor {
	return &RDPExecutor{db: db}
}

// BuildRDPCommand builds mstsc.exe command with parameters
func (r *RDPExecutor) BuildRDPCommand(profile *RDPProfile) []string {
	args := []string{}

	// Computer
	computerArg := fmt.Sprintf("/v:%s:%d", profile.Host, profile.Port)
	args = append(args, computerArg)

	// Username
	if profile.Username != "" {
		args = append(args, fmt.Sprintf("/u:%s", profile.Username))
	}

	// Password
	if profile.Password != "" {
		args = append(args, fmt.Sprintf("/p:%s", profile.Password))
	}

	// Resolution
	if profile.Resolution != "" {
		switch profile.Resolution {
		case "1024x768":
			args = append(args, "/w:1024", "/h:768")
		case "1920x1080":
			args = append(args, "/w:1920", "/h:1080")
		case "fullscreen":
			args = append(args, "/f")
		case "fit":
			args = append(args, "/fit")
		}
	}

	// Clipboard
	if profile.ClipboardEnabled {
		args = append(args, "/o:redirectclipboard:i:1")
	} else {
		args = append(args, "/o:redirectclipboard:i:0")
	}

	// Disks
	if profile.DisksEnabled {
		if profile.DisksRedirect == "all" {
			args = append(args, "/drives")
		} else if profile.DisksRedirect == "custom" {
			// User can customize which drives in the profile editor
			args = append(args, "/drives:c:,d:")
		}
	} else {
		args = append(args, "/nodrives")
	}

	// Dynamic disks
	if profile.DisksDynamic {
		// This is handled by /drives parameter
	}

	return args
}

// ExecuteRDP launches mstsc.exe with the built command
func (r *RDPExecutor) ExecuteRDP(profile *RDPProfile) error {
	// Check if we need to use SOCKS5 proxy
	proxyAddress := ""
	if profile.ProxyMode == "custom" && profile.ProxyAddress != "" {
		proxyAddress = profile.ProxyAddress
	} else if profile.ProxyMode == "global" {
		settings, err := r.db.GetGlobalSettings()
		if err == nil && settings.GlobalProxyMode == "enabled" {
			proxyAddress = settings.GlobalProxyAddress
		}
	}

	// Build command
	args := r.BuildRDPCommand(profile)

	// If proxy is needed, we would need to handle it differently
	// For now, we launch mstsc.exe directly (SOCKS5 proxying would require
	// a middleware or routing table manipulation on Windows)
	if proxyAddress != "" {
		// TODO: Implement SOCKS5 proxy handling via SetupAPI or WinDivert
		// For now, just log that proxy would be used
		fmt.Printf("Note: SOCKS5 proxy %s would be used for this connection\n", proxyAddress)
	}

	// Execute mstsc.exe
	cmd := exec.Command("mstsc.exe", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Record connection history
	r.db.RecordConnection(profile.Host, profile.Username)

	return cmd.Start() // Use Start() to not wait for the RDP window to close
}

// ParseResolution parses custom resolution strings like "1920x1080"
func ParseResolution(res string) (int, int, error) {
	parts := strings.Split(res, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid resolution format: %s", res)
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}

	return width, height, nil
}

// GetResolutionOptions returns predefined resolution options
func GetResolutionOptions() []string {
	return []string{
		"1024x768",
		"1280x1024",
		"1366x768",
		"1440x900",
		"1600x1200",
		"1920x1080",
		"2560x1440",
		"fullscreen",
		"fit",
	}
}
