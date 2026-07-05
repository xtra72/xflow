---
id: SPEC-CLI-004
title: "CLI–Web UI 기능 패리티 (xflow CLI Full Parity with Web UI / Backend API)"
version: 0.3.0
status: completed
created: 2026-06-26
updated: 2026-06-27
author: xtra
priority: high
related_specs:
  - SPEC-CLI-001
  - SPEC-CLI-002
  - SPEC-CLI-003
  - SPEC-AUTH-001
  - SPEC-AUTH-003
  - SPEC-DEVICE-001
  - SPEC-STORE-001
  - SPEC-TSDB-001
  - SPEC-CHART-001
  - SPEC-WEB-006
  - SPEC-WEB-007
  - SPEC-UPDATE-001
  - SPEC-REMOTE-001
  - SPEC-MODBUS-CLI-001
  - SPEC-PLUGIN-001
tags:
  - cli
  - parity
  - api-coverage
  - http-client
  - auth
  - device
  - store
  - tsdb
  - monitor
  - system-update
  - settings
  - remote
  - dead-command-fix
---

# SPEC-CLI-004: CLI–Web UI 기능 패리티

## HISTORY

- **0.3.0** (2026-06-27): 구현 완료 — P0~P4 전 단계 구현·검증 완료, `status` 를 `planned` →
  `completed` 로 전환. 단계별 신규 명령 그룹(remote node/group/token/release/version/command/
  audit/inventory, dashboard, chart, influxdb)을 기존 client/output/errors/resolve 스택 재사용
  으로 추가(신규 의존성 0). CLI 패키지 커버리지 85.3%, 전 게이트(build/vet/test-race/gofmt/
  golangci-lint) 통과, 회귀 0. 상세는 §8 "구현 노트" 참조. 일부 심화 항목은 후속 하위 SPEC
  후보로 명시(§8).
- **0.2.0** (2026-06-26): plugin 처리 방향 확정 반영 — 코드 조사 결과 plugin 백엔드는 **미구현
  시스템 전체**임이 확인됨(`internal/config/types.go` 의 `PluginConfig`(Directory/WASMEnabled/
  GoEnabled) 설정 타입만 존재, 매니저/레지스트리/동적 로딩(WASM/Go)/`/api/v1/plugins` 핸들러/
  노드 통합 전무, `SandboxConfig` 미사용). **결정**: plugin 시스템 구축은 본 SPEC 범위에서
  **제외**하고 독립 SPEC `SPEC-PLUGIN-001`(향후 작성)으로 분리한다. P0 plugin 교정은 기존
  "제거" 권고에서 **"CLI 를 `Hidden: true` 로 숨기고 호출 시 '미지원(SPEC-PLUGIN-001 예정)'
  안내, 죽은 `/api/v1/plugins` 호출 제거, 코드 보존"** 으로 변경한다. status 교정(P0)은 실제
  엔드포인트(`/system/version` + `/monitor/metrics` + `/flows`·`/agents` total) 재배선으로
  확정하고, `status logs` 는 실시간 로그 API 부재로 로그레벨 조회(`/monitor/loglevel`) 안내로
  재정의(또는 hidden)하며 실시간 로그 스트림은 P4(SSE) 후속으로 둔다.
- **0.1.0** (2026-06-26): 최초 작성 — "Web UI(=백엔드 API)가 지원하는 모든 기능을 xflow CLI
  에서도 지원"한다는 기능 패리티 목표를 EARS 로 정의한다. 전제: Web UI(`web/`)는 `/api/v1/*`
  핸들러만 소비하므로 "Web UI 기능 = API 라우트"이다. 코드 조사 결과(handler 20개 / CLI 클라이언트
  분석)를 근거로 (1) 도메인별 CLI 커버리지 갭 매트릭스(완비/부분/없음), (2) 도메인별 EARS
  요구사항 + 명령↔API 라우트 매핑, (3) 단계 분할(P0 역방향 불일치 교정 → P1~P4 신규/보완), (4)
  횡단(출력 포맷·인증·페이지네이션·에러처리·기존 패턴 재사용) 요구사항을 기술한다. 본 SPEC 은 PLAN
  산출물이며 구현 코드를 포함하지 않는다.

| Version | Date       | Author | Change                                                       |
| ------- | ---------- | ------ | ------------------------------------------------------------ |
| 0.3.0   | 2026-06-27 | xtra   | 구현 완료 — P0~P4 전 단계 구현·검증, status planned→completed. 신규 명령 그룹 추가(신규 의존성 0), 커버리지 85.3%·전 게이트 통과·회귀 0. 후속 하위 SPEC 후보 명시(§8) |
| 0.2.0   | 2026-06-26 | xtra   | plugin 처리 확정 — plugin 시스템 구축을 SPEC-PLUGIN-001 로 분리, P0 plugin 교정을 hidden+미지원 안내+코드 보존으로 변경; status 재배선·status logs 재정의 확정 |
| 0.1.0   | 2026-06-26 | xtra   | 최초 작성 — CLI–Web UI 기능 패리티 EARS SPEC (갭 매트릭스 + 도메인 매핑 + P0~P4 단계 + plugin/status 교정) |

> **상태(Status)** — `completed`. P0~P4 전 단계가 단계 단위로 증분 구현·검증 완료되었다.
> 구현 요약과 후속 과제는 §8 "구현 노트(Implementation Notes)" 참조.

---

## 1. 개요 (Overview)

### 1.1 목적

`xflow` CLI(클라이언트)가 **xflowd 서버의 Web UI 가 제공하는 모든 기능**을 동등하게 수행할 수
있도록 한다. Web UI(`web/`)는 백엔드 `/api/v1/*` REST API 만 소비하므로, 본 SPEC 은
**"Web UI 기능 = API 라우트"** 라는 전제 아래 "CLI 가 모든 API 도메인을 커버한다"를 패리티의
정의로 삼는다. 부수적으로, 현재 CLI 에 존재하나 **백엔드에 대응 라우트가 없는 죽은 명령**
(역방향 불일치)을 교정한다.

### 1.2 패리티 정의 (Parity Definition)

- **패리티 = API 라우트 커버리지**: Web UI 가 호출하는 각 `/api/v1/*` 엔드포인트에 대해, 그
  기능을 호출할 수 있는 CLI 명령(또는 명령 그룹)이 존재한다.
- **xflow 클라이언트 대상**: 본 SPEC 의 패리티 대상은 `xflow`(원격 서버를 HTTP 로 호출하는
  클라이언트, `internal/cli/*.go`)이다. `xflowd`(서버 데몬, `cmd/xflowd/`)의 **로컬 데몬 전용
  명령**(예: `xflowd update` — 로컬 바이너리 자가 업데이트)은 본 SPEC 범위 외이다.
- **CLI 고유 기능 보존**: 대화형 셸(`interactive`), 스크립트 실행(`script`), 로컬 설정
  (`config` — `~/.xflow/config.yaml`), 자동완성(`autocomplete`), 마법사(`wizard`) 등 CLI
  고유 기능은 그대로 유지한다(API 패리티 대상 아님).

