package chirpstack

import (
	"encoding/json"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// radioUplink 는 게이트웨이 3대가 동시에 수신한 업링크를 만든다.
//
// rxInfo 배열은 **일부러 gatewayId 오름차순이 아니다** — ChirpStack 의 중복 제거가
// Redis Set 기반이라 배열 순서에 보장이 없기 때문이며, 정렬 검증이 의미를 가지려면
// 입력이 정렬되어 있으면 안 된다.
//
// 세 번째 게이트웨이는 channel 을 명시하지 않아 0 이다 — proto3 가 기본값 필드를
// 생략하므로 실제 배포에서 정상적으로 도착하는 형태이며, 0 이 "미상" 이 아니라
// 정당한 채널 0 으로 보존되는지 검증하는 데 쓰인다.
func radioUplink() *uplink {
	return &uplink{
		Time: "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{
			DevEui: "24e124141d180806",
			Tags:   map[string]string{"location": "실습실"},
		},
		Object: map[string]any{
			"temperature": 29.8,
			"humidity":    55.2,
		},
		RxInfo: []uplinkRxInfo{
			{GatewayID: "gw-c", RSSI: -95, SNR: 2.5, Channel: 7},
			{GatewayID: "gw-a", RSSI: -57, SNR: 13.5, Channel: 3},
			{GatewayID: "gw-b", RSSI: -72, SNR: 8.0}, // channel 생략 → 0.
		},
	}
}

// marshalUplink 는 업링크를 원시 JSON 으로 만든다(handleUplink 입력).
func marshalUplink(t *testing.T, up *uplink) []byte {
	t.Helper()
	raw, err := json.Marshal(up)
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return raw
}

// ──────────────────────────────────────────────────────────────────────────
// 회귀 가드: 기본 설정(emit_radio=false, emit_report=false)의 출력은 **바이트 동일**.
// ──────────────────────────────────────────────────────────────────────────

