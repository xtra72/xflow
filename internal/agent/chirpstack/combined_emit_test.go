package chirpstack

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// multiMeasurementUplink 는 사용자 시나리오(temperature+humidity 동시 업링크)를 재현하는
// 합성 업링크이다. 실측 픽스처(testdata/packet.json, WS301 도어센서)는 measurement 가
// 1개뿐이라 fan-out 대비 combined 의 차이를 드러내지 못한다.
func multiMeasurementUplink() *uplink {
	return &uplink{
		Time: "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{
			DevEui: "24e124141d180806",
			Tags:   map[string]string{"location": "실습실", "point": "앞문"},
		},
		Object: map[string]any{
			"temperature": 29.8,
			"humidity":    55.2,
		},
	}
}

// newEmitModeAgent 는 임의의 transport 옵션을 가진 비활성화(브로커 미연결) 에이전트를
// 만든다. 비활성화이므로 Init 이 연결/watchdog 을 기동하지 않으며, 테스트가
// handleUplink 를 직접 호출한다.
func newEmitModeAgent(t *testing.T, name string, opts map[string]any) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()
	disabled := false
	raw, err := NewChirpStackAgent(agent.AgentConfig{
		ID:        "id-" + name,
		Name:      name,
		Type:      "chirpstack-client",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: opts},
	})
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	return raw.(*ChirpStackAgent)
}

// drainRaw 는 수신 채널에 쌓인 원시 레코드를 비블로킹으로 모두 꺼낸다.
func drainRaw(a *ChirpStackAgent) [][]byte {
	var out [][]byte
	for {
		select {
		case b := <-a.recvCh:
			out = append(out, b)
		default:
			return out
		}
	}
}

// recordKindOf 는 레코드 JSON 의 판별자(record) 값을 반환한다.
func recordKindOf(t *testing.T, b []byte) string {
	t.Helper()
	var disc struct {
		Record string `json:"record"`
	}
	if err := json.Unmarshal(b, &disc); err != nil {
		t.Fatalf("판별자 unmarshal: %v", err)
	}
	return disc.Record
}

// ──────────────────────────────────────────────────────────────────────────
// 특성 테스트(characterization): 기본 경로는 동결되어 있다 (REQ-FROZEN-01/02).
// ──────────────────────────────────────────────────────────────────────────

// TestEmitMode_DefaultAbsentKey_PerMeasurementFanout 는 measurement_emit_mode 키가
// 아예 없을 때 오늘의 per-measurement fan-out 이 그대로 유지되는지 검증한다.
func TestEmitMode_DefaultAbsentKey_PerMeasurementFanout(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "emit-default", nil)

	if got := a.cs().MeasurementEmitMode; got != measurementEmitModePerMeasurement {
		t.Fatalf("기본 모드 = %q, want %q", got, measurementEmitModePerMeasurement)
	}

	raw, err := json.Marshal(multiMeasurementUplink())
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	a.handleUplink(raw, "application/x")

	recs := drainRaw(a)
	if len(recs) != 2 {
		t.Fatalf("레코드 수 = %d, want 2 (measurement 당 1개)", len(recs))
	}
	byM := map[string]measurementRecord{}
	for _, b := range recs {
		if kind := recordKindOf(t, b); kind != "" {
			t.Errorf("기본 경로 레코드의 record 판별자 = %q, want 빈 문자열", kind)
		}
		var m measurementRecord
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal measurementRecord: %v", err)
		}
		byM[m.Measurement] = m
	}
	if byM["temperature"].Value != 29.8 {
		t.Errorf("temperature value = %v, want 29.8", byM["temperature"].Value)
	}
	if byM["humidity"].Value != 55.2 {
		t.Errorf("humidity value = %v, want 55.2", byM["humidity"].Value)
	}
	for k, m := range byM {
		if m.UnitID != "24e124141d180806" {
			t.Errorf("%s unit_id = %q", k, m.UnitID)
		}
		if m.TimeMs <= 0 {
			t.Errorf("%s time_ms = %d, want > 0", k, m.TimeMs)
		}
		if m.Tags["location"] != "실습실" {
			t.Errorf("%s tags = %v", k, m.Tags)
		}
	}
}

