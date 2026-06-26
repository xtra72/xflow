---
id: SPEC-CLI-004
title: "CLI–Web UI 기능 패리티 — 수용 기준"
version: 0.2.0
status: planned
created: 2026-06-26
updated: 2026-06-26
author: xtra
priority: high
---

# SPEC-CLI-004 수용 기준 (Acceptance Criteria)

> 형식: Given-When-Then. 모든 시나리오는 기존 CLI 테스트 패턴(`internal/cli/*_test.go` — cobra
> 명령 단위 실행 + HTTP 모킹/스텁 서버)과 정합해야 한다. "API 호출"은 모킹된 서버가 기대 경로/
> 메서드를 수신함을 의미한다.

## 1. 횡단 일관성 (그룹 X)

### AC-X01 글로벌 플래그 상속
- Given: 임의의 신규/보완 명령
- When: `--server`/`--token`/`--format`/`--verbose`/`--config` 를 지정해 실행
- Then: 기존 우선순위(플래그>env>config>기본값)로 서버 URL·토큰이 해석되고, 명령별 중복 해석
  로직이 없다.

### AC-X02 출력 포맷
- Given: 데이터를 반환하는 명령
- When: `--format json|yaml|table|text` 각각으로 실행
- Then: `output.go`(`PrintResult`)를 통해 해당 포맷으로 출력되며, 기본값은 `table` 이다.

### AC-X03 인증 주입
- Given: 인증 필요한 엔드포인트 호출 명령 + 유효 토큰
- When: 명령 실행
- Then: 요청에 `Authorization: Bearer <token>` 헤더가 포함된다(`client.go:163-165`).
- And: 토큰 누락/만료 시 401/403 이 `MapAPIError` 의 인증 오류 의미(힌트 포함)로 보고된다.

### AC-X04 에러/연결 처리
- Given: 임의 명령
- When: 서버가 4xx/5xx 를 반환 또는 연결 불가
- Then: 4xx/5xx 는 `MapAPIError(status, body)`, 연결 불가는 `wrapConnectionError`(서버 주소 힌트)
  로 일관 처리된다.

### AC-X05 페이지네이션
- Given: 페이지네이션 지원 목록 API 에 매핑된 명령
- When: `--page`/`--limit`(API 지원 파라미터에 대응) 지정
- Then: 쿼리 파라미터로 전달된다.
- And: API 가 페이지네이션 미지원이면 해당 플래그가 존재하지 않는다.

### AC-X06 이름→ID 해석 / AC-X07 한국어 도움말
- Given: `<id|name>` 인자를 받는 명령
- When: 이름으로 호출 / `--help` 호출
- Then: `resolve.go` 로 ID 해석된다 / 도움말이 한국어로 표시된다.

## 2. P0 — 역방향 불일치 교정

### AC-P0-01 plugin hidden + 미지원 안내
- Given: 빌드된 `xflow`
- When: `xflow --help` 출력 확인 / `xflow plugin` 또는 하위 명령 실행
- Then: plugin 명령이 `--help` 의 명령 목록에 **노출되지 않는다**(`Hidden: true`).
- And: 하위 명령 호출 시 "미지원(SPEC-PLUGIN-001 예정)" 안내가 표시된다.
- And: **`/api/v1/plugins` 호출이 발생하지 않는다**(죽은 호출 제거).
- And: plugin CLI 코드(`internal/cli/plugin.go`)는 보존된다(제거하지 않음).
> 참고: plugin 시스템 자체의 기능 동작(설치/제거/목록 실제 수행) AC 는 본 SPEC 이 아니라
> **SPEC-PLUGIN-001** 에서 다룬다.

### AC-P0-02 status 재배선
- Given: 모킹 서버가 `GET /system/version`·`GET /monitor/metrics`·`GET /flows`·`GET /agents` 를
  제공
- When: `xflow status` 실행
- Then: 네 엔드포인트를 호출해 버전 + 런타임 메트릭 + 플로우/에이전트 수(total) 요약을 표시한다.
  `/api/v1/status` 호출이 발생하지 않는다.
- And: `xflow status metrics` 는 `GET /monitor/metrics` 를 호출한다(`/api/v1/metrics` 아님).

