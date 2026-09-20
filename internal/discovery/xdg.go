package discovery

import "strings"

// Parse the literal paths in user-dirs.dirs without sourcing shell code.
// XDG accepts an absolute path or a path relative to the $HOME prefix.
func xdgDownloadDirectory(config, home string) string {
	var path string
	for _, line := range strings.Split(config, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "XDG_DOWNLOAD_DIR" {
			continue
		}
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "\"") {
			continue
		}
		value = value[1:]
		var decoded strings.Builder
		switch {
		case strings.HasPrefix(value, "$HOME/"):
			decoded.WriteString(home)
			value = value[5:]
		case strings.HasPrefix(value, "$HOME\""):
			decoded.WriteString(home)
			value = value[5:]
		case strings.HasPrefix(value, "/"):
		default:
			continue
		}
		for i := 0; i < len(value); i++ {
			if value[i] == '"' {
				tail := strings.TrimSpace(value[i+1:])
				if tail == "" || strings.HasPrefix(tail, "#") {
					path = decoded.String()
				}
				break
			}
			if value[i] == '\\' {
				i++
				if i == len(value) {
					break
				}
			}
			decoded.WriteByte(value[i])
		}
	}
	return path
}
