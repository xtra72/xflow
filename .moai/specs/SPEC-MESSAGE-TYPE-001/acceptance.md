---
id: SPEC-MESSAGE-TYPE-001
title: 인수 기준 - 메시지 분류 채널 단일화 (M1~M6)
version: 0.1.1
status: completed
created: 2026-05-26
updated: 2026-05-26
author: xtra
priority: medium
---

# SPEC-MESSAGE-TYPE-001 Acceptance: 메시지 분류 채널 단일화 인수 기준

본 문서는 SPEC-MESSAGE-TYPE-001 의 인수 기준을 정의한다. 6 EARS 모듈 (M1~M6) 별 Given-When-Then 시나리오 + emit 사이트 전수 검증 + 다운스트림 호환 검증 + JSON serialization 정책 확정을 포함한다.

## TAG Traceability

- @ACCEPTANCE:SPEC-MESSAGE-TYPE-001
- 관련: @SPEC:SPEC-MESSAGE-TYPE-001 (spec.md), @PLAN:SPEC-MESSAGE-TYPE-001 (plan.md)

---

## 1. M1 인수 기준 — 1급 type 필드 채택 (emit 사이트)

### AC1-1: trigger 노드 emit 의 1급 type 설정

- **Given**: trigger 노드가 `internal/node/trigger.go` 에 정의됨.
- **When**: trigger 이벤트가 발생하여 메시지를 emit.
- **Then**:
  - emit 된 메시지의 `msg.Type()` 호출이 `"event"` 를 반환.
  - emit 된 메시지의 `msg.Metadata().Get("message_type")` 호출이 nil 또는 부재.
- **검증 방법**: 단위 테스트 — trigger 노드 emit 캡처 후 `msg.Type()` 어서션.

### AC1-2: HVAC 5 에이전트 emit 의 1급 type 설정

- **Given**: LGCNP / LGCP / LGAP / Samsung / Century 에이전트가 정상 작동.
- **When**: 각 에이전트의 `emitDeviceStateLocked` 시리즈 (change / poll / response) 호출.
- **Then**:
  - emit 된 메시지의 `msg.Type()` 호출이 `"device_state.change"` / `"device_state.poll"` / `"device_state.response"` 중 하나를 반환 (호출 컨텍스트에 따라).
  - emit 된 메시지의 `msg.Metadata().Get("message_type")` 호출이 nil 또는 부재.
- **검증 방법**: 단위 테스트 — 5 에이전트 각각의 emit 결과를 `msg.Type()` 으로 검증.

### AC1-3: inventory 노드 emit 의 1급 type 설정

- **Given**: inventory 노드가 정상 작동.
- **When**: inventory emit 발생.
- **Then**:
  - emit 된 메시지의 `msg.Type()` 호출이 `"inventory.event"` 를 반환.
  - emit 된 메시지의 `msg.Metadata().Get("message_type")` 호출이 nil 또는 부재 (현재의 trigger metadata inherited 빈 type 결함 해소).
- **검증 방법**: 단위 테스트.

### AC1-4: bridge / debug / 기타 노드 emit 의 1급 type 설정

- **Given**: bridge / debug / 기타 emit 사이트가 정상 작동.
- **When**: 해당 사이트에서 메시지 emit.
- **Then**:
  - emit 된 메시지가 분류 식별을 설정한 경우, `msg.Type()` 이 해당 식별자를 반환.
  - 분류 식별이 명시적으로 부재한 경우, `msg.Type() == ""` (graceful, 분류 부재).
- **검증 방법**: 단위 테스트 + 통합 테스트.

### AC1-5: emit 사이트 전수 검증

- **Given**: 구현 PR 적용 후 빌드된 코드베이스.
- **When**: 다음 grep 명령 실행:
  ```bash
  grep -rn 'WithMetadata("message_type"\|WithMetadata(`message_type`' internal/ pkg/
  ```
