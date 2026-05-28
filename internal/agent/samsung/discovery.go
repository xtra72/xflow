package samsung

import (
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// 탐색 프로토콜 상수
// ---------------------------------------------------------------------------

// AddrOutdoorBroadcast 는 실외기 탐색용 브로드캐스트 주소이다 (B0 FF 10).
var AddrOutdoorBroadcast = NasaAddress{0xB0, 0xFF, 0x10}

// readBufSize 는 응답 수신 버퍼 크기이다.
const readBufSize = 1024

// ---------------------------------------------------------------------------
// 탐색 결과 타입
// ---------------------------------------------------------------------------

// DiscoveryResult 는 탐색 결과를 나타낸다.
type DiscoveryResult struct {
	Address    NasaAddress // 장치 주소
	DeviceType string      // "HVACR.ODU" 또는 "HVACR.IDU" (v0.18.3)
	Ready      bool        // 통신 준비 상태
}

// ---------------------------------------------------------------------------
// 탐색 콜백 인터페이스
// ---------------------------------------------------------------------------

// DiscoveryCallbacks 는 탐색 과정의 콜백 인터페이스이다.
type DiscoveryCallbacks interface {
	OnDeviceDiscovered(result DiscoveryResult)
	OnDiscoveryError(err error)
	OnDiscoveryComplete(results []DiscoveryResult)
}

// ---------------------------------------------------------------------------
// 실외기 탐색
// ---------------------------------------------------------------------------

// DiscoverOutdoors 는 실외기 주소를 탐색한다.
// 간소화 구현: 단계 2 (Standby Query)만 수행하여 이미 등록된 실외기를 발견한다.
//
// C001 명령을 실외기 브로드캐스트 주소(B0FF10)로 전송하고 응답을 수집한다.
// SourceAddr 이 0x10 으로 시작하는 응답만 실외기로 인식한다.
func DiscoverOutdoors(transport NasaTransport, protocol NasaProtocol, seqNum *byte, timeout time.Duration) ([]DiscoveryResult, error) {
	// 1. C001 Standby Request 프레임 생성
	msg := &NasaMessage{
		SourceAddr:  AddrController,
		DestAddr:    AddrOutdoorBroadcast,
		CommandCode: CmdStandbyRequest,
		SequenceNum: *seqNum,
		MessageSets: []NasaMessageSet{
			{Index: MsgAddrInfo, Value: []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		},
	}
	frame, err := protocol.Encode(msg)
	if err != nil {
		return nil, err
	}

	// 2. 시퀀스 번호 증가
	*seqNum++

	// 3. 프레임 전송
	if err := transport.Send(frame); err != nil {
		return nil, err
	}

	// 4. 응답 수집
	messages, err := readResponses(transport, protocol, timeout)
	if err != nil {
		return nil, err
	}

	// 5. 실외기 응답만 필터링
	var results []DiscoveryResult
	for _, m := range messages {
		if m.SourceAddr.IsOutdoor() {
			results = append(results, DiscoveryResult{
				Address:    m.SourceAddr,
				DeviceType: "HVACR.ODU",
				Ready:      false,
			})
		}
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// 실내기 탐색
// ---------------------------------------------------------------------------

// DiscoverIndoors 는 실내기 주소를 탐색한다.
// C011 명령을 실내기 브로드캐스트 주소(B2FF20)로 전송하고 응답을 수집한다.
// SourceAddr 이 0x20 으로 시작하는 응답만 실내기로 인식한다.
func DiscoverIndoors(transport NasaTransport, protocol NasaProtocol, seqNum *byte, timeout time.Duration) ([]DiscoveryResult, error) {
	// 1. C011 상태 쿼리 프레임 생성
	frame, err := protocol.BuildStatusQuery(AddrBroadcastIndoor, *seqNum)
	if err != nil {
		return nil, err
	}

	// 2. 시퀀스 번호 증가
	*seqNum++

	// 3. 프레임 전송
	if err := transport.Send(frame); err != nil {
		return nil, err
	}

	// 4. 응답 수집
	messages, err := readResponses(transport, protocol, timeout)
	if err != nil {
		return nil, err
	}

	// 5. 실내기 응답만 필터링
	var results []DiscoveryResult
	for _, m := range messages {
		if m.SourceAddr.IsIndoor() {
			results = append(results, DiscoveryResult{
				Address:    m.SourceAddr,
				DeviceType: "HVACR.IDU",
				Ready:      false,
			})
		}
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// 응답 수집 헬퍼
// ---------------------------------------------------------------------------

// readResponses 는 타임아웃까지 트랜스포트에서 응답을 읽고 디코딩한다.
// 잘못된 프레임은 건너뛰고 타임아웃 에러는 정상 종료로 처리한다.
func readResponses(transport NasaTransport, protocol NasaProtocol, timeout time.Duration) ([]*NasaMessage, error) {
	buf := make([]byte, readBufSize)
	deadline := time.Now().Add(timeout)
	var messages []*NasaMessage

	for time.Now().Before(deadline) {
		n, err := transport.Receive(buf)
		if err != nil {
			if isTimeoutErr(err) {
				break
			}
			continue
		}
		if n == 0 {
			continue
		}

		msg, err := protocol.Decode(buf[:n])
		if err != nil {
			continue // 잘못된 프레임 건너뛰기
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// ---------------------------------------------------------------------------
// 타임아웃 에러 판별 헬퍼
// ---------------------------------------------------------------------------

// isTimeoutErr 는 net.Error 타임아웃 에러인지 확인한다.
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