// TestEmitMode_ExplicitPerMeasurement_FanoutUnchanged 는 per_measurement 를 명시해도
// 키 부재와 동일한 fan-out 이 나오는지 검증한다.
func TestEmitMode_ExplicitPerMeasurement_FanoutUnchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "emit-explicit", map[string]any{
		"measurement_emit_mode": measurementEmitModePerMeasurement,
	})

	raw, err := json.Marshal(multiMeasurementUplink())
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	a.handleUplink(raw, "application/x")

	if recs := drainRaw(a); len(recs) != 2 {
		t.Fatalf("레코드 수 = %d, want 2", len(recs))
	}
}

// TestEmitMode_RealFixture_PerMeasurementUnchanged 는 실측 픽스처(WS301 업링크)에서
// 기본 경로가 오늘과 동일하게 1개 레코드를 방출하는지 검증한다.
func TestEmitMode_RealFixture_PerMeasurementUnchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "emit-fixture", nil)

	a.handleUplink(loadRawUplink(t), "application/x")

	recs := drainRaw(a)
	if len(recs) != 1 {
		t.Fatalf("레코드 수 = %d, want 1", len(recs))
	}
	var m measurementRecord
	if err := json.Unmarshal(recs[0], &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Measurement != "magnet_status" || m.Value != "close" {
		t.Errorf("record = %+v, want magnet_status=close", m)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// combined 경로 (opt-in).
// ──────────────────────────────────────────────────────────────────────────

// TestEmitMode_Combined_SingleRecordAllMeasurements 는 measurement 2개 업링크가
// combined 모드에서 정확히 1개 레코드로 접히는지, 그리고 top-level 필드가
// per-measurement 경로와 동일한지 검증한다.
func TestEmitMode_Combined_SingleRecordAllMeasurements(t *testing.T) {
	withMemDeviceIDRepo(t)
	raw, err := json.Marshal(multiMeasurementUplink())
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}

	// 동일 업링크를 기본 경로로도 흘려 top-level 필드 동일성 비교 기준을 만든다.
	base := newEmitModeAgent(t, "emit-cmp-base", nil)
	base.handleUplink(raw, "application/x")
	baseRecs := drainRaw(base)
	if len(baseRecs) != 2 {
		t.Fatalf("기준 경로 레코드 수 = %d, want 2", len(baseRecs))
	}
	var baseFirst measurementRecord
	if err := json.Unmarshal(baseRecs[0], &baseFirst); err != nil {
		t.Fatalf("unmarshal 기준 레코드: %v", err)
	}

	a := newEmitModeAgent(t, "emit-combined", map[string]any{
		"measurement_emit_mode": measurementEmitModeCombined,
	})
	a.handleUplink(raw, "application/x")

	recs := drainRaw(a)
	if len(recs) != 1 {
		t.Fatalf("레코드 수 = %d, want 1 (combined)", len(recs))
	}
	if kind := recordKindOf(t, recs[0]); kind != recordKindMeasurements {
		t.Errorf("record 판별자 = %q, want %q", kind, recordKindMeasurements)
	}

	var c combinedMeasurementRecord
	if err := json.Unmarshal(recs[0], &c); err != nil {
		t.Fatalf("unmarshal combinedMeasurementRecord: %v", err)
	}
	if len(c.Values) != 2 {
		t.Fatalf("values 개수 = %d, want 2 (%v)", len(c.Values), c.Values)
	}
	if c.Values["temperature"] != 29.8 {
		t.Errorf("values.temperature = %v, want 29.8", c.Values["temperature"])
	}
	if c.Values["humidity"] != 55.2 {
		t.Errorf("values.humidity = %v, want 55.2", c.Values["humidity"])
	}

	// top-level 필드는 per-measurement 경로와 동일해야 한다.
	if c.UnitID != baseFirst.UnitID {
		t.Errorf("unit_id = %q, want %q (per-measurement 와 동일)", c.UnitID, baseFirst.UnitID)
	}
	if c.TimeMs != baseFirst.TimeMs {
		t.Errorf("time_ms = %d, want %d (per-measurement 와 동일)", c.TimeMs, baseFirst.TimeMs)
	}
	if c.Tags["location"] != "실습실" || c.Tags["point"] != "앞문" {
		t.Errorf("tags verbatim 불일치: %v", c.Tags)
	}
}

// TestBuildCombinedMeasurementRecord_NonScalarSkip 는 combined 모드에서도 비스칼라
// 값이 skip 되고 스칼라 형제만 담기는지 검증한다 (per-measurement 와 동일 규칙).
func TestBuildCombinedMeasurementRecord_NonScalarSkip(t *testing.T) {
	up := &uplink{
		Time:       "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{DevEui: "eui"},
		Object: map[string]any{
			"sensors":     map[string]any{"a": float64(1)},
			"arr":         []any{float64(1), float64(2)},
			"temperature": 21.0,
		},
	}

	rec, ok := buildCombinedMeasurementRecord(up, parseUplinkTimeMs(up.Time), silentLogger())
	if !ok {
		t.Fatal("ok = false, want true (스칼라 형제 존재)")
	}
	if len(rec.Values) != 1 {
		t.Fatalf("values 개수 = %d, want 1 (%v)", len(rec.Values), rec.Values)
	}
	if rec.Values["temperature"] != 21.0 {
		t.Errorf("values.temperature = %v, want 21.0", rec.Values["temperature"])
	}
}

// TestEmitMode_Combined_ZeroScalarEmitsNothing 는 스칼라 measurement 가 없는 업링크가
// combined 모드에서 아무것도 방출하지 않는지(빈 payload 메시지 금지) 검증한다.
func TestEmitMode_Combined_ZeroScalarEmitsNothing(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "emit-zero", map[string]any{
		"measurement_emit_mode": measurementEmitModeCombined,
	})

	raw, err := json.Marshal(&uplink{
		Time:       "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{DevEui: "eui-zero"},
		Object:     map[string]any{"sensors": map[string]any{"a": float64(1)}},
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	a.handleUplink(raw, "application/x")

	if recs := drainRaw(a); len(recs) != 0 {
		t.Fatalf("레코드 수 = %d, want 0 (스칼라 없음)", len(recs))
	}

	// object 자체가 비어 있는 경우도 동일하다.
	if _, ok := buildCombinedMeasurementRecord(&uplink{DeviceInfo: uplinkDeviceInfo{DevEui: "e"}}, 0, silentLogger()); ok {
		t.Error("빈 object 는 ok=false 여야 한다")
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 설정 검증 / 런타임 재설정.
// ──────────────────────────────────────────────────────────────────────────

// TestMeasurementEmitMode_InvalidValueRejected 는 무효값이 조용히 폴백되지 않고
// 에러로 거부되는지 검증한다 (생성 경로 + Configure 경로 + 파서 단위).
func TestMeasurementEmitMode_InvalidValueRejected(t *testing.T) {
	badOpts := map[string]any{"measurement_emit_mode": "combined_x"}

	if err := validateMeasurementEmitMode(badOpts); !errors.Is(err, ErrInvalidMeasurementEmitMode) {
		t.Errorf("validateMeasurementEmitMode err = %v, want ErrInvalidMeasurementEmitMode", err)
	}
	if _, err := parseChirpStackConfigStrict(agent.AgentConfig{
		Transport: agent.TransportConfig{Options: badOpts},
	}); !errors.Is(err, ErrInvalidMeasurementEmitMode) {
		t.Errorf("strict 파싱 err = %v, want ErrInvalidMeasurementEmitMode", err)
	}

	// 생성 경로도 거부한다 (조용한 per_measurement 폴백 금지).
	resetNameRegistryForTest()
	disabled := false
	_, err := NewChirpStackAgent(agent.AgentConfig{
		ID:        "id-bad",
		Name:      "emit-bad",
		Type:      "chirpstack-client",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: badOpts},
	})
	if !errors.Is(err, ErrInvalidMeasurementEmitMode) {
		t.Errorf("NewChirpStackAgent err = %v, want ErrInvalidMeasurementEmitMode", err)
	}

	// 문자열이 아닌 타입도 거부한다.
	if err := validateMeasurementEmitMode(map[string]any{"measurement_emit_mode": 3}); !errors.Is(err, ErrInvalidMeasurementEmitMode) {
		t.Errorf("숫자 값 err = %v, want ErrInvalidMeasurementEmitMode", err)
	}

	// 빈 문자열은 "미지정"으로 보아 허용하고 기본값을 유지한다.
	if err := validateMeasurementEmitMode(map[string]any{"measurement_emit_mode": ""}); err != nil {
		t.Errorf("빈 문자열 err = %v, want nil", err)
	}
	cc := parseChirpStackConfig(agent.AgentConfig{
		Transport: agent.TransportConfig{Options: map[string]any{"measurement_emit_mode": ""}},
	})
	if cc.MeasurementEmitMode != measurementEmitModePerMeasurement {
		t.Errorf("빈 문자열 모드 = %q, want %q", cc.MeasurementEmitMode, measurementEmitModePerMeasurement)
	}
}

// TestMeasurementEmitMode_ConfigureRuntimeSwitch 는 Configure 로 모드를 전환하면
// 스냅샷과 실제 방출 동작이 즉시 바뀌는지 검증한다.
func TestMeasurementEmitMode_ConfigureRuntimeSwitch(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "emit-switch", nil)

	raw, err := json.Marshal(multiMeasurementUplink())
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}

	a.handleUplink(raw, "application/x")
	if recs := drainRaw(a); len(recs) != 2 {
		t.Fatalf("전환 전 레코드 수 = %d, want 2", len(recs))
	}

	disabled := false
	if err := a.Configure(agent.AgentConfig{
		ID:      "id-emit-switch",
		Name:    "emit-switch",
		Type:    "chirpstack-client",
		Enabled: &disabled,
		Transport: agent.TransportConfig{Options: map[string]any{
			"measurement_emit_mode": measurementEmitModeCombined,
		}},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := a.cs().MeasurementEmitMode; got != measurementEmitModeCombined {
		t.Fatalf("Configure 후 스냅샷 모드 = %q, want %q", got, measurementEmitModeCombined)
	}

	a.handleUplink(raw, "application/x")
	recs := drainRaw(a)
	if len(recs) != 1 {
		t.Fatalf("전환 후 레코드 수 = %d, want 1", len(recs))
	}
	if kind := recordKindOf(t, recs[0]); kind != recordKindMeasurements {
		t.Errorf("전환 후 record = %q, want %q", kind, recordKindMeasurements)
	}

	// combined → per_measurement 역방향 전환도 즉시 반영된다.
	if err := a.Configure(agent.AgentConfig{
		ID:      "id-emit-switch",
		Name:    "emit-switch",
		Type:    "chirpstack-client",
		Enabled: &disabled,
		Transport: agent.TransportConfig{Options: map[string]any{
			"measurement_emit_mode": measurementEmitModePerMeasurement,
		}},
	}); err != nil {
		t.Fatalf("Configure(역방향): %v", err)
	}
	a.handleUplink(raw, "application/x")
	if recs := drainRaw(a); len(recs) != 2 {
		t.Fatalf("역방향 전환 후 레코드 수 = %d, want 2", len(recs))
	}
}

// TestMeasurementEmitMode_ConfigureInvalidKeepsPrevious 는 Configure 가 무효값을
// 거부하면서 이전 설정을 그대로 유지하는지 검증한다(절반만 갱신 금지).
func TestMeasurementEmitMode_ConfigureInvalidKeepsPrevious(t *testing.T) {
	a := newEmitModeAgent(t, "emit-keep", map[string]any{
		"measurement_emit_mode": measurementEmitModeCombined,
	})
	if got := a.cs().MeasurementEmitMode; got != measurementEmitModeCombined {
		t.Fatalf("초기 모드 = %q, want %q", got, measurementEmitModeCombined)
	}

	disabled := false
	err := a.Configure(agent.AgentConfig{
		ID:      "id-emit-keep",
		Name:    "emit-keep",
		Type:    "chirpstack-client",
		Enabled: &disabled,
		Transport: agent.TransportConfig{Options: map[string]any{
			"measurement_emit_mode": "nope",
		}},
	})
	if !errors.Is(err, ErrInvalidMeasurementEmitMode) {
		t.Fatalf("Configure err = %v, want ErrInvalidMeasurementEmitMode", err)
	}
	if got := a.cs().MeasurementEmitMode; got != measurementEmitModeCombined {
		t.Errorf("거부 후 모드 = %q, want %q (이전 설정 유지)", got, measurementEmitModeCombined)
	}
}
