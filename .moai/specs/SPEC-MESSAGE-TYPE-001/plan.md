---
id: SPEC-MESSAGE-TYPE-001
title: 구현 계획 - 메시지 분류 채널 단일화 (message.Type() 1급 채택)
version: 0.1.0
status: planned
created: 2026-05-26
updated: 2026-05-26
author: xtra
priority: medium
---

# SPEC-MESSAGE-TYPE-001 Plan: 메시지 분류 채널 단일화 작업 계획

본 문서는 SPEC-MESSAGE-TYPE-001 의 단일 PR 전환 전략을 작업 단위로 분해하고, 사이트별 우선순위·검증 방법·롤백 전략을 정의한다.

## TAG Traceability

- @PLAN:SPEC-MESSAGE-TYPE-001
- 관련: @SPEC:SPEC-MESSAGE-TYPE-001 (spec.md), @ACCEPTANCE:SPEC-MESSAGE-TYPE-001 (acceptance.md)

---

## 0. 기본 원칙

- **단일 PR 권장**: 모든 emit 사이트 + 다운스트림 + 테스트 + Lua bridge 동시 갱신. greenfield 환경 (외부 호환 불요) 이므로 호환 alias 미적용.
- **사이트별 우선순위**: 영향 큰 것 (emit 사이트) → 다운스트림 → 테스트 → 직렬화 정책 → frontend 순.
- **검증성**: 각 그룹별 grep 검증 + 단위 테스트 + `go build` PASS + `go test -race ./...` PASS.
- **관측성**: 본 SPEC 은 메트릭 노출 불요 (단일 PR 직접 cut).

---

## 1. 작업 분해 (Tasks)

### 1.1 그룹 T1: emit 사이트 일괄 전환 (우선순위 매우 높음)

- **T1-1**: `internal/node/trigger.go` (line 482) — `WithMetadata("message_type", "event")` → `WithType("event")` 단일 라인 치환.
- **T1-2**: `internal/agent/lg/lgcnp_agent.go` — `emitDeviceStateLocked` 시리즈에서 `WithMetadata("message_type", "device_state.<trigger>")` → `WithType("device_state.<trigger>")` 또는 `msg.SetType(...)`.
  - `<trigger>` 값 확인: `change` / `poll` / `response` 등.
- **T1-3**: `internal/agent/lg/lgcp_agent.go` — 동일 패턴 적용.
- **T1-4**: `internal/agent/lg/agent.go` (LGAP) — 동일 패턴 적용.
- **T1-5**: `internal/agent/samsung/agent.go` (NASA) — 동일 패턴 적용.
- **T1-6**: `internal/agent/century/message.go` — 동일 패턴 적용.
- **T1-7**: `internal/node/inventory.go` — 신규 `WithType("inventory.event")` 추가 (현재 type 미설정 — 인벤토리 emit 의 1급 분류 식별 누락 해소).
- **T1-8**: `internal/node/debug.go` — message_type metadata 설정 분기 제거, 필요 시 `SetType("debug.log")` 적용.
- **T1-9**: `internal/node/bridge.go` — message_type 설정 분기 → `SetType` 기반.

### 1.2 그룹 T2: 다운스트림 분기 갱신 (우선순위 높음)

- **T2-1**: `internal/node/transform.go` — metadata.message_type lookup 분기 → `msg.Type()` 기반 switch/case 갱신.
- **T2-2**: `internal/node/nasa.go`, `lgcnp.go`, `lgcp.go`, `lgap.go`, `century.go` — 각 노드의 입력 메시지 분류 분기를 `msg.Type()` 기반으로 갱신.
- **T2-3**: `internal/node/dedup_helper.go` — metadata.message_type 키 lookup 사용처 제거.
- **T2-4**: `internal/node/script.go` (Lua bridge) — Lua 컨텍스트 매핑:
  - input path: `msg.Type()` 값을 Lua 의 `msg.type` 키로 노출.
  - input path: `msg.metadata.message_type` 키 매핑 제거.
  - output path: Lua 의 `msg.type = "..."` 할당을 `SetType(...)` 로 변환.

### 1.3 그룹 T3: Lua 스크립트 마이그레이션 가이드 (우선순위 중간)

- **T3-1**: `docs/migration/message-type.md` 신규 작성 — Lua 스크립트의 `msg.metadata.message_type` → `msg.type` 갱신 안내.
- **T3-2**: script 노드의 README / 도움말 (`web/src/` 의 script 노드 docs) 갱신 — `msg.type` 컨벤션 안내, `msg.metadata.message_type` 사용 중단 명시.

### 1.4 그룹 T4: JSON 직렬화 정책 적용 (M6 옵션 A)

- **T4-1**: `pkg/message/message.go` — JSON struct tag 갱신:
  - `Type string \`json:"type,omitempty"\`` 형식 적용 (옵션 A — omitempty).
