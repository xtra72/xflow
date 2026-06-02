package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- SwitchNode 인터페이스 준수 ---

var _ Node = (*SwitchNode)(nil)

// --- NewSwitchNode 테스트 ---

// TestNewSwitchNode_정상생성 은 SwitchNode가 올바르게 생성되는지 확인한다.
func TestNewSwitchNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("switch-1", "switch")
	node, err := NewSwitchNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "switch-1", node.Name())
	assert.Equal(t, "switch", node.Type())
}

// --- Init 테스트 ---

// TestSwitchNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestSwitchNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("switch-init", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	err := sn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, sn.CurrentState())
}

// --- Process 테스트 ---

// TestSwitchNode_Process_라우트없음_드롭 은 라우트가 없고 기본 포트도 없으면 드롭하는지 확인한다.
func TestSwitchNode_Process_라우트없음_드롭(t *testing.T) {
	def := flow.NewNodeDef("switch-empty", "switch")
	node, _ := NewSwitchNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSwitchNode_Process_첫번째라우트매칭 은 첫 번째 매칭 라우트의 포트로 라우팅하는지 확인한다.
func TestSwitchNode_Process_첫번째라우트매칭(t *testing.T) {
	def := flow.NewNodeDef("switch-match", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-a",
		},
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-b",
		},
	}

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "port-a", target) // 첫 번째 매칭
}

// TestSwitchNode_Process_두번째라우트매칭 은 첫 번째가 실패하고 두 번째가 매칭되는 경우를 확인한다.
func TestSwitchNode_Process_두번째라우트매칭(t *testing.T) {
	def := flow.NewNodeDef("switch-second", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return false },
			TargetPort: "port-a",
		},
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "port-b",
		},
	}

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "port-b", target)
}

// TestSwitchNode_Process_기본포트사용 은 모든 라우트 실패 시 기본 포트를 사용하는지 확인한다.
func TestSwitchNode_Process_기본포트사용(t *testing.T) {
	def := flow.NewNodeDef("switch-default", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return false },
			TargetPort: "port-a",
		},
	}
	sn.defaultPort = "fallback"

	msg := message.New()
	results, err := sn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "fallback", target)
}

// TestSwitchNode_Process_메타데이터기반라우팅 은 메시지 메타데이터로 라우팅할 수 있는지 확인한다.
func TestSwitchNode_Process_메타데이터기반라우팅(t *testing.T) {
	def := flow.NewNodeDef("switch-meta", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition: func(msg message.Message) bool {
				v, ok := msg.Metadata().Get("priority")
				return ok && v == "high"
			},
			TargetPort: "urgent",
		},
		{
			Condition: func(msg message.Message) bool {
				v, ok := msg.Metadata().Get("priority")
				return ok && v == "low"
			},
			TargetPort: "normal",
		},
	}

	tests := []struct {
		name       string
		priority   string
		targetPort string
	}{
		{"높은 우선순위", "high", "urgent"},
		{"낮은 우선순위", "low", "normal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithMetadata("priority", tt.priority))
			results, err := sn.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			target, ok := results[0].Metadata().Get("_target_port")
			assert.True(t, ok)
			assert.Equal(t, tt.targetPort, target)
		})
	}
}

// TestSwitchNode_Process_복제된메시지반환 은 반환된 메시지가 원본과 다른 인스턴스인지 확인한다.
func TestSwitchNode_Process_복제된메시지반환(t *testing.T) {
	def := flow.NewNodeDef("switch-clone", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "out",
		},
	}

	msg := message.New()
	results, _ := sn.Process(context.Background(), msg)
	require.Len(t, results, 1)

	// Clone이므로 ID가 달라야 한다
	assert.NotEqual(t, msg.ID(), results[0].ID())
}

// --- Configure 테스트 ---

// TestSwitchNode_Configure_라우트설정 은 Configure로 라우트를 설정할 수 있는지 확인한다.
func TestSwitchNode_Configure_라우트설정(t *testing.T) {
	def := flow.NewNodeDef("switch-cfg", "switch")
	node, _ := NewSwitchNode(def)

	routes := []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "configured-port",
		},
	}
	err := node.Configure(map[string]any{
		"routes":       routes,
		"default_port": "default-out",
	})
	require.NoError(t, err)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	target, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "configured-port", target)
}

