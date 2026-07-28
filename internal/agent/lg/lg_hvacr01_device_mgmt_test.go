// lg_hvacr01_device_mgmt_test.go — LG ICP-01 캡처 에이전트(Hvacr01Agent)의 디바이스
// 관리 명령(list_devices / remove_device / set_device) 및 report_enabled 방출 게이트
// 검증.
//
// Samsung NASA 와 동일한 DTO/동작을 미러링한다. 캡처 전용(자동 발견)이므로 add_device
// 는 없다. report_enabled/삭제는 IN-MEMORY 만 유지한다(로스터/설정 영속화 없음).
//
// device_id(UUID) 저장소는 lg_hvacr02_device_mgmt_test.go 의 fakeIcp02DeviceIDRepo /
// withFakeDeviceIDRepoIcp02 를 재사용한다(동일 lg 패키지 테스트 빌드).
package lg

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// fakeIcp01DeviceIDRepo 는 (agentName, unitID) → 결정적 UUID 를 반환하는 테스트용
// 저장소이다. device_id(UUID) 역매칭 경로(resolveIcp01Target)를 실제 저장소 없이
// 검증하기 위함.
type fakeIcp01DeviceIDRepo struct{}

func (fakeIcp01DeviceIDRepo) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	return "uuid:" + agentName + ":" + unitID, nil
}

func (fakeIcp01DeviceIDRepo) Get(_ context.Context, agentName, unitID string) (string, error) {
	return "uuid:" + agentName + ":" + unitID, nil
}

func (fakeIcp01DeviceIDRepo) Set(_ context.Context, agentName, unitID, deviceID string) error {
	return nil
}

// withFakeDeviceIDRepoIcp01 은 결정적 device_id 저장소를 설정하고 종료 시 복원한다.
func withFakeDeviceIDRepoIcp01(t *testing.T) {
	t.Helper()
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(fakeIcp01DeviceIDRepo{})
	t.Cleanup(func() { agent.SetDeviceIDRepository(original) })
}

// newIcp01MgmtTestAgent 는 디바이스 관리/게이트 검증용 최소 Hvacr01Agent 를 만든다.
// AutoDiscovery=true(IDU 자동 등록), DedupeFrames=false(dedup 을 배제해 report 게이트
// 단독 검증), bridgeActive=true(msgCh emit 활성), oduReportEnabled=true.
func newIcp01MgmtTestAgent(t *testing.T) *Hvacr01Agent {
	t.Helper()
	a := &Hvacr01Agent{
		BaseLifecycle:    lifecycle.NewBaseLifecycle(lifecycle.WithName("lg_hvacr01-mgmt-test")),
		agentConfig:      agent.AgentConfig{Name: "test-lg01"},
		hvacr01Config:    Hvacr01Config{DedupeFrames: false, AutoDiscovery: true, IncludeRawHex: false},
		msgCh:            make(chan []byte, 32),
		stats:            agent.NewAgentStats(),
		logger:           slog.Default(),
		recentFrames:     make([]hvacr01FrameRecord, hvacr01RecentBufferSize),
		recentNotify:     make(chan struct{}, 1),
		iduDevices:       make(map[string]*Icp01Device),
		oduState:         &Icp01ODUState{},
		oduReportEnabled: true,
		lastStates:       make(map[string]Icp01DeviceState),
		lastIDUEmit:      make(map[int][]byte),
		lastIDUParsed:    make(map[int]Hvacr01IDUParsed),
	}
	a.bridgeActive.Store(true)
	return a
}

