// influxdb_agent_dualtag_test.go (SPEC-DEVICE-IDENTITY-001 Phase C § C3)
//
// InfluxDBAgent 의 dual-tag 부착 동작을 4 state 분류 전수 시나리오와
// 옵션 토글 / 다중 source key / 메트릭 검증으로 통합 검증한다.
//
// 본 테스트는 mock DeviceResolver 와 mock InfluxClient 를 사용하여 실제
// device.Registry 또는 Influx 서버 없이 격리 실행된다.

package system

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
)

// ===== mockDevice — device.Device 의 최소 구현 =====

type mockDevice struct {
	uid       string
	name      string
	agentName string
}

func (m *mockDevice) ID() string                      { return m.agentName + ":" + m.name }
func (m *mockDevice) UID() string                     { return m.uid }
func (m *mockDevice) Name() string                    { return m.name }
func (m *mockDevice) AgentName() string               { return m.agentName }
func (m *mockDevice) Type() device.DeviceType         { return device.DeviceType("test") }
func (m *mockDevice) Protocol() string                { return "test" }
func (m *mockDevice) Online() bool                    { return true }
func (m *mockDevice) LastSeen() time.Time             { return time.Time{} }
func (m *mockDevice) State() device.DeviceState       { return device.DeviceState{Online: true} }
func (m *mockDevice) Metadata() device.DeviceMetadata { return device.DeviceMetadata{Name: m.name} }
func (m *mockDevice) Source() string                  { return "test" }
func (m *mockDevice) Capabilities() []string          { return nil }

// ===== mockResolver — DeviceResolver 의 mock =====

// mockResolver 는 (composite ref → device) 매핑을 가진 in-memory resolver 이다.
// devices 에 없는 ref 는 ErrDeviceNotFound 반환.
type mockResolver struct {
	devices  map[string]*mockDevice // composite ref (예: "lgcnp:81") -> device
	forceErr error                  // non-nil 이면 모든 lookup 이 본 에러를 반환.
}

func (m *mockResolver) ResolveDevice(ref string) (device.Device, device.DeviceRefKind, error) {
	if m.forceErr != nil {
		return nil, device.DeviceRefUnknown, m.forceErr
	}
	dev, ok := m.devices[ref]
	if !ok {
		return nil, device.DeviceRefComposite, device.ErrDeviceNotFound
	}
	return dev, device.DeviceRefComposite, nil
}

// newTestInfluxDBAgentWithResolver 는 InfluxDBAgent 를 mock client + resolver 와
// 함께 생성한다. dual_tag_emit 은 default true. source keys 는 default ["device_id"].
func newTestInfluxDBAgentWithResolver(t *testing.T, mock InfluxClient, resolver DeviceResolver) *InfluxDBAgent {
	t.Helper()
	a := newTestInfluxDBAgent(mock)
	a.deviceResolver = resolver
	return a
}

// ===== 4-state 분류 시나리오 =====

func TestInfluxDBAgent_DualTag_Both(t *testing.T) {
	// device_id="lgcnp:81" + resolver 매핑 보유 → uid 부착, state=both.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81": {uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", name: "indoor-1", agentName: "lgcnp"},
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "lgcnp:81", "agent": "lgcnp"},
		"fields":      map[string]any{"value": 23.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "lgcnp:81", captured[0].Tags["device_id"], "composite tag 보존 (Soft Deprecation)")
	assert.Equal(t, "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", captured[0].Tags["uid"], "uid tag 부착")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateBoth])
}

func TestInfluxDBAgent_DualTag_CompositeOnly(t *testing.T) {
	// device_id="orphan:99" + resolver 매핑 부재 → composite 유지, uid 미부착.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{}, // 빈 매핑
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "orphan:99"},
		"fields":      map[string]any{"value": 18.0},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "orphan:99", captured[0].Tags["device_id"])
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "unmapped 시 uid 부착 금지")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateCompositeOnly])
}

func TestInfluxDBAgent_DualTag_UIDOnly_Idempotent(t *testing.T) {
	// 이미 uid tag 보유 → 변경 없음 (idempotent), state=uid_only.
	observe.ResetTSDBDualTagForTest()

	// resolver 가 lookup 호출되어도 매핑이 없도록 비워둔다 — idempotent 보장
	// 검증을 위해.
	resolver := &mockResolver{devices: map[string]*mockDevice{}}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"uid": "existing-uuid-already-set", "device_id": "lgcnp:81"},
		"fields":      map[string]any{"value": 25.0},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "existing-uuid-already-set", captured[0].Tags["uid"], "기존 uid 보존 (idempotent)")
	assert.Equal(t, "lgcnp:81", captured[0].Tags["device_id"])

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateUIDOnly])
}

