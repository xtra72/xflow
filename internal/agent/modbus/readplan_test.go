package modbus

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 REQ-02/REQ-03 — 읽기 계획(블록 병합) 순수 함수 검증
// ---------------------------------------------------------------------------

// rg 는 테스트용 그룹 생성 헬퍼이다.
func rg(name string, fc byte, addr, qty uint16) RegisterGroupConfig {
	return RegisterGroupConfig{Name: name, FunctionCode: fc, StartAddress: addr, Quantity: qty}
}

// withInterval 은 그룹에 전용 케이던스를 부여한다.
func withInterval(g RegisterGroupConfig, d time.Duration) RegisterGroupConfig {
	g.PollInterval = d
	return g
}

// withEnabled 는 그룹의 사용 여부를 명시한다.
func withEnabled(g RegisterGroupConfig, v bool) RegisterGroupConfig {
	g.Enabled = &v
	return g
}

// blockShape 는 블록의 (fc, 시작주소, 수량, 멤버수)를 비교용으로 뽑는다.
func blockShape(plan []ReadBlock) [][4]int {
	out := make([][4]int, 0, len(plan))
	for _, b := range plan {
		out = append(out, [4]int{int(b.FunctionCode), int(b.StartAddress), int(b.Quantity), len(b.Members)})
	}
	return out
}

// TestBuildReadPlan_MergesContiguous 는 연속 그룹이 하나의 블록으로 병합됨을 검증한다(AC-04).
func TestBuildReadPlan_MergesContiguous(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{
		rg("vr", 4, 4, 2), rg("vs", 4, 6, 2), rg("vt", 4, 8, 2),
	}, 32)
	assert.Equal(t, [][4]int{{4, 4, 6, 3}}, blockShape(plan))
}

// TestBuildReadPlan_GapSplits 는 주소 간극이 있으면 병합하지 않음을 검증한다(AC-05).
// 미정의 주소 읽기로 인한 ILLEGAL DATA ADDRESS 를 회피하기 위한 규칙이다.
func TestBuildReadPlan_GapSplits(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{
		rg("a", 4, 4, 2), rg("b", 4, 8, 2), // 주소 6-7 미정의
	}, 32)
	assert.Equal(t, [][4]int{{4, 4, 2, 1}, {4, 8, 2, 1}}, blockShape(plan))
}

// TestBuildReadPlan_CohortSeparation 은 fc 또는 폴링 주기가 다르면 병합되지 않음을 검증한다(AC-06).
func TestBuildReadPlan_CohortSeparation(t *testing.T) {
	t.Run("fc 다름", func(t *testing.T) {
		plan := buildReadPlan([]RegisterGroupConfig{
			rg("h", 3, 0, 2), rg("i", 4, 2, 2),
		}, 32)
		assert.Len(t, plan, 2)
	})
	t.Run("폴링 주기 다름", func(t *testing.T) {
		plan := buildReadPlan([]RegisterGroupConfig{
			withInterval(rg("fast", 4, 0, 2), 2*time.Second),
			withInterval(rg("slow", 4, 2, 2), 300*time.Second),
		}, 32)
		require.Len(t, plan, 2)
		assert.Equal(t, 2*time.Second, plan[0].PollInterval)
		assert.Equal(t, 300*time.Second, plan[1].PollInterval)
	})
}

// TestBuildReadPlan_DisabledExcludedAndSplits 는 미사용 그룹이 블록에서 제외되고
// 블록 경계를 나눔을 검증한다(AC-07).
func TestBuildReadPlan_DisabledExcludedAndSplits(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{
		rg("a", 4, 4, 2),
		withEnabled(rg("b", 4, 6, 2), false), // 가운데 미사용
		rg("c", 4, 8, 2),
	}, 32)
	assert.Equal(t, [][4]int{{4, 4, 2, 1}, {4, 8, 2, 1}}, blockShape(plan))

	// 주소 6-7 은 어떤 블록에도 포함되지 않아야 한다.
	for _, b := range plan {
		for addr := b.StartAddress; addr < b.StartAddress+b.Quantity; addr++ {
			assert.NotContains(t, []uint16{6, 7}, addr, "미사용 구간이 읽기 범위에 포함되면 안 된다")
		}
	}
}

// TestBuildReadPlan_AllDisabled 는 전체 미사용이면 계획이 비는지 검증한다(AC-03).
func TestBuildReadPlan_AllDisabled(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{
		withEnabled(rg("a", 4, 0, 2), false),
		withEnabled(rg("b", 4, 2, 2), false),
	}, 32)
	assert.Empty(t, plan)
}

