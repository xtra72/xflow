# SPEC-MODBUS-010 구현 계획 (plan.md)

> 대상: `internal/agent/modbusserver` (type id `modbus-gateway`). 방법론: hybrid(신규 코드 TDD, 기존 편집 DDD/특성화). 시간 예측 없이 우선순위 기반 마일스톤으로 기술한다.

## 1. 기술 접근 (Technical Approach)

핵심 전략은 **`registerStore` 데코레이터 삽입**이다. 서빙 경로(`ModbusHandler` → `RequestHandler` → `registerStore`)를 거의 변경하지 않고, 백킹 디바이스에만 `registerStore`를 만족하는 `backedStore`를 끼워 upstream 통신·저장·예외를 캡슐화한다. upstream 통신은 `internal/agent/modbus`의 `ModbusTransport`를 재사용한다.

3계층으로 분해한다.

1. **설정 계층**: `DeviceConfig.Backing *BackingConfig` 추가 + `parseDevicesConfig` 확장. 부재 시 순수 slave(하위 호환).
2. **백킹 계층(신규)**: `backedStore`(registerStore 구현) + indirect 폴러. direct=실시간 조회, indirect=폴 캐시 서빙 + stale 판정. `ErrGatewayTargetFailed` sentinel.
3. **배선 계층**: `Start`/`Stop`/`processAddDevice`/`processRemoveDevice`에서 트랜스포트 connect/close + 폴러 start/stop, `RequestHandler` 예외 매핑 확장(0x0B), 프론트엔드 스키마.

## 2. 재사용 지도 (Reuse Map — file:line)

| 재사용/확장 대상                          | 위치 (file:line)                                   | 용도                                                        |
|-------------------------------------------|----------------------------------------------------|-------------------------------------------------------------|
| `registerStore` 인터페이스                | request.go:17-24                                   | `backedStore`가 구현하여 서빙 경로에 삽입 (핵심 seam)        |
| `RequestHandler` 읽기 hook                | request.go:59, 89, 127                             | store 오류 → 예외 매핑 확장(0x0B) 지점                       |
| `RequestHandler` 쓰기 hook                | request.go:171, 193, 224, 245, 281                | store 쓰기 오류 → 예외 매핑 확장 지점                        |
| `makeExceptionPDU(fc, exCode)`            | request.go:395                                     | 임의 exCode 수용 → 0x0B 프레이밍 변경 불필요                 |
| `RegisterMap` 저장/읽기 + 자체 RWMutex    | register_map.go:63, 185, 209, 282, 351, 365       | 폴러/direct 결과 저장, handler 서빙 (동시성 보호 재사용)     |
| `Device` 구조체                           | device_manager.go:17-24                            | 백킹 store/폴러 핸들 연결(디바이스별 상태)                   |
| `DeviceManager.Add/Remove/GetDevice`      | device_manager.go:177, 211, 226                    | 런타임 디바이스 관리(불변, 백킹 배선만 추가)                 |
| `DeviceConfig`                            | config.go:51-56                                    | `Backing *BackingConfig` 필드 추가                          |
| `SerialConfig`                            | config.go:42-48                                    | RTU 백킹 시리얼 파라미터 재사용                              |
| `parseDevicesConfig`                      | config.go:257                                      | `backing` 키 파싱·검증 추가                                 |
| `processAddDevice`/`processRemoveDevice`  | agent.go:1217, 1288                                | 백킹 트랜스포트/폴러 배선 훅                                 |
| `Start`/`Stop`                            | agent.go:165, 200                                  | 설정 시점 백킹 디바이스 connect/close + 폴러 lifecycle       |
| `a.mu` (RWMutex)                          | agent.go(143,182,205 등)                           | 폴러 핸들·백킹 상태 lifecycle 보호                          |
| `ModbusTransport` 인터페이스              | modbus/transport.go:18-32                          | upstream `SendAndReceive` 재사용                            |
| `NewModbusTCPTransport`                   | modbus/transport.go:59                             | TCP 백킹 트랜스포트 생성(재사용)                            |
| `ModbusRTUTransport` 생성자               | modbus/transport_rtu.go                            | RTU 백킹 트랜스포트 생성(재사용)                            |
| FC/예외 상수 + PDU 헬퍼                   | modbus/protocol.go:25-58, write.go                 | upstream 요청 PDU 조립·응답 디코딩 재사용                    |
| 프론트엔드 스키마                         | web/src/config/agentSchemas.ts (modbus-gateway)    | 백킹 필드 스키마 주도 노출                                   |
| 디바이스 에디터                           | web/src/.../ModbusServerDevicesEditor              | 스키마 렌더(신규 컴포넌트 없음)                              |

## 3. 우선순위 기반 마일스톤 (Milestones)

시간 예측을 사용하지 않는다. 의존 순서를 따른다: 설정 → 예외/store 골격 → direct → indirect → 배선 → 프론트엔드.

