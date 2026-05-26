// detect_test.go — DetectTarget 의 헤더 기반 분기 단위 테스트.
//
// 모든 테스트는 httptest 의 in-process 서버를 사용한다 — 실제 InfluxDB
// 인스턴스에 절대 접근하지 않는다 (안전 가드).
package tsdbtags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassifyVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want Target
	}{
		{"v2.7.0", TargetV2},
		{"2.7.0", TargetV2},
		{"v3.0.0", TargetV3},
		{"3.1.0", TargetV3},
		{"V2.10", TargetV2}, // 대문자
		{"v1.8.10", TargetAuto},
		{"unknown", TargetV3}, // fallback
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := classifyVersion(tc.in); got != tc.want {
				t.Errorf("classifyVersion(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDetectTarget_V2Header(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Influxdb-Version", "v2.7.0")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	got, err := DetectTarget(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DetectTarget: %v", err)
	}
	if got != TargetV2 {
		t.Errorf("v2 헤더에서 v2 가 감지되어야 함: got %v", got)
	}
}

func TestDetectTarget_V3NoHeader(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got, err := DetectTarget(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DetectTarget: %v", err)
	}
	if got != TargetV3 {
		t.Errorf("헤더 없는 응답에서 v3 fallback 이 적용되어야 함: got %v", got)
	}
}

func TestDetectTarget_NetworkFailure(t *testing.T) {
	t.Parallel()

	// 즉시 close 되는 서버 — 연결 실패 시뮬레이션.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := DetectTarget(context.Background(), url)
	if err == nil {
		t.Errorf("연결 실패 시 에러를 반환해야 함")
	}
}
