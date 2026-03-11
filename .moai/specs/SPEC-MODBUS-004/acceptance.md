# SPEC-MODBUS-004: Acceptance Criteria

## 메타데이터

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-MODBUS-004 |
| 제목 | MODBUS Reader/Writer Processing Node (`modbus_rw`) |
| 상태 | Draft |
| 관련 TAG | R-MBRW-001 ~ R-MBRW-040 |

---

## 테스트 시나리오

### TS-01: 노드 등록 및 팩토리 [R-MBRW-001]

```gherkin
Feature: modbus_rw 노드 등록

  Scenario: 빌트인 노드로 등록
    Given xflow 시스템이 초기화되면
    When 노드 Registry를 조회하면
    Then "modbus_rw" 타입이 "processing" 카테고리로 등록되어 있어야 한다
    And Registry에 12개의 빌트인 노드가 존재해야 한다

  Scenario: 팩토리로 노드 생성
    Given 유효한 NodeDef가 주어지면
    When NewModbusRWNode 팩토리를 호출하면
    Then ModbusRWNode 인스턴스가 반환되어야 한다
    And BaseNode이 임베딩되어 있어야 한다
```

### TS-02: 설정 필드 파싱 [R-MBRW-002]

```gherkin
Feature: modbus_rw 노드 설정

  Scenario: 전체 설정 파싱
    Given 다음 설정이 주어지면:
      | 필드           | 값                  |
      | agent_ref      | "modbus-server-1"   |
      | operation      | "read"              |
      | register_area  | "holding_registers" |
      | address        | 100                 |
      | count          | 10                  |
      | data_type      | "float32"           |
      | byte_order     | "big_endian"        |
      | device_id      | 1                   |
    When Configure()를 호출하면
    Then 모든 필드가 ModbusRWConfig에 정확히 파싱되어야 한다

  Scenario: 선택 필드 기본값 적용
    Given agent_ref, operation, register_area, address만 제공되면
    When Configure()를 호출하면
    Then count는 1이어야 한다
    And data_type은 "uint16"이어야 한다
    And byte_order는 "big_endian"이어야 한다
    And device_id는 1이어야 한다

  Scenario: timeout 기본값 적용
    Given timeout 설정이 없으면
    When Init()에서 timeout을 파싱하면
    Then timeout은 5초여야 한다

  Scenario: timeout 커스텀 값 적용
    Given timeout이 "10s"로 설정되면
    When Init()에서 timeout을 파싱하면
    Then timeout은 10초여야 한다
```

### TS-03: 설정 유효성 검증 [R-MBRW-003, R-MBRW-004, R-MBRW-005, R-MBRW-006, R-MBRW-033]

```gherkin
Feature: 설정 유효성 검증

  Scenario Outline: 유효하지 않은 operation 거부
    Given operation이 "<operation>"으로 설정되면
    When Configure()를 호출하면
    Then ErrInvalidOperation 에러가 반환되어야 한다

    Examples:
      | operation |
      | update    |
      | delete    |
      | ""        |

  Scenario: 읽기 전용 영역에 쓰기 설정 거부 - discrete_inputs
    Given operation이 "write"이고 register_area가 "discrete_inputs"이면
    When Configure()를 호출하면
    Then ErrReadOnlyArea 에러가 반환되어야 한다

  Scenario: 읽기 전용 영역에 쓰기 설정 거부 - input_registers
    Given operation이 "write"이고 register_area가 "input_registers"이면
    When Configure()를 호출하면
    Then ErrReadOnlyArea 에러가 반환되어야 한다

  Scenario: boolean 영역에서 data_type 무시
    Given register_area가 "coils"이고 data_type이 "float32"이면
    When Configure()를 호출하면
    Then 설정 에러 없이 성공해야 한다
    And data_type은 "uint16"으로 초기화되거나 무시되어야 한다

  Scenario Outline: 유효하지 않은 register_area 거부
    Given register_area가 "<area>"로 설정되면
    When Configure()를 호출하면
    Then ErrInvalidRegisterArea 에러가 반환되어야 한다

    Examples:
      | area              |
      | registers         |
      | invalid_area      |
      | ""                |

  Scenario: count가 0이면 거부
    Given count가 0으로 설정되면
    When Configure()를 호출하면
    Then ErrInvalidCount 에러가 반환되어야 한다

  Scenario: address가 음수이면 거부
    Given address가 음수로 제공되면
    When Configure()를 호출하면
    Then ErrInvalidAddress 에러가 반환되어야 한다
```

