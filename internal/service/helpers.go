package service

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"

	"XCLing/internal/store"
)

func dataSubdir(name string) string {
	base, err := store.DataDir()
	if err != nil || base == "" {
		return name
	}
	return filepath.Join(base, name)
}

func newUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	buf := make([]byte, 36)
	hex.Encode(buf[0:8], value[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], value[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], value[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], value[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], value[10:16])
	return string(buf)
}
