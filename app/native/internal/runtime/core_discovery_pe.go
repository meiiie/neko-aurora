package runtime

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

func discoveryPEPreflight(file io.ReaderAt, size int64) bool {
	var dos [64]byte
	if size < 64 {
		return false
	}
	if _, err := file.ReadAt(dos[:], 0); err != nil || string(dos[:2]) != "MZ" {
		return false
	}
	offset := int64(binary.LittleEndian.Uint32(dos[60:]))
	if offset < 64 || offset > 1<<20 || offset+24 > size {
		return false
	}
	var header [24]byte
	if _, err := file.ReadAt(header[:], offset); err != nil || string(header[:4]) != "PE\x00\x00" {
		return false
	}
	sections := binary.LittleEndian.Uint16(header[6:])
	optional := binary.LittleEndian.Uint16(header[20:])
	symbols := int64(binary.LittleEndian.Uint32(header[16:]))
	pointer := int64(binary.LittleEndian.Uint32(header[12:]))
	if sections < 1 || sections > 96 || optional != 240 || offset+24+int64(optional)+int64(sections)*40 > size || symbols > 65536 {
		return false
	}
	if symbols > 0 {
		end := pointer + symbols*18
		if pointer < offset || end+4 > size {
			return false
		}
		var length [4]byte
		if _, err := file.ReadAt(length[:], end); err != nil {
			return false
		}
		n := int64(binary.LittleEndian.Uint32(length[:]))
		if n < 4 || n > 1<<20 || end+n > size {
			return false
		}
	}
	return true
}
func discoveryPEVersion(file io.ReaderAt, binaryFile *pe.File) string {
	optional, ok := binaryFile.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || optional.NumberOfRvaAndSizes < 3 {
		return ""
	}
	directory := optional.DataDirectory[2]
	if directory.VirtualAddress == 0 || directory.Size < 24 || directory.Size > 2<<20 {
		return ""
	}
	locate := func(rva, length uint32) (int64, bool) {
		for _, section := range binaryFile.Sections {
			start := section.VirtualAddress
			if rva >= start && uint64(rva-start)+uint64(length) <= uint64(section.Size) {
				return int64(section.Offset) + int64(rva-start), true
			}
		}
		return 0, false
	}
	root, ok := locate(directory.VirtualAddress, directory.Size)
	if !ok {
		return ""
	}
	child := func(offset uint32, typ *uint32) (uint32, bool) {
		if uint64(offset)+16 > uint64(directory.Size) {
			return 0, false
		}
		var header [16]byte
		if _, err := file.ReadAt(header[:], root+int64(offset)); err != nil {
			return 0, false
		}
		count := int(binary.LittleEndian.Uint16(header[12:])) + int(binary.LittleEndian.Uint16(header[14:]))
		if count < 1 || count > 128 || uint64(offset)+16+uint64(count)*8 > uint64(directory.Size) {
			return 0, false
		}
		for i := 0; i < count; i++ {
			var entry [8]byte
			if _, err := file.ReadAt(entry[:], root+int64(offset)+16+int64(i*8)); err != nil {
				return 0, false
			}
			id := binary.LittleEndian.Uint32(entry[:])
			target := binary.LittleEndian.Uint32(entry[4:])
			if typ == nil || id == *typ {
				return target, true
			}
		}
		return 0, false
	}
	versionType := uint32(16)
	target, ok := child(0, &versionType)
	if !ok || target&0x80000000 == 0 {
		return ""
	}
	target, ok = child(target&0x7fffffff, nil)
	if !ok || target&0x80000000 == 0 {
		return ""
	}
	target, ok = child(target&0x7fffffff, nil)
	if !ok || target&0x80000000 != 0 || uint64(target)+16 > uint64(directory.Size) {
		return ""
	}
	var data [16]byte
	if _, err := file.ReadAt(data[:], root+int64(target)); err != nil {
		return ""
	}
	rva := binary.LittleEndian.Uint32(data[:])
	size := binary.LittleEndian.Uint32(data[4:])
	if size < 92 || size > 65536 {
		return ""
	}
	position, ok := locate(rva, size)
	if !ok {
		return ""
	}
	bytes := make([]byte, size)
	if _, err := file.ReadAt(bytes, position); err != nil {
		return ""
	}
	length := int(binary.LittleEndian.Uint16(bytes))
	valueLength := int(binary.LittleEndian.Uint16(bytes[2:]))
	if length > len(bytes) || length < 92 || valueLength < 52 || binary.LittleEndian.Uint16(bytes[4:]) != 0 {
		return ""
	}
	key := []uint16{}
	offset := 6
	for offset+2 <= length && len(key) < 32 {
		c := binary.LittleEndian.Uint16(bytes[offset:])
		offset += 2
		if c == 0 {
			break
		}
		key = append(key, c)
	}
	if string(utf16.Decode(key)) != "VS_VERSION_INFO" {
		return ""
	}
	offset = (offset + 3) &^ 3
	if offset+52 > length || binary.LittleEndian.Uint32(bytes[offset:]) != 0xfeef04bd || binary.LittleEndian.Uint32(bytes[offset+4:]) != 0x00010000 {
		return ""
	}
	ms := binary.LittleEndian.Uint32(bytes[offset+8:])
	ls := binary.LittleEndian.Uint32(bytes[offset+12:])
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}
