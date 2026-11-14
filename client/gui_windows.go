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

	pRegisterClassExW      = user32.NewProc("RegisterClassExW")
	pCreateWindowExW       = user32.NewProc("CreateWindowExW")
	pDefWindowProcW        = user32.NewProc("DefWindowProcW")
	pGetMessageW           = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessageW      = user32.NewProc("DispatchMessageW")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pShowWindow            = user32.NewProc("ShowWindow")
	pIsWindowVisible       = user32.NewProc("IsWindowVisible")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pPostMessageW          = user32.NewProc("PostMessageW")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	pAppendMenuW           = user32.NewProc("AppendMenuW")
	pTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	pDestroyMenu           = user32.NewProc("DestroyMenu")
	pShellNotifyIconW      = shell32.NewProc("Shell_NotifyIconW") // shell32, niet user32!
	pLoadIconW             = user32.NewProc("LoadIconW")
	pCreateIconFromRes     = user32.NewProc("CreateIconFromResourceEx")
	pGetWindowTextW        = user32.NewProc("GetWindowTextW")
	pSetWindowTextW        = user32.NewProc("SetWindowTextW")
	pSendMessageW          = user32.NewProc("SendMessageW")
	pGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW") // kernel32, niet user32!
	pMessageBoxW           = user32.NewProc("MessageBoxW")
	pCreateFontW           = gdi32.NewProc("CreateFontW")
	pCreateMutexW          = kernel32.NewProc("CreateMutexW")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
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

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	nimAdd     = 0
	nimDelete  = 2
	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	idiApplication = 32512
)

var cwUseDefault = int32(-2147483648)
var fontHoogte = int32(-13) // negatief = tekenhoogte i.p.v. celhoogte

// eigen vensterberichten
const (
	trayMsg   = wmApp + 1 // callback van het tray-icoon
	msgStatus = wmApp + 2 // status bijwerken
	msgShow   = wmApp + 3 // venster naar voren halen
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
	idSave
	idApplyNow
	idTest
)

