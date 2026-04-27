---
id: SPEC-BRIDGE-002
document: acceptance
version: "1.0.0"
created: "2026-03-05"
updated: "2026-03-06"
---

# SPEC-BRIDGE-002 수락 기준: Agent-Specific Dedicated Bridge Implementation

## Module 1: BridgeAdapter 인터페이스 및 Registry

### AC-001: BridgeAdapter 인터페이스 컴파일 및 구현 검증

```gherkin
Given BridgeAdapter 인터페이스가 정의되어 있고
  And DefaultAdapter가 BridgeAdapter를 구현하고 있을 때
When DefaultAdapter를 BridgeAdapter 타입 변수에 할당하면
Then 컴파일 에러 없이 성공해야 한다
  And DefaultAdapter의 모든 메서드가 BridgeAdapter 인터페이스를 충족해야 한다
```

### AC-002: AdapterRegistry 등록 및 조회

```gherkin
Given AdapterRegistry가 초기화되어 있고
  And MQTTAdapter가 "mqtt" 키로 등록되어 있을 때
When GetAdapter("mqtt")를 호출하면
Then MQTTAdapter 인스턴스와 true를 반환해야 한다

When GetAdapter("unknown")를 호출하면
Then nil과 false를 반환해야 한다
```

### AC-003: AdapterRegistry 동시성 안전성

```gherkin
Given AdapterRegistry가 초기화되어 있을 때
When 10개의 고루틴에서 동시에 RegisterAdapter와 GetAdapter를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
  And 모든 등록된 어댑터가 정상적으로 조회되어야 한다
```

### AC-004: BridgeNode와 Adapter 통합 - 어댑터 존재 시

```gherkin
Given MQTTAdapter가 "mqtt" 키로 AdapterRegistry에 등록되어 있고
  And BridgeNode가 agentType="mqtt"인 AgentRef를 가질 때
When BridgeNode.Init()이 호출되면
Then BridgeNode는 MQTTAdapter를 transformer로 사용해야 한다
  And MQTTAdapter.Validate(config)가 호출되어야 한다
```

### AC-005: BridgeNode와 Adapter 통합 - 어댑터 미존재 시 (폴백)

```gherkin
Given "custom" 타입에 대한 어댑터가 등록되어 있지 않고
  And BridgeNode가 agentType="custom"인 AgentRef를 가질 때
When BridgeNode.Init()이 호출되면
Then BridgeNode는 DefaultAdapter(기존 DefaultTransformer 동작)를 사용해야 한다
  And 기존 BridgeNode 동작과 완전히 동일하게 작동해야 한다
```

---

## Module 2: MQTT 전용 어댑터

### AC-010: MQTT 토픽 라우팅 - 에이전트에서 플로우로

```gherkin
Given MQTTAdapter가 초기화되어 있고
  And MQTT 메시지가 토픽 "sensors/temp/room1"에서 수신되었을 때
When TransformToFlow(data, AgentMeta{Topic: "sensors/temp/room1"})를 호출하면
Then 반환된 Message의 Metadata에 "mqtt.topic" = "sensors/temp/room1"이 포함되어야 한다
  And Payload가 정상적으로 변환되어야 한다
```

### AC-011: MQTT QoS 레벨 보존

```gherkin
Given MQTTAdapter가 초기화되어 있을 때
When QoS=2인 MQTT 메시지를 TransformToFlow로 변환하면
Then Message Metadata에 "mqtt.qos" = "2"가 저장되어야 한다

When "mqtt.qos" = "1"인 Message를 TransformToAgent로 변환하면
Then 반환된 AgentMeta.QoS가 1이어야 한다
```

### AC-012: MQTT Retained 플래그 처리

```gherkin
Given MQTTAdapter가 초기화되어 있을 때
When retained=true인 MQTT 메시지를 TransformToFlow로 변환하면
Then Message Metadata에 "mqtt.retained" = "true"가 저장되어야 한다

When "mqtt.retained" = "true"인 Message를 TransformToAgent로 변환하면
Then 반환된 AgentMeta.Retained가 true여야 한다
```

