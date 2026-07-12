package samsung

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// SPEC-HVACR-CONNSTATE-001: Samsung connection-state 리포팅 테스트
// ---------------------------------------------------------------------------

// newConnAgent 는 connection-state 테스트용 Samsung 에이전트를 생성한다.
// 모든 device 는 Online=false 로 시작한다(production 초기 상태와 일치).
func newConnAgent(t *testing.T, connReport, probeTimeout time.Duration, addrs ...string) (*Hvacr01Agent, *mockTransport) {
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
			TransportType:            "serial",
			SerialPort:               "/dev/ttyTest",
			PollInterval:             30 * time.Second,
			MsgChannelSize:           256,
			OfflineThreshold:         3,
			OfflineTimeout:           -1, // production 기본값(미설정) 미러 → staleOfflineThreshold 파생 경로
			ReconnectInterval:        10 * time.Millisecond,
			MaxReconnectBackoff:      50 * time.Millisecond,
			ConnectionReportInterval: connReport,
			StartupProbeTimeout:      probeTimeout,
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
			Address: addr,
			UnitID:  s,
			Type:    "HVACR.IDU",
			Online:  false,
			State:   &NasaDeviceState{RawMessageSets: make(map[uint16][]byte)},
			Source:  "config",
		}
		a.deviceIDs[s] = addr
	}

	return a, mt
}

// drainConnMsgs 는 msgCh 에서 device_connection.* 메시지만 추출하여 파싱해 반환한다.
func drainConnMsgs(t *testing.T, a *Hvacr01Agent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for {
		select {
		case b := <-a.msgCh:
			m := parseConnMsg(b)
			if m != nil {
				out = append(out, m)
			}
		default:
			return out
		}
	}
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

// AC-5: startup probe 성공 → 초기 online 방출.
func TestConn_StartupProbeSuccess_OnlineInitial(t *testing.T) {
	a, _ := newConnAgent(t, 0, 200*time.Millisecond, "200001")
	addr, _ := ParseNasaAddress("200001")

	// 첫 성공 통신 도착 → initial=online 을 방출해야 한다.
	a.handleMessage(onlineFrame(addr))

	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1, "device 당 정확히 1개의 initial")
	assert.Equal(t, "initial", msgs[0]["trigger"])
	assert.Equal(t, "online", msgs[0]["connection_state"])
	assert.Equal(t, true, msgs[0]["connected"])
	assertConnEnvelope(t, msgs[0], "initial")
}

// AC-6: startup probe 실패 → 초기 offline (offline_threshold 대기 없음) + 이후 online change.
func TestConn_StartupProbeFail_OfflineInitial_NoThreshold(t *testing.T) {
	a, _ := newConnAgent(t, 0, 50*time.Millisecond, "200001")
	addr, _ := ParseNasaAddress("200001")

	// 타임아웃 확정을 직접 트리거(단일 실패 → 즉시 offline).
	done := a.resolveStartupProbes(true)
	assert.True(t, done)

	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "initial", msgs[0]["trigger"])
	assert.Equal(t, "offline", msgs[0]["connection_state"])

	a.mu.RLock()
	ec := a.devices[addr].ErrorCount
	a.mu.RUnlock()
	assert.Equal(t, 0, ec, "offline_threshold 소진 없이 즉시 offline (S4/N8)")

	// 이후 첫 성공 poll → online change (E3).
	a.handleMessage(onlineFrame(addr))
	msgs = drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "change", msgs[0]["trigger"])
	assert.Equal(t, "online", msgs[0]["connection_state"])
}