### 1.3 배경 (현재 상태 — 코드 근거)

#### 시스템 구조

| 사실 | 근거 (file:line) |
| --- | --- |
| 백엔드 API 핸들러 20개가 `/api/v1/*` 라우트를 등록. Web UI 는 이 API 만 소비 | `internal/api/handler/*.go` (auth/device/flow/agent/store_query/tsdb/monitor/settings/dashboard/chart/influxdb_query/system_update/node/remote_*/release_*) |
| `xflow` CLI 는 `internal/cli/client.go` 의 HTTP 클라이언트(Get/Post/Put/Delete/GetRaw/PostRaw)로 API 호출 | `internal/cli/client.go:42-132` |
| 글로벌 플래그 7개(`--config/--server/--format/--token/--verbose/--quiet/--no-color`) + 우선순위(플래그>env>config>기본값) | `internal/cli/root.go:70-77, 192-229` |
| 등록된 CLI 명령 그룹: version/flow/agent/config/node/plugin/status/interactive/script/modbus | `internal/cli/root.go:80-89` |
| 출력 포맷터(table/json/yaml/text) 공통 | `internal/cli/output.go`, `PrintResult(...)` |
| API 응답 봉투 = `{success, data, error}`, 에러는 `MapAPIError(status, body)` 로 매핑 | `internal/cli/client.go:184-227`, `internal/cli/errors.go` |

#### 도메인 분류 (16) — Web UI 가 지원하는 기능

인증(auth), 플로우(flow), 에이전트(agent), 디바이스(device), 노드타입(node), 대시보드
(dashboard), 모니터링(monitor: metrics/loglevel), 원격관리(remote), 스토어(store KV), TSDB
(시계열), 차트(chart), 시스템업데이트(system/update), InfluxDB, 설정(settings — 서버 전역 KV),
릴리스(release), SSE/WS 스트림.

### 1.4 갭 매트릭스 (CLI 커버리지 — 코드 근거)

범례: 🟢 완비(API 전부 CLI 존재) / 🟡 부분(일부 누락) / 🔴 도메인 자체 부재(신규 명령 그룹 필요)

#### 🟢 완비

| 도메인 | CLI | API 라우트 근거 |
| --- | --- | --- |
| node type | `node type/list/info` | `internal/cli/node.go:14,52,172,226` |

#### 🟡 부분 (일부 API 누락)

| 도메인 | CLI 보유 | CLI 누락 (대응 API) | API 근거 |
| --- | --- | --- | --- |
| flow | list/get/create/update/delete/deploy/start/stop/restart/status/export/import | **undeploy**(`POST /flows/{id}/undeploy`), **config 수정**(`PUT /flows/{id}/config`), **tap/taps**(`POST /flows/{id}/nodes/{nodeID}/tap`, `GET /flows/{id}/taps`), **node configure**(`POST /flows/{id}/nodes/{nodeID}/configure`), **subflow-stats**(`GET /flows/{id}/subflow-stats`) | CLI: `internal/cli/flow.go:15,18,74-588`; API: `internal/api/handler/flow.go:208-231` |
| agent | list/get/create/start/stop/restart/delete/export/import/exec/topics | **enable**(`POST /agents/{id}/enable`), **disable**(`POST /agents/{id}/disable`), **config 수정**(`PUT /agents/{id}/config`), **stats 단독**(`GET /agents/{id}/stats`) | CLI: `internal/cli/agent.go:21,49-788`; API: `internal/api/handler/agent.go:203-218` |

> 참고: `agent topics`(CLI)는 별도 라우트가 아니라 `GET /agents/{id}?detail=full` 의
> `state.topics` 를 추출해 표시한다(`internal/cli/agent.go:807,832-835`). 패리티 위반 아님.

#### 🔴 도메인 자체 부재 (신규 CLI 명령 그룹 필요)

