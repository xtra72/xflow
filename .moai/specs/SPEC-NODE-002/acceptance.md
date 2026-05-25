---
id: SPEC-NODE-002
version: "1.1.0"
status: draft
created: "2026-04-10"
updated: "2026-05-14"
author: xtra
priority: high
tags: [node, framing, stream, processing]
related_spec: SPEC-NODE-001, SPEC-SERIAL-001, SPEC-SOCKET-001, SPEC-AGENT-006, SPEC-FLOW-001
---

# SPEC-NODE-002: Acceptance Criteria - Framer 노드

본 문서는 SPEC-NODE-002 의 인수 기준을 Given-When-Then 형식으로 정의한다. 각 시나리오는 자동화된 Go 테스트로 검증 가능해야 한다. 테스트 파일 경로는 참고용이며, 최종 위치는 구현 단계에서 정할 수 있다.

---

## 1. M1: Framer 노드 도메인 모델 및 등록

### AC1.1: Registry 에 framer 타입 등록

**Given** 빌트인이 모두 등록된 `internal/node.Registry` 인스턴스
**When** `Registry.Get("framer")` 를 호출하면
**Then**:
- nil 이 아닌 팩토리 함수를 반환해야 한다
- 카테고리는 "processing" 이어야 한다

### AC1.2: 기본 포트 정의

**Given** 유효한 옵션으로 생성된 framer 노드 인스턴스
**When** 노드의 기본 포트 정보를 조회하면
**Then**:
- `inputs` 는 `["in"]` 이어야 한다
- `outputs` 는 `["out"]` 을 포함해야 한다
- `error outputs` 는 `["error"]` 을 포함해야 한다

### AC1.3: 필수 framing 옵션 누락 시 팩토리 에러

**Given** `NodeDef.Options` 에 `framing` 키가 없는 정의
**When** framer 노드 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러 메시지는 `framing` 옵션 누락을 명확히 설명해야 한다

### AC1.4: 유효하지 않은 framing 값

**Given** `framing="unknown_mode"` 옵션
**When** 팩토리를 호출하면
**Then**:
- non-nil 에러를 반환해야 한다
- 에러 메시지는 허용 값 목록 (`raw`, `newline`, `length_prefix`, `fixed_size`, `stream`, `frame`) 을 포함해야 한다

### AC1.5: 옵션 키 이름이 시리얼 에이전트와 일치

**Given** 다음 옵션 셋: `framing=frame`, `stx="0x02"`, `etx="0x03"`, `length_offset=3`, `length_size=2`, `length_endian="big"`, `max_message_size=1024`, `checksum="sum8"`
**When** 팩토리를 호출하면
**Then**:
- 노드 생성이 성공해야 한다
- 내부의 `pkg/framing.Options` 필드가 위 값들을 정확히 반영해야 한다

---

## 2. M2: Process 동작 (단일 스트림)

### AC2.1: 단일 프레임 조립 - newline 모드

**Given** framer 노드 (`framing=newline`), 페이로드 `raw=[]byte("hello\n")`
**When** `Process` 를 호출하면
**Then**:
- 길이 1 의 메시지 배열을 반환해야 한다
- 반환된 메시지의 `raw` 값은 `[]byte("hello\n")` (또는 구현 정책에 따라 delimiter 제외한 `[]byte("hello")`) 이어야 한다
- 반환된 메시지의 `data` 값은 위 바이트의 hex 인코딩이어야 한다

> 구현 정책: delimiter 포함 여부는 `pkg/framing.newlineFramer` 의 기존 동작을 그대로 유지한다.

### AC2.2: 여러 프레임 한 번에 조립

**Given** framer 노드 (`framing=newline`), 페이로드 `raw=[]byte("a\nbb\nccc\n")`
**When** `Process` 를 호출하면
**Then**:
- 길이 3 의 메시지 배열을 반환해야 한다
- 각 메시지의 `raw` 는 순서대로 `a`, `bb`, `ccc` 에 대응하는 바이트이어야 한다
- 각 메시지의 메타데이터 `frame.index` 는 순서대로 0, 1, 2 이어야 한다

### AC2.3: 미완성 꼬리 버퍼 보존

