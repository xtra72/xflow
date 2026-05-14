package node

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/tsdb"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// TSDBQueryNode 는 TSDB에서 데이터를 조회하는 노드이다.
// 설정 기반으로 쿼리를 구성하고, 결과를 새 메시지의 payload에 담아 반환한다.
//
// TSDB 인스턴스는 agent_ref로 지정된 TSDB 에이전트에서 가져온다.
// Init 시 AgentResolver를 통해 에이전트를 찾고, tsdbProvider 인터페이스로
// TSDB 인스턴스에 접근한다.
type TSDBQueryNode struct {
	*BaseNode
	db             tsdb.TSDB
	resolver       AgentResolver
	agentRef       *flow.AgentRef
	measurement    string
	tagFilters     map[string]string
	seriesKey      string
	timeRange      time.Duration // 현재 시간으로부터 과거 시간 범위
	aggregation    string        // 집계 함수 (빈 문자열이면 raw)
	field          string        // 집계 대상 필드
	bucketInterval time.Duration // 다운샘플링 간격
	limit          int           // 최대 포인트 수
}

// NewTSDBQueryNode 는 새로운 TSDBQueryNode를 생성하는 팩토리 함수이다.
func NewTSDBQueryNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TSDBQueryNode{
		BaseNode: base,
		agentRef: def.AgentRef,
	}
	// WithAgentResolver 옵션으로 주입된 resolver를 필드에 저장
	if r, ok := base.config["_agent_resolver"]; ok {
		if resolver, ok := r.(AgentResolver); ok {
			n.resolver = resolver
		}
	}
	return n, nil
}

// Init 은 TSDBQueryNode를 초기화하고 AgentResolver로 TSDB 에이전트를 해석한다.
func (n *TSDBQueryNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver를 통해 TSDB 에이전트 해석
	if err := n.resolveTSDB(ctx); err != nil {
		return fmt.Errorf("tsdb-query init: %w", err)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// resolveTSDB 는 AgentResolver를 사용하여 TSDB 에이전트를 찾고 TSDB 인스턴스를 추출한다.
func (n *TSDBQueryNode) resolveTSDB(ctx context.Context) error {
	// config["_tsdb"]로 직접 주입된 경우 (테스트용)
	if db, ok := n.config["_tsdb"]; ok {
		if tsdbInst, ok := db.(tsdb.TSDB); ok {
			n.db = tsdbInst
			return nil
		}
	}

	// AgentRef가 없으면 에러
	if n.agentRef == nil {
		return fmt.Errorf("agent_ref is required for tsdb-query node")
	}

	// resolver 확인
	if n.resolver == nil {
		return fmt.Errorf("agent resolver not configured")
	}

	// 에이전트 해석
	transport, err := n.resolver.ResolveAgent(ctx, *n.agentRef)
	if err != nil {
		return fmt.Errorf("failed to resolve tsdb agent %q: %w", n.agentRef.AgentName, err)
	}

	// AgentAccessor로 원본 에이전트 추출
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return fmt.Errorf("tsdb agent transport does not support AgentAccessor")
	}

	// tsdbProvider 인터페이스로 TSDB 인스턴스 추출
	provider, ok := accessor.UnderlyingAgent().(tsdbProvider)
	if !ok {
		return fmt.Errorf("agent %q does not implement tsdbProvider", n.agentRef.AgentName)
	}

	n.db = provider.TSDB()
	if n.db == nil {
		return fmt.Errorf("tsdb agent %q returned nil TSDB instance", n.agentRef.AgentName)
	}

	return nil
}

// Shutdown 은 TSDBQueryNode를 종료한다.
func (n *TSDBQueryNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (n *TSDBQueryNode) AgentRef() flow.AgentRef {
	if n.agentRef == nil {
		return flow.AgentRef{}
	}
	return *n.agentRef
}

// Reinit 은 에이전트 재시작 후 TSDB 인스턴스를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *TSDBQueryNode) Reinit(ctx context.Context) error {
	return n.resolveTSDB(ctx)
}

// Configure 는 TSDBQueryNode의 설정을 적용한다.
//
// 지원하는 설정 키:
//   - "measurement": string
//   - "tags": map[string]any - 태그 필터
//   - "series_key": string - 직접 시리즈 키 지정
//   - "time_range": string - 시간 범위 (예: "1h", "30m", "24h")
//   - "aggregation": string - 집계 함수 ("min", "max", "avg", "sum", "count", "first", "last")
//   - "field": string - 집계 대상 필드
//   - "bucket": string - 다운샘플링 간격 (예: "5m", "1h")
//   - "limit": int/float64 - 최대 포인트 수
func (n *TSDBQueryNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// measurement 설정
	if v, ok := config["measurement"]; ok {
		if s, ok := v.(string); ok {
			n.measurement = s
		}
	}

	// tags 설정
	if v, ok := config["tags"]; ok {
		if m, ok := v.(map[string]any); ok {
			n.tagFilters = make(map[string]string, len(m))
			for k, val := range m {
				if s, ok := val.(string); ok {
					n.tagFilters[k] = s
				}
			}
		}
	}

	// series_key 설정
	if v, ok := config["series_key"]; ok {
		if s, ok := v.(string); ok {
			n.seriesKey = s
		}
	}

	// time_range 설정 (파싱: "1h", "30m", "24h" 등)
	if v, ok := config["time_range"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("tsdb-query: invalid time_range %q: %w", s, err)
			}
			n.timeRange = d
		}
	}

	// aggregation 설정
	if v, ok := config["aggregation"]; ok {
		if s, ok := v.(string); ok {
			n.aggregation = s
		}
	}

	// field 설정
	if v, ok := config["field"]; ok {
		if s, ok := v.(string); ok {
			n.field = s
		}
	}

	// bucket 설정 (다운샘플링 간격)
	if v, ok := config["bucket"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("tsdb-query: invalid bucket %q: %w", s, err)
			}
			n.bucketInterval = d
		}
	}

	// limit 설정
	if v, ok := config["limit"]; ok {
		switch val := v.(type) {
		case int:
			n.limit = val
		case float64:
			n.limit = int(val)
		}
	}

	return nil
}

