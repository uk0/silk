package point

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/core"
)

// MaxSampleTextBytes is the maximum size of a point's UTF-8 text, before
// serialization. The same contract applies with and without a point store.
const MaxSampleTextBytes = 1 << 20

// ValidateSampleMetadata checks the quality and a timestamp which round-trips
// through the historian's signed nanosecond representation. It does not
// examine Raw: that requires a declared point kind.
func ValidateSampleMetadata(v core.Value) error {
	if v.Quality > core.QualityGood || v.Time.IsZero() {
		return fmt.Errorf("point: invalid quality or missing timestamp")
	}
	if !time.Unix(0, v.Time.UnixNano()).Equal(v.Time) {
		return fmt.Errorf("point: timestamp out of range")
	}
	return nil
}

// NormalizeSample checks a sample and converts its payload to the point's
// declared scalar kind. A missing payload is valid only when quality is Bad
// or Uncertain. Reject text that would be too large or change when encoded.
func NormalizeSample(v core.Value, kind Kind) (core.Value, error) {
	if err := ValidateSampleMetadata(v); err != nil {
		return core.Value{}, err
	}
	if !kind.Valid() {
		return core.Value{}, fmt.Errorf("point: invalid sample kind")
	}
	if v.Raw != nil || v.Quality == core.QualityGood {
		raw, err := Convert(v.Raw, kind)
		if err != nil {
			return core.Value{}, err
		}
		v.Raw = raw
	}
	if text, ok := v.Raw.(string); ok {
		if len(text) > MaxSampleTextBytes {
			return core.Value{}, fmt.Errorf("point: string sample exceeds 1 MiB")
		}
		if !utf8.ValidString(text) {
			return core.Value{}, fmt.Errorf("point: string sample is not valid UTF-8")
		}
	}
	return v, nil
}
