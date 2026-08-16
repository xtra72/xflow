package chirpstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// reportUplink 는 집계 검증용 합성 업링크를 만든다.
func reportUplink(devEui string, object map[string]any, rx ...uplinkRxInfo) *uplink {
	return &uplink{
		Time: "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{
			DevEui: devEui,
			Tags:   map[string]string{"location": "실습실"},
		},
		Object: object,
		RxInfo: rx,
	}
}

// feedUplink 는 업링크 1건을 수신 경로로 흘린다.
func feedUplink(t *testing.T, a *ChirpStackAgent, up *uplink) {
	t.Helper()
	a.handleUplink(marshalUplink(t, up), "application/x")
}

// drainReports 는 수신 채널에서 measurement_report 레코드만 골라 파싱한다.
//
// emit_report 를 켠 에이전트도 event 레코드를 함께 방출하므로, 판별자로 걸러야
// 리포트만 볼 수 있다.
func drainReports(t *testing.T, a *ChirpStackAgent) []measurementReportRecord {
	t.Helper()
	var out []measurementReportRecord
	for _, b := range drainRaw(a) {
		if recordKindOf(t, b) != recordKindMeasurementReport {
			continue
		}
		var rec measurementReportRecord
		if err := json.Unmarshal(b, &rec); err != nil {
			t.Fatalf("unmarshal measurementReportRecord: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

// reportsByMeasurement 는 per_measurement 리포트를 measurement 키로 인덱싱한다.
func reportsByMeasurement(recs []measurementReportRecord) map[string]measurementReportRecord {
	m := make(map[string]measurementReportRecord, len(recs))
	for _, r := range recs {
		m[r.Measurement] = r
	}
	return m
}

// wantAgg 는 집계 뷰가 기대한 min/max/avg/count 인지 확인한다.
func wantAgg(t *testing.T, label string, got aggView, min, max, avg float64, count int64) {
	t.Helper()
	if got.Count != count {
		t.Errorf("%s.count = %d, want %d", label, got.Count, count)
	}
	if got.Min == nil || got.Max == nil || got.Avg == nil {
		t.Fatalf("%s 의 숫자 통계가 없다: %+v", label, got)
	}
	if *got.Min != min {
		t.Errorf("%s.min = %v, want %v", label, *got.Min, min)
	}
	if *got.Max != max {
		t.Errorf("%s.max = %v, want %v", label, *got.Max, max)
	}
	if *got.Avg != avg {
		t.Errorf("%s.avg = %v, want %v", label, *got.Avg, avg)
	}
}

// newReportAgent 는 emit_report 를 켠 비활성화 에이전트를 만든다.
//
// 비활성화이므로 tick goroutine 이 기동되지 않는다 — 테스트는
// emitMeasurementReports 를 직접 호출해 윈도 마감 시점을 결정적으로 통제한다.
func newReportAgent(t *testing.T, name string, extra map[string]any) *ChirpStackAgent {
	t.Helper()
	opts := map[string]any{"emit_report": true, "report_interval": 60}
	for k, v := range extra {
		opts[k] = v
	}
	return newEmitModeAgent(t, name, opts)
}

// ──────────────────────────────────────────────────────────────────────────
// 집계 정확성.
// ──────────────────────────────────────────────────────────────────────────

// TestReport_AggregatesOverWindow 는 다중 업링크 윈도의 min/max/avg/count 와
// 게이트웨이별 rssi/snr 집계, uplinks/gateways 카운트를 검증한다.
func TestReport_AggregatesOverWindow(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-agg", nil)

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 10.0},
		uplinkRxInfo{GatewayID: "gw-a", RSSI: -50, SNR: 5}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 20.0},
		uplinkRxInfo{GatewayID: "gw-a", RSSI: -60, SNR: 7},
		uplinkRxInfo{GatewayID: "gw-b", RSSI: -80, SNR: 1}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 30.0},
		uplinkRxInfo{GatewayID: "gw-b", RSSI: -70, SNR: 3}))

	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 1 {
		t.Fatalf("리포트 수 = %d, want 1 (measurement 1종)", len(recs))
	}
	rec := recs[0]

	if rec.Record != recordKindMeasurementReport {
		t.Errorf("record = %q, want %q", rec.Record, recordKindMeasurementReport)
	}
	if rec.Measurement != "temperature" {
		t.Errorf("measurement = %q, want temperature", rec.Measurement)
	}
	if rec.Stats == nil {
		t.Fatal("stats 가 없다")
	}
	wantAgg(t, "temperature", *rec.Stats, 10, 30, 20, 3)

	if rec.Uplinks != 3 {
		t.Errorf("uplinks = %d, want 3", rec.Uplinks)
	}
	if rec.Gateways != 2 {
		t.Errorf("gateways = %d, want 2", rec.Gateways)
	}
	if rec.UnitID != "dev-1" {
		t.Errorf("unit_id = %q, want dev-1", rec.UnitID)
	}
	if rec.Tags["location"] != "실습실" {
		t.Errorf("tags = %v", rec.Tags)
	}

	// 게이트웨이별 rssi/snr 이 서로 섞이지 않고 각자 집계된다.
	if len(rec.Radio) != 2 {
		t.Fatalf("radio 항목 수 = %d, want 2", len(rec.Radio))
	}
	if rec.Radio[0].GatewayID != "gw-a" || rec.Radio[1].GatewayID != "gw-b" {
		t.Fatalf("radio 정렬 = %q,%q, want gw-a,gw-b", rec.Radio[0].GatewayID, rec.Radio[1].GatewayID)
	}
	wantAgg(t, "gw-a.rssi", rec.Radio[0].RSSI, -60, -50, -55, 2)
	wantAgg(t, "gw-a.snr", rec.Radio[0].SNR, 5, 7, 6, 2)
	wantAgg(t, "gw-b.rssi", rec.Radio[1].RSSI, -80, -70, -75, 2)
	wantAgg(t, "gw-b.snr", rec.Radio[1].SNR, 1, 3, 2, 2)

	// 윈도 경계.
	if rec.WindowEndMs != rec.TimeMs {
		t.Errorf("time_ms = %d, window_end_ms = %d — 리포트 시각은 윈도 종료여야 한다", rec.TimeMs, rec.WindowEndMs)
	}
	if rec.WindowStartMs <= 0 || rec.WindowStartMs > rec.WindowEndMs {
		t.Errorf("window [%d, %d] 가 유효하지 않다", rec.WindowStartMs, rec.WindowEndMs)
	}
}