### TS-04: Agent 해석 및 타입 감지 [R-MBRW-007, R-MBRW-008, R-MBRW-009]

```gherkin
Feature: Agent 해석 및 타입 감지

  Scenario: Server Agent 자동 감지
    Given agent_ref가 MODBUS Server Agent를 참조하면
    When Init()에서 Agent를 resolve하면
    Then AgentResolver.ResolveAgent()로 AgentTransport를 획득해야 한다
    And AgentAccessor.UnderlyingAgent()로 원본 Agent를 획득해야 한다
    And agentType은 "server"여야 한다

  Scenario: Client Agent 자동 감지
    Given agent_ref가 MODBUS Client Agent를 참조하면
    When Init()에서 Agent를 resolve하면
    Then AgentResolver.ResolveAgent()로 AgentTransport를 획득해야 한다
    And AgentAccessor.UnderlyingAgent()로 원본 Agent를 획득해야 한다
    And agentType은 "client"여야 한다

  Scenario: 비-MODBUS Agent 연결 거부
    Given agent_ref가 MQTT Agent를 참조하면
    When Init()에서 Agent를 resolve하면
    Then ErrUnsupportedAgentType 에러가 반환되어야 한다

  Scenario: Agent resolve 실패
    Given agent_ref가 존재하지 않는 Agent를 참조하면
    When Init()에서 Agent를 resolve하면
    Then 초기화 에러가 반환되어야 한다
```

### TS-05: Server Agent 읽기 연산 [R-MBRW-013, R-MBRW-015, R-MBRW-016, R-MBRW-017]

```gherkin
Feature: Server Agent 읽기 연산

  Scenario: Coils 읽기
    Given agentType이 "server"이고 operation이 "read"이고 register_area가 "coils"이면
    And address가 0이고 count가 8이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_coils", address=0, count=8을 전달해야 한다
    And 출력 메시지에 result, register_area="coils", address=0, count=8, agent_type="server"가 포함되어야 한다

  Scenario: Discrete Inputs 읽기
    Given agentType이 "server"이고 register_area가 "discrete_inputs"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_discrete_inputs"를 전달해야 한다

  Scenario: Holding Registers 읽기 (uint16)
    Given agentType이 "server"이고 register_area가 "holding_registers"이고 data_type이 "uint16"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_holding_registers"를 전달해야 한다

  Scenario: Holding Registers 타입 변환 읽기 (float32)
    Given agentType이 "server"이고 register_area가 "holding_registers"이고 data_type이 "float32"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_register_typed", data_type="float32", byte_order="big_endian"을 전달해야 한다

  Scenario: Input Registers 읽기 (uint16)
    Given agentType이 "server"이고 register_area가 "input_registers"이고 data_type이 "uint16"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_input_registers"를 전달해야 한다

  Scenario: Input Registers 타입 변환 읽기 (int32)
    Given agentType이 "server"이고 register_area가 "input_registers"이고 data_type이 "int32"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="get_register_typed", data_type="int32"을 전달해야 한다

  Scenario: 원본 payload 보존 (읽기)
    Given 입력 메시지의 payload에 {"source": "sensor-1", "timestamp": 1234567890}이 있으면
    When 읽기 연산이 성공적으로 완료되면
    Then 출력 메시지의 payload에 source="sensor-1"과 timestamp=1234567890이 보존되어야 한다
    And result, register_area 등 읽기 결과가 추가되어야 한다
```

