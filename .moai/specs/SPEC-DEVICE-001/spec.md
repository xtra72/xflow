---
id: SPEC-DEVICE-001
version: "1.1.0"
status: completed
created: "2026-03-12"
updated: "2026-03-13"
author: xtra
priority: P0
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-03-12 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-03-13 | xtra | 전체 구현 완료 - M1~M9 마일스톤 완료 반영 |

# SPEC-DEVICE-001: 통합 디바이스 모델링 및 관리 시스템

## 개요

xflow의 프로토콜별 에이전트에 분산된 디바이스 모델(NASADevice, ModbusDevice)을 통합 인터페이스로 추상화하고, 중앙 레지스트리, 전용 REST API, 웹 관리 UI를 제공하여 디바이스를 프로토콜에 무관하게 일관되게 모델링하고 관리할 수 있게 한다.

## 배경 및 문제

- 현재 NASADevice(`internal/agent/samsung/device.go:13-23`)와 ModbusDevice(`internal/agent/modbus/device.go:12-22`)가 프로토콜별로 독립 정의
- 통합 디바이스 인터페이스 없음
- 디바이스 조회는 에이전트 State API를 개별 호출해야 함
- 디바이스 제어는 에이전트별 커맨드 체계를 알아야 함
- 디바이스 메타데이터(위치, 태그, 그룹) 부여 불가
- 디바이스 영속성 없음 (에이전트 재시작 시 설정에서 재구성)
- 디바이스 관리 전용 웹 UI 없음

## 대상 사용자

- **IoT 시스템 엔지니어**: 다양한 프로토콜 디바이스를 한곳에서 관리
- **빌딩 관리자**: HVAC 장비 상태 확인 및 제어
- **API 소비자**: REST API를 통한 디바이스 직접 제어

---

## Module 1: 통합 디바이스 모델 및 인터페이스

**Package**: `internal/device/`

### Environment

- xflow 에이전트 시스템이 프로토콜별 디바이스를 관리
- NASADevice와 ModbusDevice는 서로 다른 필드 구조
- Agent 인터페이스의 `StatefulAgent.State()`가 디바이스 상태를 에이전트 상태의 일부로 반환

### Requirements

- **R1.1** (Ubiquitous): 시스템은 **항상** 모든 디바이스를 프로토콜에 무관한 통합 인터페이스(Device)를 통해 표현해야 한다
- **R1.2** (Event-Driven): **WHEN** 에이전트가 새 디바이스를 감지하면 **THEN** 해당 디바이스를 통합 레지스트리에 등록해야 한다
- **R1.3** (State-Driven): **IF** 디바이스가 controllable 속성을 가지면 **THEN** 디바이스 인터페이스를 통해 명명된 명령 실행이 가능해야 한다
- **R1.4** (Ubiquitous): 시스템은 **항상** 디바이스 상태를 공통 필드(`online`, `lastSeen`, `errorCount`)와 프로토콜 확장 필드(`Properties map[string]any`)로 분리하여 제공해야 한다
- **R1.5** (Complex): **IF** 에이전트가 Running 상태이고 **AND WHEN** 디바이스 상태가 변경되면 **THEN** 디바이스 레지스트리에 최신 상태가 반영되어야 한다

### Specifications

**S1.1 Device 인터페이스** (`internal/device/device.go`):
- `ID() string` - 전역 고유 ID (`agent_name:device_id`)
- `Name() string` - 사용자 지정 이름
- `Type() DeviceType` - indoor, outdoor, controller, sensor, actuator 등
- `Protocol() string` - nasa, modbus, mqtt 등
- `AgentName() string` - 소속 에이전트 이름
- `Online() bool` - 온라인 여부
- `LastSeen() time.Time` - 마지막 통신 시각
- `State() DeviceState` - 현재 상태
- `Metadata() DeviceMetadata` - 메타데이터
- `Capabilities() []string` - 지원 기능 목록

**S1.2 DeviceState 구조체**:
- `Online bool` - 온라인 여부
- `Ready bool` - 준비 상태
- `LastSeen time.Time` - 마지막 통신 시각
- `ErrorCount int` - 에러 횟수
- `Properties map[string]any` - 프로토콜별 확장 속성

