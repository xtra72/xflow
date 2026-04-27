---
id: SPEC-LGCP-002
version: "1.2.0"
status: in-progress
created: "2026-03-31"
updated: "2026-03-31"
author: xtra
priority: high
tags: lgcp, control, rs485, serial, hvac, lg, indoor-unit
prerequisite: SPEC-LGCP-001
---

# SPEC-LGCP-002: LGCP 실내기 제어 기능 구현

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 1.0.0 | 2026-03-31 | xtra | 최초 작성 |
| 1.1.0 | 2026-03-31 | xtra | 이중 방향 제어(서모스탯 사칭 + 컨트롤러 사칭) 추가 |
| 1.2.0 | 2026-03-31 | xtra | 시퀀스 매니저 AllocSEQ 추가, 페이로드 CRC 추가 |

---

## 1. 개요

SPEC-LGCP-001에서 구현한 패시브 캡처 전용 LGCP 에이전트에 **능동 제어(Active Control)** 기능을 추가한다. 기존 캡처 루프를 유지하면서 시리얼 버스로 제어 프레임(cmd=0x0201)을 전송하여 실내기를 제어할 수 있도록 한다.

### 1.1 프로토콜 요약 (제어 관련)

**프레임 구조**:
```
[STX=0x56][LEN][DLEN=0x04][DA(4B)][SLEN=0x04][SA(4B)][CMD(2B)][SEQ0][PLEN][PAYLOAD][SEQ1][CRC16]
```

**CRC 알고리즘**: CRC-16/XMODEM (poly=0x1021, init=0x0000), Big-Endian
- CRC 계산 범위: frame[0:len-2] (STX, LEN 포함, CRC 제외)
- 기존 CalcLGCPCRC16() 함수를 그대로 사용

**확인된 제어 명령 (cmd=0x0201)**:

| 제어 항목 | 레지스터/페이로드 | 상세 |
|-----------|------------------|------|
| 전원 ON | `[18 41] [18 8V] [29 C0]` | V=압축기용량 0-F |
| 전원 OFF | `[18 40] [18 80] [29 C0]` | - |
| 설정온도 (15-30도) | `[64 8V]` | V = temp - 15 (예: 25도 = 0x8A) |
| 풍량+모드 | `[64 50 XY]` | X=풍량(1-5), Y=모드(0-4) |
| 실외기연동 ON | `[10 C1] [1A C1] [13 01]` | - |
| 실외기연동 OFF | `[10 C0] [1A C0] [13 00]` | - |

**풍량 코드**: 1=약풍, 2=중풍, 3=강풍, 4=초강, 5=자동
**모드 코드**: 0=냉방, 1=제습, 2=송풍, 3=자동, 4=난방

**주소 체계**:
- DA = 대상 실내기 (예: `44550065` = 1호기)
- SA = 컨트롤러 주소 (기본값: `44550000`)
- 브로드캐스트: `ffffffff`

**시퀀스 번호**:
- SEQ0: 명령 타입별 시퀀스 (동일 명령 재전송 시 증가)
- SEQ1: 전역 프레임 시퀀스 (모든 전송 프레임에서 증가)

### 1.2 핵심 설계 결정

- **이중 모드(Dual-Mode)**: 캡처 루프(captureLoop)와 제어 전송을 동시 지원
- **시리얼 접근 동기화**: captureLoop와 제어 전송 간 mutex 기반 직렬화 (RS-485 반이중)
- **기존 호환성 보장**: SPEC-LGCP-001의 모든 패시브 캡처 기능을 그대로 유지
- **LGAP/NASA 패턴 준수**: executor 브릿지, ControllableDevice, CommandSpec 패턴 동일
- **CRC 알고리즘**: CRC-16/XMODEM (init=0x0000) -- 실제 프레임 검증 결과 확인됨

### 1.3 LGAP 제어 에이전트와의 비교

