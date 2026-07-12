package lg

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// SPEC-HVACR-CONNSTATE-001: LGAP connection-state 리포팅 테스트
// ---------------------------------------------------------------------------

// connMockTransport 는 LGAPTransport 의 최소 테스트 구현체이다.
type connMockTransport struct {
	mu        sync.Mutex
	available atomic.Bool
}

func (m *connMockTransport) Open() error  { m.available.Store(true); return nil }
func (m *connMockTransport) Close() error { m.available.Store(false); return nil }
func (m *connMockTransport) Send(_ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}
func (m *connMockTransport) Receive(_ []byte) (int, error) { return 0, nil }
func (m *connMockTransport) Available() bool               { return m.available.Load() }
func (m *connMockTransport) Write(data []byte) (int, error) {
	return len(data), nil
}
func (m *connMockTransport) setAvailable(v bool) { m.available.Store(v) }

// newConnLGAPAgent 는 connection-state 테스트용 LGAP 에이전트를 생성한다.
// 모든 device 는 Online=false 로 시작한다. zones 는 zone byte 목록.
func newConnLGAPAgent(t *testing.T, connReport, probeTimeout time.Duration, zones ...byte) (*LGAPAgent, *connMockTransport) {
	t.Helper()

	mt := &connMockTransport{}
	mt.available.Store(true)

	a := &LGAPAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lgap")),
		agentConfig: agent.AgentConfig{
			ID:   "test-lgap-conn",
			Name: "test-lgap-conn",
			Type: "lgap",
		},
		lgapConfig: LGAPConfig{
			SerialPort:               "/dev/ttyTest",
			PollInterval:             30 * time.Second,
			OfflineThreshold:         3,
			MsgChannelSize:           256,
			ReconnectInterval:        10 * time.Millisecond,
			MaxReconnectBackoff:      50 * time.Millisecond,
			ConnectionReportInterval: connReport,
			StartupProbeTimeout:      probeTimeout,
			EventTempThreshold:       1.0,
		},
		devices:    make(map[byte]*LGAPDevice),
		deviceIDs:  make(map[string]byte),
		transport:  mt,
		protocol:   NewLGAPProtocol(),
		lastStates: make(map[byte]LGAPDeviceState),
		stopCh:     make(chan struct{}),
		msgCh:      make(chan []byte, 256),
		stats:      agent.NewAgentStats(),
		logger:     agent.ResolveLogger(agent.AgentConfig{}),
		createdAt:  time.Now(),
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		t.Fatalf("transition initializing: %v", err)
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		t.Fatalf("transition running: %v", err)
	}
	a.startedAt = time.Now()

	for _, z := range zones {
		name := zoneName(z)
		a.devices[z] = &LGAPDevice{
			Zone:   z,
			UnitID: name,
			Online: false,
			State:  &LGAPDeviceState{},
			Source: "config",
		}
		a.deviceIDs[name] = z
	}

	return a, mt
}

func zoneName(z byte) string {
	return "zone-" + string(rune('A'+int(z)))
}

// onlineResponse 는 정상(Error=0) 응답을 만든다(device online 유도).
func onlineResponse(zone byte) *LGAPResponse {
	return &LGAPResponse{
		Zone:      zone,
		Error:     0,
		ModeCombo: 0x00,
	}
}

// drainConnMsgsLG 는 msgCh 에서 device_connection.* 메시지만 파싱해 반환한다.
func drainConnMsgsLG(t *testing.T, a *LGAPAgent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for {
		select {
		case b := <-a.msgCh:
			m := parseConnMsgLG(b)
			if m != nil {
				out = append(out, m)
			}
		default:
			return out
		}
	}
}

func parseConnMsgLG(b []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	md, ok := m["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	mt, _ := md["message_type"].(string)
	if !strings.HasPrefix(mt, "device_connection.") {
		return nil
	}
	return m
}

func assertConnEnvelopeLG(t *testing.T, m map[string]any, wantTrigger string) {
	t.Helper()
	assert.Contains(t, m, "unit_id")
	assert.Contains(t, m, "device_id")
	assert.Equal(t, wantTrigger, m["trigger"])
	assert.Contains(t, m, "event_ms")
	assert.NotContains(t, m, "type", "최상위 type 필드는 없어야 한다 (AC-17)")
	cs, _ := m["connection_state"].(string)
	assert.True(t, cs == "online" || cs == "offline")
	md := m["metadata"].(map[string]any)
	assert.Equal(t, "device_connection."+wantTrigger, md["message_type"])
}

// AC-5: startup probe 성공 → 초기 online 방출.
func TestConnLG_StartupProbeSuccess_OnlineInitial(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, 200*time.Millisecond, 0x10)
	a.handleResponse(0x10, onlineResponse(0x10))

	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "initial", msgs[0]["trigger"])
	assert.Equal(t, "online", msgs[0]["connection_state"])
	assertConnEnvelopeLG(t, msgs[0], "initial")
}

