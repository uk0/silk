package core

import (
	"testing"
)

func TestUuid(t *testing.T) {
	a := make([]Uuid, 0, 1000)
	for i := 0; i < 1000; i++ {
		tmp := NewUuid()
		a = append(a, tmp)
		b, _ := ParseUuid(a[i].String())
		if a[i] != b {
			t.Fatal(a[i], "!=", b)
		}
		for j := 0; j < i; j++ {
			if a[i] == a[j] {
				t.Fatal("duplicate UUID: ", a[i])
			}
		}
	}
}