**Given** framer 노드 (`framing=newline`)
**When** 첫 번째 Process 호출에 `raw=[]byte("a\nbb")` 주입하고
**Then** 길이 1 의 메시지 배열을 반환해야 한다 (`a`)
**When** 두 번째 Process 호출에 `raw=[]byte("\nccc\n")` 주입하면
**Then** 길이 2 의 메시지 배열을 반환해야 한다 (`bb`, `ccc`)
**And** 두 번째 호출에서 반환된 첫 번째 프레임의 `frame.index` 는 1 이어야 한다

### AC2.4: LGCP 샘플 - frame 모드

**Given** framer 노드 (`framing=frame`, LGCP 옵션), 페이로드가 LGCP 샘플 3개를 이어붙인 바이트 배열
**When** `Process` 를 호출하면
**Then**:
- 길이 3 의 메시지 배열을 반환해야 한다
- 각 메시지의 `raw` 는 원본 LGCP 프레임과 일치해야 한다
- 순서가 보존되어야 한다

### AC2.5: length_prefix 빅엔디안 2프레임

**Given** framer 노드 (`framing=length_prefix`, `length_size=4`, `length_endian="big"`)
**And** 페이로드에 4바이트 길이 (빅엔디안 `0x00000003`) + 3바이트 payload + 4바이트 길이 (`0x00000002`) + 2바이트 payload 이어진 바이트
**When** `Process` 를 호출하면
**Then**:
- 길이 2 의 메시지 배열을 반환해야 한다
- 첫 번째 프레임은 3바이트, 두 번째는 2바이트이어야 한다

### AC2.6: 빈 바이트 입력

**Given** framer 노드 (`framing=newline`), 페이로드 `raw=[]byte{}`
**When** `Process` 를 호출하면
**Then**:
- 빈 메시지 배열과 nil 에러를 반환해야 한다

### AC2.7: raw 키 대신 data 키 사용

**Given** framer 노드 (`framing=newline`), 페이로드에 `raw` 키는 없고 `data="68656c6c6f0a"` (hex for "hello\n") 만 존재
**When** `Process` 를 호출하면
**Then**:
- 길이 1 의 메시지 배열을 반환해야 한다 (hex 디코딩 후 프레이밍)

### AC2.8: raw 와 data 모두 없음 → error 포트

**Given** framer 노드, 페이로드에 `raw` 와 `data` 키가 모두 없음
**When** `Process` 를 호출하면
**Then**:
- 반환된 메시지 중 `out` 포트로 갈 메시지는 0개
- `error` 포트로 갈 메시지가 1개 존재
- 에러 메시지의 메타데이터 `frame.error.code` 는 `frame.input.invalid_payload` 이어야 한다

### AC2.9: 업스트림 메타데이터 보존

**Given** 입력 메시지의 메타데이터에 `source_addr="192.168.1.10"`, `timestamp="2026-04-10T10:00:00Z"` 가 존재
**And** 페이로드는 `raw=[]byte("hello\n")`
**When** framer 노드의 `Process` 를 호출하면
**Then**:
- 반환된 출력 메시지의 메타데이터에 `source_addr`, `timestamp` 가 그대로 보존되어야 한다
- 추가로 `frame.index=0`, `frame.framer_type="newline"`, `frame.stream_key=""` 메타가 설정되어야 한다

### AC2.10: out 포트는 파싱 에러로 오염되지 않음

**Given** framer 노드 (`framing=frame`, 특정 STX/ETX)
**And** 페이로드가 "정상 프레임 1개 + 손상된 바이트 + 정상 프레임 1개" 형태
**When** `Process` 를 호출하면
**Then**:
- `out` 포트로 가는 메시지에는 정상 프레임만 포함되어야 한다 (프레임 복구 후의 정상 프레임 포함)
- 손상된 바이트에 대해서는 `error` 포트로 메시지가 전달되어야 한다
- `out` 포트 메시지에는 에러 플래그나 에러 코드가 없어야 한다

---

## 3. M3: pkg/framing 패키지 승격

### AC3.1: pkg/framing 공개 API 존재

**Given** 리팩토링이 완료된 상태
**When** `pkg/framing` 패키지의 public API 를 확인하면
**Then**:
- `Framer` 인터페이스가 존재해야 한다
- `Options` 구조체가 존재해야 한다
- `New(mode string, opts Options) (Framer, error)` 함수가 존재해야 한다
- 모드 상수 `ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame` 가 존재해야 한다

