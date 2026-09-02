package main

import (
	"os/exec"
	"strings"
)

const (
	regDefaultKey = `HKEY_CURRENT_USER\Software\Microsoft\Terminal Server Client\Default`
	regServersKey = `HKEY_CURRENT_USER\Software\Microsoft\Terminal Server Client\Servers`
)

// GetSystemHosts retrieves RDP connection history from the Windows registry.
// It reads two locations used by mstsc.exe:
//   - ...\Terminal Server Client\Default          (values MRU0, MRU1, ... = recently typed hosts)
//   - ...\Terminal Server Client\Servers\<host>    (one subkey per host ever connected to)
func GetSystemHosts() ([]string, error) {
	hostMap := make(map[string]bool)

	// 1) Recently typed hosts (MRU list). NOTE: `reg query` only accepts a single
	// key argument to list ALL of its values - it does not take multiple /v flags.
	if out, err := exec.Command("reg", "query", regDefaultKey).Output(); err == nil {
		for _, host := range parseRegMRUValues(string(out)) {
			hostMap[host] = true
		}
	}

	// 2) Every host ever connected to is stored as a subkey under "Servers".
	if out, err := exec.Command("reg", "query", regServersKey).Output(); err == nil {
		for _, host := range parseRegSubkeys(string(out), regServersKey) {
			hostMap[host] = true
		}
	}

	var hosts []string
	for host := range hostMap {
		if host != "" {
			hosts = append(hosts, host)
		}
	}

	return hosts, nil
}

// parseRegMRUValues parses `reg query` output for a key and returns the values
// of any MRU* entries. Typical line looks like:
//
//	    MRU0    REG_SZ    192.168.1.100
func parseRegMRUValues(output string) []string {
	var hosts []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if !strings.HasPrefix(fields[0], "MRU") {
			continue
		}
		// fields[1] is the type (REG_SZ); the value is everything after it
		// (joined back together in case the hostname itself had spaces, which
		// is rare but cheap to handle safely).
		value := strings.Join(fields[2:], " ")
		value = strings.TrimSpace(value)
		if value != "" {
			hosts = append(hosts, value)
		}
	}

	return hosts
}

// parseRegSubkeys parses `reg query <key>` output (without /s) which lists the
// immediate subkeys as full paths, one per line, e.g.:
//
//	HKEY_CURRENT_USER\Software\Microsoft\Terminal Server Client\Servers\192.168.1.100
func parseRegSubkeys(output string, parentKey string) []string {
	var hosts []string
	lines := strings.Split(output, "\n")
	prefix := parentKey + `\`

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		host := strings.TrimPrefix(line, prefix)
		host = strings.TrimSpace(host)
		if host != "" {
			hosts = append(hosts, host)
		}
	}

	return hosts
}

// GetSystemHostsWithDB merges system (registry) hosts with hosts saved in our
// own database, deduplicated.
func GetSystemHostsWithDB(db *Database) ([]string, error) {
	systemHosts, _ := GetSystemHosts() // best-effort; ignore errors
	dbHosts, err := db.GetAllHosts()
	if err != nil {
		return systemHosts, nil
	}

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

// GetSystemHostsOnly returns only hosts found in the Windows registry (not in our DB),
// used to show the user what mstsc.exe itself remembers, separately from saved profiles.
func GetSystemHostsOnly() []string {
	hosts, _ := GetSystemHosts()
	return hosts
}