### AC-013: MQTT 토픽 템플릿 보간

```gherkin
Given MQTTAdapter가 발행 토픽 템플릿 "devices/{device_id}/data"를 사용하고
  And Message Payload에 "device_id" = "sensor-001"이 있을 때
When TransformToAgent(msg)를 호출하면
Then 반환된 AgentMeta.Topic이 "devices/sensor-001/data"여야 한다
```

### AC-014: MQTT 설정 검증 - 유효한 설정

```gherkin
Given Direction이 "in"이고 Topics에 ["sensors/#"]가 있고 QoS가 1일 때
When MQTTAdapter.Validate(config)를 호출하면
Then nil을 반환해야 한다 (검증 통과)
```

### AC-015: MQTT 설정 검증 - 토픽 누락

```gherkin
Given Direction이 "in"이고 Topics가 비어 있을 때
When MQTTAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 "topics required" 관련 내용이 포함되어야 한다
```

### AC-016: MQTT 설정 검증 - 유효하지 않은 QoS

```gherkin
Given QoS 값이 3일 때
When MQTTAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 "QoS" 관련 내용이 포함되어야 한다
```

---

## Module 3: Modbus 전용 어댑터

### AC-020: Modbus 레지스터 -> Go 네이티브 타입 변환 (uint16)

```gherkin
Given ModbusAdapter에 RegisterDef{Name: "temperature", Address: 100, Count: 1, DataType: "uint16", Scale: 0.1, Offset: 0}가 설정되어 있고
  And 레지스터 데이터가 [0x00, 0xC8] (uint16 = 200)일 때
When TransformToFlow(data, meta)를 호출하면
Then Message Payload에 "temperature" = 20.0 (200 * 0.1)이 포함되어야 한다
```

### AC-021: Modbus 레지스터 -> Go 네이티브 타입 변환 (float32)

```gherkin
Given ModbusAdapter에 RegisterDef{Name: "power", Address: 200, Count: 2, DataType: "float32", ByteOrder: "big"}가 설정되어 있고
  And 레지스터 데이터가 IEEE 754 float32 인코딩된 값일 때
When TransformToFlow(data, meta)를 호출하면
Then Message Payload에 "power" 값이 정확한 float32 값으로 변환되어야 한다
```

### AC-022: Modbus 역변환 (Go 값 -> 레지스터 데이터)

```gherkin
Given ModbusAdapter에 RegisterDef{Name: "setpoint", Address: 300, Count: 1, DataType: "int16", Scale: 0.1}가 설정되어 있고
  And Message Payload에 "setpoint" = 25.5가 있을 때
When TransformToAgent(msg)를 호출하면
Then 반환된 데이터가 int16 값 255 (25.5 / 0.1)의 바이트 인코딩이어야 한다
  And AgentMeta.FunctionCode가 FC06 (Write Single Register)이어야 한다
```

### AC-023: Modbus 기능 코드 매핑

```gherkin
Given ModbusAdapter가 Direction="in"으로 설정되어 있을 때
When 레지스터 읽기 요청을 생성하면
Then FunctionCode가 FC03 또는 FC04 (Read Holding/Input Registers)여야 한다

Given ModbusAdapter가 Direction="out"으로 설정되어 있을 때
When 단일 레지스터 쓰기 요청을 생성하면
Then FunctionCode가 FC06 (Write Single Register)이어야 한다

Given ModbusAdapter가 Direction="out"으로 설정되어 있을 때
When 다중 레지스터 쓰기 요청을 생성하면
Then FunctionCode가 FC16 (Write Multiple Registers)이어야 한다
```

### AC-024: Modbus 설정 검증 - 유효한 설정

```gherkin
Given Unit ID가 1이고 RegisterMap에 유효한 RegisterDef가 있고 PollingInterval이 1000ms일 때
When ModbusAdapter.Validate(config)를 호출하면
Then nil을 반환해야 한다 (검증 통과)
```

### AC-025: Modbus 설정 검증 - Unit ID 범위 초과

```gherkin
Given Unit ID가 0이거나 248 이상일 때
When ModbusAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 "unit ID" 관련 내용이 포함되어야 한다
```