- **T4-2**: 직렬화 결과 검증 단위 테스트 추가 — 빈 `Type()` 케이스에서 JSON 출력에 `"type"` 키 부재 확인.

### 1.5 그룹 T5: 테스트 일괄 갱신 (우선순위 높음)

- **T5-1**: `internal/agent/century/message_test.go` — metadata.message_type 어서션 → `msg.Type()` 어서션.
- **T5-2**: `internal/agent/century/agent_device_state_test.go` — 동일 갱신.
- **T5-3**: `internal/node/transform_test.go` — 테스트 setup 의 `WithMetadata("message_type", ...)` → `WithType(...)`, 어서션도 갱신.
- **T5-4**: `internal/node/lgcp_test.go` — 동일 갱신.
- **T5-5**: `internal/node/mqtt_test.go` — 동일 갱신.
- **T5-6**: `internal/node/nasa_test.go` — 동일 갱신.
- **T5-7**: `internal/node/modbus_poller_test.go` — 동일 갱신.
- **T5-8**: `internal/node/dedup_helper_test.go` — 동일 갱신.

### 1.6 그룹 T6: Frontend 영향 검토 (우선순위 낮음)

- **T6-1**: `grep -rn 'message_type' web/src/` — script 노드 docs / debug 노드 표시 / TypeScript 타입 정의 검토.
- **T6-2**: 발견된 사용처를 `type` (top-level) 기준으로 갱신:
  - debug 노드의 분류 라벨 표시 — `msg.type` 값 사용, 빈 값 시 "Unclassified" fallback.
  - script 노드 docs 의 예시 — `msg.metadata.message_type` → `msg.type`.
  - TypeScript Message 타입 정의 — `type: string` (omitempty 반영 시 `type?: string`).

### 1.7 그룹 T7: CI grep 검증 추가

- **T7-1**: `.github/workflows/` 또는 `Makefile` 에 grep 기반 검증 추가:
  ```bash
  # WithMetadata("message_type", ...) 호출 0건 확인
  ! grep -rn 'WithMetadata("message_type"' internal/ pkg/

  # metadata.Get("message_type") 또는 ["message_type"] lookup 0건 확인
  ! grep -rn 'Get("message_type")\|\["message_type"\]' internal/ pkg/
  ```
- **T7-2**: 검증 실패 시 PR merge 차단.

---

## 2. 사이트별 우선순위

### 2.1 High Priority (영향 큰 사이트, 먼저 처리)

| 사이트 | 사유 |
|--------|------|
| HVAC 5 에이전트 emit (lgcnp_agent / lgcp_agent / LGAP agent.go / Samsung agent.go / century message.go) | 메시지 빈도 가장 높음, 분류 식별이 다운스트림 분기에 직접 영향 |
| trigger 노드 emit (`internal/node/trigger.go`) | 이벤트 entry point, "event" 분류의 시작점 |
| transform 노드 분기 (`internal/node/transform.go`) | 대부분의 다운스트림 분류 로직이 여기서 분기 |
| script 노드 Lua bridge (`internal/node/script.go`) | 사용자 정의 스크립트와의 호환 인터페이스 |

### 2.2 Medium Priority

| 사이트 | 사유 |
|--------|------|
| inventory 노드 (`internal/node/inventory.go`) | type 미설정 결함 해소 (1급 빈 문자열 문제) |
| debug / bridge / dedup 노드 | 보조 분류 사용, 영향 범위 한정적 |
| HVAC 노드 (nasa.go / lgcnp.go / lgcp.go / lgap.go / century.go) | 입력 메시지 분류 분기 갱신 |

### 2.3 Low Priority

| 사이트 | 사유 |
|--------|------|
| 테스트 파일 일괄 갱신 | production 변경 후 일괄 처리 가능 |
| pkg/message/message.go JSON tag (M6) | 마지막에 적용, downstream 정합 후 |
| frontend 검토 | 가장 후순위, 백엔드 변경 검증 후 |
| 마이그레이션 가이드 (`docs/migration/message-type.md`) | 코드 변경 후 작성 |

---

## 3. 검증 방법

### 3.1 빌드 검증

```bash
go build ./...
# 모든 패키지 컴파일 성공 (실패 시 시그니처 누락 또는 import 결함)
```

### 3.2 단위 테스트

```bash
go test -race ./...
# 모든 단위 테스트 통과
# 신규 갱신된 어서션 (msg.Type() 검증) 정상 동작 확인
```

### 3.3 grep 기반 정합성 검증