// --- Shutdown 테스트 ---

// TestSwitchNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestSwitchNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("switch-shut", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	_ = sn.Init(context.Background())
	err := sn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, sn.CurrentState())
}

// --- 동시성 테스트 ---

// TestSwitchNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestSwitchNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("switch-conc", "switch")
	node, _ := NewSwitchNode(def)
	sn := node.(*SwitchNode)

	sn.routes = []SwitchRoute{
		{
			Condition:  func(msg message.Message) bool { return true },
			TargetPort: "out",
		},
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New()
			_, _ = sn.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// =============================================================================
// SPEC-SWITCH-001: 문자열 조건 라우트 / match_mode / default_port / 동적 포트
// =============================================================================

// tempMsg 는 payload.temp 값을 가진 메시지를 생성하는 헬퍼이다.
func tempMsg(temp float64) message.Message {
	return message.New(message.WithPayload(message.NewPayload(map[string]any{"temp": temp})))
}

// targetPort 는 메시지의 _target_port 메타데이터를 반환하는 헬퍼이다.
func targetPort(t *testing.T, msg message.Message) string {
	t.Helper()
	v, ok := msg.Metadata().Get("_target_port")
	require.True(t, ok, "_target_port 메타데이터가 설정되어야 한다")
	return v
}

// targetPortSet 은 메시지 슬라이스의 _target_port 집합을 반환하는 헬퍼이다.
func targetPortSet(t *testing.T, msgs []message.Message) map[string]bool {
	t.Helper()
	set := make(map[string]bool, len(msgs))
	for _, m := range msgs {
		set[targetPort(t, m)] = true
	}
	return set
}

// --- AC-SWITCH-001: 문자열 조건 라우트 파싱 및 라우팅 ---

// TestSwitchNode_Configure_문자열조건라우트 는 {name, condition} 맵 라우트가
// 컴파일되어 올바른 포트로 라우팅되는지 확인한다.
func TestSwitchNode_Configure_문자열조건라우트(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		temp       float64
		wantLen    int
		wantTarget string
	}{
		{"hot 매칭", 35, 1, "hot"},
		{"cold 매칭", 5, 1, "cold"},
		{"미매칭 드롭", 15, 0, ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := flow.NewNodeDef("switch-str", "switch")
			node, _ := NewSwitchNode(def)
			err := node.Configure(map[string]any{
				"routes": []any{
					map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
					map[string]any{"name": "cold", "condition": "$.payload.temp < 10"},
				},
			})
			require.NoError(t, err)

			results, err := node.Process(context.Background(), tempMsg(tt.temp))
			require.NoError(t, err)
			require.Len(t, results, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantTarget, targetPort(t, results[0]))
			}
		})
	}
}

// --- AC-SWITCH-002: 잘못된 표현식 → Configure 에러 ---

// TestSwitchNode_Configure_잘못된표현식_에러 는 유효하지 않은 표현식이
// 실패한 라우트를 식별하는 에러를 반환하는지 확인한다.
func TestSwitchNode_Configure_잘못된표현식_에러(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-bad-expr", "switch")
	node, _ := NewSwitchNode(def)
	err := node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "bad", "condition": "$.payload.x >>> 1"},
		},
	})
	require.Error(t, err)
	// 에러 메시지가 실패한 라우트("bad" 또는 인덱스 0)를 식별할 수 있어야 한다.
	assert.Contains(t, err.Error(), "bad")
}

// --- AC-SWITCH-003: 라우트 필드 검증 ---

// TestSwitchNode_Configure_필드검증 는 빈 name/condition, 비문자열 타입에 대해
// Configure가 에러를 반환하는지 확인한다.
func TestSwitchNode_Configure_필드검증(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		route map[string]any
	}{
		{"빈 name", map[string]any{"name": "", "condition": "$.payload.x == 1"}},
		{"빈 condition", map[string]any{"name": "ok", "condition": ""}},
		{"비문자열 condition", map[string]any{"name": "ok", "condition": 42}},
		{"name 누락", map[string]any{"condition": "$.payload.x == 1"}},
		{"비문자열 name", map[string]any{"name": 7, "condition": "$.payload.x == 1"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := flow.NewNodeDef("switch-validate", "switch")
			node, _ := NewSwitchNode(def)
			err := node.Configure(map[string]any{
				"routes": []any{tt.route},
			})
			require.Error(t, err)
		})
	}
}

