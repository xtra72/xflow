# SPEC-MODBUS-008 수용 기준 (Acceptance Criteria)

> 연관 SPEC: [spec.md](./spec.md) · 구현 계획: [plan.md](./plan.md)
> 형식: Given / When / Then. 각 시나리오는 독립 검증 가능해야 한다. 각 AC는 REQ 모듈에 매핑된다.

---

## AC-01 — 프레임 로그 opt-in (요약, TCP+RTU) [REQ-MODBUS-008-01]

- **Given** `log_frames: true`로 설정된 `modbus-client`가 TCP 또는 RTU 트랜스포트로 동작한다.
- **When** 클라이언트가 unitID·FC로 읽기/쓰기 트랜잭션을 수행하여 ADU를 송신하고 응답을 수신한다.
- **Then** 각 TX/RX 프레임에 대해 "modbus-client: frame" 요약(dir=TX/RX, unit_id, function code, 바이트 길이)이 INFO로 로그되며, TCP MBAP·RTU CRC 프레이밍 양쪽에서 동일하게 동작한다.

## AC-02 — Raw 프레임 게이팅 + 기본 no-op [REQ-MODBUS-008-01]

- **Given** 세 가지 설정: (a) `log_frames`/`log_raw_frames` 모두 없음, (b) `log_frames: true` + `log_raw_frames: false`, (c) `log_frames: true` + `log_raw_frames: true`.
- **When** 각 설정에서 트랜잭션을 수행한다.
- **Then** (a)는 프레임 로그가 전혀 방출되지 않고(no-op, 성능/출력 무영향), (b)는 요약만 로그되며, (c)는 요약 + 전체 ADU hex가 함께 로그된다. `log_frames`가 false면 raw 여부와 무관하게 no-op이다.

## AC-03 — per-device transport override 미지정 시 에이전트 기본 상속 (하위 호환) [REQ-MODBUS-008-02]

- **Given** 에이전트 레벨 `transport: tcp`이고 어떤 디바이스도 per-device `transport`를 지정하지 않은 기존 설정.
- **When** `parseDeviceConfig`가 파싱하고 `buildDevices`가 디바이스를 구성한다.
- **Then** 모든 디바이스가 에이전트 기본값(tcp)을 상속하여 기존과 바이트 단위로 동일하게(디바이스별 독립 `ModbusTCPTransport`) 구성되며, 회귀가 없다.

## AC-04 — per-device transport override 적용 (혼재 tcp/rtu) [REQ-MODBUS-008-02]

- **Given** 에이전트 레벨 `transport: tcp`이고, 디바이스 D1은 override 없음(tcp 상속), 디바이스 D2는 per-device `transport: rtu` + 시리얼 파라미터를 지정한다.
- **When** `buildDevices`가 디바이스를 구성한다.
- **Then** D1은 `ModbusTCPTransport`로, D2는 `ModbusRTUTransport`(per-device 시리얼)로 구성된다. per-device RTU override에 시리얼 파라미터가 누락되거나 TCP override에 host가 누락되면 설정 오류로 거부된다.

## AC-05 — share_session opt-in 시 동일 엔드포인트 연결 공유 [REQ-MODBUS-008-03]

- **Given** `share_session: true`이고, 두 디바이스가 동일 `(host, port)`(TCP) 엔드포인트를 대상으로 하며 서로 다른 unitID를 갖는다.
- **When** `buildDevices`가 디바이스를 구성하고 두 디바이스가 읽기를 수행한다.
- **Then** 두 디바이스는 하나의 공유 `ModbusTCPTransport`/연결을 사용하고, 접근은 turnaround mutex 준거로 직렬화되며, 엔드포인트가 다른 디바이스는 공유하지 않는다.

## AC-06 — share_session 기본 비활성 시 현 토폴로지 유지 + 공유 제거 격리 [REQ-MODBUS-008-03]

- **Given** `share_session` 키가 없는 기존 설정(TCP 다중 디바이스).
- **When** 에이전트가 폴링한다. (그리고 별도 시나리오: `share_session: true`에서 공유 연결을 쓰는 디바이스 하나를 제거한다.)
- **Then** (a) 기본 비활성에서는 각 TCP 디바이스가 독립 연결을 유지하고 RTU는 단일 버스 공유를 유지한다(현 토폴로지 불변). (b) 공유 활성에서 한 디바이스 제거 시, 마지막 참조가 아니면 연결은 close되지 않고 나머지 디바이스의 트랜잭션은 중단 없이 계속된다.

## AC-07 — 런타임 디바이스 추가 (재시작 없음, connect on add) [REQ-MODBUS-008-04]

