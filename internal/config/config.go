package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	EncryptionKey []byte
	APIKey        string
	Port          string
	WorkerCount   int
	DBPath        string
	Simulate      bool
	LogFormat     string
}

type secretsFile struct {
	EncryptionKey string `json:"encryption_key"`
	APIKey        string `json:"api_key"`
}

func Load(secretsPath string) (*Config, error) {
	s, err := loadOrCreate(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(s.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decode encryption_key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption_key must decode to 32 bytes, got %d", len(key))
	}
	if s.APIKey == "" {
		return nil, fmt.Errorf("api_key must not be empty")
	}

	return &Config{
		EncryptionKey: key,
		APIKey:        s.APIKey,
		Port:          env("PORT", "8080"),
		WorkerCount:   envInt("WORKER_COUNT", 10),
		DBPath:        env("DB_PATH", "data/webhooks.db"),
		Simulate:      envBool("SIMULATE", false),
		LogFormat:     env("LOG_FORMAT", "text"),
	}, nil
}

func loadOrCreate(path string) (secretsFile, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var s secretsFile
		if err := json.Unmarshal(data, &s); err != nil {
			return secretsFile{}, fmt.Errorf("parse %s: %w", path, err)
		}
		return s, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return secretsFile{}, err
	}

	s := secretsFile{
		EncryptionKey: randomBase64(32),
		APIKey:        randomBase64(32),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return secretsFile{}, err
	}
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return secretsFile{}, fmt.Errorf("marshal secrets: %w", err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return secretsFile{}, err
	}
	return s, nil
}

func randomBase64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on Go 1.20+, but guard anyway.
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	return base64.StdEncoding.EncodeToString(b)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}
