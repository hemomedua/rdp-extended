package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// ==================== Win32 API bindings ====================

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procSendDlgItemMessageW = user32.NewProc("SendDlgItemMessageW")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procGetDlgItemTextW     = user32.NewProc("GetDlgItemTextW")
	procSetDlgItemTextW     = user32.NewProc("SetDlgItemTextW")
	procCheckDlgButton      = user32.NewProc("CheckDlgButton")
	procIsDlgButtonChecked  = user32.NewProc("IsDlgButtonChecked")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procGetStockObject      = gdi32.NewProc("GetStockObject")

	// appFont is set once at startup (see CreateMainWindow) and applied to
	// every control we create. Without this, controls default to the
	// legacy "System" bitmap font (GDI stock SYSTEM_FONT), which does NOT
	// contain Cyrillic (or most non-Latin) glyphs - text in those scripts
	// silently fails to render even though the underlying UTF-16 string is
	// correct. DEFAULT_GUI_FONT is the actual OS-configured UI font (the
	// same one every native Windows dialog uses), which properly covers
	// the user's configured language/locale.
	appFont uintptr
)

const (
	WM_SETFONT       = 0x0030
	DEFAULT_GUI_FONT = 17
)

func applyFont(hwndControl uintptr) {
	if hwndControl == 0 || appFont == 0 {
		return
	}
	procSendMessageW.Call(hwndControl, WM_SETFONT, appFont, 1)
}

type WNDCLASSEXW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

const (
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_TABSTOP          = 0x00010000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000

	CBS_DROPDOWNLIST = 0x0003
	CBS_DROPDOWN     = 0x0002
	ES_PASSWORD      = 0x0020
	BS_AUTOCHECKBOX  = 0x0003

	SW_SHOW = 5

	WM_CREATE  = 0x0001
	WM_DESTROY = 0x0002
	WM_CLOSE   = 0x0010
	WM_COMMAND = 0x0111

	WM_APP           = 0x8000
	WM_HISTORY_READY = WM_APP + 1

	CB_GETLBTEXT    = 0x0148
	CB_ADDSTRING    = 0x0143
	CB_RESETCONTENT = 0x014B
	CB_GETCURSEL    = 0x0147
	CB_SETCURSEL    = 0x014E
	CB_GETCOUNT     = 0x0146

	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_GETCURSEL    = 0x0188
	LB_GETTEXT      = 0x0189
	LB_GETCOUNT     = 0x018B
	LBS_NOTIFY      = 0x0001
	LBN_DBLCLK      = 2

	// NOTE: CBN_SELCHANGE is 1, NOT 5 (5 is CBN_EDITCHANGE - a different
	// notification, sent when the user types in an editable combo's text
	// box). Using the wrong value here meant the host-selection handler
	// was effectively never invoked when picking an item from the list.
	CBN_SELCHANGE = 1
	CBN_EDITCHANGE = 5 // sent while typing/pasting in an editable combo's text box
	BN_CLICKED    = 0

	MB_OK        = 0x00000000
	MB_ICONERROR = 0x00000010
	MB_ICONINFO  = 0x00000040

	BST_CHECKED = 1
)

// Control IDs - Main window
const (
	ID_HOST_COMBO = 101
	ID_USER_COMBO = 102
	ID_CONNECT    = 103
	ID_NEW        = 104
	ID_EDIT       = 105
	ID_DELETE     = 106
	ID_SETTINGS   = 107
	ID_HISTORY    = 109
	ID_PWD_STATUS = 110
)

// Control IDs - Edit window
const (
	ID_E_HOST       = 201
	ID_E_PORT       = 202
	ID_E_USER       = 203
	ID_E_PASS       = 204
	ID_E_RES        = 205
	ID_E_CLIPBOARD  = 206
	ID_E_DISKS      = 207 // now a 4-choice dropdownlist, not a checkbox
	ID_E_PROXYMODE  = 208
	ID_E_PROXYADDR  = 209
	ID_E_SAVE       = 210
	ID_E_CANCEL     = 211
	ID_E_DELETE     = 212
	ID_E_COLORDEPTH = 213
)

// Control IDs - Settings window
const (
	ID_S_PROXYMODE  = 301
	ID_S_PROXYADDR  = 302
	ID_S_SAVE       = 303
	ID_S_CANCEL     = 304
	ID_S_RES        = 305
	ID_S_COLORDEPTH = 306
	ID_S_CLIPBOARD  = 307
	ID_S_DISKS      = 308
)

// Control IDs - History window
const (
	ID_H_LIST  = 401
	ID_H_CLOSE = 402
)

// ==================== Globals ====================

var (
	db          *Database
	rdpExecutor *RDPExecutor

	hInstance    uintptr
	mainHwnd     uintptr
	editHwnd     uintptr
	settingsHwnd uintptr

	hostList          []string
	currentProfiles   []RDPProfile
	editingProfile    *RDPProfile
	isEditingExisting bool

	historyHwnd     uintptr
	historySelHosts []string // host to jump to, "" if the row is just a header/info line
	historySelUsers []string // username to prefill, "" if none

	historyDataMu    sync.Mutex
	historyDataRows  []string
	historyDataHosts []string
	historyDataUsers []string

	usernameHintsMu sync.Mutex
	usernameHints   map[string]string // host -> last-used username, from registry (best effort)
)

