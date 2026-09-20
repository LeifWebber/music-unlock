package discovery

import "testing"

func TestXDGDownloadDirectory(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"localized", `XDG_DOWNLOAD_DIR="$HOME/下载"`, "/home/user/下载"},
		{"redirected", `XDG_DOWNLOAD_DIR="/mnt/data/Downloads"`, "/mnt/data/Downloads"},
		{"disabled", `XDG_DOWNLOAD_DIR="$HOME"`, "/home/user"},
		{"escaped", `XDG_DOWNLOAD_DIR="$HOME/With \"quotes\" and \\backslash"`, "/home/user/With \"quotes\" and \\backslash"},
		{"comment", "#XDG_DOWNLOAD_DIR=\"/ignored\"\n XDG_DOWNLOAD_DIR = \"/chosen\" # downloads", "/chosen"},
		{"unrelated", `XDG_MUSIC_DIR="$HOME/Music"`, ""},
		{"relative", `XDG_DOWNLOAD_DIR="downloads"`, ""},
		{"unterminated", `XDG_DOWNLOAD_DIR="$HOME/broken`, ""},
		{"shell expression", `XDG_DOWNLOAD_DIR="$(touch /tmp/unmus-never-run)"`, ""},
		{"last setting", "XDG_DOWNLOAD_DIR=\"/one\"\nXDG_DOWNLOAD_DIR=\"/two\"", "/two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := xdgDownloadDirectory(tc.config, "/home/user"); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
