---
id: SPEC-NODE-002
version: "1.1.0"
status: completed
created: "2026-04-10"
updated: "2026-05-14"
author: xtra
priority: high
tags: [node, framing, stream, processing]
related_spec: SPEC-NODE-001, SPEC-SERIAL-001, SPEC-SOCKET-001, SPEC-AGENT-006, SPEC-FLOW-001
---

# SPEC-NODE-002: Framer 노드 - 바이트 스트림 범용 프레이밍 처리 노드

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-NODE-002 |
| 제목 | Framer 노드 - 바이트 스트림 범용 프레이밍 처리 노드 |
| 버전 | 1.1.0 |
| 상태 | completed |
| 작성일 | 2026-04-10 |
| 작성자 | xtra |
| 우선순위 | high |
| 관련 SPEC | SPEC-NODE-001 (Node 시스템), SPEC-SERIAL-001 (Serial 에이전트 프레이밍), SPEC-SOCKET-001 (Socket 에이전트), SPEC-AGENT-006 (Transport 설정), SPEC-FLOW-001 (Flow 그래프) |
| 도메인 | Node System / Stream Processing / Protocol Framing |

---

## 1. Overview (개요)

### 1.1 배경

xflow 는 시리얼 에이전트의 경우 `internal/agent/serial/framing.go` 에 여섯 가지 프레이밍 모드 (raw, newline, length_prefix, fixed_size, stream, frame) 를 내장하고 있다. 이 프레이밍 로직은 `SerialFramer` 인터페이스로 정의되며, `SerialConnReader` 가 시리얼 포트에서 읽은 바이트를 인터페이스 구현체에 위임하여 완성된 프레임을 추출한다. LG ICP-02 / ACP5 / STX-ETX 같은 복잡한 프로토콜은 `frameFramer` 가 담당하며, LG ICP-02 샘플 회귀 테스트 (`TestFrameFramer_Icp02Samples`) 로 정확성이 검증되어 있다.

TCP 에이전트는 `internal/agent/socket/framing.go` 에 별도의 framer 구현을 가지고 있으나 네 가지 모드 (raw, newline, length_prefix, fixed_size) 만 지원하며, `stream` 과 `frame` 모드가 없다. 사용자가 LG ICP-02 장비를 TCP 프록시나 IP 컨버터 (Serial-to-TCP) 로 접근할 때, 또는 ACP5 게이트웨이를 TCP 로 노출할 때, 현재 xflow 는 이를 프레이밍할 방법이 없다.

UDP 에이전트의 경우 각 datagram 이 이미 메시지 경계를 가지므로 프레이밍이 불필요하지만, 어떤 UDP 장비는 여러 프레임을 하나의 datagram 에 담거나 단일 프레임을 여러 datagram 으로 분할할 수 있다. 이때도 프레이밍이 필요하다.

사용자는 다음과 같이 요청하였다:

> "시리얼 in, tcp in 등으로부터 받은 스트림에서 시리얼 에이전트에 있는 프레이밍 기능을 수행하는 노드 구현"

### 1.2 근본 원인 분석

**원인 1 - 프레이밍 로직의 소스 에이전트 종속**

`SerialFramer` 인터페이스와 여섯 개의 framer 구현체 (`rawFramer`, `newlineFramer`, `lengthPrefixFramer`, `fixedSizeFramer`, `streamFramer`, `frameFramer`) 는 `internal/agent/serial` 패키지 내부에 있다. 이 로직은 `pkg/` 공개 패키지가 아니므로 노드 계층 (`internal/node/`) 에서 직접 사용할 수 없고, 시리얼 에이전트 외부 (TCP 에이전트, 노드 등) 에서 재사용도 불가능하다.

**원인 2 - TCP 에이전트의 프레이밍이 별도 구현**

`internal/agent/socket/framing.go` 는 시리얼의 `SerialFramer` 와 유사한 인터페이스를 갖지만 구현은 독립적이며 `stream`, `frame` 모드를 지원하지 않는다. 같은 알고리즘이 두 패키지에 이원화되어 있어 유지보수 부담이 발생한다.

**원인 3 - 플로우 그래프에서 프레이밍 위치의 경직성**

현재 사용자가 프레이밍을 적용하려면 반드시 에이전트 설정 (`framing=frame` 등) 으로 지정해야 한다. 동일 에이전트가 수집하는 바이트 스트림에 대해 플로우 그래프 상에서 조건부로 프레이밍을 적용하거나, 프레이밍 결과를 여러 하위 파이프라인으로 분기하는 구조는 표현할 수 없다. 또한 한 에이전트가 여러 연결을 동시에 수용하는 TCP server 같은 경우에도 "연결별 프레이밍" 을 에이전트 내부에 강제하는 구조이다.

### 1.3 제안 기능

1. `internal/agent/serial/framing.go` 의 framer 로직을 공개 패키지 `pkg/framing` 으로 승격한다. 시리얼 에이전트와 신규 노드는 동일한 framer 엔진을 공유한다.
2. `framer` 라는 새로운 처리 노드를 `internal/node/` 에 도입한다. 이 노드는 serial-in, tcp-in, udp-in 등 **임의의 바이트 스트림 소스 노드** 의 출력을 입력으로 받아 완성된 프레임을 하나 이상 출력한다.
3. 노드는 **메타데이터 기반 스트림 키** (예: `connection_id`, `serial.port`) 로 내부 버퍼를 분리하여 TCP 서버처럼 다중 연결을 가진 소스 노드에서도 각 연결별 독립적인 프레이밍을 수행한다.
4. 출력 메시지는 기존 소스 노드 규약 (`Payload.Set("raw", []byte)` + `Payload.Set("data", hex)`) 을 그대로 따르므로 downstream Output, DebugNode, Transform 노드가 수정 없이 동작한다.
5. 프레이밍 파싱 에러는 별도 `error` 출력 포트로 전달되며, 에러 코드와 원인 메타데이터가 포함된다.
6. 시리얼 에이전트의 기존 in-agent 프레이밍 경로는 불변이며, 사용자는 "에이전트 프레이밍" 과 "노드 프레이밍" 을 자유롭게 선택할 수 있다 (이중 지원).

### 1.4 핵심 원칙

