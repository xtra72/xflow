package modbusserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	modbus "github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// 캡처 slog 핸들러 (프레임/디바이스 로그 검증용)
// ---------------------------------------------------------------------------

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

type captureHandler struct {
	mu    sync.Mutex
	recs  []capturedRecord
	level slog.Level
}

func newCaptureHandler(level slog.Level) *captureHandler {
	return &captureHandler{level: level}
}

func (h *captureHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{level: r.Level, msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.recs = append(h.recs, rec)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) snapshot() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]capturedRecord, len(h.recs))
	copy(out, h.recs)
	return out
}

// framesByDir 는 "modbus-gateway: frame" 레코드 중 dir 이 일치하는 것을 반환한다.
func framesByDir(recs []capturedRecord, dir string) []capturedRecord {
	var out []capturedRecord
	for _, r := range recs {
		if r.msg == "modbus-gateway: frame" && r.attrs["dir"] == dir {
			out = append(out, r)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// (a) 클라이언트 레지스트리 + list_clients
// ---------------------------------------------------------------------------

// 레지스트리 단위: 여러 요청에 걸쳐 remote_addr + 접근 unit_id 집합을 기록한다.
func TestClientRegistry_RecordsUnitIDSetAcrossRequests(t *testing.T) {
	r := NewClientRegistry()
	r.Add("10.0.0.5:6000")
	r.RecordAccess("10.0.0.5:6000", 1)
	r.RecordAccess("10.0.0.5:6000", 2)
	r.RecordAccess("10.0.0.5:6000", 1) // 중복 unit → 집합엔 1개, 카운트는 증가

	list := r.List()
	require.Len(t, list, 1)
	assert.Equal(t, "10.0.0.5:6000", list[0].RemoteAddr)
	assert.Equal(t, []byte{1, 2}, list[0].UnitIDs) // 오름차순 집합
	assert.Equal(t, int64(3), list[0].RequestCount)
	assert.False(t, list[0].LastSeen.IsZero())

	r.Remove("10.0.0.5:6000")
	assert.Len(t, r.List(), 0)
}

// 통합: 연결된 클라이언트가 list_clients 로 노출되고 접근 unit_id 가 기록된다.
func TestProcessListClients_ReturnsConnectedClient(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg, nil)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)
	require.NoError(t, msa.Start(context.Background()))
	defer func() { _ = msa.Stop(context.Background()) }()

	addr := msa.ListenAddr()
	require.NotNil(t, addr)

	conn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// unit 1 로 읽기 요청 → 처리되면 RecordAccess 가 unit_id 1 을 기록한다.
	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 1, pdu)
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(frame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(respBuf) // 응답 수신 = 요청 처리(RecordAccess) 완료 보장
	require.NoError(t, err)

	// 연결이 살아있는 동안 list_clients 조회.
	data, _ := json.Marshal(map[string]any{"command": "list_clients"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	clients, ok := result["clients"].([]any)
	require.True(t, ok)
	require.Len(t, clients, 1)

	c := clients[0].(map[string]any)
	assert.NotEmpty(t, c["remote_addr"])
	assert.Contains(t, c, "connected_at")
	assert.Contains(t, c, "last_seen")
	assert.GreaterOrEqual(t, c["request_count"].(float64), float64(1))
	unitIDs, ok := c["unit_ids"].([]any)
	require.True(t, ok)
	require.Len(t, unitIDs, 1)
	assert.Equal(t, float64(1), unitIDs[0]) // JSON 정수 배열
}

// RTU 는 per-client 개념이 없어 list_clients 가 빈 배열을 반환한다.
func TestProcessListClients_RTUReturnsEmpty(t *testing.T) {
	l := &RTUListener{}
	assert.Nil(t, l.Clients())
}

// ---------------------------------------------------------------------------
// (b) 프레임 로그 (log_frames / log_raw_frames)
// ---------------------------------------------------------------------------

// newFrameLogHandler 는 프레임 로그 검증용 핸들러+ModbusHandler 를 net.Pipe 로 구동한다.
func runOneRequest(t *testing.T, frames, raw bool) []capturedRecord {
	t.Helper()
	rm := newTestRegisterMap()
	rm.WriteHoldingRegisters(0, []uint16{100})
	dm := newTestDeviceManager(rm, 1)
	msgCh := make(chan map[string]any, 10)

	cap := newCaptureHandler(slog.LevelDebug)
	logger := slog.New(cap)
	obs := &serverObs{logFrames: &atomic.Bool{}, logRawFrames: &atomic.Bool{}, registry: NewClientRegistry()}
	obs.logFrames.Store(frames)
	obs.logRawFrames.Store(raw)
	handler := NewModbusHandler(dm, msgCh, false, obs, logger)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go handler.HandleConnection(ctx, server)

	pdu := buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1)
	frame := buildMBAPFrame(1, 1, pdu)
	_, err := client.Write(frame)
	require.NoError(t, err)

	respBuf := make([]byte, 256)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = client.Read(respBuf) // 응답 수신 = RX/TX 로그 모두 방출 완료
	require.NoError(t, err)

	return cap.snapshot()
}

func TestLogFrames_EmitsRXandTX(t *testing.T) {
	recs := runOneRequest(t, true, false)

	rx := framesByDir(recs, "RX")
	tx := framesByDir(recs, "TX")
	require.Len(t, rx, 1)
	require.Len(t, tx, 1)

	// 요약 필드 확인 (unit_id, fc, len). raw off → hex 없음.
	assert.Equal(t, uint64(1), rx[0].attrs["unit_id"]) // slog 는 byte 를 uint64 로 저장
	assert.Equal(t, "0x03", rx[0].attrs["fc"])
	assert.Contains(t, rx[0].attrs, "len")
	assert.NotContains(t, rx[0].attrs, "hex")
	assert.NotContains(t, tx[0].attrs, "hex")
}

func TestLogFrames_RawIncludesHex(t *testing.T) {
	recs := runOneRequest(t, true, true)

	rx := framesByDir(recs, "RX")
	tx := framesByDir(recs, "TX")
	require.Len(t, rx, 1)
	require.Len(t, tx, 1)
	assert.Contains(t, rx[0].attrs, "hex")
	assert.NotEmpty(t, rx[0].attrs["hex"])
	assert.Contains(t, tx[0].attrs, "hex")
	assert.NotEmpty(t, tx[0].attrs["hex"])
}

func TestLogFrames_DisabledEmitsNothing(t *testing.T) {
	recs := runOneRequest(t, false, false)
	assert.Empty(t, framesByDir(recs, "RX"))
	assert.Empty(t, framesByDir(recs, "TX"))
}

// ---------------------------------------------------------------------------
// (c) Configure 로 log_frames 라이브 갱신 (재시작 없음)
// ---------------------------------------------------------------------------

func TestConfigure_LiveUpdatesLogFrames(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg, nil)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 기본값: 두 토글 모두 off.
	assert.False(t, msa.obs.logFrames.Load())
	assert.False(t, msa.obs.logRawFrames.Load())

	// Configure 로 두 토글을 켠다(재시작 없이 즉시 반영).
	newCfg := testAgentConfig()
	newCfg.Transport.Options["log_frames"] = true
	newCfg.Transport.Options["log_raw_frames"] = true
	require.NoError(t, msa.Configure(newCfg))

	assert.True(t, msa.obs.logFrames.Load())
	assert.True(t, msa.obs.logRawFrames.Load())
	// 재시작이 아니라 라이브 갱신이므로 상태는 그대로 Running.
	assert.Equal(t, lifecycle.StateRunning, msa.CurrentState())
}

// ---------------------------------------------------------------------------
// (d) add_device / remove_device INFO 로그
// ---------------------------------------------------------------------------

func TestDeviceRoster_AddRemoveEmitInfoLogs(t *testing.T) {
	cap := newCaptureHandler(slog.LevelInfo)
	cfg := testAgentConfig()
	cfg.Logger = slog.New(cap)

	a, err := NewModbusServerAgent(cfg, nil)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// add_device unit 2 (testAgentConfig 는 unit 1 을 이미 보유).
	addData, _ := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"unit_id": 2,
			"name":    "dev-2",
			"register_map": map[string]any{
				"holding_registers": map[string]any{"start_address": 0, "count": 10},
			},
		},
	})
	_, err = msa.Process(addData)
	require.NoError(t, err)

	// remove_device unit 2 (마지막 디바이스가 아니므로 허용).
	rmData, _ := json.Marshal(map[string]any{
		"command": "remove_device",
		"params":  map[string]any{"unit_id": 2},
	})
	_, err = msa.Process(rmData)
	require.NoError(t, err)

	recs := cap.snapshot()
	var added, removed *capturedRecord
	for i := range recs {
		switch recs[i].msg {
		case "modbus-gateway: device added":
			added = &recs[i]
		case "modbus-gateway: device removed":
			removed = &recs[i]
		}
	}
	require.NotNil(t, added, "expected INFO 'device added' log")
	require.NotNil(t, removed, "expected INFO 'device removed' log")
	assert.Equal(t, slog.LevelInfo, added.level)
	assert.Equal(t, uint64(2), added.attrs["unit_id"]) // slog 는 byte 를 uint64 로 저장
	assert.Equal(t, "dev-2", added.attrs["name"])
	assert.Equal(t, slog.LevelInfo, removed.level)
	assert.Equal(t, uint64(2), removed.attrs["unit_id"])
}