func TestInfluxDBAgent_DualTag_Unmapped(t *testing.T) {
	// device_id tag 자체가 없음 → 변경 없음, state=unmapped.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{devices: map[string]*mockDevice{}}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "system_load",
		"tags":        map[string]any{"host": "server01"},
		"fields":      map[string]any{"load1": 0.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "device_id 없으면 uid 부착 없음")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateUnmapped])
}

// ===== opt-out (dual_tag_emit=false) =====

func TestInfluxDBAgent_DualTag_DisabledByConfig(t *testing.T) {
	// dual_tag_emit=false → augmentation 자체 skip, 메트릭 증가 없음.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81": {uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", name: "indoor-1", agentName: "lgcnp"},
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)
	a.influxConfig.DualTagEmit = false // 강제 OFF

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "lgcnp:81"},
		"fields":      map[string]any{"value": 23.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "dual_tag_emit=false 면 uid 부착 안 됨")

	got := observe.CollectTSDBDualTag()
	assert.Empty(t, got, "OFF 상태에서는 어떤 state 도 증가하지 않아야 한다")
}

// ===== 다중 source key =====

func TestInfluxDBAgent_DualTag_MultiSourceKeys(t *testing.T) {
	// source_keys=["device_id", "id"] + 입력에 "id" 만 존재 → "id" source 사용.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"samsung:42": {uid: "b66cb779-6852-4d4e-ae3f-8f4d9b2c3e4f", name: "outdoor", agentName: "samsung"},
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)
	a.influxConfig.DualTagEmitSourceKeys = []string{"device_id", "id"} // 우선순위 변경

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"id": "samsung:42"}, // "device_id" 부재, "id" 만 있음
		"fields":      map[string]any{"value": 30.0},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "b66cb779-6852-4d4e-ae3f-8f4d9b2c3e4f", captured[0].Tags["uid"], "두 번째 source key 의 값 사용")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateBoth])
}

func TestInfluxDBAgent_DualTag_FirstMatchingSourceKeyWins(t *testing.T) {
	// 두 source key 모두 존재 시 첫 번째 매칭 사용.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81":   {uid: "uid-from-device_id", name: "indoor-1", agentName: "lgcnp"},
			"samsung:42": {uid: "uid-from-id", name: "outdoor", agentName: "samsung"},
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)
	a.influxConfig.DualTagEmitSourceKeys = []string{"device_id", "id"}

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "lgcnp:81", "id": "samsung:42"}, // 둘 다 존재
		"fields":      map[string]any{"value": 23.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	assert.Equal(t, "uid-from-device_id", captured[0].Tags["uid"],
		"우선순위 목록의 첫 번째 매칭 (device_id) 이 사용되어야 함")
}

// ===== resolver nil graceful degradation =====

func TestInfluxDBAgent_DualTag_NilResolver_GracefulSkip(t *testing.T) {
	// resolver=nil 일 때 — 단독 테스트 환경 시뮬레이션.
	observe.ResetTSDBDualTagForTest()

	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgent(mock) // resolver 주입 없음 → nil

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "lgcnp:81"},
		"fields":      map[string]any{"value": 23.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 1)
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "resolver=nil 이면 augmentation skip")

	got := observe.CollectTSDBDualTag()
	assert.Empty(t, got, "resolver=nil 이면 메트릭도 증가하지 않음")
}

// ===== 배치 write (각 항목 독립 분류) =====

func TestInfluxDBAgent_DualTag_BatchMixedStates(t *testing.T) {
	// 한 배치에 4 종 state 가 혼재 — 각각 정확히 분류되어 메트릭에 기록.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81": {uid: "uid-1", name: "indoor-1", agentName: "lgcnp"},
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	batch := []map[string]any{
		// both: mapped device
		{"measurement": "temp", "tags": map[string]any{"device_id": "lgcnp:81"}, "fields": map[string]any{"v": 1.0}},
		// composite_only: orphan composite
		{"measurement": "temp", "tags": map[string]any{"device_id": "orphan:99"}, "fields": map[string]any{"v": 2.0}},
		// uid_only: already has uid
		{"measurement": "temp", "tags": map[string]any{"uid": "preexisting"}, "fields": map[string]any{"v": 3.0}},
		// unmapped: no device_id at all
		{"measurement": "system_load", "tags": map[string]any{"host": "srv1"}, "fields": map[string]any{"v": 0.5}},
	}
	data, _ := json.Marshal(batch)

	_, err := a.Process(data)
	require.NoError(t, err)
	require.Len(t, captured, 4)

	// 항목별 검증.
	assert.Equal(t, "uid-1", captured[0].Tags["uid"], "[0] both: uid 부착")
	_, has0 := captured[1].Tags["uid"]
	assert.False(t, has0, "[1] composite_only: uid 미부착")
	assert.Equal(t, "preexisting", captured[2].Tags["uid"], "[2] uid_only: 기존 uid 보존")
	_, has3 := captured[3].Tags["uid"]
	assert.False(t, has3, "[3] unmapped: uid 미부착")

	// 메트릭 검증 — 4 state 각 1회.
	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateBoth])
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateCompositeOnly])
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateUIDOnly])
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateUnmapped])
}