1. **엔진 단일화**: 프레이밍 알고리즘은 `pkg/framing` 에 단 하나만 존재하며, 에이전트와 노드는 동일한 엔진을 공유한다. 같은 입력에 대해 비트 단위 동일한 프레임이 생성되어야 한다.
2. **소스 무관성**: framer 노드는 serial-in, tcp-in, udp-in, file-in, MQTT 바이너리 등 **임의의 바이트 스트림 소스** 를 입력으로 받는다. 노드는 소스의 종류를 알 필요가 없으며, 페이로드에 `raw` 또는 `data` 키만 있으면 동작한다.
3. **스트림 독립성**: 한 노드가 여러 스트림을 처리할 때 각 스트림의 버퍼는 서로 완벽히 격리된다. 스트림 키는 메타데이터 기반이며, 키가 없으면 단일 공용 버퍼로 동작한다.
4. **하위 호환성**: 시리얼 에이전트의 `framing=frame` 경로와 `SerialConnReader` 동작은 변경되지 않는다. 기존 테스트 (특히 `TestFrameFramer_Icp02Samples`) 는 리팩토링 후에도 반드시 통과해야 한다.
5. **에러 분리**: 프레이밍 파싱 에러는 `out` 포트로 흘러서는 안 되며, 별도 `error` 포트로 라우팅된다. downstream 정상 파이프라인은 에러에 오염되지 않는다.
6. **자원 제한**: 다중 스트림 버퍼는 DoS 공격을 받을 수 있으므로 `max_streams`, `max_message_size`, `buffer_size` 상한으로 자원 사용을 제한한다.
7. **수신 전용**: 본 SPEC 은 수신 방향 (`Read`) 프레이밍만 다룬다. 프레임 생성 방향 (`Write`) 은 본 SPEC 의 범위 외이며, 필요 시 후속 SPEC 으로 분리한다.

---

## 2. Environment (환경)

### 2.1 현재 아키텍처

**프레이밍 엔진 (이동 대상)**

- `internal/agent/serial/framing.go` (471 라인): `SerialFramer` 인터페이스, 여섯 개의 framer 구현체, `FramerOptions` 구조체, `NewSerialFramer` 팩토리, `SerialConnReader` 래퍼
- `internal/agent/serial/common.go`: 프레이밍 모드 문자열 상수 (`FramingRaw`, `FramingNewline`, `FramingLengthPrefix`, `FramingFixedSize`, `FramingStream`, `FramingFrame`)
- `internal/agent/serial/framing_test.go` (1256 라인): 단위 테스트 및 `TestFrameFramer_Icp02Samples` LG ICP-02 회귀 테스트 (커밋 bbc024e 에서 추가)

**노드 시스템**

- `internal/node/base.go`: `BaseNode` 공통 라이프사이클 및 뮤텍스 보호
- `internal/node/registry.go` (203 라인): `NodeFactory` 타입, `Registry` 구조체, `registerBuiltins` 에서 빌트인 노드 등록
- 기존 처리 노드 예: `internal/node/transform.go`, `internal/node/filter.go`
- 기존 소스 노드 예: `internal/node/serial_io.go`, `internal/node/tcp_io.go`

**메시지 페이로드 규약**

- `pkg/message/payload.go`: `Payload` 인터페이스 (`map[string]any`), `Set`, `Get`, `GetPath`, `Keys`, `ToMap`
- 소스 노드 규약: `Payload.Set("raw", []byte(...))` 와 `Payload.Set("data", hex.EncodeToString(...))` 를 함께 설정
- 메타데이터: `message.Metadata()` 로 접근, 업스트림 source_addr, timestamp, connection_id 등을 보관

**플로우 그래프 정의**

- `pkg/flow/flow.go`: `NodeDef` 에 `Type`, `Options`, `Inputs`, `Outputs` 필드
- 처리 노드의 `Process(ctx, msg) ([]message.Message, error)` 시그니처는 단일 입력 메시지에 대해 0개 이상의 출력 메시지를 반환할 수 있으므로 framer 노드의 0/1/다 프레임 모델에 자연스럽게 부합한다.

### 2.2 영향 범위

**신규 파일**

- `pkg/framing/framing.go`: `Framer` 인터페이스, `Options` 구조체, `New` 팩토리, 모드 상수 (`ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame`)
- `pkg/framing/raw.go`, `pkg/framing/newline.go`, `pkg/framing/length_prefix.go`, `pkg/framing/fixed_size.go`, `pkg/framing/stream.go`, `pkg/framing/frame.go`: 여섯 개의 framer 구현체 (파일 분할은 plan.md 에서 결정)
- `pkg/framing/framing_test.go`: 단위 테스트 (serial/framing_test.go 에서 이동)
- `internal/node/framer.go`: 신규 framer 노드 구현
- `internal/node/framer_test.go`: 신규 framer 노드 단위 테스트
- `internal/node/framer_parity_test.go`: 시리얼 에이전트 vs framer 노드 parity 검증

**수정 파일**

- `internal/agent/serial/framing.go`: `pkg/framing` 으로의 위임 wrapper 또는 타입 alias 유지 (backward compatibility)
- `internal/agent/serial/framing_test.go`: framer 유닛 테스트는 `pkg/framing/framing_test.go` 로 이동하고, `SerialConnReader` 관련 테스트는 유지
- `internal/agent/serial/common.go`: 프레이밍 모드 상수는 `pkg/framing` 을 참조하거나 별칭 유지
- `internal/node/registry.go`: `registerBuiltins` 에 `framer` 타입 등록

**수정 없음 (Out of Scope)**

- 시리얼 에이전트의 `framing=frame` 경로 및 `SerialConnReader` 동작
- TCP 에이전트 (`internal/agent/socket/tcp_client.go`, `tcp_server.go`) 의 내부 동작 - framer 노드가 TCP 에이전트에 직접 통합되는 것이 아니라 플로우 그래프 상에서 tcp-in 다음에 배치되는 구조이다.
- UDP 에이전트 내부 동작
- 기존 에이전트 설정 스키마, API 응답 구조, Web UI 스키마
- 다른 프레이밍 모드 추가 (CRC-16, XMODEM 등)

### 2.3 TCP 서버 connection_id 메타데이터 의존성