// TestBuildReadPlan_MaxBlockSplit 는 상한 초과 시 분할됨을 검증한다(AC-11).
func TestBuildReadPlan_MaxBlockSplit(t *testing.T) {
	groups := make([]RegisterGroupConfig, 0, 40)
	for i := uint16(0); i < 40; i++ {
		groups = append(groups, rg("r", 4, i, 1))
	}
	plan := buildReadPlan(groups, 32)
	assert.Equal(t, [][4]int{{4, 0, 32, 32}, {4, 32, 8, 8}}, blockShape(plan))
	for _, b := range plan {
		assert.LessOrEqual(t, b.Quantity, uint16(32))
	}
}

// TestBuildReadPlan_OversizedSingleGroupPassesThrough 는 단일 그룹이 상한보다 커도
// 분할하지 않고 단독 블록으로 통과함을 검증한다(AC-14, 기존 동작 보존).
func TestBuildReadPlan_OversizedSingleGroupPassesThrough(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{rg("big", 3, 0, 50)}, 32)
	assert.Equal(t, [][4]int{{3, 0, 50, 1}}, blockShape(plan))
}

// TestBuildReadPlan_OverlappingGroups 는 주소가 겹치는 그룹도 하나의 블록으로 합쳐지고
// 합집합 범위만 읽음을 검증한다.
func TestBuildReadPlan_OverlappingGroups(t *testing.T) {
	plan := buildReadPlan([]RegisterGroupConfig{
		rg("wide", 3, 0, 10), rg("inner", 3, 5, 3),
	}, 32)
	assert.Equal(t, [][4]int{{3, 0, 10, 2}}, blockShape(plan))
}

// TestClampMaxBlock 는 블록 상한 클램프를 검증한다(AC-10, AC-13).
func TestClampMaxBlock(t *testing.T) {
	assert.Equal(t, DefaultMaxBlockRegisters, clampMaxBlock(0), "0(미지정) → 기본 32")
	assert.Equal(t, uint16(1), clampMaxBlock(1))
	assert.Equal(t, uint16(56), clampMaxBlock(56))
	assert.Equal(t, uint16(MaxRegistersRead), clampMaxBlock(500), "규격 상한으로 클램프")
}

// TestEffectiveMaxBlock_DeviceOverride 는 디바이스 오버라이드 상속을 검증한다(AC-12).
func TestEffectiveMaxBlock_DeviceOverride(t *testing.T) {
	cfg := ModbusConfig{MaxBlockRegisters: 32}
	override := uint16(56)
	assert.Equal(t, uint16(32), effectiveMaxBlock(DeviceConfig{}, cfg), "미지정 → 에이전트 값 상속")
	assert.Equal(t, uint16(56), effectiveMaxBlock(DeviceConfig{MaxBlockRegisters: &override}, cfg))
}

// TestParseConfig_MaxBlockRegisters 는 설정 파싱(기본값·클램프·디바이스 오버라이드)을
// 검증한다(AC-10, AC-12, AC-13).
func TestParseConfig_MaxBlockRegisters(t *testing.T) {
	base := func(opts map[string]any) map[string]any {
		m := map[string]any{
			"read_mode": "cached",
			"devices": []any{
				map[string]any{"id": "d1", "host": "h", "unit_id": 1,
					"register_groups": []any{map[string]any{"function_code": 3, "start_address": 0, "quantity": 1}}},
			},
		}
		for k, v := range opts {
			m[k] = v
		}
		return m
	}

	t.Run("미지정 → 32", func(t *testing.T) {
		cfg, err := parseModbusConfig(base(nil))
		require.NoError(t, err)
		assert.Equal(t, DefaultMaxBlockRegisters, cfg.MaxBlockRegisters)
		assert.Nil(t, cfg.Devices[0].MaxBlockRegisters, "디바이스 미지정은 nil 상속")
	})
	t.Run("0 → 1 로 클램프되지 않고 기본값", func(t *testing.T) {
		cfg, err := parseModbusConfig(base(map[string]any{"max_block_registers": 0}))
		require.NoError(t, err)
		assert.Equal(t, DefaultMaxBlockRegisters, cfg.MaxBlockRegisters)
	})
	t.Run("500 → 125 클램프", func(t *testing.T) {
		cfg, err := parseModbusConfig(base(map[string]any{"max_block_registers": 500}))
		require.NoError(t, err)
		assert.Equal(t, uint16(MaxRegistersRead), cfg.MaxBlockRegisters)
	})
	t.Run("디바이스 오버라이드", func(t *testing.T) {
		opts := base(map[string]any{"max_block_registers": 32})
		opts["devices"].([]any)[0].(map[string]any)["max_block_registers"] = 56
		cfg, err := parseModbusConfig(opts)
		require.NoError(t, err)
		require.NotNil(t, cfg.Devices[0].MaxBlockRegisters)
		assert.Equal(t, uint16(56), *cfg.Devices[0].MaxBlockRegisters)
	})
}