### Primary Goal (Priority High)

- **M1 — 백킹 설정 스키마 + 검증**: `BackingConfig` 타입, `DeviceConfig.Backing`, `parseDevicesConfig` `backing` 파싱/검증. 부재 시 순수 slave 특성화 테스트(하위 호환 HARD). (REQ-01)
- **M2 — 예외 골격 + 예외 매핑**: `ExceptionGatewayTargetFailed byte = 0x0B`, `ErrGatewayTargetFailed` sentinel, `RequestHandler` 읽기/쓰기 오류 매핑 확장(errors.Is → 0x0B). 순수 slave 회귀 없음 검증. (REQ-05)
- **M3 — `backedStore` + Direct 모드**: `registerStore` 구현 데코레이터; direct 읽기(upstream 조회+저장+서빙), direct 쓰기(전달+미러), 끊김 시 0x0B. 트랜스포트 재사용. (REQ-02/03)

### Secondary Goal (Priority Medium)

- **M4 — Indirect 모드 + 폴러**: 폴러 goroutine(ticker+context), RegisterMap 갱신, `lastSuccess` 추적, stale 판정(timeout 이내 서빙/초과 0x0B), indirect 쓰기 즉시 전달. (REQ-04)
- **M5 — 런타임/수명 배선**: `Start`/`Stop`/`processAddDevice`/`processRemoveDevice`에 connect/close + 폴러 start/stop, `a.mu` 보호. race 클린. (REQ-02/04/05)

### Final Goal (Priority Low)

- **M6 — 프론트엔드 스키마 + 관측/문서**: `agentSchemas.ts` + `ModbusServerDevicesEditor` 백킹 필드, 백킹 상태 로그, README 갱신, 커버리지 ≥85% 확인. (REQ-06)

### Optional Goal

- **M7(선택) — 백킹 관측성**: 백킹 조회/폴 성공·실패 카운터를 `get_device_status`(agent.go:1317)에 노출(스키마 확장). 필요 시에만.

## 4. 아키텍처 설계 방향 (design.md 상세)

- 서빙 경로 불변 원칙: `ModbusHandler`/`GetDevice`/`RequestHandler` 뼈대는 유지. 백킹은 `registerStore` 데코레이터로만 삽입.
- 상태 소유: `backedStore`가 (a) upstream 트랜스포트 참조, (b) 내부 `*RegisterMap`, (c) mode, (d) `lastSuccess`/timeout, (e) 폴러 취소 함수를 소유. `Device`는 이 store를 `ReqHandler.store`로 연결.
- 동시성: RegisterMap 자체 RWMutex(register_map.go:63) 재사용 + `lastSuccess` 원자/뮤텍스 + 트랜스포트 자체 직렬화(transport.go:99). `a.mu`는 폴러 lifecycle만.

## 5. 리스크 및 대응 (Risks)

| 리스크                                                            | 영향 | 대응                                                                                          |
|-------------------------------------------------------------------|------|-----------------------------------------------------------------------------------------------|
| upstream 요청 PDU 조립/디코딩 로직 중복 위험                       | 중   | `internal/agent/modbus`의 protocol.go/write.go PDU 헬퍼를 최대한 재사용, 신규 조립 최소화       |
| 폴러 goroutine 누수(remove/Stop 경로 누락)                        | 높음 | context 취소 + `a.mu` 보호 폴러 핸들 등록/해제, `go test -race` + goroutine 누수 테스트로 강제 |
| direct 읽기 시 리스너 goroutine 블로킹(upstream 지연)             | 중   | upstream 요청에 `timeout` 데드라인 부여, 미도달 시 대기 없이 0x0B                              |
| 예외 매핑 확장이 순수 slave 회귀 유발                             | 높음 | 순수 slave 경로는 `ErrGatewayTargetFailed` 미반환 → 특성화 테스트로 바이트 동일성 보장          |
| stale 경계 조건(경과 == timeout) 모호성                          | 낮음 | `<= timeout` 이내 서빙으로 명시(§spec 5.4), 경계 AC(AC-09)로 고정                              |
| RTU 백킹 시 단일 버스 경합(다중 백킹 디바이스 동일 포트)         | 중   | 트랜스포트 자체 turnaround mutex 재사용, 필요 시 포트별 트랜스포트 공유(design.md에서 확정)     |
| 실제 하드웨어 타이밍 정확성                                      | 낮음 | 범위 밖(§8 Non-Goal), 잔여 위험으로 문서화                                                     |

## 6. Definition of Done (요약)

- REQ-01~06 명세 구현, AC-01~13 통과.
- 백킹 없는 디바이스 순수 slave 특성화 테스트 통과(하위 호환 HARD).
- type id `modbus-gateway` 보존, `go.mod` 신규 modbus 모듈 없음.
- `go test -race ./internal/agent/modbusserver/...` 클린, 커버리지 ≥85%.
- 코드 주석 한국어, TRUST 5 게이트 통과.
