package chirpstack

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 로스터 메타데이터/measurement 구조 변경 (group·location 승격, dev_eui 이동,
// measurement per-element {value,time_ms})
// ---------------------------------------------------------------------------

// fixtureUplinkTimeMs 는 픽스처/테스트 업링크의 time("2026-08-11T23:32:01.129+00:00")
// 을 UnixMilli 로 환산한 값이다. 벽시계가 아니라 업링크에서 파생한 시각인지 검증하는
// 고정 기준점이다.
const fixtureUplinkTimeMs int64 = 1786491121129

// rawUplinkCustom 은 time/tags/object 를 자유롭게 지정한 원시 업링크 JSON 을 만든다.
// tags 는 nil 이면 deviceInfo 에서 아예 생략된다.
func rawUplinkCustom(t *testing.T, timeStr string, tags map[string]any, obj map[string]any) []byte {
	t.Helper()
	devInfo := map[string]any{
		"devEui":            fixtureDevEui,
		"deviceName":        "WS301-180806",
		"deviceProfileName": "WS301",
		"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
	}
	if tags != nil {
		devInfo["tags"] = tags
	}
	b, err := json.Marshal(map[string]any{
		"time":       timeStr,
		"deviceInfo": devInfo,
		"object":     obj,
	})
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return b
}

// metadataAfterTags 는 지정 태그로 업링크 1건을 처리한 뒤 첫 디바이스의 Metadata 를
// 반환한다.
func metadataAfterTags(t *testing.T, agentName string, tags map[string]any) (*ChirpStackAgent, deviceMetadataSnapshot) {
	t.Helper()
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, agentName)
	a.handleUplink(rawUplinkCustom(t, "2026-08-11T23:32:01.129+00:00", tags,
		map[string]any{"temperature": 21.5}), "application/x")

	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	md := devs[0].Metadata()
	return a, deviceMetadataSnapshot{
		group:    md.Group,
		location: md.Location,
		labels:   md.Labels,
		props:    devs[0].State().Properties,
	}
}

// deviceMetadataSnapshot 은 단언에 쓰는 관측값 묶음이다.
type deviceMetadataSnapshot struct {
	group    string
	location string
	labels   map[string]string
	props    map[string]any
}

// TestMetadata_PromotesGroupAndLocation 는 group/location 태그가 전용 필드로 승격되고
// Labels 에서는 제거되며(복사가 아니라 이동), 나머지 태그는 Labels 에 남는지 검증한다.
func TestMetadata_PromotesGroupAndLocation(t *testing.T) {
	_, got := metadataAfterTags(t, "meta-promote", map[string]any{
		"group":    "3층",
		"location": "실습실",
		"point":    "앞문",
		"spot":     "앞문",
	})

	if got.group != "3층" {
		t.Errorf("Group = %q, want 3층", got.group)
	}
	if got.location != "실습실" {
		t.Errorf("Location = %q, want 실습실", got.location)
	}
	for _, k := range []string{"group", "location"} {
		if v, ok := got.labels[k]; ok {
			t.Errorf("Labels[%s] 잔존(=%q) — 승격은 이동이지 복사가 아니다", k, v)
		}
	}
	if got.labels["point"] != "앞문" || got.labels["spot"] != "앞문" {
		t.Errorf("승격 대상이 아닌 태그가 Labels 에서 사라졌다: %v", got.labels)
	}
}

// TestMetadata_GroupStaysEmptyWithoutTag 는 group 태그가 없을 때 Group 이 빈 문자열로
// 남는지 검증한다 — deviceProfileName 등으로 값을 지어내면 그룹 조회가 거짓 결과를 낸다.
//
// 태그 구성은 실측 데이터({location, point, spot})와 동일하다.
func TestMetadata_GroupStaysEmptyWithoutTag(t *testing.T) {
	_, got := metadataAfterTags(t, "meta-nogroup", map[string]any{
		"location": "실습실",
		"point":    "앞문",
		"spot":     "앞문",
	})

	if got.group != "" {
		t.Errorf("Group = %q, want \"\" (group 태그 없음)", got.group)
	}
	if got.location != "실습실" {
		t.Errorf("Location = %q, want 실습실", got.location)
	}
	if len(got.labels) != 3 { // point, spot, dev_eui
		t.Errorf("Labels = %v, want {point, spot, dev_eui}", got.labels)
	}
}