// menu-id's
const (
	idMSettings = 1001 + iota
	idMNow
	idMNext
	idMTest
	idMMonitor
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

type guiState struct {
	mu     sync.Mutex
	hwnd   uintptr
	cfg    Config
	status string
	cmds   chan guiCmd

	editServer  uintptr
	editTag     uintptr
	editMinutes uintptr
	chkMonitor  uintptr
	chkRandom   uintptr
	chkHidden   uintptr
	cmbPosition uintptr
	lblStatus   uintptr
}

var g = &guiState{cmds: make(chan guiCmd, 8)}

func (s *guiState) setCfg(c Config) { s.mu.Lock(); s.cfg = c; s.mu.Unlock() }
func (s *guiState) getCfg() Config  { s.mu.Lock(); defer s.mu.Unlock(); return s.cfg }
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
	cfg := g.getCfg()
	menu := call(pCreatePopupMenu)

	appendMenu(menu, mfString, idMSettings, "Instellingen…")
	appendMenu(menu, mfString, idMNow, "Nu wisselen")
	appendMenu(menu, mfString, idMNext, "Volgende wallpaper")
	appendMenu(menu, mfString, idMTest, "Verbinding testen")
	appendMenu(menu, mfSeparator, 0, "")

	monFlag := uintptr(mfString)
	if cfg.PerMonitor {
		monFlag |= mfChecked
	}
	appendMenu(menu, monFlag, idMMonitor, "Per monitor een andere wallpaper")

	sub := call(pCreatePopupMenu)
	for _, k := range intervalKeuzes {
		flags := uintptr(mfString)
		huidig := 0
		if d, err := time.ParseDuration(cfg.Every); err == nil {
			huidig = int(d.Minutes())
		}
		if k.minuten == huidig {
			flags |= mfChecked
		}
		label := fmt.Sprintf("%d minuten", k.minuten)
		if k.minuten == 0 {
			label = "uit (niet wisselen)"
		}
		appendMenu(sub, flags, k.id, label)
	}
	appendMenu(menu, mfPopup, sub, "Interval")

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
		g.cmds <- guiCmd{kind: "apply"}
	case idMNext:
		g.cmds <- guiCmd{kind: "next"}
	case idMTest:
		g.cmds <- guiCmd{kind: "test"}
	case idMMonitor:
		cfg.PerMonitor = !cfg.PerMonitor
		cfg.Save()
		g.setCfg(cfg)
		g.setStatus("per monitor: " + aanUit(cfg.PerMonitor))
	case idMQuit:
		afsluiten()
	default:
		for _, k := range intervalKeuzes {
			if keuze == k.id {
				cfg.Every = fmt.Sprintf("%dm", k.minuten)
				cfg.Save()
				g.setCfg(cfg)
				g.cmds <- guiCmd{kind: "interval", every: time.Duration(k.minuten) * time.Minute}
				if k.minuten == 0 {
					g.setStatus("wisselen uit")
				} else {
					g.setStatus(fmt.Sprintf("wisselt elke %d minuten", k.minuten))
				}
				return
			}
		}
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

	maakLabel := func(tekst string, x, y int) uintptr {
		h := makeControl("STATIC", tekst, ssLeft, x, y, 140, 20, 0)
		call(pSendMessageW, h, 0x0030 /*WM_SETFONT*/, vet, 1)
		return h
	}
	maakEdit := func(tekst string, x, y, w int, id uintptr, extra uintptr) uintptr {
		h := makeControl("EDIT", tekst, wsBorder|wsTabStop|esAutoHScroll|extra, x, y, w, 24, id)
		call(pSendMessageW, h, 0x0030, vet, 1)
		return h
	}

	maakLabel("Server-URL", 16, 20)
	g.editServer = maakEdit(cfg.Server, 150, 17, 310, idServer, 0)

	maakLabel("Tags (kommagescheiden)", 16, 54)
	g.editTag = maakEdit(cfg.Tag, 150, 51, 310, idTag, 0)

	minuten := 0
	if d, err := time.ParseDuration(cfg.Every); err == nil {
		minuten = int(d.Minutes())
	}
	maakLabel("Interval (minuten, 0 = uit)", 16, 88)
	g.editMinutes = maakEdit(strconv.Itoa(minuten), 150, 85, 80, idMinutes, esNumber)

	maakLabel("Weergave", 16, 122)
	g.cmbPosition = makeControl("COMBOBOX", "", cbsDropDownList|wsTabStop|wsVScroll, 150, 120, 200, 200, idPosition)
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

	g.chkMonitor = makeControl("BUTTON", "Per monitor een andere wallpaper", bsAutoCheckBox|wsTabStop, 16, 154, 260, 22, idChkMonitor)
	g.chkRandom = makeControl("BUTTON", "Willekeurige keuze", bsAutoCheckBox|wsTabStop, 290, 154, 170, 22, idChkRandom)
	g.chkHidden = makeControl("BUTTON", "Bij opstarten alleen in de systeembalk (geen venster)", bsAutoCheckBox|wsTabStop, 16, 178, 444, 22, idChkHidden)
	call(pSendMessageW, g.chkMonitor, 0x0030, vet, 1)
	call(pSendMessageW, g.chkRandom, 0x0030, vet, 1)
	call(pSendMessageW, g.chkHidden, 0x0030, vet, 1)
	if cfg.StartHidden {
		call(pSendMessageW, g.chkHidden, bmSetCheck, bstChecked, 0)
	}
	if cfg.PerMonitor {
		call(pSendMessageW, g.chkMonitor, bmSetCheck, bstChecked, 0)
	}
	if cfg.Random {
		call(pSendMessageW, g.chkRandom, bmSetCheck, bstChecked, 0)
	}

	btnSave := makeControl("BUTTON", "Opslaan en toepassen", bsPushButton|wsTabStop, 16, 210, 155, 28, idSave)
	btnNow := makeControl("BUTTON", "Nu wisselen", bsPushButton|wsTabStop, 180, 210, 110, 28, idApplyNow)
	btnTest := makeControl("BUTTON", "Verbinding testen", bsPushButton|wsTabStop, 299, 210, 161, 28, idTest)
	for _, b := range []uintptr{btnSave, btnNow, btnTest} {
		call(pSendMessageW, b, 0x0030, vet, 1)
	}

	g.lblStatus = makeControl("STATIC", "klaar", ssLeft, 16, 248, 444, 90, idStatus)
	call(pSendMessageW, g.lblStatus, 0x0030, vet, 1)
}

func readForm() Config {
	cfg := g.getCfg()
	cfg.Server = normalizeServer(getText(g.editServer))
	cfg.Tag = getText(g.editTag)
	if n, err := strconv.Atoi(getText(g.editMinutes)); err == nil && n >= 0 {
		cfg.Every = fmt.Sprintf("%dm", n)
	}
	if cfg.Every == "" {
		cfg.Every = "30m"
	}
	cfg.PerMonitor = call(pSendMessageW, g.chkMonitor, bmGetCheck, 0, 0) == bstChecked
	cfg.Random = call(pSendMessageW, g.chkRandom, bmGetCheck, 0, 0) == bstChecked
	cfg.StartHidden = call(pSendMessageW, g.chkHidden, bmGetCheck, 0, 0) == bstChecked
	sel := int(call(pSendMessageW, g.cmbPosition, cbGetCurSel, 0, 0))
	if sel >= 0 && sel < len(posities) {
		cfg.Position = posities[sel]
	} else if cfg.Position == "" {
		cfg.Position = "fill"
	}
	return cfg
}