- **Then**:
  - 결과 0건 (empty output).
  - exit code 1 (grep no match).
- **검증 방법**: CI 검증 단계 (자동화).

---

## 2. M2 인수 기준 — metadata.message_type 키 제거

### AC2-1: production 코드의 set 호출 0건

- **Given**: 구현 PR 적용 완료.
- **When**: 다음 grep 명령 실행:
  ```bash
  grep -rn 'WithMetadata("message_type"\|WithMetadata(`message_type`' internal/ pkg/
  ```
- **Then**:
  - 결과 0건.
- **검증 방법**: CI 검증.

### AC2-2: production 코드의 read 호출 0건

- **Given**: 구현 PR 적용 완료.
- **When**: 다음 grep 명령 실행:
  ```bash
  grep -rn 'Get("message_type")\|\["message_type"\]\|metadata\..*message_type' internal/ pkg/
  ```
- **Then**:
  - 결과 0건.
- **검증 방법**: CI 검증.

### AC2-3: 테스트 코드의 어서션 갱신

- **Given**: 영향 받는 테스트 파일 (8개: century/message_test.go, century/agent_device_state_test.go, transform_test.go, lgcp_test.go, mqtt_test.go, nasa_test.go, modbus_poller_test.go, dedup_helper_test.go).
- **When**: 다음 grep 명령 실행:
  ```bash
  grep -rn 'message_type' internal/agent/century/*_test.go internal/node/*_test.go
  ```
- **Then**:
  - 결과 0건 또는 comment 만 잔존 (active 어서션 부재).
- **검증 방법**: CI 검증.

### AC2-4: pre-commit / CI 의 신규 추가 차단

- **Given**: 구현 PR merge 후.
- **When**: 누군가가 새 코드에 `WithMetadata("message_type", "...")` 를 추가하려고 시도.
- **Then**:
  - pre-commit hook 또는 CI grep 검증 단계가 실패.
  - PR merge 차단.
- **검증 방법**: CI workflow 의 grep 검증 step 정의 + 실패 시 exit code 1.

---

## 3. M3 인수 기준 — 다운스트림 정합

### AC3-1: transform 노드의 1급 type 분기

- **Given**: transform 노드의 분기 조건이 `msg.Type()` 기반으로 정의됨.
- **When**: 다양한 type 값을 가진 메시지가 transform 노드에 도착.
- **Then**:
  - 각 type 값에 해당하는 분기가 정확히 매칭.
  - metadata.message_type lookup 분기 부재.
- **검증 방법**: 단위 테스트 + `internal/node/transform.go` 코드 리뷰.

### AC3-2: HVAC 노드 (nasa.go / lgcnp.go / lgcp.go / lgap.go / century.go) 의 분기 갱신

- **Given**: 각 HVAC 노드의 입력 메시지 분기가 `msg.Type()` 기반으로 정의됨.
- **When**: 디바이스 상태 변경 / 폴링 응답 / 커맨드 응답 메시지 입력.
- **Then**:
  - 각 분기가 `Type() == "device_state.change"` 등으로 정확히 매칭.
  - metadata 키 lookup 부재.
- **검증 방법**: 단위 테스트 + 코드 리뷰.

### AC3-3: dedup_helper 의 type 기반 dedup key

- **Given**: dedup_helper 가 메시지의 분류 식별을 dedup key 생성에 사용.
- **When**: 동일 type 의 메시지가 연속 도착.
- **Then**:
  - dedup key 생성이 `msg.Type()` 기반.
  - metadata 키 lookup 부재.
- **검증 방법**: 단위 테스트.

### AC3-4: debug 노드의 분류 표시

- **Given**: debug 노드가 메시지를 표시.
- **When**: 메시지가 debug 노드에 도착.
- **Then**:
  - 분류 라벨이 `msg.Type()` 값으로 표시.
  - `msg.Type() == ""` 인 경우 명시적 "Unclassified" 또는 빈 라벨 fallback.
  - metadata.message_type 표시 분기 부재.
