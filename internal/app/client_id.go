package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const clientIDFile = "client_id"

var clientIDPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{5,63}$`)

type AppInfo struct {
	ClientID string `json:"clientId"`
	Version  string `json:"version"`
}

func LoadOrCreateClientID(appDir string) (string, error) {
	path := filepath.Join(appDir, clientIDFile)
	if b, err := os.ReadFile(path); err == nil {
		id := sanitizeClientID(string(b))
		if id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read client id: %w", err)
	}

	id, err := newClientID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", fmt.Errorf("create app dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write client id: %w", err)
	}
	return id, nil
}

func newClientID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate client id: %w", err)
	}
	return "NC-" + strings.ToUpper(hex.EncodeToString(b)), nil
}

func sanitizeClientID(raw string) string {
	id := strings.ToUpper(strings.TrimSpace(raw))
	id = strings.ReplaceAll(id, " ", "_")
	if !clientIDPattern.MatchString(id) {
		return ""
	}
	return id
}

func safeFilenamePrefix(raw string) string {
	id := sanitizeClientID(raw)
	if id == "" {
		return "netchecker"
	}
	return id
}

func (a *App) GetAppInfo() AppInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return AppInfo{
		ClientID: a.clientID,
		Version:  a.version,
	}
}
