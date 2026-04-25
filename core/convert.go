package core

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

type objId uint

func (this *objId) Scan(state fmt.ScanState, verb rune) error {
	*this = 0
	state.SkipSpace()

	r, _, err := state.ReadRune()
	if err != nil {
		return errors.New("Error in scaning object id: " + err.Error())
	}

	if r == 'n' {
		tok, err := state.Token(false, nil)
		if err != nil {
			return errors.New("Error in scaning object id: " + err.Error())
		}
		if string(tok) != "il" {
			return errors.New("Error in scaning object id: invalid syntax")
		}
		return nil
	}

	if r != '<' {
		return errors.New("Error in scaning object id: invalid syntax")
	}

	//r, _, err = state.ReadRune()
	//if err != nil || r != '>' {
	//	return errors.New("Error in scaning object id: invalid syntax")
	//}

	tok, err := state.Token(false, nil)
	if err != nil {
		return errors.New("Error in scaning object id: " + err.Error())
	}
	if tok[len(tok)-1] != '>' {
		return errors.New("Error in scaning object id: invalid syntax")
	}
	id, err := strconv.Atoi(string(tok[:len(tok)-1]))
	if err != nil {
		return errors.New("Error in scaning object id: invalid syntax")
	}
	*this = objId(id)
	return nil
}

type StringReader string

func (r *StringReader) Read(b []byte) (n int, err error) {
	n = copy(b, *r)
	*r = (*r)[n:]
	if n == 0 {
		err = io.EOF
	}
	return
}

type quotedString string

func (this *quotedString) Scan(state fmt.ScanState, verb rune) error {
	*this = ""
	state.SkipSpace()

	r, _, err := state.ReadRune()
	if err != nil {
		return errors.New("Error in scaning string: 33" + err.Error())
	}

	if r != '"' {
		state.UnreadRune()
		tok, err := state.Token(true, nil)
		if err != nil {
			return errors.New("Error in scaning string: '\"' expected.")
		}
		*this = quotedString(string(tok))
		return nil
	}

	state.UnreadRune()
	skipNext := true
	inString := true
	tok, err := state.Token(true, func(r rune) bool {
		if !inString {
			return false
		}
		if skipNext {
			skipNext = false
			return true
		}
		if r == '\\' {
			skipNext = true
			return true
		}

		if r == '"' {
			inString = false
		}
		return true
	})
	if err != nil {
		return errors.New("Error in scaning string: 111" + err.Error())
	}

	s1, err := strconv.Unquote(string(tok))
	if err != nil {
		return errors.New("Error in scaning string: " + err.Error() + " :　" + string(tok))
	}
	*this = quotedString(s1)

	return nil
}

type stringSlice []string

func (this *stringSlice) Scan(state fmt.ScanState, verb rune) error {
	*this = nil
	state.SkipSpace()
	r, _, _ := state.ReadRune()

	if r != '[' {
		return errors.New("Error in scaning []string: '[' expected.")
	}

	for {
		state.SkipSpace()
		r, _, err := state.ReadRune()
		if err != nil {
			return errors.New("Error in scaning []string: " + err.Error())
		}
		if r == ']' {
			break
		}

		state.UnreadRune()

		//if r != '"' {
		//	return errors.New("Error in scaning []string: : '\"' expected.")
		//}

		//state.UnreadRune()
		//skipNext := true
		//inString := true
		//tok, err := state.Token(true, func(r rune) bool {
		//	if !inString {
		//		return false
		//	}
		//	if skipNext {
		//		skipNext = false
		//		return true
		//	}
		//	if r == '\\' {
		//		skipNext = true
		//		return true
		//	}

		//	if r == '"' {
		//		inString = false
		//	}
		//	return true
		//})
		//if err != nil {
		//	return errors.New("Error in scaning []string: " + err.Error())
		//}
		////fmt.Println(string(tok))

		//s1, err := strconv.Unquote(string(tok))
		//if err != nil {
		//	return errors.New("Error in scaning []string: " + err.Error())
		//}
		var s1 quotedString
		err = s1.Scan(state, verb)
		if err != nil {
			return errors.New("Error in scaning []string: " + err.Error())
		}
		*this = append(*this, string(s1))
	}

	return nil
}