### AC-026: Modbus 설정 검증 - 폴링 인터벌 미달

```gherkin
Given Direction이 "in"이고 PollingInterval이 50ms일 때
When ModbusAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 "polling interval" 관련 내용이 포함되어야 한다
```

---

## Module 4: HTTP 전용 어댑터

### AC-030: HTTP JSON Content-Type 처리

```gherkin
Given HTTPAdapter가 초기화되어 있고
  And HTTP 응답 Content-Type이 "application/json"이고
  And 응답 본문이 '{"status": "ok", "value": 42}'일 때
When TransformToFlow(data, AgentMeta{ContentType: "application/json", StatusCode: 200})를 호출하면
Then Message Payload에 "status" = "ok"과 "value" = 42가 포함되어야 한다
  And Metadata에 "http.status_code" = "200"이 저장되어야 한다
  And Metadata에 "http.content_type" = "application/json"이 저장되어야 한다
```

### AC-031: HTTP 4xx/5xx 에러 응답 처리

```gherkin
Given HTTPAdapter가 초기화되어 있고
  And HTTP 응답 StatusCode가 404일 때
When TransformToFlow(data, meta)를 호출하면
Then 반환된 Message의 Metadata에 "http.status_code" = "404"가 저장되어야 한다
  And Message에 에러 플래그가 설정되어야 한다
```

### AC-032: HTTP 헤더 전달

```gherkin
Given HTTP 응답에 "X-Request-Id: abc-123" 헤더가 있을 때
When TransformToFlow(data, AgentMeta{Headers: {"X-Request-Id": "abc-123"}})를 호출하면
Then Message Metadata에 "http.header.x-request-id" = "abc-123"이 저장되어야 한다
```

### AC-033: HTTP URL 경로 템플릿

```gherkin
Given HTTPAdapter에 URL 패턴 "/api/devices/{id}/status"가 설정되어 있고
  And Message Payload에 "id" = "dev-001"이 있을 때
When TransformToAgent(msg)를 호출하면
Then 반환된 AgentMeta.URLPath가 "/api/devices/dev-001/status"여야 한다
```

### AC-034: HTTP 설정 검증

```gherkin
Given Content-Type이 "application/json"이고 타임아웃이 30초일 때
When HTTPAdapter.Validate(config)를 호출하면
Then nil을 반환해야 한다 (검증 통과)

Given 타임아웃이 0 또는 음수일 때
When HTTPAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
```

---

## Module 5: 프론트엔드 에이전트별 설정 UI

### AC-040: 에이전트 타입별 동적 설정 패널 렌더링

```gherkin
Given 사용자가 Bridge Node를 선택했을 때
When 에이전트 타입을 "mqtt"로 변경하면
Then MQTT 전용 설정 패널(토픽, QoS, Retained 필드)이 표시되어야 한다
  And 기존 범용 설정 필드는 유지되어야 한다

When 에이전트 타입을 "modbus-tcp"로 변경하면
Then Modbus 전용 설정 패널(Unit ID, 폴링 인터벌, 레지스터 맵)이 표시되어야 한다
  And MQTT 전용 필드는 숨겨져야 한다
```

### AC-041: MQTT 설정 패널 동작

```gherkin
Given MQTT 설정 패널이 표시되어 있을 때
When 사용자가 구독 토픽 "sensors/#"를 입력하고 QoS를 2로 선택하면
Then Bridge Node의 config에 topics=["sensors/#"]와 qos=2가 반영되어야 한다

When 사용자가 추가 구독 토픽 "actuators/+"를 입력하면
Then topics 배열에 ["sensors/#", "actuators/+"]가 포함되어야 한다
```

### AC-042: Modbus 레지스터 맵 편집

```gherkin
Given Modbus 설정 패널이 표시되어 있을 때
When 사용자가 레지스터 항목을 추가하고 Name="temperature", Address=100, Count=1, DataType="uint16"을 입력하면
Then 레지스터 맵 테이블에 해당 항목이 표시되어야 한다
  And Bridge Node의 config.registerMap에 해당 RegisterDef가 반영되어야 한다
```

