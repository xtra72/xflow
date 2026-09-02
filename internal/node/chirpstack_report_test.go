package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/pkg/message"
)

// payloadMap 은 payload 의 중첩 맵 값을 꺼낸다.
func payloadMap(t *testing.T, raw any, label string) map[string]any {
	t.Helper()
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s 타입 = %T, want map[string]any", label, raw)
	}
	return m
}

// ──────────────────────────────────────────────────────────────────────────
// measurement_report 분기.
// ──────────────────────────────────────────────────────────────────────────

// TestBuildChirpStackMessage_MeasurementReportPerMeasurement 는 per_measurement
// 리포트 레코드가 measurement.report 메시지로 빌드되는지 검증한다.
//
// device_state.report 와 **다른 타입**이라는 점이 핵심이다 — 두 메시지가 같은
// 타입으로 접히면 다운스트림이 스냅샷 재방출과 구간 집계를 구분할 수 없다.
func TestBuildChirpStackMessage_MeasurementReportPerMeasurement(t *testing.T) {
	endTS := time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)
	startMs := endTS.UnixMilli() - 60_000

	rec := map[string]any{
		"record":      "measurement_report",
		"measurement": "temperature",
		"stats": map[string]any{
			"min": 10.0, "max": 30.0, "avg": 20.0, "count": 3,
		},
		"radio": []any{
			map[string]any{
				"gateway_id": "gw-a",
				"rssi":       map[string]any{"min": -60.0, "max": -50.0, "avg": -55.0, "count": 2},
				"snr":        map[string]any{"min": 5.0, "max": 7.0, "avg": 6.0, "count": 2},
			},
		},
		"uplinks":         3,
		"gateways":        1,
		"unit_id":         "24e124141d180806",
		"time_ms":         endTS.UnixMilli(),
		"window_start_ms": startMs,
		"window_end_ms":   endTS.UnixMilli(),
		"tags":            map[string]string{"location": "실습실"},
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackMessage returned ok=false")
	}

	if msg.Type() != "measurement.report" {
		t.Errorf("type = %q, want measurement.report", msg.Type())
	}
	if !msg.Timestamp().Equal(endTS) {
		t.Errorf("timestamp = %v, want %v (윈도 종료)", msg.Timestamp().UTC(), endTS)
	}
	if m, _ := msg.Metadata().Get("measurement"); m != "temperature" {
		t.Errorf("metadata.measurement = %q, want temperature", m)
	}
	tags, ok := msg.Metadata().GetGroup("tags")
	if !ok || tags["location"] != "실습실" {
		t.Errorf("metadata.tags = %v, ok=%v", tags, ok)
	}

	// per_measurement 통계는 payload 최상위 flat 키이다.
	for k, want := range map[string]any{"min": 10.0, "max": 30.0, "avg": 20.0} {
		if v, _ := msg.Payload().Get(k); v != want {
			t.Errorf("payload.%s = %v, want %v", k, v, want)
		}
	}
	if v, _ := msg.Payload().Get("count"); v != int64(3) {
		t.Errorf("payload.count = %v(%T), want int64(3)", v, v)
	}
	if v, _ := msg.Payload().Get("uplinks"); v != int64(3) {
		t.Errorf("payload.uplinks = %v, want 3", v)
	}
	if v, _ := msg.Payload().Get("gateways"); v != 1 {
		t.Errorf("payload.gateways = %v, want 1", v)
	}
	if v, _ := msg.Payload().Get("window_start_ms"); v != startMs {
		t.Errorf("payload.window_start_ms = %v, want %d", v, startMs)
	}
	if v, _ := msg.Payload().Get("window_end_ms"); v != endTS.UnixMilli() {
		t.Errorf("payload.window_end_ms = %v, want %d", v, endTS.UnixMilli())
	}
	// combined 전용 키는 없어야 한다.
	if _, present := msg.Payload().Get("measurements"); present {
		t.Error("per_measurement 리포트에 payload.measurements 가 있다")
	}
	// unit_id 는 device 그룹 승격 과정에서 payload 에서 제거된다 (event 경로와 동일).
	if _, present := msg.Payload().Get("unit_id"); present {
		t.Error("payload.unit_id 가 제거되지 않았다")
	}

	// 게이트웨이별 집계.
	radioRaw, ok := msg.Payload().Get("radio")
	if !ok {
		t.Fatal("payload.radio 가 없다")
	}
	radio, ok := radioRaw.([]any)
	if !ok {
		t.Fatalf("payload.radio 타입 = %T, want []any", radioRaw)
	}
	if len(radio) != 1 {
		t.Fatalf("radio 항목 수 = %d, want 1", len(radio))
	}
	g := payloadMap(t, radio[0], "radio[0]")
	if g["gateway_id"] != "gw-a" {
		t.Errorf("radio[0].gateway_id = %v, want gw-a", g["gateway_id"])
	}
	rssi := payloadMap(t, g["rssi"], "radio[0].rssi")
	if rssi["min"] != -60.0 || rssi["max"] != -50.0 || rssi["avg"] != -55.0 || rssi["count"] != int64(2) {
		t.Errorf("radio[0].rssi = %v", rssi)
	}
	snr := payloadMap(t, g["snr"], "radio[0].snr")
	if snr["avg"] != 6.0 {
		t.Errorf("radio[0].snr.avg = %v, want 6.0", snr["avg"])
	}
}

