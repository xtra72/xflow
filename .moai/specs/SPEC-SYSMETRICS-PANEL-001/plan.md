# SPEC-SYSMETRICS-PANEL-001 구현 계획 (plan.md)

방법론: Hybrid (신규 코드는 TDD, 기존 파일 수정은 DDD). `.moai/config/sections/quality.yaml` 의 `development_mode: hybrid` 를 따른다.

마일스톤 순서는 의존 방향을 따른다 — 백엔드 스냅샷이 없으면 프론트가 폴링할 대상이 없다.

---

## M1 — 에이전트 상태 스냅샷 (백엔드)

담당 REQ: 01, 02, 03, 04, 05

**신규**: `internal/agent/system/sysmetrics_state.go`

- 스냅샷 보관 구조체. `SysMetricsAgent` 에 필드 하나를 더한다(`mu` 는 기존 것을 쓴다 — 별도 락을 두면 두 락의 순서 규칙이 생긴다).
- `State() map[string]any` 구현 + `var _ agent.StatefulAgent = (*SysMetricsAgent)(nil)` 컴파일 타임 고정.
- `targets` 는 설정값이 아니라 **실제 표본에 등장한 이름**으로 채운다. 설정이 비어 있으면(=전체) 무엇이 잡히는지는 표본만이 안다.

**수정**: `sysmetrics_agent.go` `emitSample`

```
collectSample(...)
  → 스냅샷 갱신            ← 여기가 먼저 (REQ-02)
  → json.Marshal
  → select { ch <- payload / done / default }
```

순서가 계약이다. 채널 전송 뒤로 옮기면 플로우 없는 사용자에게 패널이 영원히 빈다.

**테스트** (`sysmetrics_state_test.go`):
- 표본 전 `State()` → `status: "no_sample"`
- 표본 1회 후 필드 구성 + `collected_at`
- 채널 버퍼를 가득 채운 상태에서 표본을 돌려도 스냅샷이 갱신되는지 (REQ-02 의 회귀 가드)
- 수집 비활성 지표는 키 자체가 없는지 (REQ-21 의 백엔드 절반)
- `-race` 로 표본 루프 + `State()` 동시 호출

**완료 기준**: `go test -race ./internal/agent/system/` 통과, `GET /agents/{id}?detail=full` 응답에 `state` 가 실린다.

---

## M2 — 스냅샷 → 시리즈 변환 (프론트, 순수 함수)

담당 REQ: 07, 11, 12, 14, 15, 21

**신규**: `web/src/pages/dashboard/panels/sysmetrics/sysMetricsSeries.ts`

순수 함수만 둔다. 차트·React 를 섞지 않아야 테스트가 빨라지고 rate 계산의 경계(되감김·0 경과시간·결측 대상)를 정면으로 검증할 수 있다.

- `toSnapshot(state)` — API `state` 맵을 타입 있는 스냅샷으로. 알 수 없는 키는 버린다.
- `sumDiskIO(snapshot)` / `sumNetwork(snapshot)` — 전체 장치·인터페이스 합산 (REQ-07)
- `sumStorage(snapshot)` — 마운트 합계 + 합계 기준 사용률 (REQ-14). 중복 볼륨은 합치지 않는다 (REQ-15)
- `deltaRate(prev, next, unitTime)` — 누적 차분. 되감김이면 0 (REQ-12), 경과 0ms 이면 직전 값 유지
- `isCollected(snapshot, metric)` — 수집 꺼짐 판별 (REQ-21)

**테스트**: `sysMetricsSeries.test.ts` — 위 5함수의 정상·경계. 특히 되감김과 `collected_at` 동일(중복 표본) 케이스.

**완료 기준**: `vitest` 통과. 이 단계에서는 화면에 아무것도 안 나온다 — 의도된 것이다.

---

## M3 — 폴링 훅 + 설정 읽기

담당 REQ: 08, 16, 18, 20

**신규**:
- `useSysMetricsSnapshot.ts` — `getAgent(agentId, 'full')` 를 `refreshMs` 주기로 폴링. 쿼리 키를 `['agent', agentId, 'full']` 로 두어 같은 에이전트를 보는 여러 패널이 캐시를 공유한다(위험표 2번 완화).
- `sysMetricsPanelConfig.ts` — `readPanelItems` / `readMaxCols` / `readRefreshMs` / `readWindowSec` / `readTargets` / `readAgentRef`. monitor 쪽 함수를 그대로 쓸 수 있는 것은 재작성하지 않고 import 한다.

에이전트 부재·중지 폴백(REQ-20)은 훅이 판정 결과를 함께 돌려주고 세 패널이 같은 안내 컴포넌트를 쓴다.

