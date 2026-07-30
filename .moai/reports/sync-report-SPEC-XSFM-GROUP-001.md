# Sync Report — SPEC-XSFM-GROUP-001

- **SPEC**: SPEC-XSFM-GROUP-001 — xsfm 그룹 1급 개념 도입 (그룹 엔티티 · 다대다 멤버십 · 일괄 제어)
- **동기화 일자**: 2026-07-30
- **구현 커밋**: `ae53b344`(백엔드 M1~M5) + `97c2bafd`(프런트 M6~M7)
- **브랜치**: `feature/SPEC-XSFM-GROUP-001`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier M), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md / plan.md / acceptance.md | `status` | `draft` | `completed` |
| spec.md / plan.md / acceptance.md | `version` | `0.2.0` | `0.3.0` |
| spec.md / plan.md / acceptance.md | `updated` | `2026-07-30` | `2026-07-30` (유지) |

- spec.md HISTORY 에 `0.3.0` 행 추가(M1~M7 구현 완료).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§6 구현 노트(as-implemented)** 신설, plan.md M4/M7 에 as-implemented 정련 주석 추가.

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-XSFM-GROUP-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가; **§6 구현 노트(as-implemented) IN-1~IN-4 신설** |
| `.moai/specs/SPEC-XSFM-GROUP-001/plan.md` | frontmatter `status`/`version` 갱신; M4 에 IN-1(순수 파생) 주석, M7 에 IN-2(가산형 패널)+IN-3(파일 배치) 주석 |
| `.moai/specs/SPEC-XSFM-GROUP-001/acceptance.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0` (본문 무변경) |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — xsfm 그룹 1급 개념 도입" 항목 추가(기능 + as-implemented 분기 4건 요약) |
| `.moai/project/product.md` | "제공 Agent 목록" 표의 Subway Facilities Manager 행에 그룹 1급 개념 요약 추가 |
| `.moai/project/structure.md` | `internal/agent/xsfm/` 항목에 그룹 1급 개념 확장 문단 추가(신규 파일·다대다·파생·CRUD·셀렉터·프런트) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `README.md` | 저가치 스킵. (1) xsfm 전용 example YAML(`examples/agents\|flows/`)이 없어 example 표에 추가할 행 없음. (2) README 에는 xsfm 에이전트 명령/기능 전용 섹션이 없고 `internal/agent/` 트리도 비완전 예시 목록(선행 SPEC-XSFM-001 sync 에서도 동일 사유로 스킵). 그룹 기능 없이도 README 가 실질적으로 불완전해지지 않음 → scope discipline 준수. |
| `.moai/project/tech.md` | 중복. 그룹은 순수 논리 레이어(MQTT 규약·외부 의존성 불변)로 신규 기술 스택 없음. |
| `.moai/project/architecture.md` | 해당 없음. 그룹 레이어는 기존 에이전트 아키텍처 내 비침습 추가로 상위 아키텍처 변경 없음. |

---

## as-implemented 분기 (Divergence — spec-anchored Level 2)

계획(plan.md/§4) 대비 4건의 정련이 있으며, 모두 **비침습성·무회귀를 강화**하는 방향이다(스코프 확장 아님). spec.md §6 IN-1~IN-4 에 정식 기록됨.