// TestReport_NonNumericScalarCountedWithoutStats 는 문자열/불리언 measurement 가
// count 는 세어지되 min/max/avg 는 **생략**되는지 검증한다.
//
// 0 으로 강제 변환하거나 통째로 버리는 두 실패 양상을 모두 배제한다.
func TestReport_NonNumericScalarCountedWithoutStats(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-nonnumeric", nil)

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"magnet_status": "close", "temperature": 5.0}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"magnet_status": "open", "temperature": 7.0}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"alarm": true}))

	a.emitMeasurementReports()

	byM := reportsByMeasurement(drainReports(t, a))

	magnet, ok := byM["magnet_status"]
	if !ok {
		t.Fatal("magnet_status 리포트가 없다 — 비숫자 스칼라도 count 는 보고되어야 한다")
	}
	if magnet.Stats.Count != 2 {
		t.Errorf("magnet_status.count = %d, want 2", magnet.Stats.Count)
	}
	if magnet.Stats.Min != nil || magnet.Stats.Max != nil || magnet.Stats.Avg != nil {
		t.Errorf("문자열 measurement 에 숫자 통계가 실렸다: %+v", *magnet.Stats)
	}

	alarm, ok := byM["alarm"]
	if !ok {
		t.Fatal("alarm(불리언) 리포트가 없다")
	}
	if alarm.Stats.Avg != nil {
		t.Error("불리언이 숫자로 접혔다 (true=1 강제 변환 금지)")
	}

	temp := byM["temperature"]
	wantAgg(t, "temperature", *temp.Stats, 5, 7, 6, 2)
}