### TS-06: Client Agent 읽기 연산 [R-MBRW-014, R-MBRW-018, R-MBRW-019, R-MBRW-020]

```gherkin
Feature: Client Agent 읽기 연산

  Scenario Outline: register_area에 따른 function_code 매핑
    Given agentType이 "client"이고 operation이 "read"이면
    And register_area가 "<area>"이면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="read_registers", function_code=<fc>를 전달해야 한다

    Examples:
      | area               | fc |
      | coils              | 1  |
      | discrete_inputs    | 2  |
      | holding_registers  | 3  |
      | input_registers    | 4  |

  Scenario: 기본 read mode (force)
    Given agentType이 "client"이고 operation이 "read"이면
    And 입력 메시지의 payload에 read_mode 키가 없으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 mode="force"를 전달해야 한다

  Scenario: read_mode 메시지 오버라이드
    Given agentType이 "client"이고 operation이 "read"이면
    And 입력 메시지의 payload에 read_mode="cached"가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 mode="cached"를 전달해야 한다

  Scenario: device_id 메시지 오버라이드
    Given 노드 설정의 device_id가 1이면
    And 입력 메시지의 payload에 device_id=5가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 device_id=5를 전달해야 한다

  Scenario: device_id 설정 기본값 사용
    Given 노드 설정의 device_id가 3이면
    And 입력 메시지의 payload에 device_id 키가 없으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 device_id=3을 전달해야 한다

  Scenario: 읽기 결과 출력 구조 검증
    Given agentType이 "client"이고 읽기 연산이 성공하면
    Then 출력 메시지의 payload에 다음이 포함되어야 한다:
      | 키             | 설명                        |
      | result         | 읽기 결과 값                 |
      | register_area  | 읽은 레지스터 영역            |
      | address        | 시작 주소                    |
      | count          | 읽은 레지스터 수              |
      | data_type      | 적용된 데이터 타입            |
      | agent_type     | "client"                    |
```

### TS-07: Server Agent 쓰기 연산 [R-MBRW-021, R-MBRW-023, R-MBRW-024, R-MBRW-026, R-MBRW-027, R-MBRW-028]

```gherkin
Feature: Server Agent 쓰기 연산

  Scenario: 단일 코일 쓰기
    Given agentType이 "server"이고 operation이 "write"이면
    And register_area가 "coils"이고 count가 1이면
    And 입력 메시지의 payload에 value=true가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="set_coil", address=<address>, value=true를 전달해야 한다

  Scenario: 다중 코일 쓰기
    Given agentType이 "server"이고 operation이 "write"이면
    And register_area가 "coils"이고 count가 4이면
    And 입력 메시지의 payload에 values=[true, false, true, false]가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="set_coils", address=<address>, values=[true, false, true, false]를 전달해야 한다

  Scenario: 단일 홀딩 레지스터 쓰기
    Given agentType이 "server"이고 operation이 "write"이면
    And register_area가 "holding_registers"이고 count가 1이면
    And 입력 메시지의 payload에 value=42가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="set_register", address=<address>, value=42를 전달해야 한다

  Scenario: 다중 홀딩 레지스터 쓰기
    Given agentType이 "server"이고 operation이 "write"이면
    And register_area가 "holding_registers"이고 count가 3이면
    And 입력 메시지의 payload에 values=[100, 200, 300]이 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="set_registers", address=<address>, values=[100, 200, 300]을 전달해야 한다

  Scenario: data_type 파라미터 전달
    Given agentType이 "server"이고 data_type이 "float32"이고 byte_order가 "big_endian"이면
    When 쓰기 명령을 Agent에 전달하면
    Then Process() 메시지에 data_type="float32"과 byte_order="big_endian"이 포함되어야 한다

  Scenario: 쓰기 확인 출력 구조
    Given agentType이 "server"이고 쓰기 연산이 성공하면
    Then 출력 메시지의 payload에 다음이 포함되어야 한다:
      | 키             | 값                          |
      | success        | true                        |
      | register_area  | 쓰기한 레지스터 영역          |
      | address        | 시작 주소                    |
      | count          | 쓰기한 레지스터 수            |
      | agent_type     | "server"                    |

  Scenario: 쓰기 시 원본 payload 보존
    Given 입력 메시지의 payload에 {"source": "controller-1", "value": 42}가 있으면
    When 쓰기 연산이 성공적으로 완료되면
    Then 출력 메시지의 payload에 source="controller-1"이 보존되어야 한다
    And success=true 등 쓰기 확인 정보가 추가되어야 한다
```

