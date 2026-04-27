---
id: SPEC-MODBUS-CLI-001
type: acceptance
version: "1.1.0"
created: "2026-03-16"
updated: "2026-03-16"
---

# SPEC-MODBUS-CLI-001: 수락 기준

## 품질 게이트

- 테스트 커버리지: 85% 이상 (`internal/cli/modbus.go` 기준)
- `go vet ./internal/cli/...` 통과
- `golangci-lint run ./internal/cli/...` 통과
- `go test -race ./internal/cli/ -run TestModbus` 모든 테스트 통과

---

## TC-1: 커맨드 구조 및 도움말

### TC-1.1: modbus 루트 커맨드 도움말

**Given** xflow CLI가 빌드되어 있을 때
**When** `xflow modbus --help` 명령을 실행하면
**Then** `read`, `write`, `status`, `map` 서브커맨드가 도움말에 표시되어야 한다

### TC-1.2: read 서브커맨드 도움말

**Given** xflow CLI가 빌드되어 있을 때
**When** `xflow modbus read --help` 명령을 실행하면
**Then** `--register`, `--address`, `--quantity`, `--type`, `--device`, `--force` 플래그가 도움말에 표시되어야 한다

### TC-1.3: write 서브커맨드 도움말

**Given** xflow CLI가 빌드되어 있을 때
**When** `xflow modbus write --help` 명령을 실행하면
**Then** `--register`, `--address`, `--values`, `--type`, `--device` 플래그가 도움말에 표시되어야 한다

---

## TC-2: 에이전트 타입 자동 감지

### TC-2.1: Client 에이전트 감지

**Given** `GET /api/v1/agents/{id}` 응답의 `type`이 `"modbus-tcp"`일 때
**When** `resolveModbusAgentType`을 호출하면
**Then** `"client"`를 반환해야 한다

### TC-2.2: Server 에이전트 감지

**Given** `GET /api/v1/agents/{id}` 응답의 `type`이 `"modbus-tcp-server"`일 때
**When** `resolveModbusAgentType`을 호출하면
**Then** `"server"`를 반환해야 한다

### TC-2.3: 비-MODBUS 에이전트 거부

**Given** `GET /api/v1/agents/{id}` 응답의 `type`이 `"mqtt"`일 때
**When** `resolveModbusAgentType`을 호출하면
**Then** 에러를 반환해야 하며, 에러 메시지에 실제 타입(`mqtt`)이 포함되어야 한다

### TC-2.4: 존재하지 않는 에이전트

**Given** 에이전트 ID가 존재하지 않을 때
**When** `resolveModbusAgentType`을 호출하면
**Then** API 에러가 그대로 전파되어야 한다

---

## TC-3: read 명령어

### TC-3.1: Server - holding register 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server -r hr -a 100 -q 10` 명령을 실행하면
**Then** `POST /api/v1/agents/{id}/exec`에 `{"command":"get_holding_registers","params":{"address":100,"quantity":10}}` 요청이 전송되어야 한다
**And** 응답 결과가 테이블 형식으로 출력되어야 한다

### TC-3.2: Server - input register 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server -r input -a 0 -q 5` 명령을 실행하면
**Then** `{"command":"get_input_registers","params":{"address":0,"quantity":5}}` 요청이 전송되어야 한다

### TC-3.3: Server - coils 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server -r c -a 0 -q 8` 명령을 실행하면
**Then** `{"command":"get_coils","params":{"address":0,"quantity":8}}` 요청이 전송되어야 한다

### TC-3.4: Server - discrete inputs 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server -r di -a 0 -q 8` 명령을 실행하면
**Then** `{"command":"get_discrete_inputs","params":{"address":0,"quantity":8}}` 요청이 전송되어야 한다

### TC-3.5: Client - holding register 읽기

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus read my-client -d dev1 -r holding -a 0 -q 10` 명령을 실행하면
**Then** `{"command":"read_registers","params":{"device_id":"dev1","register":"holding","address":0,"quantity":10}}` 요청이 전송되어야 한다

### TC-3.6: Client - device 플래그 누락 에러

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus read my-client -r hr -a 0 -q 10` 명령을 실행하면 (--device 없음)
**Then** `"Client 에이전트는 --device 플래그가 필수입니다"` 에러가 출력되어야 한다
**And** API 호출이 발생하지 않아야 한다

### TC-3.7: Client - force 읽기

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus read my-client -d dev1 -r hr -a 0 -q 1 --force` 명령을 실행하면
**Then** 요청 params에 `"force":true`가 포함되어야 한다

### TC-3.8: 타입 지정 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server -r hr -a 0 -q 2 -t float32` 명령을 실행하면
**Then** 요청 params에 `"data_type":"float32"`가 포함되어야 한다

### TC-3.9: 기본값으로 읽기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server` 명령을 실행하면 (모든 플래그 생략)
**Then** `{"command":"get_holding_registers","params":{"address":0,"quantity":1}}` 요청이 전송되어야 한다

### TC-3.10: JSON 출력

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus read my-server --format json -a 0 -q 1` 명령을 실행하면
**Then** 결과가 JSON 형식으로 출력되어야 한다

---

## TC-4: write 명령어

### TC-4.1: Server - 단일 레지스터 쓰기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus write my-server -a 100 -v 42` 명령을 실행하면
**Then** `{"command":"set_register","params":{"address":100,"value":42}}` 요청이 전송되어야 한다

### TC-4.2: Server - 다중 레지스터 쓰기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus write my-server -a 100 -v "100,200,300"` 명령을 실행하면
**Then** `{"command":"set_registers","params":{"address":100,"values":[100,200,300]}}` 요청이 전송되어야 한다

