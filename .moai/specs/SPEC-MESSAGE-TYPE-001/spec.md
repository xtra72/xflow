---
id: SPEC-MESSAGE-TYPE-001
title: 메시지 분류 채널 단일화 - message.Type() 1급 인터페이스 채택, metadata.message_type 폐기
version: 0.1.1
status: completed
created: 2026-05-26
updated: 2026-05-26
author: xtra
priority: medium
tags: [message, type, refactoring]
related_spec: SPEC-MSG-001, SPEC-INVENTORY-001, SPEC-BRIDGE-001, SPEC-DEBUG-001, SPEC-SCRIPT-001
---

# SPEC-MESSAGE-TYPE-001: 메시지 분류 채널 단일화 - message.Type() 1급 인터페이스 채택

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MESSAGE-TYPE-001 |
| 제목 | 메시지 분류 채널 단일화 - `message.Type()` 1급 인터페이스 채택, `metadata.message_type` 폐기 |
| 버전 | 0.1.0 |
| 상태 | planned |
| 작성일 | 2026-05-26 |
| 최종 수정 | 2026-05-26 |
| 작성자 | xtra |
| 우선순위 | medium (시스템 광역 정합, 단 외부 클라이언트 영향 없음) |
| 관련 SPEC | SPEC-MSG-001 (Message 인터페이스), SPEC-INVENTORY-001 (inventory emit), SPEC-BRIDGE-001 (bridge classification), SPEC-DEBUG-001 (debug 분류), SPEC-SCRIPT-001 (Lua script 컨텍스트) |
| 도메인 | Message Infrastructure / System-wide Refactoring |

---

## HISTORY

- **0.1.1** (2026-05-26): 구현 완료 — 4 커밋 누적 (`048fac3` T1+T2 production / `ff8b903` T3 Lua bridge + 테스트 / `ba3004f` T3 마이그레이션 가이드 + T5 주석 / HISTORY 갱신). T1~T7 모두 충족. M1~M6 모든 EARS 모듈 통과. status: completed.
- **0.1.0** (2026-05-26): 최초 작성 — 메시지 분류 이중 채널 (1급 `message.Type()` + `metadata.message_type` map 키) 문제 정의, 옵션 A (1급 채널 단일화) 채택, 6개 EARS 모듈 (M1~M6) 및 단일 PR 전환 전략 수립. greenfield 단일 운영자 환경에 따라 Soft Deprecation 단계 생략.

| Version | Date       | Author | Change                                                                                  |
| ------- | ---------- | ------ | --------------------------------------------------------------------------------------- |
| 0.1.1   | 2026-05-26 | xtra   | 구현 완료 — 4 커밋 누적, T1~T7 충족, M1~M6 모든 EARS 모듈 통과, status: completed       |
| 0.1.0   | 2026-05-26 | xtra   | 최초 작성 — `metadata.message_type` → `message.Type()` 단일화 계획, M1~M6 EARS 모듈 정의 |

---

## 1. 개요 (Overview)

xflow 시스템은 노드 간 흐르는 메시지의 **schema/type 식별**을 위한 두 가지 분류 채널이 공존하는 비대칭 상태에 있다:

1. **1급 인터페이스 메서드** — `pkg/message/message.go` 의 `Type() string` / `SetType(string)` / `WithType(string) Option`. JSON 직렬화 시 top-level `"type"` 필드로 노출.
2. **metadata map 키** — `WithMetadata("message_type", "<value>")` 컨벤션 (v0.9.0 도입). JSON 직렬화 시 `metadata.message_type` 으로 노출.

본 SPEC 은 두 채널을 **`message.Type()` 1급 인터페이스로 단일화** 하며, `metadata.message_type` 키 컨벤션을 **완전 폐기** 한다. greenfield 단일 운영자 환경 (외부 클라이언트 부재) 에 따라 Soft Deprecation 단계 없이 **단일 PR 직접 cut** 전략을 채택한다.

> 본 SPEC 은 **계획만** 정의한다. 실제 구현은 별도 단계 (`/moai run SPEC-MESSAGE-TYPE-001`) 와 단일 PR 로 진행한다.

---

## 2. 배경 (Background)