### AC-043: 설정 유효성 검증 UI - 인라인 에러

```gherkin
Given MQTT 설정 패널에서 QoS 값을 5로 입력했을 때
When 설정 저장을 시도하면
Then QoS 필드 옆에 인라인 에러 메시지가 표시되어야 한다
  And 설정이 저장되지 않아야 한다

Given Modbus 설정 패널에서 Unit ID를 0으로 입력했을 때
When 설정 저장을 시도하면
Then Unit ID 필드 옆에 인라인 에러 메시지가 표시되어야 한다
```

---

## Module 6: System Agent 통합 어댑터

### AC-050: SystemAdapter Store 서브타입 통합

```gherkin
Given SystemAdapter가 에이전트 서브타입 "store"로 초기화되어 있고
  And Message Metadata에 "store.operation" = "get"과 "store.key" = "mykey"가 있을 때
When SystemAdapter.TransformToAgent(msg)를 호출하면
Then 기존 BridgeHandler.HandleMessage()와 동일한 결과를 반환해야 한다
```

### AC-051: SystemAdapter Timer 서브타입 통합

```gherkin
Given SystemAdapter가 에이전트 서브타입 "timer"로 초기화되어 있고
  And Message Metadata에 "timer.operation" = "set_interval"이 있을 때
When SystemAdapter.TransformToAgent(msg)를 호출하면
Then 기존 TimerBridgeHandler.HandleMessage()와 동일한 결과를 반환해야 한다
```

### AC-052: SystemAdapter 설정 검증 - 유효한 서브타입

```gherkin
Given 에이전트 서브타입이 "store"일 때
When SystemAdapter.Validate(config)를 호출하면
Then nil을 반환해야 한다 (검증 통과)

Given 에이전트 서브타입이 "invalid"일 때
When SystemAdapter.Validate(config)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 유효한 서브타입 목록(store, timer, logger, event, file)이 포함되어야 한다
```

---

## 엣지 케이스 시나리오

### EC-001: nil 데이터 변환

```gherkin
Given 임의의 BridgeAdapter가 있을 때
When TransformToFlow(nil, AgentMeta{})를 호출하면
Then 적절한 에러를 반환하거나 빈 Message를 생성해야 한다
  And 패닉이 발생하지 않아야 한다
```

### EC-002: 빈 메타데이터

```gherkin
Given MQTTAdapter가 있을 때
When TransformToFlow(validData, AgentMeta{})를 호출하면 (토픽 없음)
Then Message가 생성되되 "mqtt.topic" 메타데이터가 빈 문자열이어야 한다
  And 에러가 발생하지 않아야 한다
```

### EC-003: Modbus 바이트 오더 불일치

```gherkin
Given RegisterDef의 ByteOrder가 "little"이고
  And 수신 데이터가 big-endian 형식일 때
When TransformToFlow를 호출하면
Then little-endian으로 해석하여 변환해야 한다
  And 변환 결과가 big-endian 해석과 다를 수 있음을 인지해야 한다
```

### EC-004: MQTT 토픽 템플릿 - Payload에 키 없음

```gherkin
Given 토픽 템플릿이 "devices/{device_id}/data"이고
  And Message Payload에 "device_id" 키가 없을 때
When TransformToAgent(msg)를 호출하면
Then 에러를 반환해야 한다
  And 에러 메시지에 누락된 템플릿 변수 이름이 포함되어야 한다
```

### EC-005: 동시 어댑터 사용

```gherkin
Given 하나의 AdapterRegistry에 다수의 어댑터가 등록되어 있고
  And 여러 BridgeNode가 동시에 서로 다른 어댑터를 사용할 때
When 동시에 TransformToFlow/TransformToAgent를 호출하면
Then 각 BridgeNode가 올바른 어댑터를 사용해야 한다
  And race condition이 발생하지 않아야 한다
```

### EC-006: Modbus float32 경계값

```gherkin
Given RegisterDef의 DataType이 "float32"일 때
When NaN, +Inf, -Inf, 최소 양수, 최대값에 해당하는 레지스터 데이터를 변환하면
Then 각각 적절한 Go float32 값으로 변환되어야 한다
  And math.IsNaN, math.IsInf로 확인 가능해야 한다
```

