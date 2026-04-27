# SPEC-LGCP-002: 구현 계획

---
id: SPEC-LGCP-002
document: plan
version: "1.0.0"
---

## 1. 구현 전략

### 1.1 접근 방식

기존 LGCP 패시브 에이전트(SPEC-LGCP-001)에 제어 기능을 점진적으로 추가한다. LGAP 에이전트의 제어 구현 패턴(`executor.go`, `provider.go`, `agent.go`의 Process 디스패치)을 참조하되, LGCP 프로토콜 고유의 가변 길이 프레임 빌더와 시퀀스 관리를 새로 구현한다.

### 1.2 핵심 원칙

- **최소 변경**: 기존 captureLoop, LGCPFrame, LGCPFrameParser는 수정하지 않음
- **추가 전용**: 새 파일 추가를 우선, 기존 파일은 확장만
- **패턴 일관성**: LGAP/NASA executor 브릿지 패턴 준수
- **테스트 가능성**: 모든 제어 로직은 Transport 인터페이스를 통해 mock 가능

---

## 2. 작업 분해

### M1: 프레임 빌더 및 전송

#### Task 1.1: LGCPFrameBuilder 구현
- **파일**: `internal/agent/lg/lgcp_frame_builder.go` (신규)
- **내용**:
  - `LGCPFrameBuilder` 구조체
  - `Build(da, sa []byte, cmd [2]byte, seq0 byte, payload []byte, seq1 byte) []byte`
  - DLEN=0x04, SLEN=0x04 고정 (DA/SA 4바이트)
  - LEN 계산: 전체 프레임 길이 - 1 (STX 제외)
  - CRC 계산 및 Big-Endian 추가
- **테스트**: golden test -- 알려진 제어 프레임 바이트와 비교
- **의존성**: 없음 (독립 구현)

#### Task 1.2: Transport Write 메서드
- **파일**: `internal/agent/lg/transport.go` (수정)
- **내용**:
  - `LGAPTransport` 인터페이스에 `Write(data []byte) (int, error)` 추가
  - `lgapSerialTransport`에 Write 구현: `port.Write(data)`
- **테스트**: mock transport에서 Write 호출 확인
- **의존성**: 없음

#### Task 1.3: LGCPSequenceManager
- **파일**: `internal/agent/lg/lgcp_sequence.go` (신규)
- **내용**:
  - `LGCPSequenceManager` 구조체 (sync.Mutex 보호)
  - `NextSEQ0(cmd [2]byte) byte`: CMD별 독립 카운터
  - `NextSEQ1() byte`: 전역 카운터
  - `Reset()`: 카운터 초기화
- **테스트**: 순환 확인, 동시성 테스트
- **의존성**: 없음

#### Task 1.4: 에코 필터링
- **파일**: `internal/agent/lg/lgcp_agent.go` (수정)
- **내용**:
  - `lastSentFrame []byte` + `lastSentTime time.Time` 필드 추가
  - captureLoop에서 수신 프레임이 lastSentFrame과 동일하면 건너뜀
  - 에코 판별 윈도우: 프레임 크기 / 보레이트 * 2 (여유 계수)
- **테스트**: 에코 프레임 필터링 시나리오
- **의존성**: Task 1.1

### M2: 제어 명령

#### Task 2.1: 페이로드 인코딩 함수
- **파일**: `internal/agent/lg/lgcp_control.go` (신규)
- **내용**:
  - `encodePowerPayload(on bool, compCap int) []byte`
  - `encodeTemperaturePayload(tempC float64) []byte`
  - `encodeFanModePayload(fanCode, modeCode int) []byte`
  - `encodeMultiplePayload(params map[string]any, currentState *LGCPDeviceState) []byte`
  - 풍량/모드 코드 매핑 상수 정의
- **테스트**: 각 함수의 바이트 출력 golden test
- **의존성**: 없음 (순수 함수)

#### Task 2.2: Process() 확장
- **파일**: `internal/agent/lg/lgcp_agent.go` (수정)
- **내용**:
  - `processRequest` 구조체에 `Address string` 필드 추가
  - Process() switch에 제어 명령 분기 추가:
    - `set_power` -> `processSetPower()`
    - `set_temperature` -> `processSetTemperature()`
    - `set_fan_speed` -> `processSetFanSpeed()`
    - `set_mode` -> `processSetMode()`
    - `set_multiple` -> `processSetMultiple()`
  - `control_enabled` 검사: false이면 에러 반환
  - 각 process 함수: 파라미터 검증 -> 페이로드 인코딩 -> 프레임 빌드 -> 전송 -> 상태 확인
- **테스트**: mock transport로 전체 Process() 플로우 검증
- **의존성**: Task 1.1, 1.2, 1.3, 2.1

### M3: 디바이스 제어 통합

