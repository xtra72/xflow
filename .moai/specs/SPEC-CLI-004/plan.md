---
id: SPEC-CLI-004
title: "CLI–Web UI 기능 패리티 — 구현 계획"
version: 0.2.0
status: completed
created: 2026-06-26
updated: 2026-06-27
author: xtra
priority: high
---

# SPEC-CLI-004 구현 계획 (Implementation Plan)

> 본 문서는 우선순위 기반 마일스톤으로 구성된다(시간 추정 없음). 단계 순서/의존성으로 진행
> 순서를 표현한다.

## 1. 기술 접근 (Technical Approach)

- **재사용 우선**: 모든 신규 명령은 기존 `internal/cli/client.go`(HTTP), `output.go`
  (`PrintResult` 포맷터), `errors.go`(`MapAPIError`), `resolve.go`(이름→ID), `root.go`
  (글로벌 플래그·서버/토큰 해석)를 재사용한다. 신규 HTTP/포맷/에러 스택을 만들지 않는다.
- **명령 구조 패턴**: 기존 `newFlowCmd`/`newAgentCmd`(cobra 그룹 + 서브커맨드 + 클라이언트
  포인터 주입 `**Client`)를 본떠 도메인별 `newXxxCmd(&client, confirmAction)` 를 만들고
  `root.go:80-89` 의 `AddCommand` 에 등록한다.
- **파일 배치**: 도메인별 신규 파일 — `internal/cli/auth.go`, `device.go`, `store.go`,
  `tsdb.go`, `monitor.go`, `system.go`, `settings.go`, `dashboard.go`, `chart.go`,
  `influxdb.go`, `remote.go`(+ 하위 분할 파일). 보완은 기존 `flow.go`/`agent.go` 편집. 교정은
  `plugin.go`(hidden + 미지원 안내, 코드 보존 — 실구현은 SPEC-PLUGIN-001)/`status.go`(실제
  엔드포인트 재배선).
- **라우트 검증**: 각 명령 구현 전 대응 handler 파일(`internal/api/handler/*.go`)에서 경로/
  메서드/응답 DTO 를 재확인한다(SPEC §1.4 근거 기준).

## 2. 단계별 마일스톤 (Priority Milestones)

### 0단계 (Primary Goal) — P0: 역방향 불일치 교정 [우선순위 High]

목적: 현재 404 죽은 명령(plugin/status)을 먼저 교정하여 패리티 작업의 기반을 정리한다.

- **P0-1**: `plugin` 명령 **hidden 처리 + 미지원 안내** — `root.go:85` 의 `newPluginCmd` 등록은
  유지하되 명령을 `Hidden: true` 로 표시, 호출 시 "미지원(SPEC-PLUGIN-001 예정)" 안내, 죽은
  `/api/v1/plugins` 호출 제거. plugin CLI 코드(`plugin.go`)는 **보존**하고 `plugin_test.go` 는
  hidden·미지원·죽은 호출 제거에 맞춰 갱신. plugin 시스템 실제 구현(매니저/레지스트리/WASM·Go
  동적 로딩/보안 샌드박스/노드 통합/`/api/v1/plugins` 핸들러)은 **별도 SPEC `SPEC-PLUGIN-001`**
  소관(본 단계 범위 밖). 근거: plugin 백엔드는 미구현 시스템 전체이며 설정 타입 `PluginConfig`
  (`internal/config/types.go`)만 존재.
- **P0-2**: `status`/`status metrics` 를 실제 엔드포인트로 재배선 — `status` =
  `GET /system/version` + `GET /monitor/metrics` + `GET /flows`·`GET /agents`(total) 합성,
  `status metrics` = `GET /monitor/metrics`.
- **P0-3**: `status logs` 를 로그레벨 조회(`GET /monitor/loglevel`) 안내로 재정의 또는 hidden
  처리(실시간 로그 API 부재). 로그 레벨 관리는 `monitor loglevel`(P1-MON) 위임, 실시간 로그
  스트림은 P4-STREAM(SSE) 후속.
