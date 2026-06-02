# SPEC-MODBUS-005: 인수 기준 (Acceptance Criteria)

---
id: SPEC-MODBUS-005
version: 1.0.0
type: acceptance
---

## 1. 백엔드 - 설정 구조 (M1)

### AC-001: 다중 디바이스 설정 파싱

```gherkin
Given Modbus Server 에이전트 설정에 devices 배열이 포함되어 있을 때
  And devices 배열에 unit_id가 1, 2, 3인 세 개의 디바이스가 정의되어 있을 때
When parseModbusServerConfig()를 호출하면
Then ModbusServerConfig.Devices의 길이가 3이어야 한다
  And 각 DeviceConfig의 UnitID가 1, 2, 3이어야 한다
  And 각 DeviceConfig의 RegisterMap이 올바르게 파싱되어야 한다
```

### AC-002: 하위 호환 자동 변환

```gherkin
Given Modbus Server 에이전트 설정에 단일 unit_id(1)와 register_map이 최상위에 존재할 때
  And devices 배열이 없을 때
When parseModbusServerConfig()를 호출하면
Then ModbusServerConfig.Devices의 길이가 1이어야 한다
  And Devices[0].UnitID가 1이어야 한다
  And Devices[0].RegisterMap이 원본 register_map과 동일해야 한다
```

### AC-003: Unit ID 고유성 검증

```gherkin
Given Modbus Server 에이전트 설정에 devices 배열이 포함되어 있을 때
  And 두 디바이스가 동일한 unit_id(5)를 가지고 있을 때
When parseModbusServerConfig()를 호출하면
Then 설정 오류가 반환되어야 한다
  And 오류 메시지에 중복 unit_id 정보가 포함되어야 한다
```

### AC-004: Unit ID 범위 검증

```gherkin
Given Modbus Server 에이전트 설정에 devices 배열이 포함되어 있을 때
  And 하나의 디바이스 unit_id가 0이거나 248 이상일 때
When parseModbusServerConfig()를 호출하면
Then 설정 오류가 반환되어야 한다
  And 오류 메시지에 유효 범위(1-247) 정보가 포함되어야 한다
```

---

## 2. 백엔드 - 요청 라우팅 (M2)

### AC-005: Unit ID 기반 라우팅

```gherkin
Given 세 개의 디바이스(unit_id: 1, 2, 3)가 등록된 서버 에이전트가 실행 중일 때
When unit_id 2로 FC03 (Read Holding Registers) 요청을 전송하면
Then 디바이스 2의 RegisterMap에서 값을 읽어 응답해야 한다
  And 디바이스 1, 3의 RegisterMap은 접근되지 않아야 한다
```

### AC-006: 미등록 Unit ID 무시

```gherkin
Given 디바이스(unit_id: 1, 2)가 등록된 서버 에이전트가 실행 중일 때
When unit_id 99로 Modbus 요청을 전송하면
Then 서버는 응답을 보내지 않아야 한다
  And warn 레벨 로그에 미등록 unit_id 정보가 기록되어야 한다
```

### AC-007: Broadcast 쓰기 전파

```gherkin
Given 디바이스(unit_id: 1, 2, 3)가 등록된 서버 에이전트가 실행 중일 때
  And 각 디바이스의 Holding Register 주소 0 값이 0일 때
When unit_id 0으로 FC06 (Write Single Register) 요청 (주소 0, 값 100)을 전송하면
Then 세 디바이스 모두의 Holding Register 주소 0 값이 100으로 변경되어야 한다
```

### AC-008: Broadcast 읽기 첫 번째 디바이스 응답

```gherkin
Given 디바이스(unit_id: 1, 2)가 등록된 서버 에이전트가 실행 중일 때
  And 디바이스 1의 Holding Register 주소 0 값이 42일 때
  And 디바이스 2의 Holding Register 주소 0 값이 99일 때
When unit_id 0으로 FC03 (Read Holding Registers) 요청 (주소 0, 수량 1)을 전송하면
Then 응답 값이 42 (첫 번째 디바이스)이어야 한다
```

### AC-009: 디바이스별 독립 상태

```gherkin
Given 디바이스(unit_id: 1, 2)가 등록된 서버 에이전트가 실행 중일 때
When unit_id 1로 FC06 (Write Single Register) 요청 (주소 0, 값 55)을 전송하면
Then 디바이스 1의 Holding Register 주소 0 값이 55이어야 한다
  And 디바이스 2의 Holding Register 주소 0 값은 변경되지 않아야 한다
```

### AC-010: 동시 접근 안전성