### TC-4.3: Server - coil 쓰기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus write my-server -r coils -a 0 -v true` 명령을 실행하면
**Then** `{"command":"set_coil","params":{"address":0,"value":true}}` 요청이 전송되어야 한다

### TC-4.4: Client - 레지스터 쓰기

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus write my-client -d dev1 -a 100 -v 42` 명령을 실행하면
**Then** `{"command":"write_register","params":{"device_id":"dev1","address":100,"value":42}}` 요청이 전송되어야 한다

### TC-4.5: Client - device 플래그 누락 에러

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus write my-client -a 100 -v 42` 명령을 실행하면 (--device 없음)
**Then** 에러가 출력되어야 한다

### TC-4.6: 타입 지정 쓰기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus write my-server -a 100 -v "3.14" -t float32` 명령을 실행하면
**Then** 요청 params에 `"data_type":"float32"`가 포함되어야 한다

### TC-4.7: 다중 coils 쓰기

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus write my-server -r c -a 0 -v "true,false,true"` 명령을 실행하면
**Then** `{"command":"set_coils","params":{"address":0,"values":[true,false,true]}}` 요청이 전송되어야 한다

---

## TC-5: status/map 명령어

### TC-5.1: status 명령어

**Given** `my-server`가 MODBUS 타입 에이전트일 때
**When** `xflow modbus status my-server` 명령을 실행하면
**Then** `{"command":"get_status"}` 요청이 전송되어야 한다
**And** 응답 결과가 출력되어야 한다

### TC-5.2: map 명령어 - Server

**Given** `my-server`가 `modbus-tcp-server` 타입 에이전트일 때
**When** `xflow modbus map my-server` 명령을 실행하면
**Then** `{"command":"get_map"}` 요청이 전송되어야 한다

### TC-5.3: map 명령어 - Client 에러

**Given** `my-client`가 `modbus-tcp` 타입 에이전트일 때
**When** `xflow modbus map my-client` 명령을 실행하면
**Then** "`map` 명령은 Server 에이전트에서만 사용할 수 있습니다" 에러가 출력되어야 한다

---

## TC-6: 레지스터 영역 별칭

### TC-6.1: 모든 별칭 정상 변환

**Given** 별칭 매핑 함수가 정의되어 있을 때
**When** 각 별칭을 입력하면
**Then** 다음과 같이 변환되어야 한다:

| 입력 | 기대 출력 |
|------|-----------|
| `holding` | `holding_registers` |
| `hr` | `holding_registers` |
| `input` | `input_registers` |
| `ir` | `input_registers` |
| `coils` | `coils` |
| `c` | `coils` |
| `discrete` | `discrete_inputs` |
| `di` | `discrete_inputs` |

### TC-6.2: 잘못된 별칭 에러

**Given** 별칭 매핑 함수가 정의되어 있을 때
**When** `"invalid"` 별칭을 입력하면
**Then** `"지원하지 않는 레지스터 영역: invalid"` 에러를 반환해야 한다

---

## TC-7: 에러 처리

### TC-7.1: 비-MODBUS 에이전트 거부

**Given** `mqtt-agent`가 `mqtt` 타입 에이전트일 때
**When** `xflow modbus read mqtt-agent -a 0 -q 1` 명령을 실행하면
**Then** MODBUS 타입이 아님을 알리는 에러가 출력되어야 한다

### TC-7.2: 서버 연결 실패

**Given** xflowd 서버가 실행 중이 아닐 때
**When** `xflow modbus read my-server -a 0 -q 1` 명령을 실행하면
**Then** 연결 에러가 출력되어야 한다

### TC-7.3: 에이전트 없음

**Given** `nonexistent` 에이전트가 존재하지 않을 때
**When** `xflow modbus read nonexistent -a 0 -q 1` 명령을 실행하면
**Then** 에이전트를 찾을 수 없음을 알리는 에러가 출력되어야 한다

---

## TC-8: Interactive 모드 호환

### TC-8.1: REPL 에서 modbus 명령 실행

**Given** xflow interactive 모드가 실행 중일 때
**When** `modbus read test-agent -r hr -a 100 -q 5` 명령을 입력하면
**Then** 정상적으로 실행되어야 한다

### TC-8.2: REPL 에서 서브커맨드 플래그 리셋

**Given** REPL에서 `modbus read test-agent -r hr -a 100 -q 5`를 실행한 후
**When** 다시 `modbus read test-agent -r input -a 200 -q 10`을 실행하면
**Then** 이전 실행의 플래그 값이 간섭하지 않고 새 값으로 동작해야 한다
**And** address 플래그는 실행 사이에 기본값(0)으로 리셋되어야 한다
**And** Changed 속성은 false로 리셋되어야 한다

---

## Definition of Done

- [x] `internal/cli/modbus.go` 구현 완료 (575줄)
- [x] `internal/cli/modbus_test.go` 테스트 작성 완료 (28개 테스트)
- [x] `internal/cli/root.go`에 `newModbusCmd` 등록
- [x] `xflow modbus read` - Server/Client 4개 레지스터 영역 읽기
- [x] `xflow modbus write` - Server/Client 단일/다중 값 쓰기
- [x] `xflow modbus status` - 에이전트 상태 조회
- [x] `xflow modbus map` - Server 전용 레지스터 맵 조회
- [x] 레지스터 별칭 (hr, ir, c, di) 정상 동작
- [x] 에이전트 타입 자동 감지 정상 동작
- [x] JSON/YAML/Table 출력 형식 지원
- [x] Interactive 모드 호환 (서브커맨드 플래그 재귀 리셋)
- [x] `go test -race ./internal/cli/` 통과 (37개 PASS)
- [x] `go build ./cmd/xflow/` 통과
