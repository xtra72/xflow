package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/service"
	"github.com/xtra/xflow/internal/remote"
)

// remoteSubflowFetcher 가 service.RemoteFlowFetcher 를 만족하는지 컴파일 타임 + 런타임 검증.
func TestRemoteSubflowFetcher_인터페이스만족(t *testing.T) {
	var _ service.RemoteFlowFetcher = (*remoteSubflowFetcher)(nil)
}

// server 가 nil 이면 명확한 에러를 반환해야 한다(방어).
func TestRemoteSubflowFetcher_nil서버_에러(t *testing.T) {
	f := newRemoteSubflowFetcher(nil)
	_, err := f.FetchRemoteFlow(context.Background(), "node-1", "flow-x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server")
}

// 미관리/오프라인 노드로의 fetch 는 DispatchQuery 의 ErrNodeNotManaged(503)를 그대로
// 전파해야 한다(REQ-SUBFLOW-R06 실패 의미 표면화). 빈 Server 는 어떤 노드도 관리하지
// 않으므로 IsManaged=false → ErrNodeNotManaged 를 반환한다.
func TestRemoteSubflowFetcher_미관리노드_에러전파(t *testing.T) {
	srv := remote.NewServer(remote.ServerConfig{}, nil)
	f := newRemoteSubflowFetcher(srv)

	_, err := f.FetchRemoteFlow(context.Background(), "unmanaged-node", "flow-x")
	require.Error(t, err)
	assert.ErrorIs(t, err, remote.ErrNodeNotManaged, "미관리 노드는 ErrNodeNotManaged 를 전파해야 한다")
}
