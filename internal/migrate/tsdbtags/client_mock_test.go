// client_mock_test.go — MockClient + ScanSchema 단위 테스트.
package tsdbtags

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

func TestMockClient_FromFile(t *testing.T) {
	t.Parallel()

	mc, err := NewMockClientFromFile("testdata/v2-schema.json")
	if err != nil {
		t.Fatalf("NewMockClientFromFile: %v", err)
	}
	if mc.Version() != TargetV2 {
		t.Errorf("Version = %v, want v2", mc.Version())
	}
	ms, err := mc.ListMeasurements(context.Background())
	if err != nil {
		t.Fatalf("ListMeasurements: %v", err)
	}
	if len(ms) != 3 {
		t.Errorf("v2 fixture 의 measurement 수 = %d, want 3", len(ms))
	}
}

func TestMockClient_V3FixtureFile(t *testing.T) {
	t.Parallel()

	mc, err := NewMockClientFromFile("testdata/v3-schema.json")
	if err != nil {
		t.Fatalf("NewMockClientFromFile: %v", err)
	}
	if mc.Version() != TargetV3 {
		t.Errorf("Version = %v, want v3", mc.Version())
	}
}

func TestMockClient_Ping(t *testing.T) {
	t.Parallel()

	// 정상.
	mc := NewMockClient(MockSchema{Version: TargetV2})
	if err := mc.Ping(context.Background()); err != nil {
		t.Errorf("정상 ping 에서 에러: %v", err)
	}

	// PingErr 시뮬레이션.
	mcErr := NewMockClient(MockSchema{Version: TargetV2, PingErr: "connection refused"})
	if err := mcErr.Ping(context.Background()); err == nil {
		t.Errorf("PingErr 시 에러를 반환해야 함")
	}
}

func TestMockClient_ListTagValues(t *testing.T) {
	t.Parallel()

	mc, _ := NewMockClientFromFile("testdata/v2-schema.json")
	values, err := mc.ListTagValues(context.Background(), "indoor_temp", "device_id")
	if err != nil {
		t.Fatalf("ListTagValues: %v", err)
	}
	want := []string{"lgcnp:81", "lgcnp:82", "samsung:0.0.16"}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("tag values:\n  got=%v\n  want=%v", values, want)
	}
}

func TestMockClient_Close(t *testing.T) {
	t.Parallel()

	mc := NewMockClient(MockSchema{Version: TargetV2})
	if err := mc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// 닫힌 후 Ping 호출 시 에러.
	if err := mc.Ping(context.Background()); err == nil {
		t.Errorf("닫힌 client 에서 Ping 이 에러를 반환해야 함")
	}
}

func TestMockClient_FailingMeasurement(t *testing.T) {
	t.Parallel()

	mc := NewMockClient(MockSchema{
		Version:            TargetV2,
		Measurements:       []string{"good", "bad"},
		FailingMeasurement: "bad",
		TagsByMeasurement: map[string]map[string][]string{
			"good": {"device_id": []string{"lgcnp:81"}},
		},
	})
	if _, err := mc.ListTagKeys(context.Background(), "bad"); err == nil {
		t.Errorf("FailingMeasurement 에서 에러를 반환해야 함")
	}
	if _, err := mc.ListTagKeys(context.Background(), "good"); err != nil {
		t.Errorf("good measurement 에서 에러: %v", err)
	}
}

func TestScanSchema_FullScan(t *testing.T) {
	t.Parallel()

	mc, _ := NewMockClientFromFile("testdata/v2-schema.json")
	scanned, err := ScanSchema(context.Background(), mc, nil)
	if err != nil {
		t.Fatalf("ScanSchema: %v", err)
	}

	// indoor_temp/device_id 의 3 value + outdoor_temp/device_id 2 value
	// (lgcnp:81 은 indoor + outdoor 공통이므로 단일 entry 의 measurements 가 2)
	// + indoor/agent 2 + outdoor/agent 2 + indoor/room 2 + hvac_status/device_id 3 + hvac/agent 1
	// 결과는 (value, tagKey) 별로 dedup.
	// 다음 (value, tagKey) 가 존재해야 한다:
	mustExist := []struct{ value, tagKey string }{
		{"lgcnp:81", "device_id"},
		{"lgcnp:82", "device_id"},
		{"samsung:0.0.16", "device_id"},
		{"century:7", "device_id"},
		{"lgcnp:99a", "device_id"},
		{"lgcnp:99b", "device_id"},
		{"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", "device_id"},
		{"lgcnp", "agent"},
		{"samsung", "agent"},
		{"century", "agent"},
		{"north-1", "room"},
		{"north-2", "room"},
	}
	for _, want := range mustExist {
		found := false
		for _, s := range scanned {
			if s.Value == want.value && s.TagKey == want.tagKey {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("스캔 결과에 (%q, %q) 가 없음", want.value, want.tagKey)
		}
	}

	// lgcnp:81 은 indoor_temp + outdoor_temp 두 measurement 에서 등장해야 한다.
	for _, s := range scanned {
		if s.Value == "lgcnp:81" && s.TagKey == "device_id" {
			ms := make([]string, len(s.Measurements))
			copy(ms, s.Measurements)
			sort.Strings(ms)
			want := []string{"indoor_temp", "outdoor_temp"}
			if !reflect.DeepEqual(ms, want) {
				t.Errorf("lgcnp:81 의 measurements:\n  got=%v\n  want=%v", ms, want)
			}
		}
	}
}

func TestScanSchema_RestrictMeasurements(t *testing.T) {
	t.Parallel()

	mc, _ := NewMockClientFromFile("testdata/v2-schema.json")
	scanned, err := ScanSchema(context.Background(), mc, []string{"indoor_temp"})
	if err != nil {
		t.Fatalf("ScanSchema: %v", err)
	}
	// indoor_temp 만 — outdoor_temp/hvac_status 의 unique value 는 제외.
	for _, s := range scanned {
		for _, m := range s.Measurements {
			if m != "indoor_temp" {
				t.Errorf("restrict 무시: %q 가 결과에 포함됨", m)
			}
		}
	}
	// hvac_status 에만 있는 lgcnp:99a 는 제외되어야 함.
	for _, s := range scanned {
		if s.Value == "lgcnp:99a" {
			t.Errorf("restrict 후에도 lgcnp:99a (hvac_status 전용) 가 포함됨")
		}
	}
}