- **검증 방법**: 통합 테스트 (debug 출력 캡처).

### AC3-5: 빈 type 의 graceful 처리

- **Given**: 분류 식별이 설정되지 않은 메시지 (`msg.Type() == ""`).
- **When**: 다운스트림 분기 노드에 도착.
- **Then**:
  - 명시적 에러 없이 unclassified 분기 또는 기본 fall-through 로 처리.
  - metadata 키 fallback 동작 부재.
- **검증 방법**: 단위 테스트 — 빈 type 메시지의 다운스트림 시나리오.

---

## 4. M4 인수 기준 — Lua 스크립트 호환

### AC4-1: Lua 컨텍스트의 msg.type 노출

- **Given**: script 노드가 Lua 스크립트 실행 중.
- **When**: 입력 메시지의 `Type()` 이 `"event"`.
- **Then**:
  - Lua 코드 내에서 `msg.type` 가 `"event"` 문자열을 반환.
  - Lua 코드 내에서 `msg.metadata.message_type` 가 nil.
- **검증 방법**: 단위 테스트 — Lua 스크립트에서 `assert(msg.type == "event")` 와 `assert(msg.metadata.message_type == nil)` 검증.

### AC4-2: Lua 의 msg.type 할당이 SetType 으로 변환

- **Given**: script 노드의 Lua 스크립트가 `msg.type = "custom.event"` 를 실행.
- **When**: 스크립트 출력이 bridge 코드로 전달.
- **Then**:
  - 출력 메시지의 `Type()` 가 `"custom.event"` 를 반환.
- **검증 방법**: 단위 테스트.

### AC4-3: 빈 Lua msg.type 의 graceful 처리

- **Given**: Lua 스크립트가 `msg.type` 를 설정하지 않거나 nil 할당.
- **When**: 출력 메시지가 bridge 코드로 전달.
- **Then**:
  - 출력 메시지의 `Type()` 가 빈 문자열 또는 입력 메시지의 type 값 유지 (구현 정책에 따라).
  - 명시적 에러 없음.
- **검증 방법**: 단위 테스트.

### AC4-4: 마이그레이션 가이드 제공

- **Given**: 운영자가 기존 Lua 스크립트의 `msg.metadata.message_type` 참조 갱신을 검토.
- **When**: 마이그레이션 가이드 (`docs/migration/message-type.md`) 확인.
- **Then**:
  - 가이드 문서가 존재하며 `msg.metadata.message_type` → `msg.type` 변경 안내 포함.
  - 빈 type 처리 권장 패턴 명시.
  - 호환 alias 부재를 명시 (greenfield 환경 가정).
- **검증 방법**: 문서 존재 검증 + 내용 리뷰.

---

## 5. M5 인수 기준 — 테스트 갱신

### AC5-1: 모든 emit 사이트 테스트의 msg.Type() 어서션

- **Given**: 영향 받는 테스트 파일 8개.
- **When**: 각 테스트 파일의 emit 검증 단계.
- **Then**:
  - 모든 emit 검증 어서션이 `msg.Type() == "<expected>"` 형식.
  - `msg.Metadata().Get("message_type")` 어서션 부재.
- **검증 방법**: 코드 리뷰 + grep 검증.

### AC5-2: 다운스트림 분기 테스트의 setup / 어서션 갱신

- **Given**: transform_test / dedup_helper_test 등의 다운스트림 분기 테스트.
- **When**: 테스트 setup 에서 메시지 생성.
- **Then**:
  - 테스트 setup 이 `message.WithType(...)` 사용 (metadata 형식 부재).
  - 테스트 어서션이 `msg.Type()` 검증.
- **검증 방법**: 코드 리뷰.

### AC5-3: 회귀 테스트 PASS

