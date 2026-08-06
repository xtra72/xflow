package node

// SPEC-HVACR-SYNC-001 M10: 통합 SamsungHvacr01Node 의 mirror-message 노드 브리지 테스트.
//
// 검증 대상:
//   - ExtraSourceChannels 가 mirror-message 에이전트에서 "mirror-out" 포트를 노출.
//   - mirror-out: 에이전트 tap → 노드가 mirror.uplink 메시지로 방출.
//   - Process marker 판별: mirror.uplink 메시지 → FeedMirrorWire(에이전트 상태 반영),
//     비-marker 메시지 → 기존 처리로 폴백(에이전트에 mirror-in 급전하지 않음).
//   - 비-message 에이전트: mirror-out 포트 없음(gating).

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/samsung"
	"github.com/xtra/xflow/pkg/message"
)

// newStartedMessageAgent 는 mirror-message 모드 samsung 에이전트를 생성·Start 한다.
func newStartedMessageAgent(t *testing.T) *samsung.Hvacr01Agent {
	t.Helper()
	ag, err := samsung.NewHvacr01Agent(agent.AgentConfig{
		ID:   "msg-1",
		Name: "msg-1",
		Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{
			Type:    "mirror-message",
			Options: map[string]any{"transport_type": "mirror-message"},
		},
	})
	require.NoError(t, err)
	hv, ok := ag.(*samsung.Hvacr01Agent)
	require.True(t, ok, "mirror-message 에이전트는 *samsung.Hvacr01Agent 여야 함")
	require.NoError(t, hv.Start(context.Background()))
	t.Cleanup(func() { _ = hv.Stop(context.Background()) })
	return hv
}

// makeRunningSamsungCombinedNode 는 주입된 에이전트로 통합 노드를 Init 까지 구동한다.
func makeRunningSamsungCombinedNode(t *testing.T, ag agent.Agent) *SamsungHvacr01Node {
	t.Helper()
	def := newSamsungHvacr01NodeDef("sm", "samsung_hvacr01")
	resolver := &mockNASAResolver{transport: &mockNASATransport{agent: ag}}
	nn, err := NewSamsungHvacr01Node(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	sn, ok := nn.(*SamsungHvacr01Node)
	require.True(t, ok)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "sm-1"}))
	require.NoError(t, sn.Init(context.Background()))
	t.Cleanup(func() { _ = sn.Shutdown(context.Background()) })
	return sn
}

// buildMirrorWireJSON 은 wireUplink 스키마와 동일한 업링크 와이어 JSON 을 손으로 구성한다
// (samsung 패키지의 wireUplink 는 unexported 이므로 노드 테스트에서 직접 구성).
func buildMirrorWireJSON(t *testing.T, sa string, power byte, mode string) []byte {
	t.Helper()
	modeVal, ok := samsung.StringToMode[mode]
	require.True(t, ok)
	type wset struct {
		Index uint16 `json:"index"`
		Value string `json:"value"`
	}
	w := struct {
		TS   int64  `json:"ts"`
		SA   string `json:"sa"`
		DA   string `json:"da"`
		Cmd  uint16 `json:"cmd"`
		Seq  uint8  `json:"seq"`
		Sets []wset `json:"sets"`
	}{
		TS:  1,
		SA:  sa,
		DA:  "6aeeff",
		Cmd: samsung.CmdNotification,
		Seq: 1,
		Sets: []wset{
			{Index: samsung.MsgPower, Value: hex.EncodeToString([]byte{power})},
			{Index: samsung.MsgMode, Value: hex.EncodeToString([]byte{modeVal})},
		},
	}
	b, err := json.Marshal(w)
	require.NoError(t, err)
	return b
}