// TestReport_NonScalarSkipped 는 비스칼라(중첩 객체/배열)가 event 경로와 동일하게
// 집계에서도 제외되는지 검증한다.
func TestReport_NonScalarSkipped(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-nonscalar", nil)

	feedUplink(t, a, reportUplink("dev-1", map[string]any{
		"sensors":     map[string]any{"a": 1.0},
		"arr":         []any{1.0, 2.0},
		"temperature": 21.0,
	}))
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 1 || recs[0].Measurement != "temperature" {
		t.Fatalf("리포트 = %+v, want temperature 1건만", recs)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 윈도 수명: 빈 윈도 / 리셋.
// ──────────────────────────────────────────────────────────────────────────

// TestReport_EmptyWindowEmitsNothing 는 업링크가 없던 구간이 **아무것도 방출하지
// 않는지** 검증한다 (count==0 리포트 금지, avg 의 0 나눗셈 경로 부재).
func TestReport_EmptyWindowEmitsNothing(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-empty", nil)

	// 업링크 0건 상태에서 마감.
	a.emitMeasurementReports()
	if recs := drainReports(t, a); len(recs) != 0 {
		t.Fatalf("업링크 없는 첫 윈도에서 리포트 %d건 방출됨, want 0", len(recs))
	}

	// 업링크 1건 → 마감 → 방출.
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 1.0}))
	a.emitMeasurementReports()
	if recs := drainReports(t, a); len(recs) != 1 {
		t.Fatalf("리포트 수 = %d, want 1", len(recs))
	}

	// 다시 업링크 없이 마감 → 아무것도 방출하지 않는다(직전 값 재방출 금지).
	a.emitMeasurementReports()
	if recs := drainReports(t, a); len(recs) != 0 {
		t.Fatalf("빈 윈도에서 리포트 %d건 방출됨, want 0", len(recs))
	}
}

// TestReport_WindowResetsBetweenIntervals 는 두 번째 윈도가 첫 윈도의 값을
// 누적해 들고 가지 않는지 검증한다.
func TestReport_WindowResetsBetweenIntervals(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-reset", nil)

	// 윈도 1: 10, 20 → min 10 / max 20 / avg 15 / count 2.
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 10.0}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 20.0}))
	a.emitMeasurementReports()

	first := drainReports(t, a)
	if len(first) != 1 {
		t.Fatalf("윈도1 리포트 수 = %d, want 1", len(first))
	}
	wantAgg(t, "윈도1 temperature", *first[0].Stats, 10, 20, 15, 2)

	// 윈도 2: 100 하나뿐 → min/max/avg 모두 100, count 1.
	// 누적이 이월되면 min 이 10 으로 남거나 count 가 3 이 된다.
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 100.0}))
	a.emitMeasurementReports()

	second := drainReports(t, a)
	if len(second) != 1 {
		t.Fatalf("윈도2 리포트 수 = %d, want 1", len(second))
	}
	wantAgg(t, "윈도2 temperature", *second[0].Stats, 100, 100, 100, 1)
	if second[0].Uplinks != 1 {
		t.Errorf("윈도2 uplinks = %d, want 1 (누적 이월 금지)", second[0].Uplinks)
	}

	// 윈도 경계가 이어진다: 윈도2 시작 == 윈도1 종료.
	if second[0].WindowStartMs != first[0].WindowEndMs {
		t.Errorf("윈도2 시작(%d) != 윈도1 종료(%d) — 구간이 이어져야 한다",
			second[0].WindowStartMs, first[0].WindowEndMs)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// report_emit_mode.
// ──────────────────────────────────────────────────────────────────────────

// TestReport_PerMeasurementMode 는 기본 모드가 measurement 당 1개 리포트로
// fan-out 하는지 검증한다.
func TestReport_PerMeasurementMode(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-per", nil)

	if got := a.cs().ReportEmitMode; got != reportEmitModePerMeasurement {
		t.Fatalf("기본 report_emit_mode = %q, want %q", got, reportEmitModePerMeasurement)
	}

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 10.0, "humidity": 50.0}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 30.0, "humidity": 70.0}))
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 2 {
		t.Fatalf("리포트 수 = %d, want 2 (measurement 당 1개)", len(recs))
	}
	// measurement 이름 오름차순으로 방출된다(결정적 순서).
	if recs[0].Measurement != "humidity" || recs[1].Measurement != "temperature" {
		t.Errorf("방출 순서 = %q,%q, want humidity,temperature", recs[0].Measurement, recs[1].Measurement)
	}
	for _, r := range recs {
		if len(r.Measurements) != 0 {
			t.Errorf("per_measurement 리포트에 measurements 맵이 실렸다: %v", r.Measurements)
		}
		if r.Stats == nil {
			t.Errorf("%s: stats 가 없다", r.Measurement)
		}
	}
	byM := reportsByMeasurement(recs)
	wantAgg(t, "temperature", *byM["temperature"].Stats, 10, 30, 20, 2)
	wantAgg(t, "humidity", *byM["humidity"].Stats, 50, 70, 60, 2)
}

