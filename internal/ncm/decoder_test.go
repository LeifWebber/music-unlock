package ncm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"
)

func TestMP3PreservesOtherArtworkAndUnusualTags(t *testing.T) {
	var frames bytes.Buffer
	for _, item := range []struct {
		id   string
		data []byte
	}{
		{"APIC", []byte("\x00image/png\x00\x03\x00old front")},
		{"APIC", []byte("\x00image/png\x00\x04\x00back cover")},
		{"TXXX", []byte("\x00custom\x00keep this")},
	} {
		frames.WriteString(item.id)
		_ = binary.Write(&frames, binary.BigEndian, uint32(len(item.data)))
		frames.Write([]byte{0, 0})
		frames.Write(item.data)
	}
	original := append([]byte{'I', 'D', '3', 3, 0, 0}, syncSize(frames.Len())...)
	original = append(original, frames.Bytes()...)
	audio := mp3Audio(t, fixture(t, "tone.mp3"))
	original = append(original, audio...)
	reader, err := tagMP3(bufio.NewReader(bytes.NewReader(original)), Metadata{Title: "New title"}, fixture(t, "cover.png"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mp3Audio(t, got), audio) || bytes.Contains(got, []byte("old front")) || !bytes.Contains(got, []byte("back cover")) || !bytes.Contains(got, []byte("keep this")) || bytes.Count(got, []byte("APIC")) != 2 {
		t.Fatal("replacing front cover changed audio or unrelated tags")
	}
	// A tag with an extended header is deliberately preserved intact.
	original[5] = 0x40
	reader, err = tagMP3(bufio.NewReader(bytes.NewReader(original)), Metadata{Title: "New title"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("unusual ID3 tag was rewritten")
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func decode(t *testing.T, b []byte, chunk int) (*Decoder, []byte) {
	t.Helper()
	d, err := Open(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	buf := make([]byte, chunk)
	for {
		n, e := d.Read(buf)
		out.Write(buf[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	return d, out.Bytes()
}

func flacParts(t *testing.T, b []byte) (map[byte][][]byte, []byte) {
	t.Helper()
	if len(b) < 4 || string(b[:4]) != "fLaC" {
		t.Fatal("not FLAC")
	}
	pos := 4
	blocks := map[byte][][]byte{}
	for {
		if len(b)-pos < 4 {
			t.Fatal("truncated FLAC")
		}
		h := b[pos : pos+4]
		n := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
		pos += 4
		if n > len(b)-pos {
			t.Fatal("truncated block")
		}
		blocks[h[0]&127] = append(blocks[h[0]&127], b[pos:pos+n])
		pos += n
		if h[0]&128 != 0 {
			break
		}
	}
	return blocks, b[pos:]
}
func mp3Audio(t *testing.T, b []byte) []byte {
	t.Helper()
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		n, ok := unsyncSize(b[6:10])
		if !ok || n > len(b)-10 {
			t.Fatal("invalid ID3")
		}
		return b[n+10:]
	}
	return b
}

func TestIndependentFixtures(t *testing.T) {
	want, err := os.ReadFile("../qmc/testdata/tone.flac")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 127, 65536} {
		d, got := decode(t, fixture(t, "no-meta.ncm"), size)
		if d.Extension != ".flac" || !bytes.Equal(got, want) {
			t.Fatalf("metadata-free NCM/chunk %d mismatch", size)
		}
	}
	d, got := decode(t, fixture(t, "tagged-flac.ncm"), 127)
	if d.Extension != ".flac" || d.Metadata.Title != "测试曲目" || len(d.Metadata.artists()) != 2 {
		t.Fatal("format/metadata mismatch")
	}
	blocks, audio := flacParts(t, got)
	original, expected := flacParts(t, want)
	if !bytes.Equal(audio, expected) || !bytes.Equal(blocks[0][0], original[0][0]) {
		t.Fatal("FLAC audio/STREAMINFO changed")
	}
	if len(blocks[4]) != 1 || !bytes.Contains(blocks[4][0], []byte("ARTIST=测试作者")) || len(blocks[6]) != 1 || !bytes.Contains(blocks[6][0], fixture(t, "cover.png")) {
		t.Fatal("tags/cover missing")
	}
	d, got = decode(t, fixture(t, "tagged-mp3.ncm"), 71)
	if d.Extension != ".mp3" || !bytes.Equal(mp3Audio(t, got), mp3Audio(t, fixture(t, "tone.mp3"))) {
		t.Fatal("MP3 frames changed")
	}
	if !bytes.Contains(got, []byte("测试曲目")) || !bytes.Contains(got, fixture(t, "cover.png")) {
		t.Fatal("MP3 tags/cover missing")
	}
}

func TestMalformedLengthsAndPadding(t *testing.T) {
	valid := fixture(t, "tagged-flac.ncm")
	keyLen := int(binary.LittleEndian.Uint32(valid[10:14]))
	metaPos := 14 + keyLen
	metaLen := int(binary.LittleEndian.Uint32(valid[metaPos : metaPos+4]))
	coverPos := metaPos + 4 + metaLen + 5
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"empty key", func(b []byte) { binary.LittleEndian.PutUint32(b[10:14], 0) }},
		{"huge key", func(b []byte) { binary.LittleEndian.PutUint32(b[10:14], 0xffffffff) }},
		{"bad AES block", func(b []byte) { binary.LittleEndian.PutUint32(b[10:14], 3) }},
		{"huge metadata", func(b []byte) { binary.LittleEndian.PutUint32(b[metaPos:metaPos+4], 0xffffffff) }},
		{"cover outside allocation", func(b []byte) { binary.LittleEndian.PutUint32(b[coverPos+4:coverPos+8], 0xffffffff) }},
		{"allocation past EOF", func(b []byte) { binary.LittleEndian.PutUint32(b[coverPos:coverPos+4], 0xffffffff) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := append([]byte(nil), valid...)
			tc.mutate(b)
			if _, err := Open(bytes.NewReader(b)); err == nil {
				t.Fatal("accepted malformed file")
			}
		})
	}
	for n := 0; n < coverPos+8; n++ {
		if _, err := Open(bytes.NewReader(valid[:n])); err == nil {
			t.Fatalf("accepted truncated header at %d", n)
		}
	}
	if _, err := decryptECB(make([]byte, 16), coreKey); err == nil {
		t.Fatal("accepted invalid padding")
	}
}

func FuzzOpen(f *testing.F) {
	for _, name := range []string{"no-meta.ncm", "tagged-flac.ncm", "tagged-mp3.ncm"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte("CTENFDAM"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		if d, err := Open(bytes.NewReader(b)); err == nil {
			_, _ = io.Copy(io.Discard, d)
		}
	})
}
