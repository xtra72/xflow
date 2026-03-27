# SPEC-LGCP-001: LG Internal Control Protocol Packet Capture Agent

**Version**: 1.1.0
**Status**: Draft
**Created**: 2026-03-24

## 1. Overview

LG Internal Control Protocol (LGCP) 패킷 캡처 에이전트를 구현한다. LGCP는 기존 LGAP(8바이트 고정 프레임, 헤더 0x10)과 완전히 다른 프로토콜로, 가변 길이 프레임(STX=0x56)을 사용하는 LG 시스템 에어컨 내부 통신 프로토콜이다.

프로토콜 분석이 아직 완료되지 않았으므로, 1차 목표는 **패킷 캡처(Packet Capture)** 기능 구현이다. 시리얼 버스에서 패시브 모니터링으로 패킷을 수집하고, 파싱된 프레임 이벤트를 `msgCh`를 통해 노드로 전달한다. 파일 저장은 에이전트 책임이 아니며, 플로우의 노드를 통해 파일 관리 에이전트로 라우팅된다.

### 1.1 Protocol Summary

- **Physical**: RS-485 (보레이트 설정 가능, 기본값 9600 bps)
- **Model**: 패시브 리스닝 (캡처 전용, 명령 전송 없음)
- **Frame**: 가변 길이, STX=0x56 시작
- **Frame Structure**:
  ```
  [STX=0x56][LEN][DLEN][DA(4)][SLEN][SA(4)][CMD(2)][SEQ0#][PLEN][PAYLOAD(N)][SEQ1#][CRC(2)]
  ```
- **CRC**: CRC-16/CCITT-FALSE (poly=0x1021, init=0xFFFF) XOR 0x5C56, Big-endian
- **CRC Input**: frame[2:-2] (STX, LEN 제외, CRC 제외)
- **Known Commands**:
  - `02 01`: 설정(Control)
  - `02 04`: 상태(Status)
  - `06 04`: 디바이스 상태 요청(Device status request)

### 1.2 Key Design Decisions

- **패시브 캡처 전용**: 이 에이전트는 시리얼 버스에 데이터를 전송하지 않음 (Listen-only)
- **STX+LEN 기반 프레이밍**: 프로토콜 분석이 불완전하므로 STX(0x56)와 LEN 바이트만으로 프레임 경계를 식별
- **CRC 검증은 선택적**: 캡처 시 CRC 결과를 이벤트에 포함하되, CRC 실패 프레임도 전달
- **msgCh 기반 출력**: 캡처된 프레임을 `msgCh`를 통해 노드로 전달 (xflow 아키텍처 준수)
- **파일 저장은 노드 책임**: 에이전트는 프레임 데이터를 생산하고, 파일 저장은 플로우 내 파일 관리 노드/에이전트가 담당
- **기존 LGAP 패키지 확장**: `internal/agent/lg/` 패키지에 LGCP 관련 파일 추가
- **Agent type**: `"lgcp"` (LG 벤더 프로토콜 패턴 유지)

### 1.3 LGAP vs LGCP 비교

| 항목 | LGAP (기존) | LGCP (신규) |
|------|-------------|-------------|
| STX | 0x10 | 0x56 |
| 프레임 길이 | 고정 (요청 8B, 응답 16B) | 가변 (LEN 바이트로 지정) |
| 통신 모델 | Master/Slave 폴링 | 패시브 캡처 (Listen-only) |
| CRC | (sum % 256) XOR 0x55 | CRC-16/CCITT XOR 0x5C56 |
| 주소 체계 | Zone (1 byte) | DA(4B) + SA(4B) |
| 데이터 출력 | 상태 이벤트 → msgCh | 프레임 이벤트 → msgCh |
| 프로토콜 분석 | 완료 | 미완료 (캡처 우선) |

### 1.4 Data Flow Architecture

```
Serial Bus (RS-485)
     |
     v
[LGCP Agent] --msgCh--> [Bridge Node] --flow--> [File Agent / 기타 노드]
  캡처+파싱              ReceiveMessage()         파일 저장, 분석 등
```

## 2. Requirements

### REQ-LGCP-001-01: Agent Type Registration

시스템은 **항상** `"lgcp"` 타입을 에이전트 매니저에 등록해야 한다.

