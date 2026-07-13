package samsung

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// Samsung 연결 상태(device_state 일원화) 공용 테스트 헬퍼 + probe/timeout 테스트.
//
// device_connection.* 별도 스트림을 제거하고 연결 정보를 device_state 로 일원화한 뒤,
// 아래 헬퍼는 recentSnapshots 링버퍼의 device_state 스냅샷을 소비한다. online 은
// state.online 에, 연결 진단 필드(error_count/offline_threshold/transport_connected)는
// state 그룹에 실린다.
// ---------------------------------------------------------------------------

// newConnAgent 는 연결 상태 테스트용 Samsung 에이전트를 생성한다.
// 모든 device 는 Online=false 로 시작한다(production 초기 상태와 일치).
func newConnAgent(t *testing.T, addrs ...string) (*Hvacr01Agent, *mockTransport) {
	t.Helper()

	mt := &mockTransport{available: true}
	mp := &mockProtocol{}

	a := &Hvacr01Agent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung_hvacr01")),
		agentConfig: agent.AgentConfig{
			ID:   "test-id",
			Name: "test-samsung-conn",
			Type: "samsung_hvacr01",
		},
		hvacr01Config: Hvacr01Config{
			TransportType:       "serial",
			SerialPort:          "/dev/ttyTest",
			PollInterval:        30 * time.Second,
			MsgChannelSize:      256,
			OfflineThreshold:    3,
			OfflineTimeout:      -1, // production 기본값(미설정) 미러 → staleOfflineThreshold 파생 경로
			ReconnectInterval:   10 * time.Millisecond,
			MaxReconnectBackoff: 50 * time.Millisecond,
		},
		devices:       make(map[NasaAddress]*NasaDevice),
		deviceIDs:     make(map[string]NasaAddress),
		transport:     mt,
		protocol:      mp,
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, 256),
		stats:         agent.NewAgentStats(),
		logger:        testLogger(),
		lastStates:    make(map[NasaAddress]NasaDeviceState),
		warnedUnknown: make(map[NasaAddress]bool),
		disconnectCh:  make(chan struct{}),
		createdAt:     time.Now(),
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		t.Fatalf("transition initializing: %v", err)
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		t.Fatalf("transition running: %v", err)
	}
	a.startedAt = time.Now()

	for _, s := range addrs {
		addr, err := ParseNasaAddress(s)
		require.NoError(t, err)
		a.devices[addr] = &NasaDevice{
			Address:       addr,
			UnitID:        s,
			Type:          "HVACR.IDU",
			Online:        false,
			State:         &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)},
			Source:        "config",
			ReportEnabled: true,
		}
		a.deviceIDs[s] = addr
	}

	return a, mt
}

// drainStateMsgs 는 마지막 호출 이후 recentSnapshots 링버퍼에 추가된 device_state
// 스냅샷을 watermark 기반으로 파싱해 반환한다. 연결 정보가 device_state 로 일원화되어
// 링버퍼의 모든 항목이 device_state 이므로 별도 필터 없이 전부 반환한다.
var (
	stateDrainMu        sync.Mutex
	stateDrainWatermark = map[*Hvacr01Agent]int64{}
)

func drainStateMsgs(t *testing.T, a *Hvacr01Agent) []map[string]any {
	t.Helper()
	a.recentMu.Lock()
	snaps := append([]recentStateEntry(nil), a.recentSnapshots...)
	a.recentMu.Unlock()

	stateDrainMu.Lock()
	last := stateDrainWatermark[a]
	maxSeq := last
	var out []map[string]any
	for _, s := range snaps {
		if s.Seq <= last {
			continue
		}
		if s.Seq > maxSeq {
			maxSeq = s.Seq
		}
		var m map[string]any
		if err := json.Unmarshal(s.Data, &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	stateDrainWatermark[a] = maxSeq
	stateDrainMu.Unlock()
	return out
}

// stateOnline 은 device_state 스냅샷의 state.online 값을 반환한다.
func stateOnline(m map[string]any) (bool, bool) {
	state, ok := m["state"].(map[string]any)
	if !ok {
		return false, false
	}
	v, ok := state["online"].(bool)
	return v, ok
}

// findStateMsg 는 (trigger, state.online) 이 일치하는 첫 device_state 스냅샷을 반환한다.
func findStateMsg(msgs []map[string]any, trigger string, online bool) map[string]any {
	for _, m := range msgs {
		if m["trigger"] != trigger {
			continue
		}
		if v, ok := stateOnline(m); ok && v == online {
			return m
		}
	}
	return nil
}

// onlineFrame 은 5 핵심 필드를 담은 정상 IDU 프레임을 만든다(device online 유도).
func onlineFrame(addr NasaAddress) *NasaMessage {
	return &NasaMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{0x01}},
			{Index: MsgFanSpeed, Value: []byte{0x02}},
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}},
			{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}},
		},
	}
}

