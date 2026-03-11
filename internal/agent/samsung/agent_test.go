package samsung

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// mockTransport 는 NASATransport 인터페이스의 테스트 구현체이다.
// ---------------------------------------------------------------------------

type mockTransport struct {
	mu        sync.Mutex
	openErr   error
	closeErr  error
	sendErr   error
	recvData  []byte
	recvErr   error
	sentData  [][]byte
	available bool
	opened    bool
	closed    bool
}

func (m *mockTransport) Open() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.openErr != nil {
		return m.openErr
	}
	m.opened = true
	m.available = true
	return nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = false
	m.available = false
	m.closed = true
	return m.closeErr
}

func (m *mockTransport) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	m.sentData = append(m.sentData, cp)
	return nil
}

func (m *mockTransport) Receive(buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvErr != nil {
		return 0, m.recvErr
	}
	if m.recvData != nil {
		n := copy(buf, m.recvData)
		m.recvData = nil // 한 번만 반환
		return n, nil
	}
	return 0, nil
}

func (m *mockTransport) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.available
}

func (m *mockTransport) getSentData() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sentData
}

// ---------------------------------------------------------------------------
// mockProtocol 은 NASAProtocol 인터페이스의 테스트 구현체이다.
// ---------------------------------------------------------------------------

type mockProtocol struct {
	mu                    sync.Mutex
	encodeResult          []byte
	encodeErr             error
	decodeResult          *NASAMessage
	decodeErr             error
	buildStatusResult     []byte
	buildStatusErr        error
	buildControlResult    []byte
	buildControlErr       error
	checksumResult        uint16
	parseMessageResult    []NASAMessageSet
	parseMessageErr       error
	encodeMessageResult   []byte
	buildControlCallCount int
	buildStatusCallCount  int
}

func (m *mockProtocol) Encode(msg *NASAMessage) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.encodeErr != nil {
		return nil, m.encodeErr
	}
	if m.encodeResult != nil {
		return m.encodeResult, nil
	}
	return []byte{0x32, 0x00, 0x10, 0x34}, nil
}

func (m *mockProtocol) Decode(data []byte) (*NASAMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.decodeErr != nil {
		return nil, m.decodeErr
	}
	return m.decodeResult, nil
}

func (m *mockProtocol) BuildStatusQuery(addr NASAAddress, seqNum byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buildStatusCallCount++
	if m.buildStatusErr != nil {
		return nil, m.buildStatusErr
	}
	if m.buildStatusResult != nil {
		return m.buildStatusResult, nil
	}
	return []byte{0x32, 0x00, 0x10, 0x34}, nil
}

func (m *mockProtocol) BuildControlCommand(addr NASAAddress, seqNum byte, sets []NASAMessageSet) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buildControlCallCount++
	if m.buildControlErr != nil {
		return nil, m.buildControlErr
	}
	if m.buildControlResult != nil {
		return m.buildControlResult, nil
	}
	return []byte{0x32, 0x00, 0x10, 0x34}, nil
}

func (m *mockProtocol) CalculateChecksum(data []byte) uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checksumResult
}

func (m *mockProtocol) ParseMessageSets(data []byte, count int) ([]NASAMessageSet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.parseMessageErr != nil {
		return nil, m.parseMessageErr
	}
	return m.parseMessageResult, nil
}

func (m *mockProtocol) EncodeMessageSets(sets []NASAMessageSet) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.encodeMessageResult != nil {
		return m.encodeMessageResult
	}
	return []byte{}
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestAgent 는 mock 의존성이 주입된 테스트용 NASAAgent 를 생성한다.
func newTestAgent(t *testing.T) (*NASAAgent, *mockTransport, *mockProtocol) {
	t.Helper()

	mt := &mockTransport{available: true}
	mp := &mockProtocol{
		buildControlResult: []byte{0x32, 0x00, 0x10, 0x34},
		buildStatusResult:  []byte{0x32, 0x00, 0x10, 0x34},
	}

	a := &NASAAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("samsung-nasa")),
		agentConfig: agent.AgentConfig{
			ID:   "test-id",
			Name: "test-nasa",
			Type: "samsung-nasa",
		},
		nasaConfig: NASAConfig{
			TransportType: "serial",
			SerialPort:    "/dev/ttyTest",
			PollInterval:  30 * time.Second,
			MsgChannelSize: 256,
		},
		devices:    make(map[NASAAddress]*NASADevice),
		deviceIDs:  make(map[string]NASAAddress),
		transport:  mt,
		protocol:   mp,
		stopCh:     make(chan struct{}),
		msgCh:      make(chan []byte, 256),
		stats:      agent.NewAgentStats(),
		logger:     testLogger(),
		lastStates: make(map[NASAAddress]NASADeviceState),
		createdAt:  time.Now(),
	}

	// Running 상태로 전이
	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		t.Fatalf("transition to initializing: %v", err)
	}
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		t.Fatalf("transition to running: %v", err)
	}
	a.startedAt = time.Now()

	// 테스트 디바이스 등록
	addr1, _ := ParseNASAAddress("20 00 01")
	addr2, _ := ParseNASAAddress("20 00 02")

	a.devices[addr1] = &NASADevice{
		Address:  addr1,
		DeviceID: "living-room",
		Type:     "indoor",
		Online:   true,
		LastSeen: time.Now(),
		State:    &NASADeviceState{RawMessageSets: make(map[uint16][]byte)},
		Source:   "config",
	}
	a.devices[addr2] = &NASADevice{
		Address:  addr2,
		DeviceID: "bedroom",
		Type:     "indoor",
		Online:   true,
		LastSeen: time.Now(),
		State:    &NASADeviceState{RawMessageSets: make(map[uint16][]byte)},
		Source:   "bridge",
	}
	a.deviceIDs["living-room"] = addr1
	a.deviceIDs["bedroom"] = addr2

	return a, mt, mp
}