// TestReport_CombinedMode 는 combined 모드가 디바이스당 1개 리포트로 접히고
// 모든 measurement 를 함께 담는지 검증한다.
func TestReport_CombinedMode(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-combined", map[string]any{
		"report_emit_mode": reportEmitModeCombined,
	})

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 10.0, "humidity": 50.0}))
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 30.0, "humidity": 70.0}))
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 1 {
		t.Fatalf("리포트 수 = %d, want 1 (combined)", len(recs))
	}
	rec := recs[0]
	if rec.Measurement != "" || rec.Stats != nil {
		t.Errorf("combined 리포트에 per_measurement 필드가 실렸다: measurement=%q stats=%+v", rec.Measurement, rec.Stats)
	}
	if len(rec.Measurements) != 2 {
		t.Fatalf("measurements 개수 = %d, want 2", len(rec.Measurements))
	}
	wantAgg(t, "temperature", rec.Measurements["temperature"], 10, 30, 20, 2)
	wantAgg(t, "humidity", rec.Measurements["humidity"], 50, 70, 60, 2)
	if rec.Uplinks != 2 {
		t.Errorf("uplinks = %d, want 2", rec.Uplinks)
	}
}

// TestReport_EmitModeIndependentOfMeasurementEmitMode 는 report_emit_mode 가
// measurement_emit_mode 와 **독립된 축**임을 검증한다 (event 는 combined, report 는
// per_measurement).
func TestReport_EmitModeIndependentOfMeasurementEmitMode(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-independent", map[string]any{
		"measurement_emit_mode": measurementEmitModeCombined,
		"report_emit_mode":      reportEmitModePerMeasurement,
	})

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 10.0, "humidity": 50.0}))

	// event 는 combined 1건.
	var events int
	for _, b := range drainRaw(a) {
		if recordKindOf(t, b) == recordKindMeasurements {
			events++
		}
	}
	if events != 1 {
		t.Errorf("combined event 수 = %d, want 1", events)
	}

	// report 는 per_measurement 2건.
	a.emitMeasurementReports()
	if recs := drainReports(t, a); len(recs) != 2 {
		t.Errorf("리포트 수 = %d, want 2 (report_emit_mode 는 독립)", len(recs))
	}
}

