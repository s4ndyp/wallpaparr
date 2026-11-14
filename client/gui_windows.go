//go:build windows

package main

/*
Tray-app met instellingenvenster (pure Win32 via syscalls, geen dependencies).

- dubbelklik op de exe (geen argumenten) of `wallpaparr gui` start deze GUI
- het kruisje van het venster verbergt de app naar de systeembalk (WM_CLOSE)
- rechtsklik op het tray-icoon geeft het menu met o.a. Afsluiten
- de scheduler-thread wisselt de wallpaper volgens het interval en herhaalt
  nooit twee keer achter elkaar hetzelfde beeld
*/

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed assets/wallpaparr.ico
var icoData []byte

/* ---------- Win32 basis ---------- */

var (
	// user32 komt uit apply_windows.go (zelfde pakket); hier alleen wat daar niet staat
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	pRegisterClassExW       = user32.NewProc("RegisterClassExW")
	pCreateWindowExW        = user32.NewProc("CreateWindowExW")
	pDefWindowProcW         = user32.NewProc("DefWindowProcW")
	pGetMessageW            = user32.NewProc("GetMessageW")
	pTranslateMessage       = user32.NewProc("TranslateMessage")
	pDispatchMessageW       = user32.NewProc("DispatchMessageW")
	pPostQuitMessage        = user32.NewProc("PostQuitMessage")
	pDestroyWindow          = user32.NewProc("DestroyWindow")
	pShowWindow             = user32.NewProc("ShowWindow")
	pIsWindowVisible        = user32.NewProc("IsWindowVisible")
	pSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	pPostMessageW           = user32.NewProc("PostMessageW")
	pGetCursorPos           = user32.NewProc("GetCursorPos")
	pCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	pAppendMenuW            = user32.NewProc("AppendMenuW")
	pTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	pDestroyMenu            = user32.NewProc("DestroyMenu")
	pShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW") // shell32, niet user32!
	pRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	pLoadIconW              = user32.NewProc("LoadIconW")
	pCreateIconFromRes      = user32.NewProc("CreateIconFromResourceEx")
	pGetWindowTextW         = user32.NewProc("GetWindowTextW")
	pSetWindowTextW         = user32.NewProc("SetWindowTextW")
	pSendMessageW           = user32.NewProc("SendMessageW")
	pGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW") // kernel32, niet user32!
	pMessageBoxW            = user32.NewProc("MessageBoxW")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pCreateMutexW           = kernel32.NewProc("CreateMutexW")
	pDwmSetWindowAttribute  = dwmapi.NewProc("DwmSetWindowAttribute")
	pShellExecuteW          = shell32.NewProc("ShellExecuteW")
)

const (
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsTabStop     = 0x00010000
	wsBorder      = 0x00800000
	wsVScroll     = 0x00200000

	esAutoHScroll = 0x0080
	esNumber      = 0x2000

	bsPushButton   = 0x00000000
	bsAutoCheckBox = 0x00000003
	bmGetCheck     = 0x00F0
	bmSetCheck     = 0x00F1
	bstChecked     = 1

	cbsDropDownList = 0x0003
	cbAddString     = 0x0143
	cbSetCurSel     = 0x014E
	cbGetCurSel     = 0x0147

	ssLeft = 0x00000000

	swHide    = 0
	swShow    = 5
	swRestore = 9

	wmCreate        = 0x0001
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmLButtonUp     = 0x0202
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203
	wmApp           = 0x8000

	mfString    = 0x00000000
	mfPopup     = 0x00000010
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008
	mfDisabled  = 0x00000001

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	nimAdd     = 0
	nimDelete  = 2
	nifMessage = 0x01
	nifInfo    = 0x10
	nifIcon    = 0x02
	nifTip     = 0x04

	idiApplication = 32512
)

var cwUseDefault = int32(-2147483648)
var fontHoogte = int32(-13) // negatief = tekenhoogte i.p.v. celhoogte