// testLogger 는 테스트용 slog.Logger 를 반환한다.
func testLogger() *slog.Logger {
	return slog.Default()
}

// ---------------------------------------------------------------------------
// slog import 를 위한 별도 import (파일 상단에 이미 포함됨)
// ---------------------------------------------------------------------------

// processJSON 은 JSON 요청을 생성하여 Process 에 전달하고 결과를 반환한다.
func processJSON(t *testing.T, a *NASAAgent, req any) (map[string]any, error) {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	result, err := a.Process(data)
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp, nil
}

// ===========================================================================
// 테스트 케이스
// ===========================================================================

// TestNewNASAAgent_Success 는 올바른 설정으로 에이전트가 생성되는지 검증한다.
func TestNewNASAAgent_Success(t *testing.T) {
	// SerialOpener 를 mock 으로 설정
	origOpener := SerialOpener
	SerialOpener = func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error) {
		return nil, nil
	}
	defer func() { SerialOpener = origOpener }()

	config := agent.AgentConfig{
		ID:   "nasa-1",
		Name: "NASA HVAC",
		Type: "samsung-nasa",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"transport_type":   "serial",
				"serial_port":     "/dev/ttyUSB0",
				"device_addresses": []any{"200001", "200002"},
				"device_ids":      map[string]any{"lr": "200001"},
			},
		},
	}

	a, err := NewNASAAgent(config)
	if err != nil {
		t.Fatalf("NewNASAAgent: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
	if a.Type() != "samsung-nasa" {
		t.Errorf("Type() = %q, want %q", a.Type(), "samsung-nasa")
	}
}

// TestNewNASAAgent_InvalidConfig 는 transport_type 누락 시 에러를 반환하는지 검증한다.
func TestNewNASAAgent_InvalidConfig(t *testing.T) {
	config := agent.AgentConfig{
		ID:   "nasa-1",
		Name: "NASA HVAC",
		Type: "samsung-nasa",
		Transport: agent.TransportConfig{
			Options: map[string]any{
				// transport_type 누락
				"device_addresses": []any{"200001"},
			},
		},
	}

	_, err := NewNASAAgent(config)
	if err == nil {
		t.Fatal("expected error for missing transport_type")
	}
}

// TestNASAAgent_Init 은 Init 이 올바르게 Running 상태로 전이하는지 검증한다.
func TestNASAAgent_Init(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if a.CurrentState() != lifecycle.StateRunning {
		t.Errorf("expected Running state, got %s", a.CurrentState())
	}
}

// TestNASAAgent_Start_Stop 은 Start/Stop 라이프사이클을 검증한다.
func TestNASAAgent_Start_Stop(t *testing.T) {
	a, mt, _ := newTestAgent(t)

	// Transport 를 미연결 상태로 변경하여 Start 에서 Open 이 호출되도록 한다.
	mt.mu.Lock()
	mt.available = false
	mt.mu.Unlock()

	// Start
	err := a.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	mt.mu.Lock()
	opened := mt.opened
	mt.mu.Unlock()
	if !opened {
		t.Error("expected transport to be opened")
	}

	// 잠시 대기하여 goroutine 시작
	time.Sleep(50 * time.Millisecond)

	// Stop
	err = a.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if a.CurrentState() != lifecycle.StateStopped {
		t.Errorf("expected Stopped state, got %s", a.CurrentState())
	}
}

// TestNASAAgent_Pause_Resume 은 Pause/Resume 동작을 검증한다.
func TestNASAAgent_Pause_Resume(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// Pause
	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if a.CurrentState() != lifecycle.StatePaused {
		t.Errorf("expected Paused, got %s", a.CurrentState())
	}

	a.mu.RLock()
	paused := a.paused
	a.mu.RUnlock()
	if !paused {
		t.Error("expected paused flag to be true")
	}

	// Resume
	if err := a.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if a.CurrentState() != lifecycle.StateRunning {
		t.Errorf("expected Running, got %s", a.CurrentState())
	}

	a.mu.RLock()
	paused = a.paused
	a.mu.RUnlock()
	if paused {
		t.Error("expected paused flag to be false")
	}
}

// TestNASAAgent_Health 는 상태에 따른 Health 를 검증한다.
func TestNASAAgent_Health(t *testing.T) {
	tests := []struct {
		name     string
		state    lifecycle.State
		expected agent.HealthState
	}{
		{"Running -> Healthy", lifecycle.StateRunning, agent.HealthHealthy},
		{"Paused -> Degraded", lifecycle.StatePaused, agent.HealthDegraded},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestAgent(t)
			if tc.state == lifecycle.StatePaused {
				_ = a.TransitionTo(lifecycle.StatePaused)
			}
			h := a.Health()
			if h.Status != tc.expected {
				t.Errorf("Health().Status = %q, want %q", h.Status, tc.expected)
			}
		})
	}
}

