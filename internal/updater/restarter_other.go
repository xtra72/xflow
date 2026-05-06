// @SPEC:SPEC-UPDATE-001 v0.1.0
//go:build !unix

// restarter_other.go — 비-Unix 플랫폼 (예: Windows) 용 stub.
//
// SPEC M6 는 Linux/macOS 만 지원 (Phase D 범위 외 Windows).
// 이 stub 은 빌드 호환성만 보장하고 실제 호출 시 ErrUpdateApplyFailed 반환.
package updater

import (
	"errors"
	"runtime"
)

// execNewBinary 는 비-Unix 플랫폼에서 항상 에러를 반환한다 (out-of-scope).
func execNewBinary(path string, args, env []string) error {
	return errors.New("syscall.Exec not supported on " + runtime.GOOS)
}
