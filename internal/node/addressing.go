// addressing.go (v0.18.26) 는 HVAC 상태 노드의 프로토콜 어드레싱 (group_id /
// unit_id) 입력 파싱/매칭 공통 유틸을 제공한다.
//
// 의미 (프로토콜별):
//
//	LG ICP-01    : group_id 미사용. unit_id = STX byte hex ("58" ODU /
//	               "81"-"BF" IDU 64 units).
//	Samsung NASA : group_id = NASA addr byte 1 (외기 인덱스, "00"-"0F").
//	               unit_id  = NASA addr byte 2 ("00"-"3F" indoor; outdoor 는
//	               group_id 와 동일).
//	Century ICP01: group_id 미사용. unit_id = sub_dev_id hex ("3B" 등).
//
// 두 필드 모두 OPTIONAL 문자열. 빈 값 = 필터/타깃 없음 (모든 디바이스 처리,
// broadcast request_state).

package node

import (
	"strconv"
	"strings"
)

// parseHexByte 는 hex 문자열 ("0xAB", "AB", "ab" 등) 을 byte 로 파싱한다.
//
// 동작:
//   - 앞뒤 공백 trim
//   - "0x" / "0X" 접두 제거
//   - 빈 문자열이면 (0, false)
//   - 1~2 hex digit 만 허용. 8-bit 범위 초과 시 (0, false)
//   - 유효한 경우 (byte, true)
//
// 사용 예: 노드 config 의 unit_id "58" → 0x58.
func parseHexByte(s string) (byte, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		return 0, false
	}
	return byte(n), true
}

// hexByteEqual 은 두 hex byte 문자열이 같은 byte 값을 가리키는지 비교한다.
// "0x58" 와 "58", "58" 와 "0x58" 모두 동일하게 인식. 어느 한 쪽이라도 비어
// 있으면 true (필터 없음 의미). 둘 다 유효하지 않은 경우 false.
func hexByteEqual(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return true
	}
	ba, aok := parseHexByte(a)
	bb, bok := parseHexByte(b)
	if !aok || !bok {
		return false
	}
	return ba == bb
}
