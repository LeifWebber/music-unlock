// Synthetic native process for the Windows installer contract tests.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if strings.EqualFold(filepath.Base(os.Args[0]), "curl.exe") {
		if os.Getenv("UNMUS_TEST_DOWNLOAD_FAIL") == "1" {
			os.Exit(22)
		}
		for i, a := range os.Args[1:] {
			if a == "-o" && i+2 < len(os.Args) {
				data, err := os.ReadFile(os.Getenv("UNMUS_TEST_BOOTSTRAP_SOURCE"))
				if err != nil {
					panic(err)
				}
				if err = os.WriteFile(os.Args[i+2], data, 0600); err != nil {
					panic(err)
				}
				return
			}
		}
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
	if len(os.Args) > 1 && os.Args[1] == "fail" {
		os.Exit(7)
	}
}
