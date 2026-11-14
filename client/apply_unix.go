//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

/* ---------- hulpmiddelen ---------- */

func have(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runOk(name string, args ...string) error {
	out, err := run(name, args...)
	if err != nil {
		return fmt.Errorf("%s: %v (%s)", name, err, out)
	}
	return nil
}

func isWayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }
func isX11() bool     { return os.Getenv("DISPLAY") != "" }

// friendlyMonitor: op Unix is de monitornaam al leesbaar.
func friendlyMonitor(id string) string { return id }

/* ---------- monitoren ---------- */

// listMonitors geeft de namen van de aangesloten monitoren.
func listMonitors() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := run("system_profiler", "SPDisplaysDataType")
		if err == nil {
			n := strings.Count(out, "Resolution:")
			if n == 0 {
				n = 1
			}
			mons := make([]string, n)
			for i := range mons {
				mons[i] = fmt.Sprintf("display-%d", i)
			}
			return mons, nil
		}
		return []string{"display-0"}, nil
	}

	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && have("hyprctl") {
		if out, err := run("hyprctl", "-j", "monitors"); err == nil {
			if m := jsonNames(out); len(m) > 0 {
				return m, nil
			}
		}
	}
	if have("swaymsg") {
		if out, err := run("swaymsg", "-t", "get_outputs"); err == nil {
			if m := jsonNames(out); len(m) > 0 {
				return m, nil
			}
		}
	}
	if isX11() && have("xrandr") {
		if out, err := run("xrandr", "--listmonitors"); err == nil {
			var mons []string
			for i, line := range strings.Split(out, "\n") {
				if i == 0 || strings.TrimSpace(line) == "" {
					continue
				}
				f := strings.Fields(line)
				if len(f) > 0 {
					mons = append(mons, f[len(f)-1])
				}
			}
			if len(mons) > 0 {
				return mons, nil
			}
		}
	}
	if have("wlr-randr") {
		if out, err := run("wlr-randr"); err == nil {
			var mons []string
			for _, line := range strings.Split(out, "\n") {
				if line == "" || line[0] == ' ' || line[0] == '\t' {
					continue
				}
				f := strings.Fields(line)
				if len(f) > 0 && !strings.HasSuffix(f[0], ":") {
					mons = append(mons, f[0])
				}
			}
			if len(mons) > 0 {
				return mons, nil
			}
		}
	}
	return nil, fmt.Errorf("geen monitor-informatie gevonden (xrandr/swaymsg/hyprctl ontbreken)")
}

// jsonNames peutst 'name'-velden uit een JSON-array van monitoren.
func jsonNames(s string) []string {
	var objs []map[string]any
	if err := json.Unmarshal([]byte(s), &objs); err != nil {
		return nil
	}
	var out []string
	for _, o := range objs {
		if n, ok := o["name"].(string); ok && n != "" {
			out = append(out, n)
		}
	}
	return out
}

/* ---------- wallpaper zetten ---------- */

// setWallpaperAll zet één wallpaper op alle monitoren.
func setWallpaperAll(path, position string) error {
	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf(`tell application "System Events" to set picture of every desktop to "%s"`, path)
		return runOk("osascript", "-e", script)
	}

	if isWayland() {
		if have("swww") {
			if err := runOk("swww", "img", path, "--resize", swwwResize(position)); err == nil {
				return nil
			}
		}
		if have("hyprctl") {
			if err := runOk("hyprctl", "hyprpaper", "wallpaper", ","+path); err == nil {
				return nil
			}
		}
		if have("swaybg") {
			cmd := exec.Command("swaybg", "-i", path, "-m", swaybgMode(position))
			cmd.Stdout, cmd.Stderr = nil, nil
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}

	if have("feh") && isX11() {
		if err := runOk("feh", "--no-fehbg", fehFlag(position), path); err == nil {
			return nil
		}
	}
	if have("nitrogen") {
		if err := runOk("nitrogen", "--set-zoom-fill", "--save", path); err == nil {
			return nil
		}
	}
	if have("gsettings") {
		uri := "file://" + path
		_ = runOk("gsettings", "set", "org.gnome.desktop.background", "picture-uri", uri)
		if err := runOk("gsettings", "set", "org.gnome.desktop.background", "picture-uri-dark", uri); err == nil {
			return nil
		}
	}
	if have("plasma-apply-wallpaperimage") {
		if err := runOk("plasma-apply-wallpaperimage", path); err == nil {
			return nil
		}
	}
	if have("xfconf-query") {
		out, err := run("xfconf-query", "-c", "xfce4-desktop", "-l")
		if err == nil {
			for _, p := range strings.Split(out, "\n") {
				if strings.HasSuffix(strings.TrimSpace(p), "/last-image") {
					_ = runOk("xfconf-query", "-c", "xfce4-desktop", "-p", strings.TrimSpace(p), "-s", path)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("geen geschikte wallpaper-tool gevonden (installeer feh, swww, swaybg, gsettings of plasma-apply-wallpaperimage)")
}

// setMonitorWallpaper zet een wallpaper op één specifieke monitor.
func setMonitorWallpaper(monitor string, index int, path, position string) error {
	if runtime.GOOS == "darwin" {
		return fmt.Errorf("per-monitor wordt op macOS niet ondersteund")
	}
	if isWayland() && have("swww") {
		if err := runOk("swww", "img", path, "--outputs", monitor, "--resize", swwwResize(position)); err == nil {
			return nil
		}
	}
	if isX11() && have("feh") {
		if err := runOk("feh", "--no-fehbg", fehFlag(position), "--xinerama-index", strconv.Itoa(index), path); err == nil {
			return nil
		}
	}
	return fmt.Errorf("per-monitor vereist swww (Wayland) of feh met Xinerama (X11)")
}

/* ---------- modus-omzetting ---------- */

func swwwResize(position string) string {
	switch position {
	case "fit":
		return "fit"
	case "stretch":
		return "stretch"
	case "center":
		return "no"
	default: // fill / span
		return "crop"
	}
}

func swaybgMode(position string) string {
	switch position {
	case "fit":
		return "fit"
	case "stretch":
		return "stretch"
	case "center":
		return "center"
	case "tile":
		return "tile"
	default:
		return "fill"
	}
}

func fehFlag(position string) string {
	switch position {
	case "fit":
		return "--bg-max"
	case "stretch":
		return "--bg-scale"
	case "center":
		return "--bg-center"
	case "tile":
		return "--bg-tile"
	default: // fill / span
		return "--bg-fill"
	}
}
