package driver

import "testing"

var allOrders = []ByteOrder{BigEndian, LittleEndian, BigByteSwap, LittleByteSwap}

// TestByteOrderLayout pins the exact byte arrangement of each ordering for a
// 32-bit value, the case operators most often misconfigure.
func TestByteOrderLayout(t *testing.T) {
	raw := []byte{0x12, 0x34, 0x56, 0x78}
	want := map[ByteOrder]uint32{
		BigEndian:      0x12345678, // ABCD
		LittleEndian:   0x78563412, // DCBA
		BigByteSwap:    0x34127856, // BADC
		LittleByteSwap: 0x56781234, // CDAB
	}
	for order, exp := range want {
		v, err := Decode(raw, TypeUInt32, order)
		if err != nil {
			t.Fatalf("%s: %v", order, err)
		}
		if v.(uint32) != exp {
			t.Errorf("%s: got 0x%08X, want 0x%08X", order, v.(uint32), exp)
		}
	}
}

// TestRoundTripAllTypesAllOrders encodes then decodes a representative value of
// every type in every order and checks it survives.
func TestRoundTripAllTypesAllOrders(t *testing.T) {
	cases := []struct {
		dt   DataType
		val  float64
		want interface{}
	}{
		{TypeInt16, -1234, int16(-1234)},
		{TypeUInt16, 40000, uint16(40000)},
		{TypeInt32, -123456, int32(-123456)},
		{TypeUInt32, 3000000000, uint32(3000000000)},
		{TypeInt64, -1 << 40, int64(-1 << 40)},
		{TypeUInt64, 1 << 50, uint64(1 << 50)},
		{TypeFloat32, 3.5, float32(3.5)},
		{TypeFloat64, -2.71828, -2.71828},
	}
	for _, c := range cases {
		for _, order := range allOrders {
			raw, err := Encode(c.val, c.dt, order)
			if err != nil {
				t.Fatalf("%s/%s encode: %v", c.dt, order, err)
			}
			if len(raw) != c.dt.ByteWidth() {
				t.Errorf("%s: encoded %d bytes, want %d", c.dt, len(raw), c.dt.ByteWidth())
			}
			got, err := Decode(raw, c.dt, order)
			if err != nil {
				t.Fatalf("%s/%s decode: %v", c.dt, order, err)
			}
			if got != c.want {
				t.Errorf("%s/%s round-trip: got %v (%T), want %v", c.dt, order, got, got, c.want)
			}
		}
	}
}

// TestBool covers the boolean path (no byte order).
func TestBool(t *testing.T) {
	v, _ := Decode([]byte{0}, TypeBool, BigEndian)
	if v.(bool) {
		t.Error("0 -> false")
	}
	v, _ = Decode([]byte{1}, TypeBool, BigEndian)
	if !v.(bool) {
		t.Error("1 -> true")
	}
	b, _ := Encode(true, TypeBool, BigEndian)
	if len(b) != 1 || b[0] != 1 {
		t.Errorf("encode true = %v, want [1]", b)
	}
}

// TestDecodeShortInput errors rather than panicking on a truncated payload.
func TestDecodeShortInput(t *testing.T) {
	if _, err := Decode([]byte{0x01}, TypeInt32, BigEndian); err == nil {
		t.Error("expected error for short input")
	}
}

// TestRegisterCount maps types to Modbus register spans.
func TestRegisterCount(t *testing.T) {
	for dt, want := range map[DataType]int{
		TypeBool: 1, TypeInt16: 1, TypeUInt16: 1,
		TypeInt32: 2, TypeFloat32: 2, TypeUInt32: 2,
		TypeInt64: 4, TypeFloat64: 4, TypeUInt64: 4,
	} {
		if got := dt.RegisterCount(); got != want {
			t.Errorf("%s RegisterCount = %d, want %d", dt, got, want)
		}
	}
}
