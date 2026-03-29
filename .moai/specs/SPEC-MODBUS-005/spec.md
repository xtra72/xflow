# SPEC-MODBUS-005: Modbus Server Multi-Device Support + Web UI

---
id: SPEC-MODBUS-005
version: 1.2.0
status: completed
created: 2026-03-16
updated: 2026-03-27
author: MoAI (manager-spec)
priority: high
depends_on:
  - SPEC-MODBUS-002
tags:
  - modbus
  - server
  - multi-device
  - web-ui
  - full-stack
---

## HISTORY

| 버전   | 날짜       | 변경 내용                          |
|--------|------------|------------------------------------|
| 0.1.0  | 2026-03-16 | 초안 작성 (Draft)                  |
| 1.0.0  | 2026-03-16 | 전체 구현 완료 (M1~M5), SPEC 동기화 |
| 1.1.0  | 2026-03-16 | AgentRef 해석 수정 (ID+Name 이중 검색), CLI --unit-id 플래그 추가, 노드 타입 테스트 19개 반영 |
| 1.2.0  | 2026-03-27 | ModbusServerAgent Start() 메서드에 Stopped 상태 복구 로직 추가 (Stopped→Created→Init() 재초기화) |

---

## 1. 개요 (Overview)

xflow의 Modbus TCP Server Agent는 현재 단일 Unit ID와 단일 RegisterMap만 지원한다 (SPEC-MODBUS-002). 실제 산업 환경에서는 하나의 서버 에이전트가 여러 가상 디바이스를 호스팅하고, 클라이언트 요청의 `unit_id`로 대상 디바이스를 라우팅하는 구조가 필요하다.

본 SPEC은 다음 두 영역을 정의한다:

- **백엔드**: 서버 에이전트에 다중 가상 디바이스(DeviceConfig) 지원, unit_id 기반 라우팅, exec 명령을 통한 디바이스 관리
- **프론트엔드**: 웹 UI의 Devices 탭을 확장하여 Modbus Server 에이전트의 디바이스 목록 표시, 상세 조회, 추가/삭제 관리 기능

---

## 2. 환경 (Environment)

| 항목             | 현재 상태                                          |
|------------------|----------------------------------------------------|
| 백엔드 언어      | Go 1.23+                                           |
| 프론트엔드       | React 19 + TypeScript 5.x + Tailwind CSS v4        |
| 상태 관리        | Zustand                                            |
| Modbus 서버 패키지 | `internal/agent/modbusserver/` (8 소스, 6 테스트)  |
| 웹 UI 에이전트   | `web/src/pages/agents/AgentDetailPanel.tsx`         |
| 스키마 정의      | `web/src/config/agentSchemas.ts`                   |
| 기존 SPEC        | SPEC-MODBUS-002 (완료, v1.0.0) - 단일 디바이스 서버 |
| 프로토콜         | Modbus/TCP (FC01-FC06, FC15-FC16)                  |
| Unit ID 범위     | 0-247 (0 = broadcast)                              |

---

## 3. 가정 (Assumptions)

| ID   | 가정 사항                                                                                      |
|------|-----------------------------------------------------------------------------------------------|
| A-1  | 하나의 서버 에이전트는 최대 247개의 가상 디바이스를 호스팅할 수 있다 (Unit ID 1-247)             |
| A-2  | 각 디바이스는 독립적인 RegisterMap 상태를 가지며, 디바이스 간 레지스터 간섭이 없다                |
| A-3  | Unit ID 0 (broadcast)은 모든 디바이스에 쓰기를 전파하되, 읽기는 첫 번째 디바이스에서 응답한다      |
| A-4  | 기존 단일 `unit_id` + `register_map` 설정은 자동으로 단일 디바이스로 변환되어 하위 호환성을 유지한다 |
| A-5  | 디바이스 추가/삭제는 exec 명령을 통해 런타임에 수행 가능하며, 에이전트 재시작 불필요              |
| A-6  | 프론트엔드 Devices 탭은 `samsung-nasa`와 `modbus-tcp-server` 타입 모두 지원한다                 |
| A-7  | 디바이스별 상태(connected, register 값)는 실시간 WebSocket/SSE로 업데이트된다                     |

---

## 4. 요구사항 (Requirements)

### 4.1 백엔드 - 설정 구조 변경

#### REQ-BE-001: 다중 디바이스 설정 구조

