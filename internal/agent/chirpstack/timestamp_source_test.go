package chirpstack

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// timestamp_source — 메시지 타임스탬프 소스 선택 노브 (uplink 기본 / server opt-in).
//
// 이 파일의 검증 축은 두 갈래이다:
//
//	(1) 동결 축: 키 부재 / 명시적 uplink 는 오늘의 동작과 바이트 동일해야 한다
//	    (REQ-FROZEN-02, REQ-FROZEN-A). 업링크 time 이 없거나 파싱 불가일 때 레코드가
//	    0 을 싣는 것까지 포함한다 — 0 은 다운스트림에서 "타임스탬프 없음"을 뜻하는
//	    계약값이므로 여기서 수신 시각으로 폴백하면 계약이 바뀐다.
//	(2) 신규 축: server 모드는 업링크 1건에서 파생된 모든 표면(per-measurement 레코드 /
//	    combined 레코드 / 로스터 measurement 캐시)이 **동일한** 수신 시각 하나를
//	    공유해야 한다. 표면마다 time.Now() 를 따로 부르면 마이크로초가 갈라진다.

// staleUplinkTime 은 "장비 시계가 틀어진" 상황을 재현하는 고정 과거 시각이다.
// server 모드가 이 값을 무시하고 수신 시각을 쓰는지 검증하는 데 쓴다.
const staleUplinkTime = "2020-01-02T03:04:05.678+00:00"

// tsRawUplink 는 임의의 time 문자열과 measurement 를 갖는 원시 업링크 JSON 을 만든다.
// object 는 temperature/humidity 2종이라 "한 업링크의 모든 레코드가 같은 시각을
// 공유하는가" 를 실제로 관찰할 수 있다.
func tsRawUplink(t *testing.T, timeStr string) []byte {
	t.Helper()
	up := map[string]any{
		"time": timeStr,
		"deviceInfo": map[string]any{
			"devEui":            "24e124141d180806",
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
			"tags":              map[string]any{"location": "실습실"},
		},
		"object": map[string]any{"temperature": 29.8, "humidity": 55.2},
		"rxInfo": []any{map[string]any{"gatewayId": "gw-1", "rssi": -57, "snr": 13.5}},
	}
	b, err := json.Marshal(up)
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return b
}

// tsDrainMeasurements 는 수신 채널의 per-measurement 레코드를 measurement 키로 인덱싱한다.
func tsDrainMeasurements(t *testing.T, a *ChirpStackAgent) map[string]measurementRecord {
	t.Helper()
	out := map[string]measurementRecord{}
	for _, b := range drainRaw(a) {
		var m measurementRecord
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal measurementRecord: %v", err)
		}
		out[m.Measurement] = m
	}
	return out
}

// tsConfig 는 지정한 transport 옵션을 갖는 AgentConfig 를 만든다 (Configure 경로용).
func tsConfig(name string, opts map[string]any) agent.AgentConfig {
	disabled := false
	return agent.AgentConfig{
		ID:        "id-" + name,
		Name:      name,
		Type:      "chirpstack-client",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: opts},
	}
}

// ──────────────────────────────────────────────────────────────────────────
// (1) 동결 축 — 키 부재 / 명시적 uplink 는 현행 동작 그대로.
// ──────────────────────────────────────────────────────────────────────────

// TestTimestampSource_DefaultAbsentKey_UsesUplinkTime 는 timestamp_source 키가 아예
// 없을 때 실측 픽스처(testdata/packet.json)의 업링크 time 이 그대로 쓰이는지 검증한다.
func TestTimestampSource_DefaultAbsentKey_UsesUplinkTime(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-default", nil)

	if got := a.cs().TimestampSource; got != timestampSourceUplink {
		t.Fatalf("기본 timestamp_source = %q, want %q", got, timestampSourceUplink)
	}

	raw := loadRawUplink(t)
	up, err := decodeUplink(raw)
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	want := parseUplinkTimeMs(up.Time)
	if want <= 0 {
		t.Fatalf("픽스처 업링크 time 파싱 결과 = %d, want > 0", want)
	}

	a.handleUplink(raw, "application/x")

	recs := tsDrainMeasurements(t, a)
	if len(recs) == 0 {
		t.Fatal("레코드가 방출되지 않았다")
	}
	for k, r := range recs {
		if r.TimeMs != want {
			t.Errorf("%s time_ms = %d, want %d (업링크 time)", k, r.TimeMs, want)
		}
	}
	// 로스터 캐시도 같은 값이어야 한다.
	_, cached := measSample(t, measurementsOf(t, a), "magnet_status")
	if cached != want {
		t.Errorf("캐시 time_ms = %d, want %d (업링크 time)", cached, want)
	}
}