| 도메인 | 신규 CLI 명령군 | 대응 API 라우트 | API 근거 |
| --- | --- | --- | --- |
| auth | `login`/`logout`/`whoami`/`passwd`/`refresh` | `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `PUT /auth/password`, `POST /auth/refresh` | `internal/api/handler/auth.go:33-45` |
| device | `list`/`get`/`execute`/`metadata`(set·del)/`history`/`resolve` | `GET /devices`, `GET /devices/{ref}`, `POST /devices/{id}/execute`, `PUT·DELETE /devices/{id}/metadata`, `GET /devices/{id}/history`, `GET /devices:resolve` | `internal/api/handler/device.go:151-163` |
| store | `query`/`keys`/`tags`/`meta`/`reset` | `POST /store/{agent}/query`, `GET /store/{agent}/keys`, `GET /store/{agent}/tags`, `PUT /store/{agent}/keys/{key}/meta`, `DELETE /store/{agent}/keys[/{key}]` | `internal/api/handler/store_query.go:151-161` |
| tsdb | `query`/`series`/`latest`/`stats`/`delete`/`write` | `POST /tsdb/query`, `GET /tsdb/series`, `GET /tsdb/series/{key}/latest`, `GET /tsdb/stats`, `DELETE /tsdb/series/{key}`, `POST /tsdb/write` | `internal/api/handler/tsdb.go:60-65` |
| monitor | `metrics`, `loglevel get/set[전역]/set[컴포넌트]/reset[컴포넌트]` | `GET /monitor/metrics`, `GET /monitor/loglevel`, `PUT /monitor/loglevel`, `PUT /monitor/loglevel/{component}`, `DELETE /monitor/loglevel/{component}` | `internal/api/handler/monitor.go:86-90` |
| system (원격 서버 대상) | `version`/`update check/apply/rollback/status`, `channel get/set` | `GET /system/version`, `POST /system/update/check`, `POST /system/update/apply`, `POST /system/update/rollback`, `GET /system/update/status`, `GET·PUT /system/update/channel` | `internal/api/handler/system_update.go:923-929` |
| settings | `get`/`set` (서버 전역 KV) | `GET /settings/{key}`, `PUT /settings/{key}` | `internal/api/handler/settings.go:67-68` |
| dashboard | `shared get/set`, `mine get/set` (낮은 우선순위) | `GET·PUT·DELETE /dashboards/shared`, `GET·PUT·DELETE /dashboards/mine` | `internal/api/handler/dashboard.go:74-81` |
| remote | **도메인 전체 부재** (~60개 라우트 규모) — 노드 승인/거부/폐기/목록/pending, 명령 디스패치, 그룹 관리, 등록토큰, 릴리스 관리, 버전관리, 감사, 디스플레이 오버라이드, 원격 인벤토리/편집 | `internal/api/handler/remote_*.go`, `release_*.go` (다수) | `internal/api/handler/remote_admin.go`, `remote_grouping.go`, `remote_enrollment.go`, `remote_version.go`, `release_admin.go` 등 |
| chart | `channels` | `GET /charts/channels` | `internal/api/handler/chart.go:37` |
| influxdb | `query` | `POST /influxdb/{agent}/query` | `internal/api/handler/influxdb_query.go:47` |

> **device(부분 정정)**: 현재 CLI 에는 `modbus` 명령(=특정 modbus 에이전트의 `agent exec`
> 래퍼)만 있고(`internal/cli/modbus.go`), **일반 device 그룹은 부재**하다. 따라서 device 는
> 🔴(신규 그룹)로 분류한다.

### 1.5 역방향 불일치 (기존 CLI 교정 대상 — 존재하지 않는 API 호출)

| CLI 명령 | 호출 경로 | 문제 | 근거 |
| --- | --- | --- | --- |
| `plugin list/install/remove/update` | `GET/POST/DELETE/PUT /api/v1/plugins[/{name}]` | **plugin 백엔드가 미구현 시스템 전체** → 404 죽은 명령. `internal/config/types.go` 의 `PluginConfig`(Directory/WASMEnabled/GoEnabled) 설정 타입만 존재하고, 매니저/레지스트리/동적 로딩(WASM/Go)/`/api/v1/plugins` 핸들러/노드 통합이 전무하며 `SandboxConfig` 도 미사용 → **plugin 시스템 구축은 SPEC-PLUGIN-001 소관**(§1.6 제외) | CLI: `internal/cli/plugin.go:56,93,133,166`; 설정 타입: `internal/config/types.go`(`PluginConfig`); API: plugin 핸들러 부재(handler 디렉토리에 plugin.go 없음) |
| `status` | `GET /api/v1/status` | 해당 라우트 부재 → 404 | CLI: `internal/cli/status.go:35-38`; 실제 정보원 = `/monitor/metrics`·`/system/version` |
| `status logs` | `GET /api/v1/logs?component=&lines=` | 해당 라우트 부재 → 404 | CLI: `internal/cli/status.go:62-84` |
| `status metrics` | `GET /api/v1/metrics` | 해당 라우트 부재(실제는 `/monitor/metrics`) → 404 | CLI: `internal/cli/status.go:108-116`; API: `internal/api/handler/monitor.go:86` |

### 1.6 범위 (Scope)

**포함:**
- 🟡 부분 도메인(flow/agent)의 **누락 API 보완**(§1.4 부분 표).
- 🔴 부재 도메인의 **신규 CLI 명령 그룹** 추가: auth, device, store, tsdb, monitor, system,
  settings, dashboard, chart, influxdb, remote.
- 🟢 역방향 불일치(plugin/status)의 **교정**(hidden 처리/재배선/재정의 — §4.2 그룹 P0).
  plugin CLI 는 `Hidden: true` 로 숨기고 호출 시 미지원 안내, 죽은 `/api/v1/plugins` 호출 제거,
  코드는 보존한다(실제 plugin 기능 구현은 SPEC-PLUGIN-001 위임).
- 횡단 일관성: 출력 포맷(table/json/yaml/text), 인증(글로벌 `--token`/config 토큰 상속),
  페이지네이션(목록 API 의 page/size/limit 등), 에러 처리(`MapAPIError` 의미 보존), 기존
  `client.go`/`output.go`/`errors.go` 패턴 재사용, 한국어 도움말.

**제외(Non-goals):**
- `xflowd`(서버 데몬) 전용/로컬 명령(예: `xflowd update` 로컬 자가 업데이트, 서버 부트스트랩,
  TLS 인증서 관리 등). 본 SPEC 은 `xflow` 클라이언트만 패리티 대상으로 한다.
- CLI 고유 기능(대화형/스크립트/로컬 config/자동완성/마법사)의 재설계.
- **plugin 시스템 구축은 SPEC-PLUGIN-001 소관**: plugin 백엔드는 미구현 시스템 전체
  (설정 타입 `PluginConfig` 만 존재 — §1.5)이며, 매니저/레지스트리/동적 로딩(WASM/Go)·보안
  샌드박스·노드 타입 등록 통합·`/api/v1/plugins` 핸들러·배포/검증 등은 아키텍처 결정이 많은
  대형 트랙이다. 이는 CLI 패리티(빠른 교정/API 래핑)와 성격이 다르므로 **독립 SPEC
  `SPEC-PLUGIN-001`(향후 작성)** 으로 분리한다. 본 SPEC 의 P0 는 plugin **CLI 를 hidden +
  미지원 안내로 처리하고 죽은 호출만 제거**할 뿐, plugin 기능 자체를 구현하지 않는다(§4.2
  REQ-CLI-P0-01).
- **SSE/WS 실시간 스트림의 완전 CLI 재현**: 스트림(로그/차트/모니터 WS)은 1차적으로
  스냅샷/폴링 형태로만 노출하며, 실시간 follow(`--follow`)는 선택적 후속 과제로 둔다(§4.9).
- 새로운 인증/권한 모델 도입. CLI 는 기존 JWT Bearer 토큰 흐름을 그대로 따른다.
- 신규 외부 의존성 도입(가능한 한 기존 cobra/viper/HTTP 클라이언트만 사용).

---

## 2. 환경 (Environment)

| 항목 | 상세 |
| --- | --- |
| 런타임 | Go 1.23+ |
| 대상 모듈 | `internal/cli/*.go`(신규 도메인 파일 + flow.go/agent.go 보완 + plugin.go/status.go 교정), `cmd/xflow/`(명령 등록은 `internal/cli/root.go` 경유) |
| 재사용 인프라 | `internal/cli/client.go`(HTTP), `internal/cli/output.go`(포맷터 `PrintResult`), `internal/cli/errors.go`(`MapAPIError`/`CLIError`), `internal/cli/root.go`(글로벌 플래그·서버/토큰 해석), `internal/cli/resolve.go`(이름→ID 해석) |
| API 계약 출처 | `internal/api/handler/*.go`(라우트), `internal/api/dto/*`(요청/응답 스키마) |
| 테스트 | Go `testing`+`testify`, `go test -race ./...`, 기존 CLI 테스트 패턴(`internal/cli/*_test.go` — `flow_test.go`/`agent_test.go`/`status_test.go` 등) 정합, 커버리지 85%+ |
| 개발 방법론 | Hybrid(`.moai/config/sections/quality.yaml` development_mode=hybrid): 신규 명령=TDD, 기존 flow.go/agent.go/plugin.go/status.go 변경=동작 보존 DDD |

---

## 3. 가정 (Assumptions)

- **A1**: Web UI 가 호출하는 모든 기능은 `/api/v1/*` REST 라우트로 노출된다(스트림 제외). 따라서
  CLI 가 동일 라우트를 호출하면 동일 기능을 수행한다.
- **A2**: 신규/보완 명령은 기존 `Client`(Get/Post/Put/Delete/GetRaw/PostRaw)와 출력 포맷터
  (`PrintResult`)·에러 매핑(`MapAPIError`)을 재사용한다. 신규 HTTP 스택을 만들지 않는다.
- **A3**: 인증은 기존 글로벌 흐름을 상속한다 — `--token` 플래그 > `XFLOW_TOKEN` env >
  config `auth.token`(`internal/cli/root.go:216-229`). 신규 명령도 동일 토큰으로 Bearer 인증된다.
- **A4**: `auth login` 은 예외적으로 토큰을 **획득**하는 명령이므로 토큰 없이 호출 가능해야 하며,
  획득한 토큰은 사용자가 config(`auth.token`)에 저장하거나 출력으로 받는다(저장 방식은 plan 에서
  확정).
- **A5**: 서버 라우트 경로/응답 스키마는 본 SPEC 의 권위 출처(handler/dto)를 따른다. 라우트가
  불확실하면 구현 시 handler 파일을 재확인한다(본 SPEC 의 라우트는 §1.4 근거로 검증됨).
- **A6**: `remote` 도메인은 ~60개 라우트 규모로 단일 단계에 부적합하므로, 하위 SPEC/단계로 분할
  한다(§4.8 + plan.md P3). 본 SPEC 은 remote CLI 의 **요구사항 골격과 분할 전략**만 정의한다.
- **A7**: plugin 백엔드는 미구현 시스템 전체이며(설정 타입 `PluginConfig` 만 존재 — §1.5),
  plugin 시스템 구축은 독립 SPEC `SPEC-PLUGIN-001`(향후 작성) 소관이다. 따라서 본 SPEC 의 P0 는
  plugin CLI 를 `Hidden: true` 로 숨기고 호출 시 "미지원(SPEC-PLUGIN-001 예정)" 안내 + 죽은
  `/api/v1/plugins` 호출 제거만 수행하며, 코드는 보존한다(§4.2).
- **A8**: `system` 도메인 CLI(`xflow system ...`)는 **원격 서버를 API 로 제어**한다. 로컬 데몬용
  `xflowd update` 와는 별개이며 충돌하지 않는다(서로 다른 바이너리·대상).

---

## 4. 요구사항 (EARS)

> 표기: `M`(마일스톤/요구사항 번호). 각 도메인 그룹은 단계(P0~P4, plan.md)에 매핑된다.
> 모든 명령은 글로벌 플래그(`--server/--token/--format/--verbose/--quiet/--no-color`)를 상속하며
> (REQ-CLI-X01), 출력은 `--format`(table 기본) 옵션을 따른다(REQ-CLI-X02).

### 4.1 그룹 X — 횡단 일관성 (Cross-Cutting) — 모든 명령 적용

**REQ-CLI-X01**: 글로벌 플래그 상속
시스템은 **항상** 신규/보완 명령이 기존 글로벌 플래그(`--server`, `--token`, `--format`,
`--verbose`, `--quiet`, `--no-color`, `--config`)와 그 우선순위(플래그>env>config>기본값,
`internal/cli/root.go:192-229`)를 상속하도록 해야 한다. 명령별 신규 HTTP 클라이언트/토큰 해석을
중복 구현하지 않아야 한다.

**REQ-CLI-X02**: 출력 포맷 일관성
시스템은 **항상** 모든 데이터 출력 명령이 `--format`(table|json|yaml|text)을 지원하고 기존
`output.go` 포맷터(`PrintResult`)를 재사용하도록 해야 한다. 기본값은 `table` 이며, 목록은 table,
단건은 적절한 키-값 표시를 제공해야 한다.

**REQ-CLI-X03**: 인증/권한 일관성
시스템은 **항상** 인증이 필요한 엔드포인트 호출 시 Bearer 토큰(REQ-CLI-X01 의 해석값)을 자동
주입(`client.go:163-165`)하도록 해야 한다. 토큰 누락/만료로 인한 401/403 은 `MapAPIError` 의
기존 의미(인증 오류 힌트 포함)로 보고해야 한다.

**REQ-CLI-X04**: 에러 처리 일관성
시스템은 **항상** API 오류를 기존 `MapAPIError(status, body)`(`internal/cli/errors.go`)로 매핑하여
종료 코드·힌트·사유를 일관되게 표시해야 한다. 연결 실패는 기존 `wrapConnectionError`
(`client.go:230-237`)로 처리해야 한다. 신규 명령이 자체 에러 포맷을 만들지 않아야 한다.

**REQ-CLI-X05**: 페이지네이션 일관성
**WHERE** 대응 목록 API 가 페이지네이션 파라미터(예: `page`/`size`/`limit`/`offset`/`cursor`)를
지원하면, 시스템은 그에 대응하는 명령 플래그(예: `--page`/`--limit`)를 제공하고 쿼리 파라미터로
전달해야 한다. API 가 페이지네이션을 지원하지 않으면 플래그를 추가하지 않는다(과잉 설계 금지).

**REQ-CLI-X06**: 이름→ID 해석 재사용
**WHERE** 자원이 이름 또는 ID 로 지정될 수 있으면(flow/agent/device 등), 시스템은 기존 해석
유틸(`internal/cli/resolve.go`)을 재사용하여 `<id|name>` 입력을 지원해야 한다(기존 flow/agent
명령의 UX 일관).

**REQ-CLI-X07**: 한국어 도움말
시스템은 **항상** 신규/보완 명령의 `Short`/`Long`/예시를 기존 명령과 동일하게 **한국어**로
작성해야 한다(`language.yaml` documentation=ko, 기존 flow.go/agent.go 톤 일관).

### 4.2 그룹 P0 — 역방향 불일치 교정 (Dead Command Reconciliation) — 최우선

**REQ-CLI-P0-01**: plugin 명령 교정 (hidden + 미지원 안내)
**IF** 백엔드에 plugin API(`/api/v1/plugins*`)가 존재하지 않으면(현재 상태 — plugin 백엔드는
미구현 시스템 전체, 설정 타입 `PluginConfig` 만 존재 — §1.5), **THEN** 시스템은 `plugin
list/install/remove/update` 명령을 `Hidden: true` 로 **숨기고**, 호출 시 "미지원
(SPEC-PLUGIN-001 예정)" 안내를 표시하며, 죽은 `/api/v1/plugins` 호출을 **제거**해야 한다.
plugin CLI 코드(`internal/cli/plugin.go`)는 **보존**한다(향후 SPEC-PLUGIN-001 구현 시 재사용
가능하도록). plugin 시스템(매니저/레지스트리/WASM·Go 동적 로딩/보안 샌드박스/노드 통합/
`/api/v1/plugins` 핸들러) 자체의 구현은 본 SPEC 범위 밖이며 독립 SPEC `SPEC-PLUGIN-001` 에
위임한다.
> **결정(확정)**: (a) 제거가 아니라 **hidden + 안내 + 코드 보존**으로 한다. 사유: plugin 시스템은
> SPEC-PLUGIN-001 로 분리되어 향후 구현 예정이므로 CLI 골격을 보존하는 편이 재사용에 유리하고,
> hidden 처리로 사용자 혼란/죽은 호출은 동일하게 제거된다. 실제 plugin 기능 동작은
> SPEC-PLUGIN-001 에서 다룬다. `root.go:85` 의 `newPluginCmd` 등록은 유지하되 명령을 hidden 으로
> 표시하고, `plugin.go`/`plugin_test.go` 는 죽은 API 호출 제거·미지원 안내에 맞춰 갱신한다.

**REQ-CLI-P0-02**: status 명령 재배선 (실제 엔드포인트로 — 확정)
시스템은 **항상** `status`/`status metrics` 가 존재하지 않는 `/api/v1/status`·`/api/v1/metrics`
대신 **실제 엔드포인트**를 호출하도록 재배선해야 한다. `status` 는 `GET /system/version`(버전) +
`GET /monitor/metrics`(런타임 메트릭) + `GET /flows`·`GET /agents`(각 total 카운트)를 합성해
요약(버전 + 런타임 메트릭 + 플로우/에이전트 수)을 표시하고, `status metrics` 는
`GET /monitor/metrics` 를 호출해야 한다(REQ-CLI-MON-01 과 정합).

**REQ-CLI-P0-03**: status logs 재정의 (로그 소스 명확화 — 확정)
**IF** 실시간 로그 API(`/api/v1/logs`)가 존재하지 않으면, **THEN** 시스템은 `status logs` 를
로그 레벨 조회(`GET /monitor/loglevel`) 안내로 재정의하거나(또는 hidden 처리), 미지원으로 표시
하고 대체 명령(`monitor loglevel`)을 안내해야 한다. 죽은 `/logs` 호출을 유지해서는 안 된다.
실시간 로그 스트림은 본 단계 범위 밖이며 P4(SSE `--follow` — §4.14)의 후속 과제로 둔다.
> **결정(확정)**: `status logs` 는 실시간 로그 스트림 API 부재로 인해 로그 레벨 조회
> (`GET /monitor/loglevel`) 안내로 재정의하거나 hidden 처리하고, 로그 레벨 관리는 신규
> `monitor loglevel`(REQ-CLI-MON-02~05)에 위임한다. 실시간 로그 스트림은 P4(SSE) 후속.

**REQ-CLI-P0-04**: 교정 회귀 금지
시스템은 **항상** P0 교정이 정상 동작하는 다른 CLI 명령(flow/agent/node/config 등)에 회귀를
일으키지 않도록 보장해야 한다(동작 보존 DDD — 기존 `status_test.go` 갱신 + `plugin_test.go` 를
hidden·미지원 안내·죽은 호출 제거에 맞춰 조정. plugin 명령 코드는 제거하지 않고 보존).

### 4.3 그룹 AUTH — 인증 (P1)

**REQ-CLI-AUTH-01**: 로그인
시스템은 **항상** `xflow auth login`(`--username`/`--password` 또는 프롬프트)이
`POST /auth/login` 을 호출하여 토큰을 획득하도록 제공해야 한다. 토큰 없이 호출 가능해야 하며
(A4), 획득한 access/refresh 토큰을 출력하거나 config(`auth.token`)에 저장(플래그 `--save`)할 수
있어야 한다.

**REQ-CLI-AUTH-02**: 로그아웃
시스템은 **항상** `xflow auth logout` 이 `POST /auth/logout` 을 호출하고, 로컬 config 의 저장된
토큰을 무효화/제거(선택)하도록 제공해야 한다.

**REQ-CLI-AUTH-03**: 현재 사용자 조회
시스템은 **항상** `xflow auth whoami` 가 `GET /auth/me` 를 호출하여 현재 토큰의 사용자/역할을
표시하도록 제공해야 한다.

**REQ-CLI-AUTH-04**: 비밀번호 변경
시스템은 **항상** `xflow auth passwd`(`--old`/`--new` 또는 프롬프트)가 `PUT /auth/password` 를
호출하도록 제공해야 한다. 비밀번호는 stderr 프롬프트(무에코)로 입력받을 수 있어야 한다(시크릿
노출 금지).

**REQ-CLI-AUTH-05**: 토큰 갱신
시스템은 **항상** `xflow auth refresh`(refresh 토큰 입력)가 `POST /auth/refresh` 를 호출하여 새
access 토큰을 발급받도록 제공해야 한다.

### 4.4 그룹 DEV — 디바이스 (P1)

**REQ-CLI-DEV-01**: 디바이스 목록
시스템은 **항상** `xflow device list` 가 `GET /devices` 를 호출하여 디바이스 목록을 표시하도록
제공해야 한다(페이지네이션/필터는 API 지원 시 플래그로 — REQ-CLI-X05).

**REQ-CLI-DEV-02**: 디바이스 상세
시스템은 **항상** `xflow device get <ref>` 가 `GET /devices/{ref}`(ref=UUID 또는
`agent:local_id` composite)를 호출하여 상세를 표시하도록 제공해야 한다.

**REQ-CLI-DEV-03**: 디바이스 명령 실행
시스템은 **항상** `xflow device execute <id> <command> [key=value ...]` 가
`POST /devices/{id}/execute` 를 호출하도록 제공해야 한다(기존 `agent exec` 인자 파싱 패턴 준용).

**REQ-CLI-DEV-04**: 디바이스 메타데이터 설정/삭제
시스템은 **항상** `xflow device metadata set <id> <json|key=value>` 가
`PUT /devices/{id}/metadata` 를, `xflow device metadata del <id>` 가
`DELETE /devices/{id}/metadata` 를 호출하도록 제공해야 한다.

**REQ-CLI-DEV-05**: 디바이스 이력 조회
시스템은 **항상** `xflow device history <id>` 가 `GET /devices/{id}/history` 를 호출하도록 제공
해야 한다(이력 라우트는 빌드/플래그로 조건부일 수 있음 — `device.go:163` 참고, 부재 시 명확한
미지원 안내).

**REQ-CLI-DEV-06**: 디바이스 이름 해석
시스템은 **항상** `xflow device resolve <agent> <name>` 가 `GET /devices:resolve`(또는
`GET /devices/{agent}/{name}`)를 호출하여 ref 를 해석하도록 제공해야 한다.

### 4.5 그룹 STORE — 스토어 KV (P1)

**REQ-CLI-STORE-01**: 키 조회/쿼리
시스템은 **항상** `xflow store query <agent>`(본문/플래그로 질의)가 `POST /store/{agent}/query`
를 호출하도록 제공해야 한다.

**REQ-CLI-STORE-02**: 키 목록
시스템은 **항상** `xflow store keys <agent>` 가 `GET /store/{agent}/keys`(필터 플래그 지원)를
호출하도록 제공해야 한다.

**REQ-CLI-STORE-03**: 태그 목록
시스템은 **항상** `xflow store tags <agent>` 가 `GET /store/{agent}/tags` 를 호출하도록 제공
해야 한다.

**REQ-CLI-STORE-04**: 키 메타 설정
시스템은 **항상** `xflow store meta <agent> <key>`(본문/플래그)가
`PUT /store/{agent}/keys/{key}/meta` 를 호출하도록 제공해야 한다.

**REQ-CLI-STORE-05**: 키/전체 리셋
시스템은 **항상** `xflow store reset <agent> [key]` 가 키 지정 시
`DELETE /store/{agent}/keys/{key}`, 미지정 시 `DELETE /store/{agent}/keys`(전체)를 호출하도록
제공하고, 파괴적 작업이므로 확인 프롬프트(기존 `confirmAction`)를 거쳐야 한다.

### 4.6 그룹 TSDB — 시계열 (P1)

**REQ-CLI-TSDB-01**: 시계열 쿼리
시스템은 **항상** `xflow tsdb query`(본문/플래그)가 `POST /tsdb/query` 를 호출하도록 제공해야
한다.

**REQ-CLI-TSDB-02**: 시리즈 목록
시스템은 **항상** `xflow tsdb series` 가 `GET /tsdb/series` 를 호출하도록 제공해야 한다.

**REQ-CLI-TSDB-03**: 최신값
시스템은 **항상** `xflow tsdb latest <key>` 가 `GET /tsdb/series/{key}/latest` 를 호출하도록
제공해야 한다.

**REQ-CLI-TSDB-04**: 통계
시스템은 **항상** `xflow tsdb stats` 가 `GET /tsdb/stats` 를 호출하도록 제공해야 한다.

**REQ-CLI-TSDB-05**: 시리즈 삭제
시스템은 **항상** `xflow tsdb delete <key>` 가 `DELETE /tsdb/series/{key}` 를 호출하도록 제공
하고, 파괴적 작업이므로 확인 프롬프트를 거쳐야 한다.

**REQ-CLI-TSDB-06**: 시계열 쓰기
시스템은 **항상** `xflow tsdb write`(본문/플래그)가 `POST /tsdb/write` 를 호출하도록 제공해야
한다.

### 4.7 그룹 MON — 모니터링 (P1)

**REQ-CLI-MON-01**: 메트릭 조회
시스템은 **항상** `xflow monitor metrics` 가 `GET /monitor/metrics` 를 호출하여 실시간 런타임
메트릭(cpu/memory/goroutines/heap/uptime)을 표시하도록 제공해야 한다(P0 status 재배선과 정합 —
REQ-CLI-P0-02).

**REQ-CLI-MON-02**: 로그 레벨 조회
시스템은 **항상** `xflow monitor loglevel get` 이 `GET /monitor/loglevel` 을 호출하여 전역/컴포넌트
로그 레벨을 표시하도록 제공해야 한다.

**REQ-CLI-MON-03**: 전역 로그 레벨 설정
시스템은 **항상** `xflow monitor loglevel set <level>` 이 `PUT /monitor/loglevel` 을 호출하도록
제공해야 한다.

**REQ-CLI-MON-04**: 컴포넌트 로그 레벨 설정
시스템은 **항상** `xflow monitor loglevel set <component> <level>` 이
`PUT /monitor/loglevel/{component}` 를 호출하도록 제공해야 한다.

**REQ-CLI-MON-05**: 컴포넌트 로그 레벨 초기화
시스템은 **항상** `xflow monitor loglevel reset <component>` 가
`DELETE /monitor/loglevel/{component}` 를 호출하도록 제공해야 한다.

### 4.8 그룹 SYS — 시스템 업데이트 (원격 서버 대상, P1)

**REQ-CLI-SYS-01**: 서버 버전 조회
시스템은 **항상** `xflow system version` 이 `GET /system/version` 을 호출하여 원격 서버의 버전/
업데이트 가용성을 표시하도록 제공해야 한다(SPEC-WEB-007 확장 스키마 소비).

**REQ-CLI-SYS-02**: 업데이트 확인
시스템은 **항상** `xflow system update check` 가 `POST /system/update/check` 를 호출하도록 제공
해야 한다.

**REQ-CLI-SYS-03**: 업데이트 적용
시스템은 **항상** `xflow system update apply` 가 `POST /system/update/apply` 를 호출하도록 제공
하고, 파괴적/재시작 유발 작업이므로 확인 프롬프트를 거쳐야 한다.

**REQ-CLI-SYS-04**: 롤백
시스템은 **항상** `xflow system update rollback` 이 `POST /system/update/rollback` 을 호출하도록
제공해야 한다.

**REQ-CLI-SYS-05**: 업데이트 상태
시스템은 **항상** `xflow system update status` 가 `GET /system/update/status` 를 호출하도록 제공
해야 한다.

**REQ-CLI-SYS-06**: 채널 조회/설정
시스템은 **항상** `xflow system channel get` 이 `GET /system/update/channel` 을,
`xflow system channel set <channel>` 이 `PUT /system/update/channel` 을 호출하도록 제공해야 한다.

**REQ-CLI-SYS-07**: 로컬 데몬 업데이트와의 분리
시스템은 **항상** `xflow system ...`(원격 서버 API 제어)이 `xflowd update`(로컬 데몬 자가
업데이트)와 **별개**임을 도움말에 명시하여 혼동을 방지해야 한다(A8 — 서로 다른 바이너리·대상).

### 4.9 그룹 SET — 설정 (서버 전역 KV, P1)

**REQ-CLI-SET-01**: 서버 설정 조회
시스템은 **항상** `xflow settings get <key>` 가 `GET /settings/{key}`(서버 전역 KV)를 호출하도록
제공해야 한다.

**REQ-CLI-SET-02**: 서버 설정 변경
시스템은 **항상** `xflow settings set <key> <value>` 가 `PUT /settings/{key}` 를 호출하도록 제공
해야 한다.

**REQ-CLI-SET-03**: 로컬 config 와의 분리
시스템은 **항상** `xflow settings ...`(서버 전역 KV)가 기존 `xflow config ...`(로컬
`~/.xflow/config.yaml` 설정)와 **별개**임을 도움말에 명시해야 한다(혼동 방지).

### 4.10 그룹 FLOW2 — 플로우 보완 (P2)

**REQ-CLI-FLOW2-01**: undeploy
시스템은 **항상** `xflow flow undeploy <id|name>` 이 `POST /flows/{id}/undeploy` 를 호출하도록
제공해야 한다(기존 `newFlowActionCmd` 패턴 재사용 — `flow.go:264`).

**REQ-CLI-FLOW2-02**: flow config 수정
시스템은 **항상** `xflow flow config <id|name>`(본문/플래그)가 `PUT /flows/{id}/config` 를
호출하도록 제공해야 한다.

**REQ-CLI-FLOW2-03**: subflow 통계
시스템은 **항상** `xflow flow subflow-stats <id|name>` 이 `GET /flows/{id}/subflow-stats` 를
호출하도록 제공해야 한다.

**REQ-CLI-FLOW2-04**: 노드 configure
시스템은 **항상** `xflow flow node-configure <id|name> <nodeID>`(본문/플래그)가
`POST /flows/{id}/nodes/{nodeID}/configure` 를 호출하도록 제공해야 한다.

**REQ-CLI-FLOW2-05**: tap / taps
시스템은 **항상** `xflow flow tap <id|name> <nodeID>` 가
`POST /flows/{id}/nodes/{nodeID}/tap` 를, `xflow flow taps <id|name>` 가
`GET /flows/{id}/taps` 를 호출하도록 제공해야 한다(tap 라우트가 빌드/플래그 조건부이면 — `flow.go:230`
참고 — 부재 시 미지원 안내).

### 4.11 그룹 AGENT2 — 에이전트 보완 (P2)

**REQ-CLI-AGENT2-01**: enable / disable
시스템은 **항상** `xflow agent enable <id|name>` 이 `POST /agents/{id}/enable` 을,
`xflow agent disable <id|name>` 이 `POST /agents/{id}/disable` 을 호출하도록 제공해야 한다.

**REQ-CLI-AGENT2-02**: agent config 수정
시스템은 **항상** `xflow agent config <id|name>`(본문/플래그)가 `PUT /agents/{id}/config` 를
호출하도록 제공해야 한다.

**REQ-CLI-AGENT2-03**: stats 단독 명령
시스템은 **항상** `xflow agent stats <id|name>` 이 `GET /agents/{id}/stats` 를 호출하도록 제공
해야 한다(기존 `agent topics` 가 `?detail=full` 추출이듯, stats 는 전용 라우트를 직접 호출).

### 4.12 그룹 REMOTE — 원격 관리 (P3, 하위 분할)

> **범위 주의**: remote 도메인은 `internal/api/handler/remote_*.go`/`release_*.go` 의 ~60개 라우트
> 규모로, 단일 단계로 부적합하다. 본 그룹은 **요구사항 골격 + 하위 분할 전략**만 정의하고, 실제
> 구현은 plan.md P3 의 3~4개 하위 단계(또는 별도 하위 SPEC)로 나눈다.

**REQ-CLI-REMOTE-01**: 노드 관리 명령군
시스템은 **항상** `xflow remote node list/get/approve/reject/revoke/pre-register` 가 원격 노드
목록·상세·승인·거부·폐기·사전 등록(SPEC-REMOTE-001 그룹 C/H, `remote_admin.go`/`remote_enrollment.go`)
API 에 매핑되도록 제공해야 한다(하위 단계 P3a).

**REQ-CLI-REMOTE-02**: 그룹/등록토큰 명령군
시스템은 **항상** `xflow remote group ...`(노드 그룹 배정/목록, `remote_grouping.go`)와
`xflow remote token ...`(enrollment 토큰 발급/목록/폐기, `remote_enrollment.go`)을 제공해야 한다
(하위 단계 P3b).

**REQ-CLI-REMOTE-03**: 릴리스/버전 관리 명령군
시스템은 **항상** `xflow remote release ...`(릴리스 CRUD/asset, `release_admin.go`)와
`xflow remote version ...`(타깃 버전/업데이트 소스/버전 이력, `remote_version.go`)을 제공해야
한다(하위 단계 P3c).

**REQ-CLI-REMOTE-04**: 명령 디스패치/인벤토리/감사 명령군
시스템은 **항상** 원격 노드 명령 디스패치, 인벤토리 조회(flows/agents/devices live·mirror),
감사 조회(`remote_query.go`/`remote_admin.go` audit)에 대응하는 명령을 제공해야 한다(하위 단계
P3d — 가장 무거운 영역, 우선순위 최저).

**REQ-CLI-REMOTE-05**: 분할 독립성
시스템은 **항상** P3 하위 단계(P3a~P3d)가 서로 독립적으로 구현·릴리스 가능하도록(한 하위 단계
미완이 다른 하위 단계를 막지 않도록) 명령 그룹을 분리해야 한다.

### 4.13 그룹 LOW — 저우선 도메인 (P4)

**REQ-CLI-DASH-01**: 대시보드 조회/설정
시스템은 **항상** `xflow dashboard shared get/set` 과 `xflow dashboard mine get/set` 이
`GET·PUT /dashboards/{shared,mine}` 에 매핑되도록 제공해야 한다(UI 중심 기능, 낮은 우선순위).

**REQ-CLI-CHART-01**: 차트 채널 목록
시스템은 **항상** `xflow chart channels` 가 `GET /charts/channels` 를 호출하도록 제공해야 한다.

**REQ-CLI-INFLUX-01**: InfluxDB 쿼리
시스템은 **항상** `xflow influxdb query <agent>`(본문/플래그)가 `POST /influxdb/{agent}/query` 를
호출하도록 제공해야 한다.

### 4.14 그룹 STREAM — 실시간 스트림 (선택적 후속)

**REQ-CLI-STREAM-01**: 스냅샷 우선
시스템은 **항상** SSE/WS 스트림(로그/차트/모니터)을 1차적으로 스냅샷/폴링 형태(대응 query/snapshot
API)로 노출해야 하며, 실시간 follow 는 본 SPEC 의 필수 요구가 아니다.

**REQ-CLI-STREAM-02**: 선택적 follow (Optional)
**WHERE** 가능하면, 시스템은 `--follow` 플래그로 SSE 스트림을 tail 하는 기능을 제공할 수 있다
(후속 과제 — 본 SPEC 필수 아님).

---

## 5. 명령 ↔ API 라우트 매핑 (요약)

| 단계 | CLI 명령(군) | API 라우트 | 요구사항 |
| --- | --- | --- | --- |
| P0 | `plugin *` hidden + 미지원 안내(코드 보존) | (없음 — 404 교정, 실구현 SPEC-PLUGIN-001) | REQ-CLI-P0-01 |
| P0 | `status`, `status metrics` 재배선 | `GET /system/version` + `GET /monitor/metrics` + `GET /flows`·`GET /agents`(total) | REQ-CLI-P0-02 |
| P0 | `status logs` 재정의/hidden | `GET /monitor/loglevel` 안내(실시간 로그는 P4 후속) | REQ-CLI-P0-03 |
| P1 | `auth login/logout/whoami/passwd/refresh` | `/auth/login,logout,me,password,refresh` | REQ-CLI-AUTH-01~05 |
| P1 | `device list/get/execute/metadata/history/resolve` | `/devices*` | REQ-CLI-DEV-01~06 |
| P1 | `store query/keys/tags/meta/reset` | `/store/{agent}/*` | REQ-CLI-STORE-01~05 |
| P1 | `tsdb query/series/latest/stats/delete/write` | `/tsdb/*` | REQ-CLI-TSDB-01~06 |
| P1 | `monitor metrics`, `monitor loglevel get/set/reset` | `/monitor/metrics`, `/monitor/loglevel*` | REQ-CLI-MON-01~05 |
| P1 | `system version`, `system update check/apply/rollback/status`, `system channel get/set` | `/system/version`, `/system/update/*` | REQ-CLI-SYS-01~07 |
| P1 | `settings get/set` | `/settings/{key}` | REQ-CLI-SET-01~03 |
| P2 | `flow undeploy/config/subflow-stats/node-configure/tap/taps` | `/flows/{id}/*` | REQ-CLI-FLOW2-01~05 |
| P2 | `agent enable/disable/config/stats` | `/agents/{id}/*` | REQ-CLI-AGENT2-01~03 |
| P3 | `remote node/group/token/release/version/...` | `/remote/*`, `/releases/*` | REQ-CLI-REMOTE-01~05 |
| P4 | `dashboard shared/mine`, `chart channels`, `influxdb query` | `/dashboards/*`, `/charts/channels`, `/influxdb/{agent}/query` | REQ-CLI-DASH/CHART/INFLUX |

---

## 6. 비기능 요구 (Non-Functional)

- **NFR-01 (재사용)**: 신규 명령은 `client.go`/`output.go`/`errors.go`/`resolve.go` 패턴을
  재사용한다. 신규 HTTP 스택·포맷터·에러 매핑을 만들지 않는다(REQ-CLI-X02~X04, A2).
- **NFR-02 (의존성 최소화)**: 신규 외부 의존성을 추가하지 않는다(cobra/viper/표준 net/http 만).
- **NFR-03 (한국어 도움말)**: 모든 명령 도움말/예시는 한국어(기존 톤 일관 — REQ-CLI-X07).
- **NFR-04 (플래그 상속)**: 기존 글로벌 플래그(`--server/--token/--format/--verbose/--quiet/
  --no-color/--config`)를 상속한다(REQ-CLI-X01).
- **NFR-05 (테스트 정합)**: 기존 CLI 테스트 패턴(`*_test.go` — cobra 명령 단위 + HTTP 모킹)을
  따르고 커버리지 85%+ 를 유지한다.
- **NFR-06 (시크릿 안전)**: 비밀번호/토큰은 무에코 프롬프트 또는 stderr 로만 입력받고, 평문 출력/
  로그 기록을 피한다(REQ-CLI-AUTH-04 일관).
- **NFR-07 (회귀 0)**: P0 교정 및 신규 명령 추가가 기존 명령(flow/agent/node/config/modbus/
  interactive/script)에 회귀를 일으키지 않는다.

---

## 7. 추적성 (Traceability)

- 본 SPEC: `SPEC-CLI-004`
- 구현 계획: `plan.md`(단계 P0~P4, 우선순위)
- 수용 기준: `acceptance.md`(도메인별 Given-When-Then)
- 연관 SPEC: SPEC-CLI-001/002/003(기존 CLI), SPEC-AUTH-001/003, SPEC-DEVICE-001,
  SPEC-STORE-001, SPEC-TSDB-001, SPEC-CHART-001, SPEC-WEB-006/007, SPEC-UPDATE-001,
  SPEC-REMOTE-001, SPEC-MODBUS-CLI-001
- 분리 SPEC: `SPEC-PLUGIN-001`(향후 작성) — plugin 시스템 구축(매니저/레지스트리/WASM·Go 동적
  로딩/보안 샌드박스/노드 타입 등록 통합/`/api/v1/plugins` 핸들러/배포·검증) 전담. 본 SPEC 의 P0 는
  plugin CLI 의 hidden 처리·미지원 안내·죽은 호출 제거만 담당한다(§1.6, §4.2 REQ-CLI-P0-01).

---

## 8. 구현 노트 (Implementation Notes)

> 구현 완료(2026-06-27). P0~P4 전 단계를 단계 단위로 증분 구현·검증했다. 본 절은 구현 결과 요약과
> 후속 과제를 기록한다.

### 8.1 단계별 구현 결과 (P0~P4)

P0~P2 는 선행 완료되었고(역방향 불일치 교정 + auth/device/store/tsdb/monitor/system/settings 신규
그룹 + flow/agent 보완), 본 동기화는 새로 완료된 P3·P4 까지 포함한 전 단계를 반영한다.

| 단계 | 구현 내용 | 신규 파일(`internal/cli/`) | 커밋 |
| --- | --- | --- | --- |
| P0 | `plugin *` hidden + 미지원 안내(코드 보존), `status`/`status metrics` 재배선, `status logs` 재정의 | (기존 plugin.go/status.go 교정) | (선행) |
| P1 | `auth`/`device`/`store`/`tsdb`/`monitor`/`system`/`settings` 신규 명령 그룹 | (선행) | (선행) |
| P2 | `flow undeploy/config/subflow-stats/node-configure/tap/taps`, `agent enable/disable/config/stats` 보완 | (선행) | (선행) |
| P3a | `xflow remote node`(list/get/approve/reject/revoke/pre-register) | `remote.go` | `23a5f58`, `c998a58`(get 교정) |
| P3b | `xflow remote group`(list/set/clear/rename/delete/update/command) + `xflow remote token`(create/list/revoke) | `remote_group.go`, `remote_token.go` | `5fbf308` |
| P3c | `xflow remote release`(list/create/delete/delete-asset) + `xflow remote version`(target get·set/source get·set/history/update) | `remote_release.go`, `remote_version.go` | `77c81b2` |
| P3d | `xflow remote command` / `xflow remote audit` / `xflow remote inventory`(flows\|agents\|devices, mirror+live) | `remote_inventory.go` | `4c135ad` |
| P4 | `xflow dashboard`(shared/mine get·set), `xflow chart channels`, `xflow influxdb query` | `dashboard.go`, `chart.go`, `influxdb.go` | `0846ee8` |

신규 명령 파일에는 각각 대응 `*_test.go` 가 동반된다.

### 8.2 재사용 및 의존성

- 모든 신규/보완 명령은 기존 스택을 재사용한다 — `client.go`(HTTP Get/Post/Put/Delete),
  `output.go`(`PrintResult` 포맷터), `errors.go`(`MapAPIError`/`wrapConnectionError`),
  `resolve.go`(이름→ID 해석), `root.go`(글로벌 플래그·서버/토큰 해석). **신규 외부 의존성 0**
  (cobra/viper/표준 net/http 만 — NFR-02 충족).
- 파괴적 작업(release/version 등)은 확인 프롬프트(`confirmAction`) + `--yes` 우회 플래그로 게이트
  한다. enrollment 토큰은 생성 직후 1회만 표시한다(시크릿 안전 — NFR-06).

### 8.3 P3a `get` 개선

P3a 초기 구현(`23a5f58`) 이후 `remote node get` 을 실제 단건 조회 엔드포인트
`GET /remote/nodes/{id}` 로 재배선(`c998a58`)했다. 기존에는 목록 조회 후 클라이언트 측에서 필터링
했으나, 전용 GET 엔드포인트 직접 호출로 정확성과 응답 효율을 개선했다.

### 8.4 품질 상태

- CLI 패키지(`internal/cli`) 커버리지 **85.3%**(목표 85%+ 충족 — NFR-05).
- 전 품질 게이트 통과: `go build`, `go vet`, `go test -race ./...`, `gofmt`, `golangci-lint`.
- 정상 동작하는 기존 명령(flow/agent/node/config/modbus/interactive/script)에 **회귀 0**
  (NFR-07 충족).

### 8.5 후속 과제 (별도 하위 SPEC 후보)

아래 항목은 본 SPEC 범위에서 **의도적으로 제외**되었으며, 후속 하위 SPEC 후보로 남긴다.

- **remote 심화 per-resource 조회**: flow status/logs/nodes, agent stats/config/devices/topics/
  store/sessions/series, device state/commands/metadata 등 원격 노드의 자원별 상세 읽기.
- **remote 편집 CRUD**: 원격 노드 상의 flow/agent 생성·수정·삭제(create/update/delete).
- **SSE/WS 실시간 스트림**: `--follow`(로그 tail), chart WebSocket, 로그 스트림 등 실시간 스트림의
  완전 CLI 재현(§4.14 STREAM 그룹의 선택적 후속 — REQ-CLI-STREAM-02).
- **dashboard delete 라우트**: `DELETE /dashboards/{shared,mine}` 대응 CLI(현재는 get/set 만 구현).