// TestMetadata_TagMatchingIsExactLowercase 는 승격 키 매칭이 정확한 소문자 매칭임을
// 명시적으로 고정한다 (tagKeyGroup/tagKeyLocation 상수 주석의 결정).
//
// 대소문자 변형("Location"/"GROUP"/"Group")은 승격되지 않고 Labels 에 그대로 남는다 —
// 값이 사라지지 않는 양성 실패이며, 사용자가 태그 이름을 고치면 된다.
func TestMetadata_TagMatchingIsExactLowercase(t *testing.T) {
	_, got := metadataAfterTags(t, "meta-case", map[string]any{
		"Location": "대문자L",
		"LOCATION": "전부대문자",
		"Group":    "대문자G",
		"GROUP":    "전부대문자G",
	})

	if got.location != "" {
		t.Errorf("Location = %q, want \"\" — 정확 소문자 매칭만 승격한다", got.location)
	}
	if got.group != "" {
		t.Errorf("Group = %q, want \"\" — 정확 소문자 매칭만 승격한다", got.group)
	}
	for k, want := range map[string]string{
		"Location": "대문자L",
		"LOCATION": "전부대문자",
		"Group":    "대문자G",
		"GROUP":    "전부대문자G",
	} {
		if got.labels[k] != want {
			t.Errorf("Labels[%s] = %q, want %q — 승격되지 않은 태그는 보존된다", k, got.labels[k], want)
		}
	}
}

// TestMetadata_DevEuiMovedToLabels 는 dev_eui 가 Metadata().Labels 에 있고
// DeviceState.Properties 에서는 사라졌는지 검증한다 (불변 식별 정보 ≠ 런타임 상태).
func TestMetadata_DevEuiMovedToLabels(t *testing.T) {
	_, got := metadataAfterTags(t, "meta-eui", map[string]any{"point": "앞문"})

	if got.labels[labelKeyDevEui] != fixtureDevEui {
		t.Errorf("Labels[dev_eui] = %q, want %q", got.labels[labelKeyDevEui], fixtureDevEui)
	}
	if v, ok := got.props[labelKeyDevEui]; ok {
		t.Errorf("properties[dev_eui] 존재(=%v) — Properties 에서 제거되어야 한다", v)
	}
}

// TestMetadata_DevEuiIsReservedLabelKey 는 사용자 ChirpStack 태그가 dev_eui 라는 이름을
// 써도 에이전트 값이 이기고, 덮어쓴 사실이 디바이스를 지목한 Warn 으로 남는지 검증한다.
func TestMetadata_DevEuiIsReservedLabelKey(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meta-eui-collide")

	// 경고를 관측하기 위해 에이전트 로거를 버퍼로 교체한다.
	var buf bytes.Buffer
	a.logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	a.handleUplink(rawUplinkCustom(t, "2026-08-11T23:32:01.129+00:00",
		map[string]any{"dev_eui": "사용자가-지정한-값", "point": "앞문"},
		map[string]any{"temperature": 21.5}), "application/x")

	labels := a.DeviceProvider().Devices()[0].Metadata().Labels
	if labels[labelKeyDevEui] != fixtureDevEui {
		t.Errorf("Labels[dev_eui] = %q, want 에이전트 값 %q — 예약 키는 사용자 태그가 이길 수 없다",
			labels[labelKeyDevEui], fixtureDevEui)
	}
	if labels["point"] != "앞문" {
		t.Errorf("충돌하지 않는 태그가 손실됐다: %v", labels)
	}

	logged := buf.String()
	if !strings.Contains(logged, "예약 라벨 키 충돌") {
		t.Errorf("충돌 경고가 없다. 로그:\n%s", logged)
	}
	if !strings.Contains(logged, fixtureDevEui) || !strings.Contains(logged, "WS301-180806") {
		t.Errorf("경고가 디바이스를 지목하지 않는다(devEui/deviceName 누락). 로그:\n%s", logged)
	}
}

// TestMetadata_NoTagsStillCarriesDevEuiLabel 은 태그가 아예 없어도 dev_eui 라벨이
// 생기고 Group/Location 은 비어 있는지 검증한다 (Labels nil 초기화 경로).
func TestMetadata_NoTagsStillCarriesDevEuiLabel(t *testing.T) {
	_, got := metadataAfterTags(t, "meta-notags", nil)

	if got.group != "" || got.location != "" {
		t.Errorf("Group=%q Location=%q, want 둘 다 \"\"", got.group, got.location)
	}
	if len(got.labels) != 1 || got.labels[labelKeyDevEui] != fixtureDevEui {
		t.Errorf("Labels = %v, want {dev_eui: %q} 만", got.labels, fixtureDevEui)
	}
}