### AC3.2: LGCP 회귀 테스트 통과

**Given** Phase 0 리팩토링 완료 후 상태
**When** `TestFrameFramer_LGCPSamples` (위치: `pkg/framing/framing_test.go` 또는 `internal/agent/serial/framing_test.go`) 를 실행하면
**Then**:
- 테스트가 통과해야 한다
- 모든 LGCP 샘플이 정확히 동일한 프레임으로 조립되어야 한다

### AC3.3: SerialConnReader 테스트 보존

**Given** Phase 0 리팩토링 완료 후 상태
**When** `internal/agent/serial/framing_test.go` 의 `SerialConnReader` 관련 테스트를 실행하면
**Then**:
- 모든 테스트가 통과해야 한다

### AC3.4: 시리얼 에이전트 framing=frame 경로 동작 보존

**Given** 리팩토링 완료 후 상태
**And** 시리얼 에이전트가 `framing=frame` 옵션으로 동작 중
**When** LGCP 샘플 바이트 스트림이 입력으로 주어지면
**Then**:
- 에이전트가 생성하는 프레임 시퀀스는 리팩토링 이전과 동일해야 한다 (characterization test 기준)

### AC3.5: pkg/framing 이 시리얼 패키지에 역의존성 없음

**Given** `pkg/framing` 패키지
**When** import 그래프를 확인하면
**Then**:
- `pkg/framing` 은 `internal/agent/serial` 또는 xflow 의 어떤 internal 패키지도 import 해서는 안 된다
- `pkg/framing` 은 standard library 와 `pkg/` 내부 패키지만 의존해야 한다

### AC3.6: rawFramer 가 호출자 버퍼와 aliasing 되지 않음 (R3.8, v1.1.0)

**Given** `pkg/framing.New(ModeRaw, opts)` 로 생성한 framer
**When** 재사용 읽기 버퍼 `buf` 를 통해 `Read` 를 호출하여 `n` 바이트 프레임을 얻으면
**Then**:
- 반환된 슬라이스는 `buf[:n:n]` 형태로 capacity 가 `n` 으로 봉인되어야 한다
- 반환된 슬라이스에 append 를 수행해도 `buf` 의 미사용 영역(`buf[n:]`)이 변조되어서는 안 된다
- 후속 `Read` 가 `buf` 를 덮어써도 이전에 반환된 프레임 바이트가 변조되어서는 안 된다

---

## 4. M4: 다중 스트림 버퍼 관리

### AC4.1: connection_id 키로 독립 버퍼 분리

**Given** framer 노드 (`framing=newline`, `stream_key_metadata="connection_id"`)
**When** 다음 메시지들을 순차적으로 Process 에 주입하면:
- msg1: metadata `connection_id="A"`, raw `"aa"`
- msg2: metadata `connection_id="B"`, raw `"bb"`
- msg3: metadata `connection_id="A"`, raw `"\n"`
- msg4: metadata `connection_id="B"`, raw `"\n"`
**Then**:
- msg1, msg2 의 Process 는 빈 결과를 반환해야 한다
- msg3 의 Process 는 1개 프레임 (`aa`) 을 반환해야 한다
- msg4 의 Process 는 1개 프레임 (`bb`) 을 반환해야 한다
- 반환된 각 프레임의 `frame.stream_key` 메타가 각각 `A`, `B` 이어야 한다

### AC4.2: frame.index 가 스트림별 독립

**Given** AC4.1 과 동일한 설정
**When** connection_id=A 로 2개 프레임, connection_id=B 로 2개 프레임이 완성되도록 메시지를 주입하면
**Then**:
- A 스트림의 두 프레임의 `frame.index` 는 각각 0, 1 이어야 한다
- B 스트림의 두 프레임의 `frame.index` 도 각각 0, 1 이어야 한다

### AC4.3: stream_key_metadata 없을 때 단일 공용 버퍼

**Given** framer 노드 (`framing=newline`, `stream_key_metadata="connection_id"`)
**When** 메타데이터에 `connection_id` 가 없는 메시지를 주입하면
**Then**:
- 내부적으로 단일 공용 버퍼 (스트림 키는 빈 문자열) 로 처리해야 한다
- 반환된 프레임의 `frame.stream_key` 는 빈 문자열이어야 한다