- `RegisterLGCPTypes()` 함수를 `internal/agent/lg/` 패키지에 구현한다.
- `cmd/xflowd/main.go`에서 호출하여 런타임에 `lgcp` 타입 에이전트 생성을 지원한다.
- LGAP 등록 함수(`RegisterLGLGAPTypes()`)와 동일한 패턴을 따른다.

### REQ-LGCP-001-02: Serial Transport Configuration

시스템은 **항상** LGCP 에이전트의 시리얼 포트 설정을 `LGCPConfig`로 관리해야 한다.

- 설정 항목: `serial_port`(필수), `baud_rate`(기본 9600), `data_bits`(8), `stop_bits`(1), `parity`(none), `read_timeout`(500ms)
- `LGAPTransport` 인터페이스를 재사용하되, LGCP 전용 설정 구조체 `LGCPConfig`를 정의한다.
- 시리얼 트랜스포트는 기존 `lgapSerialTransport`를 재사용한다 (팩토리 함수로 생성).

### REQ-LGCP-001-03: Frame Parser (STX+LEN Framing)

**WHEN** 시리얼 스트림에서 바이트를 수신하면 **THEN** 시스템은 STX(0x56)와 LEN 바이트를 사용하여 프레임 경계를 식별해야 한다.

- **프레임 감지**: 바이트 스트림에서 STX=0x56 을 스캔한다.
- **길이 읽기**: STX 다음 바이트를 LEN으로 읽어 프레임 전체 길이를 결정한다.
- **프레임 수집**: LEN에 지정된 바이트 수만큼 추가로 읽어 완전한 프레임을 구성한다.
- **부분 프레임 처리**: 타임아웃 내에 완전한 프레임을 수신하지 못하면 부분 프레임으로 기록하고 다음 STX를 탐색한다.
- **손상 데이터 처리**: STX 이전의 비 프레임 바이트는 가비지로 기록(선택)하고 건너뛴다.
- **LEN 유효성 검사**: LEN 값이 최소 프레임 크기(17바이트: 헤더 15 + SEQ1 1 + CRC 2) 미만이거나 비합리적으로 큰 경우(예: 255바이트 초과) 해당 바이트를 건너뛰고 다음 STX를 탐색한다.

### REQ-LGCP-001-04: CRC-16 Verification

**WHEN** 완전한 프레임이 수집되면 **THEN** 시스템은 CRC-16/CCITT-FALSE 검증을 수행해야 한다.

- CRC 계산 범위: `frame[2:-2]` (STX, LEN 제외, CRC 2바이트 제외)
- 알고리즘: CRC-16/CCITT-FALSE (poly=0x1021, init=0xFFFF, RefIn=False, RefOut=False)
- 마스크: 계산 결과에 `XOR 0x5C56` 적용
- 저장 형식: Big-endian (상위 바이트, 하위 바이트 순)
- CRC 검증 결과(pass/fail)를 프레임 이벤트에 포함한다.
- **CRC 실패 프레임도 msgCh로 전달한다** (프로토콜 분석 목적).

### REQ-LGCP-001-05: Frame Header Parsing

**WHEN** 완전한 프레임이 수집되면 **THEN** 시스템은 헤더 필드를 파싱해야 한다.

- 파싱 대상 필드:
  - `DLEN` (1B): 목적지 주소 길이
  - `DA` (DLEN bytes): 목적지 주소
  - `SLEN` (1B): 출발지 주소 길이
  - `SA` (SLEN bytes): 출발지 주소
  - `CMD` (2B): 명령 코드
  - `SEQ0#` (1B): 명령 시퀀스 번호
  - `PLEN` (1B): 페이로드 길이
  - `PAYLOAD` (PLEN bytes): 페이로드 데이터
  - `SEQ1#` (1B): 프레임 시퀀스 번호
- 파싱 결과는 프레임 이벤트의 메타데이터로 포함한다.
- 파싱 실패 시에도 원시 데이터는 이벤트로 전달한다.

### REQ-LGCP-001-06: Frame Event Output via msgCh

**WHEN** 프레임이 캡처되면 **THEN** 시스템은 JSON 이벤트를 `msgCh`로 전달해야 한다.

