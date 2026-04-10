// Package serial 의 framing.go 는 시리얼 포트 전용 프레임 리더인
// SerialConnReader 를 제공한다.
//
// 실제 프레이밍 알고리즘은 공개 패키지 `pkg/framing` 으로 이동되었으며,
// 시리얼 에이전트는 해당 패키지의 Framer 구현체를 사용한다.
// SerialConnReader 는 리더 수명 동안 하나의 bufio.Scanner 를 유지하여
// newlineFramer 와 같은 scanner 기반 구현체의 버퍼 데이터 손실을 방지한다.
package serial

import (
	"bufio"
	"io"

	"github.com/xtra/xflow/pkg/framing"
)

// SerialConnReader 는 시리얼 포트 전용 프레임 리더이다.
// newlineFramer 가 Read() 호출마다 새 bufio.Scanner 를 생성하는 문제를 해결하여
// 리더 수명 동안 하나의 scanner 를 유지한다.
type SerialConnReader struct {
	framer  framing.Framer
	reader  io.Reader
	scanner *bufio.Scanner
}

// NewSerialConnReader 는 새 SerialConnReader 를 생성한다.
// framer 가 ScannerConfigurer 를 구현하는 경우 (예: newlineFramer)
// scanner 를 초기화하여 버퍼링 상태를 유지한다.
func NewSerialConnReader(f framing.Framer, r io.Reader) *SerialConnReader {
	cr := &SerialConnReader{
		framer: f,
		reader: r,
	}
	if sc, ok := f.(framing.ScannerConfigurer); ok {
		cr.scanner = sc.ConfigureScanner(r)
	}
	return cr
}

// Read 는 리더에서 하나의 프레임을 읽는다.
// scanner 가 설정된 경우 내부 scanner 를 재사용하여 버퍼 데이터 손실을 방지한다.
func (cr *SerialConnReader) Read() ([]byte, error) {
	if cr.scanner != nil {
		if cr.scanner.Scan() {
			b := cr.scanner.Bytes()
			result := make([]byte, len(b))
			copy(result, b)
			return result, nil
		}
		if err := cr.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return cr.framer.Read(cr.reader)
}