- 산출물: 죽은 호출 0, 기존 테스트(`status_test.go`) 갱신, 회귀 0.
- 요구사항: REQ-CLI-P0-01~04.

### 1단계 (Secondary Goal) — P1: 핵심 신규 도메인 [우선순위 High]

목적: 사용 빈도·운영 중요도가 높은 부재 도메인을 신규 명령 그룹으로 채운다. 각 도메인은 서로
독립적이므로 병렬 구현 가능(파일 분리 — 쓰기 충돌 없음).

- **P1-AUTH**: `auth login/logout/whoami/passwd/refresh` (REQ-CLI-AUTH-01~05). 의존: 토큰
  획득/저장 흐름 결정(§4).
- **P1-DEV**: `device list/get/execute/metadata/history/resolve` (REQ-CLI-DEV-01~06).
- **P1-STORE**: `store query/keys/tags/meta/reset` (REQ-CLI-STORE-01~05).
- **P1-TSDB**: `tsdb query/series/latest/stats/delete/write` (REQ-CLI-TSDB-01~06).
- **P1-MON**: `monitor metrics`, `monitor loglevel get/set/reset` (REQ-CLI-MON-01~05).
  의존: P0-2(메트릭 재배선과 정합).
- **P1-SYS**: `system version`, `system update check/apply/rollback/status`, `system channel
  get/set` (REQ-CLI-SYS-01~07).
- **P1-SET**: `settings get/set` (REQ-CLI-SET-01~03).
- 요구사항: REQ-CLI-AUTH/DEV/STORE/TSDB/MON/SYS/SET.

### 2단계 (Tertiary Goal) — P2: 부분 도메인 보완 [우선순위 Medium]

목적: 이미 존재하는 flow/agent 그룹에 누락 명령을 추가(동작 보존 DDD — 기존 명령 회귀 0).

- **P2-FLOW2**: `flow undeploy/config/subflow-stats/node-configure/tap/taps`
  (REQ-CLI-FLOW2-01~05). `newFlowActionCmd` 패턴 재사용.
- **P2-AGENT2**: `agent enable/disable/config/stats` (REQ-CLI-AGENT2-01~03).
- 의존: 없음(P1 과 독립이나 우선순위상 후순위).

### 3단계 (Final Goal) — P3: 원격 관리 (하위 분할) [우선순위 Medium-Low]

목적: ~60개 라우트 규모의 remote 도메인을 4개 하위 단계로 분할하여 독립 구현. 각 하위 단계는
별도 하위 SPEC 으로 승격할 수 있다(SPEC-CLI-005/006/... 또는 본 SPEC 하위 단계).

- **P3a — 노드 관리**: `remote node list/get/approve/reject/revoke/pre-register`
  (`remote_admin.go`/`remote_enrollment.go`). REQ-CLI-REMOTE-01.
- **P3b — 그룹/토큰**: `remote group ...`, `remote token ...`
  (`remote_grouping.go`/`remote_enrollment.go`). REQ-CLI-REMOTE-02.
- **P3c — 릴리스/버전**: `remote release ...`, `remote version ...`
  (`release_admin.go`/`remote_version.go`). REQ-CLI-REMOTE-03.
- **P3d — 명령/인벤토리/감사**: 명령 디스패치, 인벤토리(live·mirror), 감사
  (`remote_query.go`/`remote_admin.go`). REQ-CLI-REMOTE-04. (가장 무거움 — 최저 우선순위)
- 분할 원칙: 하위 단계 상호 독립(REQ-CLI-REMOTE-05).

### 4단계 (Optional Goal) — P4: 저우선 도메인 [우선순위 Low]

목적: UI 중심/마이너 도메인을 마지막에 채워 완전 패리티 달성.