시스템은 **항상** `devices` 배열을 통해 여러 가상 디바이스를 설정할 수 있어야 한다.

**EARS**: 시스템은 **항상** `ModbusServerConfig.Devices []DeviceConfig` 필드를 통해 복수의 가상 디바이스를 정의할 수 있어야 한다.

```
DeviceConfig:
  unit_id:      byte     (1-247, 필수)
  name:         string   (선택, 관리 표시용)
  register_map: RegisterMapConfig (필수)
```

#### REQ-BE-002: 하위 호환성

**WHEN** 기존 형식(단일 `unit_id` + `register_map`)으로 설정이 제공되면, **THEN** 시스템은 자동으로 단일 DeviceConfig로 변환하여 정상 동작해야 한다.

**EARS**: **WHEN** `unit_id`와 `register_map`이 최상위에 존재하고 `devices` 배열이 없으면 **THEN** 시스템은 해당 설정을 `devices: [{unit_id: <unit_id>, register_map: <register_map>}]`로 자동 변환해야 한다.

#### REQ-BE-003: Unit ID 고유성 검증

시스템은 **항상** 동일 에이전트 내에서 중복 Unit ID를 거부해야 한다.

**EARS**: 시스템은 **항상** `devices` 배열 내 `unit_id` 값의 고유성을 검증하고, 중복 시 설정 오류를 반환해야 한다.

### 4.2 백엔드 - 요청 라우팅

#### REQ-BE-004: Unit ID 기반 요청 라우팅

**WHEN** Modbus TCP 요청이 수신되면 **THEN** 시스템은 요청의 `unit_id` 필드를 기준으로 해당 디바이스의 RegisterMap으로 라우팅해야 한다.

**EARS**: **WHEN** TCP 프레임의 Unit ID가 등록된 디바이스의 `unit_id`와 일치하면 **THEN** 해당 디바이스의 RegisterMap에서 요청을 처리해야 한다.

#### REQ-BE-005: 미등록 Unit ID 처리

**IF** 요청의 Unit ID가 어떤 디바이스에도 매칭되지 않으면 **THEN** 시스템은 해당 요청을 무시하고(응답 없음) 경고 로그를 남겨야 한다.

**EARS**: **IF** TCP 프레임의 Unit ID가 등록된 어떤 디바이스의 `unit_id`와도 일치하지 않으면 **THEN** 시스템은 응답을 보내지 않고 `warn` 레벨 로그를 기록해야 한다.

#### REQ-BE-006: Broadcast (Unit ID 0) 처리

**WHEN** Unit ID 0(broadcast)으로 쓰기 요청이 수신되면 **THEN** 시스템은 모든 등록된 디바이스에 해당 쓰기를 전파해야 한다.

**EARS**: **WHEN** TCP 프레임의 Unit ID가 0이고 Function Code가 쓰기(FC05, FC06, FC15, FC16)이면 **THEN** 시스템은 모든 디바이스의 RegisterMap에 동일한 쓰기를 수행해야 한다. 읽기 요청(FC01-FC04)의 경우 첫 번째 디바이스에서 응답한다.

### 4.3 백엔드 - Exec 명령

#### REQ-BE-007: 디바이스 목록 조회 (list_devices)

**WHEN** `list_devices` exec 명령이 실행되면 **THEN** 시스템은 모든 등록된 디바이스의 정보를 반환해야 한다.

**EARS**: **WHEN** exec 명령 `list_devices`가 호출되면 **THEN** 각 디바이스의 `unit_id`, `name`, 레지스터 영역별 개수, 상태를 포함한 목록을 JSON으로 반환해야 한다.

#### REQ-BE-008: 디바이스 추가 (add_device)

**WHEN** `add_device` exec 명령이 유효한 DeviceConfig와 함께 실행되면 **THEN** 시스템은 런타임에 새 디바이스를 추가해야 한다.

**EARS**: **WHEN** exec 명령 `add_device`가 `{unit_id, name, register_map}` 파라미터와 함께 호출되면 **THEN** 시스템은 Unit ID 고유성을 검증한 후 새 디바이스를 등록하고 즉시 요청 수신을 시작해야 한다.

#### REQ-BE-009: 디바이스 삭제 (remove_device)

**WHEN** `remove_device` exec 명령이 유효한 unit_id와 함께 실행되면 **THEN** 시스템은 해당 디바이스를 제거해야 한다.