### AC4.4: max_streams 초과 시 reject

**Given** framer 노드 (`framing=newline`, `max_streams=2`)
**And** connection_id=A, B 에 대한 스트림 버퍼가 이미 존재
**When** connection_id=C 인 메시지를 Process 에 주입하면
**Then**:
- 반환된 메시지 중 `out` 포트 메시지는 0개
- `error` 포트로 메시지가 1개 전달되어야 한다
- 에러 메시지의 `frame.error.code` 는 `frame.buffer.max_streams_exceeded` 이어야 한다
- A, B 스트림의 기존 버퍼 상태는 영향받지 않아야 한다

### AC4.5: stream_idle_timeout lazy 삭제

**Given** framer 노드 (`framing=newline`, `stream_idle_timeout=100ms`)
**And** connection_id=A 스트림 버퍼에 일부 미완성 바이트가 존재
**When** 150ms 경과 후 connection_id=B 로 새 메시지를 Process 에 주입하면
**Then**:
- Process 호출의 초입에서 idle 감지 로직이 A 스트림을 삭제해야 한다
- A 스트림의 미완성 바이트는 손실된다 (이는 timeout 의 의도된 동작)
- 이후 connection_id=A 로 새 메시지가 오면 새 스트림 버퍼가 생성된다

### AC4.6: Stop 시 미완성 버퍼 경고

**Given** framer 노드가 running 중이며 connection_id=A 에 `raw=[]byte("partial")` 이 버퍼에 남아 있음 (delimiter 없음)
**When** 노드의 `Stop` 이 호출되면
**Then**:
- `error` 포트로 메시지가 전달되어야 한다
- 에러 메시지의 `frame.error.code` 는 `frame.buffer.incomplete_on_stop` 이어야 한다
- 에러 메시지의 메타에 `frame.stream_key="A"` 와 미완성 바이트 크기 (`frame.buffer.bytes_at_error=7`) 가 포함되어야 한다
- Stop 은 성공적으로 완료되어야 한다

### AC4.7: 스트림 간 오염 없음

**Given** framer 노드 (`framing=frame`), A 와 B 스트림 존재
**When** A 스트림으로 손상된 바이트 (ETX 불일치) 가 주입되면
**Then**:
- A 스트림에 대해서만 error 메시지가 발행되어야 한다
- B 스트림의 버퍼 상태는 영향받지 않아야 한다
- B 로 이어 주입되는 정상 바이트는 정상 프레임으로 조립되어야 한다

---

## 5. M5: Parity (시리얼 에이전트 vs Framer 노드)

### AC5.1: LGCP 샘플 parity

**Given** LGCP 샘플 바이트 스트림 (기존 `TestFrameFramer_LGCPSamples` 와 동일한 입력)
**When** 다음 두 경로로 프레임 시퀀스를 생성하면:
- 경로 A: `pkg/framing.New(ModeFrame, opts)` 로 framer 직접 생성 후 `Read` 반복 호출
- 경로 B: framer 노드 (`framing=frame`, 동일 opts) 의 `Process` 를 호출 (단일 Process 호출로 전체 바이트 주입)
**Then**:
- 두 경로의 프레임 시퀀스 길이가 동일해야 한다
- 각 인덱스의 프레임 바이트가 정확히 일치해야 한다

### AC5.2: 청크 분할 stress - newline 모드

**Given** 여러 프레임이 포함된 바이트 스트림 (예: `"a\nbb\nccc\ndddd\n"`)
**When** 다음 두 경로로 프레임 시퀀스를 생성하면:
- 경로 A: framer 노드에 한 번의 Process 호출로 전체 바이트 주입
- 경로 B: framer 노드에 1바이트씩 나눠서 Process 를 반복 호출
**Then**:
- 두 경로의 누적 출력 프레임 시퀀스가 정확히 일치해야 한다

### AC5.3: 청크 분할 stress - length_prefix 모드

**Given** 여러 length_prefix 프레임이 포함된 바이트 스트림
**When** 경로 A (전체 한 번에) 와 경로 B (1바이트씩) 로 처리하면
**Then**:
- 결과 프레임 시퀀스가 정확히 일치해야 한다

### AC5.4: 에러 복구 parity - frame 모드 ETX 불일치

