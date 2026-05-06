// @SPEC:SPEC-UPDATE-001 v0.1.0
// rollback.go — Phase C 롤백.
//
// SPEC M7 (롤백):
//   - 매 update apply 마다 이전 바이너리를 <binary>.previous 로 백업
//   - 새 바이너리 health check 실패 시 자동 롤백 (Phase D restarter 가 트리거)
//   - 사용자 명시 롤백 (xflowd update rollback) 도 동일 절차
//   - 백업 부재 시 ErrUpdateRollbackFailed 반환
//   - 롤백 후 추가 자동 update 일시 중지 (운영자 명시 lock 해제 필요) — 본 모듈 책임 외
//
// 디자인 결정:
//   - go-update Apply 를 재사용해 backup → main 역방향 atomic rename
//   - 복원 후 백업 파일 정리 (재롤백 방지: M7 "loop 방지")
//   - context 는 인터페이스 일관성용 (현재 구현은 동기 - 향후 timeout 확장 대비)
package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/inconshreveable/go-update"
)

// Rollback 은 백업된 이전 바이너리로 복원하는 컴포넌트이다.
//
// 사용 흐름:
//  1. NewRollback(binaryPath) 로 인스턴스 생성
//  2. CanRollback() 으로 백업 존재 확인
//  3. (선택) BackupInfo() 로 백업 메타데이터 조회
//  4. Restore(ctx) 로 복원 수행
type Rollback struct {
	binaryPath string
	backupPath string
}

// NewRollback 은 Rollback 인스턴스를 생성한다.
//
// binaryPath 는 현재 실행 중 바이너리 경로. 백업 경로는 자동으로
// <binaryPath>.previous 로 결정된다 (M5 / M7 컨벤션).
func NewRollback(binaryPath string) *Rollback {
	return &Rollback{
		binaryPath: binaryPath,
		backupPath: binaryPath + ".previous",
	}
}

// CanRollback 은 백업 파일이 존재하고 일반 파일인 경우 true 를 반환한다.
//
// 백업이 디렉토리이거나 심볼릭 링크 등 비정상 상태이면 false (안전 보수적 판정).
func (r *Rollback) CanRollback() bool {
	info, err := os.Stat(r.backupPath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// BackupInfo 는 백업 파일의 기본 메타데이터를 반환한다.
//
// 백업이 없으면 ErrUpdateRollbackFailed wrapping 에러 반환.
type BackupInfo struct {
	// Path 는 백업 파일의 절대 경로 (대개 <binaryPath>.previous).
	Path string
	// Size 는 바이트 단위 크기.
	Size int64
	// ModTime 은 백업 파일 마지막 수정 시각 (대개 백업 생성 시각과 일치).
	ModTime time.Time
	// Mode 는 파일 권한 비트 (롤백 후 권한 복원 참고용).
	Mode os.FileMode
}

// BackupInfo 는 백업 파일 메타데이터를 조회한다.
//
// 백업 부재 / 권한 부족 / 디렉토리인 경우 ErrUpdateRollbackFailed 반환.
func (r *Rollback) BackupInfo() (BackupInfo, error) {
	info, err := os.Stat(r.backupPath)
	if err != nil {
		return BackupInfo{}, fmt.Errorf("%w: stat backup at %s: %v",
			ErrUpdateRollbackFailed, r.backupPath, err)
	}
	if !info.Mode().IsRegular() {
		return BackupInfo{}, fmt.Errorf("%w: backup at %s is not a regular file",
			ErrUpdateRollbackFailed, r.backupPath)
	}
	return BackupInfo{
		Path:    r.backupPath,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Mode:    info.Mode().Perm(),
	}, nil
}

// RollbackResult 는 Restore 성공 시 반환되는 결과 정보이다.
type RollbackResult struct {
	// RestoredFrom 은 복원 소스 경로 (백업 파일 경로).
	RestoredFrom string
	// RestoredTo 는 복원 대상 경로 (메인 바이너리 경로).
	RestoredTo string
	// RestoredAt 은 복원 완료 시각 (UTC time.Now()).
	RestoredAt time.Time
}

// Restore 는 백업 파일을 메인 바이너리 위치로 atomic 복원한다.
//
// 흐름:
//  1. context 취소 확인 (fail-fast)
//  2. CanRollback() 검증
//  3. 백업 파일 메모리 로드
//  4. go-update.Apply 로 atomic rename (역방향: backup → main)
//  5. 백업 파일 제거 (재롤백 방지, M7 loop 방지)
//  6. RollbackResult 반환
//
// 에러 시 ErrUpdateRollbackFailed wrapping. 메인 바이너리는 변경되지 않거나
// (실패 시) go-update 가 자동 복원을 시도한다.
func (r *Rollback) Restore(ctx context.Context) (RollbackResult, error) {
	// Fail-fast: context 취소 확인
	if err := ctx.Err(); err != nil {
		return RollbackResult{}, fmt.Errorf("%w: context cancelled: %v",
			ErrUpdateRollbackFailed, err)
	}

	if !r.CanRollback() {
		return RollbackResult{}, fmt.Errorf("%w: no backup at %s",
			ErrUpdateRollbackFailed, r.backupPath)
	}

	backupBytes, err := os.ReadFile(r.backupPath)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("%w: read backup: %v",
			ErrUpdateRollbackFailed, err)
	}

	// 백업 데이터로 메인 바이너리를 atomic 교체.
	// 롤백 시점에는 추가 백업 (.old) 을 만들지 않음 → OldSavePath 비어 있음
	// (현재 메인 바이너리는 health check 실패한 상태이므로 보존 의미 없음).
	updateOpts := update.Options{
		TargetPath: r.binaryPath,
		TargetMode: 0o755,
	}

	if err := update.Apply(bytes.NewReader(backupBytes), updateOpts); err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return RollbackResult{}, fmt.Errorf(
				"%w: restore failed and internal rollback also failed: %v (internal err: %v)",
				ErrUpdateRollbackFailed, err, rerr)
		}
		return RollbackResult{}, fmt.Errorf("%w: apply rollback: %v",
			ErrUpdateRollbackFailed, err)
	}

	// 백업 파일 제거 (재롤백 방지 + 디스크 정리)
	// 제거 실패는 critical 하지 않음 (다음 update apply 가 덮어씀)
	_ = os.Remove(r.backupPath)

	return RollbackResult{
		RestoredFrom: r.backupPath,
		RestoredTo:   r.binaryPath,
		RestoredAt:   time.Now(),
	}, nil
}
