# 마이그레이션 가이드 — message.type 1급 채널 단일화

xflowd v1.x (SPEC-MESSAGE-TYPE-001 적용) 부터 메시지 분류 식별 채널은
`message.Type()` 1급 인터페이스로 단일화되었다. 이전의 `metadata.message_type`
키 컨벤션은 **완전 폐기** 되었다.

본 문서는 운영자가 Lua 스크립트 / 외부 통합 / 도구를 갱신할 때 참고할
체크리스트와 변환 패턴을 제공한다.

---

## 1. 변경 요약

| 항목 | 이전 (v0.x) | 현재 (v1.x) |
|------|------------|------------|
| Go 코드 emit | `WithMetadata("message_type", "X")` | `WithType("X")` 또는 `msg.SetType("X")` |
| Go 코드 read | `msg.Metadata().Get("message_type")` | `msg.Type()` |
| JSON top-level | `"type": ""` (빈 값 노출) + `"metadata": { "message_type": "X" }` | `"type": "X"` (값 있을 때만) |
| Lua 컨텍스트 | `msg.metadata.message_type` | `msg.type` |

호환 alias 는 제공하지 않는다 (greenfield 단일 운영자 환경). 모든 사이트는
1급 `type` 채널로 일괄 갱신되었다.

---

## 2. Lua 스크립트 마이그레이션

### 2.1 분류 식별 읽기

이전 (v0.x):

```lua
local mt = msg.metadata.message_type
if mt == "event" then
    msg.payload.classified = true
end
```

현재 (v1.x):

```lua
local mt = msg.type
if mt == "event" then
    msg.payload.classified = true
end
```

### 2.2 분류 식별 쓰기

이전 (v0.x):

```lua
msg.metadata.message_type = "custom.event"
return msg
```

현재 (v1.x):

```lua
msg.type = "custom.event"
return msg
```

### 2.3 빈 type 의 graceful 처리

`msg.type` 키는 항상 존재하지만 빈 문자열(`""`) 일 수 있다. Lua 의
truthy 검사로 graceful 하게 처리한다.

```lua
if msg.type and msg.type ~= "" then
    -- 분류 식별이 설정된 경우
    handle_classified(msg)
else
    -- 분류 부재 — 기본 경로
    handle_unclassified(msg)
end
```

### 2.4 metadata 의 다른 키는 보존

본 SPEC 은 `message_type` 키만 폐기한다. `agent`, `trigger_id`, `location`,
`unit_id`, `device_id` 등 다른 metadata 키들은 그대로 보존된다.

```lua
local agent = msg.metadata.agent           -- 보존
local trigger_id = msg.metadata.trigger_id -- 보존
local mt = msg.type                         -- 1급 채널
```

---

## 3. JSON 출력 형태 변경

### 3.1 분류 식별이 설정된 메시지

이전 (v0.x):

```json
{
  "id": "uuid-...",
  "type": "",
  "payload": { "...": "..." },
  "metadata": {
    "message_type": "event",
    "trigger_id": "t-001"
  }
}
```

현재 (v1.x):

```json
{
  "id": "uuid-...",
  "type": "event",
  "payload": { "...": "..." },
  "metadata": {
    "trigger_id": "t-001"
  }
}
```

### 3.2 분류 식별이 없는 메시지 (omitempty)

```json
{
  "id": "uuid-...",
  "payload": { "...": "..." },
  "metadata": {
    "trigger_id": "t-001"
  }
}
```

`type` 키가 부재한다는 점이 v0.x 의 `"type": ""` 노이즈와 다르다.
downstream parser 는 키 존재 여부로 분류 부재를 판단한다.

---

## 4. 외부 통합 갱신 체크리스트

| 영역 | 작업 |
|------|------|
| Lua 스크립트 (`script` 노드) | `msg.metadata.message_type` → `msg.type` 일괄 치환 |
| MQTT 구독자 | JSON 파싱 시 top-level `type` 키 사용 (`metadata.message_type` 파싱 코드 제거) |
| 외부 로깅 / SIEM | 분류 라벨 추출 경로를 top-level `type` 로 변경 |
| 모니터링 대시보드 | 메시지 type 표시 / 필터 / 분기 로직 갱신 |
| 통합 테스트 | `metadata.message_type` 어서션 → `msg.Type()` / JSON top-level `type` 검증 |

---

## 5. 식별자 컨벤션 (v1.x 시점)

| 도메인 | type 값 | 발행 사이트 |
|--------|---------|------------|
| Trigger event | `"event"` | `internal/node/trigger.go` |
| HVAC device state (변경) | `"device_state.change"` | LG HVACR-01/LGCP/LGAP/Samsung/Century 노드 |
| HVAC device state (폴) | `"device_state.poll"` | 동일 |
| HVAC device state (응답) | `"device_state.response"` | 동일 |
| Inventory snapshot | `"inventory.event"` | `internal/node/inventory.go` |
| Bridge agent push | `"event"` (기본) | `internal/node/bridge.go` (어댑터 설정값 없으면) |
| Modbus poll | `"event"` 또는 `"response"` | `internal/node/modbus.go`, `modbus_poller.go` |
| InfluxDB query | `"response"` | `internal/node/influxdb_query.go`, `tsdb_query.go` |

새 도메인 식별자는 별도 SPEC 갱신으로 추가한다.

---

## 6. 회귀 검증

운영자가 마이그레이션 후 자체 통합을 검증할 때:

1. **JSON 출력 확인**: emit 사이트의 메시지 JSON 출력에 top-level
   `"type": "<expected>"` 노출 및 `metadata.message_type` 키 부재.
2. **Lua 스크립트 동작**: `msg.type` 으로 분류 분기 정상 동작, `msg.metadata.message_type`
   가 nil 임을 확인.
3. **빈 type 처리**: 분류 미설정 메시지가 downstream 에서 graceful 하게
   처리되는지 확인.

---

## 7. 호환성

호환 alias 는 제공하지 않는다. 본 변경은 단일 PR 직접 cut 으로 적용되었으며,
v1.0 부터 강제된다. 외부 클라이언트가 있는 경우 위 체크리스트로 동시 갱신
필요.

---

## 8. 관련 SPEC

- @SPEC:SPEC-MESSAGE-TYPE-001 — 본 변경의 정의 문서
- @PLAN:SPEC-MESSAGE-TYPE-001 — 작업 분해
- @ACCEPTANCE:SPEC-MESSAGE-TYPE-001 — 인수 기준
- `pkg/message/message.go` — `Type() / SetType() / WithType()` 인터페이스

---

Version: 1.0.0
Last Updated: 2026-05-26