// icp01TestIDUFrame 은 검증 통과(RedundancyValid/StructureValid/RangeOk)하는 IDU 프레임을
// 구성한다. IDUAddr = 0x80 + iduNum → addrHex "81"~"85".
func icp01TestIDUFrame(iduNum int, roomTemp float64, ts time.Time) *Icp01IDUFrame {
	return &Icp01IDUFrame{
		IDUAddr:         byte(0x80 + iduNum),
		IDUNum:          iduNum,
		RedundancyValid: true,
		StructureValid:  true,
		RangeOk:         true,
		SetTempReliable: true,
		SetTemp:         24.0,
		RoomTemp:        roomTemp,
		InletTemp:       20.0,
		OutletTemp:      18.0,
		OpMode:          0x01, // bit5=0 → power ON
		FanByte:         0x30, // known → quiet
		DevType:         0x72,
		SlotNum:         byte(0x50 + iduNum),
		DeviceID:        byte(iduNum),
		Timestamp:       ts,
	}
}

// icp01TestODUFrameSEQ02 는 실시간 냉동 사이클(SEQ=0x02) ODU 프레임을 구성한다.
func icp01TestODUFrameSEQ02(ts time.Time) *Icp01ODUFrame {
	var raw [icp01ODUFrameLen]byte
	copy(raw[:], icp01TestODU_SEQ02)
	return &Icp01ODUFrame{
		Raw:           raw,
		SEQ:           0x02,
		Timestamp:     ts,
		ChecksumValid: true,
	}
}

// drainMsgCh 는 msgCh 에 쌓인 메시지 수를 세면서 비운다.
func drainMsgCh(a *Hvacr01Agent) int {
	n := 0
	for {
		select {
		case <-a.msgCh:
			n++
		default:
			return n
		}
	}
}

// TestHvacr01_ReportEnabled_False_SuppressesIDUEmit 는 report_enabled=false 인 IDU 가
// change/report 방출을 모두 억제하는지 검증한다.
func TestHvacr01_ReportEnabled_False_SuppressesIDUEmit(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	ts := time.Now()

	// 최초 관측(자동 발견, report_enabled=true) → change emit 1건.
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, ts))
	require.Equal(t, 1, drainMsgCh(a), "report_enabled=true 는 IDU device_state 를 방출해야 한다")

	// report off 로 전환.
	a.mu.Lock()
	a.iduDevices["81"].ReportEnabled = false
	a.mu.Unlock()

	// 상태 변경(온도) 유도 → off 이므로 change 방출 억제.
	a.handleIDUFrame(icp01TestIDUFrame(1, 27.0, ts.Add(time.Second)))
	assert.Equal(t, 0, drainMsgCh(a), "report_enabled=false 는 change 방출을 억제해야 한다")

	// 주기 report(emitAllDeviceStates) 도 억제.
	a.emitAllDeviceStates("report")
	assert.Equal(t, 0, drainMsgCh(a), "report_enabled=false 는 주기 report 도 억제해야 한다")

	// 억제되어도 디바이스 상태 캐시는 계속 갱신되어야 한다(online 유지).
	a.mu.RLock()
	dev, ok := a.iduDevices["81"]
	online := ok && dev.Online
	a.mu.RUnlock()
	require.True(t, ok, "report off 여도 디바이스는 삭제되지 않는다")
	assert.True(t, online, "report off 여도 상태(online)는 갱신된다")
}

// TestHvacr01_ReportEnabled_True_EmitsIDU 는 기본(true) IDU 가 정상 방출하는지 검증한다.
func TestHvacr01_ReportEnabled_True_EmitsIDU(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)

	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))

	assert.Equal(t, 1, drainMsgCh(a), "report_enabled=true 는 device_state 를 방출해야 한다")
	a.mu.RLock()
	re := a.iduDevices["81"].ReportEnabled
	a.mu.RUnlock()
	assert.True(t, re, "자동 발견 IDU 는 기본 report_enabled=true 여야 한다")
}

