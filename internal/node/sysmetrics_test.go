package node

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/xtra/xflow/pkg/flow"
)

// sysmetrics-in 노드 테스트.
//
// 노드의 하중 지지점은 "에이전트의 일괄 레코드를 flow 메시지로 정확히 옮기는가"다.
// 수신 루프·재초기화 골격은 chirpstack-in 과 동일하므로, 여기서는 변환과 설정 검증을
// 잠근다.

// batchJSON 은 에이전트가 내보내는 일괄 레코드 형상을 그대로 만든다.
func batchJSON(t *testing.T, measurement string, timeMs int64, fields map[string]any) []byte {
	t.Helper()
	rec := map[string]any{
		"measurement": measurement,
		"time_ms":     timeMs,
		"fields":      fields,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("레코드 직렬화 실패: %v", err)
	}
	return data
}

func TestBuildSysMetricsMessage_기본변환(t *testing.T) {
	const timeMs int64 = 1_700_000_000_000
	data := batchJSON(t, "sysmetrics", timeMs, map[string]any{
		"cpu": map[string]float64{"usage_percent": 42.5},
	})

	msg, ok := buildSysMetricsMessage(data, "n1", nil, "sys", DefaultEmitOptions())
	if !ok {
		t.Fatal("변환 실패, want 성공")
	}

	// measurement 는 항상 하나 — 값은 payload 에 그룹별로 중첩된다.
	if got, _ := msg.Metadata().Get("measurement"); got != "sysmetrics" {
		t.Errorf("measurement = %v, want sysmetrics", got)
	}
	cpu, ok := msg.Payload().Get("cpu")
	if !ok {
		t.Fatal("payload.cpu 누락")
	}
	group, ok := cpu.(map[string]any)
	if !ok {
		t.Fatalf("payload.cpu 타입 = %T, want 오브젝트", cpu)
	}
	if group["usage_percent"] != 42.5 {
		t.Errorf("payload.cpu.usage_percent = %v, want 42.5", group["usage_percent"])
	}
	if got := msg.Timestamp(); !got.Equal(time.UnixMilli(timeMs)) {
		t.Errorf("Timestamp = %v, want %v", got, time.UnixMilli(timeMs))
	}
	if msg.Type() != "event" {
		t.Errorf("Type = %q, want event", msg.Type())
	}
}

func TestBuildSysMetricsMessage_모든그룹을payload로옮긴다(t *testing.T) {
	// 그룹이 하나라도 빠지면 그 지표는 영영 저장되지 않는다.
	data := batchJSON(t, "sysmetrics", 1, map[string]any{
		"cpu":     map[string]float64{"usage_percent": 42.5},
		"memory":  map[string]float64{"used_bytes": 8000},
		"storage": map[string]map[string]float64{"/data": {"used_bytes": 250}},
		"network": map[string]map[string]float64{"en0": {"bytes_recv": 200}},
	})

	msg, ok := buildSysMetricsMessage(data, "n1", nil, "sys", DefaultEmitOptions())
	if !ok {
		t.Fatal("변환 실패, want 성공")
	}

	for _, group := range []string{"cpu", "memory", "storage", "network"} {
		if _, ok := msg.Payload().Get(group); !ok {
			t.Errorf("payload.%s 누락", group)
		}
	}
}

func TestBuildSysMetricsMessage_인스턴스중첩을보존한다(t *testing.T) {
	// 노드가 구조를 펴면 payload 형상이 에이전트가 정한 것과 달라져,
	// 하류 참조 경로가 두 곳에서 갈린다.
	data := batchJSON(t, "sysmetrics", 1, map[string]any{
		"network": map[string]map[string]float64{
			"en0": {"bytes_recv": 200},
			"lo0": {"bytes_recv": 5},
		},
	})

	msg, ok := buildSysMetricsMessage(data, "n1", nil, "sys", DefaultEmitOptions())
	if !ok {
		t.Fatal("변환 실패, want 성공")
	}

	raw, _ := msg.Payload().Get("network")
	net, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("payload.network 타입 = %T", raw)
	}
	en0, ok := net["en0"].(map[string]any)
	if !ok {
		t.Fatalf("payload.network.en0 타입 = %T", net["en0"])
	}
	if en0["bytes_recv"] != float64(200) {
		t.Errorf("payload.network.en0.bytes_recv = %v, want 200", en0["bytes_recv"])
	}
	if _, ok := net["lo0"]; !ok {
		t.Error("payload.network.lo0 누락 — 인스턴스가 덮어써졌다")
	}
}

