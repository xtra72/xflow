# SPEC-MODBUS-006 구현 계획 (Plan)

> 연관 SPEC: [spec.md](./spec.md) · 수용 기준: [acceptance.md](./acceptance.md)
> 대상: `internal/agent/modbus/` (type id `modbus-tcp`) 확장 — 신규 패키지 복제 금지.

---

## 1. 개발 방법론

Hybrid.

- **신규 코드는 TDD**: RTU CRC-16, RTU ADU 프레이밍/파싱, 시리얼 동기식 마스터, 4순열 바이트순서 변환 — 테스트 우선(RED-GREEN-REFACTOR).
- **기존 패키지 편집은 DDD**: `protocol.go` ADU-중립 리팩터링, `agent.go`/`config.go`/`device.go` 편집 — 특성화(characterization) 테스트로 `modbus-tcp` 동작 보존 후 개선(ANALYZE-PRESERVE-IMPROVE).

기존 modbus 패키지 테스트는 행위 보존 대상으로 취급하여, 리팩터링 전후 동일 결과를 보장한다.

---

## 2. 기술 제약 (Technology Constraints)

| 제약                     | 내용                                                                        |
|--------------------------|-----------------------------------------------------------------------------|
| 언어/버전                | Go 1.23+                                                                     |
| 시리얼 의존성            | `go.bug.st/serial` (century에서 이미 사용) 재사용, 신규 modbus lib 금지     |
| CRC 규격                 | Modbus RTU CRC-16: poly 0xA001(반사형), init 0xFFFF, 리틀엔디언 부착        |
| CRC 재사용 금지          | century(CRC-16/ARC, init 0x0000), samsung(CCITT) — Modbus RTU CRC 아님      |
| 트랜스포트 인터페이스    | `ModbusTransport`(transport.go:16) 유지, mock 가능성 보존                    |
| type id 보존             | `modbus-tcp` 불변 (register.go, cmd/xflowd/main.go 배선 유지)                |
| 설정 소스                | `AgentConfig.Transport.Options`(map[string]any), `parseModbusConfig`         |
| 프론트엔드               | 스키마 주도(`agentSchemas.ts`), 신규 React 컴포넌트 없이 필드 세트 확장      |
| 하위 호환                | `transport` 생략 → tcp 기본, 그룹 `poll_interval` 생략 → 기본 주기 폴백, word-swap 별칭 유지 |

---

## 3. 마일스톤 (Milestones)

시간 예측 없이 우선순위/의존 순서로 기술한다.

### M1 — PDU 빌더 ADU-중립 리팩터링 (Priority High, 선행)
- `protocol.go` PDU 빌더에서 7바이트 MBAP 하드코딩 분리 → PDU만 생성.
- `ModbusTCPTransport`가 MBAP 부착을 담당하도록 이동.
- 특성화 테스트로 리팩터링 전후 `modbus-tcp` 동작 동일성 검증(DDD).

### M2 — RTU CRC-16 + ADU 프레이밍/파싱 (Priority High)
- CRC-16(0xA001/init 0xFFFF) 신규 구현(TDD, 알려진 벡터로 검증).
- RTU ADU 빌드(`[unitID][PDU][CRC-lo][CRC-hi]`) 및 파싱/검증.

### M3 — RTU 시리얼 동기식 마스터 (Priority High, M1·M2 의존)
- `go.bug.st/serial` 기반 시리얼 오픈(century 패턴), 반이중 turnaround mutex(`internal/agent/serial` 참고).
- T3.5 정적 준수, `ModbusRTUTransport`가 `ModbusTransport` 만족.

### M4 — 트랜스포트 선택 + 설정 파싱 (Priority High, M3 의존)
- `parseModbusConfig`에 `transport`(tcp|rtu) + RTU 시리얼 파라미터 파싱, 미지정 시 tcp 기본.
- 선택된 트랜스포트로 라우팅.

### M5 — 그룹별 독립 폴링 (Priority Medium)
- 그룹별 선택적 `poll_interval` 파싱, 다중 티커/그룹 스케줄러, 미지정 폴백.
- `PollingConfigurable` 런타임 변경 경로 반영.

### M6 — 데이터 타입 변환 확장 (Priority Medium)
- `internal/modbus`에 `raw` + 4순열 바이트순서(ABCD/BADC/CDAB/DCBA) 추가, word-swap 별칭 매핑(TDD).
- (선택) float64/uint64.

### M7 — 상태·통계 표면화 (Priority Medium, M3·M4 의존)
- 디바이스별·그룹별 성공/오류/지연 카운터, (선택) `ConnectionStatsProvider` 구현.

### M8 — 등록·프론트엔드·하위호환 마감 (Priority Medium, 종속 마감)
- `register.go`/`cmd/xflowd/main.go` type id 보존 확인.
- `agentSchemas.ts` 필드 세트 + 스키마 맵 엔트리(조건부 시리얼 필드).
- 하위 호환 특성화 테스트(transport 생략 = 기존 TCP 동작) 최종 검증.

