package core

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	//	"io"
)

var zeroUuid = Uuid{}

var errIrregalUuid = errors.New("irregal uuid")

// UUID 是128位整数, 字符串形式为 xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
// 其中x为十六进制数字0~9或a~f(小写)
// 例如 123e4567-e89b-12d3-a456-426655440000
// 按照标准, UUID有5种格式, 其中某些值是有特定含义的, 但我们不关心这些.
// 我们只保证生成的UUID是合法的, 并且用作全局唯一标识.
type Uuid [16]byte

// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" (8-4-4-4-12) 格式
func (v Uuid) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		v[:4], v[4:6], v[6:8], v[8:10], v[10:])
}

// 表示为压缩的字符串格式, 零表示为"0", 其他值表示为Base64的前22字符, 即去掉最后的"=="
func (v Uuid) Compact() string {
	if v.IsZero() {
		return "0"
	}
	return base64.URLEncoding.EncodeToString(v[0:])[0:22]
}

// 零
func (v Uuid) IsZero() bool {
	return zeroUuid == v
}

// 生成新的UUID
func NewUuid() (ret Uuid) {
	n, err := rand.Read(ret[:])
	if n != 16 || err != nil {
		ret = [16]byte{}
		return
	}
	//	t := time.Now().UTC().Unix() / 3600 / 24
	//	ret[0] = byte((t>>8)&0xff) ^ ret[15]
	//	ret[1] = byte(t&0xff) ^ ret[14]
	ret[6] = (ret[6] & 0x0f) | 0x40
	ret[8] = (ret[8] & 0x3f) | 0x80
	return
}

func NewUuidStr() string {
	return NewUuid().Compact()
}

func hex(v byte) byte {
	switch v {
	case '0':
		return 0x00
	case '1':
		return 0x01
	case '2':
		return 0x02
	case '3':
		return 0x03
	case '4':
		return 0x04
	case '5':
		return 0x05
	case '6':
		return 0x06
	case '7':
		return 0x07
	case '8':
		return 0x08
	case '9':
		return 0x09
	case 'A', 'a':
		return 0x0a
	case 'B', 'b':
		return 0x0b
	case 'C', 'c':
		return 0x0c
	case 'D', 'd':
		return 0x0d
	case 'E', 'e':
		return 0x0e
	case 'F', 'f':
		return 0x0f
	}

	return 0xff
}

// 解释"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" (8-4-4-4-12) 或压缩格式的Uuid
//  ""和"0"将被识别为零值
func ParseUuid(s string) (ret Uuid, err error) {
	switch len(s) {
	case 36: // 标准格式
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			err = errors.New(`invalid UUID string: "` + s + `"`)
			return
		}

		for i, pos := range []int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34} {
			a := hex(s[pos])
			b := hex(s[pos+1])
			if a == 0xff || b == 0xff {
				ret = [16]byte{}
				err = errors.New(`invalid UUID string: "` + s + `"`)
				return
			}
			ret[i] = (a << 4) | b
		}

		err = nil
		return

	case 24: // Base64带"=="结尾
		fallthrough
	case 22: // Base64不带"=="结尾
		var buf []byte
		var s1 string
		if len(s) == 22 {
			s1 = s + "=="
		} else {
			s1 = s + "=="
		}
		buf, err = base64.URLEncoding.DecodeString(s1)
		if err == nil {
			copy(ret[0:], buf)
			return
		}
		return
	case 0: // 空字符串
		return
	case 1:
		if s[0] == '0' {
			return
		}
	}
	ret = [16]byte{}
	err = errors.New(`invalid UUID string: "` + s + `"`)
	return

}

func (this *Uuid) Scan(state fmt.ScanState, verb rune) error {
	*this = Uuid{}

	n := 0
	tok, err := state.Token(true, func(r rune) bool {
		if n == 36 {
			return false
		}
		n++
		return true
	})

	if err != nil {
		return err
	}

	uuid, err := ParseUuid(string(tok))
	if err != nil {
		return err
	}
	*this = uuid
	return nil
}

func (this *Uuid) GobEncode() ([]byte, error) {
	if this.IsZero() {
		return nil, nil
	}
	return (*this)[0:], nil
}

func (this *Uuid) GobDecode(data []byte) (err error) {
	if len(data) == 0 {
		*this = [16]byte{}
		return
	}
	if len(data) != 16 {
		*this = [16]byte{}
		err = errIrregalUuid
		return
	}
	copy((*this)[0:], data)
	return
}
