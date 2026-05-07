// @SPEC:SPEC-UPDATE-001 v0.1.0 M10
// update.go — REST API DTOs for xflowd auto-update endpoints.
//
// 본 파일은 핸들러와 클라이언트가 공유하는 단순 데이터 구조체만 포함한다.
// 비즈니스 로직은 internal/api/handler/system_update.go 의 UpdateService 가 담당한다.
//
// 모든 응답은 dto.APIResponse[T] 엔벨로프로 래핑되어 클라이언트에 전달된다 (response.go 참조).
package dto

import "time"

// VersionResponse 는 GET /api/v1/system/version 응답 페이로드이다.
//
// 현재 실행 중인 xflowd 의 빌드 메타데이터 + 채널 + 마지막으로 확인된 최신 버전을 포함한다.
// LatestVersion / UpdateAvailable 은 최근 Check 가 호출된 적이 있을 때만 채워진다.
type VersionResponse struct {
	// Version 은 현재 실행 중 바이너리의 버전 (예: "v0.3.0").
	// build 시 ldflags 로 주입되며 정의되지 않으면 "dev".
	Version string `json:"version"`
	// Commit 은 빌드 시 ldflags 로 주입된 git commit SHA (짧은 형식).
	Commit string `json:"commit"`
	// BuildDate 은 빌드 시각 (ISO 8601 또는 "unknown").
	BuildDate string `json:"build_date"`
	// GoVersion 은 빌드에 사용된 Go 런타임 버전 (예: "go1.25.0").
	GoVersion string `json:"go_version"`
	// Channel 은 운영자가 설정한 업데이트 채널 (stable / beta / nightly).
	Channel string `json:"channel"`
	// UpdateAvailable 은 마지막 Check 결과 새 버전이 있는지 여부.
	// Check 가 한 번도 호출되지 않았으면 false.
	UpdateAvailable bool `json:"update_available"`
	// LatestVersion 은 마지막 Check 에서 발견된 최신 버전 (없으면 빈 문자열).
	LatestVersion string `json:"latest_version,omitempty"`
}

// ChannelInfoResponse 는 GET /api/v1/system/update/channel 응답 페이로드이다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6)
// 현재 활성 채널과 선택 가능한 모든 채널 enum 을 반환한다.
// Web UI 의 채널 선택 드롭다운에서 사용된다.
type ChannelInfoResponse struct {
	// Current 는 현재 cfg 에 설정된 채널 (stable / beta / nightly).
	Current string `json:"current"`
	// Available 은 시스템이 지원하는 모든 채널 enum (정렬: stable, beta, nightly).
	Available []string `json:"available"`
}

// ChangeChannelRequest 는 PUT /api/v1/system/update/channel 요청 본문이다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M7)
// Channel 값은 stable / beta / nightly 중 하나여야 한다.
// 잘못된 값은 ErrUpdateChannelInvalid (400) 로 매핑된다.
type ChangeChannelRequest struct {
	// Channel 은 새로 적용할 채널 enum.
	Channel string `json:"channel"`
}

// ChangeChannelResponse 는 PUT /api/v1/system/update/channel 응답 페이로드이다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M7)
// 채널 변경은 in-memory 만 적용되며, yaml 파일은 변경되지 않는다.
// 영구 저장 원하는 운영자는 `xflowd update channel <name>` CLI 또는 yaml 직접 수정 필요.
//
// 채널 변경 직후 새 채널로 즉시 Check 가 실행되고, 결과는 CheckResult 에 포함된다.
// Check 실패 시 (예: 네트워크 에러) CheckResult 는 nil 이며 Message 에 경고가 기재된다.
// 채널 변경 자체는 여전히 적용된다 (Previous != Current).
type ChangeChannelResponse struct {
	// Previous 는 변경 전 채널.
	Previous string `json:"previous"`
	// Current 는 변경 후 채널 (요청과 동일).
	Current string `json:"current"`
	// CheckResult 는 새 채널로 즉시 실행한 Check 결과 (실패 시 nil).
	CheckResult *CheckResponse `json:"check_result"`
	// Message 는 영구 저장 안내 또는 check 실패 경고 메시지.
	Message string `json:"message,omitempty"`
}

// CheckResponse 는 POST /api/v1/system/update/check 응답 페이로드이다.
//
// GitHub Releases 채널에서 신규 버전을 조회한 결과를 반환한다.
// 네트워크 에러는 503 으로 매핑되어 본 페이로드 대신 ErrorDetail 이 반환된다.
type CheckResponse struct {
	// Current 는 호출 시점의 현재 버전.
	Current string `json:"current"`
	// Latest 는 채널에서 발견된 최신 버전 (Available=false 라도 채워짐).
	Latest string `json:"latest"`
	// Available 은 latest > current 이고 platform asset 이 존재할 때 true.
	Available bool `json:"available"`
	// Channel 은 조회에 사용된 채널 enum.
	Channel string `json:"channel"`
	// ReleaseURL 은 GitHub release 페이지 URL (운영자가 변경사항 검토용).
	ReleaseURL string `json:"release_url,omitempty"`
	// PublishedAt 은 release 게시 시각 (UTC).
	PublishedAt time.Time `json:"published_at"`
}

