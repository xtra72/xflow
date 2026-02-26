---
id: SPEC-MODBUS-002
type: plan
version: "1.0.0"
created: "2026-02-26"
updated: "2026-02-27"
author: xtra
---

# SPEC-MODBUS-002 구현 계획: MODBUS/TCP Server Agent 구현

## 1. 작업 분해

### Primary Goal: 에러, 설정, 레지스터 맵 기반 구조

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | 서버 전용 센티널 에러 정의 (6개) | `internal/agent/modbusserver/errors.go` | High |
| 2 | ModbusServerConfig 설정 파싱 및 검증 | `internal/agent/modbusserver/config.go` | High |
| 3 | RegisterMap 구조체 및 초기화/읽기/쓰기/변경추적 | `internal/agent/modbusserver/register_map.go` | High |
| 4 | 설정 파싱 테스트 | `internal/agent/modbusserver/config_test.go` | High |
| 5 | RegisterMap 단위 테스트 | `internal/agent/modbusserver/register_map_test.go` | High |

### Secondary Goal: TCP 리스너, 연결 핸들러, 요청 핸들러

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 6 | TCP 리스너 (Accept 루프, 연결 관리, 동시 연결 제한) | `internal/agent/modbusserver/listener.go` | High |
| 7 | 연결 핸들러 (MBAP 프레임 읽기, Unit ID 매칭, 디스패치) | `internal/agent/modbusserver/handler.go` | High |
| 8 | 요청 핸들러 (FC01~FC04 읽기, FC05/FC06/FC15/FC16 쓰기, Exception 응답) | `internal/agent/modbusserver/request.go` | High |
| 9 | 핸들러 테스트 (net.Pipe 기반) | `internal/agent/modbusserver/handler_test.go` | High |
| 10 | 요청 핸들러 테스트 | `internal/agent/modbusserver/request_test.go` | High |
| 11 | TCP 리스너 테스트 | `internal/agent/modbusserver/listener_test.go` | High |

### Final Goal: 에이전트 코어, 등록, 통합 테스트

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 12 | ModbusServerAgent 메인 구현 (Agent + MessageReceiver + StatefulAgent) | `internal/agent/modbusserver/agent.go` | High |
| 13 | Process() 명령 처리 (set_coil, set_register, set_input, get_map, get_status) | `internal/agent/modbusserver/agent.go` | High |
| 14 | RegisterModbusServerTypes 타입 등록 | `internal/agent/modbusserver/register.go` | High |
| 15 | main.go에서 RegisterModbusServerTypes 호출 추가 | `cmd/xflowd/main.go` | High |
| 16 | 에이전트 통합 테스트 | `internal/agent/modbusserver/agent_test.go` | High |

### Optional Goal: 예제 및 문서

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 17 | 예제 서버 에이전트 설정 YAML | `examples/agents/modbus-server.yaml` | Medium |
| 18 | 예제 플로우 YAML (Bridge 연동) | `examples/flows/modbus-server-bridge.yaml` | Medium |

---

## 2. 기술 접근 방식

### 2.1 아키텍처 개요

```
외부 MODBUS 클라이언트 (HMI/SCADA/PLC)
        │
        │ TCP 연결
        ▼
┌─────────────────────────────────────────────────┐
│              ModbusServerAgent                   │
│                                                  │
│  ┌──────────┐   ┌──────────────┐                │
│  │ Listener │──▶│ Handler Pool │                │
│  │ (Accept) │   │ (per-conn    │                │
│  └──────────┘   │  goroutine)  │                │
│                 └──────┬───────┘                │
│                        │                        │
│                        ▼                        │
│                 ┌──────────────┐                │
│                 │ Request      │                │
│                 │ Handler      │                │
│                 │ (FC dispatch)│                │
│                 └──────┬───────┘                │
│                        │                        │
│                        ▼                        │
│                 ┌──────────────┐                │
│                 │ RegisterMap  │◀── Process()   │
│                 │ (shared)     │    (Bridge)    │
│                 │ sync.RWMutex │                │
│                 └──────┬───────┘                │
│                        │                        │
│                        ▼                        │
│                 ┌──────────────┐                │
│                 │   msgCh      │──▶ Bridge Node │
│                 │ (변경 알림)   │   (BridgeIn)   │
│                 └──────────────┘                │
└─────────────────────────────────────────────────┘
```

### 2.2 패키지 분리 전략

서버 에이전트는 `internal/agent/modbusserver/` 패키지에 위치한다. 클라이언트 에이전트(`internal/agent/modbus/`)의 `protocol.go` 상수, 디코딩 유틸리티, 에러 타입을 직접 import하여 재사용한다.