- **Given** 폴링 중인 `modbus-client`와 `agent_ref`로 이를 참조하는 플로우 노드가 있고, 특정 register_groups + (선택) transport override를 갖는 신규 디바이스 설정이 준비된다.
- **When** 노드가 디바이스 추가 명령(`Process()`)을 발행한다.
- **Then** 에이전트는 재시작 없이 `a.mu`(RWMutex) 보호 하에 `parseDeviceConfig` 검증(init 경로 동일)을 통과한 디바이스를 F2 규칙으로 트랜스포트 선택하여 생성·connect하고 폴링 대상에 편입하며, 다음 폴 시점부터 해당 디바이스가 폴링된다. 중복 ID 추가는 원자적으로 거부된다.

## AC-08 — 런타임 디바이스 삭제 (close on remove, 공유 마지막-참조) [REQ-MODBUS-008-04]

- **Given** 폴링 중인 디바이스 D가 있고(옵션: F3 공유 연결 사용 중), 노드가 이를 참조한다.
- **When** 노드가 디바이스 삭제 명령(`Process()`)을 발행한다.
- **Then** 에이전트는 D를 폴링 대상에서 제외하고 연결을 close한다. IF D가 F3 공유 연결을 사용 중이면 마지막 참조일 때만 실제 close한다. 존재하지 않는 디바이스 삭제는 오류로 거부되고 에이전트는 직전 상태로 계속 동작한다(부분 적용 없음).

## AC-09 — 하위 호환 종합 (기존 설정 바이트 동일 동작) [REQ-MODBUS-008-05]

- **Given** per-device 필드 없음 + `share_session` 없음 + 프레임 로그 없음의 기존 `modbus-client` 설정.
- **When** `parseModbusConfig`가 파싱하고 에이전트가 폴링/읽기/쓰기를 수행한다.
- **Then** `modbus-client` type id가 보존되고, TCP 디바이스별 독립 연결 + RTU 단일 버스 공유 토폴로지가 유지되며, 폴링·읽기·쓰기 동작이 기존 특성화 테스트(M0 스냅샷)와 바이트 단위로 동일하다(회귀 없음).

## AC-10 — 명령 표면 일관성 + 등록/프론트엔드 + 통계 처리 [REQ-MODBUS-008-05]

- **Given** F1 디바이스 등록 명령과 신규 설정 필드(per-device transport, share_session, 프레임 토글)가 구현된다.
- **When** 노드가 디바이스 등록 명령을 발행하고, 프론트엔드가 에이전트 설정을 렌더링한다.
- **Then** 디바이스 등록은 기존 `Process` 문자열 명령 스위치(agent.go:801-824) 스타일과 일관되게 노출되고(신규 병렬 메커니즘 없음), 신규 필드는 `agentSchemas.ts` 스키마 주도로 표시되며(신규 React 컴포넌트 없이), 런타임 추가 디바이스/그룹의 통계는 확정 방침대로 확장(생성)되고, init-후 불변이던 맵의 런타임 변경에 대해 a.mu 보호 또는 원자적 교체로 폴링 goroutine과의 경합이 없다.

---

## 품질 게이트 / 완료 정의 (Definition of Done)

| 항목                     | 기준                                                                       |
|--------------------------|----------------------------------------------------------------------------|
| 빌드                     | `go build ./...` 오류 없음                                                  |
| 정적 분석                | `go vet ./...` 및 golangci-lint 경고 없음                                   |
| 커버리지                 | ≥ 85% (신규 프레임 로그·공유 트랜스포트·per-device 선택 로직 우선)          |
| 경합 검증                | `go test -race` 클린 (F3 공유 연결·F1 런타임 변경)                          |
| 의존성                   | `go.mod`에 신규 외부 modbus 모듈 미추가                                     |
| 행위 보존 (HARD)         | 기존 `modbus-client` 특성화 테스트 전부 통과(AC-09 포함, M0 스냅샷 동일)    |
| type id 보존             | `modbus-client` type id 불변, 등록 배선 유지                               |
| 수용 시나리오            | AC-01 ~ AC-10 전부 통과                                                     |

## 검증 방법 및 도구

- 단위/특성화 테스트: `go test ./internal/agent/modbus/...`
- mock `ModbusTransport`로 프레임 로그·공유 연결·per-device 선택·런타임 add/remove 경로 검증(하드웨어 불필요).
- 하위 호환 특성화: M0 베이스라인 스냅샷과 편집 후 결과의 바이트 동일성 비교(회귀 게이트).
- 실 하드웨어 RTU 타이밍은 잔여 위험으로 plan.md에 기록되며, 본 수용 기준은 mock 기반 로직 정확성까지 검증한다.
