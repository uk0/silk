package point_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sync"
	"testing"

	"github.com/uk0/silk/point"
)

func TestCompareNumbersAgainstExactRationalOrder(t *testing.T) {
	ints := []int64{math.MinInt64, math.MinInt64 + 1, -(1 << 53) - 1, -(1 << 53), -2, -1, 0, 1, 2, 1 << 53, 1<<53 + 1, math.MaxInt64 - 1, math.MaxInt64}
	floats := []float64{-math.MaxFloat64, math.Nextafter(-0x1p63, math.Inf(-1)), -0x1p63, math.Nextafter(-0x1p63, 0), -0x1p53, -1.5, -1, -math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0, math.SmallestNonzeroFloat64, 0.5, 1, 1.5, 0x1p53, math.Nextafter(0x1p63, 0), 0x1p63, math.MaxFloat64}
	check := func(i int64, f float64) {
		t.Helper()
		want := new(big.Rat).SetInt64(i).Cmp(new(big.Rat).SetFloat64(f))
		got, err := point.CompareNumbers(i, f)
		reverse, reverseErr := point.CompareNumbers(f, i)
		if err != nil || reverseErr != nil || got != want || reverse != -want {
			t.Fatalf("int %d vs %.17g: %d/%d, %v/%v; exact %d", i, f, got, reverse, err, reverseErr, want)
		}
	}
	for _, i := range ints {
		for _, f := range floats {
			check(i, f)
		}
	}
	rng := rand.New(rand.NewSource(0x51_1c))
	for i := 0; i < 5000; i++ {
		f := math.Float64frombits(rng.Uint64())
		if !math.IsInf(f, 0) && !math.IsNaN(f) {
			check(int64(rng.Uint64()), f)
		}
	}
	for _, bad := range []any{math.NaN(), math.Inf(1), math.Inf(-1), nil, struct{}{}, json.Number("9223372036854775808")} {
		if _, err := point.CompareNumbers(int64(1), bad); err == nil {
			t.Fatalf("invalid numeric comparison accepted: %#v", bad)
		}
	}
}

func TestExpressionCheckedIntegerArithmeticAgainstBigInt(t *testing.T) {
	limits := []int64{math.MinInt64, math.MinInt64 + 1, -1 << 53, -3, -1, 0, 1, 3, 1 << 53, math.MaxInt64 - 1, math.MaxInt64}
	check := func(a, b int64, op string) {
		t.Helper()
		x, y := big.NewInt(a), big.NewInt(b)
		var want *big.Int
		switch op {
		case "+":
			want = new(big.Int).Add(x, y)
		case "-":
			want = new(big.Int).Sub(x, y)
		case "*":
			want = new(big.Int).Mul(x, y)
		}
		e, err := point.Compile(fmt.Sprintf("x %s (%d)", op, b))
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.Evaluate(a)
		if !want.IsInt64() {
			if err == nil {
				t.Fatalf("overflow accepted: %d %s %d = %v", a, op, b, got)
			}
		} else if err != nil || got != want.Int64() {
			t.Fatalf("%d %s %d: %v (%T), %v; exact %s", a, op, b, got, got, err, want)
		}
	}
	for _, a := range limits {
		for _, b := range limits {
			for _, op := range []string{"+", "-", "*"} {
				check(a, b, op)
			}
		}
	}
	rng := rand.New(rand.NewSource(53))
	for i := 0; i < 1000; i++ {
		a, b := int64(rng.Uint64()), int64(rng.Uint64())
		for _, op := range []string{"+", "-", "*"} {
			check(a, b, op)
		}
	}
}

func TestExpressionIntegerErrorsAndShortCircuit(t *testing.T) {
	for _, tc := range []struct {
		source string
		input  any
	}{
		{"x+1", int64(math.MaxInt64)}, {"x-1", int64(math.MinInt64)}, {"-x", int64(math.MinInt64)},
		{"x*-1", int64(math.MinInt64)}, {"x<<1", int64(math.MaxInt64)}, {"x<<63", int64(1)},
		{"x<<64", int64(0)}, {"x>>-1", int64(2)}, {"x%0", int64(2)}, {"x/1", int64(math.MaxInt64)},
		{"x&1", 9.0}, {"x&1", "9"}, {"x&1", true}, {"x&1", json.Number("9.0")}, {"^x", 1.5},
	} {
		e, err := point.Compile(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := e.Evaluate(tc.input); err == nil {
			t.Fatalf("invalid operation accepted: %s(%v) = %v", tc.source, tc.input, got)
		}
	}
	for _, source := range []string{"9223372036854775808", "-9223372036854775809", "-(9223372036854775809)", "0xffffffffffffffff", "1e309", "x.Foo()", "x[1]", "f(x)", "[]int{1}"} {
		if _, err := point.Compile(source); err == nil {
			t.Fatal("unsafe/out-of-range compile accepted", source)
		}
	}
	for source, want := range map[string]bool{"true || (1<<63)>0": true, "false && x%0>0": false} {
		e, err := point.Compile(source)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := e.Evaluate(int64(1)); err != nil || got != want {
			t.Fatal("integer error bypassed short circuit", source, got, err)
		}
	}
	for _, source := range []string{"-(9223372036854775808)", "-(0x8000000000000000)", "-((0b1_000000000000000000000000000000000000000000000000000000000000000))"} {
		e, err := point.Compile(source)
		if err != nil {
			t.Fatal(err)
		}
		if v, err := e.Evaluate(nil); err != nil || v != int64(math.MinInt64) {
			t.Fatal(source, v, err)
		}
	}
}

func TestExpressionImmutableConcurrentIntegerEvaluation(t *testing.T) {
	e, err := point.Compile("((x >> 4) & 0xf) == 10 && x > 9007199254740992")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if v, err := e.Evaluate(int64(9007199254740992 + 0xa5)); err != nil || v != true {
					t.Error(v, err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestJSONIntegerCastsPreserveExactnessAndText(t *testing.T) {
	n := json.Number("9223372036854775807")
	if v, err := point.Convert(n, point.String); err != nil || v != string(n) {
		t.Fatal("integer text changed", v, err)
	}
	if v, err := point.Convert(n, point.Int); err != nil || v != int64(math.MaxInt64) {
		t.Fatal("integer cast changed", v, err)
	}
	for _, n := range []json.Number{"9007199254740993", "9223372036854775807", "9223372036854775808"} {
		if v, err := point.Convert(n, point.Float); err == nil {
			t.Fatal("implicit lossy JSON integer cast", n, v)
		}
	}
}