```bash
# 1. WithMetadata("message_type", ...) 호출 0건 확인 (production + test)
grep -rn 'WithMetadata("message_type"\|WithMetadata(`message_type`' internal/ pkg/
# 기대: empty result

# 2. metadata 의 message_type 키 lookup 0건 확인
grep -rn 'Get("message_type")\|\["message_type"\]' internal/ pkg/
# 기대: empty result

# 3. SetType / WithType 호출 사이트 검증 (예상 증가)
grep -rn 'SetType\|WithType(' internal/ pkg/ | wc -l
# 기대: 62 (현재) + 신규 추가 (각 emit 사이트별)
```

### 3.4 통합 시나리오 검증

- **시나리오 1**: trigger 이벤트 → transform 노드 → debug 노드 — 분류가 모든 단계에서 `msg.Type() == "event"` 로 일관.
- **시나리오 2**: HVAC LGCNP 디바이스 상태 변경 → lgcnp 노드 → bridge 노드 — 분류가 `msg.Type() == "device_state.change"` 로 일관.
- **시나리오 3**: inventory 노드 emit → JSON 직렬화 검증 — top-level `"type": "inventory.event"` 노출, metadata 에 message_type 키 부재.
- **시나리오 4**: Lua 스크립트가 `msg.type` 읽기/쓰기 — 양방향 변환 정상.
- **시나리오 5**: 빈 type 의 메시지가 다운스트림에 도달 — unclassified 분기로 graceful fall-through.

### 3.5 JSON 직렬화 정책 검증 (M6)

- 옵션 A (omitempty) 채택 시:
  - 빈 `Type()` 의 JSON 출력에 `"type"` 키 부재 확인.
  - 값 있는 `Type()` 의 JSON 출력에 `"type": "<value>"` 노출 확인.

### 3.6 Frontend 영향 검증

- `grep -rn 'message_type' web/src/` 결과 0건 또는 마이그레이션 완료.
- debug 노드 UI 의 분류 라벨이 정상 표시 (빈 값 시 "Unclassified" fallback).

---

## 4. 롤백 전략

### 4.1 단일 PR 롤백

- `git revert <merge-commit>` 으로 즉시 전체 롤백 가능.
- 영속 데이터 변경 없음 — 인터페이스 + 직렬화 정책만 변경되므로 메시지 큐 / DB 영향 없음.

### 4.2 부분 롤백 시나리오

- T4 (JSON 직렬화 정책) 만 롤백 필요 시: `pkg/message/message.go` 의 struct tag 갱신만 revert.
- T2 (다운스트림 분기) 만 회귀 발견 시: 해당 노드 파일만 revert + production hotfix.

### 4.3 영속 메시지 호환 (A2 가정)

- 외부 MQTT broker / 메시지 큐에 직렬화된 `metadata.message_type` 키를 가진 inflight 메시지가 잔존하더라도:
  - 새 코드는 metadata 의 message_type 키를 무시 (graceful — 해당 메시지의 `Type()` 은 빈 문자열).
  - 다운스트림은 unclassified 분기로 fall-through.
- A2 가정으로 잔존 가능성 낮음 — 실제 발견 시 마이그레이션 스크립트 별도 검토.

---

## 5. 단일 commit 메시지

```
docs(spec): SPEC-MESSAGE-TYPE-001 — message.Type() 단일화 (metadata.message_type 폐기)
```

구현 PR 의 commit 메시지 (별도):

```
refactor(message): message.Type() 1급 채널 단일화 (metadata.message_type 폐기)

- emit 사이트 일괄 전환 (trigger / HVAC 5 / inventory / bridge / debug)
- 다운스트림 분기 갱신 (transform / dedup / script Lua bridge)
- 테스트 어서션 갱신 (8 파일)
- JSON 직렬화 정책: type omitempty 적용
- frontend: message_type 사용처 type 으로 갱신
- CI grep 검증 추가

Refs: SPEC-MESSAGE-TYPE-001

BREAKING CHANGE (내부): metadata.message_type 키 폐기, Lua 스크립트의 msg.type
사용으로 갱신. greenfield 환경 가정 (외부 클라이언트 부재) 으로 호환 alias 미제공.
```

---

## 6. 기술 부채 / 후속 작업

- **SCHEMA 식별자 typed constant 도입**: 현재 string literal 사용. 별도 SPEC 으로 `MessageType = "event" | "device_state.change" | ...` enum 화 검토.
- **script 노드의 type 기반 dispatch 헬퍼**: Lua 측에서 `script.OnType("event", fn)` 같은 헬퍼 제공. 별도 SPEC 으로.
- **외부 통합 (MQTT bridge) 의 type 노출 정책**: 별도 SPEC.
- **debug 노드의 분류별 색상/아이콘 매핑**: UI 개선 별도 SPEC.

---

## 7. Status: planned

본 SPEC 은 **계획 단계** 이며, 단일 PR 직접 전환 전략을 따른다. 구현은 별도 세션 (`/moai run SPEC-MESSAGE-TYPE-001`) 에서 진행한다.