// Process 는 TSDB에서 데이터를 조회하고, 결과를 새 메시지의 payload에 담아 반환한다.
func (n *TSDBQueryNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.db == nil {
		return nil, ErrTSDBNotConfigured
	}

	// 쿼리 구성
	now := time.Now()
	q := tsdb.Query{
		SeriesKey:      n.seriesKey,
		Measurement:    n.measurement,
		TagFilters:     n.tagFilters,
		Field:          n.field,
		BucketInterval: n.bucketInterval,
		Limit:          n.limit,
	}

	// aggregation 설정
	if n.aggregation != "" {
		q.Aggregation = tsdb.AggregateFunc(n.aggregation)
	}

	// 시간 범위 설정
	if n.timeRange > 0 {
		q.Start = now.Add(-n.timeRange)
		q.End = now
	}

	// 쿼리 실행
	results, err := n.db.Execute(q)
	if err != nil {
		return nil, fmt.Errorf("tsdb-query: %w", err)
	}

	// 결과를 직렬화 가능한 형태로 변환
	serializedResults := make([]map[string]any, 0, len(results))
	for _, r := range results {
		points := make([]map[string]any, 0, len(r.Points))
		for _, p := range r.Points {
			point := map[string]any{
				"timestamp": p.Timestamp.UnixMilli(),
				"fields":    p.Fields,
			}
			points = append(points, point)
		}
		serializedResults = append(serializedResults, map[string]any{
			"series_key": r.SeriesKey,
			"points":     points,
			"stats": map[string]any{
				"scanned_points":  r.Stats.ScannedPoints,
				"returned_points": r.Stats.ReturnedPoints,
				"execution_time":  r.Stats.ExecutionTime.String(),
			},
		})
	}

	// 결과 메시지 생성
	resultPayload := message.NewPayload(map[string]any{
		"results": serializedResults,
		"query": map[string]any{
			"measurement": n.measurement,
			"series_key":  n.seriesKey,
			"aggregation": n.aggregation,
			"field":       n.field,
			"time_range":  n.timeRange.String(),
		},
	})

	resultMsg := message.New(
		message.WithPayload(resultPayload),
	)
	resultMsg.Metadata().Set("message_type", "response")

	return []message.Message{resultMsg}, nil
}
