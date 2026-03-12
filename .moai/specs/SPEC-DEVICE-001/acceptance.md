---
id: SPEC-DEVICE-001
type: acceptance
status: planned
created: "2026-03-12"
updated: "2026-03-12"
---

# SPEC-DEVICE-001 인수 테스트 기준

---

## Module 1: 통합 디바이스 모델 테스트

### Scenario 1.1: NASA 디바이스 어댑터 변환

- **Given**: NASADevice가 NASAAgent에 등록되어 있다 (Address: 20.01.00, Type: indoor, State: Power=true, Mode=cool, TargetTemp=24)
- **When**: NASA 어댑터를 통해 Device 인터페이스로 변환한다
- **Then**: `Device.ID()`는 `"{agent_name}:20.01.00"`을 반환한다
- **And**: `Device.Type()`은 `"indoor"`을 반환한다
- **And**: `Device.Protocol()`은 `"nasa"`를 반환한다
- **And**: `Device.State().Properties`에 `power=true`, `mode="cool"`, `target_temp=24.0`이 포함된다

### Scenario 1.2: ControllableDevice 명령 실행

- **Given**: NASA 디바이스가 ControllableDevice로 변환되어 있다
- **When**: `Execute(ctx, "set_temperature", {"value": 22.0})`를 호출한다
- **Then**: NASAAgent의 `Process("set_temperature", ...)`가 호출된다
- **And**: 결과에 성공 상태가 포함된다

### Scenario 1.3: MODBUS 디바이스 어댑터 변환

- **Given**: ModbusDevice가 MODBUSAgent에 등록되어 있다 (SlaveID: 1, Name: "온도센서", Type: sensor)
- **When**: MODBUS 어댑터를 통해 Device 인터페이스로 변환한다
- **Then**: `Device.ID()`는 `"{agent_name}:1"`을 반환한다
- **And**: `Device.Type()`은 `"sensor"`를 반환한다
- **And**: `Device.Protocol()`은 `"modbus"`를 반환한다

---

## Module 2: 디바이스 레지스트리 테스트

### Scenario 2.1: 에이전트 등록 시 디바이스 통합

- **Given**: NASAAgent에 3개의 디바이스가 있다
- **And**: MODBUSAgent에 2개의 디바이스가 있다
- **When**: 두 에이전트의 DeviceProvider가 레지스트리에 등록된다
- **Then**: `registry.Count()`는 5를 반환한다
- **And**: `registry.List(DeviceFilter{})`는 5개의 디바이스를 반환한다

### Scenario 2.2: 프로토콜별 필터링

- **Given**: NASA 디바이스 3개와 MODBUS 디바이스 2개가 레지스트리에 등록되어 있다
- **When**: `registry.List(DeviceFilter{Protocol: "nasa"})`를 호출한다
- **Then**: 3개의 NASA 디바이스만 반환된다

### Scenario 2.3: 에이전트 중지 시 디바이스 오프라인 처리

- **Given**: NASAAgent의 DeviceProvider가 등록되어 있다
- **When**: NASAAgent가 중지되어 `UnregisterProvider`가 호출된다
- **Then**: 해당 디바이스들의 Online 상태가 `false`로 변경된다
- **And**: 디바이스는 목록에서 제거되지 않는다

### Scenario 2.4: 메타데이터 영속성

- **Given**: 디바이스 `"nasa-agent:20.01.00"`에 메타데이터가 설정되어 있다 (Location: "1층 로비", Tags: ["hvac"])
- **When**: 에이전트가 재시작된다
- **Then**: 메타데이터가 이전과 동일하게 유지된다

### Scenario 2.5: 중복 등록 방지

- **Given**: 디바이스 `"nasa-agent:20.01.00"`이 레지스트리에 이미 등록되어 있다
- **When**: 동일 ID로 다시 등록을 시도한다
- **Then**: 기존 디바이스 정보가 업데이트된다 (중복 생성되지 않음)

---

## Module 3: 디바이스 REST API 테스트

### Scenario 3.1: 디바이스 목록 조회