### AC-P0-03 status logs 재정의
- Given: 실시간 로그 API(`/api/v1/logs`) 미존재
- When: `xflow status logs` 실행
- Then: 로그레벨 조회(`GET /monitor/loglevel`) 안내로 재정의되거나(또는 hidden 처리) 미지원
  안내 + `monitor loglevel` 대체 안내가 표시된다. **죽은 `/logs` 호출이 발생하지 않는다.**
- And: 실시간 로그 스트림(SSE)은 본 단계 대상이 아니며 P4-STREAM 후속이다.

### AC-P0-04 회귀 0
- Given: 기존 명령(flow/agent/node/config/modbus)
- When: P0 교정 후 실행
- Then: 모든 기존 명령이 정상 동작한다(기존 테스트 통과).

## 3. P1 — 핵심 신규 도메인

### AC-AUTH (REQ-CLI-AUTH-01~05)
- `auth login --username u --password p` → `POST /auth/login`, 토큰 획득. 토큰 미보유 상태에서도
  호출 가능. `--save` 시 config `auth.token` 저장.
- `auth logout` → `POST /auth/logout`.
- `auth whoami` → `GET /auth/me`, 사용자/역할 표시.
- `auth passwd` → `PUT /auth/password`. 비밀번호는 무에코 프롬프트로 입력, 평문 로그 없음.
- `auth refresh` → `POST /auth/refresh`, 새 access 토큰 표시.

### AC-DEV (REQ-CLI-DEV-01~06)
- `device list` → `GET /devices`.
- `device get <ref>` → `GET /devices/{ref}` (UUID 또는 `agent:local_id`).
- `device execute <id> <cmd> [k=v]` → `POST /devices/{id}/execute`.
- `device metadata set <id> ...` → `PUT /devices/{id}/metadata`; `device metadata del <id>` →
  `DELETE /devices/{id}/metadata`.
- `device history <id>` → `GET /devices/{id}/history` (라우트 부재 시 미지원 안내).
- `device resolve <agent> <name>` → `GET /devices:resolve` 또는 `GET /devices/{agent}/{name}`.

### AC-STORE (REQ-CLI-STORE-01~05)
- `store query <agent>` → `POST /store/{agent}/query`.
- `store keys <agent>` → `GET /store/{agent}/keys` (필터 플래그 → 쿼리).
- `store tags <agent>` → `GET /store/{agent}/tags`.
- `store meta <agent> <key>` → `PUT /store/{agent}/keys/{key}/meta`.
- `store reset <agent> [key]` → key 지정 `DELETE .../keys/{key}`, 미지정 `DELETE .../keys`
  (확인 프롬프트 필수, `--yes` 우회).

### AC-TSDB (REQ-CLI-TSDB-01~06)
- `tsdb query` → `POST /tsdb/query`; `tsdb series` → `GET /tsdb/series`;
  `tsdb latest <key>` → `GET /tsdb/series/{key}/latest`; `tsdb stats` → `GET /tsdb/stats`;
  `tsdb delete <key>` → `DELETE /tsdb/series/{key}` (확인 프롬프트); `tsdb write` →
  `POST /tsdb/write`.

### AC-MON (REQ-CLI-MON-01~05)
- `monitor metrics` → `GET /monitor/metrics`.
- `monitor loglevel get` → `GET /monitor/loglevel`.
- `monitor loglevel set <level>` → `PUT /monitor/loglevel`.
- `monitor loglevel set <component> <level>` → `PUT /monitor/loglevel/{component}`.
- `monitor loglevel reset <component>` → `DELETE /monitor/loglevel/{component}`.

### AC-SYS (REQ-CLI-SYS-01~07)
- `system version` → `GET /system/version`.
- `system update check` → `POST /system/update/check`.
- `system update apply` → `POST /system/update/apply` (확인 프롬프트).
- `system update rollback` → `POST /system/update/rollback`.
- `system update status` → `GET /system/update/status`.
- `system channel get` → `GET /system/update/channel`; `system channel set <ch>` →
  `PUT /system/update/channel`.
- `system --help` 에 "원격 서버 제어, `xflowd update`(로컬 데몬)와 별개" 명시.

### AC-SET (REQ-CLI-SET-01~03)
- `settings get <key>` → `GET /settings/{key}`; `settings set <key> <value>` →
  `PUT /settings/{key}`.
- `settings --help` 에 "서버 전역 KV, `xflow config`(로컬 파일)와 별개" 명시.