// TestNASAAgent_Type 는 Type() 이 "samsung-nasa" 를 반환하는지 검증한다.
func TestNASAAgent_Type(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if a.Type() != "samsung-nasa" {
		t.Errorf("Type() = %q, want %q", a.Type(), "samsung-nasa")
	}
}

// TestNASAAgent_Info 는 Info() 스냅샷을 검증한다.
func TestNASAAgent_Info(t *testing.T) {
	a, _, _ := newTestAgent(t)
	info := a.Info()
	if info.ID != "test-id" {
		t.Errorf("Info().ID = %q, want %q", info.ID, "test-id")
	}
	if info.Name != "test-nasa" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "test-nasa")
	}
	if info.Type != "samsung-nasa" {
		t.Errorf("Info().Type = %q, want %q", info.Type, "samsung-nasa")
	}
	if info.State != lifecycle.StateRunning {
		t.Errorf("Info().State = %q, want %q", info.State, lifecycle.StateRunning)
	}
}

// TestNASAAgent_Process_SetPower 는 set_power 명령을 검증한다.
func TestNASAAgent_Process_SetPower(t *testing.T) {
	tests := []struct {
		name      string
		power     bool
		wantPower bool
	}{
		{"power on", true, true},
		{"power off", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestAgent(t)
			resp, err := processJSON(t, a, map[string]any{
				"command":   "set_power",
				"device_id": "living-room",
				"params":    map[string]any{"power": tc.power},
			})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if resp["status"] != "ok" {
				t.Errorf("status = %v, want ok", resp["status"])
			}
			result := resp["result"].(map[string]any)
			if result["power"] != tc.wantPower {
				t.Errorf("result.power = %v, want %v", result["power"], tc.wantPower)
			}
		})
	}
}

