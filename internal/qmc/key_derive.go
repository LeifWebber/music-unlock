// Adapted from Unlock Music CLI v0.2.12 (MIT). See THIRD_PARTY_NOTICES.md.
package qmc

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math"

	"golang.org/x/crypto/tea"
)

func simpleMakeKey(salt byte, length int) []byte {
	keyBuf := make([]byte, length)
	for i := 0; i < length; i++ {
		tmp := math.Tan(float64(salt) + float64(i)*0.1)
		keyBuf[i] = byte(math.Abs(tmp) * 100.0)
	}
	return keyBuf
}

const rawKeyPrefixV2 = "QQMusic EncV2,Key:"

func deriveKey(rawKey []byte) ([]byte, error) {
	rawKeyDec := make([]byte, base64.StdEncoding.DecodedLen(len(rawKey)))
	n, err := base64.StdEncoding.Decode(rawKeyDec, rawKey)
	if err != nil {
		return nil, err
	}
	rawKeyDec = rawKeyDec[:n]

	if bytes.HasPrefix(rawKeyDec, []byte(rawKeyPrefixV2)) {
		rawKeyDec, err = deriveKeyV2(bytes.TrimPrefix(rawKeyDec, []byte(rawKeyPrefixV2)))
		if err != nil {
			return nil, fmt.Errorf("deriveKeyV2 failed: %w", err)
		}
	}
	return deriveKeyV1(rawKeyDec)
}

func deriveKeyV1(rawKeyDec []byte) ([]byte, error) {
	if len(rawKeyDec) < 16 {
		return nil, errors.New("key length is too short")
	}

	simpleKey := simpleMakeKey(106, 8)
	teaKey := make([]byte, 16)
	for i := 0; i < 8; i++ {
		teaKey[i<<1] = simpleKey[i]
		teaKey[i<<1+1] = rawKeyDec[i]
	}

	rs, err := decryptTencentTea(rawKeyDec[8:], teaKey)
	if err != nil {
		return nil, err
	}
	return append(rawKeyDec[:8], rs...), nil
}

var (
	deriveV2Key1 = []byte{
		0x33, 0x38, 0x36, 0x5A, 0x4A, 0x59, 0x21, 0x40,
		0x23, 0x2A, 0x24, 0x25, 0x5E, 0x26, 0x29, 0x28,
	}

	deriveV2Key2 = []byte{
		0x2A, 0x2A, 0x23, 0x21, 0x28, 0x23, 0x24, 0x25,
		0x26, 0x5E, 0x61, 0x31, 0x63, 0x5A, 0x2C, 0x54,
	}
)

func deriveKeyV2(raw []byte) ([]byte, error) {
	buf, err := decryptTencentTea(raw, deriveV2Key1)
	if err != nil {
		return nil, err
	}

	buf, err = decryptTencentTea(buf, deriveV2Key2)
	if err != nil {
		return nil, err
	}

	n, err := base64.StdEncoding.Decode(buf, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func decryptTencentTea(inBuf []byte, key []byte) ([]byte, error) {
	if len(inBuf) < 16 || len(inBuf)%8 != 0 {
		return nil, errors.New("invalid Tencent TEA ciphertext length")
	}
	blk, err := tea.NewCipherWithRounds(key, 32)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(inBuf))
	var previousBlock, previousCipher, block [8]byte
	for pos := 0; pos < len(inBuf); pos += 8 {
		for i := range block {
			block[i] = inBuf[pos+i] ^ previousBlock[i]
		}
		blk.Decrypt(block[:], block[:])
		for i := range block {
			plain[pos+i] = block[i] ^ previousCipher[i]
		}
		copy(previousCipher[:], inBuf[pos:pos+8])
		previousBlock = block
	}
	start := 1 + int(plain[0]&7) + 2
	end := len(plain) - 7
	if start > end {
		return nil, errors.New("invalid Tencent TEA padding")
	}
	for _, b := range plain[end:] {
		if b != 0 {
			return nil, errors.New("invalid Tencent TEA padding or key")
		}
	}
	return plain[start:end], nil
}
