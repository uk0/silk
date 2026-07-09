// Package driver connects silk's real-time tag database to industrial field
// devices over Modbus TCP and Siemens S7. It is UI-agnostic (no gui import):
// a device configuration — connection + a list of tag points — polls the device
// and writes each value into a core.TagDB, from where silk's existing bindings,
// animation, alarms, trends and codegen drive the screen. Read-write points also
// push tag edits back to the device.
//
// codec.go is the shared value layer: it converts between a device's raw
// register/byte payload and a Go value, for every common PLC data type and all
// four register/byte orderings.
package driver

import (
	"encoding/binary"
	"fmt"
	"math"
)

// DataType is a PLC variable type. Widths: Bool 1, 16-bit 2, 32-bit 4, 64-bit 8.
type DataType int

const (
	TypeBool DataType = iota
	TypeInt16
	TypeUInt16
	TypeInt32
	TypeUInt32
	TypeInt64
	TypeUInt64
	TypeFloat32
	TypeFloat64
)

// ByteWidth is the number of raw bytes a value of this type occupies.
func (dt DataType) ByteWidth() int {
	switch dt {
	case TypeBool:
		return 1
	case TypeInt16, TypeUInt16:
		return 2
	case TypeInt32, TypeUInt32, TypeFloat32:
		return 4
	case TypeInt64, TypeUInt64, TypeFloat64:
		return 8
	}
	return 0
}

// RegisterCount is how many 16-bit Modbus registers the type spans.
func (dt DataType) RegisterCount() int {
	w := dt.ByteWidth()
	if w < 2 {
		return 1
	}
	return w / 2
}

func (dt DataType) String() string {
	switch dt {
	case TypeBool:
		return "Bool"
	case TypeInt16:
		return "Int16"
	case TypeUInt16:
		return "UInt16"
	case TypeInt32:
		return "Int32"
	case TypeUInt32:
		return "UInt32"
	case TypeInt64:
		return "Int64"
	case TypeUInt64:
		return "UInt64"
	case TypeFloat32:
		return "Float32"
	case TypeFloat64:
		return "Float64"
	}
	return "?"
}

// ByteOrder selects how a multi-byte value's bytes are laid out across the
// device's registers. Modeled as two independent operations — reversing the
// order of the 16-bit words, and swapping the two bytes within each word — so
// the four industry orderings compose cleanly and generalise to 16/32/64-bit:
//
//	ABCD  BigEndian       高低      words as-is, bytes as-is (Modbus/S7 default)
//	DCBA  LittleEndian    低高      words reversed, bytes swapped
//	BADC  BigByteSwap     字节交换   words as-is, bytes swapped
//	CDAB  LittleByteSwap  字交换     words reversed, bytes as-is (common for 32-bit)
type ByteOrder int

const (
	BigEndian ByteOrder = iota
	LittleEndian
	BigByteSwap
	LittleByteSwap
)

func (o ByteOrder) String() string {
	switch o {
	case BigEndian:
		return "ABCD"
	case LittleEndian:
		return "DCBA"
	case BigByteSwap:
		return "BADC"
	case LittleByteSwap:
		return "CDAB"
	}
	return "?"
}

func (o ByteOrder) wordSwap() bool { return o == LittleEndian || o == LittleByteSwap }
func (o ByteOrder) byteSwap() bool { return o == LittleEndian || o == BigByteSwap }

// normalize converts between device byte order and canonical big-endian. It is
// its own inverse (byte-swap and word-swap are each involutions and operate on
// independent structure), so it serves both decode (device→big) and encode
// (big→device). Input length must be even for the multi-byte path.
func (o ByteOrder) normalize(raw []byte) []byte {
	out := make([]byte, len(raw))
	copy(out, raw)
	n := len(out)
	if o.byteSwap() {
		for i := 0; i+1 < n; i += 2 {
			out[i], out[i+1] = out[i+1], out[i]
		}
	}
	if o.wordSwap() {
		words := n / 2
		for i := 0; i < words/2; i++ {
			j := words - 1 - i
			out[i*2], out[j*2] = out[j*2], out[i*2]
			out[i*2+1], out[j*2+1] = out[j*2+1], out[i*2+1]
		}
	}
	return out
}

// Decode turns a raw device payload into a Go value of dt, honouring order.
// The returned concrete types are: bool, int16, uint16, int32, uint32, int64,
// uint64, float32, float64. It errors if raw is shorter than the type width.
func Decode(raw []byte, dt DataType, order ByteOrder) (interface{}, error) {
	w := dt.ByteWidth()
	if len(raw) < w {
		return nil, fmt.Errorf("driver: %s needs %d bytes, got %d", dt, w, len(raw))
	}
	if dt == TypeBool {
		return raw[0] != 0, nil
	}
	b := order.normalize(raw[:w]) // canonical big-endian
	switch dt {
	case TypeInt16:
		return int16(binary.BigEndian.Uint16(b)), nil
	case TypeUInt16:
		return binary.BigEndian.Uint16(b), nil
	case TypeInt32:
		return int32(binary.BigEndian.Uint32(b)), nil
	case TypeUInt32:
		return binary.BigEndian.Uint32(b), nil
	case TypeInt64:
		return int64(binary.BigEndian.Uint64(b)), nil
	case TypeUInt64:
		return binary.BigEndian.Uint64(b), nil
	case TypeFloat32:
		return math.Float32frombits(binary.BigEndian.Uint32(b)), nil
	case TypeFloat64:
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	}
	return nil, fmt.Errorf("driver: unsupported type %s", dt)
}

// Encode turns a Go value into a raw device payload of dt in order. v is coerced
// through float64 for numerics (matching how silk tags carry values), so an
// int32 point fed a tag's float64 value round-trips. Bool accepts bool or any
// nonzero numeric.
func Encode(v interface{}, dt DataType, order ByteOrder) ([]byte, error) {
	if dt == TypeBool {
		if asFloat(v) != 0 {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	}
	f := asFloat(v)
	b := make([]byte, dt.ByteWidth())
	switch dt {
	case TypeInt16:
		binary.BigEndian.PutUint16(b, uint16(int16(f)))
	case TypeUInt16:
		binary.BigEndian.PutUint16(b, uint16(f))
	case TypeInt32:
		binary.BigEndian.PutUint32(b, uint32(int32(f)))
	case TypeUInt32:
		binary.BigEndian.PutUint32(b, uint32(f))
	case TypeInt64:
		binary.BigEndian.PutUint64(b, uint64(int64(f)))
	case TypeUInt64:
		binary.BigEndian.PutUint64(b, uint64(f))
	case TypeFloat32:
		binary.BigEndian.PutUint32(b, math.Float32bits(float32(f)))
	case TypeFloat64:
		binary.BigEndian.PutUint64(b, math.Float64bits(f))
	default:
		return nil, fmt.Errorf("driver: unsupported type %s", dt)
	}
	return order.normalize(b), nil // big-endian -> device order (normalize is self-inverse)
}

// asFloat coerces the common numeric/bool concrete types to float64.
func asFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int16:
		return float64(x)
	case uint16:
		return float64(x)
	case int32:
		return float64(x)
	case uint32:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}