#### Task 3.1: CommandSpec 정의
- **파일**: `internal/agent/lg/lgcp_device.go` (수정)
- **내용**:
  - `lgcpIndoorCommands() []device.CommandSpec` 함수
  - 5개 명령(set_power, set_temperature, set_fan_speed, set_mode, set_multiple)의 CommandSpec
- **의존성**: 없음

#### Task 3.2: LGCPExecutor 구현
- **파일**: `internal/agent/lg/lgcp_executor.go` (신규)
- **내용**:
  - `newLGCPExecutor(agent *LGCPAgent, address string) adapter.CommandExecutor`
  - LGAP executor 패턴 동일: processRequest JSON -> agent.Process()
- **테스트**: executor -> Process() -> mock transport 체인
- **의존성**: Task 2.2

#### Task 3.3: Provider 수정
- **파일**: `internal/agent/lg/lgcp_provider.go` (수정)
- **내용**:
  - `Devices()`: indoor 타입이면 `adapter.NewControllableNASADevice()` 사용
  - `Device()`: 동일 로직 적용
  - commands 목록을 `lgcpIndoorCommands()`에서 가져옴
- **테스트**: provider가 indoor에 ControllableDevice, controller에 Device 반환 확인
- **의존성**: Task 3.1, 3.2

### M4: 상태 확인

#### Task 4.1: 비동기 상태 확인 구현
- **파일**: `internal/agent/lg/lgcp_control.go` (수정)
- **내용**:
  - `waitForStateChange(ctx context.Context, address string, prevState LGCPDeviceState, timeout time.Duration) (bool, error)`
  - captureLoop가 업데이트하는 디바이스 상태를 polling으로 확인 (100ms 간격)
  - context 취소 또는 타임아웃 시 종료
- **테스트**: mock 상태 변경 시나리오
- **의존성**: Task 2.2

### M5: 웹 UI 및 예제

#### Task 5.1: agentSchemas.ts 업데이트
- **파일**: `web/src/config/agentSchemas.ts` (수정)
- **내용**: LG_LGCP_FIELDS에 controller_address, control_verify_timeout, control_enabled 추가
- **의존성**: 없음

#### Task 5.2: 예제 YAML 업데이트
- **파일**: `examples/agents/lgcp-hvac.yaml` (수정)
- **내용**: control 관련 설정 추가 (주석 포함)
- **의존성**: 없음

---

## 3. 파일 변경 목록

### 신규 파일 (6개)

| 파일 | 설명 |
|------|------|
| `internal/agent/lg/lgcp_frame_builder.go` | 제어 프레임 빌더 |
| `internal/agent/lg/lgcp_frame_builder_test.go` | 프레임 빌더 테스트 |
| `internal/agent/lg/lgcp_sequence.go` | 시퀀스 번호 관리자 |
| `internal/agent/lg/lgcp_sequence_test.go` | 시퀀스 테스트 |
| `internal/agent/lg/lgcp_control.go` | 제어 명령 처리 및 페이로드 인코딩 |
| `internal/agent/lg/lgcp_control_test.go` | 제어 명령 테스트 |
| `internal/agent/lg/lgcp_executor.go` | CommandExecutor 구현 |
| `internal/agent/lg/lgcp_executor_test.go` | Executor 테스트 |

### 수정 파일 (6개)

| 파일 | 변경 내용 |
|------|----------|
| `internal/agent/lg/transport.go` | Write 메서드 추가 |
| `internal/agent/lg/lgcp_agent.go` | Process() 확장, writeMu, 에코 필터, control_enabled |
| `internal/agent/lg/lgcp_config.go` | ControllerAddress, ControlVerifyTimeout, ControlEnabled |
| `internal/agent/lg/lgcp_provider.go` | ControllableDevice 반환 |
| `internal/agent/lg/lgcp_device.go` | lgcpIndoorCommands() |
| `internal/agent/lg/lgcp_errors.go` | 제어 관련 에러 변수 |
| `web/src/config/agentSchemas.ts` | LG_LGCP_FIELDS 확장 |
| `examples/agents/lgcp-hvac.yaml` | control 설정 추가 |

---

## 4. 의존성 그래프

```
Task 1.1 (FrameBuilder) ──┐
Task 1.2 (Write)      ────┤
Task 1.3 (Sequence)   ────┤──► Task 2.2 (Process) ──► Task 3.2 (Executor) ──► Task 3.3 (Provider)
Task 2.1 (Payload)    ────┘           │
                                      ▼
Task 1.4 (Echo) ◄──── Task 1.1   Task 4.1 (StateVerify)

Task 3.1 (CommandSpec) ──────────────────────────────► Task 3.3 (Provider)

Task 5.1 (WebUI) ─── (독립)
Task 5.2 (Example) ── (독립)
```

---

## 5. 기술 접근

### 5.1 프레임 빌더 설계