- **이벤트 구조**:
  ```json
  {
    "type": "lgcp_frame",
    "timestamp": "2026-03-24T10:30:45.123456789+09:00",
    "seq": 1,
    "raw_hex": "562d044455006604445500000204f81a...",
    "length": 45,
    "crc_valid": true,
    "parsed": {
      "da": "44550066",
      "sa": "44550000",
      "cmd": "0204",
      "seq0": 248,
      "plen": 26,
      "payload_hex": "1100...",
      "seq1": 152
    },
    "error": null
  }
  ```
- **부분 프레임**: `type`을 `"lgcp_partial_frame"`으로, `error` 필드에 오류 내용 기록
- **msgCh 풀 시**: 드롭하고 경고 로그 출력 (LGAP 패턴 동일)
- 브릿지 노드가 `ReceiveMessage()`로 이벤트를 수신하여 플로우 내 다음 노드로 전달한다.
- 파일 저장은 플로우 내 파일 관리 노드/에이전트가 담당한다.

### REQ-LGCP-001-07: Agent Lifecycle

시스템은 **항상** 에이전트 생명주기를 `lifecycle.BaseLifecycle`에 따라 관리해야 한다.

- **Start**: 시리얼 트랜스포트를 열고 캡처 고루틴을 시작한다.
- **Stop**: 캡처 고루틴을 종료하고 msgCh를 드레인한다.
- **Pause**: 캡처를 일시정지한다 (시리얼 연결은 유지).
- **Resume**: 캡처를 재개한다.
- 트랜스포트 연결 실패 시 지수 백오프 재연결 로직을 적용한다.

### REQ-LGCP-001-08: Capture Statistics

시스템은 **항상** 캡처 통계를 추적해야 한다.

- 추적 항목:
  - `frames_captured`: 캡처된 총 프레임 수
  - `frames_valid`: CRC 유효 프레임 수
  - `frames_invalid`: CRC 무효 프레임 수
  - `frames_partial`: 부분 프레임 수
  - `bytes_read`: 총 수신 바이트
  - `capture_start_time`: 캡처 시작 시각
- `agent.AgentStats`를 활용하여 기본 메시지 통계와 통합한다.
- `agent.StatefulAgent` 인터페이스의 `State()` 메서드에서 통계를 반환한다.

### REQ-LGCP-001-09: Agent Configuration

시스템은 **항상** YAML 설정에서 LGCP 에이전트 옵션을 파싱해야 한다.

- **LGCPConfig 필드**:
  - `serial_port` (string, 필수): 시리얼 포트 경로
  - `baud_rate` (int, 기본 9600): 보레이트
  - `data_bits` (int, 기본 8): 데이터 비트
  - `stop_bits` (int, 기본 1): 스톱 비트
  - `parity` (string, 기본 "none"): 패리티
  - `read_timeout` (duration, 기본 "500ms"): 읽기 타임아웃
  - `msg_channel_size` (int, 기본 256): 메시지 채널 버퍼 크기
  - `reconnect_interval` (duration, 기본 "5s"): 재연결 시도 간격
  - `max_reconnect_backoff` (duration, 기본 "5m"): 최대 재연결 백오프
  - `verify_crc` (bool, 기본 true): CRC 검증 활성화 여부

### REQ-LGCP-001-10: Passive Operation

시스템은 시리얼 버스에 데이터를 전송**하지 않아야 한다**.

- LGCP 에이전트는 순수 리스너이다. `Process()` 메서드는 캡처 통계 조회 명령만 지원한다.
- 지원 명령: `get_stats` (캡처 통계 조회), `get_recent` (최근 N개 프레임 조회)
- 제어/설정 명령은 지원하지 않으며, 시도 시 에러를 반환한다.

### REQ-LGCP-001-11: Web UI Schema

시스템은 **항상** 웹 UI에서 LGCP 에이전트 생성/관리를 지원해야 한다.

- `agentSchemas.ts`에 `LG_LGCP_FIELDS` 정의 (시리얼 포트, 보레이트 등)
- `AGENT_TYPES`에 `lgcp` 타입 등록
- 설정 YAML 예시 제공

### REQ-LGCP-001-12: Example Configuration

시스템은 **항상** 예제 에이전트 설정 파일을 제공해야 한다.

- `examples/agents/lgcp-capture.yaml`: LGCP 캡처 에이전트 설정 예시
- 모든 설정 필드에 주석으로 설명 포함