### TS-08: Client Agent 쓰기 연산 [R-MBRW-022, R-MBRW-025]

```gherkin
Feature: Client Agent 쓰기 연산

  Scenario: 단일 코일 쓰기
    Given agentType이 "client"이고 operation이 "write"이면
    And register_area가 "coils"이고 count가 1이면
    And 입력 메시지의 payload에 value=true가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="write_coil", device_id=<device_id>, address=<address>, value=true를 전달해야 한다

  Scenario: 다중 코일 쓰기
    Given agentType이 "client"이고 operation이 "write"이면
    And register_area가 "coils"이고 count가 4이면
    And 입력 메시지의 payload에 values=[true, false, true, true]가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="write_coils"를 전달해야 한다

  Scenario: 단일 홀딩 레지스터 쓰기
    Given agentType이 "client"이고 operation이 "write"이면
    And register_area가 "holding_registers"이고 count가 1이면
    And 입력 메시지의 payload에 value=42가 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="write_register", device_id=<device_id>, address=<address>, value=42를 전달해야 한다

  Scenario: 다중 홀딩 레지스터 쓰기
    Given agentType이 "client"이고 operation이 "write"이면
    And register_area가 "holding_registers"이고 count가 3이면
    And 입력 메시지의 payload에 values=[100, 200, 300]이 있으면
    When Process()에 메시지가 입력되면
    Then Agent Process()에 command="write_registers"를 전달해야 한다

  Scenario: 쓰기 값 누락 에러
    Given agentType이 "client"이고 operation이 "write"이면
    And 입력 메시지의 payload에 value도 values도 없으면
    When Process()에 메시지가 입력되면
    Then 에러 포트로 ErrMissingWriteValue 에러가 전달되어야 한다
    And 출력 포트에는 메시지가 전달되지 않아야 한다

  Scenario: device_id 오버라이드 (쓰기)
    Given 노드 설정의 device_id가 1이고
    And 입력 메시지의 payload에 device_id=7이 있으면
    When 쓰기 명령을 Agent에 전달하면
    Then Agent Process()에 device_id=7을 전달해야 한다
```

### TS-09: 에러 처리 [R-MBRW-029, R-MBRW-030, R-MBRW-031, R-MBRW-032, R-MBRW-034, R-MBRW-035]

```gherkin
Feature: 에러 처리

  Scenario: Agent Process() 에러 전달
    Given Agent Process() 호출이 에러를 반환하면
    When Process()가 에러를 수신하면
    Then 에러 포트로 원본 메시지와 에러 정보를 전달해야 한다
    And 출력 포트에는 메시지가 전달되지 않아야 한다

  Scenario: timeout 에러 처리
    Given Agent Process() 호출이 context deadline exceeded로 실패하면
    When Process()가 timeout 에러를 수신하면
    Then 에러 포트로 timeout 관련 정보를 포함한 에러 메시지를 전달해야 한다

  Scenario: Agent Paused 상태 처리
    Given Agent가 Paused 상태이면
    When Process()에 메시지가 입력되면
    Then 에러 포트로 "Agent가 일시 중지 상태" 에러를 전달해야 한다

  Scenario: Agent Stopped 상태 처리
    Given Agent가 Stopped 상태이면
    When Process()에 메시지가 입력되면
    Then 에러 포트로 "Agent가 중지 상태" 에러를 전달해야 한다

  Scenario: 패닉 복구
    Given Agent Process() 호출 중 패닉이 발생하면
    When defer recover()가 패닉을 캐치하면
    Then 에러 포트로 패닉 정보를 포함한 에러 메시지를 전달해야 한다
    And 노드는 크래시하지 않아야 한다
    And 다음 메시지를 정상적으로 처리할 수 있어야 한다

  Scenario: 읽기 전용 영역 쓰기 런타임 차단
    Given register_area가 "discrete_inputs"이고 operation이 "write"이면
    When 설정 검증 시
    Then ErrReadOnlyArea 에러가 반환되어야 한다

  Scenario: Client Agent 유효하지 않은 device_id
    Given agentType이 "client"이고 device_id가 존재하지 않는 디바이스를 참조하면
    When Process()에 읽기 명령이 전달되면
    Then 에러 포트로 "디바이스를 찾을 수 없음" 에러를 전달해야 한다
```