// AC-7: Start 시 transport 미가용 → 전체 device 초기 offline.
func TestConn_TransportUnavailable_AllOfflineInitial(t *testing.T) {
	a, mt := newConnAgent(t, 0, 60*time.Millisecond, "200001", "200002", "200003")
	mt.setAvailable(false)

	// probe 루프를 직접 구동(background 와 동일 로직).
	a.wg.Add(1)
	go a.startupProbeLoop()
	a.wg.Wait()

	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 3, "device 당 1개씩 initial=offline")
	seen := map[string]bool{}
	for _, m := range msgs {
		assert.Equal(t, "initial", m["trigger"])
		assert.Equal(t, "offline", m["connection_state"])
		assert.Equal(t, false, m["transport_connected"])
		seen[m["unit_id"].(string)] = true
	}
	assert.Len(t, seen, 3, "3개 device 모두 개별 방출")
}

// AC-8: Start 는 probe 완료를 기다리지 않고 즉시 반환 (비동기).
func TestConn_StartReturnsImmediately(t *testing.T) {
	// probe timeout 을 크게 잡아 probe 가 in-flight 상태로 유지되게 한다.
	a, _ := newConnAgent(t, 0, 5*time.Second, "200001", "200002")

	start := time.Now()
	require.NoError(t, a.Start(context.Background()))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 500*time.Millisecond, "Start 는 probe timeout(5s)을 기다리지 않아야 한다")

	require.NoError(t, a.Stop(context.Background()))
}

// AC-9 / S5: per-device 순서 — initial 미방출 device 는 주기 report 에서 제외.
func TestConn_ReportSkipsUnemittedInitial(t *testing.T) {
	a, _ := newConnAgent(t, time.Hour, 5*time.Second, "200001", "200002")
	addr1, _ := ParseNasaAddress("200001")

	// device1 만 online 확정(initial=online). device2 는 미방출 상태.
	a.handleMessage(onlineFrame(addr1))
	_ = drainConnMsgs(t, a) // initial 소비

	// 주기 report 직접 트리거.
	a.emitConnectionReports()
	msgs := drainConnMsgs(t, a)

	require.Len(t, msgs, 1, "initial 방출된 device 만 report 대상 (S5)")
	assert.Equal(t, "report", msgs[0]["trigger"])
	assert.Equal(t, "20.00.01", msgs[0]["unit_id"]) // SPEC-DEVICE-IDENTITY-001: dotted address
}

// AC-10: in-flight probe 중 Stop → leak 없음, Stop 이후 방출 없음.
func TestConn_StopDuringProbe_NoLeakNoEmit(t *testing.T) {
	a, _ := newConnAgent(t, 0, 5*time.Second, "200001", "200002")

	require.NoError(t, a.Start(context.Background()))
	// probe 가 in-flight (device offline, timeout 대기 중).
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, a.Stop(context.Background()))

	// Stop 이후 어떤 connection 메시지도 남아있지 않아야 한다(초기 offline 포함).
	msgs := drainConnMsgs(t, a)
	assert.Empty(t, msgs, "Stop 시작 이후에는 어떤 메시지도 방출되지 않아야 한다 (N10)")
}

// AC-13: offline device 가 주기 report 에 포함.
func TestConn_ReportIncludesOfflineDevices(t *testing.T) {
	a, _ := newConnAgent(t, time.Hour, 5*time.Second, "200001", "200002", "200003")
	addr1, _ := ParseNasaAddress("200001")
	addr2, _ := ParseNasaAddress("200002")
	addr3, _ := ParseNasaAddress("200003")

	// 세 device 모두 initial 확정: 2개 online, 1개 offline.
	a.handleMessage(onlineFrame(addr1))
	a.handleMessage(onlineFrame(addr2))
	a.mu.Lock()
	a.devices[addr3].connInitialEmitted = true // offline 로 initial 확정 시뮬레이션
	a.mu.Unlock()
	_ = drainConnMsgs(t, a)

	a.emitConnectionReports()
	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 3)

	states := map[string]string{}
	for _, m := range msgs {
		assert.Equal(t, "report", m["trigger"])
		states[m["unit_id"].(string)] = m["connection_state"].(string)
	}
	// SPEC-DEVICE-IDENTITY-001: unit_id 는 dotted address format
	assert.Equal(t, "offline", states["20.00.03"], "offline device 도 report 에 포함 (S2/N5)")
}