**Given** `"정상 프레임 1 + 손상 + 정상 프레임 2"` 형태의 바이트 스트림
**When** 경로 A (serial agent `framing=frame` 경로) 와 경로 B (에이전트 `raw` + framer 노드 `framing=frame` 경로) 로 처리하면
**Then**:
- 두 경로에서 추출되는 정상 프레임 시퀀스가 일치해야 한다
- 두 경로에서 발생하는 에러의 종류 (ETX mismatch) 가 일치해야 한다

### AC5.5: 시리얼 에이전트 기존 프레이밍 경로 불변

**Given** 리팩토링 완료 후 상태
**When** 시리얼 에이전트를 `framing=frame` 으로 설정하고 LGCP 장비와 통신 (또는 mock 포트 대체) 하면
**Then**:
- 에이전트가 방출하는 메시지의 `raw`, `data` 필드와 메타데이터가 리팩토링 이전과 관측 가능한 범위에서 동일해야 한다

---

## 6. M6: Registry 및 포트 정의

### AC6.1: 팩토리 옵션 파싱 - 유효 케이스

**Given** `NodeDef.Options` = `{"framing": "length_prefix", "length_size": 4, "length_endian": "big", "max_message_size": 4096}`
**When** framer 노드 팩토리를 호출하면
**Then**:
- non-nil 노드와 nil 에러를 반환해야 한다
- 노드 내부의 `pkg/framing.Options` 가 위 값을 정확히 반영해야 한다

### AC6.2: 팩토리 옵션 파싱 - length_endian 오타

**Given** `NodeDef.Options` = `{"framing": "length_prefix", "length_size": 4, "length_endian": "middle"}`
**When** 팩토리를 호출하면
**Then**:
- non-nil 에러를 반환해야 한다
- 에러 메시지는 `length_endian` 의 허용 값을 언급해야 한다

### AC6.3: 팩토리 옵션 파싱 - 음수 max_message_size

**Given** `NodeDef.Options` = `{"framing": "frame", "max_message_size": -1, ...}`
**When** 팩토리를 호출하면
**Then**:
- non-nil 에러를 반환해야 한다

### AC6.4: 노드 라이프사이클 - Start/Stop

**Given** 유효한 옵션으로 생성된 framer 노드
**When** `Start(ctx)` → `Process(ctx, msg)` → `Stop(ctx)` 을 순차 호출하면
**Then**:
- 모든 호출이 에러 없이 완료되어야 한다
- Start 전에 Process 호출 시 동작은 BaseNode 관례를 따른다

### AC6.5: Stop 의 idempotency

**Given** running 상태의 framer 노드
**When** `Stop(ctx)` 을 연속으로 두 번 호출하면
**Then**:
- 두 번째 호출도 에러 없이 (또는 이미 stopped 상태를 나타내는 관례적 결과로) 완료되어야 한다

---

## 7. M7: 에러 분류 및 관측성

### AC7.1: ETX 불일치 에러 코드 (frame 모드)

**Given** framer 노드 (`framing=frame`, 특정 STX/ETX 설정)
**And** 페이로드가 `"STX + 정상 길이 필드 + 잘못된 ETX"` 형태
**When** Process 를 호출하면
**Then**:
- `error` 포트로 메시지가 1개 전달되어야 한다
- 메타데이터의 `frame.error.code` 는 `frame.parse.etx_mismatch` 이어야 한다
- 메타데이터에 `frame.framer_type="frame"`, `frame.stream_key=""`, `frame.buffer.bytes_at_error=` (숫자) 가 포함되어야 한다

### AC7.2: max_size_exceeded 에러 코드 (length_prefix)

**Given** framer 노드 (`framing=length_prefix`, `max_message_size=100`)
**And** 페이로드가 길이 필드로 `200` 을 지시
**When** Process 를 호출하면
**Then**:
- `error` 포트로 메시지가 전달되어야 한다
- `frame.error.code` 는 `frame.parse.max_size_exceeded` 이어야 한다

### AC7.3: checksum_mismatch 에러 코드 (frame 모드)

**Given** framer 노드 (`framing=frame`, `checksum="sum8"`)
**And** 페이로드가 정상 STX/length/ETX 지만 checksum 이 잘못됨
**When** Process 를 호출하면
**Then**:
- `error` 포트로 메시지가 전달되어야 한다
- `frame.error.code` 는 `frame.parse.checksum_mismatch` 이어야 한다