### TS-10: context timeout [R-MBRW-012]

```gherkin
Feature: context timeout 설정

  Scenario: 기본 timeout 적용
    Given timeout 설정이 없으면
    When Process()에서 Agent Process()를 호출하면
    Then 5초 timeout이 적용된 context를 사용해야 한다

  Scenario: 커스텀 timeout 적용
    Given timeout이 "10s"로 설정되면
    When Process()에서 Agent Process()를 호출하면
    Then 10초 timeout이 적용된 context를 사용해야 한다

  Scenario: timeout 초과 시 에러 전달
    Given timeout이 "1s"로 설정되고 Agent 응답이 2초 걸리면
    When Process()에서 Agent Process()를 호출하면
    Then context deadline exceeded 에러가 에러 포트로 전달되어야 한다
```

### TS-11: 프론트엔드 스키마 [R-MBRW-036, R-MBRW-037, R-MBRW-038]

```gherkin
Feature: 프론트엔드 노드 스키마

  Scenario: nodeSchemas.ts에 modbus_rw 등록
    Given nodeSchemas.ts 파일이 로드되면
    When NODE_SCHEMAS에서 "modbus_rw"를 조회하면
    Then NodeTypeSchema 객체가 반환되어야 한다

  Scenario: configSchema 필드 확인
    Given modbus_rw의 configSchema가 주어지면
    Then 다음 필드가 정의되어 있어야 한다:
      | 필드           | 타입           | 필수   |
      | agent_ref      | agent_select   | true   |
      | operation      | select         | true   |
      | register_area  | select         | true   |
      | address        | number         | true   |
      | count          | number         | false  |
      | data_type      | select         | false  |
      | byte_order     | select         | false  |
      | device_id      | number         | false  |

  Scenario: operation select 옵션
    Given modbus_rw의 operation 필드가 주어지면
    Then options에 "read"와 "write"가 포함되어야 한다
    And 기본값은 "read"여야 한다

  Scenario: register_area select 옵션
    Given modbus_rw의 register_area 필드가 주어지면
    Then options에 "coils", "discrete_inputs", "holding_registers", "input_registers"가 포함되어야 한다
    And 기본값은 "holding_registers"여야 한다

  Scenario: data_type select 옵션
    Given modbus_rw의 data_type 필드가 주어지면
    Then options에 "uint16", "int16", "float32", "uint32", "int32"가 포함되어야 한다
    And 기본값은 "uint16"이어야 한다

  Scenario: 기본 포트 정의
    Given modbus_rw의 defaultPorts가 주어지면
    Then 3개의 포트가 정의되어야 한다:
      | name   | direction |
      | input  | input     |
      | output | output    |
      | error  | error     |
```

### TS-12: 프론트엔드 메타데이터 [R-MBRW-039, R-MBRW-040]

