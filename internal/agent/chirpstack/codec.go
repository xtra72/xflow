package chirpstack

import (
	"fmt"
	"sync"
)

// DownlinkCommand 는 제어 노드가 코덱에 전달하는 타입드 다운링크 명령이다
// (SPEC-CHIRPSTACK-002 REQ-M2-01).
//
//   - DevEui: 대상 디바이스 EUI (토픽/페이로드 구성용).
//   - Name: 코덱이 해석하는 명령 이름 (예: "reboot").
//   - Params: 명령 파라미터. JSON 유래 값이므로 숫자는 float64 일 수 있다.
//   - Confirmed: LoRaWAN confirmed 다운링크 요청 여부(페이로드로 통과, A4).
type DownlinkCommand struct {
	DevEui    string
	Name      string
	Params    map[string]any
	Confirmed bool
}

// DownlinkCodec 은 deviceProfile 별 다운링크 인코딩 계약이다 (REQ-M2-01).
//
// 반환값: fPort(코덱 결정), data(TLV 등 원시 바이트), confirmed(페이로드 통과값), err.
// 코덱이 모르는 command 또는 잘못된 파라미터는 반드시 err 를 반환해야 하며, 이때
// 호출자는 발행하지 않는다(REQ-M2-04: 일반 passthrough 다운링크 금지).
type DownlinkCodec interface {
	Encode(cmd DownlinkCommand) (fPort uint8, data []byte, confirmed bool, err error)
}

// downlinkCodecs 는 deviceProfileName 키 코덱 레지스트리이다 (확장점).
// v1 seed 는 Milesight WS301 하나이며, 후속 deviceProfile 은 RegisterDownlinkCodec
// 호출만으로 확장된다.
var (
	downlinkCodecMu sync.RWMutex
	downlinkCodecs  = make(map[string]DownlinkCodec)
)

// RegisterDownlinkCodec 은 deviceProfileName 에 코덱을 등록한다(동일 키는 덮어쓴다).
// 빈 프로파일명이나 nil 코덱은 무시한다.
func RegisterDownlinkCodec(deviceProfileName string, codec DownlinkCodec) {
	if deviceProfileName == "" || codec == nil {
		return
	}
	downlinkCodecMu.Lock()
	downlinkCodecs[deviceProfileName] = codec
	downlinkCodecMu.Unlock()
}

// LookupDownlinkCodec 은 deviceProfileName 의 코덱을 조회한다. 미등록이면 ok=false.
func LookupDownlinkCodec(deviceProfileName string) (DownlinkCodec, bool) {
	downlinkCodecMu.RLock()
	defer downlinkCodecMu.RUnlock()
	codec, ok := downlinkCodecs[deviceProfileName]
	return codec, ok
}

// unregisterDownlinkCodecForTest 는 테스트에서 등록한 코덱을 되돌린다.
func unregisterDownlinkCodecForTest(deviceProfileName string) {
	downlinkCodecMu.Lock()
	delete(downlinkCodecs, deviceProfileName)
	downlinkCodecMu.Unlock()
}

// EncodeDownlink 는 deviceProfile 의 코덱으로 명령을 인코딩한다 (REQ-M2-01/04).
//
// 미등록 프로파일은 에러이다 — 코덱 없는 프로파일에 대해 일반(generic) passthrough
// 다운링크를 만들어내지 않는다(REQ-M2-04).
func EncodeDownlink(deviceProfileName string, cmd DownlinkCommand) (fPort uint8, data []byte, confirmed bool, err error) {
	codec, ok := LookupDownlinkCodec(deviceProfileName)
	if !ok {
		return 0, nil, false, fmt.Errorf(
			"chirpstack-codec: deviceProfile %q 에 등록된 다운링크 코덱이 없습니다 (generic passthrough 금지)",
			deviceProfileName)
	}
	fPort, data, confirmed, err = codec.Encode(cmd)
	if err != nil {
		return 0, nil, false, err
	}
	if len(data) == 0 {
		return 0, nil, false, fmt.Errorf(
			"chirpstack-codec: deviceProfile %q 코덱이 command %q 에 대해 빈 페이로드를 반환했습니다",
			deviceProfileName, cmd.Name)
	}
	return fPort, data, confirmed, nil
}