// AC-14: 개별 메시지 (배칭 금지).
func TestConn_ReportIndividualMessages(t *testing.T) {
	a, _ := newConnAgent(t, time.Hour, 5*time.Second, "200001", "200002")
	a.mu.Lock()
	for _, d := range a.devices {
		d.connInitialEmitted = true
	}
	a.mu.Unlock()

	a.emitConnectionReports()
	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 2, "N개 device → N개 개별 메시지, 배열 배칭 금지 (N3/AC-14)")
	for _, m := range msgs {
		assert.NotContains(t, m, "devices", "배열 배칭 금지")
	}
}

// AC-4: Samsung 대칭 online/offline — offline 후 복구 시 online change.
func TestConn_SamsungSymmetricOnlineRecovery(t *testing.T) {
	a, _ := newConnAgent(t, 0, 5*time.Second, "200001")
	addr, _ := ParseNasaAddress("200001")

	// 1) online baseline
	a.handleMessage(onlineFrame(addr))
	_ = drainConnMsgs(t, a)

	// 2) offline_threshold 도달 → offline change
	for i := 0; i < a.hvacr01Config.OfflineThreshold; i++ {
		a.incrementErrorCount(addr)
	}
	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "change", msgs[0]["trigger"])
	assert.Equal(t, "offline", msgs[0]["connection_state"])

	// 3) 복구 → online change (기존 Samsung 에 없던 대칭 이벤트)
	a.handleMessage(onlineFrame(addr))
	msgs = drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "change", msgs[0]["trigger"])
	assert.Equal(t, "online", msgs[0]["connection_state"])
}

// AC-3: change 는 tick 과 독립적으로 즉시 방출 (report 비활성 상태에서도).
func TestConn_ChangeIndependentOfTick(t *testing.T) {
	a, _ := newConnAgent(t, 0, 5*time.Second, "200001") // connection_report_interval = 0
	addr, _ := ParseNasaAddress("200001")

	a.handleMessage(onlineFrame(addr)) // initial=online
	_ = drainConnMsgs(t, a)

	for i := 0; i < a.hvacr01Config.OfflineThreshold; i++ {
		a.incrementErrorCount(addr)
	}
	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1, "interval=0 이어도 change 는 즉시 방출 (N2/AC-2/AC-3)")
	assert.Equal(t, "change", msgs[0]["trigger"])
	assert.Equal(t, "offline", msgs[0]["connection_state"])
}

// AC-11: reconnect 는 change, 두 번째 initial 없음.
func TestConn_ReconnectNoSecondInitial(t *testing.T) {
	a, _ := newConnAgent(t, 0, 50*time.Millisecond, "200001")
	addr, _ := ParseNasaAddress("200001")

	// transport 미가용 startup → initial=offline
	done := a.resolveStartupProbes(true)
	assert.True(t, done)
	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	assert.Equal(t, "initial", msgs[0]["trigger"])

	// 여러 번 재연결/재offline/재online 반복 → 모두 change, initial 재방출 없음
	a.handleMessage(onlineFrame(addr)) // online change
	for i := 0; i < a.hvacr01Config.OfflineThreshold; i++ {
		a.incrementErrorCount(addr)
	}
	a.handleMessage(onlineFrame(addr)) // online change 다시

	msgs = drainConnMsgs(t, a)
	for _, m := range msgs {
		assert.NotEqual(t, "initial", m["trigger"], "두 번째 initial 은 절대 없음 (N9)")
	}
}

