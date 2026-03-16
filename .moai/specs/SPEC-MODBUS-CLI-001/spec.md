---
id: SPEC-MODBUS-CLI-001
version: "1.1.0"
status: completed
created: "2026-03-16"
updated: "2026-03-16"
author: xtra
priority: P1
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-03-16 | xtra | 초기 SPEC 작성 |
| 1.1.0 | 2026-03-16 | xtra | 구현 완료 + interactive 모드 플래그 리셋 수정 |

# SPEC-MODBUS-CLI-001: MODBUS CLI 전용 명령어

## 개요

xflow CLI에 `modbus` 서브커맨드를 추가하여, MODBUS Client/Server 에이전트의 레지스터 읽기/쓰기를 간편하게 수행할 수 있는 전용 CLI 인터페이스를 제공한다. 기존 `xflow agent exec` 경로의 번거로운 사용법을 대체하는 인체공학적(ergonomic) 단축 명령어를 구현한다.

## 배경 및 문제

- 현재 MODBUS 에이전트 제어는 `xflow agent exec <agent> <command> key=value ...` 형식으로만 가능
- MODBUS 명령어가 에이전트 타입(Client/Server)에 따라 다름 (Client: `read_registers`, Server: `get_holding_registers`)
- 사용자가 에이전트 타입을 알고 올바른 Process 명령을 선택해야 하는 부담
- 레지스터 영역 이름이 길고 직관적이지 않음 (`get_holding_registers` vs `hr`)
- 출력이 범용 JSON이라 레지스터 값 확인이 불편

## 대상 사용자

- **IoT 시스템 엔지니어**: MODBUS 디바이스 레지스터를 CLI로 빠르게 확인/제어
- **빌딩 관리자**: HVAC, 전력 계측기 등 MODBUS 장비 상태 점검
- **DevOps 엔지니어**: 스크립트/자동화에서 MODBUS 레지스터 일괄 읽기/쓰기

---

## Module 1: modbus 서브커맨드 구조

**Package**: `internal/cli/modbus.go`

### Environment

- xflow CLI는 Cobra + Viper 기반 (Go 1.23+)
- 모든 CLI 명령어는 REST API를 통해 xflowd 서버와 통신
- Agent 식별은 UUID 또는 에이전트 이름으로 가능 (`resolveAgentID`)
- MODBUS Client Agent 타입: `modbus-tcp`
- MODBUS Server Agent 타입: `modbus-tcp-server`

### Assumptions

- 에이전트 상세 조회 API (`GET /api/v1/agents/{id}`)가 `type` 필드를 반환
- Client 에이전트와 Server 에이전트의 Process 명령 세트가 서로 다름
- `key=value` 파싱은 기존 `parseParamValue` 함수를 재사용

### Requirements

- **R1.1** (Ubiquitous): 시스템은 **항상** `xflow modbus` 서브커맨드를 통해 MODBUS 에이전트 제어 기능을 제공해야 한다
- **R1.2** (Event-Driven): **WHEN** 사용자가 `xflow modbus read <agent>` 명령을 실행하면 **THEN** 대상 에이전트 타입을 자동 감지하여 적절한 Process 명령(Client: `read_registers`, Server: `get_holding_registers` 등)을 전송해야 한다
- **R1.3** (Event-Driven): **WHEN** 사용자가 `xflow modbus write <agent>` 명령을 실행하면 **THEN** 대상 에이전트 타입을 자동 감지하여 적절한 쓰기 명령(Client: `write_register`/`write_registers`, Server: `set_register`/`set_registers`)을 전송해야 한다
- **R1.4** (Ubiquitous): 시스템은 **항상** 레지스터 영역 단축 별칭을 지원해야 한다
- **R1.5** (Unwanted): 시스템은 존재하지 않거나 MODBUS 타입이 아닌 에이전트에 대해 명령을 전송**하지 않아야 한다**

### Specifications

**S1.1 커맨드 트리**:

```
xflow modbus
  +-- read <agent> [flags]       # 레지스터 읽기
  +-- write <agent> [flags]      # 레지스터 쓰기
  +-- status <agent>             # 에이전트 상태 조회
  +-- map <agent>                # 레지스터 맵 조회 (Server 전용)
```

**S1.2 `newModbusCmd` 함수**:
- `root.go`의 `NewRootCmd()`에 `rootCmd.AddCommand(newModbusCmd(&client))` 등록
- `newModbusCmd`는 `*cobra.Command`를 반환하며, 하위에 `read`, `write`, `status`, `map` 서브커맨드를 등록

**S1.3 에이전트 식별**:
- 첫 번째 positional 인자로 에이전트 ID 또는 이름을 받음
- 기존 `resolveAgentID` 함수를 재사용하여 UUID로 변환
- `--name` 플래그 지원 (기존 `agent exec` 패턴 동일)

---

## Module 2: 에이전트 타입 자동 감지

**Package**: `internal/cli/modbus.go`

### Environment

- `GET /api/v1/agents/{id}` 응답에 `type` 필드가 포함
- Client: `"type": "modbus-tcp"`, Server: `"type": "modbus-tcp-server"`

### Requirements

- **R2.1** (Event-Driven): **WHEN** modbus 서브커맨드가 실행되면 **THEN** `GET /api/v1/agents/{id}` API를 호출하여 에이전트 타입을 확인해야 한다
- **R2.2** (Unwanted): 에이전트 타입이 `modbus-tcp` 또는 `modbus-tcp-server`가 아닌 경우 시스템은 명령을 실행**하지 않아야 하며** 에러 메시지를 출력해야 한다
- **R2.3** (State-Driven): **IF** 에이전트 타입이 `modbus-tcp` (Client)이면 **THEN** Client 전용 Process 명령 세트를 사용해야 한다
- **R2.4** (State-Driven): **IF** 에이전트 타입이 `modbus-tcp-server` (Server)이면 **THEN** Server 전용 Process 명령 세트를 사용해야 한다

### Specifications

**S2.1 타입 감지 함수**:

```
func resolveModbusAgentType(client *Client, agentID string) (string, error)
```

- `GET /api/v1/agents/{id}` 호출
- 응답의 `type` 필드 확인
- `"modbus-tcp"` -> `"client"` 반환
- `"modbus-tcp-server"` -> `"server"` 반환
- 그 외 -> 에러 반환: `"에이전트 '%s'은(는) MODBUS 타입이 아닙니다 (type: %s)"`

**S2.2 명령어 매핑 테이블**:

| CLI 명령 | Client (`modbus-tcp`) | Server (`modbus-tcp-server`) |
|----------|----------------------|------------------------------|
| `read` (register=holding) | `read_registers` + `register=holding` | `get_holding_registers` |
| `read` (register=input) | `read_registers` + `register=input` | `get_input_registers` |
| `read` (register=coils) | `read_registers` + `register=coils` | `get_coils` |
| `read` (register=discrete) | `read_registers` + `register=discrete` | `get_discrete_inputs` |
| `write` (register=holding) | `write_register` / `write_registers` | `set_register` / `set_registers` |
| `write` (register=coils) | `write_coil` / `write_coils` | `set_coil` / `set_coils` |
| `status` | `get_status` | `get_status` |
| `map` | (미지원 - 에러) | `get_map` |

---

## Module 3: read 명령어

**Package**: `internal/cli/modbus.go`

### Environment

- Client Agent의 `read_registers`는 `device_id` 파라미터가 필수
- Server Agent의 `get_holding_registers`는 `address`와 `quantity` 파라미터가 필수

### Requirements

- **R3.1** (Event-Driven): **WHEN** `xflow modbus read <agent>` 명령이 실행되면 **THEN** 지정된 레지스터 영역의 값을 읽어 테이블 형식으로 출력해야 한다
- **R3.2** (Ubiquitous): 시스템은 **항상** 레지스터 영역 별칭을 정규 이름으로 변환해야 한다
- **R3.3** (Optional): **가능하면** 데이터 타입(`--type`)을 지정하여 레지스터 값을 해당 타입으로 해석한 결과도 함께 출력해야 한다
- **R3.4** (Complex): **IF** 에이전트가 Client 타입이고 **AND WHEN** `--device` 플래그가 생략되면 **THEN** 에러를 반환해야 한다

