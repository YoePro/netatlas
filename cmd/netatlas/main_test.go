package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServerServesPagesAndStaticAssets(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	tests := []string{
		"/",
		"/login",
		"/map",
		"/sensors",
		"/sensor",
		"/html/index.html",
		"/static/css/base.css",
		"/static/css/layout.css",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			res, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
		})
	}
}

func TestLoadServerConfigReadsUISection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "custom", "html"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "custom", "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.ini")
	content := `[ui]
listen_address = 0.0.0.0
port = 9090
html_dir = custom/html
static_dir = custom/static
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.listenAddress != "0.0.0.0" || cfg.port != 9090 {
		t.Fatalf("listen config = %s:%d", cfg.listenAddress, cfg.port)
	}
	if cfg.htmlDir != filepath.Join(dir, "custom/html") || cfg.staticDir != filepath.Join(dir, "custom/static") {
		t.Fatalf("asset dirs = %q/%q", cfg.htmlDir, cfg.staticDir)
	}
	if cfg.listenAddr() != "0.0.0.0:9090" {
		t.Fatalf("listenAddr = %q", cfg.listenAddr())
	}
}

func TestMergeServerConfigAppliesFlagOverrides(t *testing.T) {
	fileCfg := serverConfig{
		configPath:    "config.ini",
		listenAddress: "0.0.0.0",
		port:          8080,
		htmlDir:       "web/html",
		staticDir:     "web/static",
	}
	flagCfg := serverConfig{
		configPath:    "other.ini",
		listenAddress: "192.168.1.20",
		port:          9090,
	}

	cfg := mergeServerConfig(fileCfg, flagCfg)
	if cfg.configPath != "other.ini" || cfg.listenAddress != "192.168.1.20" || cfg.port != 9090 {
		t.Fatalf("merged config = %#v", cfg)
	}
	if cfg.htmlDir != "web/html" || cfg.staticDir != "web/static" {
		t.Fatalf("asset dirs should come from file config: %#v", cfg)
	}
}

func TestServerServesMockAPI(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/sensors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	var sensors []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sensors); err != nil {
		t.Fatal(err)
	}
	if len(sensors) == 0 {
		t.Fatal("sensors response was empty")
	}
}
