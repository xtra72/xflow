package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// newStyleTestLogger 는 실제 공유 핸들러 옵션(logHandlerOpts)을 사용하는 JSON
// 로거를 buf 에 연결해 반환한다. 이렇게 하면 replaceLogAttr(시간 재포맷 +
// 식별자 스타일 필터)가 실제 프로덕션 경로와 동일하게 적용된다.
func newStyleTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, logHandlerOpts))
}

// decodeLine 은 buf 의 마지막 JSON 로그 라인을 map 으로 파싱한다.
func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := bytes.TrimSpace(buf.Bytes())
	if len(line) == 0 {
		t.Fatalf("로그 출력이 비어 있다")
	}
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		t.Fatalf("JSON 파싱 실패: %v (line=%q)", err, line)
	}
	return m
}

func TestSetGetLogIDStyle(t *testing.T) {
	// 테스트 종료 후 전역 상태를 기본값으로 복원한다.
	t.Cleanup(func() { SetLogIDStyle(IDStyleBoth) })

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"both", "both", "both"},
		{"name", "name", "name"},
		{"id", "id", "id"},
		{"대문자 정규화", "NAME", "name"},
		{"공백 트림", "  id  ", "id"},
		{"빈 값은 both", "", "both"},
		{"알 수 없는 값은 both", "garbage", "both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLogIDStyle(tt.in)
			if got := GetLogIDStyle(); got != tt.want {
				t.Errorf("GetLogIDStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReplaceLogAttrFiltering 은 각 스타일에서 id 계열/이름 계열 필드가 올바르게
// 유지/생략되는지 검증한다.
func TestReplaceLogAttrFiltering(t *testing.T) {
	t.Cleanup(func() { SetLogIDStyle(IDStyleBoth) })

	// 로그 라인에 붙일 대표 필드 (id 계열 + 이름 계열 + 항상 유지 계열).
	logArgs := []any{
		// id 계열
		"device_uid", "uuid-1234",
		"device_id", "agent:81",
		"instance_id", "inst-1",
		"flow_id", "flow-1",
		"node_id", "node-1",
		"bridge_id", "br-1",
		"sub_dev_id", "0x3B",
		"id", "generic-1",
		// 이름 계열
		"name", "reader-01",
		"device", "lg_hvacr01/indoor-1",
		"device_agent", "lg_hvacr01",
		"device_name", "indoor-1",
		// 항상 유지 계열
		"component", "agent.modbus.reader-01",
		"type", "modbus",
		"actor", "admin",
	}

	idKeys := []string{
		"device_uid", "device_id", "instance_id", "flow_id",
		"node_id", "bridge_id", "sub_dev_id", "id",
	}
	nameKeys := []string{"name", "device", "device_agent", "device_name"}
	alwaysKeys := []string{"component", "type", "actor"}

	tests := []struct {
		style       string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			style:       IDStyleBoth,
			wantPresent: append(append(append([]string{}, idKeys...), nameKeys...), alwaysKeys...),
			wantAbsent:  nil,
		},
		{
			style:       IDStyleName,
			wantPresent: append(append([]string{}, nameKeys...), alwaysKeys...),
			wantAbsent:  idKeys,
		},
		{
			style:       IDStyleID,
			wantPresent: append(append([]string{}, idKeys...), alwaysKeys...),
			wantAbsent:  nameKeys,
		},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			SetLogIDStyle(tt.style)

			var buf bytes.Buffer
			logger := newStyleTestLogger(&buf)
			logger.Info("test message", logArgs...)

			got := decodeLine(t, &buf)

			for _, k := range tt.wantPresent {
				if _, ok := got[k]; !ok {
					t.Errorf("style=%s: 필드 %q 가 있어야 하지만 생략됨", tt.style, k)
				}
			}
			for _, k := range tt.wantAbsent {
				if _, ok := got[k]; ok {
					t.Errorf("style=%s: 필드 %q 가 생략되어야 하지만 남아 있음 (값=%v)",
						tt.style, k, got[k])
				}
			}

			// msg 는 어떤 스타일에서도 항상 유지되어야 한다.
			if got["msg"] != "test message" {
				t.Errorf("style=%s: msg 필드가 손실됨: %v", tt.style, got["msg"])
			}
		})
	}
}

// TestBothModeNoRegression 은 both 모드(기본)에서 출력이 필터링 이전과 동일함을
// 보장한다 — 무회귀 핵심 검증.
func TestBothModeNoRegression(t *testing.T) {
	t.Cleanup(func() { SetLogIDStyle(IDStyleBoth) })
	SetLogIDStyle(IDStyleBoth)

	var buf bytes.Buffer
	logger := newStyleTestLogger(&buf)
	logger.Info("msg", "device_uid", "u1", "name", "n1", "actor", "admin")

	got := decodeLine(t, &buf)
	for _, k := range []string{"device_uid", "name", "actor"} {
		if _, ok := got[k]; !ok {
			t.Errorf("both 모드에서 %q 필드가 유지되어야 한다", k)
		}
	}
}

// TestTimeReformatStillApplies 는 스타일 필터링과 무관하게 time 재포맷(소수점
// 6자리 고정)이 계속 적용됨을 검증한다.
func TestTimeReformatStillApplies(t *testing.T) {
	t.Cleanup(func() { SetLogIDStyle(IDStyleBoth) })

	for _, style := range []string{IDStyleBoth, IDStyleName, IDStyleID} {
		SetLogIDStyle(style)
		var buf bytes.Buffer
		logger := newStyleTestLogger(&buf)
		logger.Info("msg")

		got := decodeLine(t, &buf)
		ts, ok := got[slog.TimeKey].(string)
		if !ok {
			t.Fatalf("style=%s: time 필드가 문자열이 아니다: %v", style, got[slog.TimeKey])
		}
		// 소수점 6자리 고정 포맷(...T...:....000000...)이어야 한다.
		if len(ts) < len("2006-01-02T15:04:05.000000Z") {
			t.Errorf("style=%s: time 포맷이 6자리 고정이 아님: %q", style, ts)
		}
	}
}

// TestNestedGroupNotFiltered 는 중첩 그룹 내부의 식별자 필드는 필터링하지 않음을
// 검증한다 (최상위 그룹에서만 필터링 — 과도 필터링 방지).
func TestNestedGroupNotFiltered(t *testing.T) {
	t.Cleanup(func() { SetLogIDStyle(IDStyleBoth) })
	SetLogIDStyle(IDStyleName) // id 계열 생략 모드

	var buf bytes.Buffer
	logger := newStyleTestLogger(&buf)
	// 그룹 내부에 device_uid 를 넣는다.
	logger.LogAttrs(context.Background(), slog.LevelInfo, "msg",
		slog.Group("nested", slog.String("device_uid", "u1")))

	got := decodeLine(t, &buf)
	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested 그룹이 없다: %v", got)
	}
	if _, ok := nested["device_uid"]; !ok {
		t.Errorf("중첩 그룹 내부 device_uid 는 유지되어야 한다 (최상위만 필터링)")
	}
}
