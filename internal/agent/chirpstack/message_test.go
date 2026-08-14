package chirpstack

import (
	"io"
	"log/slog"
	"testing"
)

// silentLogger 는 테스트 소음을 줄이는 no-op 로거이다.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recordsByMeasurement 는 편의상 measurement→record 맵으로 인덱싱한다.
func recordsByMeasurement(recs []measurementRecord) map[string]measurementRecord {
	m := make(map[string]measurementRecord, len(recs))
	for _, r := range recs {
		m[r.Measurement] = r
	}
	return m
}

// TestBuildMeasurementRecords_SingleScalar 는 실측 픽스처(단일 measurement)의
// fan-out 을 검증한다 (AC-1a, REQ-FROZEN-01).
func TestBuildMeasurementRecords_SingleScalar(t *testing.T) {
	up, err := decodeUplink(loadRawUplink(t))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}

	recs := buildMeasurementRecords(up, parseUplinkTimeMs(up.Time), silentLogger())
	if len(recs) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Measurement != "magnet_status" {
		t.Errorf("measurement = %q", r.Measurement)
	}
	if r.Value != "close" {
		t.Errorf("value = %v, want close", r.Value)
	}
	if r.UnitID != "24e124141d180806" {
		t.Errorf("unit_id = %q", r.UnitID)
	}
	// time = 2026-08-11T23:32:01.129Z → UnixMilli.
	if r.TimeMs <= 0 {
		t.Errorf("time_ms = %d, want > 0", r.TimeMs)
	}
	if r.Tags["location"] != "실습실" || r.Tags["point"] != "앞문" || r.Tags["spot"] != "앞문" {
		t.Errorf("tags verbatim mismatch: %v", r.Tags)
	}
}

// TestBuildMeasurementRecords_MultiFanout 는 다중 measurement fan-out 을 검증한다
// (AC-1b): 각 스칼라가 별도 레코드로 방출되고 동일 time/unit/tags 를 공유한다.
func TestBuildMeasurementRecords_MultiFanout(t *testing.T) {
	up := &uplink{
		Time: "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{
			DevEui: "aabbccdd",
			Tags:   map[string]string{"location": "L"},
		},
		Object: map[string]any{
			"temperature": 24.5,
			"humidity":    float64(60),
			"co2":         float64(800),
			"battery":     float64(95),
		},
	}

	recs := buildMeasurementRecords(up, parseUplinkTimeMs(up.Time), silentLogger())
	if len(recs) != 4 {
		t.Fatalf("len(records) = %d, want 4", len(recs))
	}
	byM := recordsByMeasurement(recs)
	for _, k := range []string{"temperature", "humidity", "co2", "battery"} {
		r, ok := byM[k]
		if !ok {
			t.Errorf("missing measurement %q", k)
			continue
		}
		if r.UnitID != "aabbccdd" {
			t.Errorf("%s unit_id = %q", k, r.UnitID)
		}
		if r.TimeMs != recs[0].TimeMs {
			t.Errorf("%s time_ms differs", k)
		}
		if r.Tags["location"] != "L" {
			t.Errorf("%s tags mismatch", k)
		}
	}
	if byM["temperature"].Value != 24.5 {
		t.Errorf("temperature value = %v", byM["temperature"].Value)
	}
}

// TestBuildMeasurementRecords_NonScalarSkip 는 비스칼라 값 skip + 스칼라 형제
// 방출을 검증한다 (AC-1c, REQ-M3-05).
func TestBuildMeasurementRecords_NonScalarSkip(t *testing.T) {
	up := &uplink{
		Time:       "2026-08-11T23:32:01.129+00:00",
		DeviceInfo: uplinkDeviceInfo{DevEui: "eui"},
		Object: map[string]any{
			"sensors":     map[string]any{"a": float64(1)},
			"arr":         []any{float64(1), float64(2)},
			"temperature": 21.0,
		},
	}

	recs := buildMeasurementRecords(up, parseUplinkTimeMs(up.Time), silentLogger())
	if len(recs) != 1 {
		t.Fatalf("len(records) = %d, want 1 (only scalar sibling)", len(recs))
	}
	if recs[0].Measurement != "temperature" {
		t.Errorf("measurement = %q, want temperature", recs[0].Measurement)
	}
}

// TestParseUplinkTimeMs 는 RFC3339(오프셋/소수초) 파싱과 UnixMilli 변환을 검증한다.
func TestParseUplinkTimeMs(t *testing.T) {
	// 2026-08-11T23:32:01.129+00:00 → 결정적 절대값 대신 밀리초 성분으로 검증.
	ms := parseUplinkTimeMs("2026-08-11T23:32:01.129+00:00")
	if ms <= 0 {
		t.Fatalf("time_ms = %d, want > 0", ms)
	}
	// 소수 3자리(밀리초) 보존 검증: 129ms 성분.
	if ms%1000 != 129 {
		t.Errorf("time_ms millis = %d, want 129", ms%1000)
	}
	if parseUplinkTimeMs("") != 0 {
		t.Error("empty time should yield 0")
	}
	if parseUplinkTimeMs("not-a-time") != 0 {
		t.Error("invalid time should yield 0")
	}
}
