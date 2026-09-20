package qqmusic

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf16"
)

func windowsAccount(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		data = []byte(decodeUTF16(data[2:]))
	}
	section := ""
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && section == "account" && strings.EqualFold(strings.TrimSpace(key), "uin") {
			uin := strings.Trim(strings.TrimSpace(value), "\"")
			if n, err := strconv.ParseUint(uin, 10, 64); err == nil && n != 0 {
				return uin, nil
			}
		}
	}
	return "", errors.New("未找到 QQ 音乐当前账号，请启动客户端并登录后重试")
}

func decodeUTF16(b []byte) string {
	chars := make([]uint16, len(b)/2)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(chars))
}

// Only accept a named credential paired with the current UIN in the same
// record. Never guess tokens from arbitrary base64-looking process data.
func windowsCredentials(data []byte, uin string) (Credentials, bool) {
	if c, ok := credentialsASCII(data, uin); ok {
		return c, true
	}
	for _, marker := range []string{"authst", "qqmusic_key="} {
		var wide []byte
		for _, c := range []byte(marker) {
			wide = append(wide, c, 0)
		}
		if bytes.Contains(data, wide) {
			for alignment := 0; alignment < 2; alignment++ {
				if c, ok := credentialsASCII([]byte(decodeUTF16(data[alignment:])), uin); ok {
					return c, true
				}
			}
		}
	}
	return Credentials{}, false
}

func credentialsASCII(data []byte, uin string) (Credentials, bool) {
	for cursor := 0; cursor < len(data); {
		rel := bytes.Index(data[cursor:], []byte(`"authst"`))
		if rel < 0 {
			break
		}
		pos := cursor + rel
		cursor = pos + 8
		start := max(0, pos-16384)
		prefix := data[start:pos]
		// The innermost object normally is the API's comm object. A bounded
		// walk also accepts an outer object containing a nested comm object.
		for tries := 0; tries < 16; tries++ {
			left := bytes.LastIndexByte(prefix, '{')
			if left < 0 {
				break
			}
			var record map[string]json.RawMessage
			chunk := data[start+left : min(len(data), pos+16384)]
			if json.NewDecoder(bytes.NewReader(chunk)).Decode(&record) == nil {
				value := func(name string) string {
					var s string
					if json.Unmarshal(record[name], &s) == nil {
						return s
					}
					return string(record[name])
				}
				login := value("tmeLoginType")
				if login == "" {
					login = "1"
				}
				if value("uin") == uin && validToken(value("authst")) && validLoginType(login) {
					return Credentials{uin, value("authst"), login}, true
				}
			}
			prefix = prefix[:left]
		}
	}
	for cursor := 0; cursor < len(data); {
		rel := bytes.Index(data[cursor:], []byte("qqmusic_key="))
		if rel < 0 {
			break
		}
		pos := cursor + rel
		cursor = pos + 12
		start, end := max(0, pos-4096), min(len(data), pos+8192)
		if i := bytes.LastIndexAny(data[start:pos], "\x00\r\n"); i >= 0 {
			start += i + 1
		}
		if i := bytes.IndexAny(data[pos:end], "\x00\r\n"); i >= 0 {
			end = pos + i
		}
		fields := map[string]string{}
		for _, part := range strings.Split(string(data[start:end]), ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok {
				fields[k] = v
			}
		}
		login := fields["tmeLoginType"]
		if login == "" {
			login = "1"
		}
		if fields["qqmusic_uin"] == uin && validToken(fields["qqmusic_key"]) && validLoginType(login) {
			return Credentials{uin, fields["qqmusic_key"], login}, true
		}
	}
	return Credentials{}, false
}

func validLoginType(s string) bool { return s == "1" || s == "2" || s == "3" }
func validToken(s string) bool {
	if len(s) < 16 || len(s) > 8192 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("+/-_=", c)) {
			return false
		}
	}
	return true
}
