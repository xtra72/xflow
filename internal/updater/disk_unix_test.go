// @SPEC:SPEC-UPDATE-001 v0.1.0
// disk_unix_test.go — realDisk smoke test (linux/darwin only).
//
// 운영체제별 statfs 동작 검증 (실제 syscall 호출).

//go:build linux || darwin

package updater

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealDisk_AvailableBytes_TempDir 은 t.TempDir() 의 가용량을 조회한다.
//
// CI 환경에서도 /tmp 는 항상 mount 되어 있어야 정상이다.
func TestRealDisk_AvailableBytes_TempDir(t *testing.T) {
	d := realDisk{}
	avail, err := d.AvailableBytes(t.TempDir())
	require.NoError(t, err)
	assert.Greater(t, avail, uint64(0), "expected non-zero available bytes")
}

// TestRealDisk_AvailableBytes_NonexistentPath 는 잘못된 경로에서 에러 반환을 검증.
func TestRealDisk_AvailableBytes_NonexistentPath(t *testing.T) {
	d := realDisk{}
	_, err := d.AvailableBytes("/nonexistent/path/that/should/not/exist/zz")
	require.Error(t, err)
}
