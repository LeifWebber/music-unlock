//go:build !windows

package discovery

import "path/filepath"

func musicDirectory(home string) string { return filepath.Join(home, "Music") }
