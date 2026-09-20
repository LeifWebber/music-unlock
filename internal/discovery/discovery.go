// Package discovery locates bounded client download directories, never whole disks.
package discovery

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"howett.net/plist"
)

type Source struct{ Path, Platform string }
type Environment struct{ Home, Music, AppData, OS string }

func Downloads() ([]Source, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return Find(Environment{home, musicDirectory(home), os.Getenv("APPDATA"), runtime.GOOS}), nil
}

func Find(env Environment) []Source {
	canonicalHome, _ := filepath.EvalSymlinks(env.Home)
	var candidates []Source
	add := func(platform string, paths ...string) {
		for _, p := range paths {
			if p != "" {
				candidates = append(candidates, Source{p, platform})
			}
		}
	}
	if env.OS == "darwin" {
		for _, path := range []string{
			"Library/Preferences/com.tencent.QQMusicMac.plist",
			"Library/Containers/com.tencent.QQMusicMac/Data/Library/Preferences/com.tencent.QQMusicMac.plist",
		} {
			if data, err := readConfig(filepath.Join(env.Home, path)); err == nil {
				var pref map[string]any
				if _, err := plist.Unmarshal(data, &pref); err == nil {
					for _, key := range []string{"DownloadPath", "downloadPath", "downloadDirectory", "DownloadDirectory", "musicDownloadPath"} {
						if value, ok := pref[key].(string); ok {
							add("QQ音乐", value)
						}
					}
				}
			}
		}
	}
	if env.OS == "windows" && env.AppData != "" {
		base := filepath.Join(env.AppData, "Tencent", "QQMusic")
		for _, name := range []string{"QQMusicServiceConfig.ini", "QQMusicConfig.ini"} {
			if data, err := readConfig(filepath.Join(base, name)); err == nil {
				add("QQ音乐", windowsDownloadPaths(data)...)
			}
		}
	}
	add("QQ音乐", filepath.Join(env.Music, "VipSongsDownload"), filepath.Join(env.Music, "QQ音乐"), filepath.Join(env.Music, "QQMusic"))
	add("网易云音乐", filepath.Join(env.Music, "网易云音乐"), filepath.Join(env.Music, "CloudMusic"))
	if env.OS == "darwin" {
		for _, base := range []string{"Library/Containers/com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac", "Library/Application Support/QQMusicMac"} {
			// iQmc contains downloaded encrypted songs; iMusic is playback cache.
			add("QQ音乐", filepath.Join(env.Home, base, "iQmc"))
		}
	}
	var roots []Source
	for _, s := range candidates {
		if !filepath.IsAbs(s.Path) {
			continue
		}
		path, err := filepath.EvalSymlinks(s.Path)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		// A malformed setting must not turn automatic mode into a home/disk scan.
		if path == filepath.Clean(env.Home) || path == canonicalHome || path == filepath.Dir(path) {
			continue
		}
		s.Path = path
		skip := false
		for _, old := range roots {
			if contains(old.Path, path) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		kept := roots[:0]
		for _, old := range roots {
			if !contains(path, old.Path) {
				kept = append(kept, old)
			}
		}
		roots = append(kept, s)
	}
	return roots
}

func contains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readConfig(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 4<<20))
}

func windowsDownloadPaths(data []byte) []string {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		words := make([]uint16, (len(data)-2)/2)
		for i := range words {
			words[i] = binary.LittleEndian.Uint16(data[2+i*2:])
		}
		data = []byte(string(utf16.Decode(words)))
	}
	var paths []string
	section := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "downloadpath" || key == "downloadsavepath" || key == "downloaddir" || key == "songdownloadpath" || (section == "download" && (key == "savepath" || key == "path")) {
			value = strings.Trim(strings.TrimSpace(value), "\"")
			if value != "" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}