### 2.1 이중 분류 채널의 역사적 원인

**Phase 0 (초기 설계)** — `pkg/message/message.go` 는 초기부터 `Type() / SetType()` 를 1급 인터페이스로 제공했다. 사용 사이트: `century.go`, `nasa.go`, `mqtt.go`, `serial_io.go`, `tcp_io.go`, `lg_hvacr01.go`, `bridge.go`, `influxdb_query.go`, `transform_test.go`, `expression.go`, `expr_eval.go` 등 25개 파일.

**Phase 1 (v0.9.0 — metadata.message_type 컨벤션 도입)** — HVAC 5 에이전트 (LG HVACR-02/LG HVACR-01/LGAP/Samsung/Century) 의 `emitDeviceStateLocked` 시리즈와 trigger 노드가 분류 식별을 위해 새로운 컨벤션 `WithMetadata("message_type", "event")` 또는 `WithMetadata("message_type", "device_state.<trigger>")` 를 도입했다. 이는 1급 메서드 존재를 인지하지 못한 채 도메인별 컨벤션으로 자리 잡았다.

### 2.2 현재 상황 통계

- **`WithMetadata("message_type", ...)` 직접 호출**: 1건 (`internal/node/trigger.go:482`)
- **`metadata.message_type` 키 참조** (production + test): 23 파일
- **`SetType` / `WithType` 호출**: 25 파일 (이미 광범위 사용)
- **두 채널 모두 설정하는 사이트**: 거의 없음
- **두 채널 중 어느 것도 설정 안 함**: 일부 존재 — inventory 노드 emit 시 trigger metadata 만 inherited (1급 `Type()` 빈 문자열).

### 2.3 증상 (Symptom)

inventory 노드 출력 (실제 관측):

```json
{
  "type": "",                          // 1급 필드, 빈 값
  "payload": {...},
  "metadata": {
    "message_type": "event",           // map 키, 실제 분류 값
    ...
  }
}
```

- top-level `"type"` 가 빈 문자열로 노이즈가 됨.
- 다운스트림 분류 로직이 두 채널 사이에서 양분 — 어떤 사이트는 `msg.Type()` 만 검사, 어떤 사이트는 `msg.Metadata().Get("message_type")` 만 검사.
- 신규 노드 구현 시 어느 채널을 써야 하는지 컨벤션 부재.

### 2.4 현재 시스템의 문제점

| 영역 | 현재 상태 | 문제 |
|------|----------|------|
| 분류 채널 | 1급 + metadata 두 채널 공존 | 양분된 진실 |
| trigger 노드 emit | `WithMetadata("message_type", "event")` | 1급 `Type()` 빈 문자열 |
| HVAC 5 에이전트 emit | metadata 만 설정 | 1급 `Type()` 빈 문자열, JSON top-level 노이즈 |
| inventory 노드 emit | 어느 채널도 설정 안 함 (trigger 의 metadata inherited) | top-level `type=""` |
| transform 노드 | metadata 키 lookup 분기 | type-safe 부재 |
| script 노드 (Lua) | `msg.metadata.message_type` 컨벤션 노출 | 호환성 부담 |
| debug 노드 | 양쪽 모두 표시 시도 | 표시 일관성 부재 |
| JSON 직렬화 | top-level `"type"` 빈 문자열 + `metadata.message_type` 키 노이즈 | downstream 파싱 복잡도 |

### 2.5 이중 채널이 만드는 비용

- **양분된 진실**: 한 시스템 안에 두 종류의 분류 채널이 공존하여 디버깅·로깅·필터·분기 일관성 부재.
- **type-safety 부재**: metadata 는 `map[string]any` 의 string lookup — 컴파일 타임 보장 없음.
- **visibility 저하**: `msg.Metadata().Get("message_type")` 가 `msg.Type()` 보다 훨씬 verbose.
- **JSON 출력 노이즈**: top-level `"type"` 가 빈 채로 직렬화되고, 실제 값은 metadata 안에 묻혀 있음.
- **신규 사이트의 일관성 부재**: HVAC 5 에이전트 패턴 (metadata) vs 다른 노드 패턴 (1급) 의 혼재로 신규 노드가 어느 쪽을 따를지 모호.