// TestSwitchNode_Configure_에러시기존설정보존 는 routes 파싱 에러 발생 시
// 기존 설정이 변경되지 않는지(원자성) 확인한다.
func TestSwitchNode_Configure_에러시기존설정보존(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-atomic", "switch")
	node, _ := NewSwitchNode(def)

	// 유효한 라우트로 1차 설정.
	require.NoError(t, node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
		},
	}))

	// 잘못된 라우트로 2차 설정 시도 → 에러.
	err := node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "broken", "condition": "$.payload.x >>> 1"},
		},
	})
	require.Error(t, err)

	// 기존 라우트("hot")가 보존되어야 한다.
	results, err := node.Process(context.Background(), tempMsg(35))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "hot", targetPort(t, results[0]))
}

// --- AC-SWITCH-004: 기존 Go 함수 라우트 보존 (하위 호환) ---

// TestSwitchNode_Configure_Go함수라우트보존 는 []SwitchRoute 클로저 라우트가
// 그대로 동작하는지 확인한다.
func TestSwitchNode_Configure_Go함수라우트보존(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-closure", "switch")
	node, _ := NewSwitchNode(def)
	err := node.Configure(map[string]any{
		"routes": []SwitchRoute{
			{Condition: func(m message.Message) bool { return true }, TargetPort: "a"},
		},
	})
	require.NoError(t, err)

	results, err := node.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "a", targetPort(t, results[0]))
}

// --- AC-SWITCH-010 / AC-SWITCH-011: first 모드 (기본 + 첫 매칭) ---

// TestSwitchNode_Process_first모드_첫매칭만 는 match_mode 미설정/first에서
// 여러 라우트가 매칭되어도 첫 번째 포트로만 라우팅하는지 확인한다.
func TestSwitchNode_Process_first모드_첫매칭만(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		matchMode any // nil = 미설정
	}{
		{"match_mode 미설정 (기본 first)", nil},
		{"match_mode first 명시", "first"},
		{"match_mode 미인식 값 (first 폴백)", "bogus"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := map[string]any{
				"routes": []any{
					map[string]any{"name": "warm", "condition": "$.payload.temp >= 20"},
					map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
				},
			}
			if tt.matchMode != nil {
				cfg["match_mode"] = tt.matchMode
			}

			def := flow.NewNodeDef("switch-first", "switch")
			node, _ := NewSwitchNode(def)
			require.NoError(t, node.Configure(cfg))

			results, err := node.Process(context.Background(), tempMsg(35))
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, "warm", targetPort(t, results[0])) // 첫 매칭
		})
	}
}

// --- AC-SWITCH-012: all 모드 팬아웃 ---

// TestSwitchNode_Process_all모드_팬아웃 은 match_mode=all에서 매칭되는 모든
// 라우트로 팬아웃하고 순서를 보존하는지 확인한다.
func TestSwitchNode_Process_all모드_팬아웃(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-all", "switch")
	node, _ := NewSwitchNode(def)
	require.NoError(t, node.Configure(map[string]any{
		"match_mode": "all",
		"routes": []any{
			map[string]any{"name": "warm", "condition": "$.payload.temp >= 20"},
			map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
		},
	}))

	src := tempMsg(35)
	results, err := node.Process(context.Background(), src)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// _target_port 집합 == {warm, hot}.
	assert.Equal(t, map[string]bool{"warm": true, "hot": true}, targetPortSet(t, results))
	// 순서 보존 (라우트 순서).
	assert.Equal(t, "warm", targetPort(t, results[0]))
	assert.Equal(t, "hot", targetPort(t, results[1]))
	// 각 출력은 원본의 Clone (ID가 원본과 다르다).
	for _, r := range results {
		assert.NotEqual(t, src.ID(), r.ID())
	}
}