// timeoutErr 은 net.Error(Timeout()==true) 를 구현하는 테스트용 에러이다.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// TestIsReadTimeout 은 읽기 데드라인 초과(수신 메시지 없음)만 timeout 으로 분류하고,
// 그 외 에러는 로그 대상으로 남기는지 검증한다.
func TestIsReadTimeout(t *testing.T) {
	assert.True(t, isReadTimeout(timeoutErr{}), "net.Error Timeout()==true 는 read timeout")
	assert.True(t, isReadTimeout(os.ErrDeadlineExceeded), "os.ErrDeadlineExceeded 는 read timeout")
	assert.True(t, isReadTimeout(fmt.Errorf("wrap: %w", timeoutErr{})), "래핑된 timeout 도 감지")
	assert.False(t, isReadTimeout(nil), "nil 은 timeout 아님")
	assert.False(t, isReadTimeout(errors.New("connection reset")), "일반 에러는 timeout 아님(로그 대상)")
}

// TestProbeSilentDevices_PassiveMode 는 passive 모드에서 침묵한 online 디바이스에
// 확인 probe(status query)가 전송되는지 검증한다.
//
// 회귀 배경: passive(status_query_enabled=false)는 블랭킷 폴을 안 하므로, 디바이스가
// 자체 브로드캐스트 주기(> offline 임계값)로만 통신하면 정상 online 인데도 90초 임계에
// 걸려 online/offline 플래핑했다. 침묵 시 확인 probe 로 응답을 유도하면 online 이
// 유지되어 플래핑이 사라진다.
func TestProbeSilentDevices_PassiveMode(t *testing.T) {
	a, mt := newConnAgent(t, "200001")
	a.hvacr01Config.StatusQueryEnabled = false // passive
	// staleness 활성 유지(OfflineTimeout=-1 파생). PollInterval(30s) 이 probe 임계.

	addr, err := ParseNasaAddress("200001")
	require.NoError(t, err)
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-40 * time.Second) // 30초 이상 침묵
	a.mu.Unlock()

	before := len(mt.getSentData())
	a.probeSilentDevices()
	assert.Greater(t, len(mt.getSentData()), before,
		"침묵한 online 디바이스에 확인 probe 가 전송되어야 함")
}

// TestProbeSilentDevices_FreshDeviceNotProbed 는 최근 수신한(침묵 아님) 디바이스는
// probe 하지 않는지 검증한다(불필요한 버스 트래픽 방지).
func TestProbeSilentDevices_FreshDeviceNotProbed(t *testing.T) {
	a, mt := newConnAgent(t, "200001")
	a.hvacr01Config.StatusQueryEnabled = false

	addr, err := ParseNasaAddress("200001")
	require.NoError(t, err)
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now() // 방금 수신 → 침묵 아님
	a.mu.Unlock()

	before := len(mt.getSentData())
	a.probeSilentDevices()
	assert.Equal(t, before, len(mt.getSentData()),
		"최근 수신 디바이스는 probe 하지 않아야 함")
}

// TestProbeSilentDevices_DisabledWhenStalenessOff 는 offline_timeout==0(staleness 비활성)
// 이면 probe 도 하지 않는지 검증한다.
func TestProbeSilentDevices_DisabledWhenStalenessOff(t *testing.T) {
	a, mt := newConnAgent(t, "200001")
	a.hvacr01Config.StatusQueryEnabled = false
	a.hvacr01Config.OfflineTimeout = 0 // staleness 비활성

	addr, err := ParseNasaAddress("200001")
	require.NoError(t, err)
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = time.Now().Add(-40 * time.Second)
	a.mu.Unlock()

	before := len(mt.getSentData())
	a.probeSilentDevices()
	assert.Equal(t, before, len(mt.getSentData()),
		"staleness 비활성이면 probe 도 불필요")
}

// TestDeviceState_IncludesConnectionFields 는 device_state 스냅샷의 state 그룹에
// online 과 함께 연결 진단 필드(error_count/offline_threshold/transport_connected)가
// 포함되는지 검증한다(device_connection 일원화의 핵심 계약).
func TestDeviceState_IncludesConnectionFields(t *testing.T) {
	a, mt := newConnAgent(t, "200001")
	addr, _ := ParseNasaAddress("200001")

	// 첫 online 프레임 → device_state change 방출(online=false→true 전이).
	a.handleMessage(onlineFrame(addr))

	msgs := drainStateMsgs(t, a)
	require.NotEmpty(t, msgs, "online 전이는 device_state 스냅샷을 방출해야 한다")
	m := msgs[len(msgs)-1]

	state, ok := m["state"].(map[string]any)
	require.True(t, ok, "state 그룹이 있어야 한다")
	assert.Equal(t, true, state["online"], "online 은 state 그룹에 있어야 한다")

	// error_count: 정상 프레임 수신으로 0 (JSON 이므로 float64).
	require.Contains(t, state, "error_count")
	assert.Equal(t, float64(0), state["error_count"])

	// offline_threshold: 설정값 3.
	require.Contains(t, state, "offline_threshold")
	assert.Equal(t, float64(a.hvacr01Config.OfflineThreshold), state["offline_threshold"])

	// transport_connected: mockTransport available=true.
	require.Contains(t, state, "transport_connected")
	assert.Equal(t, mt.Available(), state["transport_connected"])
}