// ===== resolver 에러 — composite_only 분기 =====

func TestInfluxDBAgent_DualTag_ResolverError(t *testing.T) {
	// resolver 가 비-ErrDeviceNotFound 에러를 반환 — 안전하게 composite_only 처리.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		forceErr: errors.New("resolver internal error"),
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "any:99"},
		"fields":      map[string]any{"value": 1.0},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err, "resolver 에러는 write 전체를 실패시키지 않아야 한다 (graceful)")
	require.Len(t, captured, 1)
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "resolver 에러 시 uid 미부착")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateCompositeOnly])
}

// ===== Device.UID() 가 빈 문자열인 경우 (Phase A graceful) =====

func TestInfluxDBAgent_DualTag_EmptyUIDFromDevice(t *testing.T) {
	// Phase A graceful degradation — Device.UID() 가 "" 반환.
	// composite 만 유지, state=composite_only.
	observe.ResetTSDBDualTagForTest()

	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81": {uid: "", name: "indoor-1", agentName: "lgcnp"}, // uid 비어있음
		},
	}
	var captured []WriteData
	mock := &mockInfluxClient{
		writeFunc: func(_ context.Context, data []WriteData) error {
			captured = data
			return nil
		},
	}
	a := newTestInfluxDBAgentWithResolver(t, mock, resolver)

	input := map[string]any{
		"measurement": "temperature",
		"tags":        map[string]any{"device_id": "lgcnp:81"},
		"fields":      map[string]any{"value": 23.5},
	}
	data, _ := json.Marshal(input)

	_, err := a.Process(data)
	require.NoError(t, err)
	_, hasUID := captured[0].Tags["uid"]
	assert.False(t, hasUID, "UID() 가 빈 문자열이면 uid 부착 금지")

	got := observe.CollectTSDBDualTag()
	assert.Equal(t, float64(1), got[observe.TSDBDualTagStateCompositeOnly])
}

// ===== augmentWriteDataWithUID 단위 테스트 (helper 직접) =====

func TestAugmentWriteDataWithUID_NilWriteData(t *testing.T) {
	// nil WriteData — panic 없이 unmapped 반환.
	resolver := &mockResolver{devices: map[string]*mockDevice{}}
	state := augmentWriteDataWithUID(nil, resolver, []string{"device_id"})
	assert.Equal(t, observe.TSDBDualTagStateUnmapped, state)
}

func TestAugmentWriteDataWithUID_NilTagsMap(t *testing.T) {
	// Tags map 이 nil 인 WriteData — source key 부재 처리.
	resolver := &mockResolver{devices: map[string]*mockDevice{}}
	wd := &WriteData{Measurement: "x", Tags: nil, Fields: map[string]any{"v": 1.0}}
	state := augmentWriteDataWithUID(wd, resolver, []string{"device_id"})
	assert.Equal(t, observe.TSDBDualTagStateUnmapped, state)
	assert.Nil(t, wd.Tags, "Tags 가 변경되지 않아야 함")
}

func TestAugmentWriteDataWithUID_EmptySourceKeys(t *testing.T) {
	// 빈 source keys 슬라이스 — unmapped 반환.
	resolver := &mockResolver{devices: map[string]*mockDevice{}}
	wd := &WriteData{Tags: map[string]string{"device_id": "lgcnp:81"}}
	state := augmentWriteDataWithUID(wd, resolver, []string{})
	assert.Equal(t, observe.TSDBDualTagStateUnmapped, state)
}

func TestAugmentWriteDataWithUID_EmptyStringInSourceKeys(t *testing.T) {
	// source keys 에 빈 문자열 — 빈 문자열은 skip 후 다음 키 검사.
	resolver := &mockResolver{
		devices: map[string]*mockDevice{
			"lgcnp:81": {uid: "u1", name: "x", agentName: "lgcnp"},
		},
	}
	wd := &WriteData{Tags: map[string]string{"device_id": "lgcnp:81"}}
	state := augmentWriteDataWithUID(wd, resolver, []string{"", "device_id"})
	assert.Equal(t, observe.TSDBDualTagStateBoth, state)
	assert.Equal(t, "u1", wd.Tags["uid"])
}