---

## 3. 사용자 결정 (Decision: 옵션 A — 1급 채널 단일화)

본 SPEC 은 **옵션 A** 를 채택한다: `metadata.message_type` 폐기 + `message.Type()` 1급 인터페이스 단일화.

### 3.1 채택 근거

| 기준 | 1급 `Type()` | metadata.message_type |
|---|---|---|
| Type safety | O (인터페이스 메서드) | X (`map[string]any` lookup) |
| Visibility | O (`msg.Type()`) | X (`msg.Metadata().Get("message_type")`) |
| JSON 출력 | O (top-level `"type"`) | X (`metadata` map 안에 묻힘) |
| 다운스트림 simplification | O (단일 호출) | X (양분 분기) |
| 신규 사이트 컨벤션 | O (1급 메서드는 명백) | X (map 키 컨벤션 학습 필요) |
| 기존 코드베이스 영향 | 25 파일 (이미 다수 사용 중) | 23 파일 |

1급 메서드가 모든 평가 축에서 우위. 이미 광범위하게 사용 중이므로 metadata 컨벤션을 폐기해 단일화하는 것이 자연스러움.

### 3.2 규칙

1. **모든 emit 사이트는 `WithType(...)` 또는 `SetType(...)` 로 분류 식별을 설정한다.**
2. **`metadata.message_type` 키는 production 코드 어디에서도 set/read 하지 않는다.**
3. **다운스트림 분류 로직은 `msg.Type()` 단일 호출로 수행한다.**
4. **JSON 출력의 canonical 분류 채널은 top-level `"type"` 필드이다.**
5. **Lua 스크립트 컨텍스트는 `msg.type` 키를 노출한다** (`msg.metadata.message_type` 컨벤션은 제거).
6. **빈 `Type()` 의 JSON 직렬화 정책은 M6 에서 결정한다** (omitempty 채택 또는 명시적 빈 값 노출).

### 3.3 Soft Deprecation 미적용 사유

- 본 시스템은 **단일 운영자 (greenfield) 환경** — 외부 클라이언트, MQTT 구독자, 외부 API 사용자 부재.
- 호환 alias 또는 Deprecation 기간을 두면 정작 cleanup 비용만 증가.
- 단일 PR 에서 모든 사이트 일괄 전환 — 한 번에 일관 상태 도달.

---

## 4. 환경 (Environment)

### 4.1 영향 받는 영역 (상세)

| 영역 | 변경 항목 | 위험도 |
|------|----------|--------|
| `internal/node/trigger.go` | line 482 `WithMetadata("message_type", "event")` → `WithType("event")` | 낮음 |
| `internal/node/inventory.go` | 신규 `WithType("inventory.event")` 추가 또는 inherit 전략 결정 | 낮음 |
| `internal/agent/lg/lg_hvacr01_agent.go` | `emitDeviceStateLocked` 시리즈의 metadata 설정 → `SetType("device_state.<trigger>")` | 중간 |
| `internal/agent/lg/lg_hvacr02_agent.go` | `emitDeviceStateLocked` 시리즈 동일 | 중간 |
| `internal/agent/lg/agent.go` (LGAP) | `emitDeviceStateLocked` 시리즈 동일 | 중간 |
| `internal/agent/samsung/agent.go` (NASA) | emit 시리즈 동일 | 중간 |
| `internal/agent/century/message.go` | emit 시리즈 동일 | 중간 |
| `internal/node/debug.go` | message_type 설정 + 분류 표시 → `Type()` 기반 | 낮음 |
| `internal/node/bridge.go` | message_type 설정 → `SetType` 기반 | 낮음 |
| `internal/node/modbus_poller.go` (test) | message_type 검증 → `Type()` 검증 | 낮음 |
| `internal/node/nasa.go`, `lg_hvacr01.go`, `lg_hvacr02.go`, `lgap.go`, `century.go` | metadata 읽는 분류 분기 → `Type()` 호출 | 중간 |
| `internal/node/transform.go` / `transform_test.go` | metadata.message_type lookup → `msg.Type()` | 낮음 |
| `internal/node/script.go` (Lua) | Lua 컨텍스트의 `msg.type` 키 노출 (M4) | 중간 |
| `internal/node/dedup_helper.go` | metadata 키 사용 → `Type()` | 낮음 |
| `pkg/message/message.go` | JSON `"type"` omitempty 정책 결정 (M6) | 낮음 |
| `web/src/` | message_type 사용처 검토 (script 노드 docs, debug 표시 등) | 낮음 |
| 테스트 전체 | metadata.message_type 검증 → `Type()` 검증 | 중간 |