func utf16ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func msgBox(text, title string, flags uint32) {
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16ptr(text))), uintptr(unsafe.Pointer(utf16ptr(title))), uintptr(flags))
}

func comboAddString(hwndParent uintptr, id int, text string) {
	procSendDlgItemMessageW.Call(hwndParent, uintptr(id), CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16ptr(text))))
}

func comboReset(hwndParent uintptr, id int) {
	procSendDlgItemMessageW.Call(hwndParent, uintptr(id), CB_RESETCONTENT, 0, 0)
}

func comboSetSel(hwndParent uintptr, id int, index int) {
	procSendDlgItemMessageW.Call(hwndParent, uintptr(id), CB_SETCURSEL, uintptr(index), 0)
}

func comboGetSel(hwndParent uintptr, id int) int {
	ret, _, _ := procSendDlgItemMessageW.Call(hwndParent, uintptr(id), CB_GETCURSEL, 0, 0)
	return int(int32(ret))
}

func getDlgText(hwndParent uintptr, id int) string {
	buf := make([]uint16, 512)
	procGetDlgItemTextW.Call(hwndParent, uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func setDlgText(hwndParent uintptr, id int, text string) {
	procSetDlgItemTextW.Call(hwndParent, uintptr(id), uintptr(unsafe.Pointer(utf16ptr(text))))
}

func setChecked(hwndParent uintptr, id int, checked bool) {
	val := 0
	if checked {
		val = BST_CHECKED
	}
	procCheckDlgButton.Call(hwndParent, uintptr(id), uintptr(val))
}

func isChecked(hwndParent uintptr, id int) bool {
	ret, _, _ := procIsDlgButtonChecked.Call(hwndParent, uintptr(id))
	return ret == BST_CHECKED
}

func createLabel(parent uintptr, text string, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("STATIC"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, 0, hInstance, 0,
	)
	applyFont(ret)
}

// createLabelID is like createLabel but with a control ID, for labels whose
// text needs to be updated later via setDlgText (e.g. status indicators).
func createLabelID(parent uintptr, id int, text string, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("STATIC"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func createEdit(parent uintptr, id int, text string, x, y, w, h int32, password bool) {
	style := uintptr(WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP)
	if password {
		style |= ES_PASSWORD
	}
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("EDIT"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func createCombo(parent uintptr, id int, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("COMBOBOX"))),
		0,
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

// createComboEditable creates a combo box that both offers a dropdown list
// AND allows the user to type free-form text into it (CBS_DROPDOWN instead
// of CBS_DROPDOWNLIST). Used for Host/Username fields so a host:port or an
// arbitrary username can be typed directly, not just picked from a list.
func createComboEditable(parent uintptr, id int, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("COMBOBOX"))),
		0,
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWN),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func createButton(parent uintptr, id int, text string, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func createCheckbox(parent uintptr, id int, text string, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_AUTOCHECKBOX),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func createListBox(parent uintptr, id int, x, y, w, h int32) {
	ret, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("LISTBOX"))),
		0,
		uintptr(WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|WS_VSCROLL|LBS_NOTIFY),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	applyFont(ret)
}

func listReset(hwndParent uintptr, id int) {
	procSendDlgItemMessageW.Call(hwndParent, uintptr(id), LB_RESETCONTENT, 0, 0)
}

func listAddString(hwndParent uintptr, id int, text string) {
	procSendDlgItemMessageW.Call(hwndParent, uintptr(id), LB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16ptr(text))))
}

func listGetSel(hwndParent uintptr, id int) int {
	ret, _, _ := procSendDlgItemMessageW.Call(hwndParent, uintptr(id), LB_GETCURSEL, 0, 0)
	return int(int32(ret))
}

// ==================== Entry point ====================

// CreateMainWindow is the entry point called from main.go
func CreateMainWindow(database *Database, executor *RDPExecutor) error {
	// CRITICAL: Win32 windows and message queues are affine to the OS thread
	// that created them. Go's scheduler can otherwise migrate this goroutine
	// to a different OS thread after any blocking call, which silently
	// corrupts window/message handling and causes exactly the kind of
	// intermittent, hard-to-reproduce freezes seen when opening child
	// windows. Pinning to one OS thread for the lifetime of the GUI fixes
	// this at the root.
	runtime.LockOSThread()

	db = database
	rdpExecutor = executor

	hMod, _, _ := procGetModuleHandleW.Call(0)
	hInstance = hMod

	appFont, _, _ = procGetStockObject.Call(DEFAULT_GUI_FONT)

	mainProc := syscall.NewCallback(mainWndProc)
	editProc := syscall.NewCallback(editWndProc)
	settingsProc := syscall.NewCallback(settingsWndProc)
	historyProc := syscall.NewCallback(historyWndProc)

	appIcon, _, _ := procLoadIconW.Call(hInstance, 1) // resource ID 1, embedded via rsrc_windows_amd64.syso

	registerClass("RDPMainWindowClass", mainProc, appIcon)
	registerClass("RDPEditWindowClass", editProc, appIcon)
	registerClass("RDPSettingsWindowClass", settingsProc, appIcon)
	registerClass("RDPHistoryWindowClass", historyProc, appIcon)

	hosts, _ := GetSystemHostsWithDB(db)
	sort.Strings(hosts)
	hostList = hosts

	mainHwnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("RDPMainWindowClass"))),
		uintptr(unsafe.Pointer(utf16ptr("RDP+ Extended"))),
		uintptr(WS_OVERLAPPEDWINDOW),
		200, 200, 480, 300,
		0, 0, hInstance, 0,
	)

	procShowWindow.Call(mainHwnd, SW_SHOW)
	procUpdateWindow.Call(mainHwnd)

	go prefetchUsernameHints() // best-effort, off the UI thread; never blocks startup

	runMessageLoop()
	return nil
}

