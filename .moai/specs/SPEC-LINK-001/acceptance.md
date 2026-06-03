# SPEC-LINK-001 인수 기준 (Acceptance Criteria)

> 가상(네임드) 링크 — 와이어 가상화 표시로 캔버스 연결 간소화
> Given-When-Then 형식. 핵심 불변식: **엔진 라우팅/메시지 전달 동작 불변.**

## 1. 인수 시나리오

### AC-01: 와이어 가상화 — 선 숨김 + 엔드포인트 배지 (REQ-LINK-010, 020~022)

- **Given** 소스 노드와 타겟 노드가 와이어 하나로 연결되어 있고,
- **When** 사용자가 그 와이어를 `virtual=true`, `name="sensor"` 로 설정하면,
- **Then** 캔버스에서 두 노드 사이의 긴 연결선이 사라지고,
- **And** 소스(출력) 포트 옆에 "출력 링크 sensor" 배지가 표시되고,
- **And** 타겟(입력) 포트 옆에 "입력 링크 sensor" 배지가 표시되며,
- **And** 메시지는 여전히 동일하게 전달된다(라우팅 불변).

### AC-02: 이름 기반 그룹 표시 + 클릭 하이라이트 (REQ-LINK-023, 024)

- **Given** 같은 플로우 내에 `name="sensor"` 인 가상 와이어가 여러 개 있고,
- **When** 사용자가 한 가상 링크 배지를 클릭하면,
- **Then** 같은 이름("sensor") 그룹/상대 엔드포인트들이 하이라이트되어 어디에 연결되어 있는지 보이고,
- **And** 표시는 이름 라벨만 사용한다(자동 색상/배지 색 구분 없음).

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

- [ ] REQ-LINK-001 ~ 031 모두 구현 및 검증.
- [ ] 신규 코드 커버리지 85% 이상.
- [ ] 엔진 회귀 0건(큐/카운터/브로드캐스트/순서 불변 입증).
- [ ] 저장-로드, export-import 라운드트립에서 `virtual`/`name` 보존 테스트 통과.
- [ ] 선택-only dirty 금지 / 편집-only dirty 테스트 통과.
- [ ] 비가상 와이어 렌더 회귀 없음(CustomEdge 렌더 테스트).
- [ ] Go `go test -race` 클린, `go vet`/lint 클린.
- [ ] 프론트 Vitest/Testing Library 통과, TypeScript strict 통과.

## 4. 검증 방법 및 도구

| 영역 | 도구 | 검증 대상 |
|------|------|-----------|
| Wire 직렬화 | Go testing + testify | `virtual`/`name` JSON 보존 |
| 어댑터 통과 | Go testing | convertReactFlowEdgesToWires `virtual` 통과 |
| 라운드트립 | Go testing | 저장-로드 / export-import 보존 |
| 엔진 불변 | 기존 엔진 회귀 테스트 (`-race`) | 큐/카운터/브로드캐스트/순서 |
| 가상화 렌더 | Vitest + Testing Library | 선 숨김 + 배지 표시 |
| 하이라이트 | Vitest | 같은 이름 그룹/엔드포인트 강조 |
| dirty 정책 | Vitest (editorStore) | 선택 vs 편집 |

## 5. OPEN QUESTIONS (인수 전 확정 필요)

1. 같은 소스 포트 + 같은 이름의 가상 와이어 N개 → 출력 링크 배지 1개로 합칠지, 와이어별로 둘지.
2. 링크 클릭 하이라이트 → "숨긴 선을 일시적으로 그림" vs "엔드포인트만 강조".
3. 배지 구현 → 포트 핸들 옆 라벨(`NodeHandle`/`CustomNode`) vs 별도 오버레이.
