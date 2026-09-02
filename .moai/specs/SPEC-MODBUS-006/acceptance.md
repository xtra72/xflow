# SPEC-MODBUS-006 수용 기준 (Acceptance Criteria)

> 연관 SPEC: [spec.md](./spec.md) · 구현 계획: [plan.md](./plan.md)
> 형식: Given / When / Then. 각 시나리오는 독립 검증 가능해야 한다.

---

## AC-01 — RTU 읽기 왕복 (mock 시리얼, CRC 검증) [REQ-MODBUS-006-01]

- **Given** `transport: rtu`로 설정되고 mock `ModbusTransport`(시리얼) 위에서 동작하는 modbus 클라이언트가 있고, 서버가 FC03(holding register) 읽기에 유효한 RTU 응답(정확한 CRC-16 0xA001/init 0xFFFF 포함)을 반환하도록 준비된다.
- **When** 클라이언트가 특정 unitID·start·quantity로 FC03 읽기를 수행한다.
- **Then** 요청 ADU는 `[unitID][PDU][CRC-lo][CRC-hi]` 형식이고 CRC가 올바르게 계산되며, 응답의 CRC 검증을 통과하여 디코딩된 레지스터 값이 기대값과 일치한다.

## AC-02 — 트랜스포트 선택 라우팅 (tcp vs rtu) [REQ-MODBUS-006-01, REQ-MODBUS-006-05]

- **Given** 동일한 PDU 빌더(ADU-중립 리팩터링 후)를 공유하는 두 설정: 하나는 `transport: tcp`, 다른 하나는 `transport: rtu`.
- **When** 각 설정으로 클라이언트를 초기화하고 읽기 요청을 발행한다.
- **Then** `tcp`는 `ModbusTCPTransport`(MBAP/TCP)로, `rtu`는 `ModbusRTUTransport`(CRC 프레이밍)로 라우팅되며, 각 트랜스포트가 자신의 ADU 계층만 부착하고 PDU 본문은 동일하다.

## AC-03 — transport 생략 시 기존 TCP 동작 유지 (하위 호환 특성화) [REQ-MODBUS-006-05]

- **Given** `transport` 키가 전혀 없는 기존 `modbus-tcp` 설정(`Transport.Options`).
- **When** `parseModbusConfig`가 설정을 파싱하고 에이전트가 폴링을 시작한다.
- **Then** 트랜스포트는 `tcp`로 해석되고, `modbus-tcp` type id가 보존되며, 폴링·읽기·쓰기 동작이 기존 특성화 테스트와 바이트 단위로 동일하다(회귀 없음).

## AC-04 — 서로 다른 poll_interval 그룹의 독립 폴링 [REQ-MODBUS-006-02]

- **Given** 한 디바이스에 두 개의 `register_group`이 있고, 그룹 A는 `poll_interval` 짧게, 그룹 B는 길게(또는 미지정 → 기본 주기) 설정된다.
- **When** 에이전트가 일정 시간 동안 폴링을 수행한다.
- **Then** 그룹 A는 그룹 B보다 더 자주 폴링되며(각 그룹이 자신의 cadence로 독립 폴링), 미지정 그룹은 기본 주기로 폴백하고, 한 그룹의 폴링이 다른 그룹의 정시성을 강제로 병합하지 않는다.

## AC-05 — 바이트순서 순열 디코딩 + raw 패스스루 [REQ-MODBUS-006-03]

- **Given** 알려진 워드 페어(예: 두 개의 16비트 레지스터)와 기대 int32/float32 값, 그리고 바이트순서 `CDAB`가 지정된다.
- **When** 변환기가 `CDAB`로 디코딩하고, 별도로 동일 워드를 `raw`로 요청한다.
- **Then** `CDAB` 결과는 기대 타입 값과 일치하고, `raw`는 변환 없이 원본 워드(uint16 배열)를 그대로 반환하며, 기존 `big_endian`/`little_endian`(word-swap) 설정은 종전과 동일한 결과를 낸다(별칭 하위 호환).

## AC-06 — 트랜스포트 오류 후 자동 재연결 + 디바이스 오류 통계 증가 [REQ-MODBUS-006-04]