| 항목 | LGAP (기존) | LGCP (본 SPEC) |
|------|-------------|----------------|
| 통신 모델 | Master/Slave 폴링 (동기) | 패시브 캡처 + 능동 제어 (비동기) |
| 시리얼 동기화 | pollMu Mutex | captureLoop 일시정지 + writeMu |
| 프레임 빌더 | 8바이트 고정 | 가변 길이, CRC-16/XMODEM |
| 주소 체계 | Zone (1바이트) | DA/SA (각 4바이트) |
| 시퀀스 관리 | 없음 | SEQ0 (명령별) + SEQ1 (전역) |
| 상태 확인 | 폴링 응답으로 즉시 확인 | 캡처된 응답 프레임으로 비동기 확인 |

---

## 2. 요구사항

### 2.1 프레임 빌더 및 전송 (M1)

#### REQ-LGCP-002-01: 제어 프레임 빌더

시스템은 **항상** 0x0201 제어 프레임을 올바르게 구성해야 한다.

- `LGCPFrameBuilder` 구조체를 구현한다.
- 입력: DA(4바이트), SA(4바이트), CMD(2바이트), SEQ0, 페이로드, SEQ1
- 출력: STX + LEN + 헤더 + 페이로드 + CRC16 완성 프레임 (`[]byte`)
- CRC 계산: 기존 `CalcLGCPCRC16()` 함수 사용 (CRC-16/XMODEM, init=0x0000)
- CRC 범위: frame[0:len-2] (STX, LEN 포함)
- LEN 필드: CRC 2바이트를 포함한 프레임 전체 길이 (STX 제외)

#### REQ-LGCP-002-02: 시리얼 전송 기능

**WHEN** 제어 명령이 요청되면 **THEN** 시스템은 시리얼 포트로 프레임을 전송해야 한다.

- `LGAPTransport` 인터페이스에 `Write(data []byte) (int, error)` 메서드를 추가한다.
- 기존 `lgapSerialTransport`에 Write 구현을 추가한다.
- RS-485 반이중 특성상 captureLoop와 전송을 동기화한다.
- 동기화 전략: `writeMu sync.Mutex`로 전송 직렬화, 전송 중 captureLoop는 자신의 프레임을 무시 (에코 필터링)

#### REQ-LGCP-002-03: 시퀀스 번호 관리

시스템은 **항상** 올바른 시퀀스 번호를 생성해야 한다.

- SEQ0: 명령 타입(CMD)별 카운터, 0x00-0xFF 순환
- SEQ1: 전역 프레임 카운터, 모든 전송에서 증가, 0x00-0xFF 순환
- `LGCPSequenceManager` 구조체로 관리
- 동시성 안전: `sync.Mutex` 사용

#### REQ-LGCP-002-04: 컨트롤러 주소 설정

시스템은 **항상** 설정 가능한 컨트롤러 주소(SA)를 사용해야 한다.

- `LGCPConfig`에 `ControllerAddress` 필드 추가 (기본값: `"44550000"`)
- YAML 설정: `controller_address: "44550000"`
- 주소 유효성 검증: 8자리 hex 문자열

### 2.2 제어 명령 (M2)

#### REQ-LGCP-002-05: 전원 제어 (set_power)

**WHEN** `set_power` 명령이 요청되면 **THEN** 시스템은 전원 ON/OFF 제어 프레임을 전송해야 한다.

- 파라미터: `power` (bool, 필수), `compressor_capacity` (int 0-15, 선택, 기본 0)
- 전원 ON 페이로드: `[18 41] [18 8V] [29 C0]` (V = compressor_capacity)
- 전원 OFF 페이로드: `[18 40] [18 80] [29 C0]`
- DA = 대상 실내기 주소, SA = 컨트롤러 주소, CMD = `[02 01]`

#### REQ-LGCP-002-06: 온도 설정 (set_temperature)

**WHEN** `set_temperature` 명령이 요청되면 **THEN** 시스템은 설정 온도 변경 프레임을 전송해야 한다.

- 파라미터: `temperature` (float64, 필수, 범위 15.0-30.0)
- 페이로드: `[64 8V]` (V = int(temp) - 15)
- 소수점은 버림 처리 (프로토콜이 정수 온도만 지원)
- 범위 밖 온도는 에러 반환

#### REQ-LGCP-002-07: 풍량 설정 (set_fan_speed)

**WHEN** `set_fan_speed` 명령이 요청되면 **THEN** 시스템은 풍량 변경 프레임을 전송해야 한다.