**재사용 항목 (import "github.com/xtra/xflow/internal/agent/modbus"):**
- MBAP 상수: `MBAPHeaderSize`, `MBAPProtocolID`
- FC 상수: `FC01ReadCoils` ~ `FC16WriteMultipleRegisters`
- Exception 상수: `ExceptionIllegalFunction` ~ `ExceptionSlaveDeviceFailure`
- 수량 상수: `MaxCoilsRead`, `MaxRegistersRead`, `MaxCoilsWrite`, `MaxRegistersWrite`
- 디코딩: `decodeCoils()`, `decodeRegisters()`
- 에러 타입: `ModbusException`
- 프레임 빌드 함수 중 서버 응답 빌드에 참조할 인코딩 패턴

**서버 전용 구현:**
- MBAP 요청 파싱 (클라이언트는 응답을 파싱하지만, 서버는 요청을 파싱)
- MBAP 응답 빌드 (클라이언트는 요청을 빌드하지만, 서버는 응답을 빌드)
- TCP 리스너 및 다중 연결 관리
- RegisterMap (RegisterCache와 유사하나 서버 관점의 초기화/변경추적 포함)

### 2.3 RegisterMap 설계 (Module 3, 5)

RegisterMap은 클라이언트의 RegisterCache와 유사한 구조이지만, 다음이 다르다:

- **초기화**: 설정에서 주소 범위와 초기값을 받아 맵을 사전 생성
- **주소 검증**: 맵에 정의된 주소만 접근 가능 (정의되지 않은 주소는 Exception 반환)
- **변경 추적**: 쓰기 시 이전 값과 비교하여 변경 이벤트 생성
- **읽기 전용 구분**: DiscreteInputs/InputRegisters는 외부 FC 쓰기 불가, Bridge Process()만 가능

```go
type RegisterMap struct {
    coils            map[uint16]bool
    discreteInputs   map[uint16]bool
    holdingRegisters map[uint16]uint16
    inputRegisters   map[uint16]uint16
    // 주소 범위 검증용 (설정에서 파싱)
    coilRange            AddressRange // start, count
    discreteInputRange   AddressRange
    holdingRegisterRange AddressRange
    inputRegisterRange   AddressRange
    mu                   sync.RWMutex
}

type AddressRange struct {
    Start uint16
    Count uint16
}
```

### 2.4 TCP 리스너 설계 (Module 2)

- `net.Listen("tcp", addr)` 로 리스닝
- Accept 루프를 별도 goroutine에서 실행
- `sync.WaitGroup`으로 활성 연결 추적
- `atomic.Int32`로 현재 연결 수 카운트
- `context.Context`로 shutdown 시그널 전달

### 2.5 연결 핸들러 설계 (Module 3)

각 클라이언트 연결마다 독립적인 goroutine이 할당된다:

1. `net.Conn`에서 MBAP Header (7 bytes) 읽기
2. Length 필드로 나머지 PDU 읽기
3. Unit ID 매칭 확인
4. Function Code에 따라 요청 핸들러 디스패치
5. 응답 프레임 빌드하여 `conn.Write()` 전송

유휴 타임아웃: `conn.SetReadDeadline()`으로 구현

### 2.6 요청 핸들러 설계 (Module 4)

읽기 응답 빌드:
- MBAP Header (Transaction ID echo, Protocol ID, Length, Unit ID)
- PDU: FC + Byte Count + Data

쓰기 응답 빌드:
- FC05/FC06: echo-back (요청과 동일한 응답)
- FC15/FC16: 시작 주소 + 수량 echo

Exception 응답 빌드:
- MBAP Header (Length = 3: Unit ID + Error FC + Exception Code)
- PDU: (FC | 0x80) + Exception Code

### 2.7 Bridge 연동 설계 (Module 6)

기존 MODBUS Client Agent와 동일한 패턴:
- `Process(data []byte)` 메서드로 레지스터 변경 명령 수신
- `ReceiveMessage(ctx)` 메서드로 변경 알림 전송
- JSON 기반 메시지 포맷
- `msgCh` 채널(버퍼 크기 설정 가능)로 비동기 메시지 전달
- 비블로킹 전송 (채널 가득 차면 드롭 + 경고 로그)

---

## 3. 의존성 분석

### 3.1 내부 의존성

| 패키지 | 용도 | 변경 필요 |
|--------|------|-----------|
| `internal/agent` | Agent, MessageReceiver, StatefulAgent 인터페이스, AgentConfig | 변경 없음 |
| `internal/agent/modbus` | protocol.go 상수/함수, cache.go 참조, errors.go 재사용 | 변경 없음 (import만) |
| `pkg/lifecycle` | BaseLifecycle (상태 전이) | 변경 없음 |
| `cmd/xflowd/main.go` | RegisterModbusServerTypes 호출 추가 | 1줄 추가 |

### 3.2 외부 의존성

| 패키지 | 버전 | 용도 | 설치 |
|--------|------|------|------|
| `github.com/stretchr/testify` | v1.9+ | 테스트 어설션 | 이미 사용 중 |

