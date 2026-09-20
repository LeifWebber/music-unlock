package discovery

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

func musicDirectory(home string) string {
	if path, err := windows.KnownFolderPath(windows.FOLDERID_Music, 0); err == nil {
		return path
	}
	return filepath.Join(home, "Music")
}