```gherkin
Given 다중 디바이스가 등록된 서버 에이전트가 실행 중일 때
When 100개의 고루틴에서 동시에 서로 다른 unit_id로 읽기/쓰기 요청을 전송하면
Then 모든 요청이 정상 처리되어야 한다
  And 데이터 레이스가 발생하지 않아야 한다 (-race 플래그 검증)
```

---

## 3. 백엔드 - Exec 명령 (M3)

### AC-011: list_devices 명령

```gherkin
Given 디바이스(unit_id: 1 "Temp Sensor", unit_id: 2 "Pressure Sensor")가 등록된 에이전트가 실행 중일 때
When list_devices exec 명령을 실행하면
Then 응답 JSON에 devices 배열이 포함되어야 한다
  And devices 배열의 길이가 2이어야 한다
  And 각 디바이스에 unit_id, name, registers (coils, discrete_inputs, holding_registers, input_registers 개수), status, created_at이 포함되어야 한다
  And total 필드가 2이어야 한다
```

### AC-012: add_device 명령 성공

```gherkin
Given 디바이스(unit_id: 1)가 등록된 에이전트가 실행 중일 때
When add_device exec 명령을 unit_id 3, name "New Device", register_map {holding_registers: [{start_address: 0, count: 10}]}와 함께 실행하면
Then 응답 JSON의 success가 true이어야 한다
  And list_devices 실행 시 디바이스가 2개로 증가해야 한다
  And unit_id 3으로 Modbus 요청을 전송하면 정상 응답해야 한다
```

### AC-013: add_device 중복 Unit ID 오류

```gherkin
Given 디바이스(unit_id: 1)가 등록된 에이전트가 실행 중일 때
When add_device exec 명령을 unit_id 1로 실행하면
Then 응답 JSON의 success가 false이어야 한다
  And error 메시지에 중복 unit_id 정보가 포함되어야 한다
```

### AC-014: remove_device 명령 성공

```gherkin
Given 디바이스(unit_id: 1, 2)가 등록된 에이전트가 실행 중일 때
When remove_device exec 명령을 unit_id 2와 함께 실행하면
Then 응답 JSON의 success가 true이어야 한다
  And list_devices 실행 시 디바이스가 1개로 감소해야 한다
  And unit_id 2로 Modbus 요청을 전송하면 응답이 없어야 한다
```

### AC-015: remove_device 마지막 디바이스 보호

```gherkin
Given 디바이스(unit_id: 1) 하나만 등록된 에이전트가 실행 중일 때
When remove_device exec 명령을 unit_id 1과 함께 실행하면
Then 응답 JSON의 success가 false이어야 한다
  And error 메시지에 "cannot remove last device" 정보가 포함되어야 한다
  And 디바이스는 여전히 등록되어 있어야 한다
```

### AC-016: get_device_status 명령

```gherkin
Given 디바이스(unit_id: 1)가 등록되고 일부 요청이 처리된 에이전트가 실행 중일 때
When get_device_status exec 명령을 unit_id 1과 함께 실행하면
Then 응답 JSON에 레지스터 값, 요청 통계, 생성 시각이 포함되어야 한다
```

### AC-017: AgentStats에 device_count 포함

```gherkin
Given 디바이스가 3개 등록된 에이전트가 실행 중일 때
When 에이전트 통계를 조회하면
Then Stats.Extra에 "device_count" 키가 존재해야 한다
  And 값이 3이어야 한다
```

---

## 4. 프론트엔드 - Devices 탭 (M4)

### AC-018: Modbus Server Devices 탭 표시

```gherkin
Given modbus-tcp-server 타입 에이전트의 상세 패널이 열려 있을 때
When Devices 탭을 클릭하면
Then ModbusDevicesTab 컴포넌트가 렌더링되어야 한다
  And list_devices exec 명령이 호출되어야 한다
  And 디바이스 카드 목록이 표시되어야 한다
```

### AC-019: 디바이스 카드 정보 표시

```gherkin
Given Modbus Server Devices 탭에 디바이스 목록이 로드된 상태일 때
When 디바이스 카드를 확인하면
Then 각 카드에 Unit ID, 디바이스 이름, 레지스터 영역별 개수가 표시되어야 한다
  And 상태 배지 (active/inactive)가 표시되어야 한다
```

### AC-020: 디바이스 상세 조회

```gherkin
Given Modbus Server Devices 탭에 디바이스 목록이 표시된 상태일 때
When 특정 디바이스 카드를 클릭하면
Then get_device_status exec 명령이 호출되어야 한다
  And 해당 디바이스의 레지스터 맵 상세 정보가 표시되어야 한다
  And 요청 통계가 표시되어야 한다
```

