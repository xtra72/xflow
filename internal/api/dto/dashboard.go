// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-6)
// dto/dashboard.go — 대시보드 API 의 요청/응답 DTO.
//
// 응답 형태는 spec.md 의 데이터 모델 (lines 324-331) 을 따른다:
//
//	{
//	  "scope":     "global" | "user",
//	  "owner":     null | "<username>",
//	  "version":   <int64>,
//	  "updatedAt": <int64 epoch ms>,
//	  "payload":   { ... }
//	}
//
// 요청 시 클라이언트가 보내는 scope/owner/version/updatedAt 은 모두 서버에 의해
// 무시된다 (UB-003). 서버는 URL + JWT + 저장소 상태에서 결정한다.

package dto

import "encoding/json"

// DashboardSnapshot 은 단일 대시보드 snapshot 의 API 응답 표현이다.
//
// Owner 는 *string 으로 정의되어 scope=global 시 JSON 상 null 로 직렬화된다 (spec
// 라인 305: "owner: string | null").
type DashboardSnapshot struct {
	Scope     string          `json:"scope"`     // "global" | "user"
	Owner     *string         `json:"owner"`     // null when scope=global
	Version   int64           `json:"version"`   // 서버 부여 단조 증가
	UpdatedAt int64           `json:"updatedAt"` // epoch ms (int64)
	Payload   json.RawMessage `json:"payload"`   // 클라이언트가 PUT 한 JSON 원본
}

// DashboardPutRequest 는 PUT 요청 본문의 형식이다.
//
// 서버가 무시하는 필드 (UB-003):
//   - scope     : URL 로부터 결정. body 에 있으면 URL 과 불일치 검증 (UB-006).
//   - owner     : JWT Claims.Username 으로 결정.
//   - version   : 서버 부여.
//   - updatedAt : 서버 부여.
//
// 본 DTO 는 payload 만 명시적으로 디코딩하고, scope 는 UB-006 검증용으로 함께
// 디코딩한다. 그 외 필드는 핸들러가 io.LimitReader 로 본문을 받아 json.Unmarshal
// 단계에서 무시한다 (Bind 호출 안 함 — DisallowUnknownFields 와 충돌 회피).
type DashboardPutRequest struct {
	// Scope 가 명시되어 있으면 URL 과 일치해야 한다 (UB-006). 빈 문자열이면 검증 skip.
	Scope string `json:"scope,omitempty"`

	// Payload 는 실제 저장될 JSON. 빈 객체 ({}) 도 허용.
	Payload json.RawMessage `json:"payload"`
}