**EARS**: **WHEN** exec 명령 `remove_device`가 `{unit_id}` 파라미터와 함께 호출되면 **THEN** 시스템은 해당 디바이스를 등록 해제하고 레지스터 상태를 정리해야 한다. 마지막 디바이스는 삭제할 수 없다.

#### REQ-BE-010: 디바이스 상태 조회 (get_device_status)

**WHEN** `get_device_status` exec 명령이 unit_id와 함께 실행되면 **THEN** 시스템은 해당 디바이스의 상세 상태를 반환해야 한다.

**EARS**: **WHEN** exec 명령 `get_device_status`가 `{unit_id}` 파라미터와 함께 호출되면 **THEN** 해당 디바이스의 레지스터 값, 최근 요청 통계, 생성 시각 등 상세 정보를 반환해야 한다.

### 4.4 백엔드 - 상태 및 통계

#### REQ-BE-011: 디바이스별 독립 상태

시스템은 **항상** 각 디바이스의 레지스터 상태를 독립적으로 관리해야 한다.

**EARS**: 시스템은 **항상** 각 `DeviceConfig`에 대해 독립적인 `RegisterMap` 인스턴스를 생성하고, 디바이스 간 레지스터 읽기/쓰기가 간섭하지 않아야 한다.

#### REQ-BE-012: 에이전트 Stats에 디바이스 수 포함

시스템은 **항상** 에이전트 통계에 현재 등록된 디바이스 수를 포함해야 한다.

**EARS**: 시스템은 **항상** `AgentStats`의 `extra` 필드에 `device_count` 키로 현재 디바이스 수를 보고해야 한다.

### 4.5 프론트엔드 - Devices 탭 확장

#### REQ-FE-001: Modbus Server 디바이스 탭 지원

**WHEN** 에이전트 타입이 `modbus-tcp-server`인 상세 패널이 열리면 **THEN** Devices 탭에서 해당 에이전트의 디바이스 목록을 표시해야 한다.

**EARS**: **WHEN** `AgentDetailPanel`에서 `agentType`이 `modbus-tcp-server`이고 Devices 탭이 선택되면 **THEN** `list_devices` exec 명령 결과를 기반으로 디바이스 목록을 렌더링해야 한다.

#### REQ-FE-002: 디바이스 목록 표시

시스템은 **항상** 디바이스 목록에 다음 정보를 표시해야 한다:
- Unit ID
- 디바이스 이름 (설정된 경우)
- 레지스터 영역별 개수 (Coils, DI, HR, IR)
- 상태 (active/inactive)

**EARS**: 시스템은 **항상** 각 디바이스 카드에 `unit_id`, `name`, 레지스터 요약, 상태 배지를 표시해야 한다.

#### REQ-FE-003: 디바이스 상세 조회

**WHEN** 사용자가 디바이스 목록에서 특정 디바이스를 선택하면 **THEN** 해당 디바이스의 레지스터 맵 상세 정보를 표시해야 한다.

**EARS**: **WHEN** 사용자가 디바이스 카드를 클릭하면 **THEN** `get_device_status` exec 명령을 호출하여 레지스터 값, 세그먼트 정보, 요청 통계를 표시하는 상세 뷰를 렌더링해야 한다.

#### REQ-FE-004: 디바이스 추가 UI

**WHEN** 사용자가 "디바이스 추가" 버튼을 클릭하면 **THEN** Unit ID, 이름, 레지스터 맵 설정을 입력할 수 있는 폼을 표시해야 한다.

**EARS**: **WHEN** 사용자가 추가 버튼을 클릭하면 **THEN** 모달 또는 인라인 폼을 통해 `unit_id` (1-247), `name` (선택), `register_map` 설정을 입력받고 `add_device` exec 명령을 실행해야 한다.

#### REQ-FE-005: 디바이스 삭제 UI

**WHEN** 사용자가 디바이스의 삭제 버튼을 클릭하면 **THEN** 확인 후 해당 디바이스를 삭제해야 한다.

**EARS**: **WHEN** 사용자가 삭제 버튼을 클릭하면 **THEN** 확인 다이얼로그를 표시하고, 확인 시 `remove_device` exec 명령을 실행하여 디바이스를 제거해야 한다. 마지막 디바이스인 경우 삭제 버튼을 비활성화한다.

### 4.6 프론트엔드 - 스키마 확장

#### REQ-FE-006: agentSchemas 업데이트