func onCommand(id uint16) {
	switch id {
	case idSave:
		cfg := readForm()
		if err := cfg.Save(); err != nil {
			g.setStatus("opslaan mislukt: " + err.Error())
			return
		}
		g.setCfg(cfg)
		d, _ := time.ParseDuration(cfg.Every)
		g.cmds <- guiCmd{kind: "interval", every: d}
		g.cmds <- guiCmd{kind: "apply"}
	case idApplyNow:
		g.cmds <- guiCmd{kind: "apply"}
	case idTest:
		g.cmds <- guiCmd{kind: "test"}
	}
}

func updateStatus() {
	g.mu.Lock()
	s := g.status
	g.mu.Unlock()
	setText(g.lblStatus, s)
	trayTip(s)
}

func wndProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmCreate:
		g.hwnd = hwnd
		maakControls()
		return 0
	case wmCommand:
		onCommand(uint16(wparam & 0xffff))
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

/* ---------- scheduler ---------- */

func scheduler() {
	var ticker *time.Ticker
	var tick <-chan time.Time
	var lastID string

	stelIntervalIn := func(d time.Duration) {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
			tick = nil
		}
		if d > 0 {
			ticker = time.NewTicker(d)
			tick = ticker.C
		}
	}

	cfg := g.getCfg()
	if d, err := time.ParseDuration(cfg.Every); err == nil {
		stelIntervalIn(d)
	}

	// panics in de achtergrond mogen de app niet stil laten verdwijnen
	veilig := func(naam string, f func()) {
		defer func() {
			if r := recover(); r != nil {
				guiLog("PANIC in %s: %v\n%s", naam, r, debug.Stack())
				g.setStatus("interne fout bij " + naam + " — zie gui.log")
			}
		}()
		f()
	}

	doe := func() {
		cfg := g.getCfg()
		if cfg.Server == "" {
			g.setStatus("nog geen server ingesteld — vul de server-URL in en klik Opslaan")
			return
		}
		g.setStatus("bezig…")
		rec, err := applyChoice(cfg, cfg.Random, lastID)
		if err != nil {
			guiLog("wisselen mislukt: %v", err)
			g.setStatus("fout: " + err.Error())
			return
		}
		lastID = rec.ID
		guiLog("gezet: %s (%dx%d) id=%s", rec.Title, rec.Width, rec.Height, rec.ID)
		g.setStatus(fmt.Sprintf("gezet: %s (%dx%d)", rec.Title, rec.Width, rec.Height))
	}

	// eerste toepassing meteen
	veilig("eerste toepassing", doe)

	for {
		select {
		case c := <-g.cmds:
			switch c.kind {
			case "apply", "next":
				veilig("wisselen", doe)
			case "test":
				veilig("verbinding testen", func() {
					cfg := g.getCfg()
					g.setStatus("verbinding testen…")
					recs, err := fetchRecords(cfg.Server, cfg.Tag)
					if err != nil {
						guiLog("verbindingstest mislukt: %v", err)
						g.setStatus("verbinding mislukt: " + err.Error())
					} else {
						guiLog("verbindingstest ok: %d wallpapers", len(recs))
						g.setStatus(fmt.Sprintf("verbinding ok — %d wallpaper(s) op de server", len(recs)))
					}
				})
			case "interval":
				stelIntervalIn(c.every)
			}
		case <-tick:
			veilig("wisselen", doe)
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

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if env := os.Getenv("WALLPAPARR_SERVER"); env != "" {
		cfg.Server = normalizeServer(env)
	}
	if cfg.Every == "" {
		cfg.Every = "30m"
	}
	if cfg.Position == "" {
		cfg.Position = "fill"
	}
	g.setCfg(cfg)
	guiLog("start: server=%q tags=%q interval=%q per_monitor=%v random=%v verborgen=%v",
		cfg.Server, cfg.Tag, cfg.Every, cfg.PerMonitor, cfg.Random, cfg.StartHidden)

	icon := loadIcon()
	hInst := call(pGetModuleHandleW, 0)
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
		uintptr(unsafe.Pointer(u16("Wallpaparr — wallpaper-client"))),
		style, uintptr(cwUseDefault), uintptr(cwUseDefault), 490, 400, 0, 0, hInst, 0)
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
	trayAdd("klaar")
	go scheduler()

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
