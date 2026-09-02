# Sync Report — SPEC-XSFM-NAMESEL-001

- **SPEC**: SPEC-XSFM-NAMESEL-001 — xsfm 이름 기반 제어 셀렉터 (device_name / group_name)
- **동기화 일자**: 2026-07-30
- **구현 커밋**: `2acb980d`(백엔드+노드 M1~M3)
- **브랜치**: `feature/SPEC-XSFM-GROUP-001`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier M), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` (핵심 M1~M3 기준) |
| spec.md | `version` | `0.2.0` | `0.3.0` |
| spec.md | `updated` | `2026-07-30` | `2026-07-30` (유지) |
| plan.md / acceptance.md | 버전 노트(top-of-file) | `0.2.0` | `0.3.0` + 상태 `completed` |

- spec.md 는 12-필드 frontmatter 형식을 유지하며 `status`/`version` 만 갱신(`updated` 는 동일 일자로 유지).
- plan.md / acceptance.md 는 frontmatter 대신 **top-of-file 버전 노트** 형식을 사용하므로 그 형식에 맞춰 버전·상태를 갱신(기존 XSFM SPEC 관례 일치).
- spec.md HISTORY 에 `0.3.0` 행 추가(M1~M3 구현 완료, M4 이연 명시).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§7 Implementation Notes(as-implemented)** 신설(분기 4건 §7.1~§7.4 + M4-이연 범위 명시 §7.5).

### "completed" 범위 명시 (overclaim 방지)

- `status: completed` 전이는 **핵심 기능(M1~M3: 에이전트 리졸버·디스패치 + 노드 pass-through + 테스트/무회귀)** 에 한한다.
- **M4(프런트엔드 이름 제어 UI)는 plan §2 에서 선택·저우선(Priority Low)으로 분류되어 이연(DEFERRED)** 되었다. 이름 셀렉터 기능은 에이전트+노드 레벨에서 이미 완전히 사용 가능하다(flow 노드 제어 라우팅 + HTTP exec 계약 `params.device_name`/`params.group_name` 승격). 따라서 전용 UI 없이도 기능이 온전하며, "completed" 는 M4 UI 를 포함하지 않는다(spec.md §7.5 명시).

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-XSFM-NAMESEL-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가(M1~M3 완료 + M4 이연); **§7 Implementation Notes(as-implemented) §7.1~§7.5 신설**(분기 4건 + M4-이연 범위 명시) |
| `.moai/specs/SPEC-XSFM-NAMESEL-001/plan.md` | top-of-file 버전 노트 `0.3.0`(status: completed) + M1~M3 구현 완료 + 검증 증거 + M4 이연 요약 추가 |
| `.moai/specs/SPEC-XSFM-NAMESEL-001/acceptance.md` | top-of-file 버전 노트 `0.3.0`(status: completed) + AC 1.x~5.x 통과 요약 + M4 이연 추가; §6 DoD 체크박스 갱신(M1~M3 반영, M4 이연 표기) |
| `CHANGELOG.md` | `[Unreleased]` 최상단 `추가` 에 "xsfm 이름 기반 제어 셀렉터(device_name/group_name)" 항목 추가(기능 + as-implemented 분기 4건 + M4 이연 요약) |
| `.moai/project/product.md` | "제공 Agent 목록" 표 Subway Facilities Manager 행에 이름 기반 제어 셀렉터 요약 추가 |
| `.moai/project/structure.md` | `internal/agent/xsfm/` 항목에 이름 셀렉터 확장 문단 추가(신규 `name_resolver.go`·우선순위 체인·전 타입 매칭·노드 pass-through·분기 4건·M4 이연) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `internal/**`, `web/**` (코드) | 본 sync 는 문서 동기화 전용. 코드는 `2acb980d` 에 이미 커밋됨(범위 밖). |
| SPEC-XSFM-GROUP-001 / LINE-001 / AGENT-IO-001 등 타 SPEC 파일 | 본 SPEC 범위 밖(무변경 제약). |
| `data.json`/`output.json`/`packet.json`/`web/data/`/`.moai/memory/*` | 무관한 미커밋 변경(범위 밖, 미변경). |
| `.moai/project/architecture.md`/`tech.md` | 이름 셀렉터는 순수 논리 해소 레이어로 기존 아키텍처/기술 스택 경계를 변경하지 않는 가산 셀렉터 → 증분 기재 대상 아님(GROUP-001/LINE-001/AGENT-IO-001 동일 판단). |
| README / API 문서 | MQTT 토픽/페이로드 규약 불변(NF-03), 이름 셀렉터는 논리 해소일 뿐 — 외부 공개 API/README 노출 표면 변경 없음. |
| 프런트엔드 (M4 UI) | M4 이연(선택·저우선). 프런트 미변경 — vitest/`tsc` 무변경. |

---

## 구현 노트 요약 (as-implemented 분기 — spec.md §7)

| # | 분기 | 사양 | 구현 | 사유 |
|---|------|------|------|------|
| 1 | 우선순위 체인 **분산 배치** + `handleIndividualControl` 추출 | §4.3: `dispatchControl`/`handleSelectorControl` 단일 순차 게이트 확장 | 체인이 `dispatchControl`(device_id) + `handleSelectorControl`(device_name if-guard + group_name 최종 case) 로 분산, 개별 제어는 `handleIndividualControl` 추출로 device_id/device_name 공유 | 개별/집계 셀렉터 경계를 코드 구조에 반영, extract-method 로 두 개별 진입점 중복 제거(동작 보존·enforce simplicity). RD-4 순서 불변 |
| 2 | `GroupByName` 전 타입 매칭 = **`allGroups()` 추출·공유** | §4.2/RD-5: 전 타입(custom+파생 station/line) 표시명 매칭 | `handleListGroups` 에서 `allGroups()` 추출, `GroupByName`/`handleListGroups` 가 공유 | 전 타입 집합 도출 로직 중복 방지(DRY), 목록·해소 경로 단일 SSOT → 결과 일관성 |
| 3 | `group_name` fan-out **집계 셀렉터 라벨** | §4.3: 해소된 group id 로 group_id 경로 재사용 | 집계 응답 셀렉터 참조 `selectorRef{Type:"group_name"}`(원 셀렉터 종류 보존) | 집계 응답 소비자가 group_id 가 아닌 group_name 으로 지정됐음을 식별. fan-out 실행은 해소 id 로 기존 경로 무변경 재사용 |
| 4 | **구현 배치**(사양 권장 대안 채택) | §4.3: 리졸버는 신규 파일 **또는** `group_membership.go` 확장 | 리졸버를 신규 `name_resolver.go` 배치, 디스패치는 `group.go`, `control.go` 불변 | `agent.go`/`control.go` diff 최소화(GROUP-001 신규 파일 패턴 계승), 이름 해소 응집도 |

> §7.5: M4(프런트 이름 제어 UI)는 선택·저우선으로 이연 — "completed" 는 M1~M3 핵심에 한함(overclaim 방지).

---

## 검증 증거 (오케스트레이터 제공 + 본 sync 확인)

> 아래 커밋·파일은 본 sync 단계에서 직접 관찰(git show --stat)로 확인. 테스트·커버리지 수치는 run-phase(오케스트레이터) 제공 값.

- **구현 커밋 존재 확인** (직접 관찰, `git show --stat 2acb980d`):
  - `2acb980d feat(xsfm): 이름 기반 제어 셀렉터 device_name/group_name (SPEC-XSFM-NAMESEL-001 M1~M3)` — `agent.go`(+14), `errors.go`(+4), `group.go`(+74/-21 근사), `group_membership.go`(+14), `name_resolver.go`(신규 +85), `name_resolver_test.go`(신규 +381), `internal/node/xsfm.go`(+45), `internal/node/xsfm_test.go`(+77) — 8 files, +673/-21
- **백엔드 테스트** (run-phase 제공): `go test ./...` exit 0 (**42 pkgs, 0 FAIL**), `-race` 클린, **신규 함수 커버리지 100%**
- **무회귀** (run-phase 제공): 기존 셀렉터(device_id/station/line/group_id)·우선순위·집계 응답 무회귀(NF-01), CRUD `name` 무회귀(A-1), 그룹 fan-out 무회귀. node/api-service green. MQTT 토픽/페이로드 규약 불변(NF-03).
- **프런트** (M4 이연): 프런트 미변경 → 기존 vitest/`tsc` 자동 무회귀.

### 미검증(Gaps)

- 테스트 스위트·커버리지 수치는 run-phase 오케스트레이터 보고 값을 인용(본 sync 단계에서 `go test` 를 재실행하지 않음 — 문서 동기화 전용 phase).
- `-race` 클린·42 pkgs·신규 함수 커버리지 100% 는 커밋 시점 측정값이며, 본 sync 는 트리를 코드 변경 없이 유지하므로 재측정 불필요.
- **M4(프런트엔드 이름 제어 UI)는 미착수(이연)** — 해당 UI 및 그 vitest 검증은 이 SPEC 범위에서 수행되지 않았다. 착수 시 별도 검증이 필요하다.

### 잔여 위험(Residual Risk)

- 이름 충돌로 인한 잦은 모호성 거부(`ErrAmbiguousName`) 가능성 — fail-closed(RD-2) 정책상 데이터 무결성 우선. 명확한 에러로 사용자에게 id 셀렉터 안내, RD-6(대소문자 구분)으로 의도치 않은 광범위 매치 방지.
- `group_name` 전 타입 매칭(RD-5)으로 커스텀 그룹명과 파생 station/line 표시명이 충돌하면 다중 매치 → 안전 거부. 타입 우선순위를 두지 않는 결정(RD-5) 유지.
- M4 미구현 상태에서 이름 제어는 flow 노드 메시지 또는 HTTP exec 경로로만 사용 — 전용 UI 편의 기능은 후속 착수 시 제공.

---

## 3-Phase Close

- **plan → run → sync** 3-phase 완료. run-phase 구현(`2acb980d`, M1~M3) 위에서 sync-phase 문서 동기화 수행.
- SPEC 상태 `draft → completed` 전이(spec.md frontmatter, 핵심 M1~M3 기준). 커밋은 오케스트레이터가 처리(본 sync 는 커밋하지 않음).
- **M4(프런트엔드 이름 제어 UI)는 선택적 후속(optional follow-up)으로 이연** — 핵심 요구(이름 기반 제어)는 에이전트+노드 레벨에서 완전 충족.
- 후속: (선택) M4 프런트 이름 제어 UI 착수 시 별도 검증.
