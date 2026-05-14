package node

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// influxdbVarPattern 은 쿼리 내 $variable 패턴을 매칭하는 정규식이다.
var influxdbVarPattern = regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)

// InfluxDBQueryNode 는 입력 메시지 기반으로 InfluxDB 쿼리를 실행하는 노드이다.
// 설정된 쿼리 또는 입력 메시지의 payload에서 쿼리를 가져와 실행하고,
// 결과를 새 메시지의 payload에 담아 반환한다.
type InfluxDBQueryNode struct {
	*BaseNode
	agent     influxdbMessageReceiver
	resolver  AgentResolver
	agentRef  *flow.AgentRef
	query     string        // 설정 기반 쿼리 (빈 문자열이면 payload에서 추출)
	language  string        // 쿼리 언어 (기본값: "flux")
	timeout   time.Duration // 쿼리 타임아웃 (기본값: 10s)
	resultKey string        // 결과를 저장할 payload 키 (기본값: "results")
}

// NewInfluxDBQueryNode 는 새로운 InfluxDBQueryNode를 생성하는 팩토리 함수이다.
func NewInfluxDBQueryNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &InfluxDBQueryNode{
		BaseNode:  base,
		agentRef:  def.AgentRef,
		language:  "flux",
		timeout:   10 * time.Second,
		resultKey: "results",
	}
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 InfluxDBQueryNode를 초기화하고 AgentResolver로 InfluxDB 에이전트를 해석한다.
func (n *InfluxDBQueryNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.resolveInfluxDBReceiver(ctx); err != nil {
		return fmt.Errorf("influxdb-query init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveInfluxDBReceiver 는 AgentResolver를 사용하여 InfluxDB 에이전트(MessageReceiver)를 찾는다.
func (n *InfluxDBQueryNode) resolveInfluxDBReceiver(ctx context.Context) error {
	// config["_influxdb_agent"]로 직접 주입된 경우 (테스트용)
	if a, ok := n.config["_influxdb_agent"]; ok {
		if agent, ok := a.(influxdbMessageReceiver); ok {
			n.agent = agent
			return nil
		}
	}

	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for influxdb-query node")
	}

	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve influxdb agent %q: %w", n.agentRef.AgentName, err)
	}

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("influxdb agent transport does not support AgentAccessor")
	}

	agent, ok := accessor.UnderlyingAgent().(influxdbMessageReceiver)
	if !ok {
		return fmt.Errorf("agent %q does not implement influxdbMessageReceiver", n.agentRef.AgentName)
	}

	n.agent = agent
	return nil
}

// Shutdown 은 InfluxDBQueryNode를 종료한다.
func (n *InfluxDBQueryNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (n *InfluxDBQueryNode) AgentRef() flow.AgentRef {
	if n.agentRef == nil {
		return flow.AgentRef{}
	}
	return *n.agentRef
}

// Reinit 은 에이전트 재시작 후 agent 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *InfluxDBQueryNode) Reinit(ctx context.Context) error {
	return n.resolveInfluxDBReceiver(ctx)
}

// Configure 는 InfluxDBQueryNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "query": string - InfluxDB 쿼리 문자열 ($variable 치환 지원)
//   - "language": string - 쿼리 언어 (기본값: "flux")
//   - "timeout": string - 쿼리 타임아웃 (예: "10s")
//   - "result_key": string - 결과를 저장할 payload 키 (기본값: "results")
func (n *InfluxDBQueryNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	if v, ok := config["query"]; ok {
		if s, ok := v.(string); ok {
			n.query = s
		}
	}

	if v, ok := config["language"]; ok {
		if s, ok := v.(string); ok {
			n.language = s
		}
	}

	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("influxdb-query: invalid timeout %q: %w", s, err)
			}
			n.timeout = d
		}
	}

	if v, ok := config["result_key"]; ok {
		if s, ok := v.(string); ok {
			n.resultKey = s
		}
	}

	return nil
}

// Process 는 InfluxDB 쿼리를 실행하고, 결과를 새 메시지의 payload에 담아 반환한다.
func (n *InfluxDBQueryNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.agent == nil {
		return nil, fmt.Errorf("influxdb-query: agent not configured")
	}

	// 쿼리 결정: 설정 쿼리($variable 치환) 또는 payload["query"]
	query := n.query
	if query != "" {
		query = substituteInfluxDBVariables(query, msg.Payload())
	} else {
		// payload에서 쿼리 추출
		if v, ok := msg.Payload().Get("query"); ok {
			if s, ok := v.(string); ok {
				query = s
			}
		}
	}

	if query == "" {
		return nil, fmt.Errorf("influxdb-query: no query found (configure 'query' or provide in payload)")
	}

	// 쿼리 요청 구성
	qr := struct {
		Query    string `json:"query"`
		Language string `json:"language,omitempty"`
	}{
		Query:    query,
		Language: n.language,
	}

	queryJSON, err := json.Marshal(qr)
	if err != nil {
		return nil, fmt.Errorf("influxdb-query: marshal error: %w", err)
	}

	// 쿼리 실행
	if _, err := n.agent.Process(queryJSON); err != nil {
		return nil, fmt.Errorf("influxdb-query: %w", err)
	}

	// 결과 수신 (타임아웃 적용)
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	resultData, err := n.agent.ReceiveMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("influxdb-query: receive result: %w", err)
	}

	// 결과 파싱
	var results []map[string]any
	if err := json.Unmarshal(resultData, &results); err != nil {
		return nil, fmt.Errorf("influxdb-query: unmarshal result: %w", err)
	}

	// 결과 메시지 생성 (입력 메타데이터 보존)
	resultPayload := message.NewPayload(map[string]any{
		n.resultKey: results,
	})

	resultMsg := message.New(
		message.WithPayload(resultPayload),
	)
	resultMsg.Metadata().Set("message_type", "response")

	return []message.Message{resultMsg}, nil
}

// substituteInfluxDBVariables 는 쿼리 내 $variable 패턴을 payload 값으로 치환한다.
//
// 치환 규칙:
//   - 문자열 값: 작은따옴표로 감싼다 ('value')
//   - 숫자 값: 그대로 사용한다 (100, 22.5)
//   - 불리언 값: 문자열로 변환한다 (true, false)
//   - 누락된 변수: 원본 $variable을 유지한다
func substituteInfluxDBVariables(query string, payload message.Payload) string {
	return influxdbVarPattern.ReplaceAllStringFunc(query, func(match string) string {
		varName := match[1:] // $ 제거
		v, ok := payload.Get(varName)
		if !ok {
			return match // 누락된 변수는 원본 유지
		}

		switch val := v.(type) {
		case string:
			return "'" + val + "'"
		case int:
			return strconv.Itoa(val)
		case int64:
			return strconv.FormatInt(val, 10)
		case float64:
			return strconv.FormatFloat(val, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(val)
		default:
			return fmt.Sprintf("%v", val)
		}
	})
}
