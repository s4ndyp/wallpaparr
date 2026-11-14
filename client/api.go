package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record is één rij uit de PocketBase-collectie 'wallpapers'.
type Record struct {
	ID             string `json:"id"`
	CollectionID   string `json:"collectionId"`
	CollectionName string `json:"collectionName"`
	Title          string `json:"title"`
	Tags           string `json:"tags"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Bytes          int64  `json:"bytes"`
	File           string `json:"file"`
	Created        string `json:"created"`
}

// Screen is één rij uit de 'screens'-collectie (met expand).
type Screen struct {
	ID                     string `json:"id"`
	Key                    string `json:"key"`
	Device                 string `json:"device"`
	Monitor                string `json:"monitor"`
	Index                  int    `json:"index"`
	Width                  int    `json:"width,omitempty"`
	Height                 int    `json:"height,omitempty"`
	LaatstGewien           string `json:"laatst_gezien,omitempty"`
	Playlist               string `json:"playlist,omitempty"`
	HuidigeWallpaper       string `json:"huidige_wallpaper,omitempty"`
	Bron                   string `json:"bron,omitempty"`
	ExpandPlaylist         string `json:"expand.playlist,omitempty"`
	ExpandHuidigeWallpaper string `json:"expand.huidige_wallpaper,omitempty"`
}

// tagList splitst een kommagescheiden tags-string in losse tags.
func tagList(s string) []string {
	var uit []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			uit = append(uit, t)
		}
	}
	return uit
}

// Config staat in het gebruikersconfiguratiemapje.
type Config struct {
	Server      string           `json:"server"`
	Device      string           `json:"device"`
	Poll        int              `json:"poll"`
	Tag         string           `json:"tag"`
	PerMonitor  bool             `json:"per_monitor"`
	Random      bool             `json:"random"`
	StartHidden bool             `json:"start_hidden"`
	Every       string           `json:"every"`
	Position    string           `json:"position"`
	Monitors    []MonitorProfiel `json:"monitors,omitempty"`
}

// Save schrijft de configuratie weg (de GUI gebruikt dit); vult defaults in.
func (c Config) Save() error {
	if c.Every == "" {
		c.Every = "30m"
	}
	if c.Position == "" {
		c.Position = "fill"
	}
	return saveConfig(c)
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wallpaparr", "config.json"), nil
}

func loadConfig() (Config, error) {
	cfg := Config{Every: "30m", Position: "fill"}
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cfg, nil
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg, nil
}

func saveConfig(cfg Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// mustConfig: config laden, met env-var en CLI als overschrijving.
func mustConfig(cliServer string) Config {
	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if env := os.Getenv("WALLPAPARR_SERVER"); env != "" {
		cfg.Server = normalizeServer(env)
	}
	if cliServer != "" {
		cfg.Server = normalizeServer(cliServer)
	}
	if cfg.Every == "" {
		cfg.Every = "30m"
	}
	if cfg.Position == "" {
		cfg.Position = "fill"
	}
	return cfg
}

func normalizeServer(s string) string {
	s = strings.TrimSpace(strings.TrimRight(s, "/"))
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "http://" + s
	}
	return s
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// fetchRecords haalt alle wallpapers op, optioneel gefilterd op tags.
func fetchRecords(server, tags string) ([]Record, error) {
	if server == "" {
		return nil, fmt.Errorf("geen server ingesteld")
	}
	extra := ""
	if f := buildFilter(tags); f != "" {
		extra = "&filter=" + url.QueryEscape(f)
	}
	var out []Record
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s/api/collections/wallpapers/records?page=%d&perPage=200&sort=-created%s",
			server, page, extra)
		var lr listResponse
		if err := getJSON(u, &lr); err != nil {
			return nil, err
		}
		out = append(out, lr.Items...)
		if page >= lr.TotalPages || len(lr.Items) == 0 {
			break
		}
	}
	return out, nil
}

// listResponse het antwoord van een records-lijst.
type listResponse struct {
	Page       int      `json:"page"`
	TotalPages int      `json:"totalPages"`
	TotalItems int      `json:"totalItems"`
	Items      []Record `json:"items"`
}

func buildFilter(tags string) string {
	parts := []string{}
	for _, t := range strings.Split(tags, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		t = strings.ReplaceAll(t, "'", "\\'")
		parts = append(parts, fmt.Sprintf("tags~'%s'", t))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, "||") + ")"
}

func getJSON(u string, dst any) error {
	resp, err := httpClient.Get(u)
	if err != nil {
		return fmt.Errorf("%s onbereikbaar: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s gaf HTTP %d: %s", u, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// download haalt het bestand op naar de cachemap en geeft het pad terug.
func download(server string, r Record) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "wallpaparr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(r.File)
	dest := filepath.Join(dir, r.ID+"-"+name)

	fileURL := fmt.Sprintf("%s/api/files/%s/%s/%s", server, r.CollectionID, r.ID, url.PathEscape(name))
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return dest, nil // al in de cache
	}

	resp, err := httpClient.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("downloaden mislukt: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloaden mislukt: HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Screen upsert
func screenUpsert(server string, s Screen) error {
	bestaand, err := screenByKey(server, s.Key)
	if err != nil {
		return err
	}
	if bestaand == nil {
		return postScreen(server, s)
	}
	s.ID = bestaand.ID
	return patchScreen(server, s.ID, s)
}

func postScreen(server string, s Screen) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", server+"/api/collections/screens/records", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("POST scherm gaf HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func patchScreen(server, id string, s Screen) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PATCH", server+"/api/collections/screens/records/"+id, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("PATCH scherm %s gaf HTTP %d: %s", id, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func screenByKey(server, key string) (*Screen, error) {
	if server == "" {
		return nil, fmt.Errorf("geen server ingesteld")
	}
	filter := fmt.Sprintf("(key='%s')", url.QueryEscape(key))
	u := fmt.Sprintf("%s/api/collections/screens/records?perPage=1&filter=%s", server, url.QueryEscape(filter))
	var rows []Screen
	if err := getJSON(u, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func screensForDevice(server, device string) ([]Screen, error) {
	if server == "" {
		return nil, fmt.Errorf("geen server ingesteld")
	}
	filter := fmt.Sprintf("(device='%s')", url.QueryEscape(device))
	u := fmt.Sprintf("%s/api/collections/screens/records?perPage=200&sort=index&expand=playlist,huidige_wallpaper&filter=%s",
		server, url.QueryEscape(filter))
	var out []Screen
	if err := getJSON(u, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func getWallpaper(server, id string) (Record, error) {
	if server == "" {
		return Record{}, fmt.Errorf("geen server ingesteld")
	}
	var r Record
	u := fmt.Sprintf("%s/api/collections/wallpapers/records/%s?expand=wallpapers", server, url.QueryEscape(id))
	if err := getJSON(u, &r); err != nil {
		return r, err
	}
	return r, nil
}

// geschiedenisCreate veld het toegepaste beeld in de geschiedenis.
func geschiedenisCreate(server, screenID, wpID, device, bron string) error {
	if server == "" {
		return fmt.Errorf("geen server ingesteld")
	}
	body := map[string]any{
		"screen":    screenID,
		"wallpaper": wpID,
		"device":    device,
		"bron":      bron,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", server+"/api/collections/geschiedenis/records", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("geschiedenis POST gaf HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