현재 TCP 서버 소스 노드 (`internal/node/tcp_io.go`) 가 다중 클라이언트 연결을 수용할 때 각 연결에서 발생하는 메시지의 메타데이터에 `connection_id` 를 포함하는지는 구현 시점에 확인이 필요하다. 만약 포함하지 않는다면 framer 노드의 다중 스트림 분리는 TCP 서버에 대해 동작하지 않고 모든 연결의 바이트가 단일 공용 버퍼로 병합된다. 이 경우 본 SPEC 은 다음 전략 중 하나를 선택한다 (plan.md 결정 (f)):

- 전략 1: 본 SPEC 의 일부로 `TCPInNode` 서버 모드가 `connection_id` 를 주입하도록 최소 수정을 포함한다.
- 전략 2: `connection_id` 주입은 별도 SPEC (예: SPEC-NODE-003) 으로 분리하고, 본 SPEC 은 "connection_id 가 주입된 경우" 를 전제로 설계한다. 주입이 없을 때에는 framer 노드가 단일 공용 버퍼로 동작하며, 사용자는 TCP 서버에 대해 연결별 프레이밍을 원하면 해당 별도 SPEC 을 기다린다.

---

## 3. Assumptions (가정)

1. 사용자는 serial-in, tcp-in, udp-in 등 임의의 바이트 스트림 소스 노드의 출력을 framer 노드의 입력으로 연결할 수 있다.
2. 소스 노드의 출력 메시지 페이로드에는 `raw` 키로 `[]byte` 가 있거나, 없는 경우 `data` 키로 hex 문자열이 있다.
3. framer 노드는 단일 입력 포트 (`in`) 를 가지며 플로우 그래프 상에서 한 소스 노드에만 연결된다. 여러 소스 노드를 하나의 framer 노드에 연결하려는 경우는 본 SPEC 의 범위 외이다.
4. 하나의 framer 노드 인스턴스는 Process 호출 간에 상태 (버퍼) 를 유지한다. 플로우가 재시작되면 모든 버퍼는 초기화된다.
5. 메타데이터에 `connection_id` 같은 스트림 키가 포함되어 있는지 여부는 소스 노드의 구현에 따른다. 본 SPEC 은 키가 있으면 분리, 없으면 단일 버퍼로 동작하도록 설계한다.
6. `pkg/framing` 의 framer 구현은 뮤텍스 보호 없이도 단일 고루틴에서 사용될 때 정확히 동작한다 (framer 인스턴스 자체는 stateless 하거나 내부적으로 안전하다). framer 노드는 자신의 스트림 버퍼별 접근을 직렬화할 책임을 가진다.
7. 동시 다중 Process 호출은 기존 노드 라이프사이클에 따라 직렬화되거나, framer 노드가 내부 뮤텍스로 직렬화한다.
8. 시리얼 에이전트의 `framing=frame` 경로는 동일한 `pkg/framing` 엔진을 사용하므로 리팩토링 후에도 LG ICP-02 샘플 회귀 테스트가 통과한다.

---

## 4. Requirements (요구사항 - EARS Format)

### M1: Framer 노드 도메인 모델 및 노출 API

**R1.1 (Ubiquitous)**: 시스템은 항상 `framer` 라는 이름의 처리 노드 타입을 `internal/node/registry.go` 의 `registerBuiltins` 에 등록해야 한다.

**R1.2 (Ubiquitous)**: 시스템은 항상 framer 노드를 "processing" 카테고리에 배치해야 한다.

**R1.3 (Ubiquitous)**: 시스템은 항상 framer 노드가 다음 포트 구조를 제공해야 한다:

| 포트 | 방향 | 용도 |
|------|------|------|
| `in` | input (단일) | 바이트 스트림 입력 메시지 |
| `out` | output | 완성된 프레임 출력 |
| `error` | output | 프레이밍 에러 출력 |

**R1.4 (Ubiquitous)**: 시스템은 항상 framer 노드가 `NodeDef.Options` 맵에서 다음 필수 키를 읽어 프레이밍 모드를 결정해야 한다:

- `framing`: 문자열, 허용 값은 `raw`, `newline`, `length_prefix`, `fixed_size`, `stream`, `frame` 중 하나

**R1.5 (Ubiquitous)**: 시스템은 항상 framer 노드가 `pkg/framing.Options` 의 모든 필드에 대응하는 옵션 키를 `NodeDef.Options` 에서 받아들여야 한다. 옵션 키 이름은 시리얼 에이전트의 기존 옵션 이름 (`buffer_size`, `delimiter`, `fixed_size`, `max_message_size`, `stx`, `etx`, `length_offset`, `length_size`, `length_endian`, `length_includes_header`, `length_adjustment`, `checksum`) 을 그대로 재사용해야 한다.

**R1.6 (Ubiquitous)**: 시스템은 항상 framer 노드가 다음 추가 옵션 키를 `NodeDef.Options` 에서 받아들여야 한다:

| 키 | 타입 | 기본값 | 용도 |
|----|------|--------|------|
| `stream_key_metadata` | string | `"connection_id"` | 다중 스트림 분리에 사용할 메타데이터 키 이름 |
| `stream_idle_timeout` | duration | `0` (비활성) | 스트림 버퍼의 유휴 타임아웃 |
| `max_streams` | int | `0` (무제한) | 동시 스트림 수 상한 |

**R1.7 (State-Driven)**: IF `framing` 옵션이 없거나 허용 값이 아닌 경우 THEN 시스템은 framer 노드 팩토리에서 에러를 반환하고 노드 생성을 실패시켜야 한다.

**R1.8 (Unwanted)**: 시스템은 framer 노드의 옵션 키 이름을 시리얼 에이전트의 기존 옵션과 다르게 명명해서는 안 된다.

---

### M2: Framer 노드 Process 동작 명세

**R2.1 (Event-Driven)**: WHEN framer 노드의 `Process` 메서드가 입력 메시지를 수신하면, THEN 시스템은 메시지 페이로드에서 `raw` 키로 `[]byte` 를 읽어야 한다.

**R2.2 (State-Driven)**: IF 페이로드에 `raw` 키가 없으면 THEN 시스템은 `data` 키에서 hex 문자열을 읽고 디코딩하여 바이트 배열을 구성해야 한다.