- 파라미터: `fan_speed` (string, 필수: "low", "medium", "high", "turbo", "auto")
- 풍량 코드 매핑: low=1, medium=2, high=3, turbo=4, auto=5
- 페이로드: `[64 50 XY]` (X = 풍량 코드, Y = 현재 모드 코드)
- 현재 모드를 디바이스 상태에서 조회하여 Y 값 결정 (상태 미확인 시 기본 0=냉방)

#### REQ-LGCP-002-08: 운전모드 설정 (set_mode)

**WHEN** `set_mode` 명령이 요청되면 **THEN** 시스템은 운전모드 변경 프레임을 전송해야 한다.

- 파라미터: `mode` (string, 필수: "cooling", "dehumidify", "fan", "auto", "heating")
- 모드 코드 매핑: cooling=0, dehumidify=1, fan=2, auto=3, heating=4
- 페이로드: `[64 50 XY]` (X = 현재 풍량 코드, Y = 모드 코드)
- 현재 풍량을 디바이스 상태에서 조회하여 X 값 결정 (상태 미확인 시 기본 5=자동)

#### REQ-LGCP-002-09: 복합 제어 (set_multiple)

**WHEN** `set_multiple` 명령이 요청되면 **THEN** 시스템은 여러 설정을 하나의 프레임으로 전송해야 한다.

- 파라미터: `power` (bool, 선택), `temperature` (float64, 선택), `fan_speed` (string, 선택), `mode` (string, 선택)
- 모든 제어 레지스터를 하나의 0x0201 프레임 페이로드에 결합
- LGAP 에이전트의 `set_multiple` 패턴과 동일한 Process JSON 인터페이스

### 2.3 디바이스 제어 통합 (M3)

#### REQ-LGCP-002-10: ControllableDevice 인터페이스 구현

**IF** 디바이스 타입이 "indoor"이면 **THEN** 시스템은 ControllableDevice 인터페이스를 제공해야 한다.

- `LGCPDeviceProvider`를 수정하여 indoor 타입 디바이스에 ControllableDevice 반환
- controller/broadcast 타입은 기존대로 읽기 전용 Device 반환
- `adapter.NewControllableNASADevice()` 패턴을 재사용하여 executor 브릿지 구성

#### REQ-LGCP-002-11: CommandExecutor 구현

시스템은 **항상** LGCP 제어 명령을 CommandExecutor 클로저로 변환해야 한다.

- `newLGCPExecutor(agent *LGCPAgent, address string) adapter.CommandExecutor` 함수 구현
- LGAP executor 패턴과 동일: Process() JSON 호출을 통한 명령 위임
- address 파라미터로 대상 실내기 주소를 지정

#### REQ-LGCP-002-12: CommandSpec 정의

시스템은 **항상** 각 제어 명령의 CommandSpec을 정의해야 한다.

- `lgcpIndoorCommands() []device.CommandSpec` 함수 구현
- 명령별 CommandSpec:
  - `set_power`: ParamSpec{Name:"power", Type:"bool", Required:true}, ParamSpec{Name:"compressor_capacity", Type:"int", Min:0, Max:15}
  - `set_temperature`: ParamSpec{Name:"temperature", Type:"float", Required:true, Min:15, Max:30}
  - `set_fan_speed`: ParamSpec{Name:"fan_speed", Type:"string", Required:true, Options:["low","medium","high","turbo","auto"]}
  - `set_mode`: ParamSpec{Name:"mode", Type:"string", Required:true, Options:["cooling","dehumidify","fan","auto","heating"]}
  - `set_multiple`: 위 파라미터 모두 선택적으로 포함

### 2.4 상태 확인 (M4)

#### REQ-LGCP-002-13: 제어 후 상태 확인

**WHEN** 제어 프레임이 전송된 후 **THEN** 시스템은 캡처된 응답 프레임을 통해 상태 변경을 확인해야 한다.

- 제어 전송 후 configurable 타임아웃(기본 3초) 동안 대상 디바이스의 상태 변경을 대기
- 기존 captureLoop가 수신하는 응답 프레임에서 상태 변경을 감지
- 상태 변경 감지 시 `stateChanged()` 함수 활용
- 타임아웃 시 "command sent, verification timeout" 응답 반환 (에러가 아닌 경고)
- `LGCPConfig`에 `ControlVerifyTimeout` 필드 추가 (기본값: 3초)

