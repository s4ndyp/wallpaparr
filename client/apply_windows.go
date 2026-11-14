//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

/*
Windows-implementatie.

- Wallpaper zetten op alle monitoren: IDesktopWallpaper (Windows 8+) per monitor,
  met SystemParametersInfoW als terugvalpad.
- Per monitor een andere wallpaper: IDesktopWallpaper::SetWallpaper.
- Aantal monitoren: IDesktopWallpaper::GetMonitorDevicePathCount.
*/

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	ole32  = syscall.NewLazyDLL("ole32.dll")

	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance      = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree         = ole32.NewProc("CoTaskMemFree")
)

const (
	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	spifSendChange      = 0x02
	smCMonitors         = 80
	clsctxAll           = 0x17
	coinitApartment     = 0x2
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidDesktopWallpaper = guid{0xC2CF3110, 0x460E, 0x4FC1, [8]byte{0xB9, 0xD0, 0x8A, 0x1C, 0x0C, 0x9C, 0xC4, 0xBD}}
	iidDesktopWallpaper   = guid{0xB92B56A9, 0x8B55, 0x4E14, [8]byte{0x9A, 0x89, 0x01, 0x99, 0xBB, 0xB6, 0xF9, 0x3B}}
)

// iDesktopWallpaper is de COM-interface (alleen de methodes die we nodig hebben).
type iDesktopWallpaper struct {
	vtbl *[19]uintptr
}

var (
	comOnce sync.Once
	comErr  error
)

func initCOM() error {
	comOnce.Do(func() {
		// COM is per-thread en Go kan een goroutine tussen OS-threads verplaatsen:
		// zonder LockOSThread geeft CoCreateInstance CO_E_NOTINITIALIZED (0x800401f0)
		// omdat CoInitializeEx op een ándere thread is uitgevoerd.
		runtime.LockOSThread()
		hr, _, _ := procCoInitializeEx.Call(0, coinitApartment)
		if int32(hr) < 0 {
			comErr = fmt.Errorf("CoInitializeEx mislukt (0x%08x)", uint32(hr))
		}
	})
	return comErr
}

func newDesktopWallpaper() (*iDesktopWallpaper, func(), error) {
	if err := initCOM(); err != nil {
		return nil, func() {}, err
	}
	var p unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidDesktopWallpaper)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidDesktopWallpaper)),
		uintptr(unsafe.Pointer(&p)),
	)
	if int32(hr) < 0 || p == nil {
		return nil, func() {}, fmt.Errorf("IDesktopWallpaper niet beschikbaar (0x%08x) — Windows 8 of nieuwer vereist", uint32(hr))
	}
	w := (*iDesktopWallpaper)(p)
	return w, func() { _, _ = w.invoke(2) }, nil // Release
}

func (w *iDesktopWallpaper) invoke(idx int, args ...uintptr) (uintptr, error) {
	fn := w.vtbl[idx]
	a := append([]uintptr{uintptr(unsafe.Pointer(w))}, args...)
	r, _, _ := syscall.SyscallN(fn, a...)
	if int32(r) < 0 {
		return r, fmt.Errorf("COM-fout 0x%08x", uint32(r))
	}
	return r, nil
}

// monitorCount = GetMonitorDevicePathCount (vtable 6)
func (w *iDesktopWallpaper) monitorCount() (int, error) {
	var n uint32
	_, err := w.invoke(6, uintptr(unsafe.Pointer(&n)))
	return int(n), err
}

// monitorPath = GetMonitorDevicePathAt (vtable 5)
func (w *iDesktopWallpaper) monitorPath(i int) (string, error) {
	var p *uint16
	_, err := w.invoke(5, uintptr(uint32(i)), uintptr(unsafe.Pointer(&p)))
	if err != nil || p == nil {
		return "", err
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return utf16PtrToString(p), nil
}

// utf16PtrToString leest een NUL-beëindigde UTF-16-string uit COM-geheugen.
func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var s []uint16
	for i := 0; i < 4096; i++ {
		c := *(*uint16)(unsafe.Add(unsafe.Pointer(p), uintptr(i*2)))
		if c == 0 {
			break
		}
		s = append(s, c)
	}
	return syscall.UTF16ToString(s)
}

// setWallpaper = IDesktopWallpaper::SetWallpaper (vtable 3)
func (w *iDesktopWallpaper) setWallpaper(monitor, path string) error {
	m, err := syscall.UTF16PtrFromString(monitor)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	_, err = w.invoke(3, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(p)))
	return err
}

// setPosition = SetPosition (vtable 10)
func (w *iDesktopWallpaper) setPosition(pos int) error {
	_, err := w.invoke(10, uintptr(uint32(pos)))
	return err
}

func positionIndex(position string) int {
	switch position {
	case "center":
		return 0
	case "stretch":
		return 1
	case "fit":
		return 2
	case "span":
		return 4
	case "tile":
		return 5
	default: // fill
		return 3
	}
}

/* ---------- publieke platform-API ---------- */

func listMonitors() ([]string, error) {
	w, release, err := newDesktopWallpaper()
	if err == nil {
		defer release()
		if n, err := w.monitorCount(); err == nil && n > 0 {
			out := make([]string, 0, n)
			for i := 0; i < n; i++ {
				if p, err := w.monitorPath(i); err == nil {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	n, _, _ := procGetSystemMetrics.Call(smCMonitors)
	if n == 0 {
		n = 1
	}
	mons := make([]string, n)
	for i := range mons {
		mons[i] = fmt.Sprintf("monitor-%d", i)
	}
	if err != nil {
		return mons, nil // terugval: telling via GetSystemMetrics
	}
	return mons, nil
}

func setWallpaperAll(path, position string) error {
	w, release, err := newDesktopWallpaper()
	if err == nil {
		defer release()
		_ = w.setPosition(positionIndex(position))
		if n, err := w.monitorCount(); err == nil && n > 0 {
			ok := 0
			for i := 0; i < n; i++ {
				p, err := w.monitorPath(i)
				if err != nil {
					continue
				}
				if err := w.setWallpaper(p, path); err == nil {
					ok++
				}
			}
			if ok > 0 {
				return nil
			}
		}
	}
	return systemParametersWallpaper(path)
}

func setMonitorWallpaper(monitor string, index int, path, position string) error {
	w, release, err := newDesktopWallpaper()
	if err != nil {
		return err
	}
	defer release()
	_ = w.setPosition(positionIndex(position))
	if monitor == "" {
		monitor, err = w.monitorPath(index)
		if err != nil {
			return err
		}
	}
	return w.setWallpaper(monitor, path)
}

func systemParametersWallpaper(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, e := procSystemParametersInfoW.Call(
		spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), spifUpdateIniFile|spifSendChange)
	if r == 0 {
		return fmt.Errorf("SystemParametersInfoW mislukt (%v)", e)
	}
	return nil
}

// friendlyMonitor maakt van een COM-device-pad iets leesbaars.
func friendlyMonitor(id string) string {
	// \\?\DISPLAY#DEL40E8#5&2b8f...&0&UID4352#{guid}
	s := strings.TrimPrefix(id, `\\?\`)
	parts := strings.Split(s, "#")
	if len(parts) >= 2 && parts[1] != "" {
		return parts[1]
	}
	return id
}