// AC-6: startup probe 실패 → 초기 offline (threshold 대기 없음) + 이후 online change.
func TestConnLG_StartupProbeFail_OfflineInitial_NoThreshold(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, 50*time.Millisecond, 0x10)

	assert.True(t, a.resolveStartupProbes(true))
	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "initial", msgs[0]["trigger"])
	assert.Equal(t, "offline", msgs[0]["connection_state"])

	a.mu.RLock()
	ec := a.devices[0x10].ErrorCount
	a.mu.RUnlock()
	assert.Equal(t, 0, ec, "threshold 소진 없이 즉시 offline (S4/N8)")

	a.handleResponse(0x10, onlineResponse(0x10))
	msgs = drainConnMsgsLG(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "change", msgs[0]["trigger"])
	assert.Equal(t, "online", msgs[0]["connection_state"])
}

// AC-7: Start 시 transport 미가용 → 전체 device 초기 offline.
func TestConnLG_TransportUnavailable_AllOfflineInitial(t *testing.T) {
	a, mt := newConnLGAPAgent(t, 0, 60*time.Millisecond, 0x10, 0x11, 0x12)
	mt.setAvailable(false)

	a.connWg.Add(1)
	go a.startupProbeLoop()
	a.connWg.Wait()

	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 3)
	seen := map[string]bool{}
	for _, m := range msgs {
		assert.Equal(t, "initial", m["trigger"])
		assert.Equal(t, "offline", m["connection_state"])
		seen[m["unit_id"].(string)] = true
	}
	assert.Len(t, seen, 3)
}

// AC-8: Start 는 probe 완료를 기다리지 않고 즉시 반환.
func TestConnLG_StartReturnsImmediately(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, 5*time.Second, 0x10, 0x11)
	start := time.Now()
	require.NoError(t, a.Start(context.Background()))
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 500*time.Millisecond)
	require.NoError(t, a.Stop(context.Background()))
}

// AC-9 / S5: initial 미방출 device 는 주기 report 에서 제외.
func TestConnLG_ReportSkipsUnemittedInitial(t *testing.T) {
	a, _ := newConnLGAPAgent(t, time.Hour, 5*time.Second, 0x10, 0x11)
	a.handleResponse(0x10, onlineResponse(0x10))
	_ = drainConnMsgsLG(t, a)

	a.emitConnectionReports()
	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 1, "initial 방출된 device 만 report (S5)")
	assert.Equal(t, "report", msgs[0]["trigger"])
}

// AC-10: in-flight probe 중 Stop → leak 없음, Stop 이후 방출 없음.
func TestConnLG_StopDuringProbe_NoLeakNoEmit(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, 5*time.Second, 0x10, 0x11)
	require.NoError(t, a.Start(context.Background()))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, a.Stop(context.Background()))

	msgs := drainConnMsgsLG(t, a)
	assert.Empty(t, msgs, "Stop 이후 어떤 메시지도 남지 않아야 한다 (N10/AC-10)")
}

// AC-15: bulk offline 시 device 당 개별 change (offline), 두 번째 initial 아님.
func TestConnLG_BulkOffline_IndividualChanges(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 0, 5*time.Second, 0x10, 0x11, 0x12)
	// 세 device online + initial 확정
	a.handleResponse(0x10, onlineResponse(0x10))
	a.handleResponse(0x11, onlineResponse(0x11))
	a.handleResponse(0x12, onlineResponse(0x12))
	_ = drainConnMsgsLG(t, a)

	a.setAllDevicesOffline()
	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 3, "M개 device → M개 개별 change")
	for _, m := range msgs {
		assert.Equal(t, "change", m["trigger"], "두 번째 initial 아님 (N9)")
		assert.Equal(t, "offline", m["connection_state"])
	}
}

