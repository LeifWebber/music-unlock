package qmc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"
	"unicode/utf16"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".bin")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecoderGolden(t *testing.T) {
	for _, name := range []string{"qmc0_static", "mflac_map", "mgg_map", "mflac_rc4", "mflac0_rc4"} {
		t.Run(name, func(t *testing.T) {
			raw := append(fixture(t, name+"_raw"), fixture(t, name+"_suffix")...)
			for _, chunk := range []int{1, 127, 5120, 65536} {
				d, err := Open(bytes.NewReader(raw), nil)
				if err != nil {
					t.Fatal(err)
				}
				var output bytes.Buffer
				buf := make([]byte, chunk)
				for {
					n, err := d.Read(buf)
					output.Write(buf[:n])
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(output.Bytes(), fixture(t, name+"_target")) {
					t.Fatalf("golden mismatch with chunk size %d", chunk)
				}
			}
		})
	}
}

func musicexFooter() []byte {
	b := make([]byte, 192)
	for _, field := range []struct {
		offset int
		value  string
	}{{12, "test-mid"}, {72, "F0M000test.mflac"}} {
		for i, c := range utf16.Encode([]rune(field.value)) {
			binary.LittleEndian.PutUint16(b[field.offset+i*2:], c)
		}
	}
	binary.LittleEndian.PutUint32(b[176:], 192)
	binary.LittleEndian.PutUint32(b[180:], 1)
	copy(b[184:], "musicex\x00")
	return b
}

func TestMusicexExternalKey(t *testing.T) {
	raw := append(fixture(t, "mflac_map_raw"), musicexFooter()...)
	_, err := Open(bytes.NewReader(raw), nil)
	var missing *MissingKeyError
	if !errors.As(err, &missing) || missing.MediaName != "F0M000test.mflac" || missing.SongMID != "test-mid" {
		t.Fatalf("unexpected missing-key error: %v", err)
	}
	d, err := Open(bytes.NewReader(raw), func(name string) string {
		if name != missing.MediaName {
			t.Fatal(name)
		}
		return string(fixture(t, "mflac_map_key_raw"))
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, fixture(t, "mflac_map_target")) {
		t.Fatal("musicex footer leaked into output or audio changed")
	}
}

func TestMalformedInputs(t *testing.T) {
	footer := musicexFooter()
	binary.LittleEndian.PutUint32(footer[176:], 0xffffffff)
	qtag := append(bytes.Repeat([]byte{0}, 24), []byte{255, 255, 255, 255, 'Q', 'T', 'a', 'g'}...)
	for _, input := range [][]byte{nil, []byte("short"), bytes.Repeat([]byte{0}, 32), footer, qtag} {
		if _, err := Open(bytes.NewReader(input), nil); err == nil {
			t.Fatal("accepted malformed input")
		}
	}
	raw := append(fixture(t, "mflac_map_raw"), fixture(t, "mflac_map_suffix")...)
	if _, err := Open(bytes.NewReader(raw), func(string) string { return "invalid-key" }); err == nil {
		t.Fatal("accepted wrong key")
	}
}

func TestMusicexFLACTrailer(t *testing.T) {
	// Locally generated 1-second sine wave; no private music is in this fixture.
	plain, err := os.ReadFile("testdata/tone.flac")
	if err != nil {
		t.Fatal(err)
	}
	ekey := fixture(t, "mflac_map_key_raw")
	key, err := deriveKey(ekey)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name            string
		marker, corrupt bool
	}{{"ordinary", false, false}, {"QQ trailer", true, false}, {"corrupt last frame", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.Clone(plain)
			if tc.corrupt {
				payload[len(payload)-1] ^= 1
			}
			if tc.marker {
				payload = append(payload, qqFLACTrailer...)
			}
			cipher, err := newMapCipher(key)
			if err != nil {
				t.Fatal(err)
			}
			cipher.Decrypt(payload, 0)
			raw := append(payload, musicexFooter()...)
			d, err := Open(bytes.NewReader(raw), func(string) string { return string(ekey) })
			if tc.corrupt {
				if err == nil {
					t.Fatal("trimmed marker without validating last frame")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(d)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, plain) {
				t.Fatal("audio changed or trailing marker remains")
			}
		})
	}
}

func FuzzDecoder(f *testing.F) {
	f.Add([]byte("short"))
	f.Add(musicexFooter())
	f.Add(bytes.Repeat([]byte{255}, 64))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Open(bytes.NewReader(b), nil) })
}

func FuzzDeriveKey(f *testing.F) {
	f.Add([]byte("AAAA"))
	f.Add([]byte("UVFNdXNpYyBFbmNWMixLZXk6"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) <= 65535 {
			_, _ = deriveKey(b)
		}
	})
}
