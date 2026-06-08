# SPEC-LINK-001 인수 기준 (Acceptance Criteria)

> 가상(네임드) 링크 — 와이어 가상화 표시로 캔버스 연결 간소화
> Given-When-Then 형식. 핵심 불변식: **엔진 라우팅/메시지 전달 동작 불변.**

## 1. 인수 시나리오

### AC-01: 와이어 가상화 — 선 숨김 + 컴팩트 인디케이터 (REQ-LINK-010, 020~022)

> 구현 반영(v1.1.0): 인라인 이름 배지 → 컴팩트 LinkIndicator + 측면 팝오버.

- **Given** 소스 노드와 타겟 노드가 와이어 하나로 연결되어 있고,
- **When** 사용자가 그 와이어를 `virtual=true`, `name="sensor"` 로 설정하면,
- **Then** (기본 숨김 모드에서) 캔버스의 두 노드 사이 긴 연결선이 사라지고,
- **And** 소스(출력) 포트 옆에 컴팩트 LinkIndicator(링크 아이콘 + 개수)가 표시되고,
- **And** 타겟(입력) 포트 옆에 컴팩트 LinkIndicator 가 표시되며,
- **When** 출력 포트의 인디케이터를 클릭하면 노드 **오른쪽**에 팝오버 목록이 뜨고(입력 포트는 **왼쪽**),
- **And** 목록에 이름 그룹("sensor")별 1행으로 상대 끝점(상대 `노드:포트`)이 표시되며,
- **And** 메시지는 여전히 동일하게 전달된다(라우팅 불변).

### AC-02: 이름 기반 그룹 표시 + 팝오버 선택 하이라이트 (REQ-LINK-023, 024)

- **Given** 같은 플로우 내에 `name="sensor"` 인 가상 와이어가 여러 개 있고,
- **When** 사용자가 인디케이터를 클릭해 팝오버를 열고 그 안에서 "sensor" 항목을 선택하면,
- **Then** 같은 이름("sensor") 그룹의 숨겨진 와이어가 또렷한 파란 dashed 선으로 일시 표시되고 상대 엔드포인트들이 하이라이트되어 어디에 연결되어 있는지 보이고,
- **And** 표시는 이름 라벨만 사용한다(자동 색상/배지 색 구분 없음),
- **And** 빈 이름 그룹은 그룹 식별이 불가하므로 하이라이트되지 않는다.

### AC-03: 가상화 해제 — 다시 선으로 표시 (REQ-LINK-010, 025)

- **Given** `virtual=true` 인 가상 와이어가 배지로 표시되고 있고,
- **When** 사용자가 `virtual=false` 로 되돌리면,
- **Then** 두 노드 사이에 다시 연결선(베지어 곡선)이 표시되고,
- **And** 메시지 전달 동작은 변하지 않는다(라우팅 불변).

### AC-04: 저장/로드 및 export/import 보존 (REQ-LINK-003, 004)

- **Given** `virtual=true`, `name="sensor"` 인 와이어가 포함된 플로우가 있고,
- **When** 플로우를 저장한 뒤 다시 로드하거나, export 한 뒤 다시 import 하면,
- **Then** 해당 와이어의 `virtual`/`name` 값이 그대로 유지된다.

### AC-05: 하위 호환 — 기존/비가상 와이어 불변 (REQ-LINK-025, 030, 031)

- **Given** `virtual` 필드가 없는 기존 플로우(또는 `virtual=false` 와이어)가 있고,
- **When** 플로우를 로드하고 실행하면,
- **Then** 와이어는 기존과 동일하게 연결선으로 렌더링되고,
- **And** 메시지 전달/큐/카운터 동작이 기존과 100% 동일하다(회귀 0).

### AC-06: dirty 정책 — 선택만으로 dirty 금지 (REQ-LINK-012)

- **Given** 저장된(미변경) 플로우가 열려 있고,
- **When** 사용자가 와이어를 선택(클릭)만 하면,
- **Then** 미저장(dirty/빨간점) 상태가 되지 않고,
- **And** `virtual` 토글이나 `name` 을 실제로 편집하면 그때 dirty 상태가 된다.

### AC-07: 가상 와이어 표시 토글 — 숨김/점선 전환 (REQ-LINK-040)

- **Given** `virtual=true` 인 와이어가 있고 가상 와이어 표시 토글이 꺼져 있고(기본),
- **Then** 그 와이어의 선은 캔버스에 그려지지 않고 포트 옆 LinkIndicator 로만 표시되며,
- **When** 사용자가 툴바의 "가상 와이어 표시" 토글을 켜면,
- **Then** 그 와이어가 **점선(dotted)** 으로 렌더되어 일반 실선 연결과 구분되고,
- **And** 토글 전환만으로는 dirty/undo/저장이 발생하지 않는다(순수 표시 상태).

### AC-08: 클릭/선택 시 가상 와이어 일시 표시 (REQ-LINK-041)

- **Given** `virtual=true` 인 와이어가 있고,
- **When** (숨김 모드든 표시 모드든) 그 와이어가 선택되거나 그 이름 그룹이 하이라이트되면,
- **Then** 해당 가상 와이어가 또렷한 파란 dashed 선으로 드러나며(클릭으로 연결 확인),
- **And** 표시 모드의 점선(dotted)과 클릭 강조의 dashed 는 시각적으로 구분된다.

### AC-09: 방향성 연결 포커스 (REQ-LINK-042)