func PersistSscan(s string, a ...interface{}) (n int, err error) {
	sr := (*StringReader)(&s)
	return PersistFscan(sr, a...)
}

func PersistFscan(r io.Reader, a ...interface{}) (n int, err error) {
	for n = 0; n < len(a); n++ {
		v := a[n]
		switch x := v.(type) {
		case *[]string:
			_, err = fmt.Fscan(r, (*stringSlice)(x))
		case *string:
			_, err = fmt.Fscan(r, (*quotedString)(x))
		case *interface{}:
			panic("can't scan type: *interface {}")
		default:
			_, err = fmt.Fscan(r, x)
		}
		if err != nil {
			return
		}
	}
	return
}

func PersistString(val interface{}) (string, error) {
	buf, err := appendPersistVal(nil, val)
	if err != nil {
		return "", err
	}
	return string(buf), err
}

func appendPersistVal(buf []byte, val interface{}) ([]byte, error) {
	switch x := val.(type) {
	case nil:
		return append(buf, `nil`...), nil
	case string:
		return strconv.AppendQuote(buf, x), nil
	case int:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case int8:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case int16:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case int32:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case int64:
		return strconv.AppendInt(buf, int64(x), 10), nil
	case uint: // byte
		return strconv.AppendUint(buf, uint64(x), 10), nil
	case uint8:
		return strconv.AppendUint(buf, uint64(x), 10), nil
	case uint16:
		return strconv.AppendUint(buf, uint64(x), 10), nil
	case uint32:
		return strconv.AppendUint(buf, uint64(x), 10), nil
	case uint64:
		return strconv.AppendUint(buf, uint64(x), 10), nil
	case float32:
		return strconv.AppendFloat(buf, float64(x), 'g', 8, 32), nil
	case float64:
		return strconv.AppendFloat(buf, float64(x), 'g', 16, 64), nil
	case bool:
		if x {
			return append(buf, `true`...), nil
		} else {
			return append(buf, `false`...), nil
		}
	case objId:
		if x == 0 {
			return append(buf, `nil`...), nil
		}
		return append(strconv.AppendInt([]byte{'<'}, int64(x), 10), '>'), nil
	case interface {
		String() string
	}:
		return []byte(x.String()), nil
	default:
		v := reflect.ValueOf(val)
		if v.Kind() == reflect.Slice {
			slen := v.Len()
			if slen == 0 {
				buf = append(buf, '[', ']')
				return buf, nil
			}
			buf = append(buf, '[')
			for i := 0; i < slen; i++ {
				var err error
				buf, err = appendPersistVal(buf, v.Index(i).Interface())
				if err != nil {
					return buf, err
				}
				buf = append(buf, ' ')
			}
			buf[len(buf)-1] = ']'
			return buf, nil
		}
		if v.Kind() == reflect.Ptr {
			return appendPersistVal(buf, v.Elem().Interface())
		}
		return append(buf, '#', '#', '#'), errors.New("unsupported type: " + v.Type().String())
	}
	panic("internal error")
}

func VisualString(val interface{}) string {
	return fmt.Sprint(val)
}

//func anyToString(val interface{}) string {
//	if val == nil {
//		return ""
//	}
//	s := fmt.Sprint(val)
//	if core.IsDebug() {
//		tmp := reflect.New(reflect.TypeOf(val)).Interface()
//		err := anyFromString(s, tmp)
//		if err != nil {
//			core.Warn(err)
//		}
//	}
//	return s
//}

//func anyFromString(s string, ptr interface{}) error {
//	switch x := ptr.(type) {
//	case *string:
//		*x = s
//		return nil
//	case interface {
//		FromString(string) error
//	}:
//		return x.FromString(s)
//	default:
//		_, err := fmt.Sscan(s, ptr)
//		return err
//	}

//}
