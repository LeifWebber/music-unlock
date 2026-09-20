package qqmusic

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func wide(s string) []byte {
	var b []byte
	for _, c := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

func TestWindowsAccount(t *testing.T) {
	for _, input := range [][]byte{[]byte("[Other]\nUin=111\n[Account]\nUin=12345\n"), append([]byte{255, 254}, wide("[Account]\r\nUin=12345\r\n")...)} {
		got, err := windowsAccount(input)
		if err != nil || got != "12345" {
			t.Fatal(got, err)
		}
	}
	if _, err := windowsAccount([]byte("[History]\nUin=12345")); err == nil {
		t.Fatal("used a non-current account")
	}
}

func TestWindowsCredentials(t *testing.T) {
	json := `{"authst":"test-auth-token-123456","uin":"12345","tmeLoginType":"3"}`
	for _, data := range [][]byte{[]byte("noise\x00" + json + "\x00noise"), wide(json), append([]byte{0}, wide(json)...), []byte("qqmusic_key=test-auth-token-123456; qqmusic_uin=12345; tmeLoginType=3\x00")} {
		got, ok := windowsCredentials(data, "12345")
		if !ok || got.AuthST != "test-auth-token-123456" || got.LoginType != "3" {
			t.Fatal("valid record not found")
		}
		if _, ok := windowsCredentials(data, "98765"); ok {
			t.Fatal("used another account")
		}
	}
	for _, data := range []string{`{"authst":"test-auth-token-123456"}`, `{"authst":"test-auth-token-123456","uin":"12345","tmeLoginType":"9"}`, "randomBase64Token1234567890="} {
		if _, ok := windowsCredentials([]byte(data), "12345"); ok {
			t.Fatal("accepted unbound/guessed token")
		}
	}
}