// AC-16 / S3: report trigger 의 event_ms 는 tick 시점 time.Now, last_seen_ms 는 별개.
func TestConn_ReportEventMsCanonical(t *testing.T) {
	a, _ := newConnAgent(t, time.Hour, 5*time.Second, "200001")
	addr, _ := ParseNasaAddress("200001")

	past := time.Now().Add(-1 * time.Hour)
	a.mu.Lock()
	a.devices[addr].Online = true
	a.devices[addr].LastSeen = past
	a.devices[addr].connInitialEmitted = true
	a.mu.Unlock()

	before := time.Now().UnixMilli()
	a.emitConnectionReports()
	after := time.Now().UnixMilli()

	msgs := drainConnMsgs(t, a)
	require.Len(t, msgs, 1)
	eventMs := int64(msgs[0]["event_ms"].(float64))
	lastSeenMs := int64(msgs[0]["last_seen_ms"].(float64))
	assert.GreaterOrEqual(t, eventMs, before)
	assert.LessOrEqual(t, eventMs, after)
	assert.Equal(t, past.UnixMilli(), lastSeenMs, "last_seen_ms 는 실제 마지막 통신 시각 유지")
	assert.NotEqual(t, eventMs, lastSeenMs)
}

// AC-19 (deadlock 회귀 방지): lock 보유 경로(offline/probe/report)에서 방출 시
// a.Name()/a.ID() 를 호출하지 않아야 한다. 호출하면 이 테스트가 timeout(deadlock) 된다.
func TestConn_NoDeadlockUnderLock(t *testing.T) {
	a, _ := newConnAgent(t, time.Hour, 30*time.Millisecond, "200001", "200002")
	addr1, _ := ParseNasaAddress("200001")

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		// startup probe (background, lock 보유 emit)
		a.wg.Add(1)
		go a.startupProbeLoop()
		a.wg.Wait()
		// online baseline + offline change (lock 보유 emit)
		a.handleMessage(onlineFrame(addr1))
		for i := 0; i < a.hvacr01Config.OfflineThreshold; i++ {
			a.incrementErrorCount(addr1)
		}
		// report (lock 보유 emit)
		a.emitConnectionReports()
	}()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock 의심: lock 보유 중 a.Name()/a.ID() 호출 회귀 (N1/AC-19)")
	}
}

// parseConnMsg 는 raw 메시지를 파싱하여 device_connection.* 이면 map 을, 아니면 nil 을 반환한다.
func parseConnMsg(b []byte) map[string]any {
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

// parseConnMsg / assertConnEnvelope 는 connection_test 와 config 테스트에서 공유한다.
func assertConnEnvelope(t *testing.T, m map[string]any, wantTrigger string) {
	t.Helper()
	assert.Contains(t, m, "unit_id")
	assert.Contains(t, m, "device_id")
	assert.Equal(t, wantTrigger, m["trigger"])
	assert.Contains(t, m, "event_ms")
	assert.NotContains(t, m, "type", "최상위 type 필드는 없어야 한다 (AC-17)")
	cs, _ := m["connection_state"].(string)
	assert.True(t, cs == "online" || cs == "offline", "connection_state ∈ {online, offline}")
	md, ok := m["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "device_connection."+wantTrigger, md["message_type"])
	assert.True(t, strings.HasPrefix(md["message_type"].(string), "device_connection."))
}

// TestProbeSilentDevices_PassiveMode 는 passive 모드에서 침묵한 online 디바이스에
// 확인 probe(status query)가 전송되는지 검증한다.
//
// 회귀 배경: passive(status_query_enabled=false)는 블랭킷 폴을 안 하므로, 디바이스가
// 자체 브로드캐스트 주기(> offline 임계값)로만 통신하면 정상 online 인데도 90초 임계에
// 걸려 online/offline 플래핑했다. 침묵 시 확인 probe 로 응답을 유도하면 online 이
// 유지되어 플래핑이 사라진다.
func TestProbeSilentDevices_PassiveMode(t *testing.T) {
	a, mt := newConnAgent(t, 0, 0, "200001")
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
	a, mt := newConnAgent(t, 0, 0, "200001")
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
	a, mt := newConnAgent(t, 0, 0, "200001")
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
