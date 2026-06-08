// broadcaster_m10_test.go 는 M10(그룹 L) MonitoringBroadcaster 의 추가 로그 writer
// 합류(monitor.logs 원격 스트림 탭용)를 검증한다(@SPEC:SPEC-REMOTE-001 M10, REQ-L06).
//
// 원격 monitor.logs 스트림은 노드의 로그 파이프라인을 in-process 로 탭해야 한다(A18).
// 브로드캐스터가 기본 writer 체인의 단독 소유자이므로, WithExtraLogWriter 로 추가 writer
// 를 합류시켜 노드의 로그 hub 가 동일 로그 라인을 수신하도록 한다(SetDefaultWriter 클로버
// 없이 공존).
package ws

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// TestWithExtraLogWriter 는 추가 로그 writer 가 기본 writer 체인에 합류되어 로그를
// 수신하는지 검증한다(REQ-L06 — 노드 로그 hub in-process 탭).
func TestWithExtraLogWriter(t *testing.T) {
	hub, cleanup := newTestHub(t)
	defer cleanup()

	sr := newMockStreamRouter()
	var extra bytes.Buffer

	b := NewMonitoringBroadcaster(hub, &mockMetricsSource{}, nil,
		WithStreamRouter(sr),
		WithExtraLogWriter(&extra),
	)
	if b.extraLogWriter == nil {
		t.Fatal("extraLogWriter 가 설정되지 않음")
	}

	// Start 후 기본 writer 가 extra 를 포함한 MultiWriter 여야 한다(SetDefaultWriter 경유).
	b.Start(t.Context())
	defer b.Stop()

	dw := sr.defWrite
	if dw == nil {
		t.Fatal("Start 후 기본 writer 가 설정되어야 함")
	}

	// 기본 writer 에 로그 라인을 쓰면 extra writer 도 수신해야 한다(MultiWriter 합류 검증).
	const line = `{"level":"INFO","msg":"m10-log"}`
	_, _ = io.WriteString(dw, line)
	if !strings.Contains(extra.String(), "m10-log") {
		t.Errorf("추가 로그 writer 가 로그를 수신하지 못함: got %q", extra.String())
	}
}

// TestWithExtraLogWriter_Nil 는 nil 추가 writer 가 무시되는지 검증한다(하위 호환).
func TestWithExtraLogWriter_Nil(t *testing.T) {
	hub, cleanup := newTestHub(t)
	defer cleanup()

	var w io.Writer // nil
	b := NewMonitoringBroadcaster(hub, &mockMetricsSource{}, slog.Default(),
		WithExtraLogWriter(w),
	)
	if b.extraLogWriter != nil {
		t.Error("nil 추가 writer 는 무시되어야 함")
	}
}