- **Given** online 상태의 디바이스가 트랜스포트 오류를 3회 연속 반환하도록 mock이 준비되고, 이후 정상 응답으로 복구된다.
- **When** 폴링이 오류 구간과 복구 구간을 통과한다.
- **Then** 디바이스는 3회 연속 오류 후 offline으로 전이하고 오류 카운터(디바이스별/그룹별)가 증가하며, 재연결 조건 충족 시 해당 트랜스포트 방식으로 `TryReconnect`가 수행되어 online으로 복귀하고 health(online 비율)에 반영된다.

## AC-07 — 잘못된 RTU CRC 응답 거부 (unwanted-behavior) [REQ-MODBUS-006-01]

- **Given** `transport: rtu` 클라이언트와, CRC가 손상된(불일치) RTU 응답을 반환하는 mock 시리얼.
- **When** 클라이언트가 읽기를 수행하고 응답을 수신한다.
- **Then** CRC 재계산 검증이 실패하여 응답이 유효한 결과로 상위에 반환되지 않고 오류로 처리되며, 오류 통계가 증가한다(손상 프레임을 정상값으로 사용하지 않음).

## AC-08 — 노드 주도 런타임 설정 변경(재시작 없음) + init 전용 필드 거부 [REQ-MODBUS-006-02, REQ-MODBUS-006-05]

- **Given** 폴링 중인 `modbus-tcp` 클라이언트 에이전트와, `agent_ref`로 이 에이전트를 참조하는 플로우 노드가 있다. 초기 설정에는 특정 `register_group`이 기본(또는 긴) `poll_interval`로 폴링되고 있다.
- **When** 노드가 `Process()` `set_config` 명령(`{"command":"set_config","device_id":...,"params":{...}}`)을 발행하여 (a) 해당 그룹의 `poll_interval`을 더 짧게 변경하거나 그룹을 추가/수정하고, 이어 (b) 동일 명령으로 `transport`를 `tcp`→`rtu`로 전환하려 시도한다.
- **Then** (a) 에이전트는 재시작 없이 `a.mu`(RWMutex) 보호 하에 그룹 스케줄/주기를 갱신하고, 변경은 다음 폴 시점부터 반영되어(더 짧은 주기로 폴링 관측) `parseModbusConfig`와 동일한 검증을 통과한다. (b) 트랜스포트 전환 요청은 init 전용 필드이므로 오류로 거부되고, 에이전트는 중단·재시작 없이 직전 설정(tcp, 이전 그룹/주기 반영분)으로 계속 정상 동작한다. 잘못되거나 안전하지 않은 런타임 변경은 부분 적용 없이 거부된다.

---

## 품질 게이트 / 완료 정의 (Definition of Done)

| 항목                     | 기준                                                              |
|--------------------------|-------------------------------------------------------------------|
| 빌드                     | `go build ./...` 오류 없음                                         |
| 정적 분석                | `go vet ./...` 및 golangci-lint 경고 없음                          |
| 커버리지                 | ≥ 85% (신규 RTU CRC/프레이밍/시리얼 마스터, 4순열 변환 우선)       |
| 의존성                   | `go.mod`에 신규 외부 modbus 모듈 미추가 (go.bug.st/serial 재사용)  |
| 행위 보존                | 기존 `modbus-tcp` 특성화 테스트 전부 통과(AC-03 포함)             |
| type id 보존             | `modbus-tcp` type id 불변, 등록 배선 유지                          |
| 수용 시나리오            | AC-01 ~ AC-08 전부 통과                                            |

## 검증 방법 및 도구

- 단위/특성화 테스트: `go test ./internal/agent/modbus/... ./internal/modbus/... ./internal/node/...`
- mock `ModbusTransport`(시리얼)로 RTU CRC·프레이밍·오류 경로 검증(하드웨어 불필요).
- CRC-16는 알려진 테스트 벡터로 검증.
- 실 하드웨어 RTU 타이밍(T3.5/turnaround)은 잔여 위험으로 plan.md에 기록되며, 본 수용 기준은 mock 기반 로직 정확성까지 검증한다.
