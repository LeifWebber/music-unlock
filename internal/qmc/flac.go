package qmc

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Observed after the final FLAC frame in macOS QQ Music 11.7 musicex files.
// Never strip by length alone: require the exact marker, a valid final frame
// header/CRC, and a sample boundary matching STREAMINFO.
var qqFLACTrailer = []byte{0xf0, 0x00, 0xff, 0x0f, 0x44, 0x44, 0x40, 0x48, 0x46, 0x3c, 0x36, 0x0e, 0x55, 0xff, 0xf0}

func flacAudioLength(readAt func(int64, int64) ([]byte, error), cipher streamCipher, audioLen int64) (int64, error) {
	if audioLen < 42+int64(len(qqFLACTrailer)) {
		return audioLen, nil
	}
	tail, err := readAt(audioLen-int64(len(qqFLACTrailer)), int64(len(qqFLACTrailer)))
	if err != nil {
		return 0, err
	}
	cipher.Decrypt(tail, int(audioLen)-len(tail))
	if !bytes.Equal(tail, qqFLACTrailer) {
		return audioLen, nil
	}
	header, err := readAt(0, 42)
	if err != nil {
		return 0, err
	}
	cipher.Decrypt(header, 0)
	if header[4]&0x7f != 0 || !bytes.Equal(header[5:8], []byte{0, 0, 34}) {
		return 0, errors.New("FLAC STREAMINFO 无效")
	}
	info := header[8:]
	maxBlock := uint64(binary.BigEndian.Uint16(info[2:4]))
	totalSamples := binary.BigEndian.Uint64(info[10:18]) & ((1 << 36) - 1)
	end := audioLen - int64(len(tail))
	window := end - 42
	if window > 1<<20 {
		window = 1 << 20
	}
	frames, err := readAt(end-window, window)
	if err != nil {
		return 0, err
	}
	cipher.Decrypt(frames, int(end-window))
	for i := 0; i+8 < len(frames); i++ {
		if finalFLACFrame(frames[i:], maxBlock, totalSamples) {
			return end, nil
		}
	}
	return 0, errors.New("检测到 QQ FLAC 尾标，但无法验证最后一帧和总采样数")
}

func finalFLACFrame(b []byte, maxBlock, total uint64) bool {
	if len(b) < 8 || b[0] != 0xff || b[1]&0xfe != 0xf8 || b[3]&1 != 0 || total == 0 || maxBlock == 0 {
		return false
	}
	// FLAC uses an extended UTF-8 integer for the frame/sample number.
	pos := 4
	first := b[pos]
	pos++
	var number uint64
	if first < 0x80 {
		number = uint64(first)
	} else {
		count := 0
		for mask := byte(0x80); mask != 0 && first&mask != 0; mask >>= 1 {
			count++
		}
		if count < 2 || count > 7 || pos+count-1 > len(b) {
			return false
		}
		number = uint64(first & (0x7f >> count))
		for j := 1; j < count; j++ {
			v := b[pos]
			pos++
			if v&0xc0 != 0x80 {
				return false
			}
			number = number<<6 | uint64(v&0x3f)
		}
	}
	blockCode := b[2] >> 4
	var block uint64
	switch {
	case blockCode == 0:
		return false
	case blockCode == 1:
		block = 192
	case blockCode <= 5:
		block = 576 << uint(blockCode-2)
	case blockCode == 6:
		if pos >= len(b) {
			return false
		}
		block = uint64(b[pos]) + 1
		pos++
	case blockCode == 7:
		if pos+2 > len(b) {
			return false
		}
		block = uint64(binary.BigEndian.Uint16(b[pos:])) + 1
		pos += 2
	default:
		block = 256 << uint(blockCode-8)
	}
	switch b[2] & 15 {
	case 12:
		pos++
	case 13, 14:
		pos += 2
	case 15:
		return false
	}
	if pos+3 > len(b) {
		return false
	}
	var crc8 byte
	for _, v := range b[:pos] {
		crc8 ^= v
		for j := 0; j < 8; j++ {
			if crc8&0x80 != 0 {
				crc8 = crc8<<1 ^ 7
			} else {
				crc8 <<= 1
			}
		}
	}
	if crc8 != b[pos] {
		return false
	}
	if number > total || block > total {
		return false
	}
	if b[1]&1 == 0 {
		if number > (total-block)/maxBlock {
			return false
		}
		number *= maxBlock
	}
	if number+block != total {
		return false
	}
	var crc uint16
	for _, v := range b {
		crc ^= uint16(v) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc == 0
}