// prefetchUsernameHints loads the registry's per-host "last used username"
// hints in the background and stores them in a mutex-guarded cache, so the
// UI thread never has to shell out to reg.exe synchronously (which is what
// caused freezes previously).
func prefetchUsernameHints() {
	hints := GetServerUsernameHints()
	usernameHintsMu.Lock()
	usernameHints = hints
	usernameHintsMu.Unlock()
}

func getCachedUsernameHint(host string) string {
	usernameHintsMu.Lock()
	defer usernameHintsMu.Unlock()
	if usernameHints == nil {
		return ""
	}
	return usernameHints[host]
}

func registerClass(name string, proc uintptr, icon uintptr) {
	curCursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW

	var wc WNDCLASSEXW
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = proc
	wc.hInstance = hInstance
	wc.hCursor = curCursor
	wc.hbrBackground = 6 // COLOR_WINDOW + 1
	wc.lpszClassName = utf16ptr(name)
	wc.hIcon = icon
	wc.hIconSm = icon

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

func runMessageLoop() {
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// ==================== Main window ====================

func mainWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		createLabel(hwnd, "Host:", 15, 20, 80, 20)
		createComboEditable(hwnd, ID_HOST_COMBO, 100, 18, 340, 200)

		createLabel(hwnd, "Username:", 15, 55, 80, 20)
		createComboEditable(hwnd, ID_USER_COMBO, 100, 53, 340, 200)

		createButton(hwnd, ID_CONNECT, "Connect", 15, 95, 100, 30)
		createButton(hwnd, ID_NEW, "New Profile", 125, 95, 100, 30)
		createButton(hwnd, ID_EDIT, "Edit Profile", 235, 95, 100, 30)
		createButton(hwnd, ID_DELETE, "Delete", 345, 95, 95, 30)

		createButton(hwnd, ID_SETTINGS, "Global Settings", 15, 135, 165, 30)
		createButton(hwnd, ID_HISTORY, "Connection History", 190, 135, 165, 30)

		createLabelID(hwnd, ID_PWD_STATUS, "", 15, 172, 440, 20)
		createLabel(hwnd, "Tip: you can type a host (optionally host:port) directly, or pick one from the list.", 15, 195, 440, 40)

		refreshHostCombo(hwnd)
		if len(hostList) > 0 {
			setDlgText(hwnd, ID_HOST_COMBO, hostList[0])
			onHostChanged(hwnd)
		}
		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if id == ID_HOST_COMBO && (code == CBN_SELCHANGE || code == CBN_EDITCHANGE) {
			onHostChanged(hwnd)
		} else if id == ID_USER_COMBO && (code == CBN_SELCHANGE || code == CBN_EDITCHANGE) {
			updatePasswordStatus(hwnd)
		} else if code == BN_CLICKED {
			switch id {
			case ID_CONNECT:
				onConnectClicked(hwnd)
			case ID_NEW:
				onNewClicked(hwnd)
			case ID_EDIT:
				onEditClicked(hwnd)
			case ID_DELETE:
				onDeleteClicked(hwnd)
			case ID_SETTINGS:
				openSettingsWindow(hwnd)
			case ID_HISTORY:
				openHistoryWindow(hwnd)
			}
		}
		return 0

	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0

	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xFFFF) }

func refreshHostCombo(hwnd uintptr) {
	hosts, _ := GetSystemHostsWithDB(db)
	sort.Strings(hosts)
	hostList = hosts

	comboReset(hwnd, ID_HOST_COMBO)
	for _, h := range hostList {
		comboAddString(hwnd, ID_HOST_COMBO, h)
	}
}

func onHostChanged(hwnd uintptr) {
	raw := strings.TrimSpace(getDlgText(hwnd, ID_HOST_COMBO))
	comboReset(hwnd, ID_USER_COMBO)
	currentProfiles = nil

	if raw == "" {
		return
	}

	host, _, _ := parseHostPort(raw)

	profiles, _ := db.GetProfilesByHost(host)
	currentProfiles = profiles

	for _, p := range profiles {
		comboAddString(hwnd, ID_USER_COMBO, p.Username)
	}

	if len(profiles) > 0 {
		comboSetSel(hwnd, ID_USER_COMBO, 0)
		updatePasswordStatus(hwnd)
		return
	}

	// No saved profile for this host - offer the registry's last-used
	// username for it as a convenience, if we know one.
	if hint := getCachedUsernameHint(host); hint != "" {
		setDlgText(hwnd, ID_USER_COMBO, hint)
	}
	updatePasswordStatus(hwnd)
}