### 4.2 가정 (Assumptions)

- **A1**: 본 SPEC 의 운영 환경은 greenfield (단일 frontend 통제, 외부 클라이언트 부재, 외부 metadata.message_type 의존성 부재) 이다.
- **A2**: 기존 영속 데이터 또는 메시지 큐 (예: MQTT broker 의 inflight messages) 에 `metadata.message_type` 가 직렬화된 채로 잔존하지 않거나, 잔존하더라도 새 코드가 graceful 하게 처리할 수 있다.
- **A3**: Lua 스크립트가 `msg.metadata.message_type` 를 참조하는 사용자 정의 스크립트가 없다 (있다면 마이그레이션 가이드 별도 안내 — § 4.5 참조).
- **A4**: `message.Message` 인터페이스의 `Type() / SetType() / WithType()` 메서드는 이미 모든 production 빌드에 존재한다 (`pkg/message/message.go:83, 165, 169` 검증 완료).
- **A5**: 단일 PR 에서 모든 사이트 일괄 전환이 가능하다 (영향 사이트 수 23~25 파일로 한 세션에 처리 가능 규모).

### 4.3 Constitution 정합성

- Go 1.25+ 기존 기술 스택 (신규 의존성 없음).
- `pkg/message` 인터페이스 추가 변경 없음 — `Type/SetType/WithType` 는 이미 존재.
- Breaking 변경: 외부 클라이언트가 metadata.message_type 에 의존한다면 깨지지만, A1 가정으로 비영향.
- TRUST 5: Tested (모든 emit 사이트 + 분류 분기 사이트 회귀 테스트 필수), Trackable (단일 SPEC 으로 추적).

---

## 5. EARS 요구사항 (EARS Requirements)

본 SPEC 은 6개 EARS 모듈로 구성된다.

### M1: 1급 type 필드 채택 (모든 emit 사이트)

- **Ubiquitous**: 모든 메시지 emit 사이트는 **항상** `message.WithType(...)` 또는 `msg.SetType(...)` 를 통해 분류 식별을 설정해야 한다.
- **Ubiquitous**: `Type()` 의 반환값은 다음 SCHEMA 식별자 형식을 따라야 한다 (기존 `metadata.message_type` 값 컨벤션 그대로 이전):
  - `"event"` (trigger emit)
  - `"device_state.change"` / `"device_state.poll"` / `"device_state.response"` (HVAC 5 에이전트 emit)
  - `"inventory.event"` (inventory 노드 emit)
  - `"bridge.message"` (bridge 노드 emit)
  - `"debug.log"` (debug 노드 emit, 필요 시)
  - 기타 도메인별 식별자는 SPEC 갱신 시 추가.
- **State-driven**: IF emit 사이트가 명시적 type 설정 없이 메시지를 생성하면, THEN `Type()` 는 빈 문자열을 반환한다 (graceful, 분류 부재 의미).
- **Unwanted**: WHEN emit 사이트에서 `WithMetadata("message_type", ...)` 호출이 발견되면, THEN 빌드는 통과되지만 grep 기반 CI 검증에서 실패해야 한다 (M2 와 연계).
- **Ubiquitous**: HVAC 5 에이전트 (LG HVACR-01/LGAP/LG HVACR-02/Samsung/Century) 의 `emitDeviceStateLocked` 시리즈는 **항상** 1급 `Type()` 로 분류 식별을 설정해야 한다.

### M2: metadata.message_type 키 제거

- **Ubiquitous**: production 코드 (`internal/`, `pkg/`) 에서 `metadata.message_type` 키의 **set 호출은 항상 0건** 이어야 한다.
  - 검증 명령: `grep -rn 'WithMetadata("message_type"\|WithMetadata(\`message_type\`' internal/ pkg/` 결과 empty.