---

## 성능 기준

### PERF-001: 메시지 변환 지연 시간

```
모든 BridgeAdapter의 TransformToFlow/TransformToAgent 호출은
단일 메시지 기준 1ms 이내에 완료되어야 한다.

측정 방법: Go benchmark 테스트 (b.N 반복)
대상: MQTTAdapter, ModbusAdapter, HTTPAdapter, SystemAdapter, DefaultAdapter
환경: CI 환경 기준 (지연 시간 3ms까지 허용)
```

### PERF-002: AdapterRegistry 조회 성능

```
AdapterRegistry.GetAdapter() 호출은 O(1) 시간 복잡도를 가져야 한다.

측정 방법: 100개 어댑터 등록 후 GetAdapter 벤치마크
기준: 단일 호출 100ns 이내
```

### PERF-003: Modbus 레지스터 변환 처리량

```
ModbusAdapter는 100개 레지스터 동시 변환을 500us 이내에 완료해야 한다.

측정 방법: 100개 RegisterDef 설정 후 일괄 변환 벤치마크
```

---

## Quality Gate 기준

### 코드 커버리지

- 전체 신규 코드: 85% 이상
- 각 어댑터 패키지 개별: 85% 이상
- 엣지 케이스 테스트 포함

### Race Condition

- `go test -race ./internal/node/...` 통과
- `go test -race ./internal/node/adapter/...` 통과

### 정적 분석

- `go vet ./...` 경고 0건
- 린트 에러 0건

### 회귀 테스트

- 기존 BridgeNode 테스트 100% 통과
- 기존 BridgeHandler 테스트 100% 통과
- SPEC-BRIDGE-001 수락 기준 유지

---

## Definition of Done

- [x] Module 1: BridgeAdapter 인터페이스 및 AdapterRegistry 구현, DefaultAdapter 구현
- [x] Module 2: MQTTAdapter 구현 및 테스트 통과 (최소 6개 테스트 시나리오)
- [x] Module 3: ModbusAdapter 구현 및 테스트 통과 (최소 7개 테스트 시나리오)
- [x] Module 4: HTTPAdapter 구현 및 테스트 통과 (최소 5개 테스트 시나리오)
- [x] Module 5: 프론트엔드 에이전트별 설정 패널 구현 및 동작 확인
- [x] Module 6: SystemAdapter 구현 및 기존 BridgeHandler 동등성 검증
- [x] 기존 BridgeNode 회귀 테스트 통과
- [x] go test -race 통과
- [x] 코드 커버리지 85% 이상
- [x] go vet 경고 0건
- [x] 성능 벤치마크 기준 충족

---

## 범위 확장 수락 기준

### AC-060: PollableAdapter 인터페이스

```gherkin
Given ModbusAdapter가 PollableAdapter를 구현하고
  And RegisterDef 배열이 설정되어 있을 때
When ReadSpecs()를 호출하면
Then RegisterDef마다 ReadSpec이 생성되어야 한다
  And FunctionCode, StartAddr, Quantity가 정확해야 한다
```

### AC-061: 브릿지 주도 폴 루프

```gherkin
Given BridgeNode가 PollableAdapter를 가진 어댑터에 연결되어 있고
  And polling_interval이 1000ms로 설정되어 있을 때
When BridgeNode가 Init()을 수행하면
Then startBridgePollLoop가 시작되어야 한다
  And 1초마다 에이전트에 read_raw 명령을 전송해야 한다
  And 응답을 AssembleMessage로 조합하여 플로우에 전달해야 한다
```

### AC-062: ModbusServerAgent read_raw 명령

```gherkin
Given ModbusServerAgent가 실행 중이고
  And Input Register 0~3에 값이 저장되어 있을 때
When read_raw 명령(function_code=4, address=0, quantity=4)을 전송하면
Then 로컬 레지스터 스토어에서 값을 읽어 base64 인코딩된 데이터를 반환해야 한다
```

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-06*
*작성: MoAI SPEC Builder*