// TestSamsungCombinedNode_MirrorOut_Emits 는 message 모드에서 ExtraSourceChannels 가
// "mirror-out" 을 노출하고, 에이전트 tap 이 노드의 mirror-out 채널로 mirror.uplink
// 메시지를 방출하는지 검증한다 (M10 mirror-out).
func TestSamsungCombinedNode_MirrorOut_Emits(t *testing.T) {
	hv := newStartedMessageAgent(t)
	sn := makeRunningSamsungCombinedNode(t, hv)

	extra := sn.ExtraSourceChannels()
	require.NotNil(t, extra, "message 모드는 ExtraSourceChannels 를 제공해야 함")
	outCh, ok := extra["mirror-out"]
	require.True(t, ok, "mirror-out 포트 채널이 있어야 함")

	// 에이전트 ingress 로 업링크 급전 → 디코드 → tap → mirror-out.
	wire := buildMirrorWireJSON(t, "200000", 0x01, "cool")
	require.NoError(t, hv.FeedMirrorWire(wire))

	select {
	case msg := <-outCh:
		require.Equal(t, mirrorUplinkMsgType, msg.Type())
		v, ok := msg.Payload().Get(mirrorWirePayloadKey)
		require.True(t, ok, "mirror-out payload 에 와이어 키가 있어야 함")
		s, ok := v.(string)
		require.True(t, ok)
		var probe struct {
			SA string `json:"sa"`
		}
		require.NoError(t, json.Unmarshal([]byte(s), &probe))
		require.Equal(t, "200000", probe.SA)
	case <-time.After(2 * time.Second):
		t.Fatal("mirror-out 이 방출되지 않음")
	}
}

// TestSamsungCombinedNode_Process_MirrorIn 는 mirror.uplink marker 메시지가 Process 에서
// FeedMirrorWire 로 급전되어 에이전트 상태에 반영되고, 하류 emit 이 없음을 검증한다 (M10).
func TestSamsungCombinedNode_Process_MirrorIn(t *testing.T) {
	hv := newStartedMessageAgent(t)
	sn := makeRunningSamsungCombinedNode(t, hv)

	wire := buildMirrorWireJSON(t, "200000", 0x01, "cool")
	msg := message.New()
	msg.SetType(mirrorUplinkMsgType)
	msg.Payload().Set(mirrorWirePayloadKey, string(wire))

	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Empty(t, results, "mirror-in 은 하류로 emit 하지 않아야 함")

	addr := samsung.NasaAddress{0x20, 0x00, 0x00}
	require.Eventually(t, func() bool {
		st, gerr := hv.GetDeviceState(addr)
		return gerr == nil && st != nil && st.Power
	}, 2*time.Second, 5*time.Millisecond, "mirror-in 급전이 에이전트 상태에 반영되어야 함")
}

// TestSamsungCombinedNode_Process_NonMarkerFallthrough 는 비-marker 메시지가 mirror-in 으로
// 오인되지 않고 기존 상태 처리로 폴백함을 검증한다(에이전트에 mirror 디바이스 미등록).
func TestSamsungCombinedNode_Process_NonMarkerFallthrough(t *testing.T) {
	hv := newStartedMessageAgent(t)
	sn := makeRunningSamsungCombinedNode(t, hv)

	// 비-marker 상태 트리거(제어 키 없음, Type 비어있음) → buildStatusCommand 경로.
	msg := message.New()
	_, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)

	// 폴백 경로는 mirror-in 급전을 하지 않으므로 200000 디바이스가 등록되지 않아야 한다.
	addr := samsung.NasaAddress{0x20, 0x00, 0x00}
	st, gerr := hv.GetDeviceState(addr)
	require.True(t, gerr != nil || st == nil || !st.Power,
		"비-marker 메시지는 mirror-in 으로 급전되면 안 됨")
}

// TestSamsungCombinedNode_NonMessageAgent_NoMirrorOut 는 비-message(serial) 에이전트에서
// mirror-out 포트가 노출되지 않음을 검증한다(gating). MirrorOutCh()==nil 이므로
// startMirrorOut 이 no-op 이고 ExtraSourceChannels 는 nil 이다.
func TestSamsungCombinedNode_NonMessageAgent_NoMirrorOut(t *testing.T) {
	ag, err := samsung.NewHvacr01Agent(agent.AgentConfig{
		ID:   "serial-1",
		Name: "serial-1",
		Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"transport_type": "serial",
				"serial_port":    "/dev/null",
			},
		},
	})
	require.NoError(t, err)
	sn := makeRunningSamsungCombinedNode(t, ag)

	require.Nil(t, sn.ExtraSourceChannels(),
		"비-message 에이전트는 mirror-out 포트를 노출하면 안 됨")
}
