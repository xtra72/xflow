package samsung

// SPEC-HVACR-SYNC-001 Module 5: 와이어 포맷 (Wire Format)
//
// 게이트웨이↔서버 미러링의 MQTT payload 정규화 포맷을 정의한다.
//   - 업링크(디코드 NASA 메시지): 무손실 구조화 JSON (REQ-SYNC-001-05-01)
//   - 다운링크(제어 명령): 게이트웨이 Process 로 매핑 가능한 JSON (REQ-SYNC-001-05-02)
//
// 모든 타임스탬프는 epoch milliseconds(int64, UnixMilli)이다(REQ-SYNC-001-05-03,
// 프로젝트 규약). gateway_id 는 payload 에 넣지 않는다 — 토픽이 식별을 전담한다
// (REQ-SYNC-001-05-04).

import (
	"encoding/hex"
	"fmt"
	"time"
)

// wireMessageSet 는 NasaMessageSet 의 와이어 표현이다.
// index 는 정수, value 는 hex 인코딩 문자열(REQ-SYNC-001-05-01).
type wireMessageSet struct {
	Index uint16 `json:"index"`
	Value string `json:"value"` // hex 인코딩 (예: "3e", "01f4")
}

// wireUplink 는 디코드된 NasaMessage 의 무손실 업링크 표현이다.
// Checksum/Raw 는 재현에 불필요하므로 생략한다(서버는 Encode 로 프레임을 재구성).
type wireUplink struct {
	TS   int64            `json:"ts"`   // epoch milliseconds (UnixMilli)
	SA   string           `json:"sa"`   // Source Address, compact hex ("100000")
	DA   string           `json:"da"`   // Dest Address, compact hex
	Cmd  uint16           `json:"cmd"`  // CommandCode
	Seq  uint8            `json:"seq"`  // SequenceNum
	Sets []wireMessageSet `json:"sets"` // MessageSet 목록
}

// wireControl 은 제어 명령의 다운링크 표현이다. 필드는 processRequest 스키마와
// 1:1 대응하여 게이트웨이가 그대로 Process 입력 JSON 으로 매핑한다.
type wireControl struct {
	TS       int64          `json:"ts"`                  // epoch milliseconds
	Command  string         `json:"command"`             // set_power/set_mode/target_temperature/set_fan_speed/set_multiple
	DeviceID string         `json:"device_id,omitempty"` // 대상 device_id (우선)
	Address  string         `json:"address,omitempty"`   // 대상 주소 compact/spaced hex
	Params   map[string]any `json:"params,omitempty"`    // 명령별 파라미터 (Process params 스키마 동일)
	ReqID    string         `json:"req_id,omitempty"`    // ack 상관 키 (선택)
}

// toWireUplink 는 디코드된 NasaMessage 를 업링크 와이어 구조로 변환한다.
// ts 는 호출 시각(UnixMilli)으로 채운다.
func toWireUplink(msg *NasaMessage) wireUplink {
	sets := make([]wireMessageSet, 0, len(msg.MessageSets))
	for _, ms := range msg.MessageSets {
		sets = append(sets, wireMessageSet{
			Index: ms.Index,
			Value: hex.EncodeToString(ms.Value),
		})
	}
	return wireUplink{
		TS:   time.Now().UnixMilli(),
		SA:   msg.SourceAddr.Hex(),
		DA:   msg.DestAddr.Hex(),
		Cmd:  msg.CommandCode,
		Seq:  msg.SequenceNum,
		Sets: sets,
	}
}

// toNasaMessage 는 업링크 와이어 구조를 NasaMessage 로 역변환한다.
// SA/DA 는 ParseNasaAddress(compact hex 수용)로 파싱하며, 각 세트 value 는 hex 디코드
// 후 MessageSetValueSize 규칙과 길이가 일치하는지 검증한다(가변/미지원 니블 -1 은 예외).
// 검증 실패 시 명시적 에러를 반환한다(silent 수용 금지, 보안).
func (w wireUplink) toNasaMessage() (*NasaMessage, error) {
	sa, err := ParseNasaAddress(w.SA)
	if err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 mirror wire: invalid sa %q: %w", w.SA, err)
	}
	da, err := ParseNasaAddress(w.DA)
	if err != nil {
		return nil, fmt.Errorf("samsung_hvacr01 mirror wire: invalid da %q: %w", w.DA, err)
	}

	sets := make([]NasaMessageSet, 0, len(w.Sets))
	for _, ws := range w.Sets {
		val, decErr := hex.DecodeString(ws.Value)
		if decErr != nil {
			return nil, fmt.Errorf("samsung_hvacr01 mirror wire: invalid hex value for index 0x%04X: %w", ws.Index, decErr)
		}
		// MessageSetValueSize 규칙 검증: 고정 크기 니블(1/2/4)은 길이 일치 필수.
		if size := MessageSetValueSize(ws.Index); size >= 0 && len(val) != size {
			return nil, fmt.Errorf("samsung_hvacr01 mirror wire: index 0x%04X value size %d != expected %d",
				ws.Index, len(val), size)
		}
		sets = append(sets, NasaMessageSet{Index: ws.Index, Value: val})
	}

	return &NasaMessage{
		SourceAddr:  sa,
		DestAddr:    da,
		CommandCode: w.Cmd,
		SequenceNum: w.Seq,
		MessageSets: sets,
	}, nil
}

// toWireControl 은 processRequest 를 제어 다운링크 와이어 구조로 변환한다.
func toWireControl(req *processRequest) wireControl {
	return wireControl{
		TS:       time.Now().UnixMilli(),
		Command:  req.Command,
		DeviceID: req.DeviceID,
		Address:  req.Address,
		Params:   req.Params,
	}
}

// toProcessRequest 는 제어 다운링크 와이어 구조를 게이트웨이 Process 입력으로 변환한다.
func (w wireControl) toProcessRequest() processRequest {
	return processRequest{
		Command:  w.Command,
		Address:  w.Address,
		DeviceID: w.DeviceID,
		Params:   w.Params,
	}
}