// TestNASAAgent_Process_SetMode 는 set_mode 명령을 검증한다 (유효 + 무효).
func TestNASAAgent_Process_SetMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr error
	}{
		{"valid cool", "cool", nil},
		{"valid heat", "heat", nil},
		{"valid auto", "auto", nil},
		{"invalid mode", "turbo", ErrInvalidMode},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestAgent(t)
			_, err := processJSON(t, a, map[string]any{
				"command":   "set_mode",
				"device_id": "living-room",
				"params":    map[string]any{"mode": tc.mode},
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestNASAAgent_Process_SetTemperature 는 set_temperature 명령을 검증한다.
func TestNASAAgent_Process_SetTemperature(t *testing.T) {
	tests := []struct {
		name    string
		temp    float64
		wantErr error
	}{
		{"valid 22.0", 22.0, nil},
		{"valid 16.0 min", 16.0, nil},
		{"valid 30.0 max", 30.0, nil},
		{"too low 15.0", 15.0, ErrTemperatureOutOfRange},
		{"too high 31.0", 31.0, ErrTemperatureOutOfRange},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestAgent(t)
			_, err := processJSON(t, a, map[string]any{
				"command":   "set_temperature",
				"device_id": "living-room",
				"params":    map[string]any{"target_temp": tc.temp},
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestNASAAgent_Process_SetFanSpeed 는 set_fan_speed 명령을 검증한다.
func TestNASAAgent_Process_SetFanSpeed(t *testing.T) {
	tests := []struct {
		name    string
		speed   string
		wantErr error
	}{
		{"valid auto", "auto", nil},
		{"valid high", "high", nil},
		{"invalid speed", "turbo", ErrInvalidFanSpeed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestAgent(t)
			_, err := processJSON(t, a, map[string]any{
				"command":   "set_fan_speed",
				"device_id": "living-room",
				"params":    map[string]any{"fan_speed": tc.speed},
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestNASAAgent_Process_SetMultiple 는 set_multiple 명령을 검증한다.
func TestNASAAgent_Process_SetMultiple(t *testing.T) {
	a, _, _ := newTestAgent(t)
	resp, err := processJSON(t, a, map[string]any{
		"command":   "set_multiple",
		"device_id": "living-room",
		"params": map[string]any{
			"power":       true,
			"mode":        "cool",
			"target_temp": 24.0,
			"fan_speed":   "high",
		},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	result := resp["result"].(map[string]any)
	if result["power"] != true {
		t.Errorf("result.power = %v, want true", result["power"])
	}
	if result["mode"] != "cool" {
		t.Errorf("result.mode = %v, want cool", result["mode"])
	}
}

// TestNASAAgent_Process_GetState 는 get_state 명령을 검증한다.
func TestNASAAgent_Process_GetState(t *testing.T) {
	t.Run("by device_id", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		resp, err := processJSON(t, a, map[string]any{
			"command":   "get_state",
			"device_id": "living-room",
		})
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("status = %v, want ok", resp["status"])
		}
		if resp["device_id"] != "living-room" {
			t.Errorf("device_id = %v, want living-room", resp["device_id"])
		}
		if resp["online"] != true {
			t.Errorf("online = %v, want true", resp["online"])
		}
	})

	t.Run("by address", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		resp, err := processJSON(t, a, map[string]any{
			"command": "get_state",
			"address": "20 00 01",
		})
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("status = %v, want ok", resp["status"])
		}
	})
}

// TestNASAAgent_Process_GetAllStates 는 get_all_states 명령을 검증한다.
func TestNASAAgent_Process_GetAllStates(t *testing.T) {
	a, _, _ := newTestAgent(t)
	resp, err := processJSON(t, a, map[string]any{
		"command": "get_all_states",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	devices, ok := resp["devices"].([]any)
	if !ok {
		t.Fatalf("devices is not array: %T", resp["devices"])
	}
	if len(devices) != 2 {
		t.Errorf("devices count = %d, want 2", len(devices))
	}
}

// TestNASAAgent_Process_AddDevice 는 add_device 명령을 검증한다.
func TestNASAAgent_Process_AddDevice(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		resp, err := processJSON(t, a, map[string]any{
			"command":     "add_device",
			"address":     "20 00 03",
			"device_id":   "kitchen",
			"device_type": "indoor",
		})
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("status = %v, want ok", resp["status"])
		}

		// 등록 확인
		addr, _ := ParseNASAAddress("20 00 03")
		a.mu.RLock()
		_, exists := a.devices[addr]
		a.mu.RUnlock()
		if !exists {
			t.Error("device not found after add_device")
		}
	})

	t.Run("duplicate address", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		_, err := processJSON(t, a, map[string]any{
			"command": "add_device",
			"address": "20 00 01", // 이미 등록된 주소
		})
		if !errors.Is(err, ErrDeviceAlreadyRegistered) {
			t.Errorf("error = %v, want ErrDeviceAlreadyRegistered", err)
		}
	})

	t.Run("duplicate device_id", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		_, err := processJSON(t, a, map[string]any{
			"command":   "add_device",
			"address":   "20 00 03",
			"device_id": "living-room", // 이미 사용 중인 ID
		})
		if !errors.Is(err, ErrDuplicateDeviceID) {
			t.Errorf("error = %v, want ErrDuplicateDeviceID", err)
		}
	})
}

// TestNASAAgent_Process_RemoveDevice 는 remove_device 명령을 검증한다.
func TestNASAAgent_Process_RemoveDevice(t *testing.T) {
	t.Run("success bridge device", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		resp, err := processJSON(t, a, map[string]any{
			"command":   "remove_device",
			"device_id": "bedroom", // source: bridge
		})
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("status = %v, want ok", resp["status"])
		}
	})

	t.Run("config device protected", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		_, err := processJSON(t, a, map[string]any{
			"command":   "remove_device",
			"device_id": "living-room", // source: config
		})
		if !errors.Is(err, ErrConfigDeviceProtected) {
			t.Errorf("error = %v, want ErrConfigDeviceProtected", err)
		}
	})
}

// TestNASAAgent_Process_ListDevices 는 list_devices 명령을 검증한다.
func TestNASAAgent_Process_ListDevices(t *testing.T) {
	a, _, _ := newTestAgent(t)
	resp, err := processJSON(t, a, map[string]any{
		"command": "list_devices",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	devices, ok := resp["devices"].([]any)
	if !ok {
		t.Fatalf("devices is not array: %T", resp["devices"])
	}
	if len(devices) != 2 {
		t.Errorf("devices count = %d, want 2", len(devices))
	}
}

// TestNASAAgent_Process_InvalidCommand 는 알 수 없는 명령에 대한 에러를 검증한다.
func TestNASAAgent_Process_InvalidCommand(t *testing.T) {
	a, _, _ := newTestAgent(t)
	data, _ := json.Marshal(map[string]any{
		"command": "unknown_cmd",
	})
	_, err := a.Process(data)
	if !errors.Is(err, ErrInvalidCommand) {
		t.Errorf("error = %v, want ErrInvalidCommand", err)
	}
}

// TestNASAAgent_Process_DeviceIDResolution 는 device_id 가 address 보다 우선하는지 검증한다.
func TestNASAAgent_Process_DeviceIDResolution(t *testing.T) {
	a, _, _ := newTestAgent(t)
	// device_id 와 address 를 동시에 제공: device_id 가 우선
	resp, err := processJSON(t, a, map[string]any{
		"command":   "get_state",
		"device_id": "living-room",
		"address":   "20 00 02", // bedroom 의 주소
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	// device_id living-room 이 우선 적용되어야 한다
	if resp["device_id"] != "living-room" {
		t.Errorf("device_id = %v, want living-room", resp["device_id"])
	}
}

// TestNASAAgent_Process_OfflineDevice 는 오프라인 디바이스에 제어 명령 시 에러를 검증한다.
func TestNASAAgent_Process_OfflineDevice(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// 디바이스를 오프라인으로 설정
	addr, _ := ParseNASAAddress("20 00 01")
	a.mu.Lock()
	a.devices[addr].Online = false
	a.mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"command":   "set_power",
		"device_id": "living-room",
		"params":    map[string]any{"power": true},
	})
	_, err := a.Process(data)
	if !errors.Is(err, ErrDeviceOffline) {
		t.Errorf("error = %v, want ErrDeviceOffline", err)
	}
}

// TestNASAAgent_ReceiveMessage 는 채널 수신과 컨텍스트 취소를 검증한다.
func TestNASAAgent_ReceiveMessage(t *testing.T) {
	t.Run("receive from channel", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		expected := []byte(`{"type":"test"}`)
		a.msgCh <- expected

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		data, err := a.ReceiveMessage(ctx)
		if err != nil {
			t.Fatalf("ReceiveMessage: %v", err)
		}
		if string(data) != string(expected) {
			t.Errorf("data = %s, want %s", data, expected)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 즉시 취소

		_, err := a.ReceiveMessage(ctx)
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
	})
}

// TestNASAAgent_ListDevices 는 공개 메서드 ListDevices 를 검증한다.
func TestNASAAgent_ListDevices(t *testing.T) {
	a, _, _ := newTestAgent(t)
	devices := a.ListDevices()
	if len(devices) != 2 {
		t.Errorf("ListDevices count = %d, want 2", len(devices))
	}
}

// TestNASAAgent_GetDeviceState 는 디바이스 상태 조회를 검증한다.
func TestNASAAgent_GetDeviceState(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		addr, _ := ParseNASAAddress("20 00 01")
		state, err := a.GetDeviceState(addr)
		if err != nil {
			t.Fatalf("GetDeviceState: %v", err)
		}
		if state == nil {
			t.Fatal("expected non-nil state")
		}
	})

	t.Run("not found", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		addr, _ := ParseNASAAddress("20 00 FF")
		_, err := a.GetDeviceState(addr)
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("error = %v, want ErrDeviceNotFound", err)
		}
	})
}

// TestNASAAgent_GetDeviceByID 는 device_id 로 디바이스 조회를 검증한다.
func TestNASAAgent_GetDeviceByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		dev, err := a.GetDeviceByID("living-room")
		if err != nil {
			t.Fatalf("GetDeviceByID: %v", err)
		}
		if dev == nil {
			t.Fatal("expected non-nil device")
		}
		if dev.DeviceID != "living-room" {
			t.Errorf("DeviceID = %q, want %q", dev.DeviceID, "living-room")
		}
	})

	t.Run("not found", func(t *testing.T) {
		a, _, _ := newTestAgent(t)
		_, err := a.GetDeviceByID("non-existent")
		if !errors.Is(err, ErrDeviceIDNotFound) {
			t.Errorf("error = %v, want ErrDeviceIDNotFound", err)
		}
	})
}