// TestReport_MultiDeviceDeterministicOrder 는 여러 디바이스의 리포트가 devEui
// 오름차순으로 방출되는지 검증한다(맵 순회 무작위성 제거).
func TestReport_MultiDeviceDeterministicOrder(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-multidev", nil)

	for _, dev := range []string{"dev-c", "dev-a", "dev-b"} {
		feedUplink(t, a, reportUplink(dev, map[string]any{"temperature": 1.0}))
	}
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 3 {
		t.Fatalf("리포트 수 = %d, want 3", len(recs))
	}
	for i, want := range []string{"dev-a", "dev-b", "dev-c"} {
		if recs[i].UnitID != want {
			t.Errorf("리포트[%d].unit_id = %q, want %q", i, recs[i].UnitID, want)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 상한(cap) 강제.
// ──────────────────────────────────────────────────────────────────────────

// TestReport_MeasurementCapEnforced 는 디바이스당 measurement 상한을 넘는 신규 키가
// 무시되고, **상한 내 기존 키의 누적은 계속**되는지 검증한다.
func TestReport_MeasurementCapEnforced(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-cap-m", map[string]any{
		"report_emit_mode": reportEmitModeCombined,
	})

	obj := make(map[string]any, maxReportMeasurements*2)
	for i := 0; i < maxReportMeasurements*2; i++ {
		obj[fmt.Sprintf("m%03d", i)] = float64(i)
	}
	feedUplink(t, a, reportUplink("dev-1", obj))
	// 같은 키를 한 번 더 — 기존 키 누적이 상한과 무관하게 계속되는지 본다.
	feedUplink(t, a, reportUplink("dev-1", obj))
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 1 {
		t.Fatalf("리포트 수 = %d, want 1", len(recs))
	}
	if got := len(recs[0].Measurements); got != maxReportMeasurements {
		t.Errorf("measurements 개수 = %d, want %d (상한)", got, maxReportMeasurements)
	}
	// 채택된 키는 정렬 순서 앞쪽(m000..)이며 두 번 누적되었다.
	first, ok := recs[0].Measurements["m000"]
	if !ok {
		t.Fatal("m000 이 채택되지 않았다 — 정렬된 키 앞쪽이 입장해야 한다")
	}
	if first.Count != 2 {
		t.Errorf("m000.count = %d, want 2 (상한 도달 후에도 기존 키는 누적)", first.Count)
	}
}

// TestReport_GatewayCapEnforced 는 디바이스당 게이트웨이 상한이 강제되는지 검증한다.
func TestReport_GatewayCapEnforced(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-cap-g", nil)

	rx := make([]uplinkRxInfo, 0, maxReportGateways*3)
	for i := 0; i < maxReportGateways*3; i++ {
		rx = append(rx, uplinkRxInfo{GatewayID: fmt.Sprintf("gw-%03d", i), RSSI: -50 - i, SNR: 1})
	}
	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 1.0}, rx...))
	a.emitMeasurementReports()

	recs := drainReports(t, a)
	if len(recs) != 1 {
		t.Fatalf("리포트 수 = %d, want 1", len(recs))
	}
	if got := len(recs[0].Radio); got != maxReportGateways {
		t.Errorf("radio 항목 수 = %d, want %d (상한)", got, maxReportGateways)
	}
	if got := recs[0].Gateways; got != maxReportGateways {
		t.Errorf("gateways = %d, want %d (상한)", got, maxReportGateways)
	}
}

// TestReport_DeviceCapEnforced 는 추적 디바이스 상한이 강제되는지 검증한다.
func TestReport_DeviceCapEnforced(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-cap-d", nil)

	for i := 0; i < maxReportDevices+50; i++ {
		feedUplink(t, a, reportUplink(fmt.Sprintf("dev-%05d", i), map[string]any{"temperature": 1.0}))
	}

	a.reportsMu.Lock()
	tracked := len(a.reports)
	a.reportsMu.Unlock()

	if tracked != maxReportDevices {
		t.Errorf("추적 디바이스 수 = %d, want %d (상한)", tracked, maxReportDevices)
	}
}

// ──────────────────────────────────────────────────────────────────────────
// 설정 검증 / 수명.
// ──────────────────────────────────────────────────────────────────────────

