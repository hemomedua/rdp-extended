package main

import (
	"os/exec"
	"strings"
)

// GetSystemHosts retrieves RDP connection history from Windows registry
func GetSystemHosts() ([]string, error) {
	var hosts []string
	hostMap := make(map[string]bool)

	// Read from HKCU\Software\Microsoft\Terminal Server Client\Default
	cmd := exec.Command("reg", "query", 
		"HKEY_CURRENT_USER\\Software\\Microsoft\\Terminal Server Client\\Default",
		"/v", "MRU0", "/v", "MRU1", "/v", "MRU2", "/v", "MRU3", "/v", "MRU4")

	output, err := cmd.Output()
	if err == nil {
		parseRegOutput(string(output), &hostMap)
	}

	// Also read from recent servers list
	cmd = exec.Command("reg", "query",
		"HKEY_CURRENT_USER\\Software\\Microsoft\\Terminal Server Client\\Servers")

	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.Contains(line, "HKEY_CURRENT_USER") {
				parts := strings.Split(line, "\\")
				if len(parts) > 0 {
					host := strings.TrimSpace(parts[len(parts)-1])
					if host != "" && !strings.Contains(host, "REG_") {
						hostMap[host] = true
					}
				}
			}
		}
	}

	// Convert map to slice
	for host := range hostMap {
		if host != "" {
			hosts = append(hosts, host)
		}
	}

	return hosts, nil
}

func parseRegOutput(output string, hostMap *map[string]bool) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Registry output format: "    MRU0    REG_SZ    192.168.1.1"
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			// Last part is usually the value (hostname/IP)
			host := parts[len(parts)-1]
			if host != "" {
				(*hostMap)[host] = true
			}
		}
	}
}

// GetSystemHostsWithDB merges system hosts with database hosts
func GetSystemHostsWithDB(db *Database) ([]string, error) {
	systemHosts, _ := GetSystemHosts() // Ignore errors, just get what we can
	dbHosts, err := db.GetAllHosts()
	if err != nil {
		return systemHosts, nil
	}

	// Merge and deduplicate
	hostMap := make(map[string]bool)
	for _, h := range systemHosts {
		hostMap[h] = true
	}
	for _, h := range dbHosts {
		hostMap[h] = true
	}

	var result []string
	for h := range hostMap {
		result = append(result, h)
	}

	return result, nil
}
