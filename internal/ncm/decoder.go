// Package ncm decodes NetEase Cloud Music containers entirely offline.
// Cipher/layout adapted from Unlock Music CLI v0.2.12 (MIT); see notices.
package ncm

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var coreKey = []byte("hzHRAmso5kInbaxW")
var metaKey = []byte("#14ljk_!\\]&0U<'(")

type Metadata struct {
	Title  string          `json:"musicName"`
	Album  string          `json:"album"`
	Artist json.RawMessage `json:"artist"`
}

type Decoder struct {
	io.Reader
	Extension string
	Metadata  Metadata
	Cover     []byte
}

type cipherReader struct {
	r   io.Reader
	key [256]byte
	pos uint64
}

func (c *cipherReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	for i := 0; i < n; i++ {
		p[i] ^= c.key[byte(c.pos)]
		c.pos++
	}
	return n, err
}

func Open(r io.ReadSeeker) (*Decoder, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var header [10]byte
	if _, err = io.ReadFull(r, header[:]); err != nil || string(header[:8]) != "CTENFDAM" {
		return nil, errors.New("NCM 文件头无效或文件不完整")
	}
	readBlock := func(limit uint32) ([]byte, error) {
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return nil, err
		}
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		if n > limit || int64(n) > size-pos {
			return nil, errors.New("NCM 数据块长度无效")
		}
		b := make([]byte, n)
		_, err = io.ReadFull(r, b)
		return b, err
	}
	encrypted, err := readBlock(64 << 10)
	if err != nil {
		return nil, fmt.Errorf("NCM 密钥块: %w", err)
	}
	for i := range encrypted {
		encrypted[i] ^= 0x64
	}
	key, err := decryptECB(encrypted, coreKey)
	if err != nil || !bytes.HasPrefix(key, []byte("neteasecloudmusic")) || len(key) <= 17 {
		return nil, errors.New("NCM 歌曲密钥无效")
	}
	key = key[17:]
	metadata, err := readBlock(4 << 20)
	if err != nil {
		return nil, fmt.Errorf("NCM 元数据块: %w", err)
	}
	d := new(Decoder)
	if len(metadata) > 0 {
		if d.Metadata, err = parseMetadata(metadata); err != nil {
			return nil, err
		}
	}
	// CRC32 and one reserved byte, followed by allocated and used cover sizes.
	var trailer [13]byte
	if _, err = io.ReadFull(r, trailer[:]); err != nil {
		return nil, errors.New("NCM 封面头不完整")
	}
	allocated := binary.LittleEndian.Uint32(trailer[5:9])
	used := binary.LittleEndian.Uint32(trailer[9:13])
	pos, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if used > allocated || used > 16<<20 || int64(allocated) >= size-pos {
		return nil, errors.New("NCM 封面长度无效或音频缺失")
	}
	d.Cover = make([]byte, used)
	if _, err = io.ReadFull(r, d.Cover); err != nil {
		return nil, err
	}
	if _, err = r.Seek(pos+int64(allocated), io.SeekStart); err != nil {
		return nil, err
	}
	var box [256]byte
	for i := range box {
		box[i] = byte(i)
	}
	var j byte
	for i := range box {
		j += box[i] + key[i%len(key)]
		box[i], box[j] = box[j], box[i]
	}
	cipher := &cipherReader{r: r}
	for i := range cipher.key {
		a := byte(i + 1)
		b := box[a]
		cipher.key[i] = box[b+box[a+b]]
	}
	audio := bufio.NewReader(cipher)
	head, err := audio.Peek(4)
	if err != nil {
		return nil, errors.New("NCM 音频不完整")
	}
	switch {
	case bytes.HasPrefix(head, []byte("fLaC")):
		d.Extension = ".flac"
	case bytes.HasPrefix(head, []byte("ID3")), head[0] == 0xff && head[1]&0xe0 == 0xe0 && head[1]&6 != 0:
		d.Extension = ".mp3"
	default:
		return nil, errors.New("NCM 解密后不是受支持的 FLAC/MP3 音频")
	}
	d.Reader, err = taggedAudio(audio, d.Extension, d.Metadata, d.Cover)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func decryptECB(data, key []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("无效 AES 块长度")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	for i := 0; i < len(data); i += aes.BlockSize {
		block.Decrypt(out[i:i+aes.BlockSize], data[i:i+aes.BlockSize])
	}
	n := int(out[len(out)-1])
	if n == 0 || n > aes.BlockSize {
		return nil, errors.New("无效 AES 填充")
	}
	for _, b := range out[len(out)-n:] {
		if int(b) != n {
			return nil, errors.New("无效 AES 填充")
		}
	}
	return out[:len(out)-n], nil
}

func parseMetadata(raw []byte) (Metadata, error) {
	var meta Metadata
	for i := range raw {
		raw[i] ^= 0x63
	}
	prefix := []byte("163 key(Don't modify):")
	if !bytes.HasPrefix(raw, prefix) {
		return meta, errors.New("NCM 元数据前缀无效")
	}
	data, err := base64.StdEncoding.DecodeString(string(raw[len(prefix):]))
	if err != nil {
		return meta, errors.New("NCM 元数据编码无效")
	}
	data, err = decryptECB(data, metaKey)
	if err != nil {
		return meta, errors.New("NCM 元数据解密失败")
	}
	kind, body, ok := bytes.Cut(data, []byte(":"))
	if !ok {
		return meta, errors.New("NCM 元数据缺少类型")
	}
	switch string(kind) {
	case "music":
		err = json.Unmarshal(body, &meta)
	case "dj":
		var dj struct {
			MainMusic   Metadata `json:"mainMusic"`
			ProgramName string   `json:"programName"`
			DJName      string   `json:"djName"`
			RadioName   string   `json:"radioName"`
		}
		err = json.Unmarshal(body, &dj)
		meta = dj.MainMusic
		if dj.ProgramName != "" {
			meta.Title = dj.ProgramName
		}
		if dj.RadioName != "" {
			meta.Album = dj.RadioName
		}
		if dj.DJName != "" {
			meta.Artist, _ = json.Marshal(dj.DJName)
		}
	default:
		// Metadata types may evolve; audio recognition never trusts their format field.
		if !json.Valid(body) {
			err = errors.New("invalid JSON")
		}
	}
	if err != nil {
		return meta, errors.New("NCM 元数据 JSON 无效")
	}
	return meta, nil
}

func (m Metadata) artists() []string {
	var single string
	if json.Unmarshal(m.Artist, &single) == nil {
		if single != "" {
			return []string{single}
		}
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(m.Artist, &list) != nil {
		return nil
	}
	var names []string
	for _, item := range list {
		var row []json.RawMessage
		var name string
		if json.Unmarshal(item, &name) != nil && json.Unmarshal(item, &row) == nil && len(row) > 0 {
			_ = json.Unmarshal(row[0], &name)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