**R2.3 (State-Driven)**: IF 페이로드에 `raw` 와 `data` 키가 모두 없거나 디코딩에 실패하면 THEN 시스템은 해당 메시지를 `error` 포트로 전달하고 에러 코드 `frame.input.invalid_payload` 를 포함해야 한다.

**R2.4 (Event-Driven)**: WHEN 유효한 바이트가 입력되면, THEN 시스템은 `stream_key_metadata` 에 지정된 메타데이터 키 값을 조회하여 스트림 키를 결정해야 한다. 메타데이터 키가 없거나 값이 비어 있으면 단일 공용 버퍼 키 (예: 빈 문자열) 를 사용해야 한다.

**R2.5 (Event-Driven)**: WHEN 스트림 키가 결정되면, THEN 시스템은 해당 키에 대응하는 내부 스트림 버퍼를 조회하거나 없으면 새로 생성해야 한다.

**R2.6 (Event-Driven)**: WHEN 스트림 버퍼가 준비되면, THEN 시스템은 입력 바이트를 버퍼의 끝에 추가하고 `pkg/framing.Framer.Read` 를 반복 호출하여 가능한 한 많은 완성된 프레임을 추출해야 한다.

**R2.7 (State-Driven)**: IF `Read` 가 `io.EOF` 또는 부족한 바이트를 의미하는 에러를 반환하면 THEN 시스템은 버퍼에 남은 바이트를 보존하고 현재 호출에서 추가 프레임 추출을 중단해야 한다.

**R2.8 (Event-Driven)**: WHEN `Read` 가 하나 이상의 완성된 프레임을 반환하면, THEN 시스템은 각 프레임에 대해 입력 메시지를 clone 한 새 메시지를 생성해야 한다. 새 메시지의 페이로드는 해당 프레임 바이트만을 담아야 한다 (`raw` 키에 프레임 바이트, `data` 키에 hex 인코딩 문자열).

**R2.9 (Ubiquitous)**: 시스템은 항상 출력 메시지의 업스트림 메타데이터 (source_addr, timestamp, connection_id 등 입력 메시지의 모든 메타데이터) 를 보존해야 한다.

**R2.10 (Ubiquitous)**: 시스템은 항상 출력 메시지의 메타데이터에 다음 추가 필드를 설정해야 한다:

| 메타데이터 키 | 값 | 설명 |
|---|---|---|
| `frame.index` | int | 해당 스트림 내의 0 부터 시작하는 순차 카운터 |
| `frame.framer_type` | string | `framing` 옵션 값 (예: `frame`, `newline`) |
| `frame.stream_key` | string | 다중 스트림인 경우의 스트림 키, 단일 공용 버퍼인 경우 빈 문자열 |

**R2.11 (Event-Driven)**: WHEN 한 번의 Process 호출에서 여러 프레임이 조립되면, THEN 시스템은 프레임 조립 순서를 보존한 `[]message.Message` 로 반환해야 한다.

**R2.12 (Event-Driven)**: WHEN 한 번의 Process 호출에서 어떤 프레임도 완성되지 않으면, THEN 시스템은 빈 `[]message.Message` 와 nil 에러를 반환해야 한다.

**R2.13 (Unwanted)**: 시스템은 framer 노드의 `out` 포트로 부분적이거나 미완성된 프레임을 전달해서는 안 된다.

**R2.14 (Unwanted)**: 시스템은 framer 노드의 `out` 포트로 프레이밍 파싱 에러 메시지를 전달해서는 안 된다. 에러는 오직 `error` 포트로만 전달되어야 한다.

---

### M3: pkg/framing 패키지 승격

**R3.1 (Ubiquitous)**: 시스템은 항상 `pkg/framing` 이라는 공개 패키지를 제공하고 다음 핵심 API 를 노출해야 한다:

- `type Framer interface { Read(r io.Reader) ([]byte, error); Write(w io.Writer, data []byte) error }`
- `type Options struct { ... }` (기존 `FramerOptions` 의 모든 필드)
- `func New(mode string, opts Options) (Framer, error)`
- 모드 상수: `ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame`

**R3.2 (Ubiquitous)**: 시스템은 항상 `pkg/framing` 의 framer 구현 (raw, newline, length_prefix, fixed_size, stream, frame) 이 기존 `internal/agent/serial/framing.go` 의 구현과 **비트 단위 동일한 동작** 을 보이도록 이동해야 한다.

**R3.3 (Ubiquitous)**: 시스템은 항상 `pkg/framing` 의 framer 유닛 테스트를 `internal/agent/serial/framing_test.go` 에서 이동 또는 복제하여 포함해야 한다. 특히 `TestFrameFramer_Icp02Samples` 에 해당하는 회귀 테스트는 `pkg/framing` 에서 반드시 수행되어야 한다.

**R3.4 (Ubiquitous)**: 시스템은 항상 `internal/agent/serial` 패키지가 `pkg/framing` 을 import 하여 사용하도록 리팩토링해야 한다. `SerialConnReader` 는 `internal/agent/serial` 에 유지되지만 내부적으로 `pkg/framing.Framer` 를 사용해야 한다.

**R3.5 (Ubiquitous)**: 시스템은 항상 리팩토링 후에도 `internal/agent/serial/framing_test.go` 의 비이동 테스트 (특히 `SerialConnReader` 관련 테스트) 가 통과하도록 유지해야 한다.

**R3.6 (Ubiquitous)**: 시스템은 항상 시리얼 에이전트의 기존 `framing=frame` 경로의 관측 가능한 동작 (같은 바이트 입력에 대해 같은 프레임 시퀀스를 생성) 을 변경 없이 유지해야 한다.

**R3.7 (Unwanted)**: 시스템은 `pkg/framing` 로의 이동 과정에서 framer 알고리즘의 로직 (에러 처리, 버퍼 복구, CRC 검증 등) 을 변경해서는 안 된다.

**R3.8 (Unwanted)** (v1.1.0): 시스템은 `pkg/framing` 의 framer 구현이 호출자의 재사용 읽기 버퍼에 대한 슬라이스 참조(aliasing)를 프레임 결과로 반환하도록 해서는 안 된다. 각 framer 의 `Read`/`Drain` 가 반환하는 프레임 바이트는 호출자의 입력 버퍼와 독립된 backing array 를 소유해야 한다.