// TestHvacr01_ReportEnabled_False_SuppressesODUEmit 는 report_enabled=false 인 ODU 가
// device_state 방출을 억제하는지 검증한다. set_device(address="odu") 경로도 함께 검증.
func TestHvacr01_ReportEnabled_False_SuppressesODUEmit(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	ts := time.Now()

	// 최초 ODU SEQ=02 관측 → emit 1건.
	a.handleODUFrame(icp01TestODUFrameSEQ02(ts))
	require.Equal(t, 1, drainMsgCh(a), "report_enabled=true 는 ODU device_state 를 방출해야 한다")

	// set_device(address="odu", report_enabled=false).
	b, err := a.processSetDevice(&hvacr01ProcessRequest{
		Command: "set_device",
		Address: "odu",
		Params:  map[string]any{"report_enabled": false},
	})
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, false, resp["report_enabled"])
	assert.Equal(t, "HVACR.ODU", resp["device_type"])

	// 이후 ODU 프레임은 방출 억제.
	a.handleODUFrame(icp01TestODUFrameSEQ02(ts.Add(time.Second)))
	assert.Equal(t, 0, drainMsgCh(a), "report_enabled=false 는 ODU 방출을 억제해야 한다")
}

// TestHvacr01_SetDevice_TogglesReportEnabled 는 set_device 가 report_enabled/name 을
// 갱신하고 응답에 반영하는지 검증한다.
func TestHvacr01_SetDevice_TogglesReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))
	drainMsgCh(a)

	b, err := a.processSetDevice(&hvacr01ProcessRequest{
		Command: "set_device",
		Address: "81",
		Params:  map[string]any{"report_enabled": false, "name": "거실"},
	})
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, false, resp["report_enabled"])
	assert.Equal(t, "거실", resp["name"])
	assert.Equal(t, "81", resp["address"])

	a.mu.RLock()
	dev := a.iduDevices["81"]
	re, label := dev.ReportEnabled, dev.Label
	a.mu.RUnlock()
	assert.False(t, re, "dev.ReportEnabled 가 false 로 갱신되어야 한다")
	assert.Equal(t, "거실", label, "dev.Label 이 갱신되어야 한다")

	// 다시 on.
	_, err = a.processSetDevice(&hvacr01ProcessRequest{
		Command: "set_device",
		Address: "81",
		Params:  map[string]any{"report_enabled": true},
	})
	require.NoError(t, err)
	a.mu.RLock()
	reOn := a.iduDevices["81"].ReportEnabled
	a.mu.RUnlock()
	assert.True(t, reOn, "report_enabled 재활성화가 반영되어야 한다")
}

// TestHvacr01_SetDevice_ByDeviceID 는 device_id(UUID) 로도 set_device 가 동작하는지
// 검증한다(역매칭 헬퍼 resolveIcp01Target 경로).
func TestHvacr01_SetDevice_ByDeviceID(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))
	drainMsgCh(a)

	// 어댑터/REST 가 노출하는 device_id 는 address 기준 UUID.
	uuid := agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, "81")
	require.NotEmpty(t, uuid)

	b, err := a.processSetDevice(&hvacr01ProcessRequest{
		Command: "set_device",
		Params:  map[string]any{"device_id": uuid, "report_enabled": false},
	})
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, false, resp["report_enabled"])

	a.mu.RLock()
	re := a.iduDevices["81"].ReportEnabled
	a.mu.RUnlock()
	assert.False(t, re, "device_id(UUID) 로도 report_enabled 가 갱신되어야 한다")
}

// TestHvacr01_RemoveDevice_ByAddress 는 remove_device(address) 가 IN-MEMORY 디바이스를
// 삭제하는지 검증한다.
func TestHvacr01_RemoveDevice_ByAddress(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))

	a.mu.RLock()
	_, existed := a.iduDevices["81"]
	a.mu.RUnlock()
	require.True(t, existed, "삭제 전 디바이스가 존재해야 한다")

	b, err := a.processRemoveDevice(&hvacr01ProcessRequest{
		Command: "remove_device",
		Address: "81",
	})
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "81", resp["address"])

	a.mu.RLock()
	_, stillThere := a.iduDevices["81"]
	_, stateGone := a.lastStates["81"]
	a.mu.RUnlock()
	assert.False(t, stillThere, "remove_device 후 디바이스가 삭제되어야 한다")
	assert.False(t, stateGone, "remove_device 후 lastStates 항목도 삭제되어야 한다")
}