```go
type LGCPFrameBuilder struct {
    seq *LGCPSequenceManager
}

func (b *LGCPFrameBuilder) BuildControl(da []byte, sa []byte, payload []byte) []byte {
    cmd := [2]byte{0x02, 0x01}
    seq0 := b.seq.NextSEQ0(cmd)
    seq1 := b.seq.NextSEQ1()
    return b.Build(da, sa, cmd, seq0, payload, seq1)
}

func (b *LGCPFrameBuilder) Build(da, sa []byte, cmd [2]byte, seq0 byte, payload []byte, seq1 byte) []byte {
    // STX + LEN + DLEN(0x04) + DA(4) + SLEN(0x04) + SA(4) + CMD(2) + SEQ0 + PLEN + payload + SEQ1 + CRC(2)
    // LEN = 전체 프레임 길이 - 1 (STX 제외)
    plen := byte(len(payload))
    frameLen := 1 + 1 + 1 + 4 + 1 + 4 + 2 + 1 + 1 + len(payload) + 1 + 2 // = 19 + len(payload)
    frame := make([]byte, 0, frameLen)
    frame = append(frame, lgcpSTX)
    frame = append(frame, byte(frameLen-1)) // LEN = 프레임 크기 - STX
    frame = append(frame, 0x04)             // DLEN
    frame = append(frame, da...)            // DA (4 bytes)
    frame = append(frame, 0x04)             // SLEN
    frame = append(frame, sa...)            // SA (4 bytes)
    frame = append(frame, cmd[0], cmd[1])   // CMD
    frame = append(frame, seq0)             // SEQ0
    frame = append(frame, plen)             // PLEN
    frame = append(frame, payload...)       // PAYLOAD
    frame = append(frame, seq1)             // SEQ1
    // CRC 계산: frame[0:len] (STX, LEN 포함)
    crc := CalcLGCPCRC16(frame)
    frame = append(frame, byte(crc>>8), byte(crc&0xFF))
    return frame
}
```

### 5.2 페이로드 인코딩 예시

```go
// 전원 ON: [18 41] [18 8V] [29 C0]
func encodePowerPayload(on bool, compCap int) []byte {
    if on {
        v := byte(compCap & 0x0F)
        return []byte{0x18, 0x41, 0x18, 0x80 | v, 0x29, 0xC0}
    }
    return []byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
}

// 설정온도: [64 8V] (V = temp - 15)
func encodeTemperaturePayload(tempC float64) []byte {
    v := byte(int(tempC) - 15)
    return []byte{0x64, 0x80 | v}
}

// 풍량+모드: [64 50 XY] (X=fan 1-5, Y=mode 0-4)
func encodeFanModePayload(fanCode, modeCode int) []byte {
    xy := byte((fanCode << 4) | (modeCode & 0x0F))
    return []byte{0x64, 0x50, xy}
}
```

### 5.3 에코 필터링 전략

```go
// 전송 시 에코 정보 기록
func (a *LGCPAgent) sendFrame(frame []byte) error {
    a.writeMu.Lock()
    defer a.writeMu.Unlock()

    a.lastSentFrame = make([]byte, len(frame))
    copy(a.lastSentFrame, frame)
    a.lastSentTime = time.Now()

    _, err := a.transport.Write(frame)
    return err
}

// captureLoop에서 에코 확인
func (a *LGCPAgent) isEcho(frame []byte) bool {
    a.writeMu.Lock()
    sent := a.lastSentFrame
    sentTime := a.lastSentTime
    a.writeMu.Unlock()

    if sent == nil || time.Since(sentTime) > a.echoWindow() {
        return false
    }
    return bytes.Equal(frame, sent)
}
```

---

## 6. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| CRC 계산 범위 불일치 | 실내기가 명령 무시 | golden test로 실제 캡처 프레임과 교차 검증 |
| 시퀀스 번호 규칙 불명확 | 명령 중복/무시 | 캡처 데이터에서 시퀀스 패턴 분석, 실패 시 0x00 고정 모드 제공 |
| RS-485 타이밍 이슈 | 프레임 충돌 | 전송 전후 guard time 삽입 (configurable) |
| 에코 필터 오탐 | 정상 프레임 누락 | 시간 윈도우 + 바이트 비교 이중 조건 |
| 풍량/모드 조합 페이로드 복잡성 | 잘못된 바이트 전송 | 현재 상태 조회 실패 시 안전한 기본값 사용 |
| 실내기 모델별 차이 | 일부 명령 미지원 | 타임아웃 기반 확인으로 실패 감지, 로그 기록 |

---

## 7. 전문가 상담 권고

### Backend Expert 상담 권고

이 SPEC은 시리얼 프로토콜 제어, RS-485 반이중 동기화, 바이트 수준 프레임 빌딩을 포함한다. `expert-backend` 에이전트 상담 권고:

- RS-485 반이중 전송/수신 동기화 전략 검토
- 에코 필터링 알고리즘 최적화
- 시퀀스 번호 관리의 동시성 안전성 검증
- Transport 인터페이스 Write 추가 시 하위 호환성 영향 분석
