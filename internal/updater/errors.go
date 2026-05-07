// @SPEC:SPEC-UPDATE-001 v0.1.0
// errors.go — sentinel errors for the updater package.
//
// 모든 에러는 errors.Is(err, ErrXxx) 로 식별 가능하며, fmt.Errorf("%w", ...) 로
// 래핑된 경우에도 동작한다 (sentinel pattern).
//
// 보안 critical 에러 (ErrUpdateSignatureInvalid, ErrUpdateChecksumMismatch) 는
// 별도 보안 감사 로그로도 기록되어야 한다 (SPEC M13).
package updater

import "errors"

var (
	// ErrUpdateChannelInvalid 는 채널 URL/이름이 유효하지 않을 때 반환된다.
	// 사례: HTTP 스킴 (HTTPS 강제), enum 외 값, 잘못된 GitHub Releases URL.
	ErrUpdateChannelInvalid = errors.New("updater: invalid update channel")

	// ErrUpdateDownloadFailed 는 다운로드 실패 (네트워크 오류, 5xx, 중단) 시 반환된다.
	// SPEC M3: 임시 파일은 즉시 폐기되며 재개 시도 없음.
	ErrUpdateDownloadFailed = errors.New("updater: download failed")

	// ErrUpdateInsufficientDiskSpace 는 임시 디렉토리의 가용 공간이 부족할 때 반환된다.
	// SPEC M3: 사전 검사 임계값은 바이너리 크기 × 3 (백업 + 새 + 여유).
	ErrUpdateInsufficientDiskSpace = errors.New("updater: insufficient disk space")

	// ErrUpdateChecksumMismatch 는 SHA256 체크섬 비교 실패 시 반환된다.
	// SPEC M4: 다운로드 파일은 즉시 삭제된다.
	ErrUpdateChecksumMismatch = errors.New("updater: checksum mismatch")

	// ErrUpdateSignatureInvalid 는 Ed25519 서명 검증 실패 시 반환된다.
	// SPEC M4: 잠재적 침해 신호 → 보안 감사 로그 + 임시 파일 즉시 삭제.
	ErrUpdateSignatureInvalid = errors.New("updater: signature invalid")

	// ErrUpdateApplyFailed 는 원자적 바이너리 교체 실패 시 반환된다.
	// SPEC M5: 권한 부족, 디스크 가득, cross-device rename 등이 사례.
	ErrUpdateApplyFailed = errors.New("updater: apply failed")

	// ErrUpdateRollbackFailed 는 롤백 시도 실패 시 반환된다.
	// SPEC M7: 백업 파일 부재/손상 → 운영자 개입 필요 (critical log).
	ErrUpdateRollbackFailed = errors.New("updater: rollback failed")

	// ErrDowngradeRequiresForce 는 다운그레이드 시도가 --force flag 없이 들어왔을 때 반환된다.
	// SPEC M8: 자동 적용 모드는 다운그레이드를 절대 자동 수행하지 않음.
	ErrDowngradeRequiresForce = errors.New("updater: downgrade requires --force flag")

	// ErrUpdateInProgress 는 동시 update apply 호출 시 두 번째 호출에 반환된다.
	// SPEC M10: HTTP 409 Conflict 응답에 매핑된다.
	ErrUpdateInProgress = errors.New("updater: update already in progress")

	// ErrUpdateInvalidInput 은 보조 sentinel (입력 검증 실패용).
	// 사례: nil 공개키, 잘못된 hex hash, 빈 콘텐츠/서명.
	// SPEC 명시 9종 외에 verifier/checker 등의 입력 검증 단계에서 사용.
	ErrUpdateInvalidInput = errors.New("updater: invalid input")

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M3, M5, M13)
	// ErrUpdateRestartFailed 는 graceful drain timeout 또는 syscall.Exec 실패 시 반환된다.
	// 발생 시점:
	//   - reverifyDownloaded(): TOCTOU 재검증 실패 (변조 또는 파일 부재)
	//   - Restart() / execFn: syscall.Exec 호출 실패 (권한, 파일 손상)
	// 운영자 대응: .previous 백업이 보존되므로 `xflowd update rollback` 으로 복구 가능.
	ErrUpdateRestartFailed = errors.New("updater: restart failed (graceful drain timeout or exec failure)")

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M4, M5, M13)
	// ErrUpdateHealthCheckFailed 는 새 바이너리 self-probe 실패 시 반환된다.
	// 발생 시점:
	//   - HealthChecker.WaitHealthy() 가 timeout 동안 200 응답 미수신
	//   - 응답 version 이 expected target version 과 불일치
	//   - context 취소
	// 운영자 대응: 자동 rollback 트리거 (M5). 두 번째 health check 도 실패 시 ErrUpdateRollbackFailed.
	ErrUpdateHealthCheckFailed = errors.New("updater: post-restart health check failed")

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11, M13)
	// ErrUpdateIncompatibleVersion 은 dependency manifest 가 환경의 다른 바이너리 버전과
	// 호환되지 않을 때 반환된다.
	// 발생 시점:
	//   - CompatibilityChecker.Validate() 가 manifest 의 Compat 제약을 위반한 환경을 발견
	//   - 예: xflow-agent v0.4.0 manifest 가 xflowd >=v0.4.0 을 요구하는데 환경에 xflowd v0.3.0 이 설치됨
	// 운영자 대응: 의존 바이너리를 먼저 업그레이드하거나 --force-incompatible flag (미구현) 로 우회.
	// 보안: 호환성 위반은 단순 거부이며, 잠재적 침해 신호는 아님 (감사 로그 trigger 아님).
	ErrUpdateIncompatibleVersion = errors.New("updater: binary versions incompatible per manifest constraints")
)
