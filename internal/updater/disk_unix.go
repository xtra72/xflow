// @SPEC:SPEC-UPDATE-001 v0.1.0
// disk_unix.go — POSIX 가용 디스크 공간 조회 (linux/darwin).
//
// build tag 로 windows 제외 (downloader.go realDisk 는 동일 이름이므로 stub 필요).
// MoAI-ADK 운영 환경 (Linux/macOS) 에서만 활성화된다.

//go:build linux || darwin

package updater

import (
	"fmt"
	"syscall"
)

// realDisk 는 syscall.Statfs 를 통해 가용 바이트를 보고한다.
type realDisk struct{}

// AvailableBytes 는 path 의 mount point 의 가용 바이트 수를 반환한다.
//
// 비특권 사용자 quota (Bavail) 를 사용한다 (root quota 는 Bfree).
//
// 실패 사례:
//   - path 가 존재하지 않음 → ENOENT
//   - 권한 부족 → EACCES
func (realDisk) AvailableBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	// Bavail × Bsize = 비특권 사용자 가용 바이트
	// 일부 환경에서 Bsize 가 int32 이거나 uint32 일 수 있어 명시 변환
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
