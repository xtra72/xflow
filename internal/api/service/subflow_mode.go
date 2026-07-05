package service

import "strings"

// ---------------------------------------------------------------------------
// flow-node 참조 실행 모드 (SPEC-SUBFLOW-002 그룹 M)
// ---------------------------------------------------------------------------
//
// flow-node config 의 `mode` 키는 LOCAL(bare id) 참조의 실행 방식을 결정한다.
//   - flowModeShared(기본)  : 로컬 in-process 라이브 브리지(미확장 — 공유 단일 인스턴스 연결).
//   - flowModeInstance       : SPEC-SUBFLOW-001 인라인 확장(네임스페이스 복제본).
//
// mode 는 참조 종류(LOCAL bare id / REMOTE remote://)와 직교한다(M04). remote:// 참조는 종류상
// 항상 라이브 브리지이며 mode 는 무시된다. bare id 일 때만 mode 가 shared/instance 를 가른다.
//
// 후속(본 백엔드 핵심 M1~M3 범위 밖):
//   - M4(통계): shared=참조 플로우 직접 / instance=임베디드 병합 / 모드 배지(subflow_stats.go TODO).
//   - M5(웹 UI): nodeSchemas.ts mode 토글·연결 상태 인디케이터·편집 반영 안내(web/).
//   - M6(마이그레이션 문서): mode 미지정→shared breaking 가이드·instance 복원 경로 문서화.

const (
	// flowNodeModeKey 는 flow-node config 의 mode 키이다(internal/node 의 flowNodeConfigMode 와 동일).
	flowNodeModeKey = "mode"

	// flowModeShared 는 공유 단일 인스턴스 라이브 브리지 모드이다(기본값 — M02).
	flowModeShared = "shared"

	// flowModeInstance 는 인라인 확장(복제본) 모드이다(SPEC-SUBFLOW-001 보존).
	flowModeInstance = "instance"
)

// normalizeFlowNodeMode 는 flow-node config 의 mode 값을 결정적으로 정규화한다(M02/M03).
//
//   - 미지정(키 없음)/빈 문자열/공백/대소문자 변형 → flowModeShared(기본 — breaking).
//   - "instance"(대소문자·공백 무시) → flowModeInstance.
//   - "shared"(대소문자·공백 무시) → flowModeShared.
//   - 그 외 알 수 없는 값 → flowModeShared 로 정규화(동작 일관 — M03). 호출자가 필요 시 경고.
//
// 알 수 없는 값을 에러로 거부하지 않고 기본 shared 로 정규화하는 이유: 저장된 정의의 오타/미래
// 값으로 인해 배포가 깨지지 않도록 하되(견고성), 의미는 항상 결정적이고 문서화된 기본값을
// 따른다(M03 — "동작은 일관·문서화"). 명시적 거부가 필요하면 별도 검증 단계에서 수행한다.
func normalizeFlowNodeMode(cfg map[string]any) string {
	if cfg == nil {
		return flowModeShared
	}
	raw, _ := cfg[flowNodeModeKey].(string)
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case flowModeInstance:
		return flowModeInstance
	default:
		// "", "shared", 알 수 없는 값 모두 기본 shared.
		return flowModeShared
	}
}
