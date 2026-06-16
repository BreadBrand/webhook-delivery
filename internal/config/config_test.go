package config_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/config"
)

func TestLoadCreatesSecretsOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("EncryptionKey length = %d, want 32", len(cfg.EncryptionKey))
	}
	if cfg.APIKey == "" {
		t.Error("APIKey must not be empty")
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Error("secrets.json was not created")
	} else if info.Mode().Perm() != 0600 {
		t.Errorf("secrets.json permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadReadsExistingSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	cfg1, _ := config.Load(path)

	cfg2, err := config.Load(path)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if string(cfg1.EncryptionKey) != string(cfg2.EncryptionKey) {
		t.Error("EncryptionKey must be stable across loads")
	}
	if cfg1.APIKey != cfg2.APIKey {
		t.Error("APIKey must be stable across loads")
	}
}

func TestLoadRejectsShortKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	shortKey := base64.StdEncoding.EncodeToString([]byte("tooshort"))
	content := `{"encryption_key":"` + shortKey + `","api_key":"validkey"}`
	os.WriteFile(path, []byte(content), 0600)

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for short encryption key")
	}
}

func TestLoadRejectsEmptyAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	content := `{"encryption_key":"` + key + `","api_key":""}`
	os.WriteFile(path, []byte(content), 0600)

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for empty api_key")
	}
}

func TestDefaultValues(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("WORKER_COUNT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("SIMULATE", "")
	t.Setenv("LOG_FORMAT", "")

	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "secrets.json"))

	if cfg.Port != "8080" {
		t.Errorf("default Port = %q, want 8080", cfg.Port)
	}
	if cfg.WorkerCount != 10 {
		t.Errorf("default WorkerCount = %d, want 10", cfg.WorkerCount)
	}
	if cfg.DBPath != "data/webhooks.db" {
		t.Errorf("default DBPath = %q", cfg.DBPath)
	}
}