## 3. Architecture

### 3.1 Package Structure

기존 `internal/agent/lg/` 패키지에 LGCP 관련 파일을 추가한다:

```
internal/agent/lg/
  # --- 기존 LGAP 파일 (변경 없음) ---
  register.go              - RegisterLGLGAPTypes()
  config.go                - LGAPConfig, parseLGAPConfig()
  checksum.go              - CalcLGAPChecksum(), VerifyChecksum()
  ...

  # --- LGCP 신규 파일 ---
  lgcp_register.go         - RegisterLGCPTypes()
  lgcp_config.go           - LGCPConfig, parseLGCPConfig()
  lgcp_crc.go              - CalcLGCPCRC16(), VerifyLGCPCRC()
  lgcp_frame.go            - LGCPFrame, LGCPFrameParser (STX+LEN framing)
  lgcp_agent.go            - LGCPAgent (패시브 캡처 에이전트)
  lgcp_errors.go           - LGCP 관련 에러 변수

  # --- LGCP 테스트 ---
  lgcp_crc_test.go         - CRC-16/CCITT 계산 및 검증 테스트
  lgcp_frame_test.go       - 프레임 파서 테스트 (정상, 부분, 손상)
  lgcp_config_test.go      - 설정 파싱 테스트
  lgcp_agent_test.go       - 에이전트 통합 테스트
```

### 3.2 Component Diagram

```
                    Serial Bus (RS-485)
                         |
                         v
              +---------------------+
              |   Serial Transport  |  (LGAPTransport 인터페이스 재사용)
              |   (lgapSerialTransport)
              +---------------------+
                         |
                         v
              +---------------------+
              |   LGCPFrameParser   |  Frame Detector (STX=0x56 + LEN)
              |   - scanForSTX()    |
              |   - readFrame()     |
              |   - verifyCRC()     |
              +---------------------+
                         |
                    LGCPFrame
                         |
                         v
              +---------------------+
              |   LGCPAgent         |  Agent Lifecycle + Event Emit
              |   - captureLoop()   |
              |   - reconnectLoop() |
              |   - sendEvent()     |  → msgCh → Bridge Node → Flow
              |   - Process()       |
              +---------------------+
                         |
                      msgCh
                         |
                         v
              +---------------------+
              |   Bridge Node       |  ReceiveMessage() → Flow routing
              +---------------------+
                         |
                         v
              +---------------------+
              |   File Agent / etc  |  파일 저장, 분석, 대시보드 등
              +---------------------+
```

### 3.3 Data Types

```go
// LGCPFrame 은 파싱된 LGCP 프레임을 나타낸다.
type LGCPFrame struct {
    Raw       []byte    // 전체 원시 바이트 (STX~CRC 포함)
    Timestamp time.Time // 수신 시각
    Length    int       // LEN 필드 값
    DA        []byte    // 목적지 주소
    SA        []byte    // 출발지 주소
    CMD       [2]byte   // 명령 코드
    SEQ0      byte      // 명령 시퀀스
    Payload   []byte    // 페이로드
    SEQ1      byte      // 프레임 시퀀스
    CRCValid  bool      // CRC 검증 결과
    ParseErr  error     // 파싱 오류 (nil이면 정상)
}

// LGCPFrameEvent 는 msgCh로 전달되는 JSON 이벤트 구조이다.
type LGCPFrameEvent struct {
    Type      string        `json:"type"`               // "lgcp_frame" or "lgcp_partial_frame"
    Timestamp string        `json:"timestamp"`
    Seq       int64         `json:"seq"`
    RawHex    string        `json:"raw_hex"`
    Length    int           `json:"length"`
    CRCValid  bool          `json:"crc_valid"`
    Parsed    *ParsedHeader `json:"parsed,omitempty"`
    Error     *string       `json:"error"`
}

// ParsedHeader 는 프레임 헤더의 파싱 결과이다.
type ParsedHeader struct {
    DA         string `json:"da"`
    SA         string `json:"sa"`
    CMD        string `json:"cmd"`
    SEQ0       int    `json:"seq0"`
    PLEN       int    `json:"plen"`
    PayloadHex string `json:"payload_hex"`
    SEQ1       int    `json:"seq1"`
}
```