### Specifications

**S3.1 read 커맨드 사용법**:

```
xflow modbus read <agent> [flags]

Flags:
  --register, -r   string   레지스터 영역 (holding|hr, input|ir, coils|c, discrete|di) [기본: holding]
  --address, -a    int      시작 주소 [기본: 0]
  --quantity, -q   int      읽을 레지스터 수 [기본: 1]
  --type, -t       string   데이터 타입 (uint16, int16, uint32, int32, float32, float64)
  --device, -d     string   디바이스 ID (Client 에이전트 전용, 필수)
  --force          bool     캐시 무시하고 직접 읽기 (Client 에이전트 전용)
  --name           string   에이전트 이름으로 지정
```

**S3.2 레지스터 영역 별칭 매핑**:

| 별칭 | 정규 이름 |
|------|-----------|
| `holding`, `hr` | `holding_registers` |
| `input`, `ir` | `input_registers` |
| `coils`, `c` | `coils` |
| `discrete`, `di` | `discrete_inputs` |

**S3.3 예시 사용법**:

```bash
# Server 에이전트에서 holding register 읽기
xflow modbus read my-server -r hr -a 100 -q 10

# Client 에이전트에서 input register를 float32로 읽기
xflow modbus read my-client -d 1 -r input -a 0 -q 4 -t float32

# 단축: 기본값 활용 (holding, address=0, quantity=1)
xflow modbus read my-server
```

---

## Module 4: write 명령어

**Package**: `internal/cli/modbus.go`

### Environment

- Client Agent: `write_register` (단일), `write_registers` (다중), `write_coil`/`write_coils`
- Server Agent: `set_register` (단일), `set_registers` (다중), `set_coil`/`set_coils`
- 다중 값 쓰기는 쉼표로 구분된 값 목록으로 전달

### Requirements

- **R4.1** (Event-Driven): **WHEN** `xflow modbus write <agent>` 명령이 실행되면 **THEN** 지정된 레지스터에 값을 쓰고 결과를 출력해야 한다
- **R4.2** (State-Driven): **IF** `--values` 플래그에 쉼표로 구분된 다중 값이 제공되면 **THEN** 다중 쓰기 명령(`write_registers`/`set_registers`)을 사용해야 한다
- **R4.3** (State-Driven): **IF** `--values` 플래그에 단일 값이 제공되면 **THEN** 단일 쓰기 명령(`write_register`/`set_register`)을 사용해야 한다
- **R4.4** (Complex): **IF** 에이전트가 Client 타입이고 **AND WHEN** `--device` 플래그가 생략되면 **THEN** 에러를 반환해야 한다

### Specifications

**S4.1 write 커맨드 사용법**:

```
xflow modbus write <agent> [flags]

Flags:
  --register, -r   string   레지스터 영역 (holding|hr, coils|c) [기본: holding]
  --address, -a    int      시작 주소 [필수]
  --values, -v     string   쓸 값 (쉼표 구분, 예: "100" 또는 "100,200,300") [필수]
  --type, -t       string   데이터 타입 (uint16, int16, uint32, int32, float32, float64)
  --device, -d     string   디바이스 ID (Client 에이전트 전용, 필수)
  --name           string   에이전트 이름으로 지정
```

**S4.2 값 파싱 규칙**:
- 단일 값: `--values "100"` -> `value: 100`
- 다중 값: `--values "100,200,300"` -> `values: [100, 200, 300]`
- 부울 값 (coils): `--values "true"` 또는 `--values "true,false,true"`
- 소수 값 (float32 등): `--values "3.14"` 또는 `--values "3.14,2.71"`

**S4.3 예시 사용법**:

