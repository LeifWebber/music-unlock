package ncm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
)

func taggedAudio(r *bufio.Reader, ext string, meta Metadata, cover []byte) (io.Reader, error) {
	if meta.Title == "" && meta.Album == "" && len(meta.artists()) == 0 && len(cover) == 0 {
		return r, nil
	}
	if ext == ".flac" {
		return tagFLAC(r, meta, cover)
	}
	return tagMP3(r, meta, cover)
}

func picture(cover []byte) ([]byte, string) {
	config, format, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, ""
	}
	mime := "image/" + format
	var b bytes.Buffer
	put := func(v uint32) { _ = binary.Write(&b, binary.BigEndian, v) }
	put(3)
	put(uint32(len(mime)))
	b.WriteString(mime)
	put(0)
	put(uint32(config.Width))
	put(uint32(config.Height))
	put(0)
	put(0)
	put(uint32(len(cover)))
	b.Write(cover)
	return b.Bytes(), mime
}

func tagFLAC(r io.Reader, meta Metadata, cover []byte) (io.Reader, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, err
	}
	type block struct {
		kind byte
		data []byte
	}
	var blocks []block
	var comments []string
	vendor := "music-unlock"
	pic, _ := picture(cover)
	total := 0
	for {
		var h [4]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return nil, errors.New("FLAC 元数据头不完整")
		}
		n := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		total += n + 4
		if total > 32<<20 {
			return nil, errors.New("FLAC 元数据过大")
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, errors.New("FLAC 元数据不完整")
		}
		kind := h[0] & 0x7f
		if len(blocks) == 0 && (kind != 0 || n != 34) {
			return nil, errors.New("FLAC 缺少 STREAMINFO")
		}
		switch {
		case kind == 4:
			v, fields, err := readComments(data)
			if err != nil {
				return nil, err
			}
			vendor = v
			comments = append(comments, fields...)
		case kind == 6 && len(pic) > 0 && len(data) >= 4 && binary.BigEndian.Uint32(data) == 3:
			// Replace only front-cover pictures; preserve other artwork.
		default:
			blocks = append(blocks, block{kind, data})
		}
		if h[0]&0x80 != 0 {
			break
		}
	}
	values := map[string][]string{}
	if meta.Title != "" {
		values["TITLE"] = []string{meta.Title}
	}
	if meta.Album != "" {
		values["ALBUM"] = []string{meta.Album}
	}
	if a := meta.artists(); len(a) > 0 {
		values["ARTIST"] = a
	}
	var merged []string
	for _, c := range comments {
		key, _, _ := strings.Cut(c, "=")
		if _, replace := values[strings.ToUpper(key)]; !replace {
			merged = append(merged, c)
		}
	}
	for _, key := range []string{"TITLE", "ARTIST", "ALBUM"} {
		for _, v := range values[key] {
			merged = append(merged, key+"="+v)
		}
	}
	var tags bytes.Buffer
	put := func(s string) { _ = binary.Write(&tags, binary.LittleEndian, uint32(len(s))); tags.WriteString(s) }
	put(vendor)
	_ = binary.Write(&tags, binary.LittleEndian, uint32(len(merged)))
	for _, v := range merged {
		put(v)
	}
	blocks = append(blocks, block{4, tags.Bytes()})
	if len(pic) > 0 {
		blocks = append(blocks, block{6, pic})
	}
	var out bytes.Buffer
	out.WriteString("fLaC")
	for i, b := range blocks {
		if len(b.data) > 0xffffff {
			return nil, errors.New("FLAC 元数据块过大")
		}
		kind := b.kind
		if i == len(blocks)-1 {
			kind |= 0x80
		}
		out.Write([]byte{kind, byte(len(b.data) >> 16), byte(len(b.data) >> 8), byte(len(b.data))})
		out.Write(b.data)
	}
	return io.MultiReader(bytes.NewReader(out.Bytes()), r), nil
}