**테스트**: `useSysMetricsSnapshot.test.tsx` — 정상 폴링, 에이전트 404, 중지 상태, `collected_at` 중복 시 창 미증가.

---

## M4 — 세 패널 컴포넌트

담당 REQ: 06, 09, 10, 13, 17

순서: 시스템 → 네트워크 → 스토리지. 시스템 패널이 항목 렌더 골격을 정하고 나머지가 따른다.

- `SysMetricsSystemPanel.tsx` — CPU/메모리 타일 + IO·네트워크 종합 (REQ-06, 07)
- `SysMetricsNetworkPanel.tsx` — `NetworkChart` 재사용. 대상 미선택이면 합산 1개 (REQ-09, 10)
- `SysMetricsStoragePanel.tsx` — 마운트별 사용률 막대 + 용량. 미선택이면 합계 (REQ-13, 14)

공통: `useAutoColumns` 로 폭 기반 열 축소, `usePanelTitleVisible` 로 제목 처리 — monitor 패널과 같은 크롬 규약.

**테스트**: `SysMetricsPanels.test.tsx` — 각 패널의 항목 선택, 종합/개별 전환, 수집 꺼짐 표시, 에이전트 부재 폴백.

---

## M5 — 카탈로그 · 설정 · 렌더 배선

담당 REQ: 16, 17, 19

- `web/src/types/dashboard.ts` — `PanelType` 에 3종 추가
- `AddPanelDialog.tsx` — `system` 카테고리를 두 그룹으로 나눈다: 기존 monitor 5종(`labelKey: dashboard.addPanel.groups.monitor`) + 신규 3종(`groups.sysmetrics`). 지금 `system` 은 제목 없는 단일 그룹이므로 **그룹 제목이 처음 생긴다** — 기존 5종 쪽에도 제목을 달아야 한 쪽만 제목이 붙는 어색함이 없다.
- 에이전트 선택 스텝: 기존 `needsAgentStatus`(전체 타입)와 `needsAgent`(modbus-gateway 한정)가 있다. sysmetrics 한정 필터가 필요하므로 **선택 스텝의 타입 필터를 파라미터화**한다 — 유형마다 불리언 플래그를 늘리는 현재 방식은 세 번째부터 무너진다.
- `renderDashboardPanel.tsx` — 3 케이스 추가
- `PanelSettingsDialog.tsx` — 대상 선택 섹션에 `SysResourceSelector` 배치 (패널 유형별로 `kind` 가 다르다: network→interfaces, storage→mountpoints, system→없음)
- `i18n` ko/en — 패널 라벨·설명·항목 이름·"수집 꺼짐"·"에이전트 없음" 문구

**테스트**: `AddPanelDialog` 카탈로그 테스트에 3종 등장 + 그룹 분리 단언, `renderDashboardPanel` 분기 테스트.

---

## M6 — 검증 · 문서

- `go test -race ./internal/...`
- `npx vitest run` (web 전체)
- `npx tsc --noEmit -p tsconfig.json`
- 수동: sysmetrics 에이전트 1개 생성 → **플로우 없이** 세 패널 배치 → 값이 그려지는지 (M1 REQ-02 의 실사용 확인)
- CHANGELOG `[Unreleased]` 항목 추가
- `nodeTypeMeta` 는 노드 문서라 무관. 패널 문서 표면은 i18n 설명이 전부다.

---

## 의존 관계

```
M1 (백엔드 스냅샷)
 └─→ M2 (순수 변환)  ─┐
                      ├─→ M4 (패널) ─→ M5 (배선) ─→ M6 (검증)
     M3 (훅·설정)  ───┘
```

M2 와 M3 은 서로 독립이므로 순서를 바꿔도 된다. M1 은 반드시 먼저다.

---

## 결정 기록

| 결정 | 선택 | 근거 |
|---|---|---|
| 데이터 소스 | 에이전트 직접 조회 | 사용자 선택. 플로우 구성 없이 즉시 동작하며 신규 API 도 필요 없다(`detail=full` 재사용) |
| 기존 monitor 패널 | 분리 유지 | 사용자 선택. 기존 대시보드 무영향, 마이그레이션 불필요 |
| 패널 분할 | 3종 + 대상 선택 | 사용자 선택. 종합/개별을 유형이 아니라 설정으로 가른다 |
| rate 계산 위치 | 브라우저 | 기존 `monitor-network` 와 동일. 서버가 상태를 더 들고 있을 이유가 없다 |
| 스냅샷 갱신 시점 | 채널 전송 **이전** | 플로우 미구성 사용자에게 채널은 항상 가득 참 — 전송에 매달면 패널이 죽는다 |
| 중복 볼륨 | 합치지 않음 | 같은 볼륨인지 OS 마다 다르고 에이전트도 모른다. 임의 병합은 사용자가 고른 대상을 지운다 |