// eigen vensterberichten
const (
	trayMsg     = wmApp + 1 // callback van het tray-icoon
	msgStatus   = wmApp + 2 // status bijwerken
	msgShow     = wmApp + 3 // venster naar voren halen
	msgMonitors = wmApp + 4 // de monitorlijst is binnen
)

// control-id's
const (
	idServer = 1001 + iota
	idTag
	idMinutes
	idChkMonitor
	idChkRandom
	idChkHidden
	idStatus
	idPosition
	idPoll
	idWebUI
	idSave
	idApplyNow
	idTest
	idProfielMonitor
	idProfielTags
	idProfielCheck
)

// idRatioBase + 0..6 = de verhouding-checkboxen (1:1 t/m 27:9 + overig)
const idRatioBase = 1100

// maxMonitorRijen: aantal monitorrijen dat het venster toont (elke rij = één monitor)
const maxMonitorRijen = 6

// ratioNamen volgt de web-UI; "overig" vangt alles buiten de standaardgroepen.
var ratioNamen = []string{"1:1", "1:2", "2:3", "16:9", "21:9", "27:9", "overig"}

// menu-id's
const (
	idMSettings = 1001 + iota
	idMNow
	idMNext
	idMTest
	idMMonitor
	idMProfiles
	idMQuit
)

var intervalKeuzes = []struct {
	id      uintptr
	minuten int
}{
	{2200, 15}, {2201, 30}, {2202, 60}, {2203, 180}, {2204, 360}, {2205, 1440}, {2210, 0},
}

var posities = []string{"fill", "fit", "stretch", "center", "span", "tile"}
var positieLabels = []string{"vullen (fill)", "passend (fit)", "uitrekken (stretch)", "gecentreerd", "over monitoren (span)", "tegel"}

type point struct{ X, Y int32 }

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// call roept een Win32-functie aan. Ontbreekt de functie in de DLL (verkeerde
// DLL gekozen), dan loggen we dat en geven we 0 terug in plaats van een panic die
// het hele programma meesleept.
func call(proc *syscall.LazyProc, args ...uintptr) uintptr {
	if err := proc.Find(); err != nil {
		guiLog("Win32-functie niet gevonden: %s (%v) — verkeerde DLL?", proc.Name, err)
		return 0
	}
	r, _, _ := proc.Call(args...)
	return r
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

func putStr(dst []uint16, s string) {
	src, _ := syscall.UTF16FromString(s)
	copy(dst, src)
}

// guiLog schrijft naar %LOCALAPPDATA%\wallpaparr\gui.log — de tray-variant heeft
// geen consolevenster, dus dit is de plek waar fouten terechtkomen.
func guiLog(format string, args ...any) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "wallpaparr")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "gui.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func getText(h uintptr) string {
	var buf [1024]uint16
	call(pGetWindowTextW, h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(syscall.UTF16ToString(buf[:]))
}

func setText(h uintptr, s string) { call(pSetWindowTextW, h, uintptr(unsafe.Pointer(u16(s)))) }

/* ---------- gedeelde toestand ---------- */

type guiCmd struct {
	kind  string // apply | next | interval | save | quit
	cfg   Config
	every time.Duration
}

// monitorStatus is één regel in het venster: wat de server op een monitor zet.
type monitorStatus struct {
	Monitor string
	Toont   string
	Sinds   string
}

type guiState struct {
	mu     sync.Mutex
	hwnd   uintptr
	cfg    Config
	status string
	cmds   chan guiCmd

	device   string          // machinenaam zoals de server die kent
	monitors []string        // monitornamen (door de poller gevuld; COM hoort daar)
	stats    []monitorStatus // wat de server nu per monitor toont

	editServer  uintptr
	editPoll    uintptr
	chkHidden   uintptr
	cmbPosition uintptr
	lblStatus   uintptr

	// één regel per monitor: naam, wat er nu staat en sinds wanneer
	lblMonitor [maxMonitorRijen]uintptr
	lblToont   [maxMonitorRijen]uintptr
	lblSinds   [maxMonitorRijen]uintptr
}

var g = &guiState{cmds: make(chan guiCmd, 8)}

