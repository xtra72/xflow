// @SPEC:SPEC-UPDATE-001 v0.1.0
//go:build unix

// restarter_unix.go — Unix 계열 (Linux/macOS/BSD) 의 syscall.Exec 구현.
//
// SPEC M6: syscall.Exec 은 현재 프로세스 이미지를 새 바이너리로 교체한다.
//   - PID 보존 (supervisor 가 새 프로세스를 같은 child 로 인식)
//   - open FD 보존 (CLOEXEC 미설정 FD 가 새 바이너리로 인계됨)
//   - 정상 exec 시 호출자에게 return 안 함 (프로세스가 사라짐)
package updater

import "syscall"

// execNewBinary 는 path 의 바이너리로 현재 프로세스를 교체한다.
//
// 정상 시 return 하지 않는다 (프로세스 이미지 교체).
// 실패 시 (path 부재, ENOEXEC, 권한 부족 등) syscall 에러 반환.
func execNewBinary(path string, args, env []string) error {
	return syscall.Exec(path, args, env)
}
