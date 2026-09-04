package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type RDPExecutor struct {
	db *Database
}

func NewRDPExecutor(db *Database) *RDPExecutor {
	return &RDPExecutor{db: db}
}

// resolvedSettings holds the fully-resolved (no more "inherit"/"") settings
// that will actually be applied to a connection, after merging a profile's
// own overrides with the global defaults.
type resolvedSettings struct {
	Resolution    string // "1920x1080" / "fullscreen" / "fit" / custom "WxH"
	ColorDepth    string // "15"/"16"/"24"/"32"
	Clipboard     bool
	DisksRedirect string // "none"/"all"/"dynamic"
}

func resolveSettings(profile *RDPProfile, global *GlobalSettings) resolvedSettings {
	r := resolvedSettings{
		Resolution:    global.DefaultResolution,
		ColorDepth:    global.DefaultColorDepth,
		Clipboard:     global.DefaultClipboard,
		DisksRedirect: global.DefaultDisksRedirect,
	}

	if profile.Resolution != "" {
		r.Resolution = profile.Resolution
	}
	if profile.ColorDepth != "" {
		r.ColorDepth = profile.ColorDepth
	}
	switch profile.ClipboardMode {
	case "on":
		r.Clipboard = true
	case "off":
		r.Clipboard = false
	}
	if profile.DisksRedirect != "" {
		r.DisksRedirect = profile.DisksRedirect
	}

	return r
}

// buildRDPFileContent produces the text of a .rdp connection file. Using a
// real .rdp file (instead of mstsc.exe command-line flags) is necessary
// because mstsc.exe's actual supported CLI switches are very limited
// (/v, /admin, /f, /w, /h, /public, /multimon, /edit, /migrate) - there is
// NO /u or /p switch on stock Windows, and no CLI way to set color depth,
// clipboard redirection, or drive redirection mode at all. The .rdp file
// format supports all of this properly and is the standard mechanism every
// real RDP manager uses under the hood.
func buildRDPFileContent(profile *RDPProfile, s resolvedSettings) string {
	var b strings.Builder

	fmt.Fprintf(&b, "full address:s:%s:%d\r\n", profile.Host, profile.Port)
	if profile.Username != "" {
		fmt.Fprintf(&b, "username:s:%s\r\n", profile.Username)
	}

	switch s.Resolution {
	case "fullscreen":
		b.WriteString("screen mode id:i:2\r\n")
	case "fit":
		b.WriteString("screen mode id:i:1\r\n")
		b.WriteString("smart sizing:i:1\r\n")
	default:
		w, h, err := ParseResolution(s.Resolution)
		if err != nil {
			w, h = 1920, 1080
		}
		b.WriteString("screen mode id:i:1\r\n")
		fmt.Fprintf(&b, "desktopwidth:i:%d\r\n", w)
		fmt.Fprintf(&b, "desktopheight:i:%d\r\n", h)
	}

	if bpp, err := strconv.Atoi(s.ColorDepth); err == nil && bpp > 0 {
		fmt.Fprintf(&b, "session bpp:i:%d\r\n", bpp)
	}

	if s.Clipboard {
		b.WriteString("redirectclipboard:i:1\r\n")
	} else {
		b.WriteString("redirectclipboard:i:0\r\n")
	}

	switch s.DisksRedirect {
	case "all":
		b.WriteString("redirectdrives:i:1\r\n")
		b.WriteString("drivestoredirect:s:*\r\n")
	case "dynamic":
		// Only drives that get plugged in AFTER the session has started -
		// nothing that's already present at connection time.
		b.WriteString("redirectdrives:i:1\r\n")
		b.WriteString("drivestoredirect:s:DynamicDrives\r\n")
	default: // "none" or unknown
		b.WriteString("redirectdrives:i:0\r\n")
	}

	return b.String()
}