// TestTimestampSource_ExplicitUplink_IdenticalToDefault 는 uplink 를 명시해도 키
// 부재와 결과가 동일한지 검증한다(무회귀).
func TestTimestampSource_ExplicitUplink_IdenticalToDefault(t *testing.T) {
	withMemDeviceIDRepo(t)

	raw := tsRawUplink(t, staleUplinkTime)
	wantMs := parseUplinkTimeMs(staleUplinkTime)
	if wantMs <= 0 {
		t.Fatalf("고정 시각 파싱 결과 = %d, want > 0", wantMs)
	}

	for _, tc := range []struct {
		name string
		opts map[string]any
	}{
		{"키 부재", nil},
		{"명시적 uplink", map[string]any{"timestamp_source": timestampSourceUplink}},
		{"빈 문자열(미지정)", map[string]any{"timestamp_source": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newEmitModeAgent(t, "ts-explicit", tc.opts)
			if got := a.cs().TimestampSource; got != timestampSourceUplink {
				t.Fatalf("timestamp_source 스냅샷 = %q, want %q", got, timestampSourceUplink)
			}

			a.handleUplink(raw, "application/x")

			recs := tsDrainMeasurements(t, a)
			if len(recs) != 2 {
				t.Fatalf("레코드 수 = %d, want 2", len(recs))
			}
			for k, r := range recs {
				if r.TimeMs != wantMs {
					t.Errorf("%s time_ms = %d, want %d (업링크 time)", k, r.TimeMs, wantMs)
				}
			}
			_, cached := measSample(t, measurementsOf(t, a), "temperature")
			if cached != wantMs {
				t.Errorf("캐시 time_ms = %d, want %d", cached, wantMs)
			}
		})
	}
}

