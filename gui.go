package main

import (
	"sort"
	"syscall"
	"unsafe"
)

// ==================== Win32 API bindings ====================

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

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
	procGetDlgItemTextW     = user32.NewProc("GetDlgItemTextW")
	procSetDlgItemTextW     = user32.NewProc("SetDlgItemTextW")
	procCheckDlgButton      = user32.NewProc("CheckDlgButton")
	procIsDlgButtonChecked  = user32.NewProc("IsDlgButtonChecked")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
)

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
	ES_PASSWORD      = 0x0020
	BS_AUTOCHECKBOX  = 0x0003

	SW_SHOW = 5

	WM_CREATE  = 0x0001
	WM_DESTROY = 0x0002
	WM_CLOSE   = 0x0010
	WM_COMMAND = 0x0111

	CB_GETLBTEXT    = 0x0148
	CB_ADDSTRING    = 0x0143
	CB_RESETCONTENT = 0x014B
	CB_GETCURSEL    = 0x0147
	CB_SETCURSEL    = 0x014E

	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_GETCURSEL    = 0x0188
	LB_GETTEXT      = 0x0189
	LB_GETCOUNT     = 0x018B
	LBS_NOTIFY      = 0x0001
	LBN_DBLCLK      = 2

	CBN_SELCHANGE = 5
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
)

// Control IDs - Edit window
const (
	ID_E_HOST      = 201
	ID_E_PORT      = 202
	ID_E_USER      = 203
	ID_E_PASS      = 204
	ID_E_RES       = 205
	ID_E_CLIPBOARD = 206
	ID_E_DISKS     = 207
	ID_E_PROXYMODE = 208
	ID_E_PROXYADDR = 209
	ID_E_SAVE      = 210
	ID_E_CANCEL    = 211
	ID_E_DELETE    = 212
)

// Control IDs - Settings window
const (
	ID_S_PROXYMODE = 301
	ID_S_PROXYADDR = 302
	ID_S_SAVE      = 303
	ID_S_CANCEL    = 304
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
	historyEntries  []string // parallel to listbox rows; "" for section headers (non-selectable in practice)
	historySelHosts []string // host to jump to, "" if the row is just a header/info line
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
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("STATIC"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, 0, hInstance, 0,
	)
}

func createEdit(parent uintptr, id int, text string, x, y, w, h int32, password bool) {
	style := uintptr(WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP)
	if password {
		style |= ES_PASSWORD
	}
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("EDIT"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
}

func createCombo(parent uintptr, id int, x, y, w, h int32) {
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("COMBOBOX"))),
		0,
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
}

func createButton(parent uintptr, id int, text string, x, y, w, h int32) {
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
}

func createCheckbox(parent uintptr, id int, text string, x, y, w, h int32) {
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16ptr(text))),
		uintptr(WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_AUTOCHECKBOX),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
}

func createListBox(parent uintptr, id int, x, y, w, h int32) {
	procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16ptr("LISTBOX"))),
		0,
		uintptr(WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|WS_VSCROLL|LBS_NOTIFY),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
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
	db = database
	rdpExecutor = executor

	hMod, _, _ := procGetModuleHandleW.Call(0)
	hInstance = hMod

	mainProc := syscall.NewCallback(mainWndProc)
	editProc := syscall.NewCallback(editWndProc)
	settingsProc := syscall.NewCallback(settingsWndProc)
	historyProc := syscall.NewCallback(historyWndProc)

	registerClass("RDPMainWindowClass", mainProc)
	registerClass("RDPEditWindowClass", editProc)
	registerClass("RDPSettingsWindowClass", settingsProc)
	registerClass("RDPHistoryWindowClass", historyProc)

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

	runMessageLoop()
	return nil
}