### 2.5 웹 UI 및 예제 (M5)

#### REQ-LGCP-002-14: agentSchemas 업데이트

시스템은 **항상** LGCP 에이전트 스키마에 제어 관련 설정 필드를 포함해야 한다.

- `web/src/config/agentSchemas.ts`의 `LG_LGCP_FIELDS`에 추가:
  - `controller_address`: string, 기본값 "44550000", 컨트롤러 SA 주소
  - `control_verify_timeout`: number, 기본값 3000, 제어 후 확인 타임아웃(ms)
  - `control_enabled`: boolean, 기본값 false, 제어 기능 활성화 토글

#### REQ-LGCP-002-15: 예제 YAML 업데이트

**가능하면** 기존 LGCP 예제 YAML에 제어 관련 설정 옵션을 포함해야 한다.

- `examples/agents/` 디렉토리의 LGCP 에이전트 예제에 control 설정 추가
- 주석으로 각 제어 파라미터 설명 포함

### 2.6 하위 호환성

#### REQ-LGCP-002-16: 패시브 모드 하위 호환성

시스템은 **항상** `control_enabled: false`(기본값)일 때 기존 패시브 캡처 전용 동작을 유지해야 한다.

- 제어 관련 Process 명령(set_power 등)은 control_enabled=false 시 에러 반환
- 기존 get_stats, get_recent 명령은 영향 없이 동작
- capabilities 목록: control_enabled=false 시 `["passive-monitor"]`, true 시 `["passive-monitor", "active-control"]`

#### REQ-LGCP-002-17: 에코 필터링

**WHEN** 제어 프레임을 전송하면 **THEN** 시스템은 RS-485 에코를 captureLoop에서 필터링해야 한다.

- RS-485 반이중 특성상 전송한 프레임이 수신측에도 들어옴
- 전송 직후 일정 시간(프레임 크기 기반 계산) 내 수신된 동일 프레임을 에코로 판별
- 에코 프레임은 캡처 통계에서 제외하되 디버그 로그에 기록

---

## 2.7 구현 노트 (실증 테스트 결과)

다음 사항은 실제 하드웨어 실증 테스트에서 발견된 결과로, 초기 SPEC의 가정을 수정한다.

#### 이중 방향 제어 (Dual-Direction Control)

컨트롤러(44550000)가 3초 주기로 실내기를 폴링하여 자체 상태를 강제하므로, 단방향 제어만으로는 실내기 상태를 변경할 수 없다. 이를 해결하기 위해 두 방향의 프레임을 동시에 전송한다:

1. **서모스탯 사칭 (Unit→Controller)**: SA=44550067, DA=44550000, 0x60+ 레지스터
   - 레지스터 0x62: 활성 운전 (0x41=ON, 0x40=OFF)
   - 레지스터 0x64,0x50,XY: 풍량(X)+모드(Y)
   - 레지스터 0x64,0x8V: 설정 온도 (V=temp-15)

2. **컨트롤러 사칭 (Controller→Unit)**: SA=44550000, DA=실내기, 0x10+ 레지스터
   - 레지스터 0x10: 실외기 활성화 (0xC1=활성, 0xC0=비활성)
   - 레지스터 0x18: 전원/압축기 (0x41=ON, 0x40=OFF)
   - 레지스터 0x29: 고정값 0xC0
   - **주의**: 레지스터 0x13 포함 시 실내기가 프레임을 거부함 (관찰 전용)

#### 연속 전송 패턴

단발 전송으로는 컨트롤러의 폴링 주기를 극복할 수 없어, 연속 라운드 방식을 사용한다:

- 30라운드, 2초 간격, 서모스탯→컨트롤러 후 0.5초 지연 후 컨트롤러→실내기
- 컨트롤러의 폴링 주기(~3초) 사이에 우리 프레임이 삽입됨

#### 페이로드 CRC

제어 프레임의 페이로드는 마지막 2바이트에 자체 CRC-16/XMODEM을 포함해야 한다:
- CRC 입력: CMD(2B) + SEQ0(1B) + PLEN(1B) + register_data
- PLEN은 register_data + CRC 2바이트를 포함한 최종 페이로드 길이