// ---------------------------------------------------------------------------
// 블록 결과 분배 (AC-08) — 오프셋 계산
// ---------------------------------------------------------------------------

// TestSliceMemberData_Registers 는 레지스터 블록의 멤버 슬라이싱 오프셋을 검증한다(AC-08).
func TestSliceMemberData_Registers(t *testing.T) {
	block := ReadBlock{FunctionCode: 4, StartAddress: 4, Quantity: 6}
	// 주소 4,5,6,7,8,9 → 값 0x0004..0x0009
	data := []byte{0, 4, 0, 5, 0, 6, 0, 7, 0, 8, 0, 9}

	got, ok := sliceMemberData(block, data, rg("m", 4, 6, 2))
	require.True(t, ok)
	assert.Equal(t, []byte{0, 6, 0, 7}, got, "오프셋은 (멤버주소-블록주소)*2 여야 한다")

	got, ok = sliceMemberData(block, data, rg("first", 4, 4, 2))
	require.True(t, ok)
	assert.Equal(t, []byte{0, 4, 0, 5}, got)

	got, ok = sliceMemberData(block, data, rg("last", 4, 8, 2))
	require.True(t, ok)
	assert.Equal(t, []byte{0, 8, 0, 9}, got)
}

// TestSliceMemberData_ShortResponse 는 응답이 짧으면 ok=false 로 안전 실패함을 검증한다.
func TestSliceMemberData_ShortResponse(t *testing.T) {
	block := ReadBlock{FunctionCode: 3, StartAddress: 0, Quantity: 4}
	_, ok := sliceMemberData(block, []byte{0, 1, 0, 2}, rg("m", 3, 2, 2))
	assert.False(t, ok, "응답이 2레지스터뿐이면 주소 2-3 구간을 자를 수 없다")
}

// TestSliceMemberData_Coils 는 코일 블록의 비트 재포장을 검증한다.
// 비트 순서는 decodeCoils 와 동일하게 각 바이트의 LSB 가 앞선 주소이다.
func TestSliceMemberData_Coils(t *testing.T) {
	// 블록: 주소 0-11, 비트열 = 1,0,1,1, 0,0,0,1, 1,1,0,0
	// byte0 = 0b1000_1101 = 0x8D, byte1 = 0b0000_0011 = 0x03
	block := ReadBlock{FunctionCode: FC01ReadCoils, StartAddress: 0, Quantity: 12}
	data := []byte{0x8D, 0x03}

	// 멤버: 주소 4부터 4비트 → 0,0,0,1 → 0b0000_1000 = 0x08
	got, ok := sliceMemberData(block, data, rg("m", FC01ReadCoils, 4, 4))
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, byte(0x08), got[0])
	assert.Equal(t, []bool{false, false, false, true}, decodeCoils(got, 4))

	// 멤버: 주소 8부터 4비트 → 1,1,0,0 → 0b0000_0011 = 0x03
	got, ok = sliceMemberData(block, data, rg("m2", FC01ReadCoils, 8, 4))
	require.True(t, ok)
	assert.Equal(t, []bool{true, true, false, false}, decodeCoils(got, 4))
}

// ---------------------------------------------------------------------------
// 통합 — 실제 폴링에서 병합이 일어나고 메시지는 그룹 단위로 유지되는지 (AC-04/AC-08/AC-09)
// ---------------------------------------------------------------------------

// coalesceAgentConfig 는 fc4 연속 3그룹(각 2워드)을 가진 단일 디바이스 설정을 만든다.
func coalesceAgentConfig(maxBlock any) agent.AgentConfig {
	opts := map[string]any{
		"read_mode":        "cached",
		"mode":             "interval",
		"poll_interval":    "10s",
		"request_timeout":  "1s",
		"msg_channel_size": 64,
		"devices": []any{
			map[string]any{
				"id": "d1", "host": "10.0.0.1", "port": 502, "unit_id": 1,
				"register_groups": []any{
					map[string]any{"name": "vr", "function_code": 4, "start_address": 4, "quantity": 2},
					map[string]any{"name": "vs", "function_code": 4, "start_address": 6, "quantity": 2},
					map[string]any{"name": "vt", "function_code": 4, "start_address": 8, "quantity": 2},
				},
			},
		},
	}
	if maxBlock != nil {
		opts["max_block_registers"] = maxBlock
	}
	return agent.AgentConfig{
		ID: "modbus-coalesce", Name: "Coalesce Agent", Type: "modbus-client",
		Transport: agent.TransportConfig{Type: "modbus-tcp", Options: opts},
	}
}