- **Ubiquitous**: production 코드에서 `metadata.message_type` 키의 **read 호출은 항상 0건** 이어야 한다.
  - 검증 명령: `grep -rn 'Metadata().*"message_type"\|Get("message_type")\|\[\"message_type\"\]' internal/ pkg/` 결과 empty.
- **Ubiquitous**: 테스트 코드도 동일 — `metadata.message_type` 키 검증을 `msg.Type()` 검증으로 일괄 갱신해야 한다.
- **Unwanted**: WHEN 본 SPEC 적용 후 `metadata.message_type` 키 사용이 새로 추가되면, THEN pre-commit 또는 CI 의 grep 검증이 실패해야 한다.

### M3: 다운스트림 정합 (transform / filter / debug / 분기 로직)

- **Ubiquitous**: 모든 다운스트림 분류 로직 (transform, filter, condition, expression, debug, bridge, dedup, script) 은 **항상** `msg.Type()` 단일 호출로 분류 식별을 수행해야 한다.
- **Ubiquitous**: 다운스트림 코드에서 `metadata.message_type` 키의 lookup 분기는 **모두 제거** 되어야 한다.
- **State-driven**: IF `msg.Type()` 이 빈 문자열인 메시지가 다운스트림에 도달하면, THEN 분류 분기는 명시적 "unclassified" 경로로 처리한다 (또는 기본 분기로 fall-through). 기존 metadata 키 fallback 동작은 부재해야 한다.
- **Event-driven**: WHEN debug 노드가 메시지를 표시할 때, THEN `Type()` 값을 분류 라벨로 표시한다 (metadata.message_type 표시 분기 제거).
- **Ubiquitous**: transform 노드의 분기 조건이 metadata.message_type 에 의존하던 부분은 `msg.Type()` 기반으로 갱신되어야 한다 (예: `transform_test.go` 의 케이스 갱신).

### M4: Lua 스크립트 호환 (script 노드 컨텍스트)

- **Ubiquitous**: script 노드의 Lua 컨텍스트는 **항상** input 메시지의 `Type()` 값을 `msg.type` 키로 노출해야 한다.
- **Ubiquitous**: script 노드의 Lua 컨텍스트는 `msg.metadata.message_type` 키 노출을 **중단** 해야 한다 (Lua 스크립트가 metadata 전체를 받더라도, message_type 은 metadata 외부의 1급 키로 노출).
- **State-driven**: IF Lua 스크립트가 `msg.type = "..."` 로 분류를 설정하면, THEN bridge 코드는 이를 `SetType(...)` 호출로 변환해야 한다 (out path).
- **Optional**: WHERE 운영자가 기존 Lua 스크립트의 `msg.metadata.message_type` 참조를 사용 중이라면, 마이그레이션 가이드 (`docs/migration/message-type.md`) 를 제공하여 `msg.type` 으로 갱신 안내. 호환 alias 는 제공하지 않는다 (greenfield 가정).
- **Unwanted**: WHEN Lua 스크립트가 `msg.metadata.message_type` 를 읽으려 하면, THEN 해당 키는 nil 또는 부재하며, 스크립트는 명시적 에러 없이 nil 처리한다 (Lua 런타임 기본 동작).

### M5: 테스트 갱신

- **Ubiquitous**: 모든 emit 사이트의 단위 테스트는 **항상** `msg.Type()` 검증 어서션을 포함해야 한다.
- **Ubiquitous**: 기존 `metadata.message_type` 검증 어서션은 모두 `msg.Type()` 검증으로 일괄 치환되어야 한다.
- **Ubiquitous**: 다운스트림 분류 분기 테스트 (transform / filter / debug 등) 는 `WithType(...)` 로 setup 한 후 `Type()` 기반 분기를 검증해야 한다.
- **Event-driven**: WHEN 테스트 setup 에서 메시지가 type 설정 없이 생성되면, THEN 테스트는 `Type() == ""` 동작을 명시적으로 검증해야 한다 (regression 방지).
- **Ubiquitous**: 영향 받는 테스트 파일 (`century/message_test.go`, `century/agent_device_state_test.go`, `transform_test.go`, `lg_hvacr02_test.go`, `mqtt_test.go`, `nasa_test.go`, `modbus_poller_test.go`, `dedup_helper_test.go` 등) 의 metadata.message_type 어서션은 0건이어야 한다.

