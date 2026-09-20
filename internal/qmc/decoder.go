package qmc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// KeyLookup receives the original media filename from musicex, or an empty string.
// It returns a base64 ekey, not a decrypted cipher key.
type KeyLookup func(mediaName string) string

type MissingKeyError struct{ Kind, MediaName, SongMID string }

func (e *MissingKeyError) Error() string {
	return fmt.Sprintf("%s 文件缺少歌曲密钥（媒体标识 %q）；请用 --key-file 提供该文件的 ekey", e.Kind, e.MediaName)
}

type streamCipher interface{ Decrypt([]byte, int) }

type Decoder struct {
	raw       io.Reader
	cipher    streamCipher
	offset    int
	Extension string
}

func (d *Decoder) Read(p []byte) (int, error) {
	n, err := d.raw.Read(p)
	if n > 0 {
		d.cipher.Decrypt(p[:n], d.offset)
		d.offset += n
	}
	return n, err
}

func Supported(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".qmc0", ".qmc3", ".qmcflac", ".qmcogg":
		return true
	}
	for _, base := range []string{".mflac", ".mgg"} {
		for _, suffix := range []string{"", "0", "1", "a", "h", "l", "m"} {
			if ext == base+suffix {
				return true
			}
		}
	}
	return false
}

// Open validates footer bounds and the decrypted audio header before any output
// is created. It never reads a client cache or makes network requests.
func Open(r io.ReadSeeker, lookup KeyLookup) (*Decoder, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if size < 16 {
		return nil, errors.New("文件过短或已损坏")
	}
	readAt := func(pos int64, n int64) ([]byte, error) {
		if pos < 0 || n < 0 || n > 1<<20 || pos+n > size {
			return nil, errors.New("文件尾标长度无效")
		}
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			return nil, err
		}
		b := make([]byte, int(n))
		_, err := io.ReadFull(r, b)
		return b, err
	}
	tail, err := readAt(size-16, 16)
	if err != nil {
		return nil, err
	}
	audioLen := size
	var ekey, kind, mediaName, songMID string
	switch {
	case bytes.Equal(tail[8:], []byte("musicex\x00")):
		kind = "musicex"
		tagSize := int64(binary.LittleEndian.Uint32(tail[:4]))
		if binary.LittleEndian.Uint32(tail[4:8]) != 1 || tagSize < 192 {
			return nil, errors.New("不支持的 musicex 版本或尾标长度")
		}
		footer, err := readAt(size-tagSize, tagSize)
		if err != nil {
			return nil, err
		}
		var chars []uint16
		for i := 0x48; i < 0xac; i += 2 {
			c := binary.LittleEndian.Uint16(footer[i : i+2])
			if c == 0 {
				break
			}
			chars = append(chars, c)
		}
		mediaName = string(utf16.Decode(chars))
		chars = nil
		for i := 0x0c; i < 0x48; i += 2 {
			c := binary.LittleEndian.Uint16(footer[i : i+2])
			if c == 0 {
				break
			}
			chars = append(chars, c)
		}
		songMID = string(utf16.Decode(chars))
		audioLen -= tagSize
	case string(tail[12:]) == "QTag" || string(tail[12:]) == "STag":
		kind = string(tail[12:])
		n := int64(binary.BigEndian.Uint32(tail[8:12]))
		meta, err := readAt(size-8-n, n)
		if err != nil {
			return nil, err
		}
		audioLen -= 8 + n
		if kind == "QTag" {
			parts := strings.Split(string(meta), ",")
			if len(parts) != 3 || parts[0] == "" {
				return nil, errors.New("QTag 元数据无效")
			}
			ekey = parts[0]
		}
	default:
		n := int64(binary.LittleEndian.Uint32(tail[12:]))
		if n > 0 && n <= 0xffff {
			key, err := readAt(size-4-n, n)
			if err != nil {
				return nil, err
			}
			ekey = strings.TrimRight(string(key), "\x00")
			audioLen -= 4 + n
		}
	}
	if audioLen < 16 {
		return nil, errors.New("文件不包含完整音频头")
	}
	if lookup != nil {
		if supplied := lookup(mediaName); supplied != "" {
			ekey = supplied
		}
	}
	if ekey == "" && (kind == "musicex" || kind == "STag") {
		return nil, &MissingKeyError{kind, mediaName, songMID}
	}
	var cipher streamCipher = newStaticCipher()
	if ekey != "" {
		if len(ekey) > 0xffff {
			return nil, errors.New("ekey 长度过大")
		}
		key, err := deriveKey([]byte(ekey))
		if err != nil {
			return nil, fmt.Errorf("无法解析 ekey: %w", err)
		}
		if len(key) > 300 {
			cipher, err = newRC4Cipher(key)
		} else {
			cipher, err = newMapCipher(key)
		}
		if err != nil {
			return nil, err
		}
	}
	header, err := readAt(0, 16)
	if err != nil {
		return nil, err
	}
	cipher.Decrypt(header, 0)
	ext := ""
	switch {
	case bytes.HasPrefix(header, []byte("fLaC")):
		ext = ".flac"
	case bytes.HasPrefix(header, []byte("OggS")):
		ext = ".ogg"
	case bytes.HasPrefix(header, []byte("ID3")), header[0] == 0xff && header[1]&0xe0 == 0xe0 && header[1]&6 != 0:
		ext = ".mp3"
	default:
		return nil, errors.New("解密后音频头无效：密钥错误、文件损坏或尚不支持的加密版本")
	}
	if ext == ".flac" && kind == "musicex" {
		audioLen, err = flacAudioLength(readAt, cipher, audioLen)
		if err != nil {
			return nil, err
		}
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &Decoder{raw: io.LimitReader(r, audioLen), cipher: cipher, Extension: ext}, nil
}
