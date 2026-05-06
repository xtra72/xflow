// @SPEC:SPEC-UPDATE-001 v0.1.0
// applier.go — Phase C 원자적 바이너리 교체.
//
// SPEC M5 (원자적 교체):
//   - POSIX rename(2) 기반 atomic move (github.com/inconshreveable/go-update 위임)
//   - 교체 직전 현재 바이너리를 <binary>.previous 로 백업
//   - 동일 파일시스템 내에서만 동작 (cross-device rename 회피는 호출자 책임)
//   - 인터럽트 발생해도 파일시스템 일관성 유지 (atomic rename)
//
// SPEC M4 (TOCTOU 방어):
//   - 다운로드 직후 (Phase B downloader) 와 rename 직전 (이 파일) 두 시점에 검증
//   - 변조 감지 시 ErrUpdateApplyFailed 반환, 백업도 생성하지 않음
//
// 디자인 결정:
//   - go-update 라이브러리 활용: 검증된 atomic rename + 자동 rollback 로직
//   - 메모리 로드: 바이너리는 일반적으로 < 50MB → ReadAll 안전
//   - SkipBackup 옵션: 테스트 전용 (운영 환경은 항상 백업 생성)
package updater

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/inconshreveable/go-update"
)

// Applier 는 검증된 다운로드 바이너리를 현재 실행 위치로 원자적 교체한다.
//
// 사용 흐름:
//  1. NewApplier(verifier, binaryPath) 로 인스턴스 생성
//  2. Apply(ctx, ApplyOptions{Manifest, DownloadedPath}) 호출
//
// 보안 critical: SPEC M4 TOCTOU 방어를 위해 rename 직전 재검증 수행.
type Applier struct {
	// Verifier 는 SHA256 + Ed25519 검증을 수행한다.
	// nil 이면 Apply 가 ErrUpdateInvalidInput 반환 (안전 기본값).
	Verifier *Verifier

	// binaryPath 는 교체 대상 바이너리 경로 (현재 실행 중 바이너리 경로).
	binaryPath string

	// backupPath 는 <binaryPath>.previous (롤백 대비, M7 참조).
	backupPath string
}

// NewApplier 는 Applier 인스턴스를 생성한다.
//
// verifier 는 nil 도 허용 (Apply 호출 시 검사) — 테스트 시나리오 대응.
// binaryPath 는 빈 문자열 허용 (go-update 가 현재 실행 파일을 자동 감지).
func NewApplier(verifier *Verifier, binaryPath string) *Applier {
	return &Applier{
		Verifier:   verifier,
		binaryPath: binaryPath,
		backupPath: binaryPath + ".previous",
	}
}

// ApplyOptions 는 Apply 호출 시 전달되는 옵션 묶음이다.
type ApplyOptions struct {
	// Manifest 는 Phase B 에서 받아온 검증 메타데이터 (SHA256 + 서명 + 버전).
	Manifest Manifest

	// DownloadedPath 는 Phase B downloader 가 검증 완료 후 디스크에 저장한 임시 파일 경로.
	// Apply 는 이 파일을 다시 읽어 pre-rename 재검증 후 atomic 교체 수행.
	DownloadedPath string

	// SkipBackup 은 백업 파일 생성을 생략한다 (테스트 전용).
	// 운영 환경에서는 항상 false (M5 백업 의무).
	SkipBackup bool
}

// ApplyResult 는 Apply 성공 시 반환되는 결과 정보이다.
type ApplyResult struct {
	// NewVersion 은 교체 후 적용된 버전 (Manifest.Version 의 복사본).
	NewVersion Version

	// BackupPath 는 생성된 백업 파일 경로 (SkipBackup=true 면 빈 문자열).
	BackupPath string

	// AppliedAt 은 atomic rename 완료 시점 (UTC time.Now()).
	AppliedAt time.Time
}

// Apply 는 검증된 다운로드 바이너리를 atomic rename 으로 현재 위치에 적용한다.
//
// 흐름:
//  1. context 취소 확인 (fail-fast)
//  2. Verifier nil 검사
//  3. DownloadedPath 파일 읽기 → ErrUpdateApplyFailed
//  4. M4 TOCTOU 방어: VerifyAll(content, manifest.SHA256, manifest.Signature)
//  5. go-update Apply 로 atomic rename + 백업 생성
//  6. ApplyResult 반환
//
// 에러 시 ErrUpdateApplyFailed (또는 sentinel 보존) 으로 wrapping.
// 백업 생성 후 rename 실패 시 go-update 가 자동 rollback (이전 바이너리 복원).
func (a *Applier) Apply(ctx context.Context, opts ApplyOptions) (ApplyResult, error) {
	// Fail-fast: context 취소 확인
	if err := ctx.Err(); err != nil {
		return ApplyResult{}, fmt.Errorf("%w: context cancelled: %v", ErrUpdateApplyFailed, err)
	}

	// Verifier nil 안전 검사 (테스트에서 NewApplier(nil, ...) 호출 가능)
	if a.Verifier == nil {
		return ApplyResult{}, fmt.Errorf("%w: verifier is nil", ErrUpdateInvalidInput)
	}

	// 다운로드 파일 읽기 (전체 바이너리 메모리 로드)
	f, err := os.Open(opts.DownloadedPath)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: open downloaded file: %v", ErrUpdateApplyFailed, err)
	}
	defer func() { _ = f.Close() }()

	content, err := io.ReadAll(f)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: read downloaded file: %v", ErrUpdateApplyFailed, err)
	}

	// SPEC M4 TOCTOU 방어: rename 직전 재검증.
	// Phase B downloader 가 1차 검증 → 디스크 휴면 → 여기서 2차 검증.
	if err := a.Verifier.VerifyAll(content, opts.Manifest.SHA256, opts.Manifest.Signature); err != nil {
		return ApplyResult{}, fmt.Errorf("%w: pre-rename verification: %w", ErrUpdateApplyFailed, err)
	}

	// go-update 옵션 구성 (atomic rename + 백업)
	updateOpts := update.Options{
		TargetPath: a.binaryPath,
		TargetMode: 0o755,
	}
	if !opts.SkipBackup {
		updateOpts.OldSavePath = a.backupPath
	}

	// 권한 사전 검사 (가능한 경우)
	if err := updateOpts.CheckPermissions(); err != nil {
		return ApplyResult{}, fmt.Errorf("%w: permission check: %v", ErrUpdateApplyFailed, err)
	}

	// 원자적 교체 수행 (POSIX rename(2))
	if err := update.Apply(bytes.NewReader(content), updateOpts); err != nil {
		// go-update 내부 rollback 결과도 함께 보고 (디버깅 용이)
		if rerr := update.RollbackError(err); rerr != nil {
			return ApplyResult{}, fmt.Errorf(
				"%w: apply failed and rollback also failed: %v (rollback err: %v)",
				ErrUpdateApplyFailed, err, rerr)
		}
		return ApplyResult{}, fmt.Errorf("%w: %v", ErrUpdateApplyFailed, err)
	}

	backupPath := a.backupPath
	if opts.SkipBackup {
		backupPath = ""
	}

	return ApplyResult{
		NewVersion: opts.Manifest.Version,
		BackupPath: backupPath,
		AppliedAt:  time.Now(),
	}, nil
}