시스템은 **항상** `MODBUS_TCP_SERVER_FIELDS`에서 단일 `unit_id`와 `register_map` 필드를 `devices` 배열 설정으로 대체해야 한다.

**EARS**: 시스템은 **항상** `agentSchemas.ts`의 `MODBUS_TCP_SERVER_FIELDS`를 업데이트하여 `devices` 필드(type: `object`)를 포함하고, 기존 단일 `unit_id`와 `register_map` 필드를 제거해야 한다.

---

## 5. 제약사항 (Constraints)

| ID   | 제약 사항                                                                                  |
|------|-------------------------------------------------------------------------------------------|
| C-1  | 기존 단일 디바이스 설정과의 하위 호환성을 반드시 유지해야 한다                                |
| C-2  | 디바이스 추가/삭제 시 기존 연결된 TCP 클라이언트에 영향을 주지 않아야 한다                     |
| C-3  | `sync.RWMutex`를 사용하여 디바이스 맵 동시 접근을 보호해야 한다                               |
| C-4  | 백엔드 테스트 커버리지 85% 이상을 유지해야 한다                                              |
| C-5  | 프론트엔드는 기존 DevicesTab 컴포넌트 패턴을 재사용하여 일관성을 유지해야 한다                  |
| C-6  | Exec 명령 응답은 기존 exec 인터페이스(`ExecCommand` / `ExecResult`)를 따라야 한다             |

---

## 6. 사양 (Specifications)

### 6.1 백엔드 데이터 구조

#### SPEC-BE-001: DeviceConfig 구조체

```go
// DeviceConfig는 하나의 가상 Modbus 디바이스를 정의한다.
type DeviceConfig struct {
    UnitID      byte              // 1-247, 필수
    Name        string            // 선택, 관리용 이름
    RegisterMap RegisterMapConfig // 필수, 레지스터 맵 설정
}
```

#### SPEC-BE-002: ModbusServerConfig 변경

```go
type ModbusServerConfig struct {
    ListenAddress    string          // 기존 유지
    ListenPort       int             // 기존 유지
    MaxConnections   int             // 기존 유지
    IdleTimeout      time.Duration   // 기존 유지
    Devices          []DeviceConfig  // 신규: 다중 디바이스
    // 하위 호환 필드 (deprecated, 내부 변환용)
    UnitID           byte            // deprecated
    RegisterMap      RegisterMapConfig // deprecated
}
```

#### SPEC-BE-003: DeviceManager 구조체

```go
// DeviceManager는 다중 디바이스의 레지스터 맵을 관리한다.
type DeviceManager struct {
    mu      sync.RWMutex
    devices map[byte]*Device  // unit_id -> Device
}

type Device struct {
    Config      DeviceConfig
    RegisterMap *RegisterMap  // 독립적 레지스터 상태
    Stats       DeviceStats   // 디바이스별 통계
    CreatedAt   time.Time
}
```

### 6.2 백엔드 Exec 명령 인터페이스

#### SPEC-BE-004: list_devices 응답

```json
{
  "devices": [
    {
      "unit_id": 1,
      "name": "Temperature Sensor",
      "register_counts": {
        "coils": 16,
        "discrete_inputs": 0,
        "holding_registers": 10,
        "input_registers": 8
      },
      "status": "active",
      "created_at": "2026-03-16T10:00:00Z"
    }
  ],
  "total": 1
}
```

#### SPEC-BE-005: add_device 요청/응답

요청:
```json
{
  "unit_id": 2,
  "name": "Pressure Sensor",
  "register_map": {
    "holding_registers": [
      {"start_address": 0, "count": 10}
    ]
  }
}
```

응답:
```json
{
  "success": true,
  "device": { "unit_id": 2, "name": "Pressure Sensor", "status": "active" }
}
```

#### SPEC-BE-006: remove_device 요청/응답

요청: `{"unit_id": 2}`
응답: `{"success": true, "removed_unit_id": 2}`
오류: `{"success": false, "error": "cannot remove last device"}`

### 6.3 프론트엔드 컴포넌트 구조

#### SPEC-FE-001: ModbusDevicesSection 컴포넌트 (실제 구현)