func readComments(data []byte) (string, []string, error) {
	r := bytes.NewReader(data)
	read := func() (string, error) {
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return "", err
		}
		if uint64(n) > uint64(r.Len()) {
			return "", io.ErrUnexpectedEOF
		}
		b := make([]byte, n)
		_, err := io.ReadFull(r, b)
		return string(b), err
	}
	vendor, err := read()
	if err != nil {
		return "", nil, errors.New("FLAC 标签无效")
	}
	var count uint32
	if binary.Read(r, binary.LittleEndian, &count) != nil || count > uint32(r.Len()/4) {
		return "", nil, errors.New("FLAC 标签数量无效")
	}
	fields := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		s, err := read()
		if err != nil {
			return "", nil, errors.New("FLAC 标签不完整")
		}
		fields = append(fields, s)
	}
	return vendor, fields, nil
}

func syncSize(n int) []byte {
	return []byte{byte(n>>21) & 127, byte(n>>14) & 127, byte(n>>7) & 127, byte(n) & 127}
}
func unsyncSize(p []byte) (int, bool) {
	if len(p) < 4 || p[0]|p[1]|p[2]|p[3] >= 128 {
		return 0, false
	}
	return int(p[0])<<21 | int(p[1])<<14 | int(p[2])<<7 | int(p[3]), true
}

func tagMP3(r *bufio.Reader, meta Metadata, cover []byte) (io.Reader, error) {
	type frame struct {
		id   string
		data []byte
	}
	var frames []frame
	head, _ := r.Peek(3)
	if string(head) == "ID3" {
		var h [10]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return nil, err
		}
		n, ok := unsyncSize(h[6:])
		if !ok || n > 16<<20 {
			return nil, errors.New("ID3 标签长度无效")
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		original := append(append([]byte(nil), h[:]...), data...)
		// Preserve uncommon ID3 versions/flags byte-for-byte rather than rewrite
		// frames with compression, unsynchronisation or extended headers.
		fallback := func() (io.Reader, error) { return io.MultiReader(bytes.NewReader(original), r), nil }
		if (h[3] != 3 && h[3] != 4) || h[5] != 0 {
			return fallback()
		}
		for pos := 0; pos < len(data) && data[pos] != 0; {
			if len(data)-pos < 10 {
				return fallback()
			}
			fh := data[pos : pos+10]
			n := int(binary.BigEndian.Uint32(fh[4:8]))
			valid := true
			if h[3] == 4 {
				n, valid = unsyncSize(fh[4:8])
			}
			if !valid || n > len(data)-pos-10 || fh[8] != 0 || fh[9] != 0 {
				return fallback()
			}
			frames = append(frames, frame{string(fh[:4]), data[pos+10 : pos+10+n]})
			pos += 10 + n
		}
	}
	values := map[string][]byte{}
	for _, v := range []struct{ id, value string }{{"TIT2", meta.Title}, {"TPE1", strings.Join(meta.artists(), "\x00")}, {"TALB", meta.Album}} {
		if v.value != "" {
			values[v.id] = append([]byte{3}, []byte(v.value)...)
		}
	}
	if _, mime := picture(cover); mime != "" {
		values["APIC"] = append(append([]byte{3}, []byte(mime+"\x00\x03\x00")...), cover...)
	}
	var data bytes.Buffer
	write := func(id string, b []byte) {
		data.WriteString(id)
		data.Write(syncSize(len(b)))
		data.Write([]byte{0, 0})
		data.Write(b)
	}
	for _, f := range frames {
		_, replace := values[f.id]
		if f.id == "APIC" && replace {
			// The MIME type is null-terminated in both ID3v2.3 and v2.4;
			// replace only a front cover, preserving other artwork.
			replace = false
			if len(f.data) > 1 {
				end := bytes.IndexByte(f.data[1:], 0) + 1
				replace = end > 0 && end+1 < len(f.data) && f.data[end+1] == 3
			}
		}
		if !replace {
			write(f.id, f.data)
		}
	}
	for _, id := range []string{"TIT2", "TPE1", "TALB", "APIC"} {
		if b, ok := values[id]; ok {
			write(id, b)
		}
	}
	if data.Len() > 0xfffffff {
		return nil, errors.New("ID3 标签过大")
	}
	head = append([]byte{'I', 'D', '3', 4, 0, 0}, syncSize(data.Len())...)
	return io.MultiReader(bytes.NewReader(head), bytes.NewReader(data.Bytes()), r), nil
}
