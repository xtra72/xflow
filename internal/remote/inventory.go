// inventory.go 는 M4 클라이언트 측 인벤토리 미러링 엔진을 구현한다
// (@SPEC:SPEC-REMOTE-001 M4, spec §5.5 client / §1.5, REQ-E01/E02/E07, A04/A07, F06).
//
// 디커플링(import cycle 회피, M3 applier 와 동일 패턴):
//
//	internal/remote 는 internal/api/service / internal/api/handler 를 import 하지
//	않는다(handler 가 remote 를 import 하므로 역방향은 cycle). 본 파일은 중립
//	InventorySource 인터페이스(ListFlows/ListAgents/ListDevices → []InventoryItem)
//	만 정의하고, 구체 어댑터(FlowServiceAdapter 등) 바인딩은 cmd/xflowd 가 담당한다.
//	redaction(F06)은 cmd/xflowd 어댑터가 수행하여 이미 redacted 된 Definition 을
//	채운 InventoryItem 을 반환한다(remote 는 handler.RedactSensitiveConfig 에 비의존).
//
// 노출(exposure) 평가(REQ-E07/A04):
//
//	remote 는 ClientConfig.Exposure(all|none|명시 목록)를 보유하므로 노출 필터링은
//	여기서 수행한다. Source 는 노출 후보 전체를 redacted 로 반환하고, mirrorEngine 이
//	정책에 따라 필터한다.
//
// 델타 소스 결정(REQ-E02):
//
//	데몬에는 flow/agent/device 변경을 remote 가 구독할 수 있는 깔끔한 이벤트 소스가
//	없다(EventPublisher 는 모니터링 Hub 로의 단방향 브로드캐스트일 뿐 구독 버스가
//	아니며, engine/agent-manager 콜백은 이미 EventPublisher 에 묶여 있어 generic
//	subscribe API 가 아니다). 기존 코드 경로를 회귀 위험 없이 후킹할 수 없으므로,
//	본 구현은 **주기적 스냅샷 + diff(poll-based)** 를 델타 소스로 채택한다. 이는
//	cycle-free 하고 단위 테스트 가능하며 add/update/remove 세 op 를 자연히 도출한다.
//	poll 기반임을 log 로 명시한다(무성한 누락 방지). 향후 이벤트 소스가 생기면
//	pushDeltas 를 이벤트 트리거로 대체할 수 있다(seam).
package remote

import (
	"bytes"
	"context"
	"strings"
)

// ExposeAll / ExposeNone 은 노출 정책 문자열의 특수값이다(REQ-A04, OPEN Q5).
// 그 외 값은 쉼표 구분 명시 목록(ID 또는 Name 매칭)으로 해석한다.
const (
	ExposeAll  = "all"
	ExposeNone = "none"
)

// InventorySource 는 노드의 로컬 자원 인벤토리를 중립 InventoryItem 으로 제공한다
// (REQ-E01/E04). 반환 항목의 Definition 은 이미 redaction(F06)된 상태여야 한다.
//
// cmd/xflowd 가 FlowServiceAdapter/AgentServiceAdapter/device 레지스트리를 이
// 인터페이스로 어댑트하여 주입한다(import cycle 회피 — spec §5.5).
type InventorySource interface {
	// ListFlows 는 노출 후보 플로우 전체를 redacted InventoryItem 으로 반환한다.
	ListFlows(ctx context.Context) ([]InventoryItem, error)
	// ListAgents 는 노출 후보 에이전트 전체를 redacted InventoryItem 으로 반환한다.
	ListAgents(ctx context.Context) ([]InventoryItem, error)
	// ListDevices 는 노출 후보 IoT 디바이스 전체를 redacted InventoryItem 으로 반환한다.
	ListDevices(ctx context.Context) ([]InventoryItem, error)
}

// applyExposure 는 노출 정책에 따라 항목을 필터한다(REQ-E07/A04).
//
//   - "all"            → 전체 노출.
//   - ""/"none"        → 전부 비노출(보수적 opt-in 기본 — A04).
//   - 그 외(명시 목록)  → 쉼표 구분 토큰과 ID 또는 Name 이 일치하는 항목만 노출.
//
// 입력 슬라이스를 변경하지 않고 새 슬라이스를 반환한다(빈 결과는 비-nil 빈 슬라이스).
func applyExposure(policy string, items []InventoryItem) []InventoryItem {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case ExposeAll:
		out := make([]InventoryItem, len(items))
		copy(out, items)
		return out
	case "", ExposeNone:
		return []InventoryItem{}
	default:
		allow := parseExposureList(policy)
		out := make([]InventoryItem, 0, len(items))
		for _, it := range items {
			if _, ok := allow[it.ID]; ok {
				out = append(out, it)
				continue
			}
			if _, ok := allow[it.Name]; ok {
				out = append(out, it)
			}
		}
		return out
	}
}

// parseExposureList 는 쉼표 구분 노출 목록을 토큰 집합으로 파싱한다(공백 trim).
func parseExposureList(policy string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, tok := range strings.Split(policy, ",") {
		tok = strings.TrimSpace(tok)
		if tok != "" {
			out[tok] = struct{}{}
		}
	}
	return out
}

// itemsEqual 은 두 InventoryItem 이 미러 관점에서 동일한지 판정한다(델타 update 검출용).
// Definition 은 바이트 비교한다(이미 redacted 동일 인코딩 가정).
func itemsEqual(a, b InventoryItem) bool {
	return a.ID == b.ID &&
		a.Name == b.Name &&
		a.Kind == b.Kind &&
		a.Status == b.Status &&
		a.UpdatedAt == b.UpdatedAt &&
		bytes.Equal(a.Definition, b.Definition)
}

// diffItems 는 old → new 변경을 add/update/remove 델타로 도출한다(REQ-E02).
//
// 결정성: add/update 는 new 순서, remove 는 old 순서로 안정 출력한다(테스트 결정성).
// 동일 ID 가 old/new 모두에 있고 내용이 다르면 update, 동일하면 무변경(델타 없음).
func diffItems(kind string, oldItems, newItems []InventoryItem) []InventoryDeltaPayload {
	oldByID := make(map[string]InventoryItem, len(oldItems))
	for _, it := range oldItems {
		oldByID[it.ID] = it
	}
	newByID := make(map[string]InventoryItem, len(newItems))
	for _, it := range newItems {
		newByID[it.ID] = it
	}

	var deltas []InventoryDeltaPayload

	// add/update — new 순서.
	for _, it := range newItems {
		prev, existed := oldByID[it.ID]
		switch {
		case !existed:
			deltas = append(deltas, InventoryDeltaPayload{Op: OpAdd, Kind: kind, Item: it})
		case !itemsEqual(prev, it):
			deltas = append(deltas, InventoryDeltaPayload{Op: OpUpdate, Kind: kind, Item: it})
		}
	}

	// remove — old 순서(노출 해제 포함 — A07).
	for _, it := range oldItems {
		if _, stillPresent := newByID[it.ID]; !stillPresent {
			deltas = append(deltas, InventoryDeltaPayload{
				Op:   OpRemove,
				Kind: kind,
				Item: InventoryItem{ID: it.ID, Kind: kind},
			})
		}
	}

	return deltas
}
