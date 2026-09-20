package qqmusic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsMemoryHelper(t *testing.T) {
	if os.Getenv("MUSIC_UNLOCK_TEST_PROCESS") != "1" {
		return
	}
	record := []byte(`{"uin":"123456789","authst":"synthetic-session-for-tests-only","tmeLoginType":"3"}`)
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	runtime.KeepAlive(record)
}

func TestReadOwnedProcess(t *testing.T) {
	if os.Getenv("MUSIC_UNLOCK_TEST_PROCESS") == "1" {
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsMemoryHelper$")
	cmd.Env = append(os.Environ(), "MUSIC_UNLOCK_TEST_PROCESS=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); cmd.Process.Kill(); cmd.Wait() }()
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, ok := scanQQMusic(ctx, process, user.User.Sid, "123456789")
	if !ok || got.AuthST != "synthetic-session-for-tests-only" {
		t.Fatal("failed to read synthetic session from owned process")
	}
}