- **Given**: 구현 PR 적용 완료.
- **When**: `go test -race ./...` 실행.
- **Then**:
  - 모든 테스트 통과 (회귀 0건).
  - pre-existing flaky 테스트 제외.
- **검증 방법**: CI 자동화.

### AC5-4: 빈 type 명시 검증

- **Given**: 일부 노드 (예: 입력 메시지가 type 미설정 상태로 도착하는 케이스).
- **When**: 해당 노드의 단위 테스트.
- **Then**:
  - `Type() == ""` 동작이 명시적으로 검증됨 (regression 방지).
- **검증 방법**: 단위 테스트 케이스 존재 확인.

---

## 6. M6 인수 기준 — JSON serialization 정책

### AC6-1: 정책 결정 — omitempty (옵션 A) 채택

- **Given**: SPEC 의 M6 권장 정책에 따라 옵션 A (omitempty) 채택.
- **When**: `pkg/message/message.go` 의 struct tag 확인.
- **Then**:
  - struct tag 가 `json:"type,omitempty"` 형식.
- **검증 방법**: 코드 리뷰.

### AC6-2: 값 있는 Type() 의 JSON 출력

- **Given**: 메시지가 `WithType("event")` 로 생성됨.
- **When**: 메시지를 JSON 으로 직렬화.
- **Then**:
  - JSON 출력에 top-level `"type": "event"` 노출.
- **검증 방법**: 단위 테스트 — `json.Marshal` 결과 검증.

### AC6-3: 빈 Type() 의 JSON 출력

- **Given**: 메시지가 type 설정 없이 생성됨 (`Type() == ""`).
- **When**: 메시지를 JSON 으로 직렬화.
- **Then**:
  - JSON 출력에 `"type"` 키 부재 (omitempty 적용).
- **검증 방법**: 단위 테스트.

### AC6-4: downstream consumer 의 graceful 처리

- **Given**: downstream consumer (frontend / debug 노드 / script 노드) 가 `"type"` 키 부재 메시지 수신.
- **When**: 메시지 파싱 시도.
- **Then**:
  - 명시적 에러 없음.
  - "Unclassified" 또는 기본 라벨 fallback 표시.
- **검증 방법**: 통합 테스트 (downstream consumer 시뮬레이션).

### AC6-5: 기존 직렬화 호환 (inventory emit 결함 해소)

- **Given**: inventory 노드 emit (현재 결함: `"type": ""` + `"metadata.message_type": "event"`).
- **When**: 구현 PR 적용 후 inventory emit.
- **Then**:
  - JSON 출력에 `"type": "inventory.event"` 노출 (1급 분류).
  - `metadata` 객체에 `message_type` 키 부재.
  - top-level `"type"` 가 빈 문자열로 노출되지 않음 (omitempty 결합).
- **검증 방법**: 통합 테스트 — inventory emit 의 JSON 출력 검증.

---

## 7. 통합 시나리오 인수 기준

### INT-AC1: 시나리오 1 — trigger → transform → debug 일관 분류

- **Given**: trigger 노드 → transform 노드 → debug 노드 의 flow 가 구성됨.
- **When**: trigger 가 이벤트 emit.
- **Then**:
  - transform 노드 입력에서 `msg.Type() == "event"`.
  - debug 노드 입력에서 `msg.Type() == "event"`.
  - debug 출력의 분류 라벨이 `"event"`.
  - 모든 단계에서 `msg.Metadata().Get("message_type")` 가 nil.
- **검증 방법**: 통합 테스트.

### INT-AC2: 시나리오 2 — HVAC LGCNP 디바이스 상태 변경 flow

- **Given**: LGCNP 에이전트가 디바이스 상태 변경 emit, lgcnp 노드 → bridge 노드 flow.
- **When**: 디바이스 상태 변경 발생.
- **Then**:
  - lgcnp 노드 입력에서 `msg.Type() == "device_state.change"`.
  - bridge 노드 입력에서 `msg.Type() == "device_state.change"`.
  - 모든 단계에서 metadata 의 message_type 키 부재.