- `rawFramer.Read` 는 읽기 버퍼 `buf` 의 슬라이스를 그대로 반환하지 않고 세 인덱스 슬라이스 `buf[:n:n]` 로 backing array 의 capacity 를 봉인(cap)하여, 반환된 슬라이스에 대한 append 가 호출자 버퍼의 미사용 영역을 침범하지 않도록 한다.
- 이 제약은 한 번의 `Read` 로 분리된 여러 프레임이 동일 backing array 를 공유하여 먼저 전달된 프레임이 후속 파싱으로 변조되거나 메시지의 `raw` 가 `data` 와 어긋나는 cross-frame 오염을 방지하기 위한 것이다.
- 동일 결함의 다른 발현 지점인 소스 노드 계층(`serial-in`, `tcp-in`)의 방어적 복사는 각각 SPEC-SERIAL-001, SPEC-SOCKET-001 에서 다룬다.

---

### M4: 다중 스트림 버퍼 관리

**R4.1 (Ubiquitous)**: 시스템은 항상 framer 노드가 스트림 키별로 독립적인 `StreamBuffer` 인스턴스를 유지해야 한다. 각 `StreamBuffer` 는 자신의 `pkg/framing.Framer` 인스턴스, 누적 바이트 버퍼, 마지막 활동 시각, 프레임 카운터를 보유한다.

**R4.2 (Ubiquitous)**: 시스템은 항상 framer 노드의 스트림 버퍼 집합에 대한 접근 (조회, 생성, 삭제) 을 동시성 안전하게 수행해야 한다.

**R4.3 (State-Driven)**: IF `max_streams` 옵션이 양수이고 현재 스트림 수가 그 값에 도달한 상태에서 새로운 스트림 키에 대한 메시지가 도착하면 THEN 시스템은 `max_streams` 초과 정책 (plan.md 결정 (c) 참조) 에 따라 거부하거나 가장 오래된 버퍼를 축출해야 한다.

**R4.4 (State-Driven)**: IF `max_streams` 초과로 인해 새 스트림이 거부되면 THEN 시스템은 해당 메시지를 `error` 포트로 전달하고 에러 코드 `frame.buffer.max_streams_exceeded` 를 포함해야 한다.

**R4.5 (Optional)**: WHERE `stream_idle_timeout` 옵션이 양수이면, 시스템은 해당 시간 동안 활동이 없는 스트림 버퍼를 자동으로 삭제할 수 있다. 구현 방식 (백그라운드 고루틴 또는 Process 호출 시 lazy 검사) 은 plan.md 결정 (d) 에서 정한다.

**R4.6 (Event-Driven)**: WHEN framer 노드의 `Stop` 또는 `Close` 가 호출되면, THEN 시스템은 모든 스트림 버퍼를 flush 하고 각 버퍼에 미완성 바이트가 남아 있으면 `error` 포트로 `frame.buffer.incomplete_on_stop` 경고 메시지를 발행해야 한다.

**R4.7 (Ubiquitous)**: 시스템은 항상 각 스트림 버퍼가 자신의 `frame.index` 카운터를 독립적으로 유지하도록 해야 한다. 서로 다른 스트림 간에 인덱스가 공유되어서는 안 된다.

**R4.8 (Unwanted)**: 시스템은 한 스트림의 버퍼 오버플로나 파싱 에러가 다른 스트림의 버퍼 상태에 영향을 주도록 해서는 안 된다.

---

### M5: 기존 에이전트 프레이밍과의 공존 (이중 지원)

**R5.1 (Ubiquitous)**: 시스템은 항상 동일한 바이트 스트림 입력에 대해 다음 두 경로가 **동일한 프레임 시퀀스** 를 생성하도록 보장해야 한다:

- 경로 A: 시리얼 에이전트 `framing=frame` 으로 프레이밍 → 소스 노드 (serial-in) → downstream
- 경로 B: 시리얼 에이전트 `framing=raw` → 소스 노드 (serial-in) → framer 노드 `framing=frame` → downstream

**R5.2 (Ubiquitous)**: 시스템은 항상 경로 A 와 경로 B 의 동등성을 검증하는 **parity 테스트** 를 포함해야 한다. 테스트는 LG ICP-02 샘플 데이터 등 결정적 입력을 사용하여 두 경로가 생성한 프레임 시퀀스 (바이트 순서, 길이, 개수) 가 정확히 일치함을 확인해야 한다.

**R5.3 (Ubiquitous)**: 시스템은 항상 사용자가 선택 가능한 두 가지 배치 방식을 모두 지원해야 한다. 사용자는 기존 플로우를 변경하지 않고 새 framer 노드 방식으로 전환할 수 있어야 한다.

**R5.4 (Unwanted)**: 시스템은 framer 노드의 도입으로 인해 시리얼 에이전트의 기존 `framing=frame` 동작 (특히 LG ICP-02 샘플 처리) 에 부작용을 유발해서는 안 된다.

**R5.5 (Ubiquitous)**: 시스템은 항상 `TestFrameFramer_Icp02Samples` 회귀 테스트 (현재 `internal/agent/serial/framing_test.go` 에 존재) 를 리팩토링 후에도 통과하도록 유지해야 한다. 테스트는 `pkg/framing` 으로 이동될 수 있다.

---

### M6: Registry 등록 및 포트 정의

**R6.1 (Ubiquitous)**: 시스템은 항상 `internal/node/registry.go` 의 `registerBuiltins` 함수에 `framer` 타입을 등록해야 한다. 등록 시 팩토리 함수는 `NodeDef` 의 `Options` 를 검증하고 `pkg/framing.Options` 로 변환한 후 framer 노드 인스턴스를 생성해야 한다.

**R6.2 (Ubiquitous)**: 시스템은 항상 framer 노드의 등록 메타데이터에 다음 정보를 포함해야 한다:

- Type: `framer`
- Category: `processing`
- Description (한국어): "바이트 스트림에서 프로토콜 프레임을 분리하여 완성된 프레임을 출력"
- Default inputs: `["in"]`
- Default outputs: `["out"]`
- Default error outputs: `["error"]`

