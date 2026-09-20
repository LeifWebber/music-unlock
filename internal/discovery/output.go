package discovery

import "path/filepath"

// DefaultOutput groups all automatically discovered libraries in the user's
// system Downloads folder. Resolving it never creates directories.
func DefaultOutput() (string, error) {
	downloads, err := downloadsDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(downloads, "已解锁音乐"), nil
}