### M6: JSON serialization 정책 결정

- **Ubiquitous**: 본 SPEC 은 top-level `"type"` 필드의 JSON 직렬화 정책을 다음 두 옵션 중 하나로 확정해야 한다:
  - **옵션 A (omitempty)**: `Type()` 이 빈 문자열일 때 JSON 출력에서 `"type"` 키를 생략. 출력이 깔끔하지만, downstream parser 가 키 존재 여부로 분류 부재를 판단해야 함.
  - **옵션 B (always emit)**: `Type()` 이 빈 문자열이어도 `"type": ""` 으로 노출. 일관된 schema 이지만 빈 값 노이즈.
- **결정 (본 SPEC 의 권장)**: **옵션 A (omitempty)** — greenfield 환경에서 downstream parser 통제 가능, JSON 출력 깔끔. struct tag 변경 시 `json:"type,omitempty"` 적용.
- **Ubiquitous**: 결정된 정책에 따라 `pkg/message/message.go` 의 JSON 직렬화 구현이 갱신되어야 한다.
- **Event-driven**: WHEN downstream consumer (debug 노드, script 노드, frontend) 가 `"type"` 키 부재 메시지를 받으면, THEN 명시적 "unclassified" 처리 또는 기본 분류로 fall-through 한다.
- **Ubiquitous**: 직렬화 정책 변경 후 frontend (`web/src/`) 의 메시지 표시 로직은 `type` 키 부재를 graceful 하게 처리해야 한다 (e.g., 기본 라벨 "Unclassified" 표시).

---

## 6. 명세 (Specifications)

### 6.1 message.Type() / SetType() / WithType() — 현재 인터페이스 (변경 없음)

```go
// pkg/message/message.go (현재 정의, 본 SPEC 에서 인터페이스 변경 없음)

// WithType는 type 필드를 설정하는 Option 함수.
func WithType(t string) Option {
    return func(m *defaultMessage) {
        m.typ = t
    }
}

// Type returns the SCHEMA classification identifier.
func (m *defaultMessage) Type() string {
    return m.typ
}

// SetType updates the SCHEMA classification identifier.
func (m *defaultMessage) SetType(t string) {
    m.typ = t
}
```

### 6.2 SCHEMA 식별자 컨벤션 (1급 Type() 반환값)

| 도메인 | 식별자 형식 | 사용 사이트 |
|---|---|---|
| Event trigger | `"event"` | `internal/node/trigger.go` |
| HVAC device state | `"device_state.change"` | LG HVACR-01/LG HVACR-02/LGAP/Samsung/Century `emitDeviceStateLocked` (변경 시) |
| HVAC device state | `"device_state.poll"` | HVAC 5 (poll 응답) |
| HVAC device state | `"device_state.response"` | HVAC 5 (커맨드 응답) |
| Inventory | `"inventory.event"` | `internal/node/inventory.go` |
| Bridge | `"bridge.message"` | `internal/node/bridge.go` (필요 시) |
| Debug | `"debug.log"` | `internal/node/debug.go` (필요 시) |
| Modbus poll | `"modbus.poll"` | `internal/node/modbus_poller.go` (필요 시) |
| (기타) | 도메인별 SPEC 에서 정의 | - |

> 식별자 컨벤션은 기존 `metadata.message_type` 값을 그대로 1급으로 이전한다. 신규 식별자 추가는 본 SPEC 의 갱신 또는 도메인 SPEC 으로 관리.

### 6.3 emit 패턴 진화 (Before / After)

**Before (trigger 노드, 현재):**

```go
// internal/node/trigger.go:482
msg := message.New(
    message.WithPayload(payload),
    message.WithMetadata("message_type", "event"),     // ❌ metadata 채널
    message.WithMetadata("trigger_id", t.id),
)
```

**After (trigger 노드, SPEC 적용 후):**

