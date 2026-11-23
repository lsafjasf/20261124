// Package ebml 提供 EBML(Extensible Binary Meta Language)的基础编解码,
// 仅依赖标准库,供 Matroska 封装层使用。
package ebml

import (
	"errors"
	"fmt"
	"io"
)

// UnknownSize 表示 EBML 的"未知长度"(数据位全 1)。
const UnknownSize int64 = -1

// ErrInvalidVINT 表示变长整数编码损坏。
var ErrInvalidVINT = errors.New("ebml: 无效的 VINT 编码")

// ReadVINT 从 r 读取一个变长整数。
// 当 keepMarker 为 true 时保留长度标记位(用于元素 ID),否则剥离(用于数据长度)。
// 返回解析出的值与编码占用的字节数。
func ReadVINT(r io.Reader, keepMarker bool) (uint64, int, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, 0, err
	}
	b := first[0]
	if b == 0 {
		return 0, 0, ErrInvalidVINT
	}
	length := 1
	mask := byte(0x80)
	for mask != 0 && b&mask == 0 {
		length++
		mask >>= 1
	}
	if length > 8 {
		return 0, 0, ErrInvalidVINT
	}
	value := uint64(b)
	if !keepMarker {
		value &^= uint64(mask)
	}
	if length > 1 {
		rest := make([]byte, length-1)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, 0, err
		}
		for _, rb := range rest {
			value = value<<8 | uint64(rb)
		}
	}
	return value, length, nil
}

// ReadElementHeader 读取一个元素头,返回元素 ID、数据长度(可能为 UnknownSize)
// 以及头部本身占用的字节数。
func ReadElementHeader(r io.Reader) (id uint32, size int64, headerLen int64, err error) {
	idVal, idLen, err := ReadVINT(r, true)
	if err != nil {
		return 0, 0, 0, err
	}
	if idLen > 4 {
		return 0, 0, 0, fmt.Errorf("ebml: 元素 ID 过长 (%d 字节)", idLen)
	}
	sizeVal, sizeLen, err := ReadVINT(r, false)
	if err != nil {
		return 0, 0, 0, err
	}
	size = int64(sizeVal)
	// 数据位全 1 表示未知长度。
	if sizeVal == (uint64(1)<<(7*uint(sizeLen)))-1 {
		size = UnknownSize
	}
	return uint32(idVal), size, int64(idLen + sizeLen), nil
}

// vintLength 返回编码 value 所需的最短 VINT 字节数(数据位为 7*length)。
func vintLength(value uint64) int {
	for length := 1; length <= 8; length++ {
		if value < (uint64(1)<<(7*uint(length)))-1 {
			return length
		}
	}
	return 8
}

// AppendID 追加一个元素 ID(保留标记位)。
func AppendID(dst []byte, id uint32) []byte {
	switch {
	case id < 0x100:
		return append(dst, byte(id))
	case id < 0x10000:
		return append(dst, byte(id>>8), byte(id))
	case id < 0x1000000:
		return append(dst, byte(id>>16), byte(id>>8), byte(id))
	default:
		return append(dst, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	}
}

// AppendSize 以最短形式追加一个数据长度。
func AppendSize(dst []byte, size uint64) []byte {
	return append(dst, encodeVINT(size, vintLength(size))...)
}

// AppendUnknownSize 追加 8 字节的未知长度标记。
func AppendUnknownSize(dst []byte) []byte {
	return append(dst, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
}

// encodeVINT 以固定长度编码 VINT(自动设置标记位)。
func encodeVINT(value uint64, length int) []byte {
	out := make([]byte, length)
	v := value | uint64(1)<<(7*uint(length))
	for i := length - 1; i >= 0; i-- {
		out[i] = byte(v)
		v >>= 8
	}
	return out
}

// AppendHeader 追加完整元素头(ID + 最短长度)。
func AppendHeader(dst []byte, id uint32, size uint64) []byte {
	dst = AppendID(dst, id)
	return AppendSize(dst, size)
}

// AppendUint 追加一个无符号整数元素(最短编码)。
func AppendUint(dst []byte, id uint32, value uint64) []byte {
	var data [8]byte
	n := 0
	v := value
	for {
		data[7-n] = byte(v)
		n++
		v >>= 8
		if v == 0 {
			break
		}
	}
	payload := data[8-n:]
	dst = AppendHeader(dst, id, uint64(len(payload)))
	return append(dst, payload...)
}

// AppendFloat64 追加一个 8 字节浮点元素。
func AppendFloat64(dst []byte, id uint32, value float64) []byte {
	bits := float64bits(value)
	dst = AppendHeader(dst, id, 8)
	for i := 7; i >= 0; i-- {
		dst = append(dst, byte(bits>>(8*uint(i))))
	}
	return dst
}

// AppendString 追加一个 UTF-8 字符串元素。
func AppendString(dst []byte, id uint32, value string) []byte {
	dst = AppendHeader(dst, id, uint64(len(value)))
	return append(dst, value...)
}

// AppendBytes 追加一个二进制元素。
func AppendBytes(dst []byte, id uint32, payload []byte) []byte {
	dst = AppendHeader(dst, id, uint64(len(payload)))
	return append(dst, payload...)
}

// AppendMaster 追加主元素头,返回追加负载后的回填函数:
// 调用方在负载写完后调用 finish 完成长度回填。
// 长度位固定预留 8 字节,保证任何长度都能回填。
func AppendMaster(dst []byte, id uint32) (out []byte, finish func([]byte) []byte) {
	out = AppendID(dst, id)
	placeholder := len(out)
	out = append(out, make([]byte, 8)...)
	start := len(out)
	return out, func(buf []byte) []byte {
		size := uint64(len(buf) - start)
		copy(buf[placeholder:placeholder+8], encodeVINT(size, 8))
		return buf
	}
}