#### 시퀀스 번호 할당 (AllocSEQ)

관찰 기반 NextSEQ0/NextSEQ1은 읽기 전용이므로, 다중 프레임 전송 시 시퀀스 충돌이 발생한다. AllocSEQ0/AllocSEQ1은 할당 카운터를 전진시켜 매 호출마다 고유한 시퀀스를 보장한다. ObserveFrame이 중간에 호출되어도 할당값이 뒤로 가지 않는 high-water mark 방식이다.

#### compressor_capacity 관찰

- compCap=4: 실제 컨트롤러 관찰값, 팬 40Hz 반응
- compCap=9: 더 강한 팬 반응 (90Hz), ctrl→unit에서 사용

---

## 3. 아키텍처

### 3.1 데이터 흐름

```
                     ┌──────────────────────────────┐
                     │        LGCPAgent             │
                     │  ┌────────────────────────┐  │
 RS-485 Bus ◄───────►│  │  Serial Transport      │  │
                     │  │  (Read + Write)         │  │
                     │  └────────┬───────┬────────┘  │
                     │           │       │            │
                     │    ┌──────▼──┐ ┌──▼──────┐    │
                     │    │capture  │ │control   │    │
                     │    │Loop     │ │Send      │    │
                     │    │(패시브) │ │(능동)    │    │
                     │    └────┬────┘ └────┬─────┘   │
                     │         │           │          │
                     │    ┌────▼───────────▼────┐    │
                     │    │   Device State Mgr   │    │
                     │    │   (상태 추적/확인)    │    │
                     │    └──────────┬───────────┘    │
                     │               │                │
                     │          msgCh ▼                │
                     └──────────────────────────────┘
                                     │
                              [Bridge Node]
                                     │
                              [Flow Nodes]
```

### 3.2 시리얼 동기화 전략

RS-485는 반이중(half-duplex)이므로 동시에 읽기와 쓰기를 할 수 없다:

1. `writeMu sync.Mutex`: 제어 전송 직렬화
2. 전송 시: writeMu.Lock() -> Write -> 에코 대기 -> writeMu.Unlock()
3. captureLoop: writeMu 와 독립 동작, 에코 필터링으로 자신의 전송 프레임 무시
4. captureLoop는 중단하지 않음 -- 수신 데이터 스트림 유지

### 3.3 파일 구조 (신규/수정)

```
internal/agent/lg/
  lgcp_frame_builder.go      (신규) LGCPFrameBuilder, 페이로드 인코딩
  lgcp_frame_builder_test.go (신규)
  lgcp_sequence.go           (신규) LGCPSequenceManager
  lgcp_sequence_test.go      (신규)
  lgcp_control.go            (신규) 제어 명령 처리 (processSetPower 등)
  lgcp_control_test.go       (신규)
  lgcp_executor.go           (신규) newLGCPExecutor()
  lgcp_executor_test.go      (신규)
  lgcp_agent.go              (수정) Process() 확장, writeMu 추가, 에코 필터
  lgcp_config.go             (수정) ControllerAddress, ControlVerifyTimeout 추가
  lgcp_provider.go           (수정) ControllableDevice 반환
  lgcp_device.go             (수정) CommandSpec 정의
  lgcp_errors.go             (수정) 제어 관련 에러 추가
  transport.go               (수정) Write 메서드 추가

web/src/config/
  agentSchemas.ts            (수정) LG_LGCP_FIELDS 확장

examples/agents/
  lgcp-hvac.yaml             (수정) control 설정 추가
```

---

## 4. 마일스톤

### M1: 프레임 빌더 및 전송 (Primary Goal)

- LGCPFrameBuilder 구현 및 단위 테스트
- Transport Write 메서드 추가
- LGCPSequenceManager 구현
- 에코 필터링 구현
- REQ-LGCP-002-01 ~ REQ-LGCP-002-04

### M2: 제어 명령 (Primary Goal)

- Process() 확장: set_power, set_temperature, set_fan_speed, set_mode, set_multiple
- 페이로드 인코딩 함수 (각 제어 명령별)
- 제어 명령 단위 테스트
- REQ-LGCP-002-05 ~ REQ-LGCP-002-09