// TestNASAAgent_DeviceIDConfig 는 device_ids 설정이 올바르게 매핑되는지 검증한다.
func TestNASAAgent_DeviceIDConfig(t *testing.T) {
	a, _, _ := newTestAgent(t)

	addr1, _ := ParseNASAAddress("20 00 01")
	addr2, _ := ParseNASAAddress("20 00 02")

	a.mu.RLock()
	defer a.mu.RUnlock()

	// device_id -> address 매핑 확인
	if resolved, ok := a.deviceIDs["living-room"]; !ok {
		t.Error("living-room not found in deviceIDs")
	} else if resolved != addr1 {
		t.Errorf("living-room resolved to %v, want %v", resolved, addr1)
	}

	if resolved, ok := a.deviceIDs["bedroom"]; !ok {
		t.Error("bedroom not found in deviceIDs")
	} else if resolved != addr2 {
		t.Errorf("bedroom resolved to %v, want %v", resolved, addr2)
	}

	// 디바이스의 DeviceID 필드 확인
	if dev, ok := a.devices[addr1]; !ok {
		t.Error("device at addr1 not found")
	} else if dev.DeviceID != "living-room" {
		t.Errorf("device DeviceID = %q, want %q", dev.DeviceID, "living-room")
	}
}

// TestNASAAgent_Process_SendTransportError 는 트랜스포트 전송 에러를 검증한다.
func TestNASAAgent_Process_SendTransportError(t *testing.T) {
	a, mt, _ := newTestAgent(t)
	mt.sendErr = errors.New("transport broken")

	data, _ := json.Marshal(map[string]any{
		"command":   "set_power",
		"device_id": "living-room",
		"params":    map[string]any{"power": true},
	})
	_, err := a.Process(data)
	if err == nil {
		t.Fatal("expected error from transport send failure")
	}
}

