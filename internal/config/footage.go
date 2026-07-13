package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// footageState is the ctrl+f footage flow's own tiny state file — separate
// from config.toml (that file is hand-edited server/sounds/conversation
// settings; the last-used folder is an internal remembered value, never
// something the owner sets with `pito-tui config`). Mirrors jar.go's own
// write-through JSON file: missing or corrupt is never an error, just an
// empty start.
type footageState struct {
	LastFolder string `json:"last_folder"`
}

// FootagePath returns the footage state file location inside Dir().
func FootagePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "footage.json"), nil
}

// LoadFootageFolder reads the last-used footage folder. A missing or
// corrupt file answers "" — the ctrl+f flow's own FolderPicker falls back
// to $HOME in that case (NewFolderPicker's own contract), so there is
// nothing else to do here.
func LoadFootageFolder(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var st footageState
	if err := json.Unmarshal(raw, &st); err != nil {
		return ""
	}
	return st.LastFolder
}

// SaveFootageFolder persists the confirmed folder atomically (tmp file +
// rename), jar.go's own write pattern.
func SaveFootageFolder(path, folder string) error {
	raw, err := json.Marshal(footageState{LastFolder: folder})
	if err != nil {
		return fmt.Errorf("config: encoding footage state: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: creating %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("config: writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: writing %s: %w", path, err)
	}
	return nil
}