// ApplyRequest 는 POST /api/v1/system/update/apply 요청 본문이다.
//
// Version 이 빈 문자열이면 채널의 latest 를 자동 선택한다.
// Force 는 다운그레이드 (latest < current) 를 명시적으로 허용한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M9, M14)
// AutoRestart 는 적용 후 자동 graceful drain → exec → self health check → 자동 rollback 흐름을 활성화한다.
// 기본값 false → v0.1.0 동작과 완전히 동일 (`ready_to_restart` 상태 종료, 운영자 수동 재시작).
//
// Target 은 업데이트 대상 바이너리 이름 ("xflowd" / "xflow-agent" / "xflow"). 빈 문자열이면 default "xflowd" 사용.
// whitelist 외 값은 400 Bad Request 로 거부 (path traversal 방어).
type ApplyRequest struct {
	// Version 은 적용 대상 버전 (예: "v0.4.0"). 빈 문자열이면 latest 사용.
	Version string `json:"version,omitempty"`
	// Force 가 true 면 다운그레이드 허용 (운영자 명시 동의).
	Force bool `json:"force,omitempty"`
	// AutoRestart 가 true 면 적용 후 자동 재시작 + 자가 health check + 자동 rollback (M1, v0.2.0 신규).
	// 기본값 false → v0.1.0 backward 호환 (수동 재시작).
	AutoRestart bool `json:"auto_restart,omitempty"`
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M14)
	// Target 은 업데이트 대상 바이너리 이름.
	// 허용값: "xflowd" (기본값), "xflow-agent", "xflow".
	// 빈 문자열이면 default "xflowd" 적용 (v0.1.0 backward 호환).
	// whitelist 외 값은 ErrUpdateInvalidInput (400) 으로 거부.
	Target string `json:"target,omitempty"`
}

// ApplyResponse 는 POST /api/v1/system/update/apply 즉시 응답 페이로드이다.
//
// Apply 는 비동기로 시작되며, 본 응답은 op 추적을 위한 식별자와 초기 상태만 포함한다.
// 클라이언트는 OperationID 로 GET /api/v1/system/update/status 를 폴링한다.
type ApplyResponse struct {
	// OperationID 는 진행 중 작업의 고유 식별자 (UUID v4).
	OperationID string `json:"operation_id"`
	// Status 는 즉시 응답 시점의 상태 (대개 "starting").
	Status string `json:"status"`
	// FromVersion 은 현재 실행 중 버전.
	FromVersion string `json:"from_version"`
	// ToVersion 은 적용 대상 버전 (요청 시점에 결정되었다면 채워짐).
	ToVersion string `json:"to_version,omitempty"`
}

// RollbackResponse 는 POST /api/v1/system/update/rollback 응답 페이로드이다.
type RollbackResponse struct {
	// FromVersion 은 롤백 직전 실행 중이던 버전 (rollback 후 더 이상 실행 안됨).
	FromVersion string `json:"from_version"`
	// ToVersion 은 백업에서 복원된 버전 (rollback 직전 이전 버전).
	ToVersion string `json:"to_version"`
	// RolledBackAt 은 rollback 완료 시각 (UTC).
	RolledBackAt time.Time `json:"rolled_back_at"`
}

// UpdateStatusResponse 는 GET /api/v1/system/update/status 응답 페이로드이다.
//
// 마지막으로 시도된 update 작업의 상태를 반환한다. 한 번도 실행된 적이 없으면
// Status="idle" 가 반환되고 OperationID 는 빈 문자열.
//
// Status 값 enum (v0.1.0 9-state + v0.2.0 신규 2-state = 11-state):
//   - "idle"               : 실행된 작업 없음 (서버 시작 후 최초 상태)
//   - "starting"           : Apply 가 호출되어 현재 op 가 생성됨
//   - "checking"           : 채널에서 latest release 조회 중
//   - "downloading"        : 바이너리 / manifest 다운로드 중
//   - "verifying"          : SHA256 + Ed25519 서명 검증 중
//   - "applying"           : 원자적 바이너리 교체 중
//   - "ready_to_restart"   : 적용 완료, 재시작 대기 중 (auto_restart=false 시 종료 상태)
//   - "restarting"         : graceful drain → exec 진행 중 (M2, auto_restart=true)
//   - "health_checking"    : 새 바이너리 self-probe 중 (M4, auto_restart=true)
//   - "completed"          : 작업 성공 종료
//   - "failed"             : 작업 실패 종료 (Error 필드에 사유 기재)
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M2, M4, M14)
// "restarting" / "health_checking" 은 auto_restart=true 인 경우에만 등장한다.
// 기존 9-state 흐름 (idle → ... → ready_to_restart → completed) 은 v0.1.0 그대로 유지.
type UpdateStatusResponse struct {
	// OperationID 는 마지막 작업 ID (idle 이면 빈 문자열).
	OperationID string `json:"operation_id,omitempty"`
	// Status 는 현재 상태 enum (위 주석 참조).
	Status string `json:"status"`
	// FromVersion 은 작업 시작 시점의 현재 버전.
	FromVersion string `json:"from_version,omitempty"`
	// ToVersion 은 작업 대상 버전 (결정된 후에만 채워짐).
	ToVersion string `json:"to_version,omitempty"`
	// StartedAt 은 작업 시작 시각 (UTC). idle 이면 zero value.
	StartedAt time.Time `json:"started_at,omitempty"`
	// CompletedAt 은 작업 완료 시각 (UTC). 진행 중이면 zero value.
	CompletedAt time.Time `json:"completed_at,omitempty"`
	// Error 는 실패 시 에러 메시지 (sentinel error 의 Error() 결과).
	Error string `json:"error,omitempty"`
}
