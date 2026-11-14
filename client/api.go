package main

import (
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

type listResponse struct {
	Page       int      `json:"page"`
	TotalPages int      `json:"totalPages"`
	Items      []Record `json:"items"`
}

// Config staat in het gebruikersconfiguratiemapje.
type Config struct {
	Server      string `json:"server"`
	Tag         string `json:"tag"`
	PerMonitor  bool   `json:"per_monitor"`
	Random      bool   `json:"random"`
	StartHidden bool   `json:"start_hidden"`
	Every       string `json:"every"`
	Position    string `json:"position"`
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
		return cfg, nil // nog geen config = standaardwaarden
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

// buildFilter maakt van "a,b" het PocketBase-filter (tags~'a'||tags~'b').
func buildFilter(tags string) string {
	var parts []string
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