// TestNASAAgent_Process_BuildControlError 는 프로토콜 빌드 에러를 검증한다.
func TestNASAAgent_Process_BuildControlError(t *testing.T) {
	a, _, mp := newTestAgent(t)
	mp.buildControlErr = errors.New("protocol error")

	data, _ := json.Marshal(map[string]any{
		"command":   "set_power",
		"device_id": "living-room",
		"params":    map[string]any{"power": true},
	})
	_, err := a.Process(data)
	if err == nil {
		t.Fatal("expected error from protocol build failure")
	}
}

// TestNASAAgent_Process_DeviceNotFound 는 존재하지 않는 디바이스에 대한 에러를 검증한다.
func TestNASAAgent_Process_DeviceNotFound(t *testing.T) {
	a, _, _ := newTestAgent(t)
	data, _ := json.Marshal(map[string]any{
		"command": "get_state",
		"address": "20 00 FF",
	})
	_, err := a.Process(data)
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("error = %v, want ErrDeviceNotFound", err)
	}
}

// TestNASAAgent_Process_DeviceIDNotFound 는 존재하지 않는 device_id 에 대한 에러를 검증한다.
func TestNASAAgent_Process_DeviceIDNotFound(t *testing.T) {
	a, _, _ := newTestAgent(t)
	data, _ := json.Marshal(map[string]any{
		"command":   "get_state",
		"device_id": "non-existent",
	})
	_, err := a.Process(data)
	if !errors.Is(err, ErrDeviceIDNotFound) {
		t.Errorf("error = %v, want ErrDeviceIDNotFound", err)
	}
}

// TestNASAAgent_Configure 는 Configure 메서드를 검증한다.
func TestNASAAgent_Configure(t *testing.T) {
	a, _, _ := newTestAgent(t)
	newConfig := agent.AgentConfig{
		ID:   "test-id-2",
		Name: "updated-nasa",
		Type: "samsung-nasa",
	}
	if err := a.Configure(newConfig); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if a.Name() != "updated-nasa" {
		t.Errorf("Name() = %q, want %q", a.Name(), "updated-nasa")
	}
}

// TestNASAAgent_ID_Name 은 ID/Name 메서드를 검증한다.
func TestNASAAgent_ID_Name(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if a.ID() != "test-id" {
		t.Errorf("ID() = %q, want %q", a.ID(), "test-id")
	}
	if a.Name() != "test-nasa" {
		t.Errorf("Name() = %q, want %q", a.Name(), "test-nasa")
	}
}

// TestNASAAgent_Stats 는 Stats 스냅샷을 검증한다.
func TestNASAAgent_Stats(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// 메시지 전송 후 stats 확인
	data, _ := json.Marshal(map[string]any{
		"command":   "set_power",
		"device_id": "living-room",
		"params":    map[string]any{"power": true},
	})
	_, _ = a.Process(data)

	stats := a.Stats()
	if stats.MessagesSent == 0 {
		t.Error("expected MessagesSent > 0 after control command")
	}
}

