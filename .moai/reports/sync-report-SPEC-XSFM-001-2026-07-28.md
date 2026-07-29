# Sync Report — SPEC-XSFM-001

- **SPEC**: SPEC-XSFM-001 — 지하철 역사 설비 관리 에이전트 (MQTT)
- **동기화 일자**: 2026-07-28
- **구현 커밋**: `bbd6365`
- **버전**: v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier M)
- **Phase**: /moai sync (문서 동기화 + 3-phase close)

---

## SPEC 상태 전이

| 항목 | 이전 | 이후 |
|------|------|------|
| `status` | `in-progress` | `completed` |
| `version` | `0.3.0` | `0.3.0` (유지) |
| `updated` | `2026-07-28` | `2026-07-28` (유지) |

spec.md / plan.md / acceptance.md **본문은 수정하지 않았다**(ownership 경계: manager-docs는 sync 커밋에서 frontmatter `status`+`updated`만 변경). 구현 노트·분기는 CHANGELOG + 본 리포트에 기록했다.

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-XSFM-001/spec.md` | frontmatter `status: in-progress → completed` (본문 무변경) |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — xsfm 에이전트" 항목 추가 (기능 + 분기 요약) |
| `.moai/project/product.md` | "제공 Agent 목록" 표에 Subway Facilities Manager 행 추가 (도메인/시설물 유형) |
| `.moai/project/structure.md` | `internal/agent/` 커스텀 Agent 그룹에 `xsfm/` 패키지 항목 추가 (파일 구성·통합 지점·분기) + "Agent 활용 예시"에 Subway Facilities Manager 행 추가 |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `.moai/project/tech.md` | 중복(redundant). MQTT/Eclipse Paho는 이미 tech.md에 문서화되어 있고 xsfm는 기존 스택 재사용 + 신규 외부 의존성 0 → 신규 기술 없음. |
| `README.md` | (1) xsfm 전용 example YAML(`examples/agents|flows/`) 파일이 존재하지 않아 example 표에 추가할 행 없음. (2) README의 `internal/agent/` 트리는 예시성/비완전 목록(samsung/century/thingplus 모두 미기재) → scope discipline 위해 트리 편집 생략. |

---

## 구현 요약 (참조)

지하철 역사 시설물 관리 비전의 첫 디바이스로, MQTT 기반 설비를 개별·그룹·역사(station)·호선(line) 단위로 제어·모니터링하는 `xsfm` 에이전트. 13개 마일스톤, 신규 패키지 + 노드/디바이스 어댑터/스토리지/와이어링/프론트엔드로 구현.

- **듀얼 트랜스포트(direct/port)**: I/O 경계(CommandSink + 상태 ingress) 추상화, direct=브로커 직접 소유 / port=외부 mqtt-in/out 노드가 I/O 담당(상태 입력·제어 출력 포트 분리).
- **개별 2축 제어**: `set_power`/`set_fan_speed`(1/2/3)/`set_multiple` + 전원 OFF 게이트, **응답 대기**(pending-command 레지스트리 + 에코 상관, `ErrControlTimeout`, 동시성 안전).
- **일괄 제어**: 그룹 + station + line 셀렉터 팬아웃(우선순위 device_id>station>line>group_id, best-effort 집계).
- **역사 레지스트리**: station→line SSOT(`device_metadata` 패턴) + 위치 계층(station/place/index).
- **모니터링·감사·영속화**: observed 기반 방출 + 오프라인 감지(LWT+타임아웃), 제어/그룹 감사, 로스터 영속화(device_id 키잉, v0.2.0 하위호환).
- **통합**: 플로우 노드(`xsfm-status`/`xsfm-control`) + 디바이스 어댑터(`CommandSpec`) + `cmd/xflowd/main.go` 와이어링 + API 어댑터 + 웹 스키마.

**구현 파일**: 16개 소스 + 12개 테스트(`internal/agent/xsfm/`), `internal/node/xsfm.go`(+테스트), `internal/device/adapter/xsfm.go`(+테스트), `internal/storage/station_registry_repository.go`(+테스트), `cmd/xflowd/main.go`, `internal/api/service/agent_adapter.go`, `web/src/config/agentSchemas.ts`, `web/src/pages/agents/agentTypeMeta.ts`.

---

## 분기 (Divergence Report)

| 항목 | SPEC 계획 | 실제 구현 | 판정 |
|------|-----------|-----------|------|
| **i18n** | acceptance 9.1: en/ko i18n 문자열 | 코드베이스 관례상 에이전트 설정 필드 라벨은 `agentSchemas.ts` + `agentTypeMeta.ts`(하드코딩 한국어)에 위치 — 에이전트별 i18n JSON 아님. xsfm는 관례를 따르며 죽은 i18n 키를 추가하지 않음 | acceptance 문구 대비 관례 준수 편차(문서화됨) |
| **LWT 토픽 스킴** | 오프라인 전이 로직 | 구조적/테스트 가능하게 구현했으나 라이브 LWT 토픽 구독 와이어링은 config 기반 확정으로 이연(디바이스 매뉴얼 미확보, SPEC 가정 A-1) | 이연 |
| **파일 분해** | plan.md에 일부 파일만 명시 | audit.go/monitor.go/status.go/provider.go/pending.go 등으로 자연 분할 | 정상 분해(스코프 확장 아님) |

---

## 품질 게이트 결과 (sync-phase 재검증, 관측 증거)

sync 시점에 xsfm 관련 패키지에 대해 직접 재검증한 결과:

| 항목 | 명령 | 결과 |
|------|------|------|
| 빌드 | `go build ./...` | exit=0 (`/tmp/moai-verify/build.log`) |
| vet | `go vet ./internal/agent/xsfm/... ./internal/node/... ./internal/device/adapter/... ./internal/storage/...` | exit=0 (`/tmp/moai-verify/vet.log`) |
| xsfm 커버리지 | `go test -cover ./internal/agent/xsfm/...` | `ok ... coverage: 85.1% of statements`, exit=0 (`/tmp/moai-verify/cov.log`) |
| 통합 패키지 테스트 | `go test ./internal/node/... ./internal/device/adapter/... ./internal/storage/... ./internal/api/service/...` | 모두 `ok`, exit=0 (`/tmp/moai-verify/integ.log`) |

- **golangci-lint**(run-phase 보고): xsfm 0 issues / node 패키지 2건 pre-existing dead-code(무관).
- **race**(run-phase 보고): `go test -race` 클린.

---

## Pre-existing 무관 실패 (xsfm 아님)

전체 스위트 실행 시 다음 패키지가 실패하나, xsfm 작업과 **무관**하며 clean develop HEAD에서도 재현됨(본 작업을 stash 후 재현 확인):

- **`internal/agent/serial`**: 결정론적(deterministic) 실패.
- **`internal/agent/century`**: 부하 의존(load-flaky) 실패.

xsfm 관련 패키지(위 품질 게이트 표)는 전부 green.

---

## 비고

- 소스 코드/테스트는 수정하지 않았다(문서 동기화 전용).
- 무관 untracked 파일(`data.json`, `output.json`, `packet.json`, `SPEC-FACILITY-DASHBOARD-001/`, `statusline.yaml`, `last-session-state.json`, `settings.local.json.lock`)은 스테이징하지 않았다.
- 대시보드 패널은 본 SPEC 범위 외(SPEC-FACILITY-DASHBOARD-001) — 본 SPEC은 소비할 디바이스 계층 + 역사 레지스트리 + fan-out만 소유.
- `auto_push=false`: push 및 PR 생성 없음(오케스트레이터가 별도 처리).