func TestBuildSysMetricsMessage_값이없으면버린다(t *testing.T) {
	// 빈 메시지를 흘려보내면 하류가 빈 레코드를 쌓는다.
	data := batchJSON(t, "sysmetrics", 1, map[string]any{})

	if _, ok := buildSysMetricsMessage(data, "n1", nil, "sys", DefaultEmitOptions()); ok {
		t.Error("변환이 통과했다, want 실패")
	}
}

func TestBuildSysMetricsMessage_잘못된입력은버린다(t *testing.T) {
	opts := DefaultEmitOptions()

	tests := []struct {
		name string
		data []byte
	}{
		{name: "JSON 아님", data: []byte("not json")},
		// 측정 이름이 없으면 저장 키를 만들 수 없다.
		{name: "measurement 없음", data: []byte(`{"time_ms":1,"fields":{"cpu":{"a":1}}}`)},
		{name: "measurement 빈 문자열", data: []byte(`{"measurement":"","fields":{"cpu":{"a":1}}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := buildSysMetricsMessage(tt.data, "n1", nil, "sys", opts); ok {
				t.Error("변환이 통과했다, want 실패")
			}
		})
	}
}

func TestBuildSysMetricsMessage_시각없으면현재시각(t *testing.T) {
	// time_ms 가 0 이면 SetTimestamp 를 부르지 않아 message.New() 의 기본값이 남는다.
	data := batchJSON(t, "sysmetrics", 0, map[string]any{
		"cpu": map[string]float64{"usage_percent": 1},
	})

	before := time.Now()
	msg, ok := buildSysMetricsMessage(data, "n1", nil, "sys", DefaultEmitOptions())
	if !ok {
		t.Fatal("변환 실패, want 성공")
	}

	if msg.Timestamp().Before(before.Add(-time.Minute)) {
		t.Errorf("Timestamp = %v, 현재 시각 근처여야 한다", msg.Timestamp())
	}
}

func TestSysMetricsInNode_agentRef필수(t *testing.T) {
	n, err := NewSysMetricsInNode(flow.NodeDef{ID: "n1", Type: "sysmetrics-in"})
	if err != nil {
		t.Fatalf("NewSysMetricsInNode() 오류 = %v", err)
	}

	// agent_ref 가 없으면 노드가 무엇에 붙을지 알 수 없다 — 설정 단계에서 막는다.
	if err := n.Configure(map[string]any{}); err == nil {
		t.Fatal("오류가 없다, want 오류")
	}

	if err := n.Configure(map[string]any{"agent_ref": "sys-1"}); err != nil {
		t.Fatalf("Configure() 오류 = %v", err)
	}

	node, ok := n.(*SysMetricsInNode)
	if !ok {
		t.Fatalf("타입 = %T, want *SysMetricsInNode", n)
	}
	if got := node.AgentRef(); got.AgentID != "sys-1" {
		t.Errorf("AgentRef().AgentID = %q, want sys-1", got.AgentID)
	}
}

func TestSysMetricsInNode_리졸버없으면Init실패(t *testing.T) {
	n, err := NewSysMetricsInNode(flow.NodeDef{ID: "n1", Type: "sysmetrics-in"})
	if err != nil {
		t.Fatalf("NewSysMetricsInNode() 오류 = %v", err)
	}
	if err := n.Configure(map[string]any{"agent_ref": "sys-1"}); err != nil {
		t.Fatalf("Configure() 오류 = %v", err)
	}

	// AgentResolver 가 없으면 조용히 무음이 되는 대신 Init 에서 걸려야 한다.
	if err := n.Init(t.Context()); err == nil {
		t.Fatal("오류가 없다, want 오류")
	}
}

func TestSysMetricsInNode_레지스트리에등록되어있다(t *testing.T) {
	// 노드를 만들어도 레지스트리에 없으면 플로우 편집기에서 고를 수 없다.
	r := NewRegistry()

	if _, err := r.Create(flow.NodeDef{ID: "n1", Type: "sysmetrics-in"}); err != nil {
		t.Fatalf("sysmetrics-in 생성 실패: %v", err)
	}
}
