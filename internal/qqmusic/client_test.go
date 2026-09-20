package qqmusic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"howett.net/plist"
)

func TestParseSession(t *testing.T) {
	inner, err := plist.Marshal(map[string]any{
		"$top": map[string]any{"root": plist.UID(1)},
		"$objects": []any{"$null",
			map[string]any{"nCurrUseId": uint64(123456), "userArray": plist.UID(2)},
			map[string]any{"NS.objects": []any{plist.UID(3), plist.UID(4)}},
			map[string]any{"nUserId": uint64(111111), "strAuthst": "stale-session", "loginType": uint64(1)},
			map[string]any{"nUserId": uint64(123456), "strAuthst": plist.UID(5), "loginType": uint64(3)},
			"fake-session"}}, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := plist.Marshal(map[string]any{"AutoLoginUserInfo": inner}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{
		"Library/Preferences/com.tencent.QQMusicMac.plist",
		"Library/Containers/com.tencent.QQMusicMac/Data/Library/Preferences/com.tencent.QQMusicMac.plist",
	} {
		home := t.TempDir()
		path := filepath.Join(home, location)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, outer, 0600); err != nil {
			t.Fatal(err)
		}
		c, err := loadMacSessionFrom(home)
		if err != nil || c.UIN != "123456" {
			t.Fatal("session location/current user", err)
		}
	}
	c, err := parseSession(outer)
	if err != nil || c.UIN != "123456" || c.AuthST != "fake-session" || c.LoginType != "3" {
		t.Fatal("cannot parse synthetic session", err)
	}
	if _, err := parseSession([]byte("bad")); err == nil {
		t.Fatal("accepted malformed plist")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestKeyRequestIsScopedAndRedacted(t *testing.T) {
	c := New(Credentials{"123456", "secret-session", "3"})
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != endpoint {
			t.Fatal("wrong endpoint")
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		request := body["req_1"].(map[string]any)
		param := request["param"].(map[string]any)
		if param["filename"].([]any)[0] != "file.mflac" || param["songmid"].([]any)[0] != "mid" {
			t.Fatal("wrong song")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"req_1":{"code":0,"data":{"midurlinfo":[{"filename":"file.mflac","songmid":"mid","ekey":"test-key","result":0}]}}}`))}, nil
	})
	key, err := c.Key(context.Background(), "file.mflac", "mid")
	if err != nil || key != "test-key" {
		t.Fatal(err)
	}
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":1,"message":"secret-session"}`))}, nil
	})
	_, err = c.Key(context.Background(), "file.mflac", "mid")
	if err == nil || strings.Contains(err.Error(), "secret-session") {
		t.Fatal("error leaks response body")
	}
	if err := c.http.CheckRedirect(nil, nil); err == nil {
		t.Fatal("redirect allowed")
	}
}