**S1.3 ControllableDevice 인터페이스**:
- Device 임베딩
- `Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error)`
- `Commands() []CommandSpec`

**S1.4 CommandSpec/ParamSpec 구조체**:
- `CommandSpec`: Name, Description, Params []ParamSpec
- `ParamSpec`: Name, Type, Required, Min, Max, Enum

**S1.5 DeviceMetadata 구조체**:
- `Tags []string` - 태그 목록
- `Location string` - 위치 정보
- `Group string` - 그룹 분류
- `Labels map[string]string` - 키-값 레이블

**S1.6 DeviceProvider 인터페이스** (에이전트 구현):
- `Devices() []Device`
- `Device(id string) (Device, error)`

---

## Module 2: 디바이스 레지스트리

**Package**: `internal/device/registry.go`

### Requirements

- **R2.1** (Ubiquitous): 시스템은 **항상** 모든 에이전트의 디바이스를 하나의 중앙 레지스트리에서 통합 관리해야 한다
- **R2.2** (Event-Driven): **WHEN** 에이전트가 시작되면 **THEN** 레지스트리는 해당 에이전트의 DeviceProvider를 등록하고 디바이스 목록을 동기화해야 한다
- **R2.3** (Event-Driven): **WHEN** 에이전트가 중지되면 **THEN** 레지스트리는 해당 에이전트의 디바이스를 오프라인으로 표시해야 한다 (삭제하지 않음)
- **R2.4** (State-Driven): **IF** 디바이스 메타데이터가 설정되어 있으면 **THEN** 에이전트 재시작 후에도 메타데이터가 보존되어야 한다
- **R2.5** (Unwanted): 시스템은 동일한 디바이스 ID로 중복 등록을 **허용하지 않아야 한다**

### Specifications

**S2.1 DeviceRegistry 인터페이스**:
- `List(filter DeviceFilter) []Device`
- `Get(id string) (Device, error)`
- `Count() int`
- `RegisterProvider(agentName string, provider DeviceProvider)`
- `UnregisterProvider(agentName string)`
- `SetMetadata(id string, metadata DeviceMetadata) error`
- `GetMetadata(id string) (DeviceMetadata, error)`
- `Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)`

**S2.2 DeviceFilter 구조체**:
- `Protocol string` - 프로토콜 필터
- `AgentName string` - 에이전트 이름 필터
- `Type string` - 디바이스 타입 필터
- `Online *bool` - 온라인 상태 필터
- `Tags []string` - 태그 필터
- `Group string` - 그룹 필터

**S2.3 메타데이터 영속성**:
- `storage.DeviceMetadataRepository`를 통해 DB에 저장

---

## Module 3: 디바이스 REST API

**Package**: `internal/api/handler/device.go`

### Requirements

- **R3.1** (Ubiquitous): 시스템은 **항상** 디바이스를 프로토콜에 무관한 통합 REST API를 통해 CRUD, 상태 조회, 제어할 수 있어야 한다
- **R3.2** (Event-Driven): **WHEN** 클라이언트가 `POST /api/devices/{id}/execute` 요청을 보내면 **THEN** 시스템은 해당 디바이스의 `Execute()` 메서드를 호출하고 결과를 반환해야 한다
- **R3.3** (Event-Driven): **WHEN** 클라이언트가 `GET /api/devices` 요청을 보내면 **THEN** 시스템은 모든 에이전트의 디바이스를 통합 목록으로 반환해야 한다
- **R3.4** (State-Driven): **IF** 디바이스가 ControllableDevice를 구현하지 않으면 **THEN** execute 요청 시 `405 Method Not Allowed`를 반환해야 한다
- **R3.5** (Unwanted): 시스템은 에이전트가 중지된 디바이스에 대한 제어 명령을 **허용하지 않아야 한다** (`409 Conflict`)

### Specifications

**S3.1 API 엔드포인트**:

| 메서드 | 경로 | 설명 |
|--------|------|------|
| GET | `/api/devices` | 디바이스 목록 (필터 쿼리 지원) |
| GET | `/api/devices/{id}` | 디바이스 상세 |
| PUT | `/api/devices/{id}/metadata` | 메타데이터 설정 |
| POST | `/api/devices/{id}/execute` | 명령 실행 |
| GET | `/api/devices/{id}/commands` | 지원 명령 목록 |
| GET | `/api/devices/{id}/state` | 현재 상태 |

**S3.2 응답 포맷** (JSON):
```json
{
  "devices": [
    {
      "id": "nasa-agent:20.01.00",
      "name": "1층 로비 실내기",
      "type": "indoor",
      "protocol": "nasa",
      "agent_name": "nasa-agent",
      "online": true,
      "last_seen": "2026-03-12T10:30:00Z",
      "state": {
        "online": true,
        "ready": true,
        "error_count": 0,
        "properties": {
          "power": true,
          "mode": "cool",
          "target_temp": 24.0,
          "current_temp": 25.2
        }
      },
      "metadata": {
        "tags": ["hvac", "lobby"],
        "location": "1층 로비",
        "group": "1층",
        "labels": {}
      },
      "capabilities": ["target_temperature", "set_mode", "set_power"],
      "controllable": true
    }
  ]
}
```

**S3.3 명령 실행 요청**:
```json
{
  "command": "target_temperature",
  "params": {
    "value": 22.0
  }
}
```

**S3.4 명령 실행 응답**:
```json
{
  "success": true,
  "device_id": "nasa-agent:20.01.00",
  "command": "target_temperature",
  "result": {
    "previous_target": 24.0,
    "new_target": 22.0
  }
}
```

---

## Module 4: 디바이스 관리 웹 UI

**Package**: `web/src/pages/DevicesPage/`, `web/src/components/DevicePanel/`

### Requirements

- **R4.1** (Event-Driven): **WHEN** 사용자가 디바이스 목록 페이지에 접근하면 **THEN** 시스템은 모든 프로토콜의 디바이스를 통합 테이블로 표시해야 한다
- **R4.2** (Event-Driven): **WHEN** 사용자가 디바이스를 선택하면 **THEN** 시스템은 상세 정보를 사이드 패널 또는 상세 페이지에 표시해야 한다
- **R4.3** (State-Driven): **IF** 디바이스가 제어 가능하면 **THEN** 상세 화면에 명령 실행 UI를 제공해야 한다
- **R4.4** (Event-Driven): **WHEN** 사용자가 명령을 실행하면 **THEN** 시스템은 API를 호출하고 결과를 즉시 반영해야 한다
- **R4.5** (Optional): **가능하면** 디바이스 상태를 주기적으로 폴링하여 실시간에 가까운 상태 표시를 제공한다

### Specifications

**S4.1 페이지 구성**:
- `DevicesPage`: 디바이스 목록 테이블 (필터, 검색)
- `DeviceDetailPage` 또는 사이드 패널: 상세 + 제어
- 대시보드 `DevicePanel` 위젯 (선택)

---

## 핵심 기술 결정

1. **어댑터 패턴**: 기존 NASADevice/ModbusDevice 수정 없이 Device 인터페이스 래핑
2. **Pull 모델 레지스트리**: 상태를 복제하지 않고 에이전트에서 실시간 조회 (Single Source of Truth)
3. **CommandSpec 기반 동적 UI**: 에이전트가 명령 스키마를 제공, UI 자동 생성
4. **메타데이터만 영속**: 태그/위치/그룹은 DB 저장, 런타임 상태는 에이전트 조회

## 의존성

| 구성요소 | 경로 | 변경 내용 |
|----------|------|-----------|
| NASAAgent | `internal/agent/samsung/agent.go` | DeviceProvider 구현 추가 |
| MODBUSAgent | `internal/agent/modbus/agent.go` | DeviceProvider 구현 추가 |
| Router | `internal/api/router.go` | `/api/devices` 라우트 추가 |
| Engine | `internal/engine/` | DeviceRegistry 초기화 |
| Storage | `internal/storage/` | DeviceMetadataRepository 추가 |
| Web | `web/src/` | 새 페이지/컴포넌트 추가 |

---

status: completed
updated: "2026-03-13"