// updatePasswordStatus shows whether the currently typed/selected
// host+username has a saved password on file - mirroring how the system
// RDP client indicates a stored credential without ever revealing it.
func updatePasswordStatus(hwnd uintptr) {
	username := strings.TrimSpace(getDlgText(hwnd, ID_USER_COMBO))
	status := ""

	if username != "" {
		for _, p := range currentProfiles {
			if p.Username == username {
				if p.Password != "" {
					status = "🔒 Saved password on file for this account"
				} else {
					status = "No saved password - you will be prompted"
				}
				break
			}
		}
	}

	setDlgText(hwnd, ID_PWD_STATUS, status)
}

// parseHostPort splits "host" or "host:port" into its parts. hasPort is
// false (and port is 0) when no valid numeric port suffix is present, in
// which case host is returned unchanged (e.g. "2001:db8::1" style inputs
// are left alone rather than misparsed - we only treat the suffix after the
// LAST colon as a port if it's a plausible port number).
func parseHostPort(input string) (host string, port int, hasPort bool) {
	input = strings.TrimSpace(input)
	idx := strings.LastIndex(input, ":")
	if idx <= 0 || idx == len(input)-1 {
		return input, 0, false
	}

	portStr := input[idx+1:]
	p, ok := atoi(portStr)
	if !ok || p <= 0 || p > 65535 {
		return input, 0, false
	}

	return input[:idx], p, true
}

// buildConnectionProfile reads whatever is currently in the Host/Username
// fields (typed or picked from the dropdown) and produces a profile to
// connect with: an exact saved profile if host+username match one, or a
// sensible quick-connect profile (no stored password - mstsc will prompt).
func buildConnectionProfile(hwnd uintptr) (*RDPProfile, error) {
	rawHost := strings.TrimSpace(getDlgText(hwnd, ID_HOST_COMBO))
	if rawHost == "" {
		return nil, fmt.Errorf("please enter a host to connect to")
	}

	host, port, hasPort := parseHostPort(rawHost)
	username := strings.TrimSpace(getDlgText(hwnd, ID_USER_COMBO))

	if username != "" {
		for _, p := range currentProfiles {
			if p.Host == host && p.Username == username {
				pc := p
				if hasPort {
					pc.Port = port
				}
				return &pc, nil
			}
		}
	}

	profile := &RDPProfile{
		Host:      host,
		Port:      3389,
		Username:  username,
		ProxyMode: "direct",
		// Resolution/ColorDepth/ClipboardMode/DisksRedirect left empty:
		// a quick-connect (unsaved) profile just inherits whatever is
		// configured in Global Settings.
	}
	if hasPort {
		profile.Port = port
	}
	return profile, nil
}