// TestBuildChirpStackMessage_MeasurementReportCombined 는 combined 리포트가
// measurements 아래로 네임스페이스되는지 검증한다.
//
// 네임스페이스가 필요한 이유는 리포트에 uplinks/gateways/window_* 라는 고정 키가
// 함께 있기 때문이다 — flat 으로 폈다면 그 이름의 센서가 서로 덮어쓴다.
func TestBuildChirpStackMessage_MeasurementReportCombined(t *testing.T) {
	rec := map[string]any{
		"record": "measurement_report",
		"measurements": map[string]any{
			"temperature": map[string]any{"min": 10.0, "max": 30.0, "avg": 20.0, "count": 2},
			// 비숫자 스칼라: count 만 있고 min/max/avg 는 없다.
			"magnet_status": map[string]any{"count": 5},
			// 고정 키와 같은 이름의 센서 — 네임스페이스 덕분에 충돌하지 않는다.
			"uplinks": map[string]any{"min": 1.0, "max": 1.0, "avg": 1.0, "count": 1},
		},
		"uplinks":         7,
		"gateways":        2,
		"unit_id":         "eui",
		"time_ms":         int64(1_700_000_000_000),
		"window_start_ms": int64(1_699_999_940_000),
		"window_end_ms":   int64(1_700_000_000_000),
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("ok=false")
	}
	if msg.Type() != "measurement.report" {
		t.Errorf("type = %q, want measurement.report", msg.Type())
	}
	// combined 은 measurement 를 특정하지 않는다.
	if _, present := msg.Metadata().Get("measurement"); present {
		t.Error("combined 리포트에 metadata.measurement 가 있다")
	}
	// per_measurement 전용 flat 키는 없어야 한다.
	for _, k := range []string{"min", "max", "avg", "count"} {
		if _, present := msg.Payload().Get(k); present {
			t.Errorf("combined 리포트에 payload.%s 가 있다", k)
		}
	}

	raw, ok := msg.Payload().Get("measurements")
	if !ok {
		t.Fatal("payload.measurements 가 없다")
	}
	values := payloadMap(t, raw, "payload.measurements")

	temp := payloadMap(t, values["temperature"], "measurements.temperature")
	if temp["min"] != 10.0 || temp["max"] != 30.0 || temp["avg"] != 20.0 || temp["count"] != int64(2) {
		t.Errorf("measurements.temperature = %v", temp)
	}

	magnet := payloadMap(t, values["magnet_status"], "measurements.magnet_status")
	if magnet["count"] != int64(5) {
		t.Errorf("measurements.magnet_status.count = %v, want 5", magnet["count"])
	}
	for _, k := range []string{"min", "max", "avg"} {
		if _, present := magnet[k]; present {
			t.Errorf("비숫자 measurement 에 %s 가 지어내어졌다: %v", k, magnet)
		}
	}

	// 고정 키와 이름이 같은 센서는 measurements 안에 그대로 살아 있고,
	// payload 최상위 uplinks 는 리포트의 업링크 수를 유지한다.
	sensor := payloadMap(t, values["uplinks"], "measurements.uplinks")
	if sensor["avg"] != 1.0 {
		t.Errorf("measurements.uplinks.avg = %v, want 1.0", sensor["avg"])
	}
	if v, _ := msg.Payload().Get("uplinks"); v != int64(7) {
		t.Errorf("payload.uplinks = %v, want 7 (센서 이름 충돌 없음)", v)
	}
}