**R6.3 (Event-Driven)**: WHEN `NodeDef.Options` 에 유효하지 않은 옵션 값 (예: 음수 `max_message_size`, 잘못된 `length_endian`) 이 포함되어 있으면, THEN 팩토리는 명확한 에러 메시지와 함께 노드 생성을 실패시켜야 한다.

**R6.4 (Ubiquitous)**: 시스템은 항상 framer 노드의 라이프사이클 (`Start`, `Stop`) 이 `BaseNode` 의 기존 패턴을 따르도록 구현해야 한다.

---

### M7: 에러 분류 및 관측성

**R7.1 (Ubiquitous)**: 시스템은 항상 framer 노드의 에러를 다음 코드 체계로 분류해야 한다:

| 에러 코드 | 발생 조건 | 복구 정책 |
|-----------|----------|-----------|
| `frame.input.invalid_payload` | 페이로드에 `raw`, `data` 키 모두 없거나 디코딩 실패 | 해당 메시지 무시, 버퍼 영향 없음 |
| `frame.parse.etx_mismatch` | frame 모드에서 ETX 마커 불일치 | 다음 STX 까지 skip |
| `frame.parse.checksum_mismatch` | frame 모드에서 CRC/checksum 불일치 | 다음 STX 까지 skip |
| `frame.parse.max_size_exceeded` | 버퍼가 `max_message_size` 를 초과 | 버퍼 복구 정책은 plan.md 결정 (e) 참조 |
| `frame.parse.length_invalid` | length_prefix 모드에서 길이 필드가 유효하지 않음 | plan.md 결정 (e) 참조 |
| `frame.buffer.max_streams_exceeded` | `max_streams` 상한 초과 (거부 정책) | 해당 메시지 거부, 기존 스트림 영향 없음 |
| `frame.buffer.incomplete_on_stop` | `Stop` 호출 시 미완성 버퍼 존재 | 경고 발행 후 버퍼 삭제 |

**R7.2 (Event-Driven)**: WHEN 프레이밍 파싱 에러가 발생하면, THEN 시스템은 에러 메시지를 생성하여 `error` 포트로 전달해야 한다. 에러 메시지의 메타데이터에는 `frame.error.code`, `frame.framer_type`, `frame.stream_key`, `frame.buffer.bytes_at_error` 필드가 포함되어야 한다.

**R7.3 (Ubiquitous)**: 시스템은 항상 프레임 조립 성공 시 DEBUG 로그로 `framer_type`, `stream_key`, `frame_size`, `frame_index` 를 기록해야 한다.

**R7.4 (Ubiquitous)**: 시스템은 항상 프레이밍 파싱 에러 발생 시 WARN 로그로 에러 코드와 버퍼 상태를 기록해야 한다.

**R7.5 (Ubiquitous)**: 시스템은 항상 매 Process 호출에 대해 DEBUG 로그로 입력 바이트 수와 출력 프레임 수를 기록해야 한다.

**R7.6 (Optional)**: WHERE 관측성 메트릭 시스템이 활성화되어 있으면, 시스템은 framer 노드의 스트림별 누적 프레임 수, 누적 에러 수, 현재 버퍼 크기를 통계로 노출할 수 있다.

---

## 5. Out of Scope (범위 외)

본 SPEC 에서 다루지 않는 항목은 다음과 같다:

1. **Write 방향 프레이밍**: 프레임 생성 (바이트 스트림으로의 직렬화) 은 본 SPEC 의 범위 외이다. `pkg/framing.Framer.Write` 는 인터페이스에 포함되지만 노드 차원에서는 수신 전용이다. 송신 framer 노드가 필요하면 별도 SPEC 으로 분리한다.
2. **Modbus TCP, MQTT 등 이미 전용 파서를 가진 프로토콜**: Modbus, MQTT 메시지 구조는 이미 dedicated 노드/에이전트가 담당한다. framer 노드는 이들을 대체하지 않는다.
3. **TCP 소켓 에이전트의 framing 모드 추가**: `internal/agent/socket/framing.go` 에 stream, frame 모드를 추가하는 작업은 본 SPEC 에서 수행하지 않는다. framer 노드가 이를 플로우 그래프 차원에서 해결하기 때문이다.
4. **다중 스트림의 동적 포트 라우팅**: 각 스트림 키를 서로 다른 `out` 포트로 분기하는 기능은 본 SPEC 의 범위 외이다. 본 SPEC 의 framer 노드는 단일 `out` 포트만 제공하며, `frame.stream_key` 메타데이터로 downstream 라우팅은 가능하다.
5. **CRC-16 / XMODEM 등 신규 checksum 알고리즘 추가**: `frameFramer` 는 현재 `sum8`, `xor` 만 지원한다. LG ICP-02 의 CRC 검증이 필요한 경우 framer 노드의 downstream 에 배치된 전용 파서 노드가 담당한다.
6. **TCP 서버의 connection_id 메타데이터 주입 기본 활성화**: `TCPInNode` 서버 모드가 모든 경우에 `connection_id` 를 주입하도록 수정하는 작업은 본 SPEC 의 범위 외일 수 있다. plan.md 결정 (f) 에서 본 SPEC 에 포함할지 별도 SPEC 으로 분리할지 정한다.
7. **pkg/framing 이후의 프로토콜별 framer 추가**: ASN.1, Protobuf delimited, SLIP, HDLC 등 신규 framer 모드는 본 SPEC 이후의 별도 SPEC 으로 처리한다.
8. **flow 저장소 스키마 마이그레이션**: framer 노드는 신규 노드이므로 기존 플로우에 영향이 없으며 별도 마이그레이션이 필요하지 않다.
9. **Web UI 에 framer 노드 팔레트 항목 추가**: UI 변경은 본 SPEC 의 범위 외이며, 필요 시 별도 UI 전용 SPEC 에서 다룬다. 본 SPEC 은 빌더 API 와 플로우 JSON 레벨에서 framer 노드 사용을 가능하게 하는 데 집중한다.
10. **다중 입력 포트**: 본 SPEC 의 framer 노드는 단일 `in` 포트만 지원한다. 여러 소스를 merge 하려면 별도 merge 노드를 front 에 배치한다.

---

## 6. Non-Functional Requirements (비기능 요구사항)

### 6.1 하위 호환성 (Backward Compatibility)

