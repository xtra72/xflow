// @SPEC:SPEC-UPDATE-001 v0.1.0
// disk_other.go — 비-POSIX 환경 (windows 등) stub.
//
// 본 SPEC 은 Linux/macOS 운영 환경을 가정하지만 빌드는 호환되어야 한다.
// windows 에서는 보수적으로 max 가용량 (uint64 max) 을 반환하여 검사 통과
// (또는 추후 GetDiskFreeSpaceEx 통합 가능).

//go:build !linux && !darwin

package updater

import "math"

type realDisk struct{}

// AvailableBytes 는 비-POSIX 환경에서 충분히 큰 값을 반환한다 (검사 우회).
//
// 이는 운영 환경 (linux/darwin) 외에서의 빌드 호환성만 위한 fallback 이며,
// production 운영자는 linux/darwin 에서만 자동 업데이트를 활성화해야 한다.
func (realDisk) AvailableBytes(_ string) (uint64, error) {
	return math.MaxUint64, nil
}