```gherkin
Feature: 프론트엔드 노드 메타데이터

  Scenario: nodeTypeMeta.ts에 modbus_rw 등록
    Given nodeTypeMeta.ts 파일이 로드되면
    When NODE_TYPE_META에서 "modbus_rw"를 조회하면
    Then NodeTypeDetailMeta 객체가 반환되어야 한다

  Scenario: 메타데이터 구조 검증
    Given modbus_rw의 NodeTypeDetailMeta가 주어지면
    Then description이 비어있지 않아야 한다
    And ports 배열에 input, output, error 포트가 정의되어야 한다
    And configFields 배열에 8개 필드가 정의되어야 한다
    And configExample에 읽기 예제가 포함되어야 한다

  Scenario: configExample 읽기 예제
    Given modbus_rw의 configExample가 주어지면
    Then 다음 키가 포함되어야 한다:
      | 키             | 예제 값               |
      | agent_ref      | (에이전트 참조 값)     |
      | operation      | "read"               |
      | register_area  | "holding_registers"  |
      | address        | (숫자)               |
      | count          | (숫자)               |

  Scenario: 읽기 전용 영역 쓰기 경고 가능
    Given operation이 "write"이고 register_area가 "discrete_inputs"이면
    When 프론트엔드에서 설정을 검증하면
    Then 유효성 경고를 표시할 수 있는 정보가 메타데이터에 포함되어야 한다
```

### TS-13: 메시지 payload 보존 통합 테스트 [R-MBRW-017, R-MBRW-028]

```gherkin
Feature: 메시지 payload 보존 통합 테스트

  Scenario: 읽기 시 복잡한 payload 보존
    Given 입력 메시지의 payload가 다음과 같으면:
      ```json
      {
        "source": "plc-1",
        "timestamp": 1709020800,
        "metadata": {"zone": "A", "priority": "high"},
        "original_value": 42
      }
      ```
    When 읽기 연산이 성공적으로 완료되면
    Then 출력 메시지의 payload에 원본 키가 모두 보존되어야 한다
    And result 키가 추가되어야 한다
    And 원본 키 값이 변경되지 않아야 한다

  Scenario: 쓰기 시 복잡한 payload 보존
    Given 입력 메시지의 payload가 다음과 같으면:
      ```json
      {
        "source": "scada-1",
        "command_id": "cmd-001",
        "value": 100,
        "metadata": {"operator": "system"}
      }
      ```
    When 쓰기 연산이 성공적으로 완료되면
    Then 출력 메시지의 payload에 source, command_id, metadata가 보존되어야 한다
    And success=true가 추가되어야 한다
```

---

## Quality Gate 체크리스트 (TRUST 5)

### Tested (테스트됨)

- [ ] 모든 테스트 시나리오(TS-01 ~ TS-13)에 대한 Go 테스트가 작성되었는가
- [ ] table-driven test 패턴을 사용했는가
- [ ] Mock 인터페이스(AgentResolver, AgentTransport, AgentAccessor)가 올바르게 구현되었는가
- [ ] 테스트 커버리지 85% 이상 달성했는가
- [ ] `go test -race ./internal/node/...` 통과했는가
- [ ] 에러 케이스 테스트가 포함되어 있는가 (TS-09)
- [ ] 패닉 복구 테스트가 포함되어 있는가

### Readable (가독성)

- [ ] Go 코드가 프로젝트 코딩 컨벤션을 따르는가
- [ ] 함수/메서드 이름이 명확하고 Go 네이밍 규칙을 따르는가
- [ ] 센티널 에러가 명확한 이름으로 정의되어 있는가
- [ ] TypeScript 코드가 기존 nodeSchemas.ts / nodeTypeMeta.ts 패턴과 일관성이 있는가
- [ ] 주석이 적절히 작성되어 있는가

### Unified (일관성)

- [ ] BaseNode 임베딩 패턴이 기존 노드와 동일한가
- [ ] NodeFactory 패턴이 기존 노드와 동일한가
- [ ] Configure()/Init()/Process() 메서드 시그니처가 기존 노드와 동일한가
- [ ] 에러 포트 전달 패턴이 기존 노드(filter, mapping 등)와 동일한가
- [ ] 프론트엔드 스키마 구조가 기존 노드 스키마와 일관성이 있는가
- [ ] `go vet` 경고가 없는가