func (s *guiState) setCfg(c Config) { s.mu.Lock(); s.cfg = c; s.mu.Unlock() }
func (s *guiState) getCfg() Config  { s.mu.Lock(); defer s.mu.Unlock(); return s.cfg }

func (s *guiState) setDevice(d string) { s.mu.Lock(); s.device = d; s.mu.Unlock() }
func (s *guiState) getDevice() string  { s.mu.Lock(); defer s.mu.Unlock(); return s.device }

func (s *guiState) setMonitors(m []string) { s.mu.Lock(); s.monitors = m; s.mu.Unlock() }
func (s *guiState) getMonitors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.monitors...)
}

/* ---------- statusregels (wat de server op de monitoren zet) ---------- */

func (s *guiState) setStats(v []monitorStatus) {
	s.mu.Lock()
	s.stats = v
	s.mu.Unlock()
}

// verwerkMonitorLijst tekent per monitor wat de server er nu op zet.
func (s *guiState) verwerkMonitorLijst() {
	s.mu.Lock()
	stats := append([]monitorStatus(nil), s.stats...)
	s.mu.Unlock()

	verberg := func(i int) {
		for _, h := range []uintptr{s.lblMonitor[i], s.lblToont[i], s.lblSinds[i]} {
			call(pShowWindow, h, swHide)
		}
	}
	toon := func(h uintptr, tekst string) {
		setText(h, tekst)
		call(pShowWindow, h, swShow)
	}

	for i := 0; i < maxMonitorRijen; i++ {
		if s.lblMonitor[i] == 0 {
			continue
		}
		if i >= len(stats) {
			if i == 0 {
				toon(s.lblMonitor[0], "nog geen schermen bij de server bekend")
				call(pShowWindow, s.lblToont[0], swHide)
				call(pShowWindow, s.lblSinds[0], swHide)
			} else {
				verberg(i)
			}
			continue
		}
		toon(s.lblMonitor[i], stats[i].Monitor)
		toon(s.lblToont[i], stats[i].Toont)
		toon(s.lblSinds[i], stats[i].Sinds)
	}
}

func (s *guiState) setStatus(m string) {
	s.mu.Lock()
	s.status = m
	hwnd := s.hwnd
	s.mu.Unlock()
	if hwnd != 0 {
		call(pPostMessageW, hwnd, msgStatus, 0, 0)
	}
}

/* ---------- tray ---------- */

var trayHicon uintptr

// taskbarCreatedMsg: door Windows geregistreerd bericht dat de taakbalk opnieuw is
// gestart. Zonder heraanmelding is het icoon daarna definitief weg.
var taskbarCreatedMsg uint32

func loadIcon() uintptr {
	if len(icoData) > 22 {
		off := int(binary.LittleEndian.Uint32(icoData[18:22]))
		size := int(binary.LittleEndian.Uint32(icoData[14:18]))
		if off > 0 && size > 0 && off+size <= len(icoData) {
			h := call(pCreateIconFromRes,
				uintptr(unsafe.Pointer(&icoData[off])), uintptr(size), 1, 0x00030000, 0, 0, 0)
			if h != 0 {
				return h
			}
		}
	}
	return call(pLoadIconW, 0, idiApplication)
}

func trayData(cb func(*notifyIconData)) {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = g.hwnd
	nid.UID = 1
	cb(&nid)
}

func trayAdd(status string) {
	trayHicon = loadIcon()
	trayData(func(nid *notifyIconData) {
		nid.UFlags = nifMessage | nifIcon | nifTip
		nid.UCallbackMessage = trayMsg
		nid.HIcon = trayHicon
		putStr(nid.SzTip[:], "Wallpaparr — "+status)
		if r := call(pShellNotifyIconW, nimAdd, uintptr(unsafe.Pointer(nid))); r == 0 {
			guiLog("tray-icoon toevoegen mislukt (meldingenbal onbereikbaar in deze sessie?)")
		} else {
			guiLog("tray-icoon toegevoegd")
		}
	})
}

