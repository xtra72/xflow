package node

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// InfluxDBReadNode 는 InfluxDB에서 주기적으로 데이터를 조회하는 SourceNode이다.
// 설정된 쿼리를 pollInterval 간격으로 실행하고, 각 결과 행을 개별 메시지로 sourceCh에 전달한다.
type InfluxDBReadNode struct {
	*BaseNode
	agent        influxdbMessageReceiver
	resolver     AgentResolver
	agentRef     *flow.AgentRef
	sourceCh     chan message.Message
	stopCh       chan struct{}
	query        string        // InfluxDB 쿼리 문자열
	language     string        // 쿼리 언어 (flux, influxql 등)
	pollInterval time.Duration // 폴링 간격
	timeout      time.Duration // 쿼리 타임아웃
	outputMode   string        // "rows" (행별 개별 메시지) 또는 "batch" (전체 결과 단일 메시지)
	logger       *slog.Logger
}

var (
	_ Node       = (*InfluxDBReadNode)(nil)
	_ SourceNode = (*InfluxDBReadNode)(nil)
)

// NewInfluxDBReadNode 는 새로운 InfluxDBReadNode를 생성하는 팩토리 함수이다.
func NewInfluxDBReadNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &InfluxDBReadNode{
		BaseNode:     base,
		agentRef:     def.AgentRef,
		sourceCh:     make(chan message.Message, 64),
		stopCh:       make(chan struct{}),
		language:     "flux",
		pollInterval: 30 * time.Second,
		timeout:      10 * time.Second,
		outputMode:   "rows",
		logger:       slog.Default(),
	}
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 InfluxDBReadNode를 초기화하고 폴링 루프를 시작한다.
func (n *InfluxDBReadNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.resolveInfluxDBReceiver(ctx); err != nil {
		return fmt.Errorf("influxdb-read init: %w", err)
	}

	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveInfluxDBReceiver 는 AgentResolver를 사용하여 InfluxDB 에이전트(MessageReceiver)를 찾는다.
func (n *InfluxDBReadNode) resolveInfluxDBReceiver(ctx context.Context) error {
	// config["_influxdb_agent"]로 직접 주입된 경우 (테스트용)
	if a, ok := n.config["_influxdb_agent"]; ok {
		if agent, ok := a.(influxdbMessageReceiver); ok {
			n.agent = agent
			return nil
		}
	}

	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for influxdb-read node")
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

// Shutdown 은 폴링 루프를 중지하고 노드를 종료한다.
func (n *InfluxDBReadNode) Shutdown(_ context.Context) error {
	close(n.stopCh)
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// SourceCh 는 폴링으로 생성된 메시지 채널을 반환한다.
func (n *InfluxDBReadNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Configure 는 InfluxDBReadNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "query": string - InfluxDB 쿼리 문자열
//   - "language": string - 쿼리 언어 (기본값: "flux")
//   - "poll_interval": string - 폴링 간격 (예: "30s", "1m")
//   - "timeout": string - 쿼리 타임아웃 (예: "10s")
func (n *InfluxDBReadNode) Configure(config map[string]any) error {
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

	if v, ok := config["poll_interval"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("influxdb-read: invalid poll_interval %q: %w", s, err)
			}
			n.pollInterval = d
		}
	}

	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("influxdb-read: invalid timeout %q: %w", s, err)
			}
			n.timeout = d
		}
	}

	if v, ok := config["output_mode"]; ok {
		if s, ok := v.(string); ok {
			if s == "batch" || s == "rows" {
				n.outputMode = s
			}
		}
	}

	return nil
}

// Process 는 InfluxDBReadNode에서 사용되지 않는다 (SourceNode이므로).
func (n *InfluxDBReadNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// pollLoop 는 주기적으로 InfluxDB 쿼리를 실행하고 결과를 sourceCh에 전달한다.
func (n *InfluxDBReadNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.pollOnce()
		}
	}
}

// pollOnce 는 한 번의 쿼리를 실행하고 결과를 sourceCh에 전달한다.
func (n *InfluxDBReadNode) pollOnce() {
	if n.query == "" {
		return
	}

	// 쿼리 요청 구성
	qr := struct {
		Query    string `json:"query"`
		Language string `json:"language,omitempty"`
	}{
		Query:    n.query,
		Language: n.language,
	}

	queryJSON, err := json.Marshal(qr)
	if err != nil {
		n.logger.Error("influxdb-read: marshal query error", "error", err)
		return
	}

	// 쿼리 실행 (agent.Process로 쿼리를 트리거)
	if _, err := n.agent.Process(queryJSON); err != nil {
		n.logger.Error("influxdb-read: query trigger error", "error", err)
		return
	}

	// 결과 수신 (타임아웃 적용)
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	resultData, err := n.agent.ReceiveMessage(ctx)
	if err != nil {
		n.logger.Error("influxdb-read: receive result error", "error", err)
		return
	}

	// 결과 파싱
	var rows []map[string]any
	if err := json.Unmarshal(resultData, &rows); err != nil {
		n.logger.Error("influxdb-read: unmarshal result error", "error", err)
		return
	}

	// 빈 결과는 건너뛴다
	if len(rows) == 0 {
		return
	}

	if n.outputMode == "batch" {
		// 전체 결과를 단일 메시지로 전달
		msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
			"results": rows,
			"count":   len(rows),
		})))
		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	} else {
		// 각 행을 개별 메시지로 전달
		for _, row := range rows {
			msg := message.New(message.WithPayload(message.NewPayload(row)))
			select {
			case n.sourceCh <- msg:
			case <-n.stopCh:
				return
			}
		}
	}
}