### Secured (보안)

- [ ] 입력 설정값(address, count, register_area 등)의 유효성 검증이 설정 시점에 수행되는가
- [ ] Agent Process() 호출 중 패닉이 전파되지 않는가
- [ ] context timeout이 올바르게 적용되어 무한 대기가 방지되는가
- [ ] 읽기 전용 영역에 대한 쓰기 시도가 차단되는가

### Trackable (추적 가능)

- [ ] spec.md의 모든 TAG(R-MBRW-001 ~ R-MBRW-040)이 테스트 시나리오에 매핑되어 있는가
- [ ] plan.md의 모든 마일스톤(M1 ~ M7)이 구현 완료되었는가
- [ ] 코드 변경이 Conventional Commit 메시지로 커밋되는가
- [ ] Registry 등록이 올바르게 수행되어 12번째 빌트인 노드로 확인 가능한가

---

## TAG 매핑 (요구사항 - 테스트 시나리오)

| TAG | 테스트 시나리오 |
|-----|---------------|
| R-MBRW-001 | TS-01 |
| R-MBRW-002 | TS-02 |
| R-MBRW-003 | TS-03 |
| R-MBRW-004 | TS-03 |
| R-MBRW-005 | TS-03 |
| R-MBRW-006 | TS-03 |
| R-MBRW-007 | TS-04 |
| R-MBRW-008 | TS-04 |
| R-MBRW-009 | TS-04 |
| R-MBRW-010 | TS-05, TS-06 |
| R-MBRW-011 | TS-09 |
| R-MBRW-012 | TS-10 |
| R-MBRW-013 | TS-05 |
| R-MBRW-014 | TS-06 |
| R-MBRW-015 | TS-05, TS-06 |
| R-MBRW-016 | TS-05 |
| R-MBRW-017 | TS-05, TS-13 |
| R-MBRW-018 | TS-06 |
| R-MBRW-019 | TS-06 |
| R-MBRW-020 | TS-06 |
| R-MBRW-021 | TS-07 |
| R-MBRW-022 | TS-08 |
| R-MBRW-023 | TS-07 |
| R-MBRW-024 | TS-07 |
| R-MBRW-025 | TS-08 |
| R-MBRW-026 | TS-07 |
| R-MBRW-027 | TS-07 |
| R-MBRW-028 | TS-07, TS-13 |
| R-MBRW-029 | TS-09 |
| R-MBRW-030 | TS-09 |
| R-MBRW-031 | TS-09 |
| R-MBRW-032 | TS-09 |
| R-MBRW-033 | TS-03 |
| R-MBRW-034 | TS-03, TS-09 |
| R-MBRW-035 | TS-09 |
| R-MBRW-036 | TS-11 |
| R-MBRW-037 | TS-11 |
| R-MBRW-038 | TS-11 |
| R-MBRW-039 | TS-12 |
| R-MBRW-040 | TS-12 |

---

## Definition of Done

- [ ] `internal/node/modbus_rw.go` 구현 완료 (M1 ~ M5)
- [ ] `internal/node/registry.go`에 modbus_rw 등록 완료 (M1)
- [ ] `internal/node/modbus_rw_test.go` 테스트 작성 완료 (M7)
- [ ] `web/src/config/nodeSchemas.ts`에 modbus_rw 스키마 추가 완료 (M6)
- [ ] `web/src/pages/nodes/nodeTypeMeta.ts`에 modbus_rw 메타데이터 추가 완료 (M6)
- [ ] `go test -race ./internal/node/...` 통과
- [ ] `go vet ./internal/node/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상
- [ ] TRUST 5 Quality Gate 전체 통과
- [ ] 모든 TAG(R-MBRW-001 ~ R-MBRW-040)이 테스트 시나리오에 매핑 확인

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-11*
*작성: xtra*