- **NFR1**: 본 SPEC 적용 후에도 시리얼 에이전트의 `framing=frame` 경로는 기존과 동일한 외부 관측 동작을 보여야 한다. 특히 LG ICP-02 샘플 회귀 테스트 (`TestFrameFramer_Icp02Samples`) 는 통과해야 한다.
- **NFR2**: 기존 `internal/agent/serial/framing_test.go` 의 모든 테스트 (`SerialConnReader` 포함) 는 리팩토링 후에도 통과해야 한다. 테스트 자체는 `pkg/framing` 으로 일부 이동할 수 있으나 검증 범위는 감소하지 않아야 한다.
- **NFR3**: 기존 플로우 저장소 데이터와 에이전트 설정 스키마는 변경되지 않는다.

### 6.2 동시성 안전 (Concurrency Safety)

- **NFR4**: framer 노드의 스트림 버퍼 접근은 동시성 안전해야 하며, `go test -race ./internal/node/... ./pkg/framing/...` 이 통과해야 한다.
- **NFR5**: 한 스트림 버퍼의 읽기/쓰기와 다른 스트림 버퍼의 읽기/쓰기는 서로 블록하지 않아야 한다 (스트림 독립성).

### 6.3 성능 (Performance)

- **NFR6**: framer 노드의 Process 오버헤드는 입력 바이트 크기에 선형적이어야 하며, 추가 메모리 할당은 내부 버퍼 append 와 프레임 복사 외에 최소화되어야 한다.
- **NFR7**: 단일 스트림에서 초당 10000 프레임 이상의 조립 처리량을 유지해야 한다 (프레임당 평균 64바이트 기준, 개발 장비에서의 단위 테스트 벤치마크 기준).

### 6.4 자원 제한 (Resource Limits)

- **NFR8**: `max_message_size` 를 통해 단일 프레임의 최대 크기가 상한에 의해 제한되어야 한다.
- **NFR9**: `max_streams` 를 통해 동시 활성 스트림 수가 상한에 의해 제한되어야 한다.
- **NFR10**: `stream_idle_timeout` 을 통해 유휴 스트림이 자동 제거될 수 있어야 한다 (선택적).

### 6.5 관측성 (Observability)

- **NFR11**: 프레이밍 파싱 에러는 에러 코드, 스트림 키, 버퍼 크기와 함께 WARN 레벨로 기록되어야 한다.
- **NFR12**: 프레임 조립 성공은 스트림 키, 프레임 크기, 인덱스와 함께 DEBUG 레벨로 기록되어야 한다.

### 6.6 테스트 커버리지 (Test Coverage)

- **NFR13**: `pkg/framing` 의 라인 커버리지는 기존 `internal/agent/serial/framing.go` 와 동등하거나 그 이상이어야 한다.
- **NFR14**: `internal/node/framer.go` 의 신규 코드 라인 커버리지는 85% 이상이어야 한다.
- **NFR15**: 시리얼 에이전트 vs framer 노드 parity 테스트는 CI 에서 항상 실행되어야 한다.

### 6.7 신뢰성 (Reliability)

- **NFR16**: 프레이밍 파싱 에러가 발생해도 framer 노드 인스턴스는 계속 동작 가능한 상태를 유지해야 한다. 노드 자체가 중단되어서는 안 된다.
- **NFR17**: `Stop` 호출은 idempotent 이어야 하며 여러 번 호출해도 에러 없이 동작해야 한다.

---

## 7. Traceability

| 요구사항 | 구현 모듈 | 테스트 | 비고 |
|----------|-----------|--------|------|
| M1 (R1.1~R1.8) | `internal/node/framer.go`, `internal/node/registry.go` | `framer_test.go` (팩토리/옵션) | 도메인 모델 및 등록 |
| M2 (R2.1~R2.14) | `internal/node/framer.go` | `framer_test.go` (Process 동작) | 프로세스 동작 명세 |
| M3 (R3.1~R3.8) | `pkg/framing/*.go`, `internal/agent/serial/framing.go` | `pkg/framing/framing_test.go` (이동 포함) | 패키지 승격, 버퍼 aliasing 방지 (R3.8, v1.1.0) |
| M4 (R4.1~R4.8) | `internal/node/framer.go` (StreamBuffer, StreamMap) | `framer_test.go` (multi-stream) | 다중 스트림 관리 |
| M5 (R5.1~R5.5) | 교차 의존 (pkg/framing, serial agent, framer 노드) | `framer_parity_test.go` | 이중 지원 parity |
| M6 (R6.1~R6.4) | `internal/node/registry.go` | `registry_test.go` (등록 검증) | Registry 등록 |
| M7 (R7.1~R7.6) | `internal/node/framer.go` (에러 라우팅, 로깅) | `framer_test.go` (error port) | 에러 및 관측성 |

---

## 8. References (참조)

- **프레이밍 엔진 관련 파일**:
  - `internal/agent/serial/framing.go` (1~471): 현재 framer 구현
  - `internal/agent/serial/framing_test.go` (1~1256): 현재 유닛 테스트 및 LG ICP-02 회귀
  - `internal/agent/serial/common.go`: 프레이밍 모드 상수
- **노드 시스템 관련 파일**:
  - `internal/node/registry.go` (1~203): 노드 레지스트리 및 `registerBuiltins`
  - `internal/node/base.go`: 노드 공통 라이프사이클
  - `internal/node/transform.go`, `filter.go`: 기존 처리 노드 패턴 참조
  - `internal/node/serial_io.go`, `tcp_io.go`: 소스 노드 페이로드 규약
- **메시지 및 플로우 계층**:
  - `pkg/message/payload.go`: `Payload` 인터페이스
  - `pkg/flow/flow.go`: `NodeDef` 구조
- **관련 SPEC**:
  - SPEC-NODE-001: Node 시스템 기본 설계 (SPEC-NODE-002 는 이 위에 구축됨)
  - SPEC-SERIAL-001: 시리얼 에이전트 기본 설계
  - SPEC-SOCKET-001: Socket 에이전트 기본 설계
  - SPEC-AGENT-006: Transport 에이전트 설정 분리 (Configure 재파싱 및 hot-reload 패턴 참조)
  - SPEC-FLOW-001: Flow 그래프 실행 및 노드 배치
