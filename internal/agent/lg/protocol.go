package lg

import "fmt"

// LGAPProtocol 은 LGAP 프로토콜의 인코딩/디코딩 인터페이스를 정의한다.
type LGAPProtocol interface {
	// BuildStatusQuery 는 지정된 존에 대한 읽기 전용 요청 패킷(8바이트)을 생성한다.
	BuildStatusQuery(zone byte) []byte

	// BuildControlCommand 는 지정된 존에 대한 쓰기 요청 패킷(8바이트)을 생성한다.
	BuildControlCommand(zone byte, flags byte, modeCombo byte, temp byte) []byte

	// DecodeResponse 는 16바이트 응답 데이터를 LGAPResponse 로 파싱한다.
	DecodeResponse(data []byte) (*LGAPResponse, error)

	// CalculateChecksum 은 LGAP 체크섬을 계산한다.
	CalculateChecksum(data []byte) byte
}

// lgapProtocol 은 LGAPProtocol 인터페이스의 상태 없는 구현체이다.
type lgapProtocol struct{}

// NewLGAPProtocol 은 LGAPProtocol 인터페이스의 새 인스턴스를 반환한다.
func NewLGAPProtocol() LGAPProtocol {
	return &lgapProtocol{}
}

// BuildStatusQuery 는 지정된 존에 대한 읽기 전용 요청 패킷을 생성한다.
// TX4=0x00 (플래그 없음, 읽기 전용), TX5=0x00 (모드 변경 없음), TX6=0x00 (온도 변경 없음).
func (p *lgapProtocol) BuildStatusQuery(zone byte) []byte {
	pkt := make([]byte, RequestSize)
	pkt[0] = HeaderByte                    // TX0: 헤더
	pkt[1] = CommandByte                   // TX1: 명령
	pkt[2] = CommandID                     // TX2: 명령 ID
	pkt[3] = zone                          // TX3: 존 주소
	pkt[4] = 0x00                          // TX4: 플래그 없음 (읽기 전용)
	pkt[5] = 0x00                          // TX5: 모드 변경 없음
	pkt[6] = 0x00                          // TX6: 온도 변경 없음
	pkt[7] = CalcLGAPChecksum(pkt[:7])     // TX7: 체크섬
	return pkt
}

// BuildControlCommand 는 지정된 존에 대한 쓰기 요청 패킷을 생성한다.
// flags 에 FlagExecute(EXE) 비트를 자동으로 설정한다.
func (p *lgapProtocol) BuildControlCommand(zone byte, flags byte, modeCombo byte, temp byte) []byte {
	pkt := make([]byte, RequestSize)
	pkt[0] = HeaderByte                    // TX0: 헤더
	pkt[1] = CommandByte                   // TX1: 명령
	pkt[2] = CommandID                     // TX2: 명령 ID
	pkt[3] = zone                          // TX3: 존 주소
	pkt[4] = flags | FlagExecute           // TX4: 제어 플래그 (항상 EXE 비트 설정)
	pkt[5] = modeCombo                     // TX5: 모드/팬/스윙 조합
	pkt[6] = temp                          // TX6: 설정 온도
	pkt[7] = CalcLGAPChecksum(pkt[:7])     // TX7: 체크섬
	return pkt
}

// DecodeResponse 는 16바이트 응답 데이터를 LGAPResponse 로 디코딩한다.
// 패킷 크기, 헤더 바이트, 체크섬을 검증한다.
func (p *lgapProtocol) DecodeResponse(data []byte) (*LGAPResponse, error) {
	if len(data) != ResponseSize {
		return nil, fmt.Errorf("lgap: 응답 크기 오류: %d (예상 %d)", len(data), ResponseSize)
	}
	if data[0] != HeaderByte {
		return nil, fmt.Errorf("lgap: 응답 헤더 오류: 0x%02X", data[0])
	}
	if !VerifyChecksum(data) {
		return nil, ErrChecksumMismatch
	}

	resp := &LGAPResponse{
		Status:      data[1],
		FlagsEcho:   data[2],
		Zone:        data[4],
		Error:       data[5],
		ModeCombo:   data[6],
		TargetTemp:  data[7],
		RoomTemp:    data[8],
		PipeInTemp:  data[9],
		PipeOutTemp: data[10],
		ZoneLoad:    data[11],
		ZonePower:   data[12],
		DesignLoad:  data[13],
		ODULoad:     data[14],
	}
	copy(resp.Raw[:], data)
	return resp, nil
}

// CalculateChecksum 은 LGAP 체크섬을 계산한다.
func (p *lgapProtocol) CalculateChecksum(data []byte) byte {
	return CalcLGAPChecksum(data)
}
