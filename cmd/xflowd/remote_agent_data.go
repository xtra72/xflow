// remote_agent_data.go 는 M8(그룹 J) agent/store·agent/series query-action 의 노드 측
// read 어댑터를 정의한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.10.1, REQ-J04).
//
// store/series 데이터는 전용 HTTP 핸들러가 아니라 **시스템 에이전트**(store/tsdb)가
// 보유한다(StoreQueryHandler 가 AgentLookup 으로 에이전트를 찾아 type-assert 하는 것과
// 동일 경로). 본 파일의 어댑터는 동일한 agent.DefaultManager 인스턴스를 재사용해
// 에이전트를 이름으로 찾고, 로컬 핸들러가 호출하는 **동일 메서드**(StaticKeysSnapshot /
// TSDB().SeriesKeys)를 호출하여 로컬 API 와 IDENTICAL JSON 형상을 반환한다(A10 — 노드의
// 기존 로컬 read 재실행).
//
// READ-ONLY(REQ-J03): keys/series 조회만 수행한다(변경 없음). redaction(REQ-J06)은
// client 가 전송 전 QueryRedactor 로 적용한다.
package main

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/tsdb"
)

// remoteAgentDataErr 는 store/series 에이전트 조회 실패의 기반 오류이다(서버가 502
// node-error 로 매핑 — REQ-J07).
var remoteAgentDataErr = errors.New("remote agent data")

// agentLister 는 이름으로 에이전트를 찾기 위한 최소 인터페이스이다(*agent.DefaultManager
// 가 만족 — handler.AgentLookup 과 동형). 테스트에서는 경량 페이크로 대체 가능하다.
type agentLister interface {
	List() []agent.Agent
}

// storeKeysSnapshotter 는 store 시스템 에이전트가 제공하는 정적 키 메타 스냅샷 계약이다
// (system.StoreAgent 가 만족 — store_query.storeKeyMetaLister 와 동형). cmd 패키지에서
// handler 의 unexported 인터페이스를 재사용할 수 없어 동형 인터페이스를 로컬 선언한다.
type storeKeysSnapshotter interface {
	StaticKeysSnapshot() map[string]system.StaticKeyMeta
}

// tsdbProvider 는 TSDB 시스템 에이전트가 내부 TSDB 인스턴스를 노출하는 계약이다
// (system.TSDBAgent 가 만족). series 키 목록 조회에 사용된다.
type tsdbProvider interface {
	TSDB() tsdb.TSDB
}

// agentManagerStoreReader 는 agentLister 를 queryStoreReader 로 어댑트한다(agent/store).
// 이름으로 store 에이전트를 찾아 StaticKeysSnapshot 을 handler.StoreKeysListResponse 로
// 변환한다(GET /store/{agent}/keys 와 동일 형상).
type agentManagerStoreReader struct {
	agents agentLister
}

var _ queryStoreReader = (*agentManagerStoreReader)(nil)

// newAgentManagerStoreReader 는 agent 매니저 기반 store reader 를 생성한다.
func newAgentManagerStoreReader(agents agentLister) *agentManagerStoreReader {
	return &agentManagerStoreReader{agents: agents}
}

// StoreKeys 는 agentName 의 store 에이전트 키 스냅샷을 로컬 ListKeys 와 동일 형상으로
// 반환한다(REQ-J04). store 에이전트가 아니거나 미존재면 오류를 반환한다.
func (r *agentManagerStoreReader) StoreKeys(_ context.Context, agentName string) (*handler.StoreKeysListResponse, error) {
	ag := findAgentByNameLocal(r.agents, agentName)
	if ag == nil {
		return nil, fmt.Errorf("%w: store agent %q not found", remoteAgentDataErr, agentName)
	}
	snapshotter, ok := ag.(storeKeysSnapshotter)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not a store agent", remoteAgentDataErr, agentName)
	}
	return buildStoreKeysResponse(snapshotter.StaticKeysSnapshot()), nil
}

// buildStoreKeysResponse 는 StaticKeysSnapshot 을 handler.StoreKeysListResponse 로
// 변환한다(StoreQueryHandler.ListKeys 의 응답 빌드와 동일 — 알파벳순 정렬, Tags non-nil).
func buildStoreKeysResponse(snapshot map[string]system.StaticKeyMeta) *handler.StoreKeysListResponse {
	objects := make([]handler.StoreKeyResponse, 0, len(snapshot))
	for key, meta := range snapshot {
		tags := meta.Tags
		if tags == nil {
			tags = map[string]string{}
		}
		objects = append(objects, handler.StoreKeyResponse{
			Key:          key,
			Registration: string(meta.Source),
			DataType:     string(meta.DataType),
			MetricType:   meta.MetricType,
			Tags:         tags,
		})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return &handler.StoreKeysListResponse{Count: len(objects), Keys: objects}
}

// agentManagerSeriesReader 는 agentLister 를 querySeriesReader 로 어댑트한다(agent/series).
// 이름으로 tsdb 에이전트를 찾아 내부 TSDB 의 SeriesKeys 를 반환한다(GET /tsdb/series 의
// series 목록과 동일).
type agentManagerSeriesReader struct {
	agents agentLister
}

var _ querySeriesReader = (*agentManagerSeriesReader)(nil)

// newAgentManagerSeriesReader 는 agent 매니저 기반 series reader 를 생성한다.
func newAgentManagerSeriesReader(agents agentLister) *agentManagerSeriesReader {
	return &agentManagerSeriesReader{agents: agents}
}

// SeriesList 는 agentName 의 tsdb 에이전트가 보유한 시리즈 키 목록을 반환한다(REQ-J04).
// tsdb 에이전트가 아니거나 미존재면 오류를 반환한다.
func (r *agentManagerSeriesReader) SeriesList(_ context.Context, agentName string) ([]string, error) {
	ag := findAgentByNameLocal(r.agents, agentName)
	if ag == nil {
		return nil, fmt.Errorf("%w: tsdb agent %q not found", remoteAgentDataErr, agentName)
	}
	provider, ok := ag.(tsdbProvider)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not a tsdb agent", remoteAgentDataErr, agentName)
	}
	db := provider.TSDB()
	if db == nil {
		return []string{}, nil
	}
	return db.SeriesKeys(), nil
}

// findAgentByNameLocal 는 store_query.findAgentByName 과 동형(이름 매칭) 헬퍼이다.
// handler 의 unexported 헬퍼를 cmd 에서 재사용할 수 없어 동형으로 복제한다.
func findAgentByNameLocal(lister agentLister, name string) agent.Agent {
	if lister == nil {
		return nil
	}
	for _, a := range lister.List() {
		if a.Name() == name {
			return a
		}
	}
	return nil
}