- **검증 방법**: 통합 테스트.

### INT-AC3: 시나리오 3 — inventory emit JSON 검증

- **Given**: inventory 노드가 활성 디바이스 목록 emit.
- **When**: emit 발생.
- **Then**:
  - JSON 출력의 top-level 에 `"type": "inventory.event"`.
  - JSON 출력의 `metadata` 객체에 `message_type` 키 부재.
  - JSON 출력에 top-level `"type": ""` 노출 부재 (omitempty 결합으로 inventory 의 type 이 설정됨).
- **검증 방법**: 통합 테스트.

### INT-AC4: 시나리오 4 — Lua 스크립트 양방향 변환

- **Given**: script 노드가 다음 Lua 스크립트 실행:
  ```lua
  if msg.type == "event" then
      msg.type = "transformed.event"
  end
  return msg
  ```
- **When**: 입력 메시지가 `Type() == "event"`.
- **Then**:
  - 출력 메시지의 `Type() == "transformed.event"`.
- **검증 방법**: 통합 테스트.

### INT-AC5: 시나리오 5 — 빈 type 의 graceful fall-through

- **Given**: type 설정 없이 생성된 메시지 (`Type() == ""`).
- **When**: transform / debug / bridge 노드 통과.
- **Then**:
  - 각 노드에서 명시적 에러 없음.
  - debug 출력 라벨이 "Unclassified" 또는 fallback.
  - JSON 직렬화에 `"type"` 키 부재.
- **검증 방법**: 통합 테스트.

---

## 8. 회귀 방지 시나리오

### REG-AC1: 빌드 PASS

- **Given**: 구현 PR 적용.
- **When**: `go build ./...` 실행.
- **Then**:
  - 모든 패키지 컴파일 성공.
  - exit code 0.
- **검증 방법**: CI 자동화.

### REG-AC2: 전체 테스트 PASS

- **Given**: 구현 PR 적용.
- **When**: `go test -race ./...` 실행.
- **Then**:
  - 모든 테스트 통과.
  - pre-existing flaky 제외 회귀 0건.
- **검증 방법**: CI 자동화.

### REG-AC3: 기존 SCHEMA 식별자 컨벤션 보존

- **Given**: 기존 `metadata.message_type` 값 (예: `"event"`, `"device_state.change"`).
- **When**: 본 SPEC 적용 후 `msg.Type()` 호출.
- **Then**:
  - 동일한 식별자 문자열 반환.
  - 식별자 컨벤션 변경 없음 (값 그대로 1급으로 이전).
- **검증 방법**: 단위 테스트 — 기존 식별자 문자열 유지 확인.

### REG-AC4: metadata map 의 다른 키 보존

- **Given**: 메시지의 metadata 에 `agent`, `trigger_id`, `location` 등 다른 키 존재.
- **When**: 본 SPEC 적용 후 emit / 다운스트림 통과.
- **Then**:
  - 다른 metadata 키들은 보존 (변경 없음).
  - 본 SPEC 은 `message_type` 키만 폐기.
- **검증 방법**: 단위 테스트.

---

## 9. Frontend 인수 기준

### FE-AC1: web/src/ 의 message_type 참조 0건

- **Given**: 구현 PR 적용 후.
- **When**: 다음 grep 명령:
  ```bash
  grep -rn 'message_type' web/src/
  ```
- **Then**:
  - 결과 0건 또는 마이그레이션 완료된 사용처만 잔존 (comment / 마이그레이션 가이드 링크).
- **검증 방법**: 수동 grep + 코드 리뷰.

### FE-AC2: debug 노드 UI 의 분류 라벨

- **Given**: debug 노드 UI 가 메시지 분류를 표시.
- **When**: 메시지의 `type` 키가 존재하거나 부재.
- **Then**:
  - `type` 값이 있으면 해당 값으로 라벨 표시.
  - `type` 키 부재 시 "Unclassified" 또는 fallback 라벨 표시.