```go
msg := message.New(
    message.WithPayload(payload),
    message.WithType("event"),                          // ✅ 1급 채널
    message.WithMetadata("trigger_id", t.id),
)
```

**Before (HVAC LG HVACR-01 emitDeviceStateLocked, 현재):**

```go
msg := message.New(
    message.WithPayload(state),
    message.WithMetadata("message_type", "device_state.change"),  // ❌
    message.WithMetadata("agent", "lg_hvacr01"),
    ...
)
```

**After:**

```go
msg := message.New(
    message.WithPayload(state),
    message.WithType("device_state.change"),                       // ✅
    message.WithMetadata("agent", "lg_hvacr01"),
    ...
)
```

### 6.4 다운스트림 분기 진화 (Before / After)

**Before (transform 노드, metadata 분기):**

```go
mt, _ := msg.Metadata().Get("message_type").(string)  // ❌ map lookup + 타입 단언
switch mt {
case "event":
    ...
case "device_state.change":
    ...
}
```

**After:**

```go
switch msg.Type() {                                    // ✅ 1급 메서드
case "event":
    ...
case "device_state.change":
    ...
}
```

### 6.5 JSON serialization 진화 (M6 옵션 A 채택 시)

**Before (현재 동작, 빈 값 노출):**

```json
{
  "type": "",
  "payload": {...},
  "metadata": {
    "message_type": "event",
    "trigger_id": "t-001"
  }
}
```

**After (옵션 A omitempty 채택 시):**

```json
{
  "type": "event",
  "payload": {...},
  "metadata": {
    "trigger_id": "t-001"
  }
}
```

**After — type 미설정 케이스 (옵션 A):**

```json
{
  "payload": {...},
  "metadata": {
    "trigger_id": "t-001"
  }
  // "type" 키 부재
}
```

### 6.6 Lua 스크립트 컨텍스트 진화 (M4)

**Before:**

```lua
-- script 노드의 Lua 컨텍스트
local mt = msg.metadata.message_type   -- ❌
if mt == "event" then
    ...
end
```

**After:**

```lua
local mt = msg.type                     -- ✅
if mt == "event" then
    ...
end
```

### 6.7 에러 모델

본 SPEC 은 신규 에러 타입을 도입하지 않는다. 빈 `Type()` 은 graceful (분류 부재) 로 처리하며, downstream 의 unclassified 분기로 fall-through 한다.

---

## 7. 적용 전략 (단일 PR 직접 cut)

### 7.1 전략 개요

- **단일 PR 일괄 전환**: 모든 emit 사이트 + 다운스트림 + 테스트 + Lua bridge 동시 갱신.
- **호환 alias 미적용**: greenfield 가정 (A1) 으로 외부 호환 불요.
- **단계 분리는 plan.md** 에서 작업 분해 (사이트별 우선순위 + 단위) 로만 수행.

### 7.2 위험 완화

| 위험 | 완화 |
|------|------|
| metadata.message_type 잔존 (production) | grep 기반 CI 검증 (M2) |
| 다운스트림 분기 누락 | 영향 사이트 grep 전수 조사 + 단위 테스트 |
| Lua 스크립트 호환 | 마이그레이션 가이드 (M4) 별도 안내, 호환 alias 미제공 |
| 영속 메시지 (MQTT broker 등) | A2 가정으로 비영향, 잔존 시 graceful |
| JSON schema 변경 | M6 옵션 A 채택 시 frontend `web/src/` 갱신 동시 진행 |

### 7.3 롤백 전략

- 단일 PR 이므로 git revert 로 즉시 롤백 가능.
- 영속 데이터 변경 없음 (인터페이스 + 직렬화 정책만 변경).
- 외부 호환성 영향 없음 (A1 가정).

---

## 8. 관련 SPEC (Related SPECs)

- **SPEC-MSG-001** (참고): Message 인터페이스 기본 정의. 본 SPEC 의 `Type/SetType/WithType` 메서드 기반.
- **SPEC-INVENTORY-001** (참고): inventory 노드 emit. 본 SPEC 적용 시 `WithType("inventory.event")` 추가 필요.
- **SPEC-BRIDGE-001** (참고): bridge 노드의 분류 사용. metadata.message_type 분기를 `Type()` 으로 갱신.
- **SPEC-DEBUG-001** (참고): debug 노드의 분류 표시. metadata 표시 분기 제거.
- **SPEC-SCRIPT-001** (참고): script 노드 Lua 컨텍스트. `msg.type` 키 노출, `msg.metadata.message_type` 키 제거.