func trayTip(status string) {
	trayData(func(nid *notifyIconData) {
		nid.UFlags = nifTip
		putStr(nid.SzTip[:], "Wallpaparr — "+status)
		call(pShellNotifyIconW, 2 /*NIM_MODIFY*/, uintptr(unsafe.Pointer(nid)))
	})
}

// trayBalloon toont één melding; dat helpt om het icoon te vinden, want Windows 11
// zet nieuwe tray-iconen standaard in de verborgen-iconenoverloop.
func trayBalloon(titel, tekst string) {
	trayData(func(nid *notifyIconData) {
		nid.UFlags = nifInfo
		putStr(nid.SzInfoTitle[:], titel)
		putStr(nid.SzInfo[:], tekst)
		nid.DwInfoFlags = 0x01 // NIIF_INFO
		call(pShellNotifyIconW, 1 /*NIM_MODIFY*/, uintptr(unsafe.Pointer(nid)))
	})
}

func trayDel() {
	trayData(func(nid *notifyIconData) {
		nid.UFlags = 0
		call(pShellNotifyIconW, nimDelete, uintptr(unsafe.Pointer(nid)))
	})
}

func appendMenu(h, flags, id uintptr, tekst string) {
	call(pAppendMenuW, h, flags, id, uintptr(unsafe.Pointer(u16(tekst))))
}

func showTrayMenu() {
	hwnd := g.hwnd
	menu := call(pCreatePopupMenu)

	appendMenu(menu, mfString|mfDisabled, 0, "Wallpaparr v"+version)
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, idMSettings, "Instellingen…")
	appendMenu(menu, mfString, idMNow, "Nu synchroniseren")
	appendMenu(menu, mfString, idMTest, "Verbinding testen")
	appendMenu(menu, mfString, idMProfiles, "Open web-UI")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, idMQuit, "Afsluiten")

	var pt point
	call(pGetCursorPos, uintptr(unsafe.Pointer(&pt)))
	call(pSetForegroundWindow, hwnd) // nodig, anders sluit het menu niet netjes
	keuze := call(pTrackPopupMenu, menu, tpmRightButton|tpmReturnCmd,
		uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	call(pDestroyMenu, menu)

	switch keuze {
	case idMSettings:
		showVenster()
	case idMNow:
		g.cmds <- guiCmd{kind: "sync"}
	case idMTest:
		g.cmds <- guiCmd{kind: "test"}
	case idMProfiles:
		g.cmds <- guiCmd{kind: "web"}
	case idMQuit:
		afsluiten()
	}
}

func aanUit(b bool) string {
	if b {
		return "aan"
	}
	return "uit"
}

/* ---------- venster ---------- */

func showVenster() {
	hwnd := g.hwnd
	if hwnd == 0 {
		return
	}
	if call(pIsWindowVisible, hwnd) == 0 {
		call(pShowWindow, hwnd, swShow)
	}
	call(pSetForegroundWindow, hwnd)
}

func afsluiten() {
	cfg := g.getCfg()
	_ = cfg
	form := readForm()
	form.Save()
	trayDel()
	call(pDestroyWindow, g.hwnd)
}

func makeControl(class, tekst string, style uintptr, x, y, w, h int, id uintptr) uintptr {
	return call(pCreateWindowExW, 0,
		uintptr(unsafe.Pointer(u16(class))),
		uintptr(unsafe.Pointer(u16(tekst))),
		style|wsChild|wsVisible,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		g.hwnd, id, 0, 0)
}

