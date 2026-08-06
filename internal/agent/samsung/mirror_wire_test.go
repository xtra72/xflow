package samsung

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestWireRoundTrip 은 NasaMessage → wire JSON → NasaMessage 라운드트립이 무손실임을
// 검증한다 (AC-5.1). 다양한 index 니블 크기(1/2/4 byte)를 포함한다.
func TestWireRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		msg  *NasaMessage
	}{
		{
			name: "1바이트/2바이트 니블 혼합 (power+target_temp)",
			msg: &NasaMessage{
				SourceAddr:  NasaAddress{0x20, 0x00, 0x00},
				DestAddr:    NasaAddress{0x6A, 0xEE, 0xFF},
				CommandCode: CmdNotification,
				SequenceNum: 0x42,
				MessageSets: []NasaMessageSet{
					{Index: MsgPower, Value: []byte{0x01}},            // 1바이트 (니블 0)
					{Index: MsgTargetTemp, Value: []byte{0x00, 0xC8}}, // 2바이트 (니블 2)
					{Index: MsgCurrentHumidity, Value: []byte{0x3E}},  // 1바이트
				},
			},
		},
		{
			name: "4바이트 니블 (addr info)",
			msg: &NasaMessage{
				SourceAddr:  NasaAddress{0x10, 0x00, 0x00},
				DestAddr:    NasaAddress{0xB0, 0xFF, 0xFF},
				CommandCode: CmdNormalRequest,
				SequenceNum: 0x00,
				MessageSets: []NasaMessageSet{
					{Index: MsgAddrInfo, Value: []byte{0xFF, 0xFF, 0xFF, 0xFF}}, // 4바이트 (니블 4)
				},
			},
		},
		{
			name: "세트 없음",
			msg: &NasaMessage{
				SourceAddr:  NasaAddress{0x20, 0x01, 0x02},
				DestAddr:    NasaAddress{0x20, 0x00, 0x00},
				CommandCode: CmdControlResponse,
				SequenceNum: 0xFF,
				MessageSets: []NasaMessageSet{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 직렬화 → JSON → 역직렬화
			payload, err := json.Marshal(toWireUplink(tc.msg))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var w wireUplink
			if err := json.Unmarshal(payload, &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, err := w.toNasaMessage()
			if err != nil {
				t.Fatalf("toNasaMessage: %v", err)
			}

			// ts 제외 무손실: SA/DA/Cmd/Seq/Sets 동일해야 한다.
			if got.SourceAddr != tc.msg.SourceAddr {
				t.Errorf("SA: got %v want %v", got.SourceAddr, tc.msg.SourceAddr)
			}
			if got.DestAddr != tc.msg.DestAddr {
				t.Errorf("DA: got %v want %v", got.DestAddr, tc.msg.DestAddr)
			}
			if got.CommandCode != tc.msg.CommandCode {
				t.Errorf("Cmd: got 0x%04X want 0x%04X", got.CommandCode, tc.msg.CommandCode)
			}
			if got.SequenceNum != tc.msg.SequenceNum {
				t.Errorf("Seq: got %d want %d", got.SequenceNum, tc.msg.SequenceNum)
			}
			if !reflect.DeepEqual(got.MessageSets, tc.msg.MessageSets) {
				t.Errorf("Sets: got %+v want %+v", got.MessageSets, tc.msg.MessageSets)
			}

			// Encode 재구성 후 Decode 라운드트립도 무손실이어야 한다(서버 급전 경로).
			proto := NewNasaProtocol()
			frame, err := proto.Encode(got)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			decoded, err := proto.Decode(frame)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if decoded.SourceAddr != tc.msg.SourceAddr || decoded.CommandCode != tc.msg.CommandCode {
				t.Errorf("Encode→Decode 라운드트립 불일치: %+v", decoded)
			}
			if !reflect.DeepEqual(decoded.MessageSets, tc.msg.MessageSets) {
				t.Errorf("Encode→Decode sets 불일치: got %+v want %+v", decoded.MessageSets, tc.msg.MessageSets)
			}
		})
	}
}

// TestWireValueSizeValidation 은 MessageSetValueSize 규칙을 위반하는 value 크기를
// 역직렬화가 거부하는지 검증한다 (silent 수용 금지, 보안).
func TestWireValueSizeValidation(t *testing.T) {
	// MsgPower(0x4000, 니블 0)는 1바이트인데 2바이트를 넣으면 거부되어야 한다.
	w := wireUplink{
		SA: "200000", DA: "6AEEFF", Cmd: CmdNotification, Seq: 1,
		Sets: []wireMessageSet{{Index: MsgPower, Value: "0102"}}, // 2바이트 (잘못됨)
	}
	if _, err := w.toNasaMessage(); err == nil {
		t.Fatal("잘못된 value 크기를 수용하면 안 됨")
	}

	// 잘못된 hex 문자열도 거부.
	w2 := wireUplink{
		SA: "200000", DA: "6AEEFF", Cmd: CmdNotification, Seq: 1,
		Sets: []wireMessageSet{{Index: MsgPower, Value: "zz"}},
	}
	if _, err := w2.toNasaMessage(); err == nil {
		t.Fatal("잘못된 hex 를 수용하면 안 됨")
	}

	// 잘못된 SA hex 거부.
	w3 := wireUplink{SA: "xyz", DA: "6AEEFF", Cmd: 1, Seq: 1}
	if _, err := w3.toNasaMessage(); err == nil {
		t.Fatal("잘못된 SA 를 수용하면 안 됨")
	}
}

// TestWireControlRoundTrip 은 제어 wire ↔ processRequest 매핑을 검증한다.
func TestWireControlRoundTrip(t *testing.T) {
	req := &processRequest{
		Command:  "set_multiple",
		DeviceID: "living-room",
		Address:  "20.00.00",
		Params:   map[string]any{"power": true, "target_temperature": 24.0},
	}
	w := toWireControl(req)
	if w.Command != req.Command || w.DeviceID != req.DeviceID || w.Address != req.Address {
		t.Fatalf("control wire 필드 불일치: %+v", w)
	}
	got := w.toProcessRequest()
	if got.Command != req.Command || got.DeviceID != req.DeviceID || got.Address != req.Address {
		t.Fatalf("processRequest 역매핑 불일치: %+v", got)
	}
	if !reflect.DeepEqual(got.Params, req.Params) {
		t.Fatalf("params 불일치: got %+v want %+v", got.Params, req.Params)
	}
}

// TestWireTimestampEpochMillis 는 ts 가 int64 epoch millis 로 직렬화되는지 검증한다
// (AC-5.2, RFC3339/time.Time 문자열이 아님).
func TestWireTimestampEpochMillis(t *testing.T) {
	msg := &NasaMessage{SourceAddr: NasaAddress{0x20, 0, 0}, DestAddr: NasaAddress{0x20, 0, 0}}
	payload, err := json.Marshal(toWireUplink(msg))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var ts int64
	if err := json.Unmarshal(raw["ts"], &ts); err != nil {
		t.Fatalf("ts 는 int64 여야 함: %v", err)
	}
	if ts <= 0 {
		t.Fatalf("ts 는 양수 epoch millis 여야 함: %d", ts)
	}
}

// TestWireNoGatewayID 는 wire payload 에 gateway_id 필드가 없음을 검증한다 (AC-5.3).
func TestWireNoGatewayID(t *testing.T) {
	upMsg := &NasaMessage{SourceAddr: NasaAddress{0x20, 0, 0}, DestAddr: NasaAddress{0x20, 0, 0}}
	upPayload, _ := json.Marshal(toWireUplink(upMsg))
	var upMap map[string]any
	_ = json.Unmarshal(upPayload, &upMap)
	if _, ok := upMap["gateway_id"]; ok {
		t.Error("업링크 wire 에 gateway_id 가 있으면 안 됨(토픽이 식별 전담)")
	}

	ctrlPayload, _ := json.Marshal(toWireControl(&processRequest{Command: "set_power"}))
	var ctrlMap map[string]any
	_ = json.Unmarshal(ctrlPayload, &ctrlMap)
	if _, ok := ctrlMap["gateway_id"]; ok {
		t.Error("다운링크 wire 에 gateway_id 가 있으면 안 됨")
	}
}

// TestMirrorTopics 는 토픽 빌더가 업링크/다운링크를 분리하고 gateway_id 를 치환하는지
// 검증한다 (AC-6.1).
func TestMirrorTopics(t *testing.T) {
	tp := newMirrorTopics("", "gw01") // 빈 prefix → 기본값
	if got := tp.uplinkNasa(); got != "xflow/hvacr/gw01/up/nasa" {
		t.Errorf("uplinkNasa: %s", got)
	}
	if got := tp.downlinkControl(); got != "xflow/hvacr/gw01/down/control" {
		t.Errorf("downlinkControl: %s", got)
	}
	if got := tp.uplinkAck(); got != "xflow/hvacr/gw01/up/ack" {
		t.Errorf("uplinkAck: %s", got)
	}
	if got := tp.snapshot("100000"); got != "xflow/hvacr/gw01/up/snapshot/100000" {
		t.Errorf("snapshot: %s", got)
	}

	// 업링크와 다운링크는 서로 다른 토픽이어야 한다(방향 격리, AC-4.3).
	if tp.uplinkNasa() == tp.downlinkControl() {
		t.Error("업링크/다운링크 토픽이 동일하면 안 됨")
	}

	// custom prefix.
	tp2 := newMirrorTopics("site/a", "gwX")
	if got := tp2.uplinkNasa(); got != "site/a/gwX/up/nasa" {
		t.Errorf("custom prefix uplinkNasa: %s", got)
	}
}