// TestTimestampSource_UplinkMode_MissingOrBadTime_KeepsZero 는 동결 계약의 핵심을
// 고정한다: uplink 모드에서 time 이 없거나 파싱 불가이면 레코드는 **0** 을 싣는다.
//
// 0 은 다운스트림에서 "타임스탬프 없음"을 뜻하며, 노드는 time_ms>0 일 때만
// SetTimestamp 를 호출해 메시지 생성 시각으로 폴백한다(buildChirpStackMessage).
// 여기서 수신 시각으로 폴백하면 그 계약이 바뀌므로 절대 그렇게 하지 않는다.
// 반면 로스터 캐시는 종전대로 수신 시각으로 폴백한다(1970 표시 방지).
func TestTimestampSource_UplinkMode_MissingOrBadTime_KeepsZero(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeStr string
	}{
		{"time 부재", ""},
		{"time 파싱 불가", "not-a-time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withMemDeviceIDRepo(t)
			a := newEmitModeAgent(t, "ts-zero", map[string]any{
				"timestamp_source": timestampSourceUplink,
			})

			before := time.Now().UnixMilli()
			a.handleUplink(tsRawUplink(t, tc.timeStr), "application/x")
			after := time.Now().UnixMilli()

			recs := tsDrainMeasurements(t, a)
			if len(recs) != 2 {
				t.Fatalf("레코드 수 = %d, want 2", len(recs))
			}
			for k, r := range recs {
				if r.TimeMs != 0 {
					t.Errorf("%s time_ms = %d, want 0 (동결 계약: 타임스탬프 없음)", k, r.TimeMs)
				}
			}
			// 캐시는 수신 시각 폴백(기존 동작 유지).
			_, cached := measSample(t, measurementsOf(t, a), "temperature")
			if cached < before || cached > after {
				t.Errorf("캐시 time_ms = %d, want 수신 시각 [%d, %d]", cached, before, after)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// (2) 신규 축 — server 모드는 업링크 1건당 하나의 수신 시각을 모든 표면이 공유.
// ──────────────────────────────────────────────────────────────────────────

// TestTimestampSource_Server_AllRecordsShareOneReceiptTime 는 server 모드에서
// per-measurement 레코드 전부와 로스터 캐시가 **같은** 수신 시각을 갖는지 검증한다.
// 업링크 time 은 과거 고정값이므로, 그 값이 쓰였다면 즉시 드러난다.
func TestTimestampSource_Server_AllRecordsShareOneReceiptTime(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-server", map[string]any{
		"timestamp_source": timestampSourceServer,
	})

	if got := a.cs().TimestampSource; got != timestampSourceServer {
		t.Fatalf("timestamp_source 스냅샷 = %q, want %q", got, timestampSourceServer)
	}

	uplinkMs := parseUplinkTimeMs(staleUplinkTime)

	before := time.Now().UnixMilli()
	a.handleUplink(tsRawUplink(t, staleUplinkTime), "application/x")
	after := time.Now().UnixMilli()

	recs := tsDrainMeasurements(t, a)
	if len(recs) != 2 {
		t.Fatalf("레코드 수 = %d, want 2", len(recs))
	}

	var shared int64
	for k, r := range recs {
		if r.TimeMs == uplinkMs {
			t.Errorf("%s time_ms = %d — 업링크 time 이 쓰였다(server 모드여야 함)", k, r.TimeMs)
		}
		if r.TimeMs < before || r.TimeMs > after {
			t.Errorf("%s time_ms = %d, want 수신 시각 [%d, %d]", k, r.TimeMs, before, after)
		}
		if shared == 0 {
			shared = r.TimeMs
		} else if r.TimeMs != shared {
			t.Errorf("%s time_ms = %d, want %d — 한 업링크의 레코드는 동일 시각을 공유해야 한다",
				k, r.TimeMs, shared)
		}
	}

	// 로스터 캐시도 정확히 같은 값이어야 한다(표면 간 불일치 금지).
	m := measurementsOf(t, a)
	for _, key := range []string{"temperature", "humidity"} {
		_, cached := measSample(t, m, key)
		if cached != shared {
			t.Errorf("캐시 measurements[%s].time_ms = %d, want %d (flow 레코드와 동일해야 함)",
				key, cached, shared)
		}
	}
}

// TestTimestampSource_Server_CombinedRecordAgreesWithCache 는 combined 방출 모드에서도
// 레코드와 캐시가 같은 수신 시각을 갖는지 검증한다(두 노브의 조합 경로).
func TestTimestampSource_Server_CombinedRecordAgreesWithCache(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-server-combined", map[string]any{
		"timestamp_source":      timestampSourceServer,
		"measurement_emit_mode": measurementEmitModeCombined,
	})

	before := time.Now().UnixMilli()
	a.handleUplink(tsRawUplink(t, staleUplinkTime), "application/x")
	after := time.Now().UnixMilli()

	raws := drainRaw(a)
	if len(raws) != 1 {
		t.Fatalf("레코드 수 = %d, want 1 (combined)", len(raws))
	}
	var rec combinedMeasurementRecord
	if err := json.Unmarshal(raws[0], &rec); err != nil {
		t.Fatalf("unmarshal combinedMeasurementRecord: %v", err)
	}
	if rec.TimeMs < before || rec.TimeMs > after {
		t.Errorf("combined time_ms = %d, want 수신 시각 [%d, %d]", rec.TimeMs, before, after)
	}

	_, cached := measSample(t, measurementsOf(t, a), "temperature")
	if cached != rec.TimeMs {
		t.Errorf("캐시 time_ms = %d, want %d (combined 레코드와 동일해야 함)", cached, rec.TimeMs)
	}
}