// TestReportEmitMode_InvalidValueRejected 는 무효 enum 이 **생성 경로와 Configure
// 경로 양쪽에서** 거부되는지 검증한다 (조용한 폴백 금지).
func TestReportEmitMode_InvalidValueRejected(t *testing.T) {
	badOpts := map[string]any{"report_emit_mode": "combined_x"}

	if err := validateReportEmitMode(badOpts); !errors.Is(err, ErrInvalidReportEmitMode) {
		t.Errorf("validateReportEmitMode err = %v, want ErrInvalidReportEmitMode", err)
	}

	// 생성 경로.
	resetNameRegistryForTest()
	disabled := false
	_, err := NewChirpStackAgent(agent.AgentConfig{
		ID: "id-report-bad", Name: "report-bad", Type: "chirpstack", Enabled: &disabled,
		Transport: agent.TransportConfig{Options: badOpts},
	})
	if !errors.Is(err, ErrInvalidReportEmitMode) {
		t.Errorf("NewChirpStackAgent err = %v, want ErrInvalidReportEmitMode", err)
	}

	// Configure(strict) 경로.
	if _, err := parseChirpStackConfigStrict(agent.AgentConfig{
		Transport: agent.TransportConfig{Options: badOpts},
	}); !errors.Is(err, ErrInvalidReportEmitMode) {
		t.Errorf("strict 파싱 err = %v, want ErrInvalidReportEmitMode", err)
	}

	a := newEmitModeAgent(t, "report-cfg", map[string]any{"report_emit_mode": reportEmitModeCombined})
	err = a.Configure(agent.AgentConfig{
		ID: "id-report-cfg", Name: "report-cfg", Type: "chirpstack", Enabled: &disabled,
		Transport: agent.TransportConfig{Options: badOpts},
	})
	if !errors.Is(err, ErrInvalidReportEmitMode) {
		t.Fatalf("Configure err = %v, want ErrInvalidReportEmitMode", err)
	}
	// 거부되었으므로 이전 설정이 그대로 유지된다(절반만 갱신 금지).
	if got := a.cs().ReportEmitMode; got != reportEmitModeCombined {
		t.Errorf("거부 후 모드 = %q, want %q (이전 설정 유지)", got, reportEmitModeCombined)
	}

	// 비문자열 타입도 거부한다.
	if err := validateReportEmitMode(map[string]any{"report_emit_mode": 3}); !errors.Is(err, ErrInvalidReportEmitMode) {
		t.Errorf("숫자 값 err = %v, want ErrInvalidReportEmitMode", err)
	}
	// 빈 문자열은 "미지정" 으로 보아 허용하고 기본값을 유지한다.
	if err := validateReportEmitMode(map[string]any{"report_emit_mode": ""}); err != nil {
		t.Errorf("빈 문자열 err = %v, want nil", err)
	}
}

// TestReportConfig_CoercionSharedWithValidator 는 report_interval 이 기존 공유
// 강제변환 계층(coerceDuration)을 그대로 쓰는지 검증한다 — 숫자/정수 문자열/duration
// 문자열이 모두 같은 값으로 읽혀야 파서와 검증기가 갈라지지 않는다.
func TestReportConfig_CoercionSharedWithValidator(t *testing.T) {
	cases := []struct {
		raw  any
		want time.Duration
	}{
		{60, 60 * time.Second},
		{float64(60), 60 * time.Second},
		{"60", 60 * time.Second},
		{"1m30s", 90 * time.Second},
	}
	for _, tc := range cases {
		opts := map[string]any{"report_interval": tc.raw}
		if err := validateChirpStackOptions(opts); err != nil {
			t.Errorf("validate(%v) err = %v, want nil", tc.raw, err)
			continue
		}
		cc := parseChirpStackConfig(agent.AgentConfig{Transport: agent.TransportConfig{Options: opts}})
		if cc.ReportInterval != tc.want {
			t.Errorf("report_interval(%v) = %v, want %v", tc.raw, cc.ReportInterval, tc.want)
		}
	}

	// 해석 불가한 값은 검증기가 거부한다(파서와 동일 판정).
	if err := validateChirpStackOptions(map[string]any{"report_interval": "쉰"}); err == nil {
		t.Error("해석 불가한 report_interval 을 거부하지 않았다")
	}
	// 빈 문자열은 미지정 → 기본값(0=off).
	if err := validateChirpStackOptions(map[string]any{"report_interval": ""}); err != nil {
		t.Errorf("빈 문자열 err = %v, want nil", err)
	}
	cc := parseChirpStackConfig(agent.AgentConfig{})
	if cc.ReportInterval != 0 {
		t.Errorf("report_interval 기본값 = %v, want 0 (off)", cc.ReportInterval)
	}
}