// TestNASAAgent_Process_InvalidJSON 은 유효하지 않은 JSON 입력을 검증한다.
func TestNASAAgent_Process_InvalidJSON(t *testing.T) {
	a, _, _ := newTestAgent(t)
	_, err := a.Process([]byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestNASAAgent_Process_SetMultiple_EmptyParams 는 빈 params 에 대한 에러를 검증한다.
func TestNASAAgent_Process_SetMultiple_EmptyParams(t *testing.T) {
	a, _, _ := newTestAgent(t)
	data, _ := json.Marshal(map[string]any{
		"command":   "set_multiple",
		"device_id": "living-room",
		"params":    map[string]any{},
	})
	_, err := a.Process(data)
	if err == nil {
		t.Fatal("expected error for set_multiple with empty params")
	}
}

// TestNASAAgent_NextSeqNum 은 시퀀스 번호 래핑을 검증한다.
func TestNASAAgent_NextSeqNum(t *testing.T) {
	a, _, _ := newTestAgent(t)

	a.mu.Lock()
	a.seqNum = 0xFE
	a.mu.Unlock()

	seq1 := a.nextSeqNum()
	if seq1 != 0xFE {
		t.Errorf("seq1 = 0x%02X, want 0xFE", seq1)
	}

	seq2 := a.nextSeqNum()
	if seq2 != 0xFF {
		t.Errorf("seq2 = 0x%02X, want 0xFF", seq2)
	}

	seq3 := a.nextSeqNum()
	if seq3 != 0x00 {
		t.Errorf("seq3 = 0x%02X, want 0x00 (wrapped)", seq3)
	}
}

// TestNASAAgent_SendEvent_ChannelFull 은 msgCh 가 가득 찼을 때 드롭되는지 검증한다.
func TestNASAAgent_SendEvent_ChannelFull(t *testing.T) {
	a, _, _ := newTestAgent(t)

	// 채널을 가득 채운다
	for i := 0; i < 256; i++ {
		select {
		case a.msgCh <- []byte(`{"type":"fill"}`):
		default:
			break
		}
	}

	// 이벤트 전송 시 패닉 없이 드롭되어야 한다
	a.sendEvent("test_event", map[string]any{"key": "value"})
}

// TestNASAAgent_Process_NoAddressOrDeviceID 는 address/device_id 모두 없을 때 에러를 검증한다.
func TestNASAAgent_Process_NoAddressOrDeviceID(t *testing.T) {
	a, _, _ := newTestAgent(t)
	data, _ := json.Marshal(map[string]any{
		"command": "get_state",
	})
	_, err := a.Process(data)
	if err == nil {
		t.Fatal("expected error when no address or device_id provided")
	}
}

// ---------------------------------------------------------------------------
// handleMessage 테스트
// ---------------------------------------------------------------------------

func TestNASAAgent_HandleMessage_KnownDevice(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNASAAddress("20 00 01")

	// 디바이스를 오프라인으로 설정
	a.mu.Lock()
	a.devices[addr].Online = false
	a.mu.Unlock()

	msg := &NASAMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgMode, Value: []byte{0x01}},        // cool
			{Index: MsgFanSpeed, Value: []byte{0x02}},     // medium
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}}, // 25.0
			{Index: MsgCurrentTemp, Value: []byte{0x00, 0xF0}}, // 24.0
		},
	}
	a.handleMessage(msg)

	a.mu.RLock()
	dev := a.devices[addr]
	a.mu.RUnlock()

	if !dev.Online {
		t.Error("expected device to be online")
	}
	if !dev.State.Power {
		t.Error("expected power on")
	}
	if dev.State.Mode != "cool" {
		t.Errorf("expected mode cool, got %s", dev.State.Mode)
	}
	if dev.State.FanSpeed != "medium" {
		t.Errorf("expected fan medium, got %s", dev.State.FanSpeed)
	}
	if dev.State.TargetTemp != 25.0 {
		t.Errorf("expected target 25.0, got %.1f", dev.State.TargetTemp)
	}

	// device_online 이벤트 확인
	drainAndFindEvent(t, a.msgCh, "device_online")
}

func TestNASAAgent_HandleMessage_AutoDiscovery(t *testing.T) {
	a, _, _ := newTestAgent(t)
	a.nasaConfig.AutoDiscovery = true

	unknownAddr, _ := ParseNASAAddress("20 00 99")
	msg := &NASAMessage{
		SourceAddr:  unknownAddr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
		},
	}
	a.handleMessage(msg)

	a.mu.RLock()
	dev, ok := a.devices[unknownAddr]
	a.mu.RUnlock()

	if !ok {
		t.Fatal("expected auto-discovered device")
	}
	if dev.Source != "auto" {
		t.Errorf("expected source auto, got %s", dev.Source)
	}
	if dev.Type != "indoor" {
		t.Errorf("expected indoor type, got %s", dev.Type)
	}
	drainAndFindEvent(t, a.msgCh, "device_discovered")
}

func TestNASAAgent_HandleMessage_UnknownDevice_NoAutoDiscovery(t *testing.T) {
	a, _, _ := newTestAgent(t)
	a.nasaConfig.AutoDiscovery = false

	unknownAddr, _ := ParseNASAAddress("20 00 99")
	msg := &NASAMessage{
		SourceAddr:  unknownAddr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
	}
	a.handleMessage(msg)

	a.mu.RLock()
	_, ok := a.devices[unknownAddr]
	a.mu.RUnlock()

	if ok {
		t.Fatal("should not register unknown device without auto-discovery")
	}
}

func TestNASAAgent_HandleMessage_StateChanged(t *testing.T) {
	a, _, _ := newTestAgent(t)
	addr, _ := ParseNASAAddress("20 00 01")

	// 초기 상태 설정
	msg1 := &NASAMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgTargetTemp, Value: []byte{0x00, 0xFA}}, // 25.0
		},
	}
	a.handleMessage(msg1)
	// 이벤트 드레인
	drainEvents(a.msgCh)

	// 온도 변경
	msg2 := &NASAMessage{
		SourceAddr:  addr,
		DestAddr:    AddrController,
		CommandCode: CmdNormalRequest,
		MessageSets: []NASAMessageSet{
			{Index: MsgTargetTemp, Value: []byte{0x01, 0x04}}, // 26.0
		},
	}
	a.handleMessage(msg2)
	drainAndFindEvent(t, a.msgCh, "device_state_changed")
}