func maakControls() {
	cfg := g.getCfg()
	vet := call(pCreateFontW, uintptr(fontHoogte), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(u16("Segoe UI"))))

	maakLabel := func(tekst string, x, y, w int) uintptr {
		h := makeControl("STATIC", tekst, ssLeft, x, y, w, 20, 0)
		call(pSendMessageW, h, 0x0030 /*WM_SETFONT*/, vet, 1)
		return h
	}
	maakEdit := func(tekst string, x, y, w int, id uintptr, extra uintptr) uintptr {
		h := makeControl("EDIT", tekst, wsBorder|wsTabStop|esAutoHScroll|extra, x, y, w, 24, id)
		call(pSendMessageW, h, 0x0030, vet, 1)
		return h
	}

	maakLabel("Wallpaparr v"+version+" — de server is de baas", 16, 14, 700)

	maakLabel("Server-URL", 16, 48, 130)
	g.editServer = maakEdit(cfg.Server, 150, 45, 380, idServer, 0)

	maakLabel("Weergave", 16, 80, 130)
	g.cmbPosition = makeControl("COMBOBOX", "", cbsDropDownList|wsTabStop|wsVScroll, 150, 78, 200, 200, idPosition)
	call(pSendMessageW, g.cmbPosition, 0x0030, vet, 1)
	for _, l := range positieLabels {
		call(pSendMessageW, g.cmbPosition, cbAddString, 0, uintptr(unsafe.Pointer(u16(l))))
	}
	idx := 0
	for i, p := range posities {
		if p == cfg.Position {
			idx = i
		}
	}
	call(pSendMessageW, g.cmbPosition, cbSetCurSel, uintptr(idx), 0)

	maakLabel("Poll-interval (seconden)", 380, 80, 190)
	g.editPoll = maakEdit(strconv.Itoa(pollSeconden(cfg)), 580, 78, 60, idPoll, esNumber)

	g.chkHidden = makeControl("BUTTON", "Bij opstarten alleen in de systeembalk (geen venster)", bsAutoCheckBox|wsTabStop, 16, 112, 460, 22, idChkHidden)
	call(pSendMessageW, g.chkHidden, 0x0030, vet, 1)
	if cfg.StartHidden {
		call(pSendMessageW, g.chkHidden, bmSetCheck, bstChecked, 0)
	}

	_ = maakLabel("Rotatie, tags, verhoudingen en keuzes stel je in de web-UI in. Deze client meldt alleen zijn schermen en zet wat de server opdraagt.", 16, 140, 780)

	maakLabel("Monitor", 16, 184, 280)
	maakLabel("Toont nu", 300, 184, 290)
	maakLabel("Sinds", 600, 184, 160)
	for i := 0; i < maxMonitorRijen; i++ {
		y := 206 + i*24
		g.lblMonitor[i] = maakLabel("", 16, y, 280)
		g.lblToont[i] = maakLabel("", 300, y, 290)
		g.lblSinds[i] = maakLabel("", 600, y, 160)
		for _, h := range []uintptr{g.lblMonitor[i], g.lblToont[i], g.lblSinds[i]} {
			call(pShowWindow, h, swHide)
		}
	}

	btnSave := makeControl("BUTTON", "Opslaan", bsPushButton|wsTabStop, 16, 366, 110, 28, idSave)
	btnSync := makeControl("BUTTON", "Nu synchroniseren", bsPushButton|wsTabStop, 136, 366, 155, 28, idApplyNow)
	btnTest := makeControl("BUTTON", "Verbinding testen", bsPushButton|wsTabStop, 301, 366, 150, 28, idTest)
	btnWeb := makeControl("BUTTON", "Open web-UI", bsPushButton|wsTabStop, 461, 366, 130, 28, idWebUI)
	for _, b := range []uintptr{btnSave, btnSync, btnTest, btnWeb} {
		call(pSendMessageW, b, 0x0030, vet, 1)
	}

	g.lblStatus = makeControl("STATIC", "klaar", ssLeft, 16, 404, 780, 160, idStatus)
	call(pSendMessageW, g.lblStatus, 0x0030, vet, 1)
}

// pollSeconden is het poll-interval in seconden (config, anders 5).
func pollSeconden(cfg Config) int {
	if cfg.Poll > 0 {
		return cfg.Poll
	}
	return 5
}

// statePadOf geeft het pad van het lokale swap-bestand.
func statePadOf() string {
	p, err := statePath()
	if err != nil {
		return "wallpaparr-state.json"
	}
	return p
}