- **P4-DASH**: `dashboard shared/mine get/set` (REQ-CLI-DASH-01).
- **P4-CHART**: `chart channels` (REQ-CLI-CHART-01).
- **P4-INFLUX**: `influxdb query` (REQ-CLI-INFLUX-01).
- **P4-STREAM(선택)**: `--follow` SSE tail (REQ-CLI-STREAM-02, Optional).

## 3. 아키텍처 설계 방향

- **명령 등록**: `root.go` 의 `AddCommand` 에 도메인 그룹 추가. 그룹 함수 시그니처는 기존과
  동일(`newXxxCmd(client **Client, confirm confirmFunc)` 또는 `(client **Client)`).
- **요청 본문**: KV/쿼리/메타 등 JSON 본문은 (a) `--body @file.json`/`--body '{...}'` 또는
  (b) `key=value` 반복 플래그(기존 `agent exec` 파싱 준용) 두 방식을 일관 지원.
- **파괴적 작업**: reset/delete/update apply 등은 `confirmAction`(root.go:233) 확인 프롬프트 +
  `--yes` 우회 플래그.
- **출력**: 목록=table 헤더+행 함수(`PrintResult` 패턴), 단건=키-값. `--format json|yaml|text`
  지원.

## 4. 결정 사항 (Plan-time Decisions)

| # | 결정 | 권고안 |
| --- | --- | --- |
| D1 | plugin 명령 처리 | **확정: hidden + 미지원 안내 + 코드 보존**. plugin 백엔드는 미구현 시스템 전체(설정 타입 `PluginConfig` 만 존재) → 죽은 `/api/v1/plugins` 호출만 제거, CLI 골격 보존. plugin 시스템 구축은 **SPEC-PLUGIN-001** 로 분리 |
| D2 | status logs 처리 | **확정: 로그레벨 조회(`GET /monitor/loglevel`) 안내로 재정의 또는 hidden** + `monitor loglevel` 위임. 실시간 로그 스트림은 P4-STREAM(SSE) 후속 |
| D3 | auth login 토큰 저장 | `--save` 시 config `auth.token` 저장 / 기본은 stdout 출력(스크립트 친화) |
| D4 | system vs xflowd update | `xflow system`=원격 API 제어 / `xflowd update`=로컬 자가 업데이트. 도움말 명시 분리 |
| D5 | settings vs config | `xflow settings`=서버 전역 KV / `xflow config`=로컬 파일. 도움말 명시 분리 |
| D6 | remote 분할 | P3a~P3d 4개 하위 단계. 필요 시 별도 하위 SPEC 으로 승격 |

## 5. 위험 및 대응 (Risks & Mitigations)

| 위험 | 영향 | 대응 |
| --- | --- | --- |
| 라우트/DTO 변경으로 매핑 불일치 | 잘못된 호출 | 구현 직전 handler 파일 재확인(A5) |
| remote 도메인 과대 범위 | 단계 지연 | P3 하위 분할 + 독립성(REQ-CLI-REMOTE-05) |
| 조건부 라우트(tap/history) 부재 | 명령 404 | 빌드/플래그 조건 확인, 부재 시 미지원 안내 폴백 |
| 기존 명령 회귀 | 운영 영향 | 동작 보존 DDD + 기존 테스트 유지(NFR-07) |
| 시크릿 노출(비밀번호/토큰) | 보안 | 무에코 프롬프트·평문 로그 금지(NFR-06) |

## 6. 완료 정의 (Definition of Done — 단계별)

- 각 단계의 모든 REQ 가 acceptance.md 의 Given-When-Then 을 통과.
- 신규/변경 명령에 대한 테스트(`*_test.go`) 추가, 커버리지 85%+.
- `go test -race ./...`, `go vet`, `golangci-lint` 통과.
- 글로벌 플래그/포맷/에러 일관성(REQ-CLI-X01~07) 충족.
- 한국어 도움말 작성(REQ-CLI-X07).
