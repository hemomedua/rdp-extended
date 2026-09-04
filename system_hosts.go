package main

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const (
	regDefaultKey = `HKEY_CURRENT_USER\Software\Microsoft\Terminal Server Client\Default`
	regServersKey = `HKEY_CURRENT_USER\Software\Microsoft\Terminal Server Client\Servers`

	cpOEMCP = 1 // CP_OEMCP
)

var procMultiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")

// decodeOEMBytes converts console output from reg.exe (and other built-in
// Windows console tools) into a proper Go string. This is NOT optional:
// when a console program's output is captured through a pipe (exactly what
// exec.Command().Output() does), Windows encodes non-ASCII text using the
// OEM code page (e.g. CP866 on a Russian-locale system) - NOT UTF-8. Naively
// doing string(rawBytes) treats those OEM-encoded bytes as if they were
// already UTF-8, which corrupts every non-ASCII character (Cyrillic
// included) into the Unicode replacement character (�) once it flows
// through any UTF-8-expecting API - exactly the "ромбики с вопросом"
// symptom. Converting via MultiByteToWideChar(CP_OEMCP, ...) first fixes
// this at the source.
func decodeOEMBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	n, _, _ := procMultiByteToWideChar.Call(
		uintptr(cpOEMCP), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		0, 0,
	)
	if n == 0 {
		return string(b) // fallback - better than nothing
	}

	buf := make([]uint16, n)
	procMultiByteToWideChar.Call(
		uintptr(cpOEMCP), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&buf[0])), n,
	)

	return syscall.UTF16ToString(buf)
}

// GetSystemHosts retrieves RDP connection history from the Windows registry.
// It reads two locations used by mstsc.exe:
//   - ...\Terminal Server Client\Default          (values MRU0, MRU1, ... = recently typed hosts)
//   - ...\Terminal Server Client\Servers\<host>    (one subkey per host ever connected to)
func GetSystemHosts() ([]string, error) {
	hostMap := make(map[string]bool)

	// 1) Recently typed hosts (MRU list). NOTE: `reg query` only accepts a single
	// key argument to list ALL of its values - it does not take multiple /v flags.
	if out, err := exec.Command("reg", "query", regDefaultKey).Output(); err == nil {
		for _, host := range parseRegMRUValues(decodeOEMBytes(out)) {
			hostMap[host] = true
		}
	}

	// 2) Every host ever connected to is stored as a subkey under "Servers".
	if out, err := exec.Command("reg", "query", regServersKey).Output(); err == nil {
		for _, host := range parseRegSubkeys(decodeOEMBytes(out), regServersKey) {
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

// GetServerUsernameHints reads the "UsernameHint" value Windows stores per
// server under the Servers key (the username mstsc.exe last used to connect).
// Done as a single recursive query instead of one call per host.
func GetServerUsernameHints() map[string]string {
	hints := make(map[string]string)

	out, err := exec.Command("reg", "query", regServersKey, "/s").Output()
	if err != nil {
		return hints
	}

	prefix := regServersKey + `\`
	currentHost := ""

	for _, line := range strings.Split(decodeOEMBytes(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, prefix) {
			currentHost = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			continue
		}

		if currentHost == "" {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && fields[0] == "UsernameHint" {
			hints[currentHost] = strings.Join(fields[2:], " ")
		}
	}

	return hints
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