// AC-13: offline device 가 주기 report 에 포함.
func TestConnLG_ReportIncludesOfflineDevices(t *testing.T) {
	a, _ := newConnLGAPAgent(t, time.Hour, 5*time.Second, 0x10, 0x11)
	a.handleResponse(0x10, onlineResponse(0x10)) // online
	a.mu.Lock()
	a.devices[0x11].connInitialEmitted = true // offline initial 확정 시뮬레이션
	a.mu.Unlock()
	_ = drainConnMsgsLG(t, a)

	a.emitConnectionReports()
	msgs := drainConnMsgsLG(t, a)
	require.Len(t, msgs, 2)
	states := map[string]string{}
	for _, m := range msgs {
		states[m["unit_id"].(string)] = m["connection_state"].(string)
	}
	assert.Equal(t, "offline", states[zoneName(0x11)], "offline device 포함 (S2/N5)")
}

// AC-20: 신규 goroutine 은 Stop 시 connWg 로 join (leak 없음).
func TestConnLG_NewGoroutinesJoinedOnStop(t *testing.T) {
	a, _ := newConnLGAPAgent(t, 20*time.Millisecond, 5*time.Second, 0x10)
	require.NoError(t, a.Start(context.Background()))
	time.Sleep(30 * time.Millisecond)
	// Stop 이 connWg.Wait() 로 blocking 되지 않고 반환하면 join 성공.
	done := make(chan struct{})
	go func() { _ = a.Stop(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 이 신규 goroutine join 에서 hang (AC-20)")
	}
}

// AC-19 (deadlock 회귀 방지): lock 보유 경로 방출이 a.Name()/a.ID() 를 호출하지 않음.
func TestConnLG_NoDeadlockUnderLock(t *testing.T) {
	a, _ := newConnLGAPAgent(t, time.Hour, 30*time.Millisecond, 0x10, 0x11)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		a.connWg.Add(1)
		go a.startupProbeLoop()
		a.connWg.Wait()
		a.handleResponse(0x10, onlineResponse(0x10))
		for i := 0; i < a.lgapConfig.OfflineThreshold; i++ {
			a.incrementErrorCount(0x10)
		}
		a.setAllDevicesOffline()
		a.emitConnectionReports()
	}()
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock 의심: lock 보유 중 a.Name()/a.ID() 호출 회귀 (N1/AC-19)")
	}
}

// fakeLGDeviceIDRepo 는 agent.DeviceIDRepository 의 테스트 구현체이다.
type fakeLGDeviceIDRepo struct{ m map[string]string }

func (r *fakeLGDeviceIDRepo) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	k := agentName + ":" + unitID
	if v, ok := r.m[k]; ok {
		return v, nil
	}
	v := "uuid-" + agentName + "-" + unitID
	r.m[k] = v
	return v, nil
}

func (r *fakeLGDeviceIDRepo) Get(_ context.Context, agentName, unitID string) (string, error) {
	return r.m[agentName+":"+unitID], nil
}

// TestLGAPAgent_RemoveDevice_ByUUID 는 remove_device 가 UUID(1급 식별자)로도 디바이스를
// 제거하는지 검증한다. localID 는 레지스트리 어댑터와 동일한 formatZone(zone) 을 사용한다
// (프론트엔드/REST 가 받는 device.uid 의 근거).
func TestLGAPAgent_RemoveDevice_ByUUID(t *testing.T) {
	prev := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(&fakeLGDeviceIDRepo{m: make(map[string]string)})
	t.Cleanup(func() { agent.SetDeviceIDRepository(prev) })

	a, _ := newConnLGAPAgent(t, 0, 0, 0x01)
	a.devices[0x01].Source = "bridge" // config 는 삭제 보호되므로 bridge 로 변경

	uuid := agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, formatZone(0x01))
	if uuid == "" {
		t.Fatalf("precondition: UUID 해석 실패")
	}

	req, _ := json.Marshal(map[string]any{"command": "remove_device", "device_id": uuid})
	if _, err := a.Process(req); err != nil {
		t.Fatalf("remove_device by UUID: %v", err)
	}
	if _, ok := a.devices[0x01]; ok {
		t.Errorf("zone 0x01 디바이스가 UUID 삭제 후 제거되어야 함")
	}
}