```bash
# Server 에이전트에 holding register 쓰기
xflow modbus write my-server -a 100 -v 42

# Client 에이전트에 다중 레지스터 쓰기 (float32)
xflow modbus write my-client -d 1 -a 0 -v "3.14,2.71" -t float32

# Server 에이전트에 coil 쓰기
xflow modbus write my-server -r coils -a 0 -v true
```

---

## Module 5: status 및 map 명령어

**Package**: `internal/cli/modbus.go`

### Requirements

- **R5.1** (Event-Driven): **WHEN** `xflow modbus status <agent>` 명령이 실행되면 **THEN** 에이전트의 `get_status` Process 명령 결과를 출력해야 한다
- **R5.2** (Event-Driven): **WHEN** `xflow modbus map <agent>` 명령이 실행되면 **THEN** Server 에이전트의 `get_map` Process 명령 결과를 출력해야 한다
- **R5.3** (Unwanted): Client 에이전트에 대해 `map` 명령을 실행하면 시스템은 명령을 실행**하지 않아야 하며** "`map` 명령은 Server 에이전트에서만 사용할 수 있습니다" 에러를 출력해야 한다

### Specifications

**S5.1 status 커맨드 사용법**:

```
xflow modbus status <agent> [--name <name>]
```

**S5.2 map 커맨드 사용법**:

```
xflow modbus map <agent> [--name <name>]
```

---

## Module 6: 출력 포매팅

**Package**: `internal/cli/modbus.go`

### Requirements

- **R6.1** (Ubiquitous): 시스템은 **항상** `--format` 글로벌 플래그에 따라 출력 형식(table, json, yaml)을 결정해야 한다
- **R6.2** (State-Driven): **IF** 출력 형식이 `table`이면 **THEN** 레지스터 값을 가독성 높은 테이블 형태로 출력해야 한다
- **R6.3** (Optional): **가능하면** typed_values가 응답에 포함된 경우 raw 값과 함께 타입 변환된 값도 테이블에 표시해야 한다

### Specifications

**S6.1 read 테이블 출력 형식**:

```
ADDRESS | RAW VALUE | TYPED VALUE | TYPE
--------|-----------|-------------|------
100     | 16384     | 3.14        | float32
102     | 0         | 0.00        | float32
```

**S6.2 write 테이블 출력 형식**:

```
RESULT  | COMMAND         | ADDRESS | QUANTITY
--------|-----------------|---------|----------
OK      | write_registers | 100     | 2
```

**S6.3 기존 출력 함수 재사용**:
- JSON/YAML: `PrintResult` 함수 사용
- Table: `DetailFormatter` 또는 커스텀 테이블 헤더/로우 함수 사용

---

## 제약 조건

- **C1**: 새로운 API 엔드포인트를 추가하지 않음 - 기존 `POST /api/v1/agents/{id}/exec` 엔드포인트만 사용
- **C2**: `internal/cli/modbus.go` 단일 파일에 모든 modbus 서브커맨드를 구현
- **C3**: 기존 `agent exec` 명령어와 동일한 API 경로를 사용하므로 서버 측 변경 없음
- **C4**: Go 표준 라이브러리 + Cobra/Viper 외 추가 의존성 없음

## 추적성 태그

- **REF-PRODUCT**: product.md - "CLI Tool - xflow" 섹션, "MODBUS/TCP" Agent
- **REF-TECH**: tech.md - "CLI: Cobra + Viper" 섹션, "MODBUS/TCP" 섹션
- **REF-AGENT-CLIENT**: `internal/agent/modbus/agent.go:570-595` - Client Process 명령 분기
- **REF-AGENT-SERVER**: `internal/agent/modbusserver/agent.go:233-275` - Server Process 명령 분기
- **REF-CLI-EXEC**: `internal/cli/agent.go:690-778` - 기존 `agent exec` 구현 패턴
- **REF-CLI-ROOT**: `internal/cli/root.go:79-89` - 서브커맨드 등록 패턴