// buildFC04ResponseValues 는 지정 값들을 담은 FC04 응답 프레임을 만든다.
func buildFC04ResponseValues(values []uint16) []byte {
	byteCount := byte(len(values) * 2)
	length := uint16(3 + byteCount)
	resp := []byte{0, 0, 0, 0, byte(length >> 8), byte(length), 1, FC04ReadInputRegisters, byteCount}
	for _, v := range values {
		resp = append(resp, byte(v>>8), byte(v))
	}
	return resp
}

// TestPoll_CoalescedRead_SingleTransaction 는 연속 3그룹이 물리 읽기 1회로 합쳐지고,
// 방출 메시지는 그룹 3건으로 유지되며 각 그룹이 자기 구간 값을 받는지 검증한다(AC-04/AC-08).
func TestPoll_CoalescedRead_SingleTransaction(t *testing.T) {
	// 주소 4..9 = 값 104..109
	mt := &mockModbusTransport{connected: true,
		response: buildFC04ResponseValues([]uint16{104, 105, 106, 107, 108, 109})}
	a, _ := newTestModbusAgent(t, coalesceAgentConfig(nil), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	a.pollDevices(false)

	// 물리 읽기는 1회, 범위는 주소 4부터 6워드.
	assert.Equal(t, [][3]int{{4, 4, 6}}, readPDUAddrs(t, mt),
		"연속 3그룹은 물리 읽기 1회로 병합되어야 한다")

	// 메시지는 그룹 3건으로 유지되고, 각자 자기 구간 데이터를 받아야 한다.
	events := drainEvents(a, 200*time.Millisecond)
	byGroup := map[string][]byte{}
	for _, e := range events {
		if e["type"] != "register_data" {
			continue
		}
		// sendEvent 는 봉투를 평평하게 만든다: {"type":..., "group_name":..., "data":...}
		name, _ := e["group_name"].(string)
		// evtData["data"] 는 []byte 이므로 JSON 에서 base64 문자열로 직렬화된다.
		enc, _ := e["data"].(string)
		raw, err := base64.StdEncoding.DecodeString(enc)
		require.NoError(t, err, "그룹 %q 의 data 는 base64 여야 한다", name)
		byGroup[name] = raw
	}
	require.Len(t, byGroup, 3, "그룹 단위 메시지 3건이 방출되어야 한다")
	assert.Equal(t, []byte{0, 104, 0, 105}, byGroup["vr"])
	assert.Equal(t, []byte{0, 106, 0, 107}, byGroup["vs"])
	assert.Equal(t, []byte{0, 108, 0, 109}, byGroup["vt"])
}

// TestPoll_MaxBlockLimitsCoalescing 는 블록 상한이 병합 범위를 제한함을 검증한다(AC-11).
func TestPoll_MaxBlockLimitsCoalescing(t *testing.T) {
	mt := &mockModbusTransport{connected: true,
		response: buildFC04ResponseValues([]uint16{104, 105, 106, 107})}
	// 상한 4 → 6워드를 한 번에 못 읽으므로 4워드 + 2워드 두 블록.
	a, _ := newTestModbusAgent(t, coalesceAgentConfig(4), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	a.pollDevices(false)

	assert.Equal(t, [][3]int{{4, 4, 4}, {4, 8, 2}}, readPDUAddrs(t, mt))
}

// TestPoll_BlockFailure_AllMembersRecordedFailed 는 블록 읽기 실패가 멤버 전체 실패로
// 기록되고 캐시 갱신·메시지 방출이 없음을 검증한다(AC-09).
func TestPoll_BlockFailure_AllMembersRecordedFailed(t *testing.T) {
	mt := &mockModbusTransport{connected: true, sendRecvErr: assertErr{}}
	a, _ := newTestModbusAgent(t, coalesceAgentConfig(nil), mt)
	a.devices[0].mu.Lock()
	a.devices[0].online = true
	a.devices[0].mu.Unlock()

	a.pollDevices(false)

	// 멤버 3개 모두 실패 통계가 1회씩 기록되어야 한다.
	a.mu.RLock()
	stats := a.groupStats
	a.mu.RUnlock()
	for _, name := range []string{"vr", "vs", "vt"} {
		c, ok := stats[groupStatKey("d1", name)]
		require.True(t, ok, "그룹 %q 통계가 있어야 한다", name)
		assert.Equal(t, int64(1), c.errors.Load(), "그룹 %q 는 실패 1회여야 한다", name)
	}

	// register_data 이벤트는 방출되지 않아야 한다.
	for _, e := range drainEvents(a, 150*time.Millisecond) {
		assert.NotEqual(t, "register_data", e["type"], "실패 블록은 메시지를 방출하면 안 된다")
	}
}

// assertErr 는 테스트용 고정 오류이다.
type assertErr struct{}

func (assertErr) Error() string { return "mock send error" }
