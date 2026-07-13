package lg

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// LGAP 에이전트 테스트 공용 지원(mock transport / 에이전트 팩토리 / 헬퍼).
//
// device_connection 스트림 제거(연결 정보를 device_state 로 일원화) 이후에도 device
// 상태/전이 관련 테스트가 공유하는 헬퍼를 여기에 보존한다.
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

// newConnLGAPAgent 는 device 상태/전이 테스트용 LGAP 에이전트를 생성한다.
// 모든 device 는 Online=false 로 시작한다. zones 는 zone byte 목록.
//
// connReport / probeTimeout 파라미터는 device_connection 제거 이후 무의미하지만,
// 기존 호출부 시그니처 호환을 위해 유지하며 값은 무시한다.
func newConnLGAPAgent(t *testing.T, _, _ time.Duration, zones ...byte) (*LGAPAgent, *connMockTransport) {
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
			SerialPort:          "/dev/ttyTest",
			PollInterval:        30 * time.Second,
			OfflineThreshold:    3,
			MsgChannelSize:      256,
			ReconnectInterval:   10 * time.Millisecond,
			MaxReconnectBackoff: 50 * time.Millisecond,
			EventTempThreshold:  1.0,
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
			Zone:          z,
			UnitID:        name,
			Online:        false,
			State:         &LGAPDeviceState{},
			Source:        "config",
			ReportEnabled: true,
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
