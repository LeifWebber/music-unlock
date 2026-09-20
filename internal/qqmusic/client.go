// Package qqmusic provides keys for files that need the local QQ Music session. Session credentials remain
// in memory and are only sent to QQ Music's fixed HTTPS endpoint.
package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"howett.net/plist"
)

const endpoint = "https://u.y.qq.com/cgi-bin/musicu.fcg"

type Credentials struct{ UIN, AuthST, LoginType string }

// LoadMacSession is used only when offline key lookup could not decrypt a file.
func LoadMacSession() (Credentials, error) {
	if runtime.GOOS != "darwin" {
		return Credentials{}, errors.New("此登录信息读取器仅支持 macOS")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Credentials{}, err
	}
	return loadMacSessionFrom(home)
}

func loadMacSessionFrom(home string) (Credentials, error) {
	// Direct-download builds use the regular preferences directory; sandboxed
	// builds use their container. Only fall back when a file does not exist,
	// never silently use a second account after an authentication/parse error.
	paths := []string{
		"Library/Preferences/com.tencent.QQMusicMac.plist",
		"Library/Containers/com.tencent.QQMusicMac/Data/Library/Preferences/com.tencent.QQMusicMac.plist",
	}
	for _, path := range paths {
		b, err := readSessionFile(filepath.Join(home, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Credentials{}, errors.New("无法读取本机 QQ 音乐登录信息，请检查文件权限")
		}
		return parseSession(b)
	}
	return Credentials{}, errors.New("未找到本机 QQ 音乐登录信息，请先在客户端登录")
}

func readSessionFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 16<<20))
}

func parseSession(b []byte) (Credentials, error) {
	var outer map[string]any
	if _, err := plist.Unmarshal(b, &outer); err != nil {
		return Credentials{}, errors.New("QQ 音乐偏好文件格式无效")
	}
	archive, ok := outer["AutoLoginUserInfo"].([]byte)
	if !ok {
		return Credentials{}, errors.New("没有可用的 QQ 音乐自动登录信息")
	}
	var inner map[string]any
	if _, err := plist.Unmarshal(archive, &inner); err != nil {
		return Credentials{}, errors.New("QQ 音乐登录记录格式无效")
	}
	objects, ok := inner["$objects"].([]any)
	if !ok {
		return Credentials{}, errors.New("QQ 音乐登录记录缺少对象表")
	}
	resolve := func(value any) any {
		if uid, ok := value.(plist.UID); ok {
			if uint64(uid) >= uint64(len(objects)) {
				return nil
			}
			return objects[int(uid)]
		}
		return value
	}
	text := func(value any) string {
		switch v := resolve(value).(type) {
		case string:
			return v
		case uint64:
			return strconv.FormatUint(v, 10)
		case int64:
			return strconv.FormatInt(v, 10)
		default:
			return ""
		}
	}
	top, ok := inner["$top"].(map[string]any)
	if !ok {
		return Credentials{}, errors.New("QQ 音乐登录记录缺少根对象")
	}
	root, ok := resolve(top["root"]).(map[string]any)
	if !ok {
		return Credentials{}, errors.New("QQ 音乐登录记录根对象无效")
	}
	active := text(root["nCurrUseId"])
	if active == "" || active == "0" {
		return Credentials{}, errors.New("QQ 音乐未指定当前登录账号")
	}
	users, ok := resolve(root["userArray"]).(map[string]any)
	if !ok {
		return Credentials{}, errors.New("QQ 音乐登录记录缺少账号列表")
	}
	refs, ok := users["NS.objects"].([]any)
	if !ok {
		return Credentials{}, errors.New("QQ 音乐账号列表格式无效")
	}
	for _, ref := range refs {
		fields, ok := resolve(ref).(map[string]any)
		if !ok {
			continue
		}
		auth := text(fields["strAuthst"])
		if auth == "" {
			continue
		}
		uin := text(fields["nUserId"])
		if uin == "" || uin == "0" {
			uin = text(fields["strUserAccount"])
		}
		if uin != active {
			continue
		}
		login := text(fields["loginType"])
		if _, err := strconv.ParseUint(uin, 10, 64); err != nil || uin == "0" {
			continue
		}
		if login != "1" && login != "2" && login != "3" {
			return Credentials{}, errors.New("不支持的 QQ 音乐登录类型")
		}
		return Credentials{uin, auth, login}, nil
	}
	return Credentials{}, errors.New("没有找到有效登录凭据，请在 QQ 音乐客户端重新登录后重试")
}

type Client struct {
	credentials Credentials
	http        *http.Client
}

func New(credentials Credentials) *Client {
	return &Client{credentials, &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("禁止认证请求重定向") },
	}}
}

func (c *Client) Key(ctx context.Context, mediaName, songMID string) (string, error) {
	if mediaName == "" || songMID == "" || strings.ContainsAny(mediaName, "/\\") {
		return "", errors.New("musicex 缺少有效歌曲标识")
	}
	platform := "20"
	if runtime.GOOS == "windows" {
		platform = "27"
	}
	body := map[string]any{
		"comm": map[string]any{"authst": c.credentials.AuthST, "uin": c.credentials.UIN, "tmeLoginType": c.credentials.LoginType, "ct": "19", "cv": "1859"},
		"req_1": map[string]any{"module": "music.vkey.GetEVkey", "method": "CgiGetEVkey", "param": map[string]any{
			"filename": []string{mediaName}, "songmid": []string{songMID}, "songtype": []int{1}, "guid": "10000", "uin": c.credentials.UIN, "loginflag": 1, "platform": platform, "ctx": 1,
		}},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", errors.New("无法构建密钥请求")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("QQ 音乐密钥请求失败，请检查网络或稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("QQ 音乐密钥接口 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code    int `json:"code"`
		Request struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					Filename string `json:"filename"`
					SongMID  string `json:"songmid"`
					EKey     string `json:"ekey"`
					Result   int    `json:"result"`
				} `json:"midurlinfo"`
			} `json:"data"`
		} `json:"req_1"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", errors.New("QQ 音乐密钥接口返回无效数据")
	}
	if result.Code != 0 || result.Request.Code != 0 {
		return "", fmt.Errorf("QQ 音乐拒绝密钥请求（%d/%d）；请检查客户端登录状态和歌曲权限", result.Code, result.Request.Code)
	}
	for _, item := range result.Request.Data.Items {
		if item.Filename == mediaName && (item.SongMID == "" || item.SongMID == songMID) && item.Result == 0 && item.EKey != "" {
			return item.EKey, nil
		}
	}
	return "", errors.New("QQ 音乐未返回该歌曲的密钥，请确认当前账号仍有歌曲权限或重新登录")
}
