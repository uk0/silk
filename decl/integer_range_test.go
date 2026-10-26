package decl_test

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

func TestSDKIntegerRangeOrderAndRoundTrip(t *testing.T) {
	cases := [][3]int{{-100, 100, -42}, {200, 300, 250}, {-300, -200, -250}, {math.MinInt, math.MaxInt, math.MaxInt - 1}, {math.MinInt, math.MaxInt, math.MinInt + 1}, {-17, -17, -17}}
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for ci, values := range cases {
		for _, mode := range []string{"literal", "persisted", "alias"} {
			for oi, order := range orders {
				t.Run(fmt.Sprintf("case%d/%s/order%d", ci, mode, oi), func(t *testing.T) {
					names := [3]string{"最小值", "最大值", "值"}
					if mode == "alias" {
						names = [3]string{"min", "MAX", "Value"}
					}
					attrs := []decl.Attr{}
					for _, i := range order {
						var v interface{} = values[i]
						if mode == "persisted" {
							v = decl.Persisted{Source: strconv.Itoa(values[i])}
						}
						attrs = append(attrs, decl.P(names[i], v))
					}
					node := decl.New("gui.SpinBox", attrs...)
					for round := 0; round < 3; round++ {
						obj, err := node.Build()
						if err != nil {
							t.Fatal(err)
						}
						s := obj.(*gui.SpinBox)
						if got := ([3]int{s.Min(), s.Max(), s.Value()}); got != values {
							t.Fatalf("round%d: got%v want%v", round, got, values)
						}
						node, err = decl.FromTDoc(decl.ToTDoc(node))
						if err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func TestSDKIntegerRangeInvalidAndPartial(t *testing.T) {
	for _, bad := range []interface{}{1.5, math.NaN(), math.Inf(1), uint64(math.MaxUint64), "9223372036854775808", "-9223372036854775809", "12junk", true, nil} {
		for _, mode := range []string{"literal", "persisted", "alias"} {
			t.Run(fmt.Sprintf("%s/%T/%v", mode, bad, bad), func(t *testing.T) {
				names := [3]string{"最小值", "最大值", "值"}
				var v interface{} = bad
				if mode == "alias" {
					names = [3]string{"min", "max", "value"}
				}
				if mode == "persisted" {
					v = decl.Persisted{Source: fmt.Sprint(bad)}
				}
				obj, err := decl.New("gui.SpinBox", decl.P(names[0], v), decl.P(names[1], v), decl.P(names[2], v)).Build()
				if err != nil {
					t.Fatal(err)
				}
				s := obj.(*gui.SpinBox)
				if s.Min() != 0 || s.Max() != 99 || s.Value() != 0 {
					t.Fatalf("invalid value mutated range: %d %d %d", s.Min(), s.Max(), s.Value())
				}
			})
		}
	}
	for _, tc := range []struct {
		attrs []decl.Attr
		want  [3]int
	}{
		{[]decl.Attr{decl.P("value", -42), decl.P("min", -100)}, [3]int{-100, 99, -42}},
		{[]decl.Attr{decl.P("value", 250), decl.P("max", 300)}, [3]int{0, 300, 250}},
		{[]decl.Attr{decl.P("value", 999), decl.P("min", 200), decl.P("max", 300)}, [3]int{200, 300, 300}},
		{[]decl.Attr{decl.P("value", float64(250)), decl.P("min", float64(200)), decl.P("max", float64(300))}, [3]int{200, 300, 250}},
	} {
		obj, err := decl.New("gui.SpinBox", tc.attrs...).Build()
		if err != nil {
			t.Fatal(err)
		}
		s := obj.(*gui.SpinBox)
		if got := ([3]int{s.Min(), s.Max(), s.Value()}); got != tc.want {
			t.Errorf("partial: got%v want%v", got, tc.want)
		}
	}
}
