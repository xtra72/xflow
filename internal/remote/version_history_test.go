package remote

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
)

// memVerHistRepo 는 NodeVersionHistoryRepository 의 인메모리 테스트 구현이다.
type memVerHistRepo struct {
	mu      sync.Mutex
	entries []storage.NodeVersionHistory
}

func (m *memVerHistRepo) Append(_ context.Context, instanceID, version string, changedAtMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, storage.NodeVersionHistory{
		InstanceID: instanceID, Version: version, ChangedAt: changedAtMs,
	})
	return nil
}

func (m *memVerHistRepo) List(_ context.Context, instanceID string, limit int) ([]storage.NodeVersionHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []storage.NodeVersionHistory
	for i := len(m.entries) - 1; i >= 0; i-- { // 최신순
		if m.entries[i].InstanceID == instanceID {
			out = append(out, m.entries[i])
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memVerHistRepo) Close() error { return nil }

func (m *memVerHistRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func newVersionServer(verHist storage.NodeVersionHistoryRepository, audit storage.RemoteAuditRepository) *Server {
	return NewServer(ServerConfig{
		VersionHistory: verHist,
		Audit:          audit,
	}, nil)
}

func TestRecordVersionChange_FirstObservation(t *testing.T) {
	vh := &memVerHistRepo{}
	audit := newMemAuditRepo()
	s := newVersionServer(vh, audit)

	s.recordVersionChange(context.Background(), "node-1", "", "v1.0.0")

	hist, _ := vh.List(context.Background(), "node-1", 0)
	require.Len(t, hist, 1)
	assert.Equal(t, "v1.0.0", hist[0].Version)

	recs := audit.all()
	require.Len(t, recs, 1)
	assert.Equal(t, storage.AuditActionVersionUpdate, recs[0].Action)
	assert.Equal(t, "system", recs[0].Actor)
	assert.Contains(t, recs[0].Reason, "v1.0.0")
}

func TestRecordVersionChange_VersionBump(t *testing.T) {
	vh := &memVerHistRepo{}
	audit := newMemAuditRepo()
	s := newVersionServer(vh, audit)

	s.recordVersionChange(context.Background(), "node-1", "v1.0.0", "v1.2.0")

	hist, _ := vh.List(context.Background(), "node-1", 0)
	require.Len(t, hist, 1)
	assert.Equal(t, "v1.2.0", hist[0].Version)
	recs := audit.all()
	require.Len(t, recs, 1)
	assert.Equal(t, "v1.0.0 -> v1.2.0", recs[0].Reason)
}

func TestRecordVersionChange_NoChange(t *testing.T) {
	vh := &memVerHistRepo{}
	audit := newMemAuditRepo()
	s := newVersionServer(vh, audit)

	s.recordVersionChange(context.Background(), "node-1", "v1.0.0", "v1.0.0")

	assert.Equal(t, 0, vh.count(), "동일 버전은 기록하지 않는다")
	assert.Empty(t, audit.all())
}

func TestRecordVersionChange_EmptyNewVersion(t *testing.T) {
	vh := &memVerHistRepo{}
	s := newVersionServer(vh, nil)
	s.recordVersionChange(context.Background(), "node-1", "v1.0.0", "")
	assert.Equal(t, 0, vh.count(), "빈 버전은 기록하지 않는다")
}

func TestRecordVersionChange_NilRepoNoPanic(t *testing.T) {
	s := newVersionServer(nil, nil) // verHist 미구성
	// no-op, 패닉 없어야 한다.
	s.recordVersionChange(context.Background(), "node-1", "", "v1.0.0")
}

func TestRecordVersionChange_AuditNilStillRecordsHistory(t *testing.T) {
	vh := &memVerHistRepo{}
	s := newVersionServer(vh, nil) // audit 미구성
	s.recordVersionChange(context.Background(), "node-1", "", "v1.0.0")
	assert.Equal(t, 1, vh.count(), "audit 없어도 이력은 기록된다")
}
