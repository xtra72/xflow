// lg_hvacr02_seek_test.go — 노드 (재)시작 시 ring 백로그 재방출 방지 회귀 테스트.
//
// 증상: 노드가 Init/Reinit 될 때마다 ring 버퍼(최근 64프레임)를 통째로 재 drain·
// 재발행하여, 같은 report 가 수십 개 동시에 다운스트림으로 흘러나갔다.
// fix: 첫 진입은 seekToLatestSeq 로 최신 seq 만 잡고 백로그는 재방출하지 않는다.
package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// seekToLatestSeq 는 백로그를 재방출하지 않고 lastSeq 만 현재 최신으로 전진시킨다.
func TestLGHvacr02StatusNode_SeekToLatest_NoReplay(t *testing.T) {
	mock := &mockLGHvacr02Agent{
		processResp: []byte(`{"count":3,"frames":[` +
			`{"seq":3,"unit_id":"44550066","trigger":"report","state":{"power":false}},` +
			`{"seq":2,"unit_id":"44550066","trigger":"report","state":{"power":false}},` +
			`{"seq":1,"unit_id":"44550066","trigger":"report","state":{"power":false}}` +
			`],"last_seq":3}`),
	}
	n := newTestLGHvacr02StatusNode(mock)

	n.seekToLatestSeq()

	assert.Equal(t, int64(3), n.lastSeq, "seek 은 현재 최신 seq 로 전진해야 한다")
	assert.Equal(t, 0, len(n.sourceCh), "seek 은 ring 백로그를 재방출하면 안 된다")
}

// 대조: drainNewFrames(lastSeq=0)는 백로그를 방출한다(기존 동작). seek 가 이를
// 대체함을 명확히 하기 위한 대조 테스트.
func TestLGHvacr02StatusNode_Drain_EmitsBacklog(t *testing.T) {
	mock := &mockLGHvacr02Agent{
		processResp: []byte(`{"count":2,"frames":[` +
			`{"seq":2,"unit_id":"44550066","trigger":"report","state":{"power":false}},` +
			`{"seq":1,"unit_id":"44550066","trigger":"report","state":{"power":false}}` +
			`],"last_seq":2}`),
	}
	n := newTestLGHvacr02StatusNode(mock)

	n.drainNewFrames(n.lgHvacr02Cfg)

	assert.Equal(t, int64(2), n.lastSeq)
	assert.Equal(t, 2, len(n.sourceCh), "drain 은 백로그 프레임을 방출한다(seek 가 이를 회피)")
}
