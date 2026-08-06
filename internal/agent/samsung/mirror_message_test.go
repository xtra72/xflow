package samsung

// SPEC-HVACR-SYNC-001 M10: mirror-message 모드 (플로우 노드 I/O 중계) 백엔드 시임 테스트.
//
// 검증 대상(AC-10.x):
//   - transport_type "mirror-message" 선택 → MirrorMessageMode + 채널 급전형 transport,
//     브로커/게이트웨이 식별자 없이도 유효(MQTT 미사용).
//   - FeedMirrorWire: mirror-in 와이어 JSON → 에이전트 상태 반영(왕복).
//   - MirrorOutCh: 디코드 메시지 tap 이 mirror-out 채널로 방출.
//   - 역할 격리: mirror-message 가 아닌 역할에서 FeedMirrorWire 는 에러, MirrorOutCh 는 nil.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// messageConfig 는 mirror-message 모드 에이전트 설정을 반환한다(MQTT 필드 없음).
func messageConfig(id string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   id,
		Name: "message-mode",
		Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{
			Type: "mirror-message",
			Options: map[string]any{
				"transport_type": "mirror-message",
			},
		},
	}
}

// TestMirrorMessageTransportSelection 은 "mirror-message" transport_type 이 config switch 와
// 팩토리에 등록되고, MQTT 자격 없이도 유효함을 검증한다 (AC-10.1).
func TestMirrorMessageTransportSelection(t *testing.T) {
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type": "mirror-message",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config(mirror-message): %v", err)
	}
	if !cfg.MirrorMessageMode {
		t.Error("mirror-message 는 MirrorMessageMode 를 활성화해야 함")
	}
	if cfg.MirrorMode {
		t.Error("mirror-message 는 MirrorMode(mirror-mqtt) 를 활성화하면 안 됨")
	}

	tr, err := NewNasaTransport("mirror-message", nil)
	if err != nil {
		t.Fatalf("NewNasaTransport(mirror-message): %v", err)
	}
	if _, ok := tr.(*mirrorTransport); !ok {
		t.Errorf("mirror-message 팩토리는 *mirrorTransport 를 반환해야 함, got %T", tr)
	}
}

// TestFeedMirrorWireRoundTrip 은 mirror-in 와이어 JSON 급전이 기존 receiveLoop → Decode →
// handleMessage 경로를 통해 에이전트 상태에 반영되는지 검증한다 (AC-10.2).
func TestFeedMirrorWireRoundTrip(t *testing.T) {
	addr := NasaAddress{0x20, 0x00, 0x00}

	a := mustAgent(t, messageConfig("m1"))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// mirror-in 업링크 와이어 구성(power=on, mode=cool).
	msg := &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    NasaAddress{0x6A, 0xEE, 0xFF},
		CommandCode: CmdNotification,
		SequenceNum: 1,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{StringToMode["cool"]}},
		},
	}
	payload, err := json.Marshal(toWireUplink(msg))
	if err != nil {
		t.Fatalf("marshal wireUplink: %v", err)
	}

	if err := a.FeedMirrorWire(payload); err != nil {
		t.Fatalf("FeedMirrorWire: %v", err)
	}

	eventually(t, 2*time.Second, func() bool {
		st, gerr := a.GetDeviceState(addr)
		return gerr == nil && st != nil && st.Power
	})
	st, err := a.GetDeviceState(addr)
	if err != nil {
		t.Fatalf("GetDeviceState: %v", err)
	}
	if !st.Power || st.Mode != "cool" {
		t.Errorf("mirror-in 상태 미반영: power=%v mode=%q", st.Power, st.Mode)
	}
}

// TestMirrorOutChEmits 는 디코드된 메시지가 mirror-out 채널로 toWireUplink JSON 형태로
// 방출되는지 검증한다 (AC-10.3).
func TestMirrorOutChEmits(t *testing.T) {
	addr := NasaAddress{0x20, 0x00, 0x00}

	a := mustAgent(t, messageConfig("m2"))
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	outCh := a.MirrorOutCh()
	if outCh == nil {
		t.Fatal("mirror-message 모드는 MirrorOutCh 를 제공해야 함")
	}

	// RX 프레임 주입 → 디코드 → tap → mirror-out 채널.
	a.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	select {
	case out := <-outCh:
		var w wireUplink
		if err := json.Unmarshal(out, &w); err != nil {
			t.Fatalf("mirror-out 역직렬화: %v", err)
		}
		if w.SA != addr.Hex() {
			t.Errorf("mirror-out SA=%q, want %q", w.SA, addr.Hex())
		}
	case <-time.After(time.Second):
		t.Fatal("mirror-out 이 방출되지 않음")
	}
}

// TestMirrorMessageRoleIsolation 은 mirror-message 가 아닌 역할에서 FeedMirrorWire 가 에러를
// 반환하고 MirrorOutCh 가 nil 임을 검증한다 (역할 격리).
func TestMirrorMessageRoleIsolation(t *testing.T) {
	// mirror-mqtt(server) 역할: mirror-message 전용 시임은 비활성이어야 한다.
	srv := mustAgent(t, serverConfig("gw01"))
	if err := srv.FeedMirrorWire([]byte(`{}`)); err == nil {
		t.Error("mirror-mqtt 모드에서 FeedMirrorWire 는 에러를 반환해야 함")
	}
	if srv.MirrorOutCh() != nil {
		t.Error("mirror-mqtt 모드에서 MirrorOutCh 는 nil 이어야 함")
	}
}