```
AgentDetailPanel.tsx
  +-- ModbusDevicesSection (agentId)   ← agentType === 'modbus-tcp-server'일 때
  |     +-- 디바이스 카드 목록 (unit_id, name, register_counts, stats)
  |     +-- RegisterMapTable (접이식 영역별 테이블, ON/OFF 뱃지, Dec+Hex 값)
  |     +-- AddDeviceModal (Unit ID + 이름 + 멀티 블록 레지스터 맵 폼)
  |     +-- DeleteConfirm (마지막 디바이스 삭제 방지)
  +-- DevicesTab (agentId, agentType)  ← samsung-nasa 등 기타 프로토콜
```

#### SPEC-FE-002: DevicesTab 라우팅 (실제 구현)

```typescript
// AgentDetailPanel.tsx 내 Devices 탭 분기
if (agentType === 'modbus-tcp-server') {
  return <ModbusDevicesSection agentId={agentId} />;
}
// 기타 프로토콜은 기존 DevicesTab 사용
return <DevicesTab agentId={agentId} agentType={agentType} />;
```

#### SPEC-FE-003: 디바이스 페이지 AddDeviceDialog (추가 구현)

```
DeviceListPage.tsx
  +-- AddDeviceDialog
  |     +-- 에이전트 선택 (samsung-nasa + modbus-tcp-server 필터)
  |     +-- NASA 폼: 주소, ID, 타입
  |     +-- Modbus 폼: Unit ID, 이름, 영역별 멀티 블록 레지스터 맵
```

---

## 7. 추적성 (Traceability)

| 요구사항 ID   | 관련 파일                                        | 마일스톤 | 상태 |
|--------------|--------------------------------------------------|----------|------|
| REQ-BE-001   | `internal/agent/modbusserver/config.go`           | M1       | Done |
| REQ-BE-002   | `internal/agent/modbusserver/config.go`           | M1       | Done |
| REQ-BE-003   | `internal/agent/modbusserver/config.go`           | M1       | Done |
| REQ-BE-004   | `internal/agent/modbusserver/handler.go`          | M2       | Done |
| REQ-BE-005   | `internal/agent/modbusserver/handler.go`          | M2       | Done |
| REQ-BE-006   | `internal/agent/modbusserver/handler.go`          | M2       | Done |
| REQ-BE-007   | `internal/agent/modbusserver/agent.go`            | M3       | Done |
| REQ-BE-008   | `internal/agent/modbusserver/agent.go`            | M3       | Done |
| REQ-BE-009   | `internal/agent/modbusserver/agent.go`            | M3       | Done |
| REQ-BE-010   | `internal/agent/modbusserver/agent.go`            | M3       | Done |
| REQ-BE-011   | `internal/agent/modbusserver/device_manager.go`   | M2       | Done |
| REQ-BE-012   | `internal/agent/modbusserver/agent.go`            | M3       | Done |
| REQ-FE-001   | `web/src/pages/agents/AgentDetailPanel.tsx`        | M4       | Done |
| REQ-FE-002   | `web/src/pages/agents/AgentDetailPanel.tsx`        | M4       | Done |
| REQ-FE-003   | `web/src/pages/agents/AgentDetailPanel.tsx`        | M4       | Done |
| REQ-FE-004   | `web/src/pages/agents/AgentDetailPanel.tsx`        | M4       | Done |
| REQ-FE-005   | `web/src/pages/agents/AgentDetailPanel.tsx`        | M4       | Done |
| REQ-FE-006   | `web/src/config/agentSchemas.ts`                   | M4       | Done |

### 7.1 추가 구현 사항 (SPEC 범위 초과)

| 기능 | 관련 파일 | 설명 |
|------|-----------|------|
| 레지스터 맵 테이블 뷰 | `AgentDetailPanel.tsx` (RegisterMapTable) | 접이식 영역별 테이블, ON/OFF 뱃지, Dec+Hex 값 표시 |
| 멀티 블록 레지스터 맵 폼 | `AgentDetailPanel.tsx` | 영역별 복수 주소 블록 추가/삭제 UI |
| 디바이스별 register_defs | `config.go` (DeviceConfig.RegisterDefs) | 3-level fallback (디바이스→글로벌→기본) |
| CLI --unit-id 플래그 | `internal/cli/modbus.go` | modbus 명령에 디바이스 단위 조작 |
| 디바이스 페이지 Modbus 추가 | `DeviceListPage.tsx` (AddDeviceDialog) | 디바이스 페이지에서 Modbus 디바이스 직접 추가 |
| YAML 예제 | `examples/agents/modbus-gateway-server.yaml` | 멀티 디바이스 설정 예시 |