## 4. P2 — 부분 도메인 보완

### AC-FLOW2 (REQ-CLI-FLOW2-01~05)
- `flow undeploy <id|name>` → `POST /flows/{id}/undeploy`.
- `flow config <id|name>` → `PUT /flows/{id}/config`.
- `flow subflow-stats <id|name>` → `GET /flows/{id}/subflow-stats`.
- `flow node-configure <id|name> <nodeID>` → `POST /flows/{id}/nodes/{nodeID}/configure`.
- `flow tap <id|name> <nodeID>` → `POST /flows/{id}/nodes/{nodeID}/tap`;
  `flow taps <id|name>` → `GET /flows/{id}/taps` (조건부 라우트 부재 시 미지원 안내).
- And: 기존 flow 명령(list/get/deploy/start/stop/...) 회귀 0.

### AC-AGENT2 (REQ-CLI-AGENT2-01~03)
- `agent enable <id|name>` → `POST /agents/{id}/enable`;
  `agent disable <id|name>` → `POST /agents/{id}/disable`.
- `agent config <id|name>` → `PUT /agents/{id}/config`.
- `agent stats <id|name>` → `GET /agents/{id}/stats`.
- And: 기존 agent 명령(list/get/start/stop/topics/...) 회귀 0.

## 5. P3 — 원격 관리 (하위 분할)

### AC-REMOTE (REQ-CLI-REMOTE-01~05)
- **P3a**: `remote node list/get/approve/reject/revoke/pre-register` 가 대응 `/remote/*` API 에
  매핑된다.
- **P3b**: `remote group ...`, `remote token ...` 가 그룹핑/등록토큰 API 에 매핑된다.
- **P3c**: `remote release ...`, `remote version ...` 가 릴리스/버전 API 에 매핑된다.
- **P3d**: 명령 디스패치/인벤토리/감사 명령이 대응 API 에 매핑된다.
- And: 각 하위 단계는 독립적으로 빌드·테스트 가능하다(한 하위 단계 미완이 타 단계를 막지 않음).
- And: 구현 직전 각 `remote_*.go`/`release_*.go` 라우트가 재확인된다.

## 6. P4 — 저우선 도메인

### AC-LOW (REQ-CLI-DASH/CHART/INFLUX)
- `dashboard shared get/set`, `dashboard mine get/set` → `GET·PUT /dashboards/{shared,mine}`.
- `chart channels` → `GET /charts/channels`.
- `influxdb query <agent>` → `POST /influxdb/{agent}/query`.

### AC-STREAM (REQ-CLI-STREAM-01~02, 선택)
- 스트림 데이터는 스냅샷/폴링으로 노출된다.
- (선택) `--follow` 지정 시 SSE tail 이 동작한다(후속 과제, 필수 아님).

## 7. 품질 게이트 (Quality Gates)

| 게이트 | 기준 |
| --- | --- |
| 명령 존재 | 각 REQ 의 명령이 cobra 트리에 등록됨 |
| API 매핑 정확 | 모킹 서버가 기대 경로/메서드 수신(§spec §1.4 근거 일치) |
| 포맷 옵션 | 모든 데이터 명령이 `--format` 4종 지원 |
| 인증/권한 | Bearer 주입 + 401/403 의미 보존 |
| 에러 처리 | `MapAPIError`/`wrapConnectionError` 재사용 |
| 패턴 일관성 | `client.go`/`output.go`/`errors.go`/`resolve.go` 재사용, 신규 의존성 0 |
| 테스트 | `*_test.go` 추가, `go test -race ./...` 통과, 커버리지 85%+ |
| 정적 분석 | `go vet`, `golangci-lint` 통과 |
| 회귀 0 | 기존 명령(P0 교정 포함) 정상 동작 |
| 보안 | 비밀번호/토큰 무에코·평문 로그 금지 |

## 8. 완료 정의 (Definition of Done)

- 대상 단계의 모든 REQ 에 대응하는 AC 가 통과한다.
- spec.md §5 명령↔API 매핑 표의 해당 행이 모두 구현·검증된다.
- 횡단 요구(REQ-CLI-X01~07) 및 비기능 요구(NFR-01~07)가 충족된다.
- plan.md §4 결정 사항(D1~D6)이 코드/도움말에 반영된다.
