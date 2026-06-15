package system

import (
	"context"
	"testing"
)

// TestRepro_DataTypeInt_AcceptsIntegralFloat 는 data_type=int 키에 JSON 경유 정수값
// (float64)을 쓸 때 타입 불일치로 거부되어 metric 이 유실되던 문제를 검증한다.
//
// 서브플로우 브리지가 메시지를 JSON 직렬화하면 모든 숫자가 float64 가 되므로,
// data_type=int(mode/fan_speed) 키 쓰기가 거부되어 metric=unknown 으로 남았다.
// 수정 후: 정수값 float64 는 int 로 허용·정규화되어 쓰기 성공 + metric 적용.
func TestRepro_DataTypeInt_AcceptsIntegralFloat(t *testing.T) {
	ag := NewStoreAgent()
	if err := ag.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	adapter := NewLazyNodeStoreAdapterWithAgent(
		func() Store { return ag.ForNamespace("") },
		func() *StoreAgent { return ag },
		"",
	)

	// data_type=int 키에 정수값 float64(2.0) 쓰기 (JSON 디코딩 결과 모사).
	err := adapter.SetWithMeta(context.Background(), "dev.mode", float64(2), StoreWriteMeta{
		DataType:   "int",
		MetricType: "mode",
	})
	if err != nil {
		t.Fatalf("정수값 float64 쓰기는 성공해야 한다: %v", err)
	}

	// @spec SPEC-STORE-004: 레지스트리/저장 키가 시리즈 인코딩으로 승격되었으므로
	// (dev.mode, mode, {}) 시리즈 키로 조회한다 (data_type 은 식별 차원 아님).
	seriesKey := EncodeSeriesKey(SeriesID{Key: "dev.mode", MetricType: "mode"})
	meta, ok := ag.StaticKeyMetaFor(seriesKey)
	if !ok {
		t.Fatal("키가 등록되어야 한다")
	}
	if meta.MetricType != "mode" {
		t.Errorf("metric_type=%q, want %q", meta.MetricType, "mode")
	}
	if meta.DataType != DataTypeInt {
		t.Errorf("data_type=%q, want int", meta.DataType)
	}

	// 저장값이 int 로 정규화되었는지 확인.
	entry, gErr := ag.ForNamespace("").Get(context.Background(), seriesKey)
	if gErr != nil {
		t.Fatalf("get: %v", gErr)
	}
	if _, isInt := entry.Value.(int64); !isInt {
		t.Errorf("저장값 타입=%T, int64 로 정규화되어야 한다 (값=%v)", entry.Value, entry.Value)
	}
}

// TestRepro_DataTypeInt_RejectsNonIntegralFloat 는 비정수 float(2.5)는 data_type=int
// 키에 여전히 거부되어야 함을 검증한다(정수값만 허용하는 안전성 보존).
func TestRepro_DataTypeInt_RejectsNonIntegralFloat(t *testing.T) {
	if matchesDataType(float64(2.5), DataTypeInt) {
		t.Error("비정수 float(2.5)는 data_type=int 에 매칭되면 안 된다")
	}
	if !matchesDataType(float64(2.0), DataTypeInt) {
		t.Error("정수값 float(2.0)는 data_type=int 에 매칭되어야 한다")
	}
	if !matchesDataType(int(3), DataTypeFloat) {
		t.Error("정수(3)는 data_type=float 에 매칭되어야 한다")
	}
}