// ---------------------------------------------------------------------------
// stateChanged 테스트
// ---------------------------------------------------------------------------

func TestStateChanged(t *testing.T) {
	tests := []struct {
		name    string
		prev    NASADeviceState
		current NASADeviceState
		want    bool
	}{
		{
			name:    "identical states",
			prev:    NASADeviceState{Power: true, Mode: "cool", TargetTemp: 25.0, CurrentTemp: 24.0, FanSpeed: "auto"},
			current: NASADeviceState{Power: true, Mode: "cool", TargetTemp: 25.0, CurrentTemp: 24.0, FanSpeed: "auto"},
			want:    false,
		},
		{
			name:    "power changed",
			prev:    NASADeviceState{Power: false},
			current: NASADeviceState{Power: true},
			want:    true,
		},
		{
			name:    "mode changed",
			prev:    NASADeviceState{Mode: "cool"},
			current: NASADeviceState{Mode: "heat"},
			want:    true,
		},
		{
			name:    "target temp changed",
			prev:    NASADeviceState{TargetTemp: 25.0},
			current: NASADeviceState{TargetTemp: 26.0},
			want:    true,
		},
		{
			name:    "current temp changed",
			prev:    NASADeviceState{CurrentTemp: 24.0},
			current: NASADeviceState{CurrentTemp: 25.0},
			want:    true,
		},
		{
			name:    "fan speed changed",
			prev:    NASADeviceState{FanSpeed: "auto"},
			current: NASADeviceState{FanSpeed: "high"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stateChanged(tt.prev, tt.current)
			if got != tt.want {
				t.Errorf("stateChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼 함수
// ---------------------------------------------------------------------------

func drainEvents(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainAndFindEvent(t *testing.T, ch chan []byte, eventType string) {
	t.Helper()
	for i := 0; i < 10; i++ {
		select {
		case data := <-ch:
			var evt map[string]any
			if err := json.Unmarshal(data, &evt); err == nil {
				if evt["type"] == eventType {
					return
				}
			}
		default:
			t.Fatalf("expected event %q not found", eventType)
			return
		}
	}
	t.Fatalf("expected event %q not found in 10 messages", eventType)
}

// TestFilterMessageSets 는 unsupported 메시지 셋 필터링을 검증한다.
func TestFilterMessageSets(t *testing.T) {
	a, _, _ := newTestAgent(t)

	tests := []struct {
		name        string
		unsupported map[uint16]bool
		input       []NASAMessageSet
		wantLen     int
		wantIndices []uint16
	}{
		{
			name:        "필터 없음 - 모두 통과",
			unsupported: nil,
			input: []NASAMessageSet{
				{Index: MsgPower, Value: []byte{0x01}},
				{Index: MsgMode, Value: []byte{0x02}},
			},
			wantLen:     2,
			wantIndices: []uint16{MsgPower, MsgMode},
		},
		{
			name:        "빈 맵 - 모두 통과",
			unsupported: map[uint16]bool{},
			input: []NASAMessageSet{
				{Index: MsgPower, Value: []byte{0x01}},
			},
			wantLen:     1,
			wantIndices: []uint16{MsgPower},
		},
		{
			name:        "일부 필터링",
			unsupported: map[uint16]bool{0x4100: true, 0x4111: true},
			input: []NASAMessageSet{
				{Index: MsgPower, Value: []byte{0x01}},
				{Index: 0x4100, Value: []byte{0x00}},
				{Index: MsgMode, Value: []byte{0x02}},
				{Index: 0x4111, Value: []byte{0x00}},
			},
			wantLen:     2,
			wantIndices: []uint16{MsgPower, MsgMode},
		},
		{
			name:        "전체 필터링",
			unsupported: map[uint16]bool{0x4100: true, 0x4102: true},
			input: []NASAMessageSet{
				{Index: 0x4100, Value: []byte{0x00}},
				{Index: 0x4102, Value: []byte{0x00}},
			},
			wantLen:     0,
			wantIndices: nil,
		},
	}

	addr, _ := ParseNASAAddress("20 00 01")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a.nasaConfig.UnsupportedMsgSets = tt.unsupported
			result := a.filterMessageSets(tt.input, addr)

			if len(result) != tt.wantLen {
				t.Errorf("got %d sets, want %d", len(result), tt.wantLen)
			}
			for i, idx := range tt.wantIndices {
				if i < len(result) && result[i].Index != idx {
					t.Errorf("result[%d].Index = 0x%04X, want 0x%04X", i, result[i].Index, idx)
				}
			}
		})
	}
}
