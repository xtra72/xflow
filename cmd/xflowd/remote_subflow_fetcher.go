// remote_subflow_fetcher.go 는 SPEC-SUBFLOW-001 v1.2 그룹 R(원격 서브플로우 참조)의
// 배포 시 원격 플로우 정의 해석기(service.RemoteFlowFetcher) 구체 구현을 제공한다.
//
// service 패키지는 import cycle 을 피하기 위해 internal/remote 를 import 하지 않고
// RemoteFlowFetcher 인터페이스만 정의한다. 본 파일은 remote.Server 와 service 를 모두
// 아는 와이어링 계층(cmd/xflowd)에서 그 인터페이스를 구현하여 주입한다.
//
// 동작: Server.DispatchQuery(ctx, instanceID, "flow", "get", {"id": flowID}) 를 호출하여
// redacted 원격 플로우 정의(handler.FlowInfo 마샬)를 반환한다(SPEC-REMOTE-001 REQ-J01/J04).
// 게이팅(승인∧온라인∧노출)·redaction·실패 의미(503/504/502)는 DispatchQuery 가 그대로
// 표면화하며, service 계층이 이를 배포 거부로 매핑한다(REQ-SUBFLOW-R05/R06).
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/api/service"
	"github.com/xtra/xflow/internal/remote"
)

// remoteSubflowFetcher 는 service.RemoteFlowFetcher 를 *remote.Server 위에 구현한다.
type remoteSubflowFetcher struct {
	server *remote.Server
}

// newRemoteSubflowFetcher 는 server 모드에서 원격 서브플로우 해석기를 만든다.
func newRemoteSubflowFetcher(server *remote.Server) *remoteSubflowFetcher {
	return &remoteSubflowFetcher{server: server}
}

// FetchRemoteFlow 는 원격 노드(instanceID)의 플로우(flowID) 정의를 query 프록시
// flow/get 으로 가져온다(항상 최신, READ-ONLY). flow/get 의 args 키는 {"id": flowID} 이다
// (cmd/xflowd/remote_query.go queryID/queryArgs 와 일치).
//
// 반환 바이트는 query_result 본문(= handler.FlowInfo 마샬, redacted)이다.
// service.deserializeRemoteFlow 가 이를 flow.Flow 로 역직렬화한다.
func (f *remoteSubflowFetcher) FetchRemoteFlow(ctx context.Context, instanceID, flowID string) ([]byte, error) {
	if f.server == nil {
		return nil, fmt.Errorf("remote subflow fetcher: server 미구성")
	}
	args, err := json.Marshal(map[string]string{"id": flowID})
	if err != nil {
		return nil, fmt.Errorf("remote subflow fetcher: encode args: %w", err)
	}
	data, err := f.server.DispatchQuery(ctx, instanceID, remote.DomainFlow, remote.QueryActionGet, args)
	if err != nil {
		// DispatchQuery 의 실패 의미(ErrNodeNotManaged/ErrNoConn 503·ErrQueryTimeout 504·
		// ErrQueryFailed 502)를 그대로 전파한다 — service 계층이 배포 거부로 매핑한다.
		return nil, err
	}
	return data, nil
}

var _ service.RemoteFlowFetcher = (*remoteSubflowFetcher)(nil)