// TestTimestampSource_Server_MissingUplinkTimeStillReceiptTime 는 업링크 time 이
// 아예 없어도 server 모드는 항상 양수 수신 시각을 낸다는 것을 고정한다
// (uplink 모드의 0 계약과 대비되는 지점).
func TestTimestampSource_Server_MissingUplinkTimeStillReceiptTime(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-server-notime", map[string]any{
		"timestamp_source": timestampSourceServer,
	})

	before := time.Now().UnixMilli()
	a.handleUplink(tsRawUplink(t, ""), "application/x")
	after := time.Now().UnixMilli()

	for k, r := range tsDrainMeasurements(t, a) {
		if r.TimeMs < before || r.TimeMs > after {
			t.Errorf("%s time_ms = %d, want 수신 시각 [%d, %d]", k, r.TimeMs, before, after)
		}
	}
}

// TestResolveUplinkTimeMs 는 확정 함수 자체의 분기를 단위 수준에서 고정한다.
func TestResolveUplinkTimeMs(t *testing.T) {
	receivedAt := time.Date(2026, 8, 14, 1, 2, 3, 456_000_000, time.UTC)
	withTime := &uplink{Time: staleUplinkTime}
	noTime := &uplink{Time: ""}

	if got := resolveUplinkTimeMs(withTime, timestampSourceUplink, receivedAt); got != parseUplinkTimeMs(staleUplinkTime) {
		t.Errorf("uplink 모드 = %d, want %d", got, parseUplinkTimeMs(staleUplinkTime))
	}
	if got := resolveUplinkTimeMs(noTime, timestampSourceUplink, receivedAt); got != 0 {
		t.Errorf("uplink 모드 + time 부재 = %d, want 0", got)
	}
	if got := resolveUplinkTimeMs(withTime, timestampSourceServer, receivedAt); got != receivedAt.UnixMilli() {
		t.Errorf("server 모드 = %d, want %d", got, receivedAt.UnixMilli())
	}
	if got := resolveUplinkTimeMs(noTime, timestampSourceServer, receivedAt); got != receivedAt.UnixMilli() {
		t.Errorf("server 모드 + time 부재 = %d, want %d", got, receivedAt.UnixMilli())
	}
	// 미지정(빈 문자열)은 기본 경로(uplink)로 해석한다.
	if got := resolveUplinkTimeMs(withTime, "", receivedAt); got != parseUplinkTimeMs(staleUplinkTime) {
		t.Errorf("미지정 = %d, want 업링크 time 파생값", got)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 검증 규율 — 무효값은 조용히 폴백하지 않고 거부한다.
// ──────────────────────────────────────────────────────────────────────────

// TestTimestampSource_InvalidRejectedAtCreation 는 생성 경로에서 무효값이 에러로
// 거부되는지 검증한다(조용한 uplink 폴백 금지).
func TestTimestampSource_InvalidRejectedAtCreation(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
	}{
		{"오타", "sever"},
		{"대문자", "SERVER"},
		{"미지원 값", "gateway"},
		{"문자열 아님", 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetNameRegistryForTest()
			disabled := false
			_, err := NewChirpStackAgent(agent.AgentConfig{
				ID:      "id-ts-invalid",
				Name:    "ts-invalid",
				Type:    "chirpstack-client",
				Enabled: &disabled,
				Transport: agent.TransportConfig{
					Options: map[string]any{"timestamp_source": tc.val},
				},
			})
			if err == nil {
				t.Fatal("무효 timestamp_source 가 수락되었다 — 거부되어야 한다")
			}
			if !errors.Is(err, ErrInvalidTimestampSource) {
				t.Errorf("err = %v, want ErrInvalidTimestampSource 계열", err)
			}
		})
	}
}

