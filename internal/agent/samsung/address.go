package samsung

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// NASAAddress 는 삼성 NASA 프로토콜의 3바이트 주소를 나타낸다.
type NASAAddress [3]byte

// 사전 정의된 NASA 주소 상수
var (
	// AddrController 는 외부 컨트롤러 주소이다.
	AddrController = NASAAddress{0x6A, 0xEE, 0xFF}

	// AddrBroadcastAll 는 전체 브로드캐스트 주소이다.
	AddrBroadcastAll = NASAAddress{0xB0, 0xFF, 0xFF}

	// AddrBroadcastIndoor 는 전체 실내기 브로드캐스트 주소이다.
	AddrBroadcastIndoor = NASAAddress{0xB2, 0xFF, 0x20}
)

// String 은 "XX.XX.XX" 형식의 점 구분 16진수 문자열을 반환한다.
func (a NASAAddress) String() string {
	return fmt.Sprintf("%02X.%02X.%02X", a[0], a[1], a[2])
}

// Hex 는 "XXXXXX" 형식의 컴팩트 16진수 문자열을 반환한다.
func (a NASAAddress) Hex() string {
	return fmt.Sprintf("%02X%02X%02X", a[0], a[1], a[2])
}

// IsOutdoor 는 실외기 주소인지 여부를 반환한다 (첫 번째 바이트가 0x10).
func (a NASAAddress) IsOutdoor() bool {
	return a[0] == 0x10
}

// IsIndoor 는 실내기 주소인지 여부를 반환한다 (첫 번째 바이트가 0x20).
func (a NASAAddress) IsIndoor() bool {
	return a[0] == 0x20
}

// IsController 는 외부 컨트롤러 주소인지 여부를 반환한다 ({0x6A, 0xEE, 0xFF}).
func (a NASAAddress) IsController() bool {
	return a == AddrController
}

// IsBroadcast 는 브로드캐스트 주소인지 여부를 반환한다
// (첫 번째 바이트가 0xB0, 0xB2, 또는 0xB3).
func (a NASAAddress) IsBroadcast() bool {
	return a[0] == 0xB0 || a[0] == 0xB2 || a[0] == 0xB3
}

// OutdoorIndex 는 두 번째 바이트(물리 주소 0x00~0x0F)를 반환한다.
func (a NASAAddress) OutdoorIndex() byte {
	return a[1]
}

// IndoorIndex 는 (두 번째 바이트, 세 번째 바이트) 쌍을 반환한다.
func (a NASAAddress) IndoorIndex() (outdoor byte, indoor byte) {
	return a[1], a[2]
}

// NewOutdoorAddr 는 지정된 인덱스의 실외기 주소를 생성한다.
func NewOutdoorAddr(index byte) NASAAddress {
	return NASAAddress{0x10, index, 0x00}
}

// NewIndoorAddr 는 지정된 실외기/실내기 인덱스의 실내기 주소를 생성한다.
func NewIndoorAddr(outdoor, indoor byte) NASAAddress {
	return NASAAddress{0x20, outdoor, indoor}
}

// NewOutdoorBroadcast 는 지정된 인덱스의 실외기 브로드캐스트 주소를 생성한다.
func NewOutdoorBroadcast(index byte) NASAAddress {
	return NASAAddress{0xB0, index, 0xFF}
}

// NewIndoorBroadcast 는 지정된 실외기/실내기 인덱스의 실내기 브로드캐스트 주소를 생성한다.
func NewIndoorBroadcast(outdoor, indoor byte) NASAAddress {
	return NASAAddress{0xB3, outdoor, indoor}
}

// ParseNASAAddress 는 16진수 문자열을 NASAAddress 로 파싱한다.
// 지원 형식: "XX.XX.XX" (점 구분), "XX XX XX" (스페이스 구분) 또는 "XXXXXX" (컴팩트).
// 대소문자를 구분하지 않는다.
func ParseNASAAddress(s string) (NASAAddress, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return NASAAddress{}, ErrInvalidAddress
	}

	var raw string

	// 점 또는 스페이스 구분 형식 시도: "XX.XX.XX" 또는 "XX XX XX"
	if strings.Contains(s, ".") || strings.Contains(s, " ") {
		sep := " "
		if strings.Contains(s, ".") {
			sep = "."
		}
		parts := strings.Split(s, sep)
		if len(parts) != 3 {
			return NASAAddress{}, ErrInvalidAddress
		}
		for _, p := range parts {
			if len(p) != 2 {
				return NASAAddress{}, ErrInvalidAddress
			}
		}
		raw = parts[0] + parts[1] + parts[2]
	} else {
		// 컴팩트 형식: "XXXXXX"
		if len(s) != 6 {
			return NASAAddress{}, ErrInvalidAddress
		}
		raw = s
	}

	b, err := hex.DecodeString(raw)
	if err != nil {
		return NASAAddress{}, ErrInvalidAddress
	}

	if len(b) != 3 {
		return NASAAddress{}, ErrInvalidAddress
	}

	return NASAAddress{b[0], b[1], b[2]}, nil
}
