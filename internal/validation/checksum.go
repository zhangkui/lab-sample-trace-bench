package validation

import "hash/crc32"

func Checksum(payload []byte) uint32               { return crc32.ChecksumIEEE(payload) }
func Matches(payload []byte, expected uint32) bool { return Checksum(payload) == expected }