- **Given** 연결 포커스 토글이 켜져 있고 정확히 한 노드 N 이 선택되어 있고,
- **When** 캔버스를 보면,
- **Then** N 의 하류(N 의 출력에서 `source→target` 으로 따라간 노드/엣지) 와 상류(N 의 입력으로 `target→source` 로 따라간 노드/엣지) 만 강조되고,
- **And** 강조된 노드 **및** 그 경로의 엣지가 함께 강조되며 나머지는 흐리게(dim) 처리되고,
- **And** N 의 순수 상/하류가 아니라 단지 공통 이웃을 공유할 뿐인 형제(sibling) 노드는 강조에서 제외된다,
- **And** 포커스 토글/변경만으로는 dirty/undo/저장이 발생하지 않는다.

### AC-10: 연결 단계(focusDepth) 1..5/전체 (REQ-LINK-043)

- **Given** 연결 포커스가 켜져 있고,
- **When** `focusDepth` 를 1 로 두면,
- **Then** 선택 노드 + 직접 상/하류 이웃(1 hop)만 강조되고,
- **When** `focusDepth` 를 늘리면 각 방향으로 그 hop 수만큼 체인이 확장되며(유한 값은 1..5 로 클램프),
- **When** "전체" 를 선택하면 도달 가능한 전체 방향성 연결 체인이 강조된다(사이클은 무한 루프 없이 종료).

## 2. 백엔드 인수 기준

### AC-B1: Wire.virtual 필드 (REQ-LINK-001)

- **Given** `flow.Wire` 구조체,
- **When** 와이어를 직렬화/역직렬화하면,
- **Then** `virtual`(bool, 기본 false) 필드가 JSON 키 `virtual` 로 보존되고, 기존 `Name` 도 보존된다.

### AC-B2: 어댑터 통과 (REQ-LINK-002)

- **Given** React Flow edge 가 `{ virtual: true, name: "sensor", ... }` 메타를 가지고,
- **When** `convertReactFlowEdgesToWires` 가 wire 로 변환하면,
- **Then** 변환된 wire 에 `virtual=true`, `name="sensor"` 가 보존되고, `mode`/`buffer_size` 정책은 변하지 않는다.

### AC-B3: 엔진 라우팅 불변 (REQ-LINK-005, 006)

- **Given** `virtual=true` 인 와이어와 동일 토폴로지의 `virtual=false` 와이어,
- **When** 동일 메시지를 흘려보내면,
- **Then** 큐/카운터/브로드캐스트/메시지 순서가 두 경우 완전히 동일하고,
- **And** 엔진 와이어 라우팅 코드는 변경되지 않았다.

## 3. 품질 게이트 (Definition of Done)

- [ ] REQ-LINK-001 ~ 031, 040 ~ 044 모두 구현 및 검증.
- [ ] 신규 코드 커버리지 85% 이상.
- [ ] 엔진 회귀 0건(큐/카운터/브로드캐스트/순서 불변 입증).
- [ ] 저장-로드, export-import 라운드트립에서 `virtual`/`name` 보존 테스트 통과.
- [ ] 선택-only dirty 금지 / 편집-only dirty 테스트 통과.
- [ ] 비가상 와이어 렌더 회귀 없음(CustomEdge 렌더 테스트).
- [ ] 뷰 전용 표시 컨트롤(`showVirtualWires`/`focusConnectionsOnSelect`/`focusDepth`) 이 dirty/undo/저장/라우팅에 영향 없음 검증.
- [ ] 연결 포커스 방향성(상/하류 강조, 형제 제외) 및 depth 1..5/전체 단위 테스트 통과(`connectionFocus.test.ts`).
- [ ] Go `go test -race` 클린, `go vet`/lint 클린.
- [ ] 프론트 Vitest/Testing Library 통과, TypeScript strict 통과.

## 4. 검증 방법 및 도구

| 영역 | 도구 | 검증 대상 |
|------|------|-----------|
| Wire 직렬화 | Go testing + testify | `virtual`/`name` JSON 보존 |
| 어댑터 통과 | Go testing | convertReactFlowEdgesToWires `virtual` 통과 |
| 라운드트립 | Go testing | 저장-로드 / export-import 보존 |
| 엔진 불변 | 기존 엔진 회귀 테스트 (`-race`) | 큐/카운터/브로드캐스트/순서 |
| 가상화 렌더 | Vitest + Testing Library | 선 숨김/점선 + 컴팩트 인디케이터 + 팝오버 목록 |
| 링크 헬퍼 | Vitest (`virtualLinks.ts`) | (포트, 이름) 그룹핑 + 상대 끝점 계산 |
| 하이라이트 | Vitest | 같은 이름 그룹/엔드포인트 강조(파란 dashed) |
| 표시 토글 | Vitest (`CustomEdge`/`EditorToolbar`) | `showVirtualWires` 숨김/점선/클릭 dashed |
| 연결 포커스 | Vitest (`connectionFocus.ts`) | 방향성 BFS, depth 1..5/전체, 형제 제외, 강조/흐림 |
| dirty 정책 | Vitest (editorStore) | 선택 vs 편집; 표시 토글은 dirty 아님 |

## 5. OPEN QUESTIONS — 구현 시 확정됨 (RESOLVED)

1. 같은 (포트, 이름) 가상 와이어 N개 → **인디케이터/팝오버 항목 1개로 합침**(합쳐진 edge id 보존).
2. 링크 하이라이트 → **숨긴 선을 파란 dashed 로 일시 표시 + 엔드포인트 강조 병행.**
3. 배지 구현 → **인라인 배지 폐기, 컴팩트 LinkIndicator + 측면 LinkListPopover(NodeToolbar).**