---

## 9. TAG Traceability

- @SPEC:SPEC-MESSAGE-TYPE-001 → spec.md (이 문서)
- @PLAN:SPEC-MESSAGE-TYPE-001 → plan.md
- @ACCEPTANCE:SPEC-MESSAGE-TYPE-001 → acceptance.md
- 구현 경로:
  - **emit 사이트 (Production)**:
    - `internal/node/trigger.go` (line 482)
    - `internal/node/inventory.go`
    - `internal/agent/lg/lg_hvacr01_agent.go`
    - `internal/agent/lg/lg_hvacr02_agent.go`
    - `internal/agent/lg/agent.go` (LGAP)
    - `internal/agent/samsung/agent.go` (NASA)
    - `internal/agent/century/message.go`
    - `internal/node/debug.go`
    - `internal/node/bridge.go`
    - `internal/node/modbus_poller.go` (필요 시)
  - **다운스트림 (Production)**:
    - `internal/node/transform.go`
    - `internal/node/nasa.go`, `lg_hvacr01.go`, `lg_hvacr02.go`, `lgap.go`, `century.go` (분기 로직)
    - `internal/node/script.go` (Lua bridge)
    - `internal/node/dedup_helper.go`
  - **직렬화 (M6)**:
    - `pkg/message/message.go` (struct tag omitempty)
  - **테스트**:
    - `internal/agent/century/message_test.go`
    - `internal/agent/century/agent_device_state_test.go`
    - `internal/node/transform_test.go`
    - `internal/node/lg_hvacr02_test.go`
    - `internal/node/mqtt_test.go`
    - `internal/node/nasa_test.go`
    - `internal/node/modbus_poller_test.go`
    - `internal/node/dedup_helper_test.go`
  - **Frontend (M6 옵션 A 채택 시)**:
    - `web/src/` 의 message_type 표시 사용처 검토 (script 노드 docs, debug 표시)

---

## 10. Implementation Notes

### 10.1 단일 PR 전환의 합리성

- 영향 사이트가 25 파일 내외로 한 세션에 일괄 처리 가능한 규모.
- 호환 기간을 두면 정작 cleanup 비용이 증가 (V1/V2 wrapper, deprecation 메트릭 등).
- greenfield 단일 운영자 환경 — 외부 호환성 리스크 없음.
- git revert 로 단순 롤백.

### 10.2 1급 채널 채택의 장기적 가치

- 신규 노드 추가 시 컨벤션 모호성 제거 — "type 은 1급 메서드" 가 명백.
- type-safety + visibility — `msg.Type()` vs `msg.Metadata().Get("message_type")` 의 가독성 차이.
- JSON schema 안정성 — top-level `"type"` 이 canonical, downstream parser simplification.
- metadata map 은 도메인별 보조 데이터 (trigger_id, agent, location 등) 에 집중.

### 10.3 본 SPEC 의 범위 외 (Out of Scope)

- **새로운 SCHEMA 식별자 컨벤션 정의** — 본 SPEC 은 기존 값을 이전만. 신규 식별자는 도메인 SPEC 에서.
- **metadata map 의 다른 컨벤션 통일** — 본 SPEC 은 `message_type` 키만 폐기. 다른 metadata 키 (`agent`, `trigger_id` 등) 는 무관.
- **Message 인터페이스의 다른 메서드 변경** — `Type/SetType/WithType` 외에는 변경 없음.
- **외부 통합 (MQTT bridge, 외부 API) 의 message_type 노출** — 별도 SPEC 으로 분리 가능.

### 10.4 후속 작업 가능성

- M6 옵션 A 채택 후 downstream consumer 안정화 검증.
- SCHEMA 식별자 enum 화 (string 대신 typed constant) — 별도 SPEC 으로.
- Lua 스크립트의 type 기반 dispatch 헬퍼 (`script.OnType("event", fn)`) — 별도 SPEC 으로.
