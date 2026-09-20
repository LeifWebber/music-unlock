package discovery

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"howett.net/plist"
)

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
}
func TestMacDefaultsAndConfiguredPath(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	music := filepath.Join(home, "Music")
	custom := filepath.Join(home, "custom downloads")
	for _, p := range []string{custom, filepath.Join(music, "网易云音乐"), filepath.Join(home, "Library/Containers/com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iQmc"), filepath.Join(home, "Library/Containers/com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iMusic")} {
		mkdir(t, p)
	}
	pref := filepath.Join(home, "Library/Preferences/com.tencent.QQMusicMac.plist")
	mkdir(t, filepath.Dir(pref))
	b, _ := plist.Marshal(map[string]any{"DownloadPath": custom, "AutoLoginUserInfo": []byte("not used for directory discovery")}, plist.BinaryFormat)
	if err := os.WriteFile(pref, b, 0600); err != nil {
		t.Fatal(err)
	}
	roots := Find(Environment{Home: home, Music: music, OS: "darwin"})
	if len(roots) != 3 || roots[0].Path != custom {
		t.Fatalf("roots=%+v", roots)
	}
	for _, root := range roots {
		if filepath.Base(root.Path) == "iMusic" {
			t.Fatal("playback cache included as a download directory")
		}
	}
}

func TestWindowsRedirectedMusicAndUTF16Settings(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	music := filepath.Join(home, "redirected Music")
	app := filepath.Join(home, "AppData")
	custom := filepath.Join(home, "custom QQ")
	for _, p := range []string{custom, filepath.Join(music, "VipSongsDownload"), filepath.Join(music, "CloudMusic")} {
		mkdir(t, p)
	}
	text := "[Download]\r\nSavePath=" + custom + "\r\n[Cache]\r\nPath=" + home + "\r\n"
	data := []byte{0xff, 0xfe}
	for _, c := range utf16.Encode([]rune(text)) {
		data = binary.LittleEndian.AppendUint16(data, c)
	}
	config := filepath.Join(app, "Tencent/QQMusic/QQMusicServiceConfig.ini")
	mkdir(t, filepath.Dir(config))
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	roots := Find(Environment{Home: home, Music: music, AppData: app, OS: "windows"})
	if len(roots) != 3 || roots[0].Path != custom {
		t.Fatalf("roots=%+v", roots)
	}
}

func TestNoHomeScanAndOverlappingDirectories(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	music := filepath.Join(home, "Music")
	mkdir(t, filepath.Join(music, "QQMusic", "VipSongsDownload"))
	app := filepath.Join(home, "AppData")
	config := filepath.Join(app, "Tencent/QQMusic/QQMusicConfig.ini")
	mkdir(t, filepath.Dir(config))
	data := "DownloadPath=" + home + "\nDownloadDir=" + filepath.Join(music, "QQMusic", "VipSongsDownload") + "\n"
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	roots := Find(Environment{Home: home, Music: music, AppData: app, OS: "windows"})
	if len(roots) != 1 || roots[0].Path != filepath.Join(music, "QQMusic") {
		t.Fatalf("roots=%+v", roots)
	}
}
