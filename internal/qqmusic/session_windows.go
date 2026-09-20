package qqmusic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// LoadSession reads only QQ Music records for the current Windows user and
// current QQ Music UIN. No injection, elevation or process writes are used.
func LoadSession(ctx context.Context) (Credentials, error) {
	base := os.Getenv("APPDATA")
	if base == "" {
		return Credentials{}, errors.New("无法定位 QQ 音乐配置目录，请在正常的 Windows 用户会话中运行")
	}
	base = filepath.Join(base, "Tencent", "QQMusic")
	config, err := readSessionFile(filepath.Join(base, "QQMusicServiceConfig.ini"))
	if err != nil {
		return Credentials{}, errors.New("未找到 QQ 音乐配置，请先安装、启动 QQ 音乐并登录")
	}
	uin, err := windowsAccount(config)
	if err != nil {
		return Credentials{}, err
	}
	for _, name := range []string{"SetCookie.dat", "_SetCookie.dat"} {
		if err := ctx.Err(); err != nil {
			return Credentials{}, err
		}
		if data, err := readSessionFile(filepath.Join(base, name)); err == nil {
			if c, ok := windowsCredentials(data, uin); ok {
				return c, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return Credentials{}, errors.New("无法读取 QQ 音乐进程信息")
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return Credentials{}, errors.New("无法确定当前 Windows 用户")
	}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if ctx.Err() != nil {
			break
		}
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "QQMusic.exe") {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, entry.ProcessID)
		if err != nil {
			continue
		}
		c, ok := scanQQMusic(ctx, process, user.User.Sid, uin)
		windows.CloseHandle(process)
		if ok {
			return c, nil
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return Credentials{}, ctx.Err()
	}
	return Credentials{}, errors.New("无法读取 QQ 音乐当前登录态；请保持 QQ 音乐运行并登录，且与本工具使用相同 Windows 用户和权限后重试（此客户端版本也可能尚不兼容）")
}

func scanQQMusic(ctx context.Context, process windows.Handle, currentSID *windows.SID, uin string) (Credentials, bool) {
	var token windows.Token
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token) != nil {
		return Credentials{}, false
	}
	owner, err := token.GetTokenUser()
	token.Close()
	if err != nil || !owner.User.Sid.Equals(currentSID) {
		return Credentials{}, false
	}
	const chunkSize = 1 << 20
	const overlap = 64 << 10
	buffer := make([]byte, chunkSize)
	var total uintptr
	for address := uintptr(0); ctx.Err() == nil && total < 512<<20; {
		var region windows.MemoryBasicInformation
		if windows.VirtualQueryEx(process, address, &region, unsafe.Sizeof(region)) != nil {
			break
		}
		next := region.BaseAddress + region.RegionSize
		if next <= address {
			break
		}
		protection := region.Protect & 0xff
		readable := protection == windows.PAGE_READONLY || protection == windows.PAGE_READWRITE || protection == windows.PAGE_WRITECOPY || protection == windows.PAGE_EXECUTE_READ || protection == windows.PAGE_EXECUTE_READWRITE || protection == windows.PAGE_EXECUTE_WRITECOPY
		if region.State == windows.MEM_COMMIT && region.Protect&windows.PAGE_GUARD == 0 && readable && (region.Type == 0x20000 || region.Type == 0x1000000) {
			for pos := region.BaseAddress; pos < next && ctx.Err() == nil && total < 512<<20; {
				size := min(uintptr(len(buffer)), next-pos)
				var read uintptr
				_ = windows.ReadProcessMemory(process, pos, &buffer[0], size, &read)
				total += size
				if read > 0 {
					if c, ok := windowsCredentials(buffer[:read], uin); ok {
						return c, true
					}
				}
				if size <= overlap {
					break
				}
				pos += size - overlap
			}
		}
		address = next
	}
	return Credentials{}, false
}