### 3.4 YAML Configuration Example

```yaml
name: lgcp-hvac-capture
type: lgcp
transport:
  type: serial
  options:
    serial_port: /dev/ttyUSB1
    baud_rate: 9600
    data_bits: 8
    stop_bits: 1
    parity: none
    read_timeout: "500ms"
    msg_channel_size: 256
    verify_crc: true
    reconnect_interval: "5s"
    max_reconnect_backoff: "5m"
```

## 4. Milestones

### M1: Frame Parser + CRC (Primary Goal)

- [ ] `lgcp_crc.go`: CRC-16/CCITT-FALSE 계산 + XOR 0x5C56 마스크
- [ ] `lgcp_frame.go`: `LGCPFrameParser` (STX 스캔, LEN 기반 프레임 수집, 헤더 파싱)
- [ ] `lgcp_crc_test.go`: 프로토콜 문서 예제로 CRC 라운드트립 검증
- [ ] `lgcp_frame_test.go`: 정상/부분/손상/연속 프레임 테스트

### M2: Agent Shell (Primary Goal)

- [ ] `lgcp_config.go`: `LGCPConfig` 구조체 + `parseLGCPConfig()`
- [ ] `lgcp_errors.go`: LGCP 에러 변수
- [ ] `lgcp_agent.go`: `LGCPAgent` (BaseLifecycle, captureLoop, reconnectLoop, sendEvent → msgCh)
- [ ] `lgcp_register.go`: `RegisterLGCPTypes()`
- [ ] `lgcp_config_test.go`: 설정 파싱 테스트
- [ ] `lgcp_agent_test.go`: 에이전트 통합 테스트 (캡처 루프 + msgCh 이벤트 전달)

### M3: Integration (Secondary Goal)

- [ ] `cmd/xflowd/main.go`: `RegisterLGCPTypes()` 호출 추가
- [ ] `examples/agents/lgcp-capture.yaml`: 예제 설정 파일
- [ ] Web UI 스키마: `agentSchemas.ts`, `AGENT_TYPES` 등록

### M4: Documentation (Optional Goal)

- [ ] 프로토콜 참고 문서 업데이트
- [ ] SPEC 문서 완성 및 상태 업데이트

## 5. Test Plan

### 5.1 Unit Tests

| 파일 | 테스트 항목 | 예상 테스트 수 |
|------|------------|---------------|
| lgcp_crc_test.go | CRC-16 계산, XOR 마스크, 검증 성공/실패, 프로토콜 예제 검증 | 5-8 |
| lgcp_frame_test.go | 정상 프레임 파싱, 부분 프레임, 손상 데이터, 연속 프레임, LEN 유효성 검사 | 10-15 |
| lgcp_config_test.go | 설정 파싱, 기본값, 필수 필드, 유효성 검증 | 8-10 |
| lgcp_agent_test.go | 생명주기, 캡처 루프, Process 명령, 재연결, msgCh 이벤트 전달 | 8-10 |

### 5.2 Integration Test Patterns

- **프로토콜 예제 검증**: `references/protocols/lg_internal_control_protocol.md`의 예제 프레임을 테스트 입력으로 사용
- **Mock Transport**: `LGAPSerialOpener`를 오버라이드하여 테스트용 시리얼 스트림 주입
- **msgCh 검증**: 캡처된 프레임이 올바른 JSON 이벤트로 msgCh에 전달되는지 확인

### 5.3 Key Test Scenarios

1. **프로토콜 예제 프레임 파싱**: 문서의 `56 2D 04 44 55 00 66...` 예제를 파싱하여 DA=44550066, SA=44550000, CMD=0204, CRC 유효 확인
2. **연속 프레임 스트림**: 여러 프레임이 연속된 바이트 스트림에서 각 프레임을 올바르게 분리
3. **가비지 데이터 포함**: STX 이전에 임의 바이트가 있는 스트림에서 프레임 정상 감지
4. **부분 프레임 타임아웃**: 프레임 수집 중 타임아웃 발생 시 부분 프레임 이벤트로 전달
5. **msgCh 이벤트 전달**: 캡처된 프레임이 올바른 JSON 구조로 msgCh에 전달됨
6. **에이전트 Pause/Resume**: 일시정지/재개 시 캡처 데이터 처리 확인