// TestHvacr01_RemoveDevice_ByDeviceID 는 프론트엔드가 보내는 device_id(UUID) 로도
// 삭제가 동작하는지 검증한다.
func TestHvacr01_RemoveDevice_ByDeviceID(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))

	uuid := agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, "81")
	require.NotEmpty(t, uuid)

	_, err := a.processRemoveDevice(&hvacr01ProcessRequest{
		Command: "remove_device",
		Params:  map[string]any{"device_id": uuid},
	})
	require.NoError(t, err)

	a.mu.RLock()
	_, stillThere := a.iduDevices["81"]
	a.mu.RUnlock()
	assert.False(t, stillThere, "device_id(UUID) 로도 삭제되어야 한다")
}

// TestHvacr01_RemoveDevice_NotFound 는 존재하지 않는 디바이스 삭제 시 에러를 반환하는지
// 검증한다.
func TestHvacr01_RemoveDevice_NotFound(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)

	_, err := a.processRemoveDevice(&hvacr01ProcessRequest{
		Command: "remove_device",
		Address: "deadbeef",
	})
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// TestHvacr01_ListDevices_ReturnsReportEnabled 는 list_devices 가 IDU + ODU 각각의
// report_enabled / device_id / device_type 을 포함해 반환하는지 검증한다.
func TestHvacr01_ListDevices_ReturnsReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	ts := time.Now()
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, ts))
	a.handleIDUFrame(icp01TestIDUFrame(2, 25.0, ts))
	a.handleODUFrame(icp01TestODUFrameSEQ02(ts))
	drainMsgCh(a)

	// IDU#1 만 off.
	a.mu.Lock()
	a.iduDevices["81"].ReportEnabled = false
	a.mu.Unlock()

	b, err := a.processListDevices()
	require.NoError(t, err)

	var resp struct {
		Status  string           `json:"status"`
		Devices []map[string]any `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp.Status)

	byAddr := map[string]map[string]any{}
	for _, d := range resp.Devices {
		byAddr[d["address"].(string)] = d
	}
	require.Contains(t, byAddr, "81")
	require.Contains(t, byAddr, "82")
	require.Contains(t, byAddr, "odu")

	assert.Equal(t, false, byAddr["81"]["report_enabled"], "off IDU 는 report_enabled=false")
	assert.Equal(t, true, byAddr["82"]["report_enabled"], "on IDU 는 report_enabled=true")
	assert.Equal(t, true, byAddr["odu"]["report_enabled"], "ODU 는 기본 report_enabled=true")
	assert.NotEmpty(t, byAddr["81"]["device_id"], "device_id(UUID) 가 노출되어야 한다")
	assert.Equal(t, "HVACR.IDU", byAddr["81"]["device_type"])
	assert.Equal(t, "HVACR.ODU", byAddr["odu"]["device_type"])
}

// TestHvacr01_Provider_ExposesReportEnabled 는 provider→adapter 경로가 report_enabled 를
// REST DTO 로 노출하는지 검증한다(reportEnabledCarrier optional-interface).
func TestHvacr01_Provider_ExposesReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp01(t)
	a := newIcp01MgmtTestAgent(t)
	a.handleIDUFrame(icp01TestIDUFrame(1, 25.0, time.Now()))
	drainMsgCh(a)

	a.mu.Lock()
	a.iduDevices["81"].ReportEnabled = false
	a.mu.Unlock()

	prov := NewIcp01DeviceProvider(a)
	d, err := prov.Device("test-lg01:81")
	require.NoError(t, err)

	rc, ok := d.(interface{ ReportEnabled() bool })
	require.True(t, ok, "adapter 는 ReportEnabled() 를 구현해야 한다")
	assert.False(t, rc.ReportEnabled(), "off IDU 는 adapter.ReportEnabled()=false")
}
