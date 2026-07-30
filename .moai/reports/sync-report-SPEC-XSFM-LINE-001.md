# Sync Report — SPEC-XSFM-LINE-001

- **SPEC**: SPEC-XSFM-LINE-001 — xsfm 라인 1급화 + 코드 기반 주소 체계 + 디바이스 네이밍
- **동기화 일자**: 2026-07-30
- **구현 커밋**: `ef4f28a7`(백엔드 M1~M5) + `e8678033`(프런트 M6)
- **브랜치**: `feature/SPEC-XSFM-GROUP-001`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier M), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` |
| spec.md | `version` | `0.2.0` | `0.3.0` |
| spec.md | `updated` | `2026-07-30` | `2026-07-30` (유지) |
| plan.md / acceptance.md | 버전 노트(top-of-file) | `0.2.0` | `0.3.0` + 상태 `completed` |

- spec.md 는 12-필드 frontmatter 형식을 유지하며 `status`/`version` 만 갱신(`updated` 는 동일 일자로 유지).
- plan.md / acceptance.md 는 frontmatter 대신 **top-of-file 버전 노트** 형식을 사용하므로 그 형식에 맞춰 버전·상태를 갱신(기존 관례 일치).
- spec.md HISTORY 에 `0.3.0` 행 추가(M1~M6 구현 완료).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§7 Implementation Notes(as-implemented)** 신설(분기 5건, §7.1~§7.5).

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-XSFM-LINE-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가; **§7 Implementation Notes(as-implemented) §7.1~§7.5 신설** |
| `.moai/specs/SPEC-XSFM-LINE-001/plan.md` | top-of-file 버전 노트 `0.2.0→0.3.0` + 상태 `completed`; HISTORY 0.3.0(M1~M6 구현 완료 + 검증 증거) 추가 |
| `.moai/specs/SPEC-XSFM-LINE-001/acceptance.md` | top-of-file 버전 노트 `0.2.0→0.3.0` + 상태 `completed`; HISTORY 0.3.0(§1~§8 전 시나리오 통과) 추가 (본문 시나리오 무변경) |
| `CHANGELOG.md` | `[Unreleased]` 최상단 `추가` 에 "xsfm 라인 1급화 + 코드 기반 주소 체계 + 디바이스 네이밍" 항목 추가(기능 + as-implemented 분기 5건 요약) |
| `.moai/project/product.md` | "제공 Agent 목록" 표의 Subway Facilities Manager 행에 라인 1급화 + 코드 주소 체계 요약 추가 |
| `.moai/project/structure.md` | `internal/agent/xsfm/` 항목에 라인 1급화 확장 문단 추가(신규 파일 `line_registry.go`/`code.go`·라인 레지스트리·코드 주소·composeName·마이그레이션·프런트) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `internal/**`, `web/**` (코드) | 본 sync 는 문서 동기화 전용. 코드는 `ef4f28a7`/`e8678033` 에 이미 커밋됨(범위 밖). |
| SPEC-XSFM-GROUP-001 SPEC 파일 | 본 SPEC 범위 밖(무변경 제약). |
| `data.json`/`output.json`/`packet.json`/`.moai/memory/*` | 무관한 미커밋 변경(범위 밖, 미변경). |
| `.moai/project/architecture.md`/`tech.md` | 라인 1급화는 기존 아키텍처/기술 스택 경계를 변경하지 않는 가산 레이어 → 증분 기재 대상 아님(GROUP-001 도 동일 판단). |
| README / API 문서 | 라인 레지스트리는 내부 에이전트 명령 API 확장으로, 외부 공개 API/README 노출 표면 변경 없음. |

---

## 구현 노트 요약 (as-implemented 분기 — spec.md §7)

| # | 분기 | 사양 | 구현 | 사유 |
|---|------|------|------|------|
| 1 | `add_group{code}` **optional** | RD-2: code 필수 | keyPresent 검사, code 있으면 `custom:<code>`(포맷 검증)·없으면 레거시 `custom:<name>` 폴백 | GROUP-001 `add_group{name}` 테스트 무회귀 |
| 2 | station 코드 포맷 **미강제** | RD-6: station/line/custom 신규 코드에 `^[a-z0-9][a-z0-9_-]*$` 강제 | 검증(`validCode`)을 `add_line`·`add_group`(code 경로)에만 적용, station 코드 제외 | 기존 station 테스트가 `ST-101`/`S1` 대문자 사용, 거부 요구 AC 없음 |
| 3 | 마이그레이션 라인 코드 **slugify 미적용** | §4.4 마이그레이션 | `station.Line` 값은 `Line{Code:line, Name:line}` 원문 그대로 승격, slugify 는 커스텀 그룹 name 에만 | 라인 코드 slugify 시 `station.Line` 참조 정합 붕괴 위험 |
| 4 | `golang.org/x/text` **직접 의존성 승격** | §4.6 slugify NFC 정규화 | `go mod tidy` 로 간접→직접(require 블록) | slugify NFC 정규화(`unicode/norm`) 결정론적 구현 요구, 신규 외부 패키지 아님 |
| 5 | 디바이스 이름 표시 **프런트 무변경** | REQ-06-03: 프런트 4/3-세그먼트 표시 | `XsfmDevicesTab`·멤버 테이블이 이미 `device.name` 그대로 렌더 | 백엔드 `composeName` 산출값을 표시만 하므로 프런트 수정 불필요 |

---

## 검증 증거 (오케스트레이터 제공 + 본 sync 확인)

> 아래 커밋·파일·의존성 승격은 본 sync 단계에서 직접 관찰(git log / ls / grep)로 확인. 테스트·커버리지 수치는 run-phase(오케스트레이터) 제공 값.

- **구현 커밋 존재 확인** (직접 관찰):
  - `ef4f28a7 feat(xsfm): 라인 1급 엔티티 + 그룹 코드 + 디바이스 네이밍 백엔드 (SPEC-XSFM-LINE-001 M1~M5)`
  - `e8678033 feat(web): xsfm 라인 관리 탭 + 그룹 코드 입력 (SPEC-XSFM-LINE-001 M6)`
- **신규 백엔드 파일 존재 확인** (직접 관찰): `internal/agent/xsfm/line_registry.go`(14690 B), `internal/agent/xsfm/code.go`(4712 B)
- **`x/text` 직접 의존성 승격 확인** (직접 관찰): `go.mod` require 블록에 `golang.org/x/text v0.35.0`
- **백엔드 테스트** (run-phase 제공): `go test ./...` exit 0 (0 FAIL), `go test -race ./internal/agent/xsfm/...` 클린, xsfm 커버리지 **89.1%** (REQ-07-04 목표 85%+ 충족)
- **프런트 테스트** (run-phase 제공): vitest **2275 pass**, `tsc` 클린
- **무회귀** (run-phase 제공): SPEC-XSFM-GROUP-001 / station / node / api-service 회귀 없음

### 미검증(Gaps)

- 테스트 스위트·커버리지 수치는 run-phase 오케스트레이터 보고 값을 인용(본 sync 단계에서 `go test` 를 재실행하지 않음 — 문서 동기화 전용 phase).
- `-race` 클린·vitest 2275 는 커밋 시점 측정값이며, 본 sync 는 트리를 코드 변경 없이 유지하므로 재측정 불필요.

### 잔여 위험(Residual Risk)

- 분기 2(station 코드 포맷 미강제)로 인해 향후 station/line 코드 도메인 불일치 가능성 — 현재 AC 범위 밖이며 무회귀 우선 결정. 필요 시 후속 SPEC 에서 station 코드 정규화 검토.
- slugify 한글 Revised Romanization 은 음운 동화 미적용(§4.6-2) — 표시명이 아닌 코드 식별자 용도이므로 기능 영향 없음.

---

## 3-Phase Close

- **plan → run → sync** 3-phase 완료. run-phase 구현(`ef4f28a7`, `e8678033`) 위에서 sync-phase 문서 동기화 수행.
- SPEC 상태 `draft → completed` 전이(spec.md frontmatter). 커밋은 오케스트레이터가 처리(본 에이전트는 커밋하지 않음).
- 후속: 없음(SPEC-XSFM-LINE-001 단일 SPEC 종결).