func registerClass(name string, proc uintptr) {
	curCursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW

	var wc WNDCLASSEXW
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = proc
	wc.hInstance = hInstance
	wc.hCursor = curCursor
	wc.hbrBackground = 6 // COLOR_WINDOW + 1
	wc.lpszClassName = utf16ptr(name)

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
		createCombo(hwnd, ID_HOST_COMBO, 100, 18, 340, 200)

		createLabel(hwnd, "Username:", 15, 55, 80, 20)
		createCombo(hwnd, ID_USER_COMBO, 100, 53, 340, 200)

		createButton(hwnd, ID_CONNECT, "Connect", 15, 95, 100, 30)
		createButton(hwnd, ID_NEW, "New Profile", 125, 95, 100, 30)
		createButton(hwnd, ID_EDIT, "Edit Profile", 235, 95, 100, 30)
		createButton(hwnd, ID_DELETE, "Delete", 345, 95, 95, 30)

		createButton(hwnd, ID_SETTINGS, "Global Settings", 15, 135, 165, 30)
		createButton(hwnd, ID_HISTORY, "Connection History", 190, 135, 165, 30)

		createLabel(hwnd, "Tip: double-click a host in the list to select it, then pick a username.", 15, 180, 440, 40)

		refreshHostCombo(hwnd)
		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if id == ID_HOST_COMBO && code == CBN_SELCHANGE {
			onHostChanged(hwnd)
		} else if code == BN_CLICKED {
			switch id {
			case ID_CONNECT:
				onConnectClicked(hwnd)
			case ID_NEW:
				openEditWindow(hwnd, nil)
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
	if len(hostList) > 0 {
		comboSetSel(hwnd, ID_HOST_COMBO, 0)
		onHostChanged(hwnd)
	} else {
		comboReset(hwnd, ID_USER_COMBO)
		currentProfiles = nil
	}
}

func onHostChanged(hwnd uintptr) {
	idx := comboGetSel(hwnd, ID_HOST_COMBO)
	comboReset(hwnd, ID_USER_COMBO)
	currentProfiles = nil

	if idx < 0 || idx >= len(hostList) {
		return
	}

	host := hostList[idx]
	profiles, _ := db.GetProfilesByHost(host)
	currentProfiles = profiles

	for _, p := range profiles {
		comboAddString(hwnd, ID_USER_COMBO, p.Username)
	}
	if len(profiles) > 0 {
		comboSetSel(hwnd, ID_USER_COMBO, 0)
	}
}

func getSelectedProfile(hwnd uintptr) *RDPProfile {
	uidx := comboGetSel(hwnd, ID_USER_COMBO)
	if uidx < 0 || uidx >= len(currentProfiles) {
		return nil
	}
	p := currentProfiles[uidx]
	return &p
}

func onConnectClicked(hwnd uintptr) {
	profile := getSelectedProfile(hwnd)
	if profile == nil {
		msgBox("Please select a host and username first.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	if err := rdpExecutor.ExecuteRDP(profile); err != nil {
		msgBox("Connection failed: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}
}

func onEditClicked(hwnd uintptr) {
	profile := getSelectedProfile(hwnd)
	if profile == nil {
		msgBox("Please select a profile to edit.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}
	openEditWindow(hwnd, profile)
}

func onDeleteClicked(hwnd uintptr) {
	profile := getSelectedProfile(hwnd)
	if profile == nil {
		msgBox("Please select a profile to delete.", "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	if err := db.DeleteProfile(profile.Host, profile.Username); err != nil {
		msgBox("Error deleting profile: "+err.Error(), "RDP+ Extended", MB_OK|MB_ICONERROR)
		return
	}

	refreshHostCombo(hwnd)
}

// ==================== Edit / New profile window ====================

func openEditWindow(owner uintptr, profile *RDPProfile) {
	if profile == nil {
		editingProfile = &RDPProfile{
			Port:             3389,
			Resolution:       "1920x1080",
			ClipboardEnabled: true,
			DisksEnabled:     true,
			DisksRedirect:    "all",
			DisksDynamic:     true,
			ProxyMode:        "direct",
		}
		isEditingExisting = false
	} else {
		p := *profile
		editingProfile = &p
		isEditingExisting = true
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
		250, 150, 420, 500,
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
		createCombo(hwnd, ID_E_RES, 130, y-2, 250, 200)
		for _, r := range GetResolutionOptions() {
			comboAddString(hwnd, ID_E_RES, r)
		}
		y += 32

		createCheckbox(hwnd, ID_E_CLIPBOARD, "Enable clipboard redirection", 15, y, 300, 22)
		y += 28
		createCheckbox(hwnd, ID_E_DISKS, "Enable disk redirection (drives)", 15, y, 300, 22)
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

		// Populate values
		selectComboByText(hwnd, ID_E_RES, editingProfile.Resolution)
		selectComboByText(hwnd, ID_E_PROXYMODE, editingProfile.ProxyMode)
		setChecked(hwnd, ID_E_CLIPBOARD, editingProfile.ClipboardEnabled)
		setChecked(hwnd, ID_E_DISKS, editingProfile.DisksEnabled)

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
	editingProfile.Resolution = getDlgText(hwnd, ID_E_RES)
	editingProfile.ProxyMode = getDlgText(hwnd, ID_E_PROXYMODE)
	editingProfile.ProxyAddress = getDlgText(hwnd, ID_E_PROXYADDR)
	editingProfile.ClipboardEnabled = isChecked(hwnd, ID_E_CLIPBOARD)
	editingProfile.DisksEnabled = isChecked(hwnd, ID_E_DISKS)
	if editingProfile.DisksEnabled {
		editingProfile.DisksRedirect = "all"
	} else {
		editingProfile.DisksRedirect = "none"
	}

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
		300, 250, 380, 240,
		owner, 0, hInstance, 0,
	)

	procShowWindow.Call(settingsHwnd, SW_SHOW)
	procUpdateWindow.Call(settingsHwnd)
}

func settingsWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		settings, _ := db.GetGlobalSettings()

		createLabel(hwnd, "SOCKS5 proxy:", 15, 20, 120, 20)
		createCombo(hwnd, ID_S_PROXYMODE, 140, 18, 150, 200)
		comboAddString(hwnd, ID_S_PROXYMODE, "disabled")
		comboAddString(hwnd, ID_S_PROXYMODE, "enabled")
		selectComboByText(hwnd, ID_S_PROXYMODE, settings.GlobalProxyMode)

		createLabel(hwnd, "Address (host:port):", 15, 60, 200, 20)
		createEdit(hwnd, ID_S_PROXYADDR, settings.GlobalProxyAddress, 15, 85, 275, 22, false)

		createButton(hwnd, ID_S_SAVE, "Save", 40, 140, 100, 32)
		createButton(hwnd, ID_S_CANCEL, "Cancel", 160, 140, 100, 32)
		return 0

	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))

		if code == BN_CLICKED {
			switch id {
			case ID_S_SAVE:
				mode := getDlgText(hwnd, ID_S_PROXYMODE)
				addr := getDlgText(hwnd, ID_S_PROXYADDR)
				db.SaveGlobalSettings(&GlobalSettings{
					GlobalProxyMode:    mode,
					GlobalProxyAddress: addr,
				})
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
		populateHistoryList(hwnd)
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

// populateHistoryList fills the list with three sections:
//  1. Hosts saved in our own profile database
//  2. Hosts found in the Windows/mstsc.exe registry history (not already in our DB)
//  3. Recent connections made through this program (with timestamp)
//
// historySelHosts is kept parallel to the listbox rows: a non-empty value means
// double-clicking that row should jump to that host in the main window.
func populateHistoryList(hwnd uintptr) {
	listReset(hwnd, ID_H_LIST)
	historySelHosts = nil

	addRow := func(text, host string) {
		listAddString(hwnd, ID_H_LIST, text)
		historySelHosts = append(historySelHosts, host)
	}

	dbHosts, _ := db.GetAllHosts()
	sysHosts, _ := GetSystemHosts()

	dbHostSet := make(map[string]bool)
	for _, h := range dbHosts {
		dbHostSet[h] = true
	}

	addRow("=== Saved Profiles ===", "")
	if len(dbHosts) == 0 {
		addRow("  (none saved yet)", "")
	}
	for _, h := range dbHosts {
		addRow("  "+h, h)
	}

	addRow("", "")
	addRow("=== Windows RDP History (mstsc.exe) ===", "")
	newSysHosts := 0
	for _, h := range sysHosts {
		if dbHostSet[h] {
			continue // already listed above
		}
		addRow("  "+h, h)
		newSysHosts++
	}
	if newSysHosts == 0 {
		addRow("  (none found, or none new)", "")
	}

	addRow("", "")
	addRow("=== Recent Connections (this app) ===", "")
	history := db.GetHistory()
	if len(history) == 0 {
		addRow("  (no connections recorded yet)", "")
	} else {
		// show most recent first
		for i := len(history) - 1; i >= 0; i-- {
			e := history[i]
			addRow("  "+e.ConnectedAt+"  "+e.Username+"@"+e.Host, e.Host)
		}
	}
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

	selectHostInMainWindow(host)
	procDestroyWindow.Call(hwnd)
}

// selectHostInMainWindow makes sure host is present in the main window's host
// combo, selects it, and refreshes the username list for it.
func selectHostInMainWindow(host string) {
	found := false
	for _, h := range hostList {
		if h == host {
			found = true
			break
		}
	}
	if !found {
		hostList = append(hostList, host)
		sort.Strings(hostList)
		comboReset(mainHwnd, ID_HOST_COMBO)
		for _, h := range hostList {
			comboAddString(mainHwnd, ID_HOST_COMBO, h)
		}
	}

	selectComboByText(mainHwnd, ID_HOST_COMBO, host)
	onHostChanged(mainHwnd)
}

// ==================== Small helpers ====================

func selectComboByText(hwnd uintptr, id int, text string) {
	for i := 0; i < 20; i++ {
		buf := make([]uint16, 256)
		ret, _, _ := procSendDlgItemMessageW.Call(hwnd, uintptr(id), CB_GETLBTEXT, uintptr(i), uintptr(unsafe.Pointer(&buf[0])))
		if int32(ret) < 0 {
			break
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