### AC-021: 디바이스 추가 UI

```gherkin
Given Modbus Server Devices 탭이 열려 있을 때
When "디바이스 추가" 버튼을 클릭하면
Then Unit ID (1-247), 이름, 레지스터 맵 설정 입력 폼이 표시되어야 한다
When 유효한 값을 입력하고 확인 버튼을 클릭하면
Then add_device exec 명령이 호출되어야 한다
  And 성공 시 디바이스 목록이 갱신되어야 한다
```

### AC-022: 디바이스 삭제 UI

```gherkin
Given Modbus Server Devices 탭에 디바이스가 2개 이상 표시된 상태일 때
When 디바이스의 삭제 버튼을 클릭하면
Then 삭제 확인 다이얼로그가 표시되어야 한다
When 확인을 클릭하면
Then remove_device exec 명령이 호출되어야 한다
  And 성공 시 디바이스 목록이 갱신되어야 한다
```

### AC-023: 마지막 디바이스 삭제 방지

```gherkin
Given Modbus Server Devices 탭에 디바이스가 1개만 표시된 상태일 때
When 해당 디바이스 카드를 확인하면
Then 삭제 버튼이 비활성화(disabled) 상태이어야 한다
```

### AC-024: Samsung NASA Devices 탭 호환성

```gherkin
Given samsung_hvacr01 타입 에이전트의 상세 패널이 열려 있을 때
When Devices 탭을 클릭하면
Then 기존 NasaDevicesTab 컴포넌트가 렌더링되어야 한다
  And ModbusDevicesTab이 렌더링되지 않아야 한다
```

### AC-025: agentSchemas 업데이트

```gherkin
Given agentSchemas.ts의 MODBUS_TCP_SERVER_FIELDS가 업데이트된 상태일 때
When modbus-tcp-server 타입 에이전트 생성 폼을 열면
Then 단일 unit_id 필드 대신 devices 설정 UI가 표시되어야 한다
  And 단일 register_map 필드가 제거되어야 한다
```

---

## 5. 통합 테스트 (M5)

### AC-026: 설정 -> 라우팅 -> Exec E2E

```gherkin
Given 다중 디바이스 YAML 설정 파일이 준비되어 있을 때
When 해당 설정으로 Modbus Server 에이전트를 시작하면
Then 모든 디바이스가 정상 등록되어야 한다
  And 각 unit_id로 Modbus 요청이 올바르게 라우팅되어야 한다
  And list_devices exec 명령이 모든 디바이스를 반환해야 한다
  And add_device/remove_device로 런타임 디바이스 관리가 가능해야 한다
```

### AC-027: 단일 디바이스 설정 회귀 테스트

```gherkin
Given 기존 단일 디바이스 설정(unit_id + register_map) 파일이 있을 때
When 해당 설정으로 Modbus Server 에이전트를 시작하면
Then 에이전트가 정상 시작되어야 한다
  And 기존과 동일하게 해당 unit_id로 Modbus 요청을 처리해야 한다
  And list_devices 실행 시 디바이스 1개가 반환되어야 한다
```

---

## 6. 품질 게이트 (Quality Gates)

| 항목 | 기준 | 검증 방법 |
|------|------|-----------|
| 백엔드 테스트 커버리지 | >= 85% | `go test -cover ./internal/agent/modbusserver/...` |
| 데이터 레이스 없음 | 0 race detected | `go test -race ./internal/agent/modbusserver/...` |
| 하위 호환성 | 기존 설정 파싱 성공 | 단일 디바이스 설정 테스트 |
| TypeScript 컴파일 | 0 errors | `npx tsc --noEmit` |
| 프론트엔드 빌드 | 성공 | `npm run build` |
| EARS 요구사항 완전성 | 100% 구현 | 요구사항-테스트 매트릭스 확인 |

---

## 7. Definition of Done

- [x] 모든 요구사항(REQ-BE-001 ~ REQ-BE-012, REQ-FE-001 ~ REQ-FE-006)이 구현됨
- [x] 모든 인수 기준(AC-001 ~ AC-027)이 통과함
- [x] 백엔드 테스트 커버리지 85% 이상
- [x] `go test -race` 통과
- [x] TypeScript 컴파일 오류 없음
- [x] 기존 단일 디바이스 설정 하위 호환성 검증 완료
- [x] SPEC 상태가 completed v1.0.0으로 업데이트됨