### M3: 디바이스 제어 통합 (Secondary Goal)

- newLGCPExecutor() 구현
- LGCPDeviceProvider 수정 (ControllableDevice)
- CommandSpec 정의
- REQ-LGCP-002-10 ~ REQ-LGCP-002-12

### M4: 상태 확인 (Secondary Goal)

- 제어 후 비동기 상태 확인 구현
- 타임아웃 기반 확인 로직
- REQ-LGCP-002-13

### M5: 웹 UI 및 예제 (Optional Goal)

- agentSchemas.ts 업데이트
- 예제 YAML 업데이트
- REQ-LGCP-002-14 ~ REQ-LGCP-002-15

### M6: 하위 호환성 보장 (Throughout)

- control_enabled 토글 동작 확인
- 에코 필터링 테스트
- 기존 패시브 기능 회귀 테스트
- REQ-LGCP-002-16 ~ REQ-LGCP-002-17

---

## 5. 테스트 계획

### 5.1 단위 테스트

- **프레임 빌더**: 알려진 제어 명령의 프레임 바이트 검증 (golden test)
- **CRC 계산**: 프레임 빌더가 생성한 CRC와 기존 VerifyLGCPCRC 교차 검증
- **시퀀스 관리**: SEQ0 명령별 독립 증가, SEQ1 전역 증가 확인
- **페이로드 인코딩**: 각 제어 명령의 레지스터/값 바이트 검증
- **에코 필터링**: 전송 프레임과 동일한 수신 프레임 필터 확인
- **CommandSpec**: 파라미터 검증 (범위, 옵션)

### 5.2 통합 테스트

- **Mock Transport**: io.ReadWriter mock으로 전송/수신 시뮬레이션
- **Process() 테스트**: JSON 요청 → 프레임 전송 → 상태 확인 플로우
- **ControllableDevice**: Execute() → Process() → 프레임 전송 체인 검증
- **하위 호환성**: control_enabled=false 시 제어 명령 거부 확인

### 5.3 커버리지 목표

- 신규 파일: 85% 이상
- 수정 파일: 기존 커버리지 유지 또는 향상

---

## 6. 제약 사항

- SPEC-LGCP-001 (패시브 캡처 에이전트) 구현이 선행되어야 한다.
- 프로토콜 분석이 역공학 기반이므로, 일부 엣지 케이스에서 예상과 다른 동작 가능
- RS-485 반이중 특성으로 인한 전송/수신 타이밍 이슈 가능
- 실내기 모델에 따라 지원하지 않는 제어 명령이 있을 수 있음
- 실제 하드웨어 없이는 통합 테스트에 한계가 있으므로 mock 기반 테스트 위주

---

## 7. 트레이서빌리티

| 요구사항 ID | 마일스톤 | 파일 |
|------------|----------|------|
| REQ-LGCP-002-01 | M1 | lgcp_frame_builder.go |
| REQ-LGCP-002-02 | M1 | transport.go, lgcp_agent.go |
| REQ-LGCP-002-03 | M1 | lgcp_sequence.go |
| REQ-LGCP-002-04 | M1 | lgcp_config.go |
| REQ-LGCP-002-05 | M2 | lgcp_control.go |
| REQ-LGCP-002-06 | M2 | lgcp_control.go |
| REQ-LGCP-002-07 | M2 | lgcp_control.go |
| REQ-LGCP-002-08 | M2 | lgcp_control.go |
| REQ-LGCP-002-09 | M2 | lgcp_control.go |
| REQ-LGCP-002-10 | M3 | lgcp_provider.go |
| REQ-LGCP-002-11 | M3 | lgcp_executor.go |
| REQ-LGCP-002-12 | M3 | lgcp_device.go |
| REQ-LGCP-002-13 | M4 | lgcp_agent.go, lgcp_control.go |
| REQ-LGCP-002-14 | M5 | agentSchemas.ts |
| REQ-LGCP-002-15 | M5 | lgcp-hvac.yaml |
| REQ-LGCP-002-16 | M6 | lgcp_agent.go |
| REQ-LGCP-002-17 | M6 | lgcp_agent.go |