// TestBuildChirpStackMessage_MeasurementReportBadJSON 는 잘못된 JSON 이 ok=false 로
// 안전하게 처리되는지 검증한다.
func TestBuildChirpStackMessage_MeasurementReportBadJSON(t *testing.T) {
	data := []byte(`{"record":"measurement_report","uplinks":"not-a-number"}`)
	if _, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions()); ok {
		t.Error("타입이 어긋난 리포트 레코드는 ok=false 여야 한다")
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 무선 품질(radio) — event 경로.
// ──────────────────────────────────────────────────────────────────────────

// TestBuildChirpStackMessage_EventRadio 는 event 레코드의 radio 그룹이
// payload.radio 로 옮겨지고 순서/값이 보존되는지 검증한다.
func TestBuildChirpStackMessage_EventRadio(t *testing.T) {
	rec := map[string]any{
		"measurement": "temperature",
		"value":       29.8,
		"unit_id":     "eui",
		"time_ms":     int64(1_700_000_000_000),
		"radio": map[string]any{
			"gateways": []any{
				map[string]any{"gateway_id": "gw-a", "rssi": -57, "snr": 13.5, "channel": 3},
				map[string]any{"gateway_id": "gw-b", "rssi": -72, "snr": 8.0, "channel": 0},
			},
			"count": 2,
		},
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("ok=false")
	}
	// 동결 계약은 그대로다.
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	if v, _ := msg.Payload().Get("value"); v != 29.8 {
		t.Errorf("payload.value = %v, want 29.8", v)
	}

	radio := mustPayloadKey(t, msg, "radio")
	if radio["count"] != 2 {
		t.Errorf("radio.count = %v, want 2", radio["count"])
	}
	gws, ok := radio["gateways"].([]any)
	if !ok {
		t.Fatalf("radio.gateways 타입 = %T, want []any", radio["gateways"])
	}
	if len(gws) != 2 {
		t.Fatalf("radio.gateways 개수 = %d, want 2", len(gws))
	}
	// 에이전트가 정렬한 순서를 노드가 보존한다.
	first := payloadMap(t, gws[0], "radio.gateways[0]")
	if first["gateway_id"] != "gw-a" || first["rssi"] != -57 || first["snr"] != 13.5 || first["channel"] != uint32(3) {
		t.Errorf("radio.gateways[0] = %v", first)
	}
	// channel 0 은 0 으로 보존된다 — 생략되거나 "미상" 으로 바뀌지 않는다.
	second := payloadMap(t, gws[1], "radio.gateways[1]")
	ch, present := second["channel"]
	if !present {
		t.Error("channel 0 이 생략되었다 — 정당한 채널 0 은 보존되어야 한다")
	}
	if ch != uint32(0) {
		t.Errorf("radio.gateways[1].channel = %v, want 0", ch)
	}
}

// TestBuildChirpStackMessage_CombinedRadioNoCollision 는 combined event 에서
// "rssi" 라는 이름의 센서가 무선 품질과 충돌하지 않는지 검증한다.
//
// payload 최상위 rssi 는 센서 값이고, 무선 품질은 payload.radio 아래에만 있다.
func TestBuildChirpStackMessage_CombinedRadioNoCollision(t *testing.T) {
	rec := map[string]any{
		"record": "measurements",
		"values": map[string]any{
			"temperature": 29.8,
			"rssi":        12.34, // 문자 그대로 "rssi" 라는 이름의 센서.
		},
		"unit_id": "eui",
		"time_ms": int64(1_700_000_000_000),
		"radio": map[string]any{
			"gateways": []any{
				map[string]any{"gateway_id": "gw-a", "rssi": -57, "snr": 13.5, "channel": 3},
			},
			"count": 1,
		},
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("ok=false")
	}
	// payload 는 여전히 flat measurement 이름이다.
	if v, _ := msg.Payload().Get("temperature"); v != 29.8 {
		t.Errorf("payload.temperature = %v, want 29.8", v)
	}
	// 센서 rssi 가 무선 품질에 덮이지 않았다.
	if v, _ := msg.Payload().Get("rssi"); v != 12.34 {
		t.Errorf("payload.rssi = %v, want 12.34 (센서 값)", v)
	}
	// 무선 품질은 별도 네임스페이스에 있다.
	radio := mustPayloadKey(t, msg, "radio")
	gws := radio["gateways"].([]any)
	g := payloadMap(t, gws[0], "radio.gateways[0]")
	if g["rssi"] != -57 {
		t.Errorf("radio.gateways[0].rssi = %v, want -57", g["rssi"])
	}
}

// TestBuildChirpStackMessage_NoRadioKeyWhenAbsent 는 radio 가 없는 레코드(기본 설정)
// 에서 payload.radio 키가 아예 만들어지지 않는지 검증한다.
func TestBuildChirpStackMessage_NoRadioKeyWhenAbsent(t *testing.T) {
	rec := map[string]any{
		"measurement": "temperature", "value": 1.0, "unit_id": "eui", "time_ms": int64(1),
	}
	data, _ := json.Marshal(rec)
	msg, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("ok=false")
	}
	if _, present := msg.Payload().Get("radio"); present {
		t.Error("radio 없는 레코드에 payload.radio 키가 생겼다")
	}

	// 빈 gateways 배열도 키를 만들지 않는다.
	rec["radio"] = map[string]any{"gateways": []any{}, "count": 0}
	data, _ = json.Marshal(rec)
	msg, ok = buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("ok=false")
	}
	if _, present := msg.Payload().Get("radio"); present {
		t.Error("빈 gateways 에 payload.radio 키가 생겼다")
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 판별자 라우팅: 기존 세 분기가 교란되지 않는다.
// ──────────────────────────────────────────────────────────────────────────

// TestBuildChirpStackMessage_DiscriminatorRouting 는 네 개의 판별자가 각각 올바른
// 메시지 타입으로 라우팅되는지 검증한다.
//
// measurement_report 추가가 기존 세 분기(빈 문자열=event / measurements /
// device_state)를 건드리지 않았음을 한 표에서 확인한다.
func TestBuildChirpStackMessage_DiscriminatorRouting(t *testing.T) {
	cases := []struct {
		name     string
		rec      map[string]any
		wantType string
	}{
		{
			name:     "판별자 없음 → event",
			rec:      map[string]any{"measurement": "t", "value": 1.0, "unit_id": "e", "time_ms": int64(1)},
			wantType: "event",
		},
		{
			name:     "measurements → event(combined)",
			rec:      map[string]any{"record": "measurements", "values": map[string]any{"t": 1.0}, "unit_id": "e", "time_ms": int64(1)},
			wantType: "event",
		},
		{
			name:     "device_state → device_state.change",
			rec:      map[string]any{"record": "device_state", "trigger": "change", "unit_id": "e", "time_ms": int64(1), "state": map[string]any{"online": true}},
			wantType: "device_state.change",
		},
		{
			name:     "measurement_report → measurement.report",
			rec:      map[string]any{"record": "measurement_report", "measurement": "t", "stats": map[string]any{"count": 1}, "unit_id": "e", "time_ms": int64(1)},
			wantType: "measurement.report",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(tc.rec)
			msg, ok := buildChirpStackMessage(data, "n", nil, "", DefaultEmitOptions())
			if !ok {
				t.Fatal("ok=false")
			}
			if msg.Type() != tc.wantType {
				t.Errorf("type = %q, want %q", msg.Type(), tc.wantType)
			}
		})
	}
}

// mustPayloadKey 는 payload 키 조회가 성공했음을 단언하고 중첩 맵을 반환한다.
//
// 이름이 mustGet 이 아닌 이유는 같은 패키지의 modbus 테스트가 이미 다른 시그니처의
// mustGet 을 갖고 있기 때문이다(기존 테스트는 건드리지 않는다).
func mustPayloadKey(t *testing.T, msg message.Message, key string) map[string]any {
	t.Helper()
	v, ok := msg.Payload().Get(key)
	if !ok {
		t.Fatalf("payload.%s 가 없다", key)
	}
	return payloadMap(t, v, "payload."+key)
}