| # | 항목 | 계획 | 실제 구현 | 판정 |
|---|------|------|-----------|------|
| IN-1 | M4 기본 그룹 동기화 | `handleAddStation`/`handleRemoveStation` 성공 직후 station 그룹 upsert/remove **명시적 훅**(§4.3) | station/line 그룹을 `StationRegistry` 에서 **조회 시 순수 파생** — 훅 삽입 없음, **`station_registry.go` 완전 불변** | 강화형 정련(AC 4.1~4.5 + REQ-04-05 충족) |
| IN-2 | M7 패널 개편 | "설비 역사 패널 → 설비 그룹 패널 **개편**"(REQ-06-05) | 기존 `FacilityStationPanel` 보존 + 신규 `FacilityGroupPanel`(`facility-group` 타입) **가산(additive)** | 무회귀 강화(15개 역사-패널 테스트 보존, AC 6.5 충족) |
| IN-3 | 그룹 로직 파일 배치 | `agent.go`/`control.go` 확장 시사 | 신규 `group_membership.go` 에 에이전트-레벨 로직 배치(`agent.go` diff 최소화). `handleSelectorControl` 는 본 코드베이스상 `group.go` 에 존재 → 게이트는 `group.go` 정련, **`control.go` 불변** | 정상 분해(비침습 강화) |
| IN-4 | 미접두사 group_id | 접두사 인코딩 id(RD-2) | 접두사 없는 `group_id` 는 `Device.GroupID`(primary) 1차 대조 fallback | 하위호환(NF-02 무회귀), RD-1/RD-2 정합 |

---

## 확정 설계 결정(RD-1~4) 반영 확인

| 결정 | 구현 반영 |
|------|-----------|
| RD-1 primary group 유지 | `Device.GroupID` = 대표 그룹 유지, 단일 `group_id` 태그 방출 경로 무회귀 |
| RD-2 접두사 id | `station:`/`line:`/`custom:` 접두사 인코딩, `GroupMembers` 접두사 분기 |
| RD-3 로스터 대조 필터 | 조회/fan-out 시 실재 device_id 만 통과, `remove_device` 비침습 |
| RD-4 셀렉터 병존 | `line`/`station` 셀렉터 ↔ `group_id=line:/station:` 병존, 동일 경로 수렴 |

---

## 품질 게이트 (관측 증거)

### sync-phase 직접 재검증 (docs-only, 코드 무변경)

| 항목 | 명령 | 결과 |
|------|------|------|
| 빌드 | `go build ./internal/agent/xsfm/...` | exit=0 (`.moai/state/verify/xsfm-group/build.log`) |
| xsfm 커버리지 | `go test -cover ./internal/agent/xsfm/...` | `ok ... coverage: 88.8% of statements`, exit=0 (`.moai/state/verify/xsfm-group/cover.log`) |
| 신규 파일 존재 | `ls internal/agent/xsfm/group_{registry,membership}.go` | 둘 다 존재(406 + 462 라인) |
| `station_registry.go`/`control.go` 무변경 | `git show --stat ae53b344 97c2bafd -- .../station_registry.go .../control.go` | 두 커밋 모두 미변경(빈 diff) → IN-1/IN-3 substantiated |
| 백엔드 diff 요약 | `git show --stat ae53b344` | `group_registry.go`(+406) `group_membership.go`(+462) 신설, `errors.go`(+9, 센티널 3종), `group.go`(+8), `agent.go`(±48) |

### run-phase 보고 지표 (커밋 시점, 본 sync 에서 재실행하지 않음)

- 백엔드: `go test -race` 클린, xsfm 커버리지 88.8%(sync 재검증에서 동일 관측).
- 프런트(M6~M7, `97c2bafd`): vitest 2254 pass, `tsc` 클린.

> 미검증(Gaps): 프런트 vitest/`tsc` 는 본 sync(docs-only)에서 재실행하지 않고 run-phase 커밋 보고를 인용했다. 전체 Go 스위트(무관 패키지 `internal/agent/serial`·`century` 의 pre-existing 실패 포함)는 재실행하지 않았다.

---

## 비고

- 소스 코드/테스트 파일(`internal/**`, `web/**`)은 **수정하지 않았다**(문서/SPEC 전용).
- 무관한 pre-existing 미커밋 변경(`internal/node/*`, `web/src/config/nodeSchemas*`, `web/src/lib/flow/nodeType.ts`, 루트 `data.json`/`output.json`/`packet.json` 등)은 **손대지 않았다**.
- 커밋/푸시/PR 생성은 하지 않았다(오케스트레이터가 sync 커밋 + PR 을 별도 처리).
