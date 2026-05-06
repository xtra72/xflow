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
type ApplyRequest struct {
	// Version 은 적용 대상 버전 (예: "v0.4.0"). 빈 문자열이면 latest 사용.
	Version string `json:"version,omitempty"`
	// Force 가 true 면 다운그레이드 허용 (운영자 명시 동의).
	Force bool `json:"force,omitempty"`
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
// Status 값 enum:
//   - "idle"               : 실행된 작업 없음 (서버 시작 후 최초 상태)
//   - "starting"           : Apply 가 호출되어 현재 op 가 생성됨
//   - "checking"           : 채널에서 latest release 조회 중
//   - "downloading"        : 바이너리 / manifest 다운로드 중
//   - "verifying"          : SHA256 + Ed25519 서명 검증 중
//   - "applying"           : 원자적 바이너리 교체 중
//   - "ready_to_restart"   : 적용 완료, 재시작 대기 중
//   - "completed"          : 작업 성공 종료
//   - "failed"             : 작업 실패 종료 (Error 필드에 사유 기재)
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
