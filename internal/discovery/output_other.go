//go:build !windows

package discovery

import (
	"os"
	"path/filepath"
	"runtime"
)

func downloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(config) {
			config = filepath.Join(home, ".config")
		}
		if data, err := readConfig(filepath.Join(config, "user-dirs.dirs")); err == nil {
			if path := xdgDownloadDirectory(string(data), home); path != "" {
				return path, nil
			}
		}
	}
	return filepath.Join(home, "Downloads"), nil
}