// TestReportReporter_LifecycleIdempotent 는 리포트 goroutine 의 기동/정지가
// idempotent 하고 누수 없이 끝나는지 검증한다 (comm watchdog 과 동일 규약).
//
// interval<=0 또는 emit_report=false 면 아예 기동하지 않는다.
func TestReportReporter_LifecycleIdempotent(t *testing.T) {
	withMemDeviceIDRepo(t)

	off := newEmitModeAgent(t, "report-life-off", map[string]any{"emit_report": true})
	off.startMeasurementReporter() // report_interval 미설정(0) → 기동 안 함.
	off.mu.RLock()
	started := off.mrStarted
	off.mu.RUnlock()
	if started {
		t.Error("report_interval=0 인데 리포트 루프가 기동되었다")
	}
	off.stopMeasurementReporter() // 미기동 상태에서도 안전(no-op).

	on := newReportAgent(t, "report-life-on", map[string]any{"report_interval": "50ms"})
	on.startMeasurementReporter()
	on.startMeasurementReporter() // idempotent — 두 번째는 no-op.
	on.mu.RLock()
	started = on.mrStarted
	on.mu.RUnlock()
	if !started {
		t.Fatal("리포트 루프가 기동되지 않았다")
	}

	// tick 이 실제로 리포트를 방출한다.
	feedUplink(t, on, reportUplink("dev-1", map[string]any{"temperature": 3.0}))
	deadline := time.After(2 * time.Second)
	for {
		if len(drainReports(t, on)) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("tick 으로 리포트가 방출되지 않았다")
		case <-time.After(10 * time.Millisecond):
		}
	}

	on.stopMeasurementReporter()
	on.stopMeasurementReporter() // idempotent.
	on.mu.RLock()
	started = on.mrStarted
	on.mu.RUnlock()
	if started {
		t.Error("정지 후에도 mrStarted 가 true 이다")
	}
}

// TestReport_DisabledByDefaultAccumulatesNothing 는 emit_report 가 꺼져 있으면
// 누적 자체가 일어나지 않는지 검증한다 (기본 경로에 비용이 붙지 않는다).
func TestReport_DisabledByDefaultAccumulatesNothing(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newEmitModeAgent(t, "report-disabled", nil)

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 1.0}))

	a.reportsMu.Lock()
	tracked := len(a.reports)
	a.reportsMu.Unlock()
	if tracked != 0 {
		t.Errorf("emit_report=false 인데 %d개 디바이스가 누적되었다", tracked)
	}

	a.emitMeasurementReports()
	if recs := drainReports(t, a); len(recs) != 0 {
		t.Errorf("emit_report=false 인데 리포트 %d건 방출됨", len(recs))
	}
}

// TestReport_DeviceStatePathUntouched 는 measurement_report 도입이 기존
// device_state.report 경로를 건드리지 않았는지 검증한다 (이름 충돌 방지 확인).
func TestReport_DeviceStatePathUntouched(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newReportAgent(t, "report-devstate", map[string]any{"emit_comm_state": true})

	feedUplink(t, a, reportUplink("dev-1", map[string]any{"temperature": 1.0},
		uplinkRxInfo{GatewayID: "gw-a", RSSI: -50, SNR: 5}))
	a.emitReports() // device_state.report (기존 경로).

	var deviceStates, measurementReports int
	for _, b := range drainRaw(a) {
		switch recordKindOf(t, b) {
		case recordKindDeviceState:
			var rec deviceStateRecord
			if err := json.Unmarshal(b, &rec); err != nil {
				t.Fatalf("unmarshal deviceStateRecord: %v", err)
			}
			// 기존 계약: state 스칼라 3종이 그대로 실린다.
			if rec.State.RSSI != -50 || rec.State.SNR != 5 || rec.State.GatewayID != "gw-a" {
				t.Errorf("device_state.state 가 변형되었다: %+v", rec.State)
			}
			deviceStates++
		case recordKindMeasurementReport:
			measurementReports++
		}
	}
	if deviceStates == 0 {
		t.Error("device_state 레코드가 방출되지 않았다")
	}
	if measurementReports != 0 {
		t.Error("emitReports(device_state 경로)가 measurement_report 를 방출했다")
	}
}