func onConnectClicked(hwnd uintptr) {
	profile, err := buildConnectionProfile(hwnd)
	if err != nil {
		msgBox(err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	if err := rdpExecutor.ExecuteRDP(profile); err != nil {
		msgBox("Connection failed: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}
}

// ==================== Tri-state setting label mapping ====================
//
// ClipboardMode/DisksRedirect are stored as plain machine-readable strings
// ("on"/"off", "none"/"all"/"dynamic"), but the dropdowns show human labels.
// "" (empty) always means "inherit the global default" and only appears as
// a choice in per-profile dropdowns, not in Global Settings itself.

const inheritLabel = "Inherit (global default)"

func clipboardModeToLabel(mode string) string {
	switch mode {
	case "on":
		return "Enabled"
	case "off":
		return "Disabled"
	default:
		return inheritLabel
	}
}

func labelToClipboardMode(label string) string {
	switch label {
	case "Enabled":
		return "on"
	case "Disabled":
		return "off"
	default:
		return ""
	}
}

func disksRedirectToLabel(v string) string {
	switch v {
	case "none":
		return "None"
	case "all":
		return "All drives"
	case "dynamic":
		return "Dynamic (added later) only"
	default:
		return inheritLabel
	}
}

func labelToDisksRedirect(label string) string {
	switch label {
	case "None":
		return "none"
	case "All drives":
		return "all"
	case "Dynamic (added later) only":
		return "dynamic"
	default:
		return ""
	}
}

func onNewClicked(hwnd uintptr) {
	prefill := &RDPProfile{
		Port:      3389,
		ProxyMode: "direct",
	}

	rawHost := strings.TrimSpace(getDlgText(hwnd, ID_HOST_COMBO))
	if rawHost != "" {
		host, port, hasPort := parseHostPort(rawHost)
		prefill.Host = host
		if hasPort {
			prefill.Port = port
		}
	}
	prefill.Username = strings.TrimSpace(getDlgText(hwnd, ID_USER_COMBO))

	openEditWindow(hwnd, prefill)
}

func onEditClicked(hwnd uintptr) {
	rawHost := strings.TrimSpace(getDlgText(hwnd, ID_HOST_COMBO))
	if rawHost == "" {
		msgBox("Please enter or select a host first.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	host, port, hasPort := parseHostPort(rawHost)
	username := strings.TrimSpace(getDlgText(hwnd, ID_USER_COMBO))

	prefill := &RDPProfile{
		Host:      host,
		Port:      3389,
		Username:  username,
		ProxyMode: "direct",
	}
	if hasPort {
		prefill.Port = port
	}

	openEditWindow(hwnd, prefill)
}

func onDeleteClicked(hwnd uintptr) {
	rawHost := strings.TrimSpace(getDlgText(hwnd, ID_HOST_COMBO))
	host, _, _ := parseHostPort(rawHost)
	username := strings.TrimSpace(getDlgText(hwnd, ID_USER_COMBO))

	if host == "" || username == "" {
		msgBox("Enter/select both a host and a username to delete.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	existing, _ := db.GetProfile(host, username)
	if existing == nil {
		msgBox("No saved profile matches this host and username.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	if err := db.DeleteProfile(host, username); err != nil {
		msgBox("Error deleting profile: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	refreshHostCombo(hwnd)
	onHostChanged(hwnd)
}

// ==================== Edit / New profile window ====================

func openEditWindow(owner uintptr, prefill *RDPProfile) {
	if prefill == nil {
		prefill = &RDPProfile{
			Port:      3389,
			ProxyMode: "direct",
		}
	}

	// Decide "new vs existing" by what's actually saved, not by what the
	// caller happened to pass in - this way prefilled-but-unsaved data
	// (typed host/user not yet in the DB) correctly opens as "New Profile".
	if existing, _ := db.GetProfile(prefill.Host, prefill.Username); existing != nil {
		p := *existing
		editingProfile = &p
		isEditingExisting = true
	} else {
		p := *prefill
		editingProfile = &p
		isEditingExisting = false
	}

	procEnableWindow.Call(owner, 0)

	title := "New Profile"
	if isEditingExisting {
		title = "Edit Profile"
	}

	style := uintptr(WS_OVERLAPPEDWINDOW)
	style &^= uintptr(WS_MAXIMIZEBOX)
	style &^= uintptr(WS_THICKFRAME)

	editHwnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("RDPEditWindowClass"))),
		uintptr(unsafe.Pointer(utf16ptr(title))),
		style,
		250, 150, 420, 540,
		owner, 0, hInstance, 0,
	)

	procShowWindow.Call(editHwnd, SW_SHOW)
	procUpdateWindow.Call(editHwnd)
}

func editWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		y := int32(15)
		createLabel(hwnd, "Host:", 15, y, 100, 20)
		createEdit(hwnd, ID_E_HOST, editingProfile.Host, 130, y, 250, 22, false)
		y += 32

		createLabel(hwnd, "Port:", 15, y, 100, 20)
		createEdit(hwnd, ID_E_PORT, itoa(editingProfile.Port), 130, y, 100, 22, false)
		y += 32

		createLabel(hwnd, "Username:", 15, y, 100, 20)
		createEdit(hwnd, ID_E_USER, editingProfile.Username, 130, y, 250, 22, false)
		y += 32

		createLabel(hwnd, "Password:", 15, y, 100, 20)
		createEdit(hwnd, ID_E_PASS, editingProfile.Password, 130, y, 250, 22, true)
		y += 40

		createLabel(hwnd, "Resolution:", 15, y, 100, 20)
		createComboEditable(hwnd, ID_E_RES, 130, y-2, 250, 200)
		comboAddString(hwnd, ID_E_RES, "") // inherit = blank
		for _, r := range GetResolutionOptions() {
			comboAddString(hwnd, ID_E_RES, r)
		}
		setDlgText(hwnd, ID_E_RES, editingProfile.Resolution)
		y += 32

		createLabel(hwnd, "Color depth:", 15, y, 100, 20)
		createComboEditable(hwnd, ID_E_COLORDEPTH, 130, y-2, 150, 200)
		comboAddString(hwnd, ID_E_COLORDEPTH, "") // inherit = blank
		for _, c := range GetColorDepthOptions() {
			comboAddString(hwnd, ID_E_COLORDEPTH, c)
		}
		setDlgText(hwnd, ID_E_COLORDEPTH, editingProfile.ColorDepth)
		y += 32

		createLabel(hwnd, "Clipboard:", 15, y, 100, 20)
		createCombo(hwnd, ID_E_CLIPBOARD, 130, y-2, 200, 200)
		for _, label := range []string{inheritLabel, "Enabled", "Disabled"} {
			comboAddString(hwnd, ID_E_CLIPBOARD, label)
		}
		selectComboByText(hwnd, ID_E_CLIPBOARD, clipboardModeToLabel(editingProfile.ClipboardMode))
		y += 32

		createLabel(hwnd, "Disk redirect:", 15, y, 100, 20)
		createCombo(hwnd, ID_E_DISKS, 130, y-2, 250, 200)
		for _, label := range []string{inheritLabel, "None", "All drives", "Dynamic (added later) only"} {
			comboAddString(hwnd, ID_E_DISKS, label)
		}
		selectComboByText(hwnd, ID_E_DISKS, disksRedirectToLabel(editingProfile.DisksRedirect))
		y += 40

		createLabel(hwnd, "Proxy mode:", 15, y, 100, 20)
		createCombo(hwnd, ID_E_PROXYMODE, 130, y-2, 150, 200)
		for _, m := range []string{"direct", "global", "custom"} {
			comboAddString(hwnd, ID_E_PROXYMODE, m)
		}
		y += 32

		createLabel(hwnd, "Proxy addr:", 15, y, 100, 20)
		createEdit(hwnd, ID_E_PROXYADDR, editingProfile.ProxyAddress, 130, y, 250, 22, false)
		y += 45

		createButton(hwnd, ID_E_SAVE, "Save", 40, y, 100, 32)
		createButton(hwnd, ID_E_DELETE, "Delete", 150, y, 100, 32)
		createButton(hwnd, ID_E_CANCEL, "Cancel", 260, y, 100, 32)

		selectComboByText(hwnd, ID_E_PROXYMODE, editingProfile.ProxyMode)

		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if code == BN_CLICKED {
			switch id {
			case ID_E_SAVE:
				saveEditWindow(hwnd)
			case ID_E_DELETE:
				deleteFromEditWindow(hwnd)
			case ID_E_CANCEL:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0

	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0

	case WM_DESTROY:
		procEnableWindow.Call(mainHwnd, 1)
		procSetForegroundWindow.Call(mainHwnd)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

func saveEditWindow(hwnd uintptr) {
	editingProfile.Host = getDlgText(hwnd, ID_E_HOST)
	editingProfile.Username = getDlgText(hwnd, ID_E_USER)
	editingProfile.Password = getDlgText(hwnd, ID_E_PASS)
	editingProfile.Resolution = strings.TrimSpace(getDlgText(hwnd, ID_E_RES))
	editingProfile.ColorDepth = strings.TrimSpace(getDlgText(hwnd, ID_E_COLORDEPTH))
	editingProfile.ClipboardMode = labelToClipboardMode(getDlgText(hwnd, ID_E_CLIPBOARD))
	editingProfile.DisksRedirect = labelToDisksRedirect(getDlgText(hwnd, ID_E_DISKS))
	editingProfile.ProxyMode = getDlgText(hwnd, ID_E_PROXYMODE)
	editingProfile.ProxyAddress = getDlgText(hwnd, ID_E_PROXYADDR)

	portStr := getDlgText(hwnd, ID_E_PORT)
	if p, ok := atoi(portStr); ok && p > 0 {
		editingProfile.Port = p
	} else {
		editingProfile.Port = 3389
	}

	if editingProfile.Host == "" || editingProfile.Username == "" {
		msgBox("Host and Username are required.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	if err := db.AddProfile(editingProfile); err != nil {
		msgBox("Error saving profile: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	procDestroyWindow.Call(hwnd)
	refreshHostCombo(mainHwnd)
	jumpToHostInMainWindow(editingProfile.Host, editingProfile.Username)
}

func deleteFromEditWindow(hwnd uintptr) {
	if !isEditingExisting {
		procDestroyWindow.Call(hwnd)
		return
	}

	if err := db.DeleteProfile(editingProfile.Host, editingProfile.Username); err != nil {
		msgBox("Error deleting profile: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	procDestroyWindow.Call(hwnd)
	refreshHostCombo(mainHwnd)
	onHostChanged(mainHwnd)
}

// ==================== Settings window ====================

func openSettingsWindow(owner uintptr) {
	procEnableWindow.Call(owner, 0)

	style := uintptr(WS_OVERLAPPEDWINDOW)
	style &^= uintptr(WS_MAXIMIZEBOX)
	style &^= uintptr(WS_THICKFRAME)

	settingsHwnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("RDPSettingsWindowClass"))),
		uintptr(unsafe.Pointer(utf16ptr("Global Settings"))),
		style,
		300, 250, 400, 400,
		owner, 0, hInstance, 0,
	)

	procShowWindow.Call(settingsHwnd, SW_SHOW)
	procUpdateWindow.Call(settingsHwnd)
}

func settingsWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		settings, _ := db.GetGlobalSettings()
		y := int32(15)

		createLabel(hwnd, "SOCKS5 proxy:", 15, y, 150, 20)
		createCombo(hwnd, ID_S_PROXYMODE, 170, y-2, 150, 200)
		comboAddString(hwnd, ID_S_PROXYMODE, "disabled")
		comboAddString(hwnd, ID_S_PROXYMODE, "enabled")
		selectComboByText(hwnd, ID_S_PROXYMODE, settings.GlobalProxyMode)
		y += 30

		createLabel(hwnd, "Proxy address (host:port):", 15, y, 200, 20)
		y += 24
		createEdit(hwnd, ID_S_PROXYADDR, settings.GlobalProxyAddress, 15, y, 320, 22, false)
		y += 40

		createLabel(hwnd, "Default resolution:", 15, y, 150, 20)
		createComboEditable(hwnd, ID_S_RES, 170, y-2, 150, 200)
		for _, r := range GetResolutionOptions() {
			comboAddString(hwnd, ID_S_RES, r)
		}
		setDlgText(hwnd, ID_S_RES, settings.DefaultResolution)
		y += 32

		createLabel(hwnd, "Default color depth:", 15, y, 150, 20)
		createComboEditable(hwnd, ID_S_COLORDEPTH, 170, y-2, 150, 200)
		for _, c := range GetColorDepthOptions() {
			comboAddString(hwnd, ID_S_COLORDEPTH, c)
		}
		setDlgText(hwnd, ID_S_COLORDEPTH, settings.DefaultColorDepth)
		y += 32

		createLabel(hwnd, "Default clipboard:", 15, y, 150, 20)
		createCombo(hwnd, ID_S_CLIPBOARD, 170, y-2, 150, 200)
		comboAddString(hwnd, ID_S_CLIPBOARD, "Enabled")
		comboAddString(hwnd, ID_S_CLIPBOARD, "Disabled")
		if settings.DefaultClipboard {
			selectComboByText(hwnd, ID_S_CLIPBOARD, "Enabled")
		} else {
			selectComboByText(hwnd, ID_S_CLIPBOARD, "Disabled")
		}
		y += 32

		createLabel(hwnd, "Default disk redirect:", 15, y, 150, 20)
		createCombo(hwnd, ID_S_DISKS, 170, y-2, 200, 200)
		for _, label := range []string{"None", "All drives", "Dynamic (added later) only"} {
			comboAddString(hwnd, ID_S_DISKS, label)
		}
		selectComboByText(hwnd, ID_S_DISKS, disksRedirectToLabel(settings.DefaultDisksRedirect))
		y += 45

		createButton(hwnd, ID_S_SAVE, "Save", 60, y, 120, 32)
		createButton(hwnd, ID_S_CANCEL, "Cancel", 200, y, 120, 32)
		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if code == BN_CLICKED {
			switch id {
			case ID_S_SAVE:
				newSettings := &GlobalSettings{
					GlobalProxyMode:      getDlgText(hwnd, ID_S_PROXYMODE),
					GlobalProxyAddress:   getDlgText(hwnd, ID_S_PROXYADDR),
					DefaultResolution:    strings.TrimSpace(getDlgText(hwnd, ID_S_RES)),
					DefaultColorDepth:    strings.TrimSpace(getDlgText(hwnd, ID_S_COLORDEPTH)),
					DefaultClipboard:     getDlgText(hwnd, ID_S_CLIPBOARD) == "Enabled",
					DefaultDisksRedirect: labelToDisksRedirect(getDlgText(hwnd, ID_S_DISKS)),
				}
				if newSettings.DefaultResolution == "" {
					newSettings.DefaultResolution = "1920x1080"
				}
				if newSettings.DefaultColorDepth == "" {
					newSettings.DefaultColorDepth = "32"
				}
				if newSettings.DefaultDisksRedirect == "" {
					newSettings.DefaultDisksRedirect = "all"
				}
				db.SaveGlobalSettings(newSettings)
				procDestroyWindow.Call(hwnd)
			case ID_S_CANCEL:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0

	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0

	case WM_DESTROY:
		procEnableWindow.Call(mainHwnd, 1)
		procSetForegroundWindow.Call(mainHwnd)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

// ==================== Connection history window ====================

func openHistoryWindow(owner uintptr) {
	procEnableWindow.Call(owner, 0)

	historyHwnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("RDPHistoryWindowClass"))),
		uintptr(unsafe.Pointer(utf16ptr("Connection History"))),
		uintptr(WS_OVERLAPPEDWINDOW),
		220, 150, 520, 480,
		owner, 0, hInstance, 0,
	)

	procShowWindow.Call(historyHwnd, SW_SHOW)
	procUpdateWindow.Call(historyHwnd)
}

func historyWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		createListBox(hwnd, ID_H_LIST, 15, 15, 470, 370)
		createButton(hwnd, ID_H_CLOSE, "Close", 195, 400, 100, 32)
		listAddString(hwnd, ID_H_LIST, "Loading...")

		// IMPORTANT: reading Windows registry history shells out to reg.exe.
		// Doing that synchronously on the UI thread can freeze the whole app
		// (a GUI-subsystem process spawning a console process can block on
		// console setup that itself needs the message loop pumped). So the
		// actual work happens in a goroutine, and the result is delivered
		// back via a posted window message.
		go loadHistoryDataAsync(hwnd)
		return 0

	case WM_HISTORY_READY:
		historyDataMu.Lock()
		rows := historyDataRows
		hosts := historyDataHosts
		users := historyDataUsers
		historyDataMu.Unlock()

		listReset(hwnd, ID_H_LIST)
		for _, r := range rows {
			listAddString(hwnd, ID_H_LIST, r)
		}
		historySelHosts = hosts
		historySelUsers = users
		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if id == ID_H_LIST && code == LBN_DBLCLK {
			onHistoryDoubleClick(hwnd)
		} else if code == BN_CLICKED && id == ID_H_CLOSE {
			procDestroyWindow.Call(hwnd)
		}
		return 0

	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0

	case WM_DESTROY:
		procEnableWindow.Call(mainHwnd, 1)
		procSetForegroundWindow.Call(mainHwnd)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

// loadHistoryDataAsync runs on a background goroutine (NOT the UI thread) and
// builds the full list of rows to display, including the potentially slow
// registry/reg.exe lookups. It then posts WM_HISTORY_READY to have the UI
// thread pick up the result and populate the listbox.
func loadHistoryDataAsync(hwnd uintptr) {
	rows, hosts, users := buildHistoryRows()

	historyDataMu.Lock()
	historyDataRows = rows
	historyDataHosts = hosts
	historyDataUsers = users
	historyDataMu.Unlock()

	procPostMessageW.Call(hwnd, uintptr(WM_HISTORY_READY), 0, 0)
}

// buildHistoryRows assembles three sections:
//  1. Hosts saved in our own profile database (host + username per row)
//  2. Hosts found in the Windows/mstsc.exe registry history (not already in
//     our DB), with a last-used username hint when the registry has one
//  3. Recent connections made through this program (with timestamp)
//
// Returns the display rows and two parallel slices: the host to jump to,
// and the username to prefill (both "" for header/info rows).
func buildHistoryRows() ([]string, []string, []string) {
	var rows []string
	var hosts []string
	var users []string

	addRow := func(text, host, username string) {
		rows = append(rows, text)
		hosts = append(hosts, host)
		users = append(users, username)
	}

	dbHosts, _ := db.GetAllHosts()
	sysHosts, _ := GetSystemHosts()          // shells out to reg.exe - safe here, off the UI thread
	hints := GetServerUsernameHints()        // one recursive reg.exe query - also safe here
	usernameHintsMu.Lock()
	usernameHints = hints
	usernameHintsMu.Unlock()

	dbHostSet := make(map[string]bool)
	for _, h := range dbHosts {
		dbHostSet[h] = true
	}

	addRow("=== Saved Profiles ===", "", "")
	if len(dbHosts) == 0 {
		addRow("  (none saved yet)", "", "")
	}
	for _, h := range dbHosts {
		profiles, _ := db.GetProfilesByHost(h)
		if len(profiles) == 0 {
			addRow("  "+h, h, "")
			continue
		}
		for _, p := range profiles {
			addRow(fmt.Sprintf("  %s   (user: %s)", h, p.Username), h, p.Username)
		}
	}

	addRow("", "", "")
	addRow("=== Windows RDP History (mstsc.exe) ===", "", "")
	newSysHosts := 0
	for _, h := range sysHosts {
		if dbHostSet[h] {
			continue // already listed above
		}
		bareHost, _, _ := parseHostPort(h)
		hint := hints[bareHost]
		if hint == "" {
			hint = hints[h]
		}
		if hint != "" {
			addRow(fmt.Sprintf("  %s   (user: %s)", h, hint), h, hint)
		} else {
			addRow("  "+h, h, "")
		}
		newSysHosts++
	}
	if newSysHosts == 0 {
		addRow("  (none found, or none new)", "", "")
	}

	addRow("", "", "")
	addRow("=== Recent Connections (this app) ===", "", "")
	history := db.GetHistory()
	if len(history) == 0 {
		addRow("  (no connections recorded yet)", "", "")
	} else {
		// show most recent first
		for i := len(history) - 1; i >= 0; i-- {
			e := history[i]
			addRow("  "+e.ConnectedAt+"  "+e.Username+"@"+e.Host, e.Host, e.Username)
		}
	}

	return rows, hosts, users
}

func onHistoryDoubleClick(hwnd uintptr) {
	idx := listGetSel(hwnd, ID_H_LIST)
	if idx < 0 || idx >= len(historySelHosts) {
		return
	}

	host := historySelHosts[idx]
	if host == "" {
		return // header / info row, not selectable
	}

	username := ""
	if idx < len(historySelUsers) {
		username = historySelUsers[idx]
	}

	jumpToHostInMainWindow(host, username)
	procDestroyWindow.Call(hwnd)
}

// jumpToHostInMainWindow puts host (and, if known, username) directly into
// the main window's fields and refreshes the username list for that host.
// Since the host/username combos are editable, we just set their text
// directly - no need to find-and-select a matching list item.
func jumpToHostInMainWindow(host, username string) {
	setDlgText(mainHwnd, ID_HOST_COMBO, host)
	onHostChanged(mainHwnd)

	if username != "" {
		setDlgText(mainHwnd, ID_USER_COMBO, username)
		updatePasswordStatus(mainHwnd)
	}
}

// ==================== Small helpers ====================

func selectComboByText(hwnd uintptr, id int, text string) {
	countRet, _, _ := procSendDlgItemMessageW.Call(hwnd, uintptr(id), CB_GETCOUNT, 0, 0)
	count := int(int32(countRet))

	for i := 0; i < count; i++ {
		buf := make([]uint16, 256)
		ret, _, _ := procSendDlgItemMessageW.Call(hwnd, uintptr(id), CB_GETLBTEXT, uintptr(i), uintptr(unsafe.Pointer(&buf[0])))
		if int32(ret) < 0 {
			continue
		}
		if syscall.UTF16ToString(buf) == text {
			comboSetSel(hwnd, id, i)
			return
		}
	}
	comboSetSel(hwnd, id, 0)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	neg := false
	i := 0
	if s[0] == '-' {
		neg = true
		i = 1
	}
	n := 0
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
