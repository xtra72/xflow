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

// -----------------------------------------------------------------------------
// 대시보드 1급 엔티티 DTO (SPEC-DASHBOARD-004 M4, spec.md §2.3)
// -----------------------------------------------------------------------------
//
// 위의 DashboardSnapshot / DashboardPutRequest 는 읽기 전용 호환 shim 과 원격 노드
// 프록시 전용이며(spec.md §4.4) camelCase 를 유지한다. 아래 신규 DTO 는 spec.md
// §2.3 과 acceptance.md 가 명시한 snake_case 키를 쓴다 — 두 계약이 서로 다른
// 소비자를 향하므로 한쪽에 맞춰 다른 쪽을 바꾸면 그 소비자가 깨진다.

// DashboardMeta 는 대시보드 1장의 메타데이터 + 요청자 인가 판정 결과이다.
//
// CanEdit / CanDelete / CanGrant 를 서버가 실어 보내는 이유는 프론트가 spec.md
// §2.2 판정을 재구현하지 않게 하기 위함이다(spec.md §2.3, §2.12). 판정 규칙이
// 두 곳에 있으면 한쪽만 갱신되어 UI 와 서버가 어긋난다.
type DashboardMeta struct {
	UID        string `json:"uid"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	Visibility string `json:"visibility"` // "private" | "shared" | "acl"
	IsDefault  bool   `json:"is_default"`
	SortOrder  int64  `json:"sort_order"`
	Version    int64  `json:"version"`
	CreatedAt  int64  `json:"created_at"` // epoch ms
	UpdatedAt  int64  `json:"updated_at"` // epoch ms

	CanEdit   bool `json:"can_edit"`
	CanDelete bool `json:"can_delete"`
	CanGrant  bool `json:"can_grant"`
}

// DashboardDetail 은 단건 조회·저장 응답이다. 목록 응답(DashboardMeta)과 달리
// payload 를 포함한다(spec.md §2.3 — 목록은 payload 를 포함하지 않는다).
type DashboardDetail struct {
	DashboardMeta
	Payload json.RawMessage `json:"payload"`
}

// DashboardCreateRequest 는 POST /dashboards 요청 본문이다.
//
// owner · visibility · version · uid 는 본문에 있어도 **전부 무시된다**
// (spec.md §2.13 UB1 #4). 그래서 필드 자체를 두지 않는다 — 필드를 두고 무시하면
// "보냈는데 왜 안 되지" 라는 오해를 만든다.
type DashboardCreateRequest struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

// DashboardSaveRequest 는 PUT /dashboards/{uid} 요청 본문이다(본문 저장 전용).
type DashboardSaveRequest struct {
	Payload json.RawMessage `json:"payload"`
}

// DashboardPatchRequest 는 PATCH /dashboards/{uid} 요청 본문이다.
//
// nil 필드는 변경하지 않는다. name 은 edit 인가, visibility · is_default 는 grant
// 인가를 요구한다(spec.md §2.3 라우트 표).
type DashboardPatchRequest struct {
	Name       *string `json:"name"`
	Visibility *string `json:"visibility"`
	IsDefault  *bool   `json:"is_default"`
	SortOrder  *int64  `json:"sort_order"`
}

// DashboardACLEntry 는 대시보드 권한 부여 대상 1건이다.
//
// GrantedBy / GrantedAt 은 서버가 부여하므로 요청 시 무시되고 응답에만 채워진다.
type DashboardACLEntry struct {
	Subject   string `json:"subject"` // "user:<username>" | "role:<rolename>"
	Level     string `json:"level"`   // "view" | "edit"
	GrantedBy string `json:"granted_by,omitempty"`
	GrantedAt int64  `json:"granted_at,omitempty"` // epoch ms
}

// DashboardACLReplaceRequest 는 PUT /dashboards/{uid}/acl 의 객체 형식 본문이다.
//
// acceptance.md AC-17 은 최상위 배열(`[{...}]`)을 보낸다. 핸들러는 배열을 우선
// 받아들이고, 객체 형식(`{"entries":[...]}`)도 함께 허용한다 — 두 형식을 모두
// 받는 편이 클라이언트 구현체를 하나로 강제하는 것보다 안전하다.
type DashboardACLReplaceRequest struct {
	Entries []DashboardACLEntry `json:"entries"`
}

// DashboardUserStateResponse 는 GET/PUT /dashboard-state 응답이다.
//
// ActiveDashboardUID 는 존재하지 않는 uid 를 가리키면 빈 문자열로 정규화된다
// (spec.md §2.13 UB1 #11 — 400 이 아니다).
type DashboardUserStateResponse struct {
	ActiveDashboardUID string          `json:"active_dashboard_uid"`
	DeviceGridLayout   json.RawMessage `json:"device_grid_layout"`
	Version            int64           `json:"version"`
	UpdatedAt          int64           `json:"updated_at"` // epoch ms
}

// DashboardUserStateRequest 는 PUT /dashboard-state 요청 본문이다.
// username 은 세션 사용자로 고정되므로 본문에 두지 않는다.
type DashboardUserStateRequest struct {
	ActiveDashboardUID string          `json:"active_dashboard_uid"`
	DeviceGridLayout   json.RawMessage `json:"device_grid_layout"`
}