// stageCredential saves host+username+password into Windows Credential
// Manager under "TERMSRV/<host>", which mstsc.exe automatically checks
// before prompting for a password. This is the standard, documented way
// ("Remember me" in mstsc.exe itself works the same way under the hood) to
// achieve saved-password auto-login, since .rdp files cannot carry a plain
// password (only a per-user encrypted blob) and mstsc.exe has no CLI switch
// for it.
//
// NOTE: Windows stores exactly one credential per host this way, not one
// per (host, username) pair. If several profiles are saved for the same
// host, whichever one was connected to most recently is the one Windows
// will offer outside of this app too. Re-staging right before every connect
// (as done here) makes each Connect click use the right credential in
// practice.
func stageCredential(host, username, password string) {
	if username == "" || password == "" {
		return
	}
	// Best-effort: if cmdkey isn't available or fails, mstsc will just
	// prompt for a password instead of silently failing the connection.
	exec.Command("cmdkey", "/generic:TERMSRV/"+host, "/user:"+username, "/pass:"+password).Run()
}

// resolveProxyAddress figures out which SOCKS5 address (if any) applies to
// this connection, per-profile override taking precedence over the global
// one.
func resolveProxyAddress(profile *RDPProfile, global *GlobalSettings) string {
	switch profile.ProxyMode {
	case "custom":
		return profile.ProxyAddress
	case "global":
		if global.GlobalProxyMode == "enabled" {
			return global.GlobalProxyAddress
		}
	}
	return ""
}

// writeRDPFileUTF16 writes content as a Windows .rdp file encoded in
// UTF-16LE with a BOM. This matters as soon as the content contains any
// non-ASCII text (a Cyrillic username, for instance): mstsc.exe's own
// "Save As" produces Unicode .rdp files exactly this way, and a plain
// UTF-8 file without a BOM gets misparsed byte-by-byte against the wrong
// code page, corrupting every non-ASCII character - the same "ромбики с
// вопросом" symptom, just showing up inside mstsc.exe's own window instead
// of ours.
func writeRDPFileUTF16(path, content string) error {
	u16, err := syscall.UTF16FromString(content) // includes a trailing NUL
	if err != nil {
		return err
	}

	buf := make([]byte, 0, 2+2*len(u16))
	buf = append(buf, 0xFF, 0xFE) // UTF-16LE byte-order mark
	for _, c := range u16 {
		if c == 0 {
			break // stop before the implicit trailing NUL
		}
		buf = append(buf, byte(c), byte(c>>8))
	}

	return os.WriteFile(path, buf, 0644)
}

// ExecuteRDP launches mstsc.exe for the given profile, applying global
// defaults for anything the profile doesn't explicitly override.
func (r *RDPExecutor) ExecuteRDP(profile *RDPProfile) error {
	global, err := r.db.GetGlobalSettings()
	if err != nil {
		global = &GlobalSettings{DefaultResolution: "1920x1080", DefaultColorDepth: "32", DefaultClipboard: true, DefaultDisksRedirect: "all"}
	}
	resolved := resolveSettings(profile, global)

	stageCredential(profile.Host, profile.Username, profile.Password)

	// SOCKS5 proxy is stored per-profile/globally but mstsc.exe has no
	// built-in generic SOCKS5 support, so it is not yet actually applied to
	// the connection - tracked as a known gap, see project notes.
	_ = resolveProxyAddress(profile, global)

	rdpFileContent := buildRDPFileContent(profile, resolved)

	tmpFile, err := os.CreateTemp("", "rdpext-*.rdp")
	if err != nil {
		return fmt.Errorf("could not create temporary .rdp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close() // we'll rewrite it properly below

	if err := writeRDPFileUTF16(tmpPath, rdpFileContent); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not write .rdp file: %w", err)
	}

	cmd := exec.Command("mstsc.exe", tmpPath)
	if err := cmd.Start(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// mstsc.exe only needs the file at launch, not for the life of the
	// session - clean it up shortly after so it doesn't linger on disk.
	go func(path string) {
		time.Sleep(15 * time.Second)
		os.Remove(path)
	}(tmpPath)

	r.db.RecordConnection(profile.Host, profile.Username)

	return nil
}

// GetResolutionOptions returns predefined resolution choices for dropdowns.
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

// GetColorDepthOptions returns supported RDP color depths.
func GetColorDepthOptions() []string {
	return []string{"15", "16", "24", "32"}
}

// ParseResolution parses resolution strings like "1920x1080".
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