// frozenMeasurementShape 는 radio 필드가 추가되기 **이전**의 measurementRecord
// 정의를 문자 그대로 재현한 것이다 (필드 순서 포함).
//
// Go 의 encoding/json 은 구조체 필드를 선언 순서로 직렬화하므로, 이 구조체를
// marshal 한 바이트열과 에이전트가 실제로 방출한 바이트열이 같다면 "기본 설정의
// 출력이 오늘과 바이트 동일하다" 가 기계적으로 증명된다 — 키 유무뿐 아니라
// 키 순서와 표현까지 포함한 검증이다.
type frozenMeasurementShape struct {
	Measurement string            `json:"measurement"`
	Value       any               `json:"value"`
	UnitID      string            `json:"unit_id"`
	TimeMs      int64             `json:"time_ms"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// frozenCombinedShape 는 radio 필드 추가 이전의 combinedMeasurementRecord 정의이다.
type frozenCombinedShape struct {
	Record string            `json:"record"`
	Values map[string]any    `json:"values"`
	UnitID string            `json:"unit_id"`
	TimeMs int64             `json:"time_ms"`
	Tags   map[string]string `json:"tags,omitempty"`
}

// TestDefaultConfig_EventBytesUnchanged 는 두 신규 토글이 모두 꺼진 기본 설정에서
// per-measurement event 레코드가 **변경 이전과 바이트 동일**함을 검증한다.
//
// 이것이 동결 계약(REQ-FROZEN-01/02/04, REQ-FROZEN-A)의 회귀 가드이다.
func TestDefaultConfig_EventBytesUnchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-frozen-event", nil)

	if a.cs().EmitRadio {
		t.Fatal("emit_radio 기본값이 true 이다 — 기본은 false 여야 한다")
	}
	if a.cs().EmitReport {
		t.Fatal("emit_report 기본값이 true 이다 — 기본은 false 여야 한다")
	}

	up := radioUplink()
	a.handleUplink(marshalUplink(t, up), "application/x")

	recs := drainRaw(a)
	if len(recs) != 2 {
		t.Fatalf("레코드 수 = %d, want 2", len(recs))
	}

	timeMs := parseUplinkTimeMs(up.Time)
	// 방출 순서는 measurement 키 오름차순이다(buildMeasurementRecords).
	want := []frozenMeasurementShape{
		{Measurement: "humidity", Value: 55.2, UnitID: "24e124141d180806", TimeMs: timeMs, Tags: up.DeviceInfo.Tags},
		{Measurement: "temperature", Value: 29.8, UnitID: "24e124141d180806", TimeMs: timeMs, Tags: up.DeviceInfo.Tags},
	}
	for i := range want {
		wantJSON, err := json.Marshal(want[i])
		if err != nil {
			t.Fatalf("marshal 기대 shape: %v", err)
		}
		if string(recs[i]) != string(wantJSON) {
			t.Errorf("레코드[%d] 바이트 불일치\n got: %s\nwant: %s", i, recs[i], wantJSON)
		}
	}
}

// TestDefaultConfig_CombinedBytesUnchanged 는 combined 모드에서도 기본 설정 출력이
// 변경 이전과 바이트 동일함을 검증한다.
func TestDefaultConfig_CombinedBytesUnchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-frozen-combined", map[string]any{
		"measurement_emit_mode": measurementEmitModeCombined,
	})

	up := radioUplink()
	a.handleUplink(marshalUplink(t, up), "application/x")

	recs := drainRaw(a)
	if len(recs) != 1 {
		t.Fatalf("레코드 수 = %d, want 1", len(recs))
	}

	wantJSON, err := json.Marshal(frozenCombinedShape{
		Record: recordKindMeasurements,
		Values: map[string]any{"temperature": 29.8, "humidity": 55.2},
		UnitID: "24e124141d180806",
		TimeMs: parseUplinkTimeMs(up.Time),
		Tags:   up.DeviceInfo.Tags,
	})
	if err != nil {
		t.Fatalf("marshal 기대 shape: %v", err)
	}
	if string(recs[0]) != string(wantJSON) {
		t.Errorf("combined 레코드 바이트 불일치\n got: %s\nwant: %s", recs[0], wantJSON)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// emit_radio opt-in 경로.
// ──────────────────────────────────────────────────────────────────────────

// TestEmitRadio_AllGatewaysSortedAndCounted 는 emit_radio=true 에서 게이트웨이
// **전량**이 gateway_id 오름차순으로 실리고 count 가 정확한지 검증한다.
//
// 각 게이트웨이가 자기 rssi/snr/channel 을 유지하는지도 함께 본다 — 최적 1개로
// 접히거나 값이 섞이면 여기서 잡힌다.
func TestEmitRadio_AllGatewaysSortedAndCounted(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-on", map[string]any{"emit_radio": true})

	a.handleUplink(marshalUplink(t, radioUplink()), "application/x")

	recs := drainRaw(a)
	if len(recs) != 2 {
		t.Fatalf("레코드 수 = %d, want 2", len(recs))
	}

	for i, b := range recs {
		var m measurementRecord
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal[%d]: %v", i, err)
		}
		if m.Radio == nil {
			t.Fatalf("레코드[%d] radio 그룹이 없다", i)
		}
		if m.Radio.Count != 3 {
			t.Errorf("레코드[%d] radio.count = %d, want 3", i, m.Radio.Count)
		}
		if len(m.Radio.Gateways) != 3 {
			t.Fatalf("레코드[%d] gateways 개수 = %d, want 3", i, len(m.Radio.Gateways))
		}
		// gateway_id 오름차순(입력 rxInfo 는 c,a,b 순이었다).
		wantOrder := []string{"gw-a", "gw-b", "gw-c"}
		for j, want := range wantOrder {
			if got := m.Radio.Gateways[j].GatewayID; got != want {
				t.Errorf("레코드[%d] gateways[%d].gateway_id = %q, want %q", i, j, got, want)
			}
		}
		// 게이트웨이별 값이 각자 유지되는지.
		byID := map[string]radioGatewayView{}
		for _, g := range m.Radio.Gateways {
			byID[g.GatewayID] = g
		}
		if g := byID["gw-a"]; g.RSSI != -57 || g.SNR != 13.5 || g.Channel != 3 {
			t.Errorf("gw-a = %+v, want rssi=-57 snr=13.5 channel=3", g)
		}
		if g := byID["gw-c"]; g.RSSI != -95 || g.SNR != 2.5 || g.Channel != 7 {
			t.Errorf("gw-c = %+v, want rssi=-95 snr=2.5 channel=7", g)
		}
		// channel 생략(proto3 zero-value) → 0 으로 보존. "미상" 으로 바뀌지 않는다.
		if g := byID["gw-b"]; g.RSSI != -72 || g.SNR != 8.0 || g.Channel != 0 {
			t.Errorf("gw-b = %+v, want rssi=-72 snr=8.0 channel=0(보존)", g)
		}
	}
}

// TestEmitRadio_DeterministicAcrossRxInfoPermutation 는 rxInfo 배열 순서를 뒤집어도
// radio 그룹이 **바이트 동일**한지 검증한다 (테스트 flakiness / diff 소음 방지).
func TestEmitRadio_DeterministicAcrossRxInfoPermutation(t *testing.T) {
	up1 := radioUplink()
	up2 := radioUplink()
	// 순서 뒤집기.
	up2.RxInfo[0], up2.RxInfo[2] = up2.RxInfo[2], up2.RxInfo[0]

	r1 := buildRadioGroup(up1, 1)
	r2 := buildRadioGroup(up2, 1)

	b1, err := json.Marshal(r1)
	if err != nil {
		t.Fatalf("marshal r1: %v", err)
	}
	b2, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("marshal r2: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("rxInfo 순열에 따라 radio 그룹이 달라졌다\n a: %s\n b: %s", b1, b2)
	}
}

// TestEmitRadio_NoRxInfoOmitsGroup 는 rxInfo 가 없으면 radio 키 자체가 생략되는지
// 검증한다 (빈 배열 금지 — "게이트웨이 0대" 와 "rxInfo 없음" 을 혼동시키지 않는다).
func TestEmitRadio_NoRxInfoOmitsGroup(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-norx", map[string]any{"emit_radio": true})

	up := radioUplink()
	up.RxInfo = nil
	a.handleUplink(marshalUplink(t, up), "application/x")

	recs := drainRaw(a)
	if len(recs) == 0 {
		t.Fatal("레코드가 없다")
	}
	for i, b := range recs {
		var raw map[string]any
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatalf("unmarshal[%d]: %v", i, err)
		}
		if _, present := raw["radio"]; present {
			t.Errorf("레코드[%d] 에 radio 키가 있다 — rxInfo 부재 시 생략되어야 한다: %s", i, b)
		}
	}

	// gatewayId 가 전부 비어 있는 rxInfo 도 동일하다(키잉 불가 → 익명 링크 제외).
	if g := buildRadioGroup(&uplink{RxInfo: []uplinkRxInfo{{RSSI: -50}}}, 1); g != nil {
		t.Errorf("gatewayId 없는 rxInfo 에서 radio = %+v, want nil", g)
	}
}

// TestEmitRadio_CombinedKeepsValuesSeparate 는 combined 모드에서 radio 가 values 와
// 분리되어 있고, **"rssi" 라는 이름의 센서가 무선 품질과 충돌하지 않는지** 검증한다.
func TestEmitRadio_CombinedKeepsValuesSeparate(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-combined", map[string]any{
		"emit_radio":            true,
		"measurement_emit_mode": measurementEmitModeCombined,
	})

	up := radioUplink()
	// 센서 이름이 문자 그대로 "rssi" 인 병리적(그러나 가능한) 케이스.
	up.Object["rssi"] = 12.34

	a.handleUplink(marshalUplink(t, up), "application/x")

	recs := drainRaw(a)
	if len(recs) != 1 {
		t.Fatalf("레코드 수 = %d, want 1", len(recs))
	}

	var c combinedMeasurementRecord
	if err := json.Unmarshal(recs[0], &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// 센서 값은 values 안에 그대로 살아 있다.
	if c.Values["rssi"] != 12.34 {
		t.Errorf("values.rssi = %v, want 12.34 (센서 값이 무선 품질에 덮이면 안 된다)", c.Values["rssi"])
	}
	if c.Values["temperature"] != 29.8 {
		t.Errorf("values.temperature = %v, want 29.8", c.Values["temperature"])
	}
	// 무선 품질은 values 밖의 별도 필드이다.
	if c.Radio == nil || c.Radio.Count != 3 {
		t.Fatalf("radio = %+v, want count=3", c.Radio)
	}
	if c.Radio.Gateways[0].RSSI != -57 {
		t.Errorf("radio.gateways[0].rssi = %d, want -57", c.Radio.Gateways[0].RSSI)
	}
	if _, leaked := c.Values["radio"]; leaked {
		t.Error("values 안으로 radio 가 새어 들어갔다")
	}
}

// TestEmitRadio_ConfigureRuntimeToggle 는 Configure 로 emit_radio 를 켜고 끄는 것이
// 다음 업링크부터 즉시 반영되는지 검증한다 (per-uplink 게이트 규약).
func TestEmitRadio_ConfigureRuntimeToggle(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "radio-toggle", nil)
	raw := marshalUplink(t, radioUplink())

	a.handleUplink(raw, "application/x")
	for _, b := range drainRaw(a) {
		var m measurementRecord
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m.Radio != nil {
			t.Fatal("기본 설정인데 radio 가 실렸다")
		}
	}

	disabled := false
	if err := a.Configure(agent.AgentConfig{
		ID: "id-radio-toggle", Name: "radio-toggle", Type: "chirpstack", Enabled: &disabled,
		Transport: agent.TransportConfig{Options: map[string]any{"emit_radio": true}},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	a.handleUplink(raw, "application/x")
	recs := drainRaw(a)
	if len(recs) == 0 {
		t.Fatal("레코드가 없다")
	}
	var m measurementRecord
	if err := json.Unmarshal(recs[0], &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Radio == nil {
		t.Error("Configure 로 켠 뒤에도 radio 가 실리지 않았다")
	}
}

// TestEmitRadio_ConfigTypeRejected 는 emit_radio 가 불리언이 아닐 때 Configure 가
// 거부하는지 검증한다 (기존 불리언 노브와 동일 규율).
func TestEmitRadio_ConfigTypeRejected(t *testing.T) {
	if err := validateChirpStackOptions(map[string]any{"emit_radio": "yes"}); err == nil {
		t.Error("emit_radio 문자열을 거부하지 않았다")
	}
}