- **EARS Format**: Easy Approach to Requirements Syntax (Mavin, 2009)
- **최근 관련 커밋**:
  - `bbc024e`: `TestFrameFramer_Icp02Samples` LG ICP-02 검증 테스트 추가

---

## 9. Implementation Notes (구현 메모)

상태: `completed` - 전 Phase (0~6) 구현 완료.

### 9.1 계획 대비 실제 구현 요약

SPEC 의 M1~M7 요구사항과 plan.md 의 Phase 0~5 를 모두 구현하였다. 핵심 설계 결정 (a)~(g) 은 plan.md 에서 선택한 옵션 그대로 적용되었다.

### 9.2 계획과의 차이 (Divergences)

1. **Drain API 추가 (SPEC 범위 외, 구현 중 필요성 확인)**
   - `pkg/framing.Framer` 인터페이스에 `Drain(buf []byte) [][]byte` 메서드를 추가하였다. SPEC 의 R3.1 인터페이스 정의에는 `Read`/`Write` 만 명시되어 있었으나, framer 노드의 rolling-buffer 모델에서 `bytes.Reader` 를 매 호출마다 생성하면 `bufio.Scanner` 기반 framer 의 내부 상태가 초기화되는 문제가 발생하여 `Drain` API 가 필요하였다. `Drain` 은 누적 버퍼를 직접 받아 0개 이상의 완성 프레임을 반환하고, 소비된 바이트를 caller 가 추적할 수 있게 한다.

2. **ScannerConfigurer 인터페이스 추가 (SerialConnReader 호환)**
   - `pkg/framing` 에 `ScannerConfigurer` 인터페이스를 추가하여 `bufio.Scanner` 의 `SplitFunc` 과 `MaxTokenSize` 를 설정할 수 있게 하였다. 이는 기존 `SerialConnReader` 가 `bufio.Scanner` 를 통해 framer 를 사용하는 경로를 `pkg/framing` 위임으로 전환하기 위해 필요하였다.

3. **에러 포트의 메타데이터 마커 방식 (엔진 레벨 다중 포트 라우팅 미적용)**
   - SPEC R1.3 에서 `error` 출력 포트를 정의하였으나, 현재 엔진(`internal/engine/engine.go`)은 `Process` 반환값의 `[]message.Message` 를 모두 동일한 wire 로 전달하는 구조이므로 엔진 레벨 다중 포트 라우팅은 구현하지 않았다. 대신 에러 메시지에 `_port: "error"` 메타데이터 마커를 설정하고, 엔진이 이를 인식하여 별도 wire 로 라우팅하는 방식을 사용한다. 엔진 레벨의 본격적인 다중 출력 포트 지원은 후속 과제로 남긴다.

4. **stream 모드의 framer 노드 동작 = passthrough**
   - `stream` 프레이밍 모드는 시리얼 에이전트에서 idle-timeout 기반으로 스트림 종료를 판단하는 모드이다. framer 노드에서는 idle-timeout 기반 스트림 종료를 구현하지 않고 passthrough (입력 바이트를 그대로 출력) 로 동작한다. 이는 `raw` 모드와 실질적으로 동일하다.

5. **TCP 서버 connection_id 주입 미포함 (결정 (f) 전략 2 적용)**
   - plan.md 결정 (f) 에서 전략 2 (별도 SPEC 으로 분리) 를 선택하였으므로, TCP 서버 소스 노드의 `connection_id` 메타데이터 주입은 본 SPEC 에 포함되지 않았다. framer 노드는 `connection_id` 가 없는 경우 단일 공용 버퍼로 정상 동작한다. TCP 서버 다중 연결 시나리오에서의 연결별 프레이밍은 후속 SPEC (SPEC-NODE-003 등) 에서 다룬다.

### 9.2.1 구현 후 버그 수정

6. **ETX 선택 사항 처리** (`4ba4566`)
   - `frame` 모드에서 ETX 가 빈 값일 때 `ErrETXMismatch` 가 발생하는 문제를 수정하였다. ETX 가 설정되지 않은 프레임 프로토콜(STX + Length Prefix 만 사용하는 경우 등)을 지원하기 위해 ETX 검증을 조건부로 수행한다.

7. **LengthSize 기본값 보정** (`ab05dd8`)
   - `length_prefix` 모드에서 `length_size` 미지정 시 값이 0 으로 해석되어 프레이밍이 실패하는 문제를 수정하였다. 기본값 2 (16-bit big-endian) 를 적용한다.

8. **configInt 문자열 타입 처리** (`6ecff54`)
   - `framer_factory.go` 의 설정 파싱에서 YAML/JSON 디코딩 시 정수 필드가 `string` 타입으로 전달되는 경우를 처리하지 못하는 문제를 수정하였다. `strconv.Atoi` 폴백을 추가하여 `"2"` → `2` 변환을 지원한다.

9. **rawFramer 버퍼 aliasing 수정 (v1.1.0, R3.8 신설)** (`b2a3ed3`)
   - `pkg/framing/raw.go` 의 `rawFramer.Read` 가 재사용 읽기 버퍼 `buf` 의 슬라이스 `buf[:n]` 을 그대로 반환하여, 반환된 프레임이 후속 `Read` 또는 append 로 변조될 수 있는 결함을 수정하였다. 세 인덱스 슬라이스 `buf[:n:n]` 로 backing array 의 capacity 를 봉인하여 호출자 버퍼와의 aliasing 을 차단한다. 본 수정으로 R3.8 (Unwanted) 요구사항을 신설하였다. 동일 커밋에서 `internal/agent/socket/framing.go` 의 `newlineFramer` 와 `internal/node/tcp_io.go` 의 `TCPInNode` 도 방어적 복사로 수정되었다 (각각 SPEC-SOCKET-001 v1.2.0 참조).

### 9.3 품질 게이트 결과

- `go build ./...`: 통과
- `go vet ./...`: 통과
- `go test ./...`: 전체 통과
- `go test -race ./...`: 전체 통과 (race condition 없음)
- `pkg/framing` 커버리지: 87.0%
- `internal/node/framer.go` + `framer_factory.go` 평균 함수 커버리지: 89.3%
- Parity 테스트 (`TestFramerParity_*`): 통과
