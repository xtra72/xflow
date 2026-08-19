// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.3, §4.4)
// dashboard_shim.go — 신규 1급 엔티티 모델 → 레거시 스냅샷 형상 합성.
//
// 구 `GET /dashboards/{shared,mine}` 는 원격 노드 프록시가 소비한다
// (internal/api/handler/remote_query.go, cmd/xflowd/remote_query.go,
//  web/src/services/api/remoteService.ts). 원격 노드는 본 SPEC 미적용 버전이
// 혼재할 수 있으므로 응답 형상을 바꿀 수 없다(spec.md §4.4).
//
// 그래서 저장소를 갈아엎는 대신, 신규 모델에서 레거시 `storage.DashboardSnapshot`
// 을 **합성**해 기존 변환기(DashboardSnapshotToDTO, 시그니처 고정)에 넘긴다.
// 원격 경로 3개 파일은 무변경으로 남는다.
//
// 쓰기(PUT/DELETE)는 합성하지 않는다 — 묶음 단위 쓰기는 "어느 대시보드의 어느
// version 에 대한 쓰기인가" 를 결정할 수 없어 낙관적 동시성이 성립하지 않는다.

package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// legacyDashboardPage 는 구 payload 의 dashboardPages 원소이다
// (web/src/types/dashboard.ts DashboardPageConfig).
//
// panels / layout 은 재직렬화 없이 원본 바이트를 그대로 옮긴다 — 패널 설정은
// 타입이 열려 있어 구조체로 받으면 알 수 없는 필드가 조용히 사라진다.
type legacyDashboardPage struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"isDefault"`
	Panels    json.RawMessage `json:"panels"`
	Layout    json.RawMessage `json:"layout"`
}

// legacyDashboardPayload 는 구 스냅샷 payload 전체이다
// (web/src/types/dashboard.ts DashboardPayload).
//
// 그리드 설정 3종의 키 이름이 신규 대시보드 payload(gridCols 등)와 다르다
// (dashboardGridCols 등). 이관(M3)이 접두사를 떼는 방향이었으므로 shim 은 붙인다.
type legacyDashboardPayload struct {
	DashboardPages    []legacyDashboardPage `json:"dashboardPages"`
	ActiveDashboardID string                `json:"activeDashboardId"`
	GridCols          json.RawMessage       `json:"dashboardGridCols,omitempty"`
	ShowGridLines     json.RawMessage       `json:"dashboardShowGridLines,omitempty"`
	RefreshInterval   json.RawMessage       `json:"dashboardRefreshInterval,omitempty"`
	DeviceGridLayout  json.RawMessage       `json:"deviceGridLayout"`
}

// entityDashboardPayload 는 신규 대시보드 1장의 payload 이다 (spec.md §2.1).
type entityDashboardPayload struct {
	Panels          json.RawMessage `json:"panels"`
	Layout          json.RawMessage `json:"layout"`
	GridCols        json.RawMessage `json:"gridCols"`
	ShowGridLines   json.RawMessage `json:"showGridLines"`
	RefreshInterval json.RawMessage `json:"refreshInterval"`
}

// SynthesizeDashboardSnapshot 은 대시보드 N 장을 레거시 스냅샷 1건으로 합성한다.
//
// items 는 이미 호출자가 인가·소유권으로 걸러낸 뒤 sort_order 순으로 정렬된 것이다
// (storage.DashboardRepository.List 가 sort_order → uid 순으로 반환한다).
//
// version / updatedAt 은 포함된 행들의 **최댓값** 이다(spec.md §2.3 shim 표).
// 최댓값을 쓰는 이유: 클라이언트가 이 값을 "마지막으로 바뀐 시점" 으로 읽으므로,
// 어느 한 장이 바뀌면 묶음도 바뀐 것으로 보여야 폴링·캐시 무효화가 성립한다.
//
// 그리드 설정 3종은 첫 항목의 값을 승계한다. 구 모델에서 묶음당 1벌이었고 이관이
// 그 값을 각 대시보드에 복사했으므로(M3 buildEntityPayload), 같은 출처에서 나온
// 대시보드들은 같은 값을 갖는다.
func SynthesizeDashboardSnapshot(scope, owner string, items []storage.Dashboard) *storage.DashboardSnapshot {
	body := legacyDashboardPayload{
		DashboardPages:   make([]legacyDashboardPage, 0, len(items)),
		DeviceGridLayout: json.RawMessage(`{}`),
	}

	var maxVersion, maxUpdatedAt int64
	for i := range items {
		d := items[i]
		if d.Version > maxVersion {
			maxVersion = d.Version
		}
		if d.UpdatedAt > maxUpdatedAt {
			maxUpdatedAt = d.UpdatedAt
		}

		var entity entityDashboardPayload
		// payload 가 깨져 있어도 shim 전체를 실패시키지 않는다 — 읽기 전용 호환
		// 경로에서 500 을 내면 원격 노드 대시보드 조회가 통째로 죽는다.
		_ = json.Unmarshal(d.Payload, &entity)

		body.DashboardPages = append(body.DashboardPages, legacyDashboardPage{
			ID:        d.UID,
			Name:      d.Name,
			IsDefault: d.IsDefault,
			Panels:    emptyArrayIfBlank(entity.Panels),
			Layout:    emptyArrayIfBlank(entity.Layout),
		})

		if i == 0 {
			body.GridCols = blankToNil(entity.GridCols)
			body.ShowGridLines = blankToNil(entity.ShowGridLines)
			body.RefreshInterval = blankToNil(entity.RefreshInterval)
		}
	}

	if maxUpdatedAt == 0 {
		// 포함된 행이 없으면 "지금 조회한 빈 상태" 를 그대로 표현한다.
		maxUpdatedAt = time.Now().UnixMilli()
	}

	payload, err := json.Marshal(body)
	if err != nil {
		// legacyDashboardPayload 는 RawMessage 만 담으므로 도달하지 않는다.
		payload = []byte(`{"dashboardPages":[],"activeDashboardId":"","deviceGridLayout":{}}`)
	}

	return &storage.DashboardSnapshot{
		Scope:     scope,
		Owner:     owner,
		Version:   maxVersion,
		UpdatedAt: maxUpdatedAt,
		Payload:   payload,
	}
}

// DashboardSnapshotShim 은 신규 저장소를 레거시 스냅샷 리더로 어댑트한다.
//
// cmd/xflowd 의 원격 query 브리지(dashboard.get_shared / get_mine)가 소비한다.
// 그 경로는 노드-로컬 read 이며 요청자 컨텍스트가 없으므로, 인가 필터 없이
// 공개범위·소유자만으로 묶는다 — 원격 접근 자체는 remote.read 권한이 이미 통제한다.
type DashboardSnapshotShim struct {
	repo storage.DashboardRepository
}

// NewDashboardSnapshotShim 은 신규 저장소를 감싼 레거시 리더를 만든다.
func NewDashboardSnapshotShim(repo storage.DashboardRepository) *DashboardSnapshotShim {
	return &DashboardSnapshotShim{repo: repo}
}

// Get 은 (scope, owner) 에 해당하는 대시보드를 합성해 반환한다.
//
//	scope="global" → visibility != 'private' 전량
//	scope="user"   → owner 소유의 visibility = 'private' 전량
//
// 구 저장소와 달리 "없음" 이 성립하지 않는다 — 대시보드가 0장이어도 빈 묶음을
// 반환한다. 원격 프록시는 ErrDashboardNotFound 를 빈 상태로 환원하므로
// (cmd/xflowd/remote_query.go queryDashboard) 두 경로의 관측 결과는 동일하다.
func (s *DashboardSnapshotShim) Get(ctx context.Context, scope, owner string) (*storage.DashboardSnapshot, error) {
	items, err := s.repo.List(ctx, true)
	if err != nil {
		return nil, err
	}

	included := make([]storage.Dashboard, 0, len(items))
	for i := range items {
		d := items[i]
		if scope == "user" {
			if d.Visibility == "private" && d.Owner == owner {
				included = append(included, d)
			}
			continue
		}
		if d.Visibility != "private" {
			included = append(included, d)
		}
	}
	return SynthesizeDashboardSnapshot(scope, owner, included), nil
}

// emptyArrayIfBlank 은 없거나 JSON null 인 값을 빈 배열로 정규화한다.
// panels / layout 은 클라이언트가 항상 배열로 기대한다.
func emptyArrayIfBlank(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`[]`)
	}
	return raw
}

// blankToNil 은 없거나 JSON null 인 값을 nil 로 만들어 결과 JSON 에서 키를 생략한다
// (omitempty). 원본에 없던 설정을 shim 이 지어내지 않는다.
func blankToNil(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}