### M9 — 노드 주도 런타임 설정 변경 (Priority Medium, M4·M5 의존)
- `ModbusAgent.Process` 스위치(agent.go:576-597)에 신규 명령 `set_config` 추가(기존 명령 디스패치 확장, 신규 병렬 메커니즘 금지).
- `processSetConfig` 핸들러: `a.mu.Lock()` 하에 런타임 가변 필드(레지스터 그룹, 그룹별 `poll_interval`, 디바이스 파라미터, TypeOverlay) 갱신 — `parseModbusConfig` 파싱/검증 규칙 재사용.
- 그룹별 주기 반영은 기존 `pollResetCh`/그룹 스케줄러 경로(M5) 재사용.
- init 전용 필드(`transport` tcp↔rtu, RTU 시리얼 하드웨어 파라미터) 변경 요청은 오류로 거부, 직전 설정 유지(부분 적용 금지).
- 특성화/단위 테스트: 재시작 없는 런타임 반영(다음 폴 시점) + 트랜스포트 전환 거부(AC-08). 신규 외부 의존성 없음.

---

## 4. 재사용 맵 (Reuse Map)

| 자산                                    | 처리      | 비고                                              |
|-----------------------------------------|-----------|---------------------------------------------------|
| `internal/agent/modbus/protocol.go`     | 편집(DDD) | PDU 빌더 ADU-중립화                               |
| `internal/agent/modbus/write.go`        | 편집(DDD) | 쓰기 PDU도 ADU-중립 경로 공유                     |
| `internal/agent/modbus/transport.go`    | 편집+신규 | 인터페이스 유지, `ModbusRTUTransport` 추가        |
| `internal/agent/modbus/agent.go`        | 편집(DDD) | pollLoop 다중 티커화, 통계 훅, `Process` `set_config` 명령 추가 |
| `internal/node/modbus.go`               | 참고/편집 | 기존 `agent_ref`→`Process()` 경로로 `set_config` 발행(런타임 재구성) |
| `internal/node/modbus_poller.go`        | 참고      | 폴러 노드의 주기/그룹 제어 연동 참고               |
| `internal/agent/modbus/device.go`       | 편집(DDD) | 트랜스포트 공통 상태/재연결                       |
| `internal/agent/modbus/config.go`       | 편집(DDD) | `parseModbusConfig` 확장, tcp 기본값              |
| `internal/agent/modbus/register.go`     | 확인      | `modbus-tcp` type id 보존                         |
| `internal/modbus/types.go`              | 편집(TDD) | `raw` + 4순열 바이트순서                          |
| `internal/agent/serial/`                | 참고      | half-duplex mutex 패턴                            |
| `century/transport_serial.go`           | 참고      | go.bug.st/serial 오픈 패턴 (CRC는 재사용 금지)     |
| `internal/agent/info.go` (`AgentStats`) | 재사용    | 통계 표면화 기반                                  |
| `cmd/xflowd/main.go:396`                | 확인      | 등록 배선 유지                                    |
| `web/src/config/agentSchemas.ts`        | 편집      | 필드 세트 + 스키마 맵 엔트리                       |
| `internal/agent/modbusserver/`          | 불변      | 공유 `internal/modbus`만 확장 (서버 미변경)        |

신규 파일: RTU CRC/프레이밍/시리얼 마스터 구현 파일(예: `transport_rtu.go`, `rtu_crc.go` 등) 및 각 테스트.

---

## 5. 리스크 분석 (Risks)

| 리스크                                   | 영향 | 대응                                                                 |
|------------------------------------------|------|----------------------------------------------------------------------|
| `modbus-tcp` 행위 회귀                   | 높음 | M1 리팩터링 전 특성화 테스트 확보, 전후 동일성 검증(DDD)             |
| PDU 빌더 리팩터링 blast radius            | 중간 | PDU/ADU 경계 명확화, TCP·RTU 공유 지점만 변경, 쓰기 경로 포함 회귀 테스트 |
| RTU CRC/엔디언 오구현                     | 중간 | 알려진 CRC 벡터·레지스터 페어로 TDD, century/samsung CRC 혼동 방지    |
| 그룹별 다중 티커 도입 시 고루틴 누수/경합 | 중간 | 스케줄러 수명 관리(정지 시 티커 정리), 경쟁 테스트                     |
| RTU 실제 하드웨어 타이밍 정확성          | 잔여 | mock 시리얼로 로직 검증, 실 하드웨어 T3.5/turnaround는 잔여 위험으로 명시 |
| 설정 하위 호환 파싱 실수                 | 중간 | `transport` 생략 = tcp, `poll_interval` 생략 = 폴백 특성화 테스트     |

---

## 6. 품질 게이트 (Quality Gates)

- `go build ./...` 클린.
- `go vet ./...` / golangci-lint 클린.
- 코드 커버리지 ≥ 85% (신규 RTU/바이트순서 코드 우선).
- `go.mod`에 신규 외부 modbus 모듈 미추가.
- 기존 modbus 패키지 테스트 전부 통과(행위 보존).
