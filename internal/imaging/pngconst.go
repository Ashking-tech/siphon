package imaging

import "hash/crc32"

var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func crc32Checksum(b []byte) uint32 { return crc32.ChecksumIEEE(b) }