### AC7.4: invalid_payload 에러 코드

**Given** framer 노드
**And** 페이로드에 `raw`, `data` 키 모두 없음
**When** Process 를 호출하면
**Then**:
- `error` 포트로 메시지가 전달되어야 한다
- `frame.error.code` 는 `frame.input.invalid_payload` 이어야 한다

### AC7.5: 에러와 정상 프레임 동시 출력

**Given** framer 노드 (`framing=frame`), 페이로드에 "정상 프레임 1개 + 손상 부분 + 정상 프레임 1개"
**When** Process 를 호출하면
**Then**:
- `out` 포트로 정상 프레임 2개가 전달되어야 한다
- `error` 포트로 에러 1개가 전달되어야 한다
- 순서는 조립 순서를 따라야 한다 (정상 프레임 1 → 에러 → 정상 프레임 2 또는 실제 복구 순서)

### AC7.6: 로그 출력 (DEBUG)

**Given** 프레임이 정상 조립되는 Process 호출
**When** slog DEBUG 레벨로 로그 캡처를 활성화하고 Process 를 호출하면
**Then**:
- DEBUG 로그 엔트리가 `framer_type`, `stream_key`, `frame_size`, `frame_index` 필드와 함께 기록되어야 한다

### AC7.7: 로그 출력 (WARN)

**Given** 파싱 에러가 발생하는 Process 호출
**When** slog WARN 레벨로 로그 캡처를 활성화하고 Process 를 호출하면
**Then**:
- WARN 로그 엔트리가 `frame.error.code`, `frame.stream_key`, `frame.buffer.bytes_at_error` 필드와 함께 기록되어야 한다

---

## 8. 자원 제한 (Non-Functional)

### AC8.1: max_streams=0 은 무제한

**Given** framer 노드 (`max_streams=0`)
**When** 100 개 이상의 서로 다른 stream_key 로 메시지를 주입하면
**Then**:
- 모든 스트림이 생성되어야 한다
- max_streams_exceeded 에러는 발생하지 않아야 한다
- (주의: 실제 시스템 자원 한도까지)

### AC8.2: 단일 프레임 크기 상한

**Given** framer 노드 (`framing=frame`, `max_message_size=100`)
**When** 200 바이트의 프레임을 지시하는 length 필드가 포함된 바이트 주입
**Then**:
- `error` 포트로 `frame.parse.max_size_exceeded` 가 전달되어야 한다
- 노드는 계속 동작 가능한 상태이어야 한다

---

## 9. 동시성 및 Race Condition (Non-Functional)

### AC9.1: Concurrent Process 호출 없음 (가정)

**Given** framer 노드는 단일 고루틴 Process 호출을 가정한다 (BaseNode 관례)
**When** 여러 고루틴에서 동시에 Process 를 호출하면
**Then**:
- 내부 뮤텍스로 호출이 직렬화되어 race condition 없이 동작해야 한다
- `go test -race ./internal/node/...` 에서 data race 가 탐지되지 않아야 한다

### AC9.2: Process 중 Stop 호출

**Given** running 중인 framer 노드
**When** Process 호출 중에 다른 고루틴에서 Stop 을 호출하면
**Then**:
- 진행 중인 Process 는 완료되어야 한다
- Stop 은 진행 중인 Process 완료 후 실행되거나, 또는 Stop 이 우선될 수 있다 (BaseNode 관례에 따름)
- 어느 경우든 race condition 이 발생해서는 안 된다

---

## 10. 하위 호환성 (Regression)

### AC10.1: 기존 framing_test.go 회귀 없음

**Given** Phase 0 리팩토링 완료 후 상태
**When** `go test ./internal/agent/serial/...` 및 `go test ./pkg/framing/...` 을 실행하면
**Then**:
- 모든 테스트가 통과해야 한다
- 특히 `TestFrameFramer_LGCPSamples` 가 통과해야 한다

### AC10.2: 시리얼 에이전트 framing=frame 플로우 불변

**Given** 리팩토링 완료 후 상태
**And** 기존 플로우 JSON (serial-in → 다운스트림) 이 시리얼 에이전트의 `framing=frame` 설정으로 동작 중
**When** 동일 플로우를 실행하면
**Then**:
- 에이전트가 방출하는 메시지의 내용 (raw, data, 메타데이터) 이 리팩토링 이전과 동일해야 한다
- 플로우 JSON 수정이 필요하지 않아야 한다