// --- AC-SWITCH-020 / AC-SWITCH-021: default_port / 드롭 ---

// TestSwitchNode_Process_미매칭_default_드롭 은 미매칭 시 default_port 라우팅
// 또는 드롭 동작을 확인한다.
func TestSwitchNode_Process_미매칭_default_드롭(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		defaultPort any // nil = 미설정
		wantLen     int
		wantTarget  string
	}{
		{"default_port 설정 → 라우팅", "other", 1, "other"},
		{"default_port 미설정 → 드롭", nil, 0, ""},
		{"default_port 빈 문자열 → 드롭", "", 0, ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := map[string]any{
				"routes": []any{
					map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
				},
			}
			if tt.defaultPort != nil {
				cfg["default_port"] = tt.defaultPort
			}

			def := flow.NewNodeDef("switch-default", "switch")
			node, _ := NewSwitchNode(def)
			require.NoError(t, node.Configure(cfg))

			results, err := node.Process(context.Background(), tempMsg(5)) // 미매칭
			require.NoError(t, err)
			require.Len(t, results, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantTarget, targetPort(t, results[0]))
			}
		})
	}
}

// --- AC-SWITCH-022: all 모드 + 미매칭 → default/드롭 ---

// TestSwitchNode_Process_all모드_default상호작용 은 all 모드에서 미매칭 시
// default/드롭 규칙이 적용되고, 매칭이 있으면 default를 쓰지 않는지 확인한다.
func TestSwitchNode_Process_all모드_default상호작용(t *testing.T) {
	t.Parallel()

	t.Run("all + 0매칭 + default → default", func(t *testing.T) {
		t.Parallel()
		def := flow.NewNodeDef("switch-all-def", "switch")
		node, _ := NewSwitchNode(def)
		require.NoError(t, node.Configure(map[string]any{
			"match_mode":   "all",
			"default_port": "other",
			"routes": []any{
				map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
			},
		}))
		results, err := node.Process(context.Background(), tempMsg(5))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "other", targetPort(t, results[0]))
	})

	t.Run("all + 0매칭 + default 미설정 → 드롭", func(t *testing.T) {
		t.Parallel()
		def := flow.NewNodeDef("switch-all-drop", "switch")
		node, _ := NewSwitchNode(def)
		require.NoError(t, node.Configure(map[string]any{
			"match_mode": "all",
			"routes": []any{
				map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
			},
		}))
		results, err := node.Process(context.Background(), tempMsg(5))
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("all + 매칭 있음 → default 미사용", func(t *testing.T) {
		t.Parallel()
		def := flow.NewNodeDef("switch-all-match", "switch")
		node, _ := NewSwitchNode(def)
		require.NoError(t, node.Configure(map[string]any{
			"match_mode":   "all",
			"default_port": "other",
			"routes": []any{
				map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
			},
		}))
		results, err := node.Process(context.Background(), tempMsg(35))
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "hot", targetPort(t, results[0])) // default 미사용
	})
}

// --- AC-SWITCH-030 / 031 / 033: 동적 출력 포트 ---

// portNames 는 NodePort 슬라이스에서 (방향별) 이름 집합을 추출하는 헬퍼이다.
func portNames(ports []NodePort, dir flow.PortDirection) map[string]bool {
	set := make(map[string]bool)
	for _, p := range ports {
		if p.Direction == dir {
			set[p.Name] = true
		}
	}
	return set
}

// TestSwitchNode_Ports_라우트기반파생 은 routes + default_port로부터 출력 포트가
// 파생되는지 확인한다 (AC-SWITCH-030).
func TestSwitchNode_Ports_라우트기반파생(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-ports", "switch")
	node, _ := NewSwitchNode(def)
	require.NoError(t, node.Configure(map[string]any{
		"default_port": "other",
		"routes": []any{
			map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
			map[string]any{"name": "cold", "condition": "$.payload.temp < 10"},
		},
	}))

	ports := node.Ports()
	inputs := portNames(ports, flow.PortInput)
	outputs := portNames(ports, flow.PortOutput)

	assert.True(t, inputs["in"], "입력 포트 in 포함")
	assert.Equal(t, map[string]bool{"hot": true, "cold": true, "other": true}, outputs)
}