- **Given**: 레지스트리에 5개의 디바이스가 등록되어 있다
- **When**: `GET /api/devices` 요청을 보낸다
- **Then**: `200 OK`와 함께 5개의 디바이스 목록이 반환된다
- **And**: 각 디바이스에 `id`, `name`, `type`, `protocol`, `online`, `state`가 포함된다

### Scenario 3.2: 디바이스 필터 조회

- **Given**: NASA 3개, MODBUS 2개 디바이스가 있다
- **When**: `GET /api/devices?protocol=nasa` 요청을 보낸다
- **Then**: `200 OK`와 함께 3개의 NASA 디바이스만 반환된다

### Scenario 3.3: 디바이스 상세 조회

- **Given**: 디바이스 `"nasa-agent:20.01.00"`이 등록되어 있다
- **When**: `GET /api/devices/nasa-agent:20.01.00` 요청을 보낸다
- **Then**: `200 OK`와 함께 상세 정보가 반환된다
- **And**: `state.properties`에 프로토콜별 상태값이 포함된다
- **And**: `capabilities`에 지원 기능 목록이 포함된다

### Scenario 3.4: 명령 실행

- **Given**: ControllableDevice `"nasa-agent:20.01.00"`이 등록되어 있다
- **When**: `POST /api/devices/nasa-agent:20.01.00/execute` `{"command": "set_temperature", "params": {"value": 22}}` 요청을 보낸다
- **Then**: `200 OK`와 함께 실행 결과가 반환된다

### Scenario 3.5: 비제어 디바이스에 명령 실행 시 에러

- **Given**: ControllableDevice를 구현하지 않는 디바이스 `"modbus-agent:1"`이 있다
- **When**: `POST /api/devices/modbus-agent:1/execute` 요청을 보낸다
- **Then**: `405 Method Not Allowed`가 반환된다

### Scenario 3.6: 중지된 에이전트 디바이스 제어 시 에러

- **Given**: NASAAgent가 중지되어 `"nasa-agent:20.01.00"`이 오프라인이다
- **When**: `POST /api/devices/nasa-agent:20.01.00/execute` 요청을 보낸다
- **Then**: `409 Conflict`가 반환된다

### Scenario 3.7: 메타데이터 설정

- **Given**: 디바이스 `"nasa-agent:20.01.00"`이 등록되어 있다
- **When**: `PUT /api/devices/nasa-agent:20.01.00/metadata` `{"location": "1층 로비", "tags": ["hvac"]}` 요청을 보낸다
- **Then**: `200 OK`와 함께 업데이트된 메타데이터가 반환된다

---

## Module 4: 디바이스 관리 웹 UI 테스트

### Scenario 4.1: 디바이스 목록 페이지 렌더링

- **Given**: 5개의 디바이스가 API에서 반환된다
- **When**: `/devices` 페이지에 접근한다
- **Then**: 5개의 디바이스가 테이블에 표시된다
- **And**: 각 행에 이름, 타입, 프로토콜, 상태, 에이전트가 표시된다

### Scenario 4.2: 디바이스 필터링

- **Given**: 디바이스 목록 페이지가 표시되어 있다
- **When**: 프로토콜 필터에서 "nasa"를 선택한다
- **Then**: NASA 디바이스만 테이블에 표시된다

### Scenario 4.3: 디바이스 상세 표시

- **Given**: 디바이스 목록이 표시되어 있다
- **When**: 디바이스 "1층 로비 실내기"를 클릭한다
- **Then**: 상세 패널에 상태 카드, 프로퍼티 테이블, 제어 패널이 표시된다

### Scenario 4.4: 디바이스 명령 실행

- **Given**: ControllableDevice 상세 페이지가 표시되어 있다
- **When**: 온도 설정 슬라이더를 22도로 변경하고 적용 버튼을 클릭한다
- **Then**: API 호출이 실행된다
- **And**: 결과가 화면에 반영된다

---

## 품질 게이트

| 기준 | 목표값 |
|------|--------|
| 테스트 커버리지 (새 코드) | 85%+ |
| 테스트 커버리지 (수정 코드) | 85%+ |
| TypeScript 에러 | 0 |
| Go vet 경고 | 0 |
| `go build` 성공 | 필수 |
| `go test -race` 통과 | 필수 |

---

status: planned
updated: "2026-03-12"