// TestTimestampSource_InvalidRejectedAtConfigure 는 런타임 재설정에서도 무효값이
// 거부되고, 거부 시 이전 스냅샷이 그대로 유지되는지 검증한다.
func TestTimestampSource_InvalidRejectedAtConfigure(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-cfg-invalid", map[string]any{
		"timestamp_source": timestampSourceServer,
	})

	err := a.Configure(tsConfig("ts-cfg-invalid", map[string]any{
		"timestamp_source": "sever",
	}))
	if err == nil {
		t.Fatal("무효 timestamp_source 재설정이 수락되었다 — 거부되어야 한다")
	}
	if !errors.Is(err, ErrInvalidTimestampSource) {
		t.Errorf("err = %v, want ErrInvalidTimestampSource 계열", err)
	}
	if got := a.cs().TimestampSource; got != timestampSourceServer {
		t.Errorf("거부 후 스냅샷 = %q, want %q (이전 설정 유지)", got, timestampSourceServer)
	}
}

// TestTimestampSource_ConfigureSwitchesSnapshot 는 Configure 로 모드를 바꾸면 cs()
// 스냅샷에 반영되고, 이후 업링크가 즉시 새 모드로 처리되는지 검증한다.
func TestTimestampSource_ConfigureSwitchesSnapshot(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-cfg-switch", nil)

	if got := a.cs().TimestampSource; got != timestampSourceUplink {
		t.Fatalf("초기 스냅샷 = %q, want %q", got, timestampSourceUplink)
	}

	// uplink → server.
	if err := a.Configure(tsConfig("ts-cfg-switch", map[string]any{
		"timestamp_source": timestampSourceServer,
	})); err != nil {
		t.Fatalf("Configure(server): %v", err)
	}
	if got := a.cs().TimestampSource; got != timestampSourceServer {
		t.Fatalf("전환 후 스냅샷 = %q, want %q", got, timestampSourceServer)
	}

	before := time.Now().UnixMilli()
	a.handleUplink(tsRawUplink(t, staleUplinkTime), "application/x")
	after := time.Now().UnixMilli()
	for k, r := range tsDrainMeasurements(t, a) {
		if r.TimeMs < before || r.TimeMs > after {
			t.Errorf("전환 후 %s time_ms = %d, want 수신 시각 [%d, %d]", k, r.TimeMs, before, after)
		}
	}

	// server → uplink 로 되돌리면 다시 업링크 time 을 쓴다.
	if err := a.Configure(tsConfig("ts-cfg-switch", map[string]any{
		"timestamp_source": timestampSourceUplink,
	})); err != nil {
		t.Fatalf("Configure(uplink): %v", err)
	}
	if got := a.cs().TimestampSource; got != timestampSourceUplink {
		t.Fatalf("복귀 후 스냅샷 = %q, want %q", got, timestampSourceUplink)
	}

	wantMs := parseUplinkTimeMs(staleUplinkTime)
	a.handleUplink(tsRawUplink(t, staleUplinkTime), "application/x")
	for k, r := range tsDrainMeasurements(t, a) {
		if r.TimeMs != wantMs {
			t.Errorf("복귀 후 %s time_ms = %d, want %d (업링크 time)", k, r.TimeMs, wantMs)
		}
	}
}

// TestTimestampSource_ConfigurePreservesOtherKnobs 는 timestamp_source 추가가 기존
// 노브 파싱을 건드리지 않았는지 확인한다(회귀 방지).
func TestTimestampSource_ConfigurePreservesOtherKnobs(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "ts-knobs", map[string]any{
		"timestamp_source":      timestampSourceServer,
		"measurement_emit_mode": measurementEmitModeCombined,
		"emit_comm_state":       true,
	})

	cc := a.cs()
	if cc.TimestampSource != timestampSourceServer {
		t.Errorf("TimestampSource = %q", cc.TimestampSource)
	}
	if cc.MeasurementEmitMode != measurementEmitModeCombined {
		t.Errorf("MeasurementEmitMode = %q", cc.MeasurementEmitMode)
	}
	if !cc.EmitCommState {
		t.Error("EmitCommState = false, want true")
	}
}