// TestSwitchNode_Ports_빈라우트폴백 은 routes가 비면 [in, out] 폴백 포트를
// 반환하는지 확인한다 (AC-SWITCH-031, REQ-SWITCH-050).
func TestSwitchNode_Ports_빈라우트폴백(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-empty-ports", "switch")
	node, _ := NewSwitchNode(def)

	ports := node.Ports()
	inputs := portNames(ports, flow.PortInput)
	outputs := portNames(ports, flow.PortOutput)

	assert.True(t, inputs["in"], "입력 포트 in 포함")
	assert.True(t, outputs["out"], "출력 포트 out 폴백 포함")
}

// TestSwitchNode_Ports_중복제거 는 라우트 이름 중복 및 route.name == default_port
// 인 경우 출력 포트가 중복 제거되는지 확인한다.
func TestSwitchNode_Ports_중복제거(t *testing.T) {
	t.Parallel()

	t.Run("라우트 이름 중복 제거", func(t *testing.T) {
		t.Parallel()
		def := flow.NewNodeDef("switch-dedup-route", "switch")
		node, _ := NewSwitchNode(def)
		require.NoError(t, node.Configure(map[string]any{
			"routes": []any{
				map[string]any{"name": "dup", "condition": "$.payload.temp >= 30"},
				map[string]any{"name": "dup", "condition": "$.payload.temp < 10"},
			},
		}))
		outputs := portNames(node.Ports(), flow.PortOutput)
		assert.Equal(t, map[string]bool{"dup": true}, outputs)
	})

	t.Run("route.name == default_port 중복 제거", func(t *testing.T) {
		t.Parallel()
		def := flow.NewNodeDef("switch-dedup-def", "switch")
		node, _ := NewSwitchNode(def)
		require.NoError(t, node.Configure(map[string]any{
			"default_port": "hot",
			"routes": []any{
				map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
			},
		}))
		outputs := portNames(node.Ports(), flow.PortOutput)
		assert.Equal(t, map[string]bool{"hot": true}, outputs)
	})
}

// TestSwitchNode_Ports_재Configure반영 은 재Configure 후 Ports가 갱신된 라우트를
// 반영하는지 확인한다 (AC-SWITCH-033).
func TestSwitchNode_Ports_재Configure반영(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-reconfig", "switch")
	node, _ := NewSwitchNode(def)
	require.NoError(t, node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "a", "condition": "$.payload.temp >= 30"},
		},
	}))
	assert.Equal(t, map[string]bool{"a": true}, portNames(node.Ports(), flow.PortOutput))

	// 재Configure로 라우트 추가.
	require.NoError(t, node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "a", "condition": "$.payload.temp >= 30"},
			map[string]any{"name": "b", "condition": "$.payload.temp < 10"},
		},
	}))
	assert.Equal(t, map[string]bool{"a": true, "b": true}, portNames(node.Ports(), flow.PortOutput))
}

// --- 동시성: Process + Configure + Ports 동시 호출 ---

// TestSwitchNode_동시성안전_Configure_Process_Ports 는 Configure/Process/Ports를
// 동시에 호출해도 레이스가 없는지 확인한다 (-race).
func TestSwitchNode_동시성안전_Configure_Process_Ports(t *testing.T) {
	t.Parallel()

	def := flow.NewNodeDef("switch-conc-all", "switch")
	node, _ := NewSwitchNode(def)
	require.NoError(t, node.Configure(map[string]any{
		"routes": []any{
			map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
		},
	}))

	done := make(chan struct{})
	const workers = 8
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				switch idx % 3 {
				case 0:
					_, _ = node.Process(context.Background(), tempMsg(35))
				case 1:
					_ = node.Ports()
				default:
					_ = node.Configure(map[string]any{
						"routes": []any{
							map[string]any{"name": "hot", "condition": "$.payload.temp >= 30"},
						},
					})
				}
			}
		}(i)
	}
	for i := 0; i < workers; i++ {
		<-done
	}
}