// openURL opent een adres in de standaardbrowser.
func openURL(u string) {
	call(pShellExecuteW, 0,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16(u))), 0, 0, swShow)
}

func readForm() Config {
	cfg := g.getCfg()
	cfg.Server = normalizeServer(getText(g.editServer))
	if n, err := strconv.Atoi(getText(g.editPoll)); err == nil && n >= 1 {
		cfg.Poll = n
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 5
	}
	cfg.StartHidden = call(pSendMessageW, g.chkHidden, bmGetCheck, 0, 0) == bstChecked
	sel := int(call(pSendMessageW, g.cmbPosition, cbGetCurSel, 0, 0))
	if sel >= 0 && sel < len(posities) {
		cfg.Position = posities[sel]
	} else if cfg.Position == "" {
		cfg.Position = "fill"
	}
	return cfg
}

func onCommand(id, code uint16) {
	switch id {
	case idSave:
		cfg := readForm()
		if err := cfg.Save(); err != nil {
			g.setStatus("opslaan mislukt: " + err.Error())
			return
		}
		g.setCfg(cfg)
		zetTitel()
		g.cmds <- guiCmd{kind: "sync"}
	case idApplyNow:
		g.cmds <- guiCmd{kind: "sync"}
	case idTest:
		g.cmds <- guiCmd{kind: "test"}
	case idWebUI:
		g.cmds <- guiCmd{kind: "web"}
	}
}

// zetTitel houdt de venstertitel (met versie én server) actueel.
func zetTitel() {
	if g.hwnd == 0 {
		return
	}
	cfg := g.getCfg()
	titel := "Wallpaparr v" + version + " — wallpaper-client"
	if cfg.Server != "" {
		titel += " — " + cfg.Server
	}
	setText(g.hwnd, titel)
}

func updateStatus() {
	g.mu.Lock()
	s := g.status
	g.mu.Unlock()
	setText(g.lblStatus, s)
	trayTip(s)
}

func wndProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	// taakbalk opnieuw opgestart? dan het tray-icoon opnieuw aanmelden
	if taskbarCreatedMsg != 0 && message == taskbarCreatedMsg {
		guiLog("taakbalk opnieuw gestart — tray-icoon opnieuw aanmelden")
		trayAdd("v" + version + " — klaar")
		return 0
	}
	switch message {
	case wmCreate:
		g.hwnd = hwnd
		maakControls()
		return 0
	case wmCommand:
		onCommand(uint16(wparam&0xffff), uint16((wparam>>16)&0xffff))
		return 0
	case msgMonitors:
		g.verwerkMonitorLijst()
		return 0
	case msgStatus:
		updateStatus()
		return 0
	case msgShow:
		showVenster()
		return 0
	case wmClose:
		// kruisje = naar de systeembalk, niet stoppen
		call(pShowWindow, hwnd, swHide)
		g.setStatus("draait op de achtergrond (rechtsklik op het tray-icoon voor het menu)")
		return 0
	case wmDestroy:
		call(pPostQuitMessage, 0)
		return 0
	case trayMsg:
		switch uint32(lparam & 0xffff) {
		case wmLButtonDblClk, wmLButtonUp:
			showVenster()
		case wmRButtonUp:
			showTrayMenu()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}

/* ---------- poller: de client volgt de server ---------- */

// poller meldt de monitoren aan, pollt de server en zet wat de server opdraagt.
// De client roteert zelf niet meer: rotatie, tags en keuzes zijn serverbeleid.
func poller() {
	veilig := func(naam string, f func()) {
		defer func() {
			if r := recover(); r != nil {
				guiLog("PANIC in %s: %v\n%s", naam, r, debug.Stack())
				g.setStatus("interne fout bij " + naam + " — zie gui.log")
			}
		}()
		f()
	}

	device := g.getDevice()
	if device == "" {
		device = hostname()
	}
	laatste := loadState(statePadOf())

	var mons []string
	haalMonitoren := func() {
		if m, err := listMonitors(); err == nil {
			mons = m
			g.setMonitors(m)
			guiLog("monitoren: %d gevonden", len(m))
		} else {
			guiLog("monitoren ophalen mislukt: %v", err)
		}
	}

	doe := func() {
		cfg := g.getCfg()
		if cfg.Server == "" {
			g.setStatus("nog geen server ingesteld — vul de server-URL in en klik Opslaan")
			return
		}
		if len(mons) == 0 {
			haalMonitoren()
		}
		if len(mons) == 0 {
			g.setStatus("geen monitoren gevonden")
			return
		}

		// 1-2. de eigen schermen bij de server aanmelden
		nu := time.Now().Format("2006-01-02 15:04:05.000Z")
		for i, m := range mons {
			n := idxWithin(mons, i)
			s := Screen{
				Key:          device + "|" + m + "|" + strconv.Itoa(n),
				Device:       device,
				Monitor:      m,
				Index:        n,
				LaatstGewien: nu,
			}
			if err := screenUpsert(cfg.Server, s); err != nil {
				guiLog("scherm %q registreren mislukt: %v", s.Key, err)
			}
		}

		screens, err := screensForDevice(cfg.Server, device)
		if err != nil {
			guiLog("schermen ophalen mislukt: %v", err)
			g.setStatus("server niet bereikbaar: " + err.Error())
			return
		}

		// 3. toepassen wat de server opdraagt (alleen als het nog niet staat)
		var stats []monitorStatus
		var regels []string
		for _, s := range screens {
			toont := ""
			if s.HuidigeWallpaper != "" {
				if wp, err := getWallpaper(cfg.Server, s.HuidigeWallpaper); err == nil {
					toont = wp.Title
					if s.HuidigeWallpaper != laatste[s.Key] {
						if pad, err := download(cfg.Server, wp); err != nil {
							guiLog("downloaden mislukt: %v", err)
						} else if err := setMonitorWallpaper(s.Monitor, s.Index, pad, cfg.Position); err != nil {
							guiLog("zetten op %s mislukt: %v", s.Monitor, err)
						} else {
							_ = geschiedenisCreate(cfg.Server, s.ID, wp.ID, device, "rotatie")
							laatste[s.Key] = s.HuidigeWallpaper
							regels = append(regels, monitorLabel(s.Monitor, s.Index)+": "+wp.Title)
						}
					}
				}
			}
			stats = append(stats, monitorStatus{
				Monitor: monitorLabel(s.Monitor, s.Index),
				Toont:   toont,
				Sinds:   formatSince(s.LaatstGewien),
			})
		}
		_ = saveState(statePadOf(), laatste)
		g.setStats(stats)
		call(pPostMessageW, g.hwnd, msgMonitors, 0, 0)

		if len(regels) == 0 {
			g.setStatus("volgt de server — geen nieuwe opdracht (" + time.Now().Format("15:04:05") + ")")
		} else {
			g.setStatus("gezet:\r\n" + strings.Join(regels, "\r\n"))
		}
	}

	haalMonitoren()
	veilig("eerste synchronisatie", doe)

	// elke seconde kijken of het poll-interval verstreken is (interval is instelbaar)
	tik := time.NewTicker(time.Second)
	defer tik.Stop()
	volgende := time.Now().Add(time.Duration(pollSeconden(g.getCfg())) * time.Second)

	for {
		select {
		case c := <-g.cmds:
			switch c.kind {
			case "sync":
				veilig("synchroniseren", doe)
				volgende = time.Now().Add(time.Duration(pollSeconden(g.getCfg())) * time.Second)
			case "test":
				veilig("verbinding testen", func() {
					cfg := g.getCfg()
					if cfg.Server == "" {
						g.setStatus("nog geen server ingesteld")
						return
					}
					g.setStatus("verbinding testen…")
					if _, err := screensForDevice(cfg.Server, device); err != nil {
						guiLog("verbindingstest mislukt: %v", err)
						g.setStatus("verbinding mislukt: " + err.Error())
					} else {
						guiLog("verbindingstest ok")
						g.setStatus("verbinding ok — " + cfg.Server)
					}
				})
			case "web":
				veilig("web-UI openen", func() {
					cfg := g.getCfg()
					if cfg.Server == "" {
						g.setStatus("nog geen server ingesteld")
						return
					}
					openURL(cfg.Server)
				})
			case "monitors":
				veilig("monitoren ophalen", haalMonitoren)
			}
		case <-tik.C:
			if !time.Now().Before(volgende) {
				veilig("pollen", doe)
				volgende = time.Now().Add(time.Duration(pollSeconden(g.getCfg())) * time.Second)
			}
		}
	}
}

/* ---------- starten ---------- */

func runGUI() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// een panic in de GUI of de message loop moet in het logboek belanden, niet
	// stil het proces laten verdwijnen (de tray-variant heeft geen console)
	defer func() {
		if r := recover(); r != nil {
			guiLog("PANIC in runGUI: %v\n%s", r, debug.Stack())
		}
		guiLog("afgesloten")
	}()

	// voorkom een tweede tray-icoon
	if r, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("WallpaparrTrayMutex")))); r == 0 {
		return fmt.Errorf("mutex maken mislukt")
	} else if e == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		call(pMessageBoxW, 0,
			uintptr(unsafe.Pointer(u16("Wallpaparr draait al (zie het tray-icoon)."))),
			uintptr(unsafe.Pointer(u16("Wallpaparr"))), 0x40)
		return nil
	}

	cfgPad, _ := configPath()
	_, statErr := os.Stat(cfgPad)
	eersteKeer := statErr != nil // nog geen config = eerste start

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if env := os.Getenv("WALLPAPARR_SERVER"); env != "" {
		cfg.Server = normalizeServer(env)
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 5
	}
	if cfg.Position == "" {
		cfg.Position = "fill"
	}
	if d := kiesDevice(&cfg); d != cfg.Device {
		_ = cfg.Save()
	}
	g.setDevice(cfg.Device)
	g.setCfg(cfg)
	guiLog("start: versie=%s server=%q poll=%ds verborgen=%v", version, cfg.Server, pollSeconden(cfg), cfg.StartHidden)

	icon := loadIcon()
	hInst := call(pGetModuleHandleW, 0)
	taskbarCreatedMsg = uint32(call(pRegisterWindowMessageW, uintptr(unsafe.Pointer(u16("TaskbarCreated")))))
	klasse := u16("WallpaparrTrayWindow")
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		HIcon:         icon,
		HbrBackground: 6, // COLOR_WINDOW+1
		LpszClassName: klasse,
	}
	if call(pRegisterClassExW, uintptr(unsafe.Pointer(&wc))) == 0 {
		return fmt.Errorf("vensterklasse registreren mislukt")
	}

	style := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox)
	hwnd := call(pCreateWindowExW, 0,
		uintptr(unsafe.Pointer(klasse)),
		uintptr(unsafe.Pointer(u16("Wallpaparr v"+version+" — wallpaper-client"))),
		style, uintptr(cwUseDefault), uintptr(cwUseDefault), 820, 620, 0, 0, hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("venster maken mislukt")
	}
	g.hwnd = hwnd

	// donkere titelbalk (best effort)
	donker := int32(1)
	call(pDwmSetWindowAttribute, hwnd, 20, uintptr(unsafe.Pointer(&donker)), uintptr(unsafe.Sizeof(donker)))

	if !cfg.StartHidden {
		call(pShowWindow, hwnd, swShow)
	} else {
		call(pShowWindow, hwnd, swHide)
	}
	trayAdd("v" + version + " — klaar")
	if eersteKeer {
		trayBalloon("Wallpaparr", "De client draait in de systeembalk. Zie je het icoon niet meteen? Klik op het pijltje ^ naast de klok en sleep het naar de taakbalk.")
	}
	go poller()

	// berichtlus (moet op deze thread blijven)
	var m msg
	for {
		r := call(pGetMessageW, uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		call(pTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(pDispatchMessageW, uintptr(unsafe.Pointer(&m)))
	}
	trayDel()
	return nil
}
