package discovery

import "golang.org/x/sys/windows"

func downloadsDirectory() (string, error) {
	// Follow user redirection without requiring or creating the folder yet.
	return windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DONT_VERIFY)
}