### AC10.3: 신규 framer 노드는 기존 플로우에 영향 없음

**Given** 기존 저장된 플로우 JSON (framer 노드 없음)
**When** 리팩토링 완료 후 해당 플로우를 로드하고 실행하면
**Then**:
- 플로우 로드가 성공해야 한다
- 모든 노드가 정상적으로 동작해야 한다
- 결과 메시지가 리팩토링 이전과 동일해야 한다

---

## 11. 통합 시나리오

### AC11.1: serial-in → framer → output 파이프라인

**Given** 플로우 정의:
- 노드 1: `serial-in` (에이전트 `framing=raw`)
- 노드 2: `framer` (`framing=frame`, LGCP STX/ETX)
- 노드 3: `output` (예: debug node)
- 엣지: 1 → 2 → 3
**And** 에이전트가 LGCP 샘플 바이트 스트림을 생성 (mock)
**When** 플로우를 실행하면
**Then**:
- output 노드가 LGCP 프레임 단위의 메시지를 수신해야 한다
- 메시지 메타에 `frame.index`, `frame.framer_type=frame` 이 포함되어야 한다

### AC11.2: tcp-in (client) → framer → output 파이프라인

**Given** 플로우 정의:
- 노드 1: `tcp-in` (client mode, `framing=raw` 에이전트)
- 노드 2: `framer` (`framing=length_prefix`)
- 노드 3: `output`
**And** mock TCP 서버가 length_prefix 바이트 스트림을 송신
**When** 플로우를 실행하면
**Then**:
- output 노드가 length_prefix 프레임 단위로 메시지를 수신해야 한다

### AC11.3: 에러 포트 분기

**Given** 플로우 정의:
- 노드 1: `serial-in`
- 노드 2: `framer` (`framing=frame`)
- 노드 3: `output-ok` (정상 프레임 수신)
- 노드 4: `output-err` (에러 수신)
- 엣지: 1 → 2, 2.out → 3, 2.error → 4
**And** 에이전트가 "정상 + 손상 + 정상" 바이트 스트림을 생성
**When** 플로우를 실행하면
**Then**:
- `output-ok` 노드는 정상 프레임 2개를 수신해야 한다
- `output-err` 노드는 에러 메시지 1개를 수신해야 한다

---

## 12. 수동 Smoke Test (Personal Mode operator)

### AC12.1: Web UI / API 경유 플로우 실행

**Given** xflow 가 실행 중이고 `framer` 노드가 Registry 에 등록된 상태
**When** 플로우 정의에 `framer` 노드를 포함하여 API 로 업로드하고 실행하면
**Then**:
- 플로우가 정상적으로 시작되어야 한다
- framer 노드의 옵션 파싱 및 Process 동작이 정상이어야 한다
- 로그에 framer 노드의 DEBUG 엔트리가 관측되어야 한다

> 주의: Web UI 팔레트에 framer 노드 아이콘 추가는 본 SPEC 의 범위 외이므로 AC 에서 제외된다. UI 변경 없이 API/JSON 레벨에서 framer 를 사용할 수 있는 것만 검증한다.

---

## 13. 완료 검증 체크리스트

- [ ] AC1.x ~ AC12.x 의 모든 시나리오가 자동화된 Go 테스트로 구현되었다
- [ ] 모든 테스트가 `go test -race ./...` 에서 통과한다
- [ ] `TestFrameFramer_LGCPSamples` 가 통과한다 (pkg/framing 으로 이동 후에도)
- [x] `rawFramer` 버퍼 aliasing 방지 검증 (AC3.6, R3.8, v1.1.0)
- [ ] `SerialConnReader` 관련 기존 테스트가 통과한다
- [ ] Parity 테스트 (AC5.1 ~ AC5.5) 가 통과한다
- [ ] `pkg/framing` 커버리지가 기존 `internal/agent/serial/framing.go` 수준 이상이다
- [ ] `internal/node/framer.go` 신규 코드 커버리지가 85% 이상이다
- [ ] `go vet ./...`, `gofmt -l .`, `golangci-lint run` 모두 통과한다