// TestMeasurements_CarryUplinkDerivedTimeMs 는 measurement 가 {value, time_ms} 객체이고
// time_ms 가 벽시계가 아니라 업링크 time 에서 파생한 값인지 검증한다.
//
// 고정 기준점(fixtureUplinkTimeMs)과 정확히 일치해야 한다 — time.Now() 를 쓰면 이
// 단언이 깨진다.
func TestMeasurements_CarryUplinkDerivedTimeMs(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-time")

	a.handleUplink(rawUplinkCustom(t, "2026-08-11T23:32:01.129+00:00", nil,
		map[string]any{"temperature": 29.8, "humidity": 55.2}), "application/x")

	m := measurementsOf(t, a)
	for _, tc := range []struct {
		key  string
		want float64
	}{{"temperature", 29.8}, {"humidity", 55.2}} {
		gotVal, gotTime := measSample(t, m, tc.key)
		if gotVal != tc.want {
			t.Errorf("measurements[%s].value = %v, want %v", tc.key, gotVal, tc.want)
		}
		if gotTime != fixtureUplinkTimeMs {
			t.Errorf("measurements[%s].time_ms = %d, want %d (업링크 time 파생, 벽시계 아님)",
				tc.key, gotTime, fixtureUplinkTimeMs)
		}
	}
}

// TestMeasurements_MergeKeepsPerKeyTimeMs 는 서로 다른 시각의 업링크 2건을 병합할 때
// 키마다 자기 time_ms 를 유지하는지 검증한다 — 이번 업링크가 싣지 않은 키는 값도
// time_ms 도 이전 그대로여야 한다.
func TestMeasurements_MergeKeepsPerKeyTimeMs(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-time-merge")

	const t1 = "2026-08-11T23:32:01.129+00:00" // fixtureUplinkTimeMs
	const t2 = "2026-08-11T23:45:30.500+00:00"
	const t2Ms int64 = 1786491930500

	// 업링크 1: battery + temperature.
	a.handleUplink(rawUplinkCustom(t, t1, nil,
		map[string]any{"battery": 88.0, "temperature": 29.8}), "application/x")
	// 업링크 2: temperature 갱신 + humidity 신규. battery 는 싣지 않는다.
	a.handleUplink(rawUplinkCustom(t, t2, nil,
		map[string]any{"temperature": 30.1, "humidity": 55.2}), "application/x")

	m := measurementsOf(t, a)
	for _, tc := range []struct {
		key      string
		wantVal  float64
		wantTime int64
	}{
		{"battery", 88.0, fixtureUplinkTimeMs}, // 갱신되지 않음 → 값도 시각도 이전 그대로.
		{"temperature", 30.1, t2Ms},            // 갱신됨.
		{"humidity", 55.2, t2Ms},               // 신규.
	} {
		gotVal, gotTime := measSample(t, m, tc.key)
		if gotVal != tc.wantVal {
			t.Errorf("measurements[%s].value = %v, want %v", tc.key, gotVal, tc.wantVal)
		}
		if gotTime != tc.wantTime {
			t.Errorf("measurements[%s].time_ms = %d, want %d", tc.key, gotTime, tc.wantTime)
		}
	}
	if len(m) != 3 {
		t.Errorf("measurements = %v, want 3 keys", m)
	}
}

// TestMeasurements_TimeFallsBackToReceiptTime 는 업링크 time 이 없거나 파싱 불가일 때
// 수신 시각으로 폴백하는지 검증한다 (0 을 그대로 두면 모든 측정치가 1970 이 된다).
func TestMeasurements_TimeFallsBackToReceiptTime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeStr string
	}{
		{"빈 time", ""},
		{"파싱 불가 time", "not-a-timestamp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withMemDeviceIDRepo(t)
			a := newRunningTestAgent(t, "meas-fallback-"+tc.name)

			before := time.Now().UnixMilli()
			a.handleUplink(rawUplinkCustom(t, tc.timeStr, nil,
				map[string]any{"temperature": 21.5}), "application/x")
			after := time.Now().UnixMilli()

			_, gotTime := measSample(t, measurementsOf(t, a), "temperature")
			if gotTime < before || gotTime > after {
				t.Errorf("measurements[temperature].time_ms = %d, want 수신 시각 [%d, %d]",
					gotTime, before, after)
			}
		})
	}
}