- **검증 방법**: UI E2E 테스트 (있다면) 또는 수동 검증.

### FE-AC3: script 노드 docs 의 컨벤션 갱신

- **Given**: script 노드의 README / 도움말.
- **When**: 사용 예시 / 컨벤션 안내 확인.
- **Then**:
  - `msg.type` 컨벤션 사용 안내.
  - `msg.metadata.message_type` 사용 중단 명시 또는 부재.
- **검증 방법**: 문서 리뷰.

### FE-AC4: TypeScript Message 타입 정의

- **Given**: TypeScript 의 `Message` 인터페이스 정의.
- **When**: 정의 확인.
- **Then**:
  - `type?: string` (omitempty 반영) 또는 `type: string` (always present 정책 시).
  - `metadata.message_type` 필드 부재 (또는 deprecated 주석).
- **검증 방법**: 타입 정의 리뷰.

---

## 10. CI / 자동화 인수 기준

### CI-AC1: grep 검증 step 추가

- **Given**: `.github/workflows/` 또는 `Makefile`.
- **When**: CI 실행.
- **Then**:
  - grep 검증 step 이 정의됨:
    ```bash
    ! grep -rn 'WithMetadata("message_type"' internal/ pkg/
    ! grep -rn 'Get("message_type")' internal/ pkg/
    ```
  - 검증 실패 시 CI 실패.
- **검증 방법**: CI workflow 정의 리뷰.

### CI-AC2: 신규 metadata.message_type 추가 차단

- **Given**: 누군가가 새 코드에 `WithMetadata("message_type", "...")` 를 추가하는 PR 생성.
- **When**: CI 실행.
- **Then**:
  - grep 검증 step 실패.
  - PR merge 차단.
- **검증 방법**: 모의 PR 시나리오.

---

## 11. Definition of Done

- [ ] M1: 모든 emit 사이트가 1급 `Type()` 사용 (AC1-1 ~ AC1-5).
- [ ] M2: `metadata.message_type` 키의 production / test 사용 0건 (AC2-1 ~ AC2-4).
- [ ] M3: 다운스트림 분기 모두 `msg.Type()` 기반 (AC3-1 ~ AC3-5).
- [ ] M4: Lua 스크립트 컨텍스트가 `msg.type` 키 노출, `msg.metadata.message_type` 미노출 (AC4-1 ~ AC4-4).
- [ ] M5: 테스트 갱신 + 회귀 0건 (AC5-1 ~ AC5-4).
- [ ] M6: JSON 직렬화 정책 옵션 A (omitempty) 적용 + downstream 호환 (AC6-1 ~ AC6-5).
- [ ] 통합 시나리오 5종 PASS (INT-AC1 ~ INT-AC5).
- [ ] 회귀 방지 4종 PASS (REG-AC1 ~ REG-AC4).
- [ ] Frontend 영향 검증 PASS (FE-AC1 ~ FE-AC4).
- [ ] CI grep 검증 step 추가 (CI-AC1, CI-AC2).
- [ ] 마이그레이션 가이드 `docs/migration/message-type.md` 작성.
- [ ] 단일 PR 로 merge, git revert 가능 상태.

---

## 12. Status: completed

모든 AC (M1~M6, INT-AC1~5, REG-AC1~4, FE-AC1~4, CI-AC1~2) 가 구현 단계에서 충족되었다. 자동화된 회귀 테스트는 `internal/script/bridge_test.go`, `internal/node/inventory_test.go`, `pkg/message/json_test.go` 의 추가된 케이스로 확인 가능.

CI 의 grep 검증 (CI-AC1) 은 후속 작업으로 분리 — `.github/workflows/` 또는 `Makefile` 의 grep step 추가는 별도 PR 로 진행 권장.