외부 의존성 추가 없음. Go 표준 라이브러리의 `net`, `encoding/binary`, `sync` 등만 사용한다.

---

## 4. 리스크 분석

### 리스크 1: 다중 클라이언트 동시 쓰기 충돌

- **설명**: 여러 MODBUS 클라이언트가 동시에 같은 레지스터에 쓰기를 시도할 수 있다
- **영향**: 데이터 정합성 문제, 예측 불가능한 최종 값
- **대응**: `sync.RWMutex` 기반 직렬화로 쓰기 원자성 보장. 마지막 쓰기 승리(Last Write Wins) 정책 적용. 각 쓰기마다 변경 이벤트 발행하여 플로우에서 추적 가능

### 리스크 2: 연결 수 폭증

- **설명**: 잘못된 클라이언트나 공격으로 대량의 TCP 연결이 시도될 수 있다
- **영향**: 메모리/goroutine 리소스 고갈
- **대응**: `max_connections` 제한 적용, 초과 연결 즉시 거부, 유휴 타임아웃으로 비활성 연결 자동 정리

### 리스크 3: 포트 충돌

- **설명**: 포트 502는 root 권한이 필요하거나 다른 프로세스가 사용 중일 수 있다
- **영향**: 서버 시작 실패
- **대응**: `listen_port` 설정을 유연하게 변경 가능하도록 지원. 리스닝 실패 시 명확한 에러 메시지 출력

### 리스크 4: protocol.go 패키지 순환 의존성

- **설명**: `modbusserver` 패키지가 `modbus` 패키지를 import할 때 순환 의존 가능성
- **영향**: 컴파일 에러
- **대응**: `modbus` 패키지는 `modbusserver`를 import하지 않으므로 단방향 의존성이며 순환 없음. 필요 시 공통 상수를 별도 패키지로 분리 가능하나 현재 구조에서는 불필요

---

## 5. 마일스톤 요약

| 마일스톤 | 산출물 | 우선도 |
|----------|--------|--------|
| M1: 기반 구조 | errors.go, config.go, register_map.go + 테스트 | High |
| M2: 네트워크 레이어 | listener.go, handler.go, request.go + 테스트 | High |
| M3: 에이전트 코어 | agent.go, register.go + 통합 테스트 | High |
| M4: 시스템 통합 | main.go 등록, 전체 테스트 85%+ 커버리지 | High |
| M5: 예제 | 에이전트/플로우 설정 YAML | Medium |

---

---

## 6. 구현 완료 요약

### 6.1 마일스톤 완료 현황

| 마일스톤 | 상태 | 완료일 | 비고 |
|----------|------|--------|------|
| M1: 기반 구조 | COMPLETED | 2026-02-27 | errors.go, config.go, register_map.go + 테스트 완료 |
| M2: 네트워크 레이어 | COMPLETED | 2026-02-27 | listener.go, handler.go, request.go + 테스트 완료 |
| M3: 에이전트 코어 | COMPLETED | 2026-02-27 | agent.go, register.go + 통합 테스트 27건 완료 |
| M4: 시스템 통합 | COMPLETED | 2026-02-27 | main.go 등록, 전체 커버리지 89.5% 달성 |
| M5: 예제 | COMPLETED | 2026-02-27 | modbus-server.yaml, modbus-server-bridge.yaml 작성 |

### 6.2 계획 대비 실적

- **계획**: 소스 파일 8개 + 테스트 파일 6개 = 14개 파일
- **실적**: 소스 파일 8개 + 테스트 파일 6개 = 14개 파일 (계획 동일)
- **테스트 커버리지**: 89.5% (목표 85% 초과 달성)
- **Race Detection**: Clean
- **go vet**: 경고 0건
- **go build**: Success

### 6.3 리스크 대응 결과

| 리스크 | 대응 결과 |
|--------|-----------|
| 다중 클라이언트 동시 쓰기 충돌 | sync.RWMutex 기반 직렬화 적용, race detection 통과 확인 |
| 연결 수 폭증 | max_connections 제한 + 유휴 타임아웃 구현 완료 |
| 포트 충돌 | listen_port 설정 유연성 확인, 테스트에서 동적 포트(0) 사용 |
| protocol.go 패키지 순환 의존성 | 단방향 의존성 확인, 순환 없음 |

### 6.4 추가 구현 사항

SPEC에서 정의한 6개 센티널 에러 외에 `ErrInvalidCommand` 에러를 추가하여 총 7개 센티널 에러를 정의하였다. 이는 유효하지 않은 Process() 명령에 대한 명확한 에러 처리를 위한 것이다.

---

*SPEC-MODBUS-002 Plan v1.0.0*
*작성자: xtra*
*날짜: 2026-02-27*
