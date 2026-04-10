---
id: SPEC-NODE-002
version: "1.0.0"
status: draft
created: "2026-04-10"
updated: "2026-04-10"
author: xtra
priority: high
tags: [node, framing, stream, processing]
related_spec: SPEC-NODE-001, SPEC-SERIAL-001, SPEC-SOCKET-001, SPEC-AGENT-006, SPEC-FLOW-001
---

# SPEC-NODE-002: Implementation Plan - Framer 노드 및 pkg/framing 승격

## 1. 구현 개요

본 계획은 SPEC-NODE-002 의 요구사항 (M1~M7) 을 7개의 구현 단계 (Phase 0 ~ Phase 6) 로 분할한다. Phase 0 은 기존 프레이밍 로직의 `pkg/framing` 승격 (DDD) 이며, Phase 1 이후는 신규 framer 노드 구현 (TDD) 이다. Hybrid 개발 모드 (신규 코드는 TDD, 기존 코드 리팩토링은 DDD) 를 적용한다.

### 1.1 개발 방법론

- **Methodology**: Hybrid (TDD for new code + DDD for existing code refactoring), `.moai/config/sections/quality.yaml` 과 일치
- Phase 0 (pkg/framing 승격): DDD - characterization test 로 기존 동작을 먼저 보존한 후 이동
- Phase 1 ~ 5 (신규 framer 노드, 다중 스트림, registry, parity, 에러): TDD - RED/GREEN/REFACTOR
- Phase 6 (문서화 및 예제 플로우): 문서화 작업
- **Test Coverage Target**: 신규 코드 ≥ 85%, 리팩토링 코드는 기존 커버리지 유지 이상
- **참조 패턴**: SPEC-AGENT-005 의 "옵셔널 동작은 인터페이스로" 원칙, SPEC-AGENT-006 의 "에이전트 자체에 분류 책임 위임" 원칙을 framer 노드 설계에도 일부 적용한다 (에러 분류는 framer 엔진이 판단, 라우팅은 노드가 담당).

### 1.2 우선순위

| 우선순위 | 단계 | 설명 |
|---------|------|------|
| Primary Goal | Phase 0, 1 | pkg/framing 승격 + framer 노드 기본 Process 구현 (단일 스트림) |
| Secondary Goal | Phase 2, 3 | 다중 스트림 버퍼 관리 + Registry 등록 |
| Tertiary Goal | Phase 4, 5 | Parity 테스트 + 에러 포트 통합 및 관측성 |
| Final Goal | Phase 6 | 문서화 및 예제 플로우 |

> **시간 추정 금지**: 본 계획은 우선순위 기반이며, 각 Phase 의 절대 소요 시간은 명시하지 않는다. Phase 간 의존성은 명시한다.

---

## 2. 사전 설계 결정 (Pre-Implementation Decisions)

Plan 승인 단계에서 확정해야 할 설계 결정을 아래에 정리한다. 각 항목은 후속 Phase 의 코드 구조를 결정하므로 구현 시작 전 합의가 필요하다.

### 2.1 결정 (a): pkg/framing 타입 이름

**선택지 a1 - 간결한 재명명**:

```
pkg/framing.Framer       // 기존 SerialFramer
pkg/framing.Options      // 기존 FramerOptions
pkg/framing.New          // 기존 NewSerialFramer
```

**선택지 a2 - Serial 접두사 유지**:

```
pkg/framing.SerialFramer  // 기존 이름 그대로
pkg/framing.FramerOptions
pkg/framing.NewSerialFramer
```

**결정**: **a1 선택 (권장)**. 패키지 경로 `pkg/framing` 자체가 의미를 드러내므로 타입 이름에 `Serial` 을 남기는 것은 오해를 유발한다. framer 엔진은 시리얼과 TCP, UDP, 파일 모두에 동일하게 적용되므로 이름에서도 범용성을 반영한다. 모드 상수는 `ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame` 으로 표준화한다 (기존 문자열 값 `"raw"`, `"newline"` 등은 유지하여 하위 호환).

### 2.2 결정 (b): serial 패키지의 type alias 유지 여부

**선택지 b1 - 완전 삭제**:

- `internal/agent/serial/framing.go` 를 삭제 (또는 thin shim 만 유지)
- 시리얼 에이전트 내부 코드는 모두 `pkg/framing` 으로 import 전환

**선택지 b2 - type alias 유지 (backward compat)**:

```go
// internal/agent/serial/framing.go
package serial

import "pkg/framing"

type SerialFramer = framing.Framer      // type alias
type FramerOptions = framing.Options
var NewSerialFramer = framing.New       // func alias
```

**결정**: **b1 선택 (권장)**. xflow 는 단일 레포이므로 외부 사용자가 `internal/agent/serial.SerialFramer` 를 import 하는 케이스는 없다. Type alias 유지는 기술 부채이므로 이번 기회에 정리한다. 단, `internal/agent/serial/common.go` 의 프레이밍 모드 문자열 상수 (`FramingRaw`, `FramingNewline`, ...) 는 시리얼 에이전트 내부에서 config parsing 시 사용되므로 기존 이름을 유지하되 값은 `pkg/framing.ModeRaw` 등과 동일한 문자열로 유지한다. `SerialConnReader` 는 `internal/agent/serial` 에 남아 `pkg/framing.Framer` 를 내부에서 사용한다.

### 2.3 결정 (c): max_streams 초과 시 정책

**선택지 c1 - Reject (거부)**:

- 새 stream_key 가 도착하면 해당 메시지를 `error` 포트로 전달
- 기존 스트림 버퍼는 유지
- 장점: 기존 스트림의 데이터 손실 없음, 예측 가능성 높음
- 단점: 새 연결이 영원히 처리되지 않을 수 있음

**선택지 c2 - LRU Evict**:

- 가장 오래된 스트림 버퍼를 삭제하고 새 스트림을 받아들임
- 장점: 항상 최신 연결을 처리
- 단점: 축출된 스트림의 미완성 프레임 손실, LRU 관리 오버헤드

**결정**: **c1 선택 (Reject, 권장)**. 기본 정책은 reject 이다. 사유: (1) 스트림 버퍼 손실은 데이터 무결성 관점에서 위험. (2) LRU 구현 복잡도가 본 SPEC 의 범위를 벗어남. (3) `max_streams` 는 DoS 방어 목적이 주이므로 거부가 더 적절. 단, R4.3 의 요구사항은 "정책에 따라" 로 기술되어 있으므로 향후 옵션으로 LRU 를 추가할 여지를 남긴다 (`stream_overflow_policy: reject|lru` 옵션을 미래에 도입 가능). 본 SPEC 에서는 `reject` 만 구현한다.

### 2.4 결정 (d): stream idle timeout 구현 방식

**선택지 d1 - 백그라운드 고루틴 + ticker**:

- 노드 Start 시 GC 고루틴을 띄우고 `stream_idle_timeout / 2` 주기로 순회
- 장점: 타임아웃이 정확함
- 단점: Stop 시 고루틴 종료 관리 필요, 고루틴 추가

**선택지 d2 - Lazy on Process**:

- 매 Process 호출의 초입에서 현재 시각을 기준으로 유휴 스트림을 삭제
- 장점: 고루틴 추가 없음, 구현 단순
- 단점: Process 호출이 드물면 타임아웃이 정확하지 않을 수 있음

**결정**: **d2 선택 (Lazy, 권장)**. Process 호출이 드문 상황에서는 스트림이 실제로 사용되지 않으므로 늦게 삭제되어도 기능상 문제 없다. 고루틴 관리 복잡도를 피하고 단순성을 우선한다. 추가 최적화가 필요하면 후속 SPEC 에서 d1 로 전환한다. 기본값 `stream_idle_timeout=0` 은 비활성이므로 대부분의 사용자는 이 로직을 신경 쓸 필요 없다.

### 2.5 결정 (e): 프레임 파싱 에러 시 버퍼 복구 정책 (frame 모드 외)

`frameFramer` (STX/ETX 기반) 는 ETX 불일치 시 다음 STX 까지 skip 하는 복구 로직을 가진다. 나머지 모드 (`length_prefix`, `fixed_size`, `newline`, `stream`, `raw`) 는 별도의 복구 로직이 없다. 파싱 에러 발생 시 버퍼를 어떻게 처리할지 결정이 필요하다.

**선택지 e1 - Preserve (버퍼 유지)**:

- 에러 발생 시 버퍼를 그대로 두고 다음 Process 호출을 기다림
- 장점: 부족한 바이트로 인한 일시적 오류를 복구 가능
- 단점: 손상된 데이터가 버퍼에 남아 계속 에러를 유발할 수 있음

**선택지 e2 - Clear (버퍼 초기화)**:

- 에러 발생 시 버퍼를 완전히 비우고 다음 바이트부터 새로 시작
- 장점: 손상된 데이터 제거
- 단점: 손상되지 않은 후행 데이터까지 손실

**선택지 e3 - Partial Skip (모드별 휴리스틱)**:

- `length_prefix` 모드: 길이 필드가 `max_message_size` 를 초과하면 버퍼를 1바이트 앞으로 밀고 재시도
- `newline` / `stream` 모드: 에러 시 preserve (이들 모드는 본질적으로 delimiter 기반이므로 손상 복구가 어려움)
- `fixed_size` 모드: `max_message_size` 초과 시 clear
- `raw` 모드: 에러 없음
- `frame` 모드: 기존 STX skip 로직 유지

**결정**: **e3 선택 (Partial Skip, 권장)**. 모드별 특성이 다르므로 단일 정책은 부적절하다. 기존 frame 모드의 복구 로직은 그대로 유지 (R3.7 에 의해 변경 금지). 나머지 모드의 복구 정책은 다음과 같이 구체화:

- `length_prefix`: `max_message_size` 초과 시 clear (이미 상한 위반이므로 복구 불가능)
- `newline`: preserve (delimiter 기반, 다음 바이트가 정상 delimiter 이면 복구됨)
- `fixed_size`: preserve (바이트가 모이기를 기다림, 초과 개념 없음)
- `stream`: preserve
- `raw`: 에러 없음
- `frame`: 기존 STX skip 로직 (변경 금지)

이 정책은 Phase 1 구현 시 상세 문서화하며 테스트로 검증한다.

### 2.6 결정 (f): TCP 서버의 connection_id 메타데이터 의존성 처리

framer 노드의 다중 스트림 분리는 소스 노드가 메타데이터에 `connection_id` 를 주입하는 것에 의존한다. 현재 `internal/node/tcp_io.go` 의 `TCPInNode` 서버 모드가 이를 주입하는지는 구현 시점에 조사가 필요하다.

**선택지 f1 - 본 SPEC 에 포함**:

- Phase 3 의 Registry 등록 이후, Phase 3.5 로 TCP 서버의 `connection_id` 주입을 추가
- 장점: framer 노드가 완전히 동작 가능한 상태로 릴리스됨
- 단점: SPEC 범위가 넓어짐, 네트워크 노드 수정은 독립적인 테스트 부담 유발

**선택지 f2 - 별도 SPEC 으로 분리**:

- 본 SPEC 은 "`connection_id` 가 주입된 경우를 전제" 로 설계
- `connection_id` 가 없으면 framer 노드는 단일 공용 버퍼로 동작 (여전히 올바르지만 연결별 분리 불가)
- TCP 서버의 주입은 별도 SPEC (예: SPEC-NODE-003 또는 SPEC-SOCKET-002) 에서 처리
- 장점: 본 SPEC 의 범위가 명확, 독립적 merge 가능
- 단점: framer 노드가 TCP 서버 다중 연결 시나리오에서 완전히 동작하지 않는 과도기 존재

**결정**: **f2 선택 (별도 SPEC 으로 분리, 권장)**. 사유: (1) `pkg/framing` 승격과 framer 노드 도입 자체가 큰 변경이므로 본 SPEC 은 이에 집중한다. (2) 시리얼 및 TCP 클라이언트 (단일 연결) 시나리오에서는 framer 노드가 완전히 동작하므로 조기 가치를 제공한다. (3) TCP 서버의 `connection_id` 주입은 네트워크 노드의 변경이므로 별도 검토와 테스트가 필요하다. 본 SPEC 의 acceptance.md 에서 "TCP 서버 연결별 분리 시나리오" 는 포함하되, 해당 시나리오의 구현 가능성은 별도 SPEC 에 의존함을 명시한다.

Phase 6 의 문서화 단계에서 후속 SPEC 필요성을 명시하고, `.moai/specs/` 에 SPEC-NODE-003 (가칭: "TCP 소스 노드의 connection_id 메타데이터 주입") 의 자리를 남긴다.

### 2.7 결정 (g): Hybrid 개발 모드 적용

- **DDD (Phase 0)**: pkg/framing 승격은 기존 코드의 이동이므로 DDD 방법론 적용. characterization test 로 기존 동작 보존을 먼저 검증한 후 이동.
- **TDD (Phase 1~5)**: framer 노드는 신규 코드이므로 TDD 방법론 적용. 각 Phase 에서 RED (실패하는 테스트 작성) → GREEN (최소 구현) → REFACTOR (정리) 사이클 수행.
- 테스트 커버리지 목표: 신규 코드 ≥ 85%, pkg/framing 커버리지는 기존 `internal/agent/serial/framing.go` 수준 이상 유지.

---

## 3. Phase 0: pkg/framing 승격 (DDD)

### 3.1 목표

`internal/agent/serial/framing.go` 의 framer 로직 (여섯 개의 framer 구현체) 을 `pkg/framing` 공개 패키지로 이동한다. 시리얼 에이전트는 새 패키지를 import 하여 동일한 엔진을 사용한다. 기존 관측 가능한 동작은 완전히 보존되어야 한다.

### 3.2 변경 파일

#### 신규

- `pkg/framing/framing.go`: `Framer` 인터페이스, `Options` 구조체, 모드 상수, `New` 팩토리
- `pkg/framing/raw.go`: `rawFramer`
- `pkg/framing/newline.go`: `newlineFramer`
- `pkg/framing/length_prefix.go`: `lengthPrefixFramer`
- `pkg/framing/fixed_size.go`: `fixedSizeFramer`
- `pkg/framing/stream.go`: `streamFramer`
- `pkg/framing/frame.go`: `frameFramer`
- `pkg/framing/framing_test.go`: 기존 `internal/agent/serial/framing_test.go` 의 framer 단위 테스트를 이동 (특히 `TestFrameFramer_LGCPSamples`)

> 파일 분할은 각 framer 별 1 파일 원칙을 권장하되, 각 파일이 50 라인 이하로 지나치게 작다면 `framing.go` 에 합칠 수 있다. Phase 0 의 REFACTOR 단계에서 결정한다.

#### 수정

- `internal/agent/serial/framing.go`: framer 구현 삭제, `SerialConnReader` 만 남김. `SerialConnReader` 는 `pkg/framing.Framer` 를 내부적으로 사용하도록 변경
- `internal/agent/serial/common.go`: 모드 상수 (`FramingRaw` 등) 는 유지하되 값은 `pkg/framing.ModeRaw` 와 동일 문자열 유지
- `internal/agent/serial/config.go` (`ParseSerialConfig`): framer 생성 시 `pkg/framing.New` 호출로 변경
- `internal/agent/serial/framing_test.go`: framer 단위 테스트는 `pkg/framing/framing_test.go` 로 이동하고, `SerialConnReader` 관련 테스트만 유지

### 3.3 DDD 단계

1. **ANALYZE**:
   - `internal/agent/serial/framing.go` (471라인) 을 전체 숙독. 각 framer 의 입출력 계약 파악.
   - `internal/agent/serial/framing_test.go` (1256라인) 의 테스트를 분류: (a) framer 단위 테스트 (이동 대상), (b) `SerialConnReader` 테스트 (유지 대상), (c) 혼합 (신중히 분할)
   - `SerialConnReader` 가 `SerialFramer` 를 어떻게 사용하는지 정확히 이해 (의존 지점 식별).
   - 시리얼 에이전트의 `ParseSerialConfig` 에서 framer 가 어디서 어떻게 생성되는지 추적.

2. **PRESERVE**:
   - Characterization test 작성: 현재 상태에서 framer 들이 생성하는 프레임을 스냅샷으로 저장 (입력 바이트 배열 → 기대 출력 프레임 배열).
   - LGCP 샘플, newline 테스트 케이스, length_prefix 엔디안 케이스 등 결정적 입력을 사용.
   - 특히 `TestFrameFramer_LGCPSamples` 의 현재 동작을 characterization 기준으로 고정.

3. **IMPROVE**:
   - `pkg/framing/` 디렉토리 생성
   - framer 구현체를 한 개씩 이동하면서 매번 `go test ./...` 실행하여 회귀 없음 확인
   - 이동 순서 권장: `raw` → `newline` → `fixed_size` → `length_prefix` → `stream` → `frame` (복잡도 오름차순)
   - 각 이동 후 시리얼 에이전트의 import 경로를 `pkg/framing` 으로 전환
   - 이름 변경: `SerialFramer` → `Framer`, `FramerOptions` → `Options`, `NewSerialFramer` → `New`
   - 모드 상수 추가: `ModeRaw`, `ModeNewline` 등 (기존 문자열 값 유지)

4. **VERIFY**:
   - `go test ./...` 전체 통과
   - `TestFrameFramer_LGCPSamples` 통과 (위치는 이제 `pkg/framing/framing_test.go`)
   - `SerialConnReader` 관련 테스트 통과 (위치는 여전히 `internal/agent/serial/framing_test.go`)
   - `go test -race ./...` 통과
   - `go vet ./...`, `gofmt -l .`, `golangci-lint run` 통과
   - 시리얼 에이전트의 실제 포트를 사용하는 smoke test (존재 시) 통과

### 3.4 의존성

- 외부 의존: 없음
- 후속 의존: Phase 1 이후의 framer 노드 구현은 `pkg/framing` 에 의존

### 3.5 완료 조건

- [ ] `pkg/framing` 패키지 생성 및 여섯 개 framer 이동 완료
- [ ] 시리얼 에이전트가 `pkg/framing` 을 import 하여 사용
- [ ] `SerialConnReader` 는 `internal/agent/serial` 에 유지되고 `pkg/framing.Framer` 를 내부 사용
- [ ] `TestFrameFramer_LGCPSamples` 를 포함한 모든 기존 framer 테스트가 `pkg/framing` 에서 통과
- [ ] `SerialConnReader` 관련 테스트가 `internal/agent/serial` 에서 통과
- [ ] `go test -race ./...` 전체 통과
- [ ] 시리얼 에이전트의 `framing=frame` 경로 동작 변경 없음 (관측 가능한 동작 동일)

---

## 4. Phase 1: Framer 노드 기본 Process 구현 (단일 스트림, TDD)

### 4.1 목표

`internal/node/framer.go` 에 framer 노드를 신규 구현한다. 본 Phase 에서는 단일 공용 버퍼만 지원하며 (다중 스트림은 Phase 2), 기본 Process 동작 (바이트 읽기 → framer.Read 반복 → 프레임 출력) 을 완성한다.

### 4.2 변경 파일

#### 신규

- `internal/node/framer.go`: `FramerNode` 타입, `Process` 메서드, 단일 스트림 버퍼 로직
- `internal/node/framer_test.go`: 신규 테스트 파일

### 4.3 TDD 단계

1. **RED (실패하는 테스트 작성)**:
   - `TestFramerNode_Process_SingleFrame_NewlineMode`: `\n` delimiter 로 단일 프레임 출력
   - `TestFramerNode_Process_MultipleFrames_NewlineMode`: 한 번의 Process 호출에서 여러 프레임 출력
   - `TestFramerNode_Process_PartialFrame_BuffersRemainder`: 미완성 꼬리가 버퍼에 남음
   - `TestFramerNode_Process_PartialThenComplete`: 두 번째 Process 호출에서 남은 바이트가 완성됨
   - `TestFramerNode_Process_FrameMode_LGCPSample`: LGCP 샘플 3개 → 3개 프레임
   - `TestFramerNode_Process_LengthPrefix_BigEndian`: length prefix 모드
   - `TestFramerNode_Process_InvalidPayload_RoutesToError`: raw/data 모두 없음 → error 포트
   - `TestFramerNode_Process_EmptyBytes_ReturnsEmpty`: 0바이트 입력 → 빈 출력
   - 모든 테스트는 현재 실패해야 함 (RED).

2. **GREEN (최소 구현)**:
   - `FramerNode` 구조체 정의: `framer framing.Framer`, `buf []byte`, `index int`, `mu sync.Mutex`, `opts framing.Options`, `framingMode string`
   - `Process` 메서드 구현:
     - 페이로드에서 `raw` 또는 `data` 키 읽기
     - 둘 다 없으면 error 메시지 생성 후 returns
     - 버퍼에 바이트 추가
     - `bytes.Reader` 를 생성하여 `framer.Read` 를 반복 호출 (EOF 또는 부족 에러까지)
     - 각 완성 프레임에 대해 새 메시지 생성 (clone, set raw/data, set frame.index/framer_type 메타)
     - 버퍼에서 소비된 바이트 제거
   - 모든 테스트가 통과할 때까지 반복

3. **REFACTOR**:
   - Process 메서드의 내부 헬퍼 분리: `extractInputBytes(msg) ([]byte, error)`, `assembleFrames(buf []byte) ([][]byte, []byte, error)`
   - 에러 메시지 생성 헬퍼 추출
   - godoc 주석 (한국어) 추가

### 4.4 설계 세부

**Process 의사 코드**:

```
func (n *FramerNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
    // 1) 페이로드에서 바이트 추출
    data, err := extractInputBytes(msg)
    if err != nil {
        return []message.Message{makeErrorMessage(msg, "frame.input.invalid_payload", err)}, nil
    }

    // 2) 버퍼에 추가 (단일 공용 버퍼, Phase 1)
    n.mu.Lock()
    defer n.mu.Unlock()
    n.buf = append(n.buf, data...)

    // 3) framer.Read 반복 호출
    var frames [][]byte
    for {
        reader := bytes.NewReader(n.buf)
        frame, err := n.framer.Read(reader)
        if err == io.EOF || err == io.ErrUnexpectedEOF {
            break  // 더 많은 바이트 필요
        }
        if err != nil {
            // 파싱 에러 → error 포트로 라우팅 (버퍼 복구 정책 적용)
            n.buf = recoverBuffer(n.buf, n.framingMode, err)
            return framesToMessages(msg, frames, n.framingMode, &n.index) +
                   errorToMessage(msg, err, n.framingMode, n.buf), nil
        }
        frames = append(frames, frame)
        // 소비된 바이트 제거 (reader 의 위치 - buffer 끝이 남은 바이트)
        consumed := len(n.buf) - reader.Len()
        n.buf = n.buf[consumed:]
    }

    // 4) 프레임들을 메시지로 변환하여 반환
    return framesToMessages(msg, frames, n.framingMode, &n.index), nil
}
```

**주의**: `framer.Read(r io.Reader)` 가 `bytes.Reader` 로 얼마나 소비했는지 추적하는 방식은 구현 시 정확히 검증한다. `rawFramer`, `newlineFramer` 는 `bufio.Scanner` 를 내부에서 생성할 수 있으므로 한 번의 Read 호출이 여러 프레임을 소비할 수 있다. 이 경우 `Scanner` 가 아니라 저수준 Read 를 구현해야 할 수 있다. Phase 0 의 ANALYZE 단계에서 각 framer 의 `Read` 시맨틱스를 정확히 파악하고 필요 시 `pkg/framing` 에 `ReadAll` 또는 상태 보존 API 를 추가한다.

### 4.5 의존성

- 선행: Phase 0 (pkg/framing)
- 후속: Phase 2 가 본 Phase 의 기본 구조를 확장

### 4.6 완료 조건

- [ ] `FramerNode` 단일 스트림 Process 동작 완성
- [ ] 여섯 모드 (raw, newline, length_prefix, fixed_size, stream, frame) 모두 동작
- [ ] 미완성 꼬리 보존 동작
- [ ] 에러 시 error 포트 라우팅
- [ ] `go test -race ./internal/node/...` 통과
- [ ] 신규 코드 커버리지 ≥ 85%

---

## 5. Phase 2: 다중 스트림 버퍼 관리 (TDD)

### 5.1 목표

Phase 1 의 단일 버퍼 구조를 `map[string]*StreamBuffer` 로 확장하여 스트림 키별 독립 버퍼를 제공한다. `max_streams`, `stream_idle_timeout` (lazy), `stream_key_metadata` 옵션을 구현한다.

### 5.2 변경 파일

- `internal/node/framer.go`: `StreamBuffer` 타입 정의, `StreamMap` 관리 로직 추가
- `internal/node/framer_test.go`: 다중 스트림 테스트 추가

### 5.3 TDD 단계

1. **RED**:
   - `TestFramerNode_MultiStream_IndependentBuffers`: connection_id=A, connection_id=B 메시지 교차 → 각자 독립 프레임 조립
   - `TestFramerNode_MultiStream_FrameIndex_Independent`: 각 스트림의 frame.index 가 독립적
   - `TestFramerNode_MultiStream_NoKeyMetadata_SingleBuffer`: 메타에 키 없음 → 단일 공용 버퍼
   - `TestFramerNode_MaxStreams_Reject`: max_streams=2, 3번째 stream_key → error 포트 + `frame.buffer.max_streams_exceeded`
   - `TestFramerNode_StreamIdleTimeout_LazyRemoval`: idle timeout 경과 후 다음 Process 호출에서 제거
   - `TestFramerNode_Stop_FlushesIncompleteBuffers`: Stop 호출 시 미완성 버퍼 → error 포트로 `incomplete_on_stop`

2. **GREEN**:
   - `StreamBuffer` 구조체: `framer framing.Framer`, `buf []byte`, `lastActivity time.Time`, `index int`
   - `StreamMap` 구조체: `map[string]*StreamBuffer` + `sync.Mutex` (또는 `sync.RWMutex`)
   - Process 시 스트림 키 결정 → `getOrCreateBuffer(key)` → 기존 Process 로직 적용
   - `max_streams` 체크 로직
   - `stream_idle_timeout` lazy 삭제 (결정 d2): 매 Process 초입에서 현재 시각 기반 순회 삭제
   - `Stop` 메서드: 모든 버퍼 순회 → 미완성 바이트 감지 → error 메시지 발행 → 버퍼 삭제

3. **REFACTOR**:
   - `StreamBuffer` 관리 메서드 추출: `getOrCreate`, `evictIdle`, `flushAll`
   - 스트림 키 추출 헬퍼: `resolveStreamKey(msg, opts)`

### 5.4 설계 세부

**StreamMap 동시성 전략**:

- 기본 전략: `sync.Mutex` 로 맵 전체 보호. 개별 버퍼 동작은 framer 노드 전체의 Process 뮤텍스 (`n.mu`) 에 의해 직렬화되므로 추가 락 불필요.
- 대안: `sync.Map` 은 타입 안전성이 떨어지고 이 사용 패턴 (write-heavy) 에는 부적합.
- `StreamBuffer` 자체는 nested 뮤텍스를 가지지 않는다. framer 노드의 단일 뮤텍스로 모든 접근을 직렬화한다. NFR5 의 "스트림 독립성" 은 논리적 독립성을 의미하며, 물리적 병렬성은 요구되지 않는다.

**idle 삭제 lazy 구현**:

```
func (n *FramerNode) evictIdleStreams(now time.Time) {
    if n.opts.StreamIdleTimeout <= 0 {
        return
    }
    for key, sb := range n.streams {
        if now.Sub(sb.lastActivity) > n.opts.StreamIdleTimeout {
            delete(n.streams, key)
        }
    }
}
```

매 Process 호출의 초입에서 `evictIdleStreams(time.Now())` 호출.

### 5.5 의존성

- 선행: Phase 1
- 후속: Phase 3 (Registry 등록), Phase 4 (Parity), Phase 5 (에러)

### 5.6 완료 조건

- [ ] `StreamBuffer`, `StreamMap` 구현
- [ ] `stream_key_metadata` 에 따른 키 분리 동작
- [ ] `max_streams` reject 정책 동작
- [ ] `stream_idle_timeout` lazy 삭제 동작
- [ ] `Stop` 시 incomplete buffer flush
- [ ] `go test -race ./internal/node/...` 통과

---

## 6. Phase 3: Registry 등록 및 기본 포트 정의 (TDD)

### 6.1 목표

`internal/node/registry.go` 의 `registerBuiltins` 에 `framer` 타입을 등록하고 팩토리 함수를 작성한다. NodeDef 의 Options 를 `pkg/framing.Options` 로 변환한다.

### 6.2 변경 파일

- `internal/node/registry.go`: `registerBuiltins` 에 `framer` 등록 추가
- `internal/node/framer.go`: 팩토리 함수 `NewFramerNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 추가
- `internal/node/registry_test.go` (존재 시) 또는 신규: 등록 검증 테스트

### 6.3 TDD 단계

1. **RED**:
   - `TestRegistry_FramerType_Registered`: `Registry.Get("framer")` 가 nil 이 아님
   - `TestFramerFactory_ValidOptions`: 유효한 옵션으로 노드 생성 성공
   - `TestFramerFactory_MissingFramingOption_Error`: framing 옵션 없음 → 에러
   - `TestFramerFactory_InvalidFramingOption_Error`: framing="unknown" → 에러
   - `TestFramerFactory_InvalidLengthEndian_Error`: length_endian="middle" → 에러
   - `TestFramerFactory_NegativeMaxMessageSize_Error`: max_message_size=-1 → 에러
   - `TestFramerNode_DefaultPorts`: inputs=["in"], outputs=["out", "error"]

2. **GREEN**:
   - 옵션 파싱 헬퍼: `parseFramerOptions(def.Options) (framing.Options, framerNodeOptions, error)`
   - 옵션 검증: framing 값, 숫자 범위, length_endian 값 등
   - `pkg/framing.New` 호출로 framer 생성
   - framer 노드 인스턴스 생성 및 기본 포트 설정
   - `registerBuiltins` 에 등록 추가

3. **REFACTOR**:
   - 옵션 파싱 함수의 단위 테스트 분리
   - 에러 메시지 일관성 (모두 한국어 또는 영어)

### 6.4 의존성

- 선행: Phase 1, 2
- 후속: Phase 4, 5

### 6.5 완료 조건

- [ ] `framer` 타입이 Registry 에 등록됨
- [ ] 팩토리가 옵션을 정확히 검증하고 에러를 반환
- [ ] 기본 포트 (`in`, `out`, `error`) 정의
- [ ] 관련 테스트 통과

---

## 7. Phase 4: Serial Agent vs Framer Node Parity 테스트 (TDD + 통합)

### 7.1 목표

시리얼 에이전트의 `framing=frame` 경로와 "에이전트 framing=raw + framer 노드" 경로가 동일한 바이트 입력에 대해 동일한 프레임 시퀀스를 생성함을 검증한다. 이는 M5 요구사항 (R5.1 ~ R5.5) 의 핵심이며 SPEC 이 약속하는 이중 지원의 근거이다.

### 7.2 변경 파일

- `internal/node/framer_parity_test.go` (신규): parity 테스트 스위트

### 7.3 테스트 전략

1. **RED**:
   - `TestParity_LGCPSamples_SerialAgentVsFramerNode`:
     - Given: LGCP 샘플 바이트 스트림 (기존 `TestFrameFramer_LGCPSamples` 와 동일)
     - When:
       - 경로 A: `pkg/framing.New(ModeFrame, opts)` 로 framer 생성 → 직접 Read 호출 → 프레임 시퀀스 A
       - 경로 B: `NewFramerNode(..., framing=frame, opts)` → Process 호출 → 프레임 시퀀스 B
     - Then: 시퀀스 A 와 시퀀스 B 의 길이, 각 프레임의 바이트 배열이 정확히 일치
   - `TestParity_NewlineMode_StressChunked`:
     - 한 번에 모든 바이트를 Process 에 주입 vs 1바이트씩 나눠서 주입 → 결과 프레임 시퀀스 동일
   - `TestParity_LengthPrefix_BigEndian_Chunked`:
     - 같은 stress pattern
   - `TestParity_FrameMode_ETXMismatch_Recovery`:
     - 파싱 에러가 있는 입력에서 경로 A 와 경로 B 의 복구 후 프레임이 동일

2. **GREEN**:
   - Phase 0 에서 `pkg/framing` 이 올바르게 이동되었다면 대부분 즉시 통과
   - 차이가 발견되면 framer 노드의 Process 로직 (Phase 1) 을 수정하여 정합성 확보
   - 특히 `bytes.Reader` 를 재생성할 때 framer 의 내부 상태가 초기화되는 경우 주의

3. **REFACTOR**:
   - 테스트 헬퍼 `runFramerDirectly(mode, opts, data)` 와 `runFramerNode(mode, opts, data)` 분리
   - table-driven test 로 LGCP 샘플 여러 케이스 묶음

### 7.4 의존성

- 선행: Phase 0, 1, 2, 3
- 후속: Phase 5

### 7.5 완료 조건

- [ ] Parity 테스트가 LGCP 샘플, newline, length_prefix 모드에 대해 통과
- [ ] 차이가 발견된 경우 framer 노드 수정으로 해결
- [ ] 기존 `TestFrameFramer_LGCPSamples` 도 여전히 통과

---

## 8. Phase 5: 에러 포트 통합 및 관측성 (TDD)

### 8.1 목표

R7.1 의 에러 코드 체계를 구현하고, 각 에러 경로가 정확한 코드와 메타데이터를 가지고 `error` 포트로 전달되도록 한다. 로깅 (DEBUG/INFO/WARN) 을 추가한다.

### 8.2 변경 파일

- `internal/node/framer.go`: 에러 메시지 생성 헬퍼 통합, 로깅 추가
- `internal/node/framer_test.go`: 에러 시나리오 테스트 추가

### 8.3 TDD 단계

1. **RED**:
   - `TestFramerNode_Error_ETXMismatch_FrameMode`: frame mode 에서 ETX 불일치 → error 포트, 코드 `frame.parse.etx_mismatch`, 메타에 `frame.framer_type=frame`, `frame.stream_key`, `frame.buffer.bytes_at_error` 포함
   - `TestFramerNode_Error_MaxSizeExceeded_LengthPrefix`: length_prefix 에서 max_message_size 초과 → error 포트, 코드 `frame.parse.max_size_exceeded`
   - `TestFramerNode_Error_InvalidPayload_NoRawNoData`: payload 에 raw/data 없음 → error 포트, 코드 `frame.input.invalid_payload`
   - `TestFramerNode_Error_MaxStreamsExceeded`: max_streams 초과 → error 포트, 코드 `frame.buffer.max_streams_exceeded`
   - `TestFramerNode_Error_ErrorDoesNotPollutOut`: error 발생 시 out 포트로 프레임이 나가지 않음 (단, 에러 이전에 조립된 프레임은 out 으로 나감)

2. **GREEN**:
   - 에러 코드 상수 정의:
     ```go
     const (
         ErrCodeInvalidPayload       = "frame.input.invalid_payload"
         ErrCodeETXMismatch          = "frame.parse.etx_mismatch"
         ErrCodeChecksumMismatch     = "frame.parse.checksum_mismatch"
         ErrCodeMaxSizeExceeded      = "frame.parse.max_size_exceeded"
         ErrCodeLengthInvalid        = "frame.parse.length_invalid"
         ErrCodeMaxStreamsExceeded   = "frame.buffer.max_streams_exceeded"
         ErrCodeIncompleteOnStop     = "frame.buffer.incomplete_on_stop"
     )
     ```
   - `pkg/framing` 의 에러 타입을 판별하여 코드 매핑. 필요 시 `pkg/framing` 에 `errors.Is` 호환 sentinel 에러 추가.
   - 에러 메시지 빌더: `makeErrorMessage(srcMsg, code string, cause error, bufBytesAtError int, framerType, streamKey string) message.Message`
   - 로깅: `slog.Debug` (프레임 조립), `slog.Warn` (파싱 에러), `slog.Debug` (Process 호출 요약)

3. **REFACTOR**:
   - 에러 분류 로직을 `classifyFramerError(err error) string` 함수로 추출
   - 로깅 필드를 `slog.Attr` 로 정리

### 8.4 pkg/framing 에 sentinel 에러 추가 (필요 시)

기존 `internal/agent/serial/framing.go` 는 `fmt.Errorf` 로 동적 에러를 반환한다. 에러 코드 매핑을 위해 `pkg/framing` 에 다음 sentinel 에러를 추가하는 것이 바람직하다:

```go
var (
    ErrETXMismatch       = errors.New("frame: ETX mismatch")
    ErrChecksumMismatch  = errors.New("frame: checksum mismatch")
    ErrMaxSizeExceeded   = errors.New("frame: max message size exceeded")
    ErrLengthInvalid     = errors.New("frame: length field invalid")
)
```

기존 `fmt.Errorf` 호출을 `fmt.Errorf("...: %w", ErrXxx)` 로 변경. 이는 Phase 0 의 REFACTOR 단계에 포함하거나, Phase 5 에서 수행한다 (결정: Phase 5).

> 이 변경은 기존 serial framer 의 동작을 변경하지 않으므로 characterization test 가 계속 통과한다.

### 8.5 의존성

- 선행: Phase 1, 2, 3, 4
- 후속: Phase 6

### 8.6 완료 조건

- [ ] 모든 에러 코드가 구현되고 테스트됨
- [ ] error 포트가 out 포트와 정확히 분리됨
- [ ] 로깅이 적절한 레벨과 필드로 출력됨
- [ ] `pkg/framing` sentinel 에러 도입 (필요 시)

---

## 9. Phase 6: 문서화 및 예제 플로우

### 9.1 목표

framer 노드의 사용법, 옵션, 플로우 예제를 문서화한다. 후속 SPEC (TCP 서버 connection_id 주입) 의 필요성을 명시한다.

### 9.2 문서 업데이트

- [ ] `CHANGELOG.md`: SPEC-NODE-002 entry 추가
  - "pkg/framing 공개 패키지 신설 - 시리얼 에이전트의 프레이밍 엔진을 외부 재사용 가능하도록 승격"
  - "framer 노드 추가 - 임의의 바이트 스트림 소스 노드 (serial-in, tcp-in, udp-in) 의 출력을 받아 프로토콜 프레임으로 분리"
- [ ] 노드 사용 문서 (위치는 프로젝트 관례에 따름, 없으면 `docs/nodes/framer.md` 신설):
  - 노드 설명, 입출력 포트, 옵션 표 (framing, buffer_size, delimiter, stx, etx, ..., stream_key_metadata, max_streams)
  - 플로우 JSON 예제:
    - 예제 A: serial-in (framing=raw) → framer (framing=frame, stx/etx 설정) → output
    - 예제 B: tcp-in (client mode) → framer (framing=length_prefix) → transform → output
  - "에이전트 프레이밍" vs "노드 프레이밍" 선택 가이드
  - 에러 코드 참조표
- [ ] `pkg/framing` 패키지의 package godoc 작성 (한국어)
- [ ] `internal/node/framer.go` 의 타입 및 함수 godoc 작성
- [ ] `.moai/specs/` 하위에 SPEC-NODE-003 (또는 SPEC-SOCKET-002) 플레이스홀더 생성 안내:
  - TCP 서버 connection_id 메타데이터 주입 SPEC 의 필요성을 README 또는 roadmap 에 기록

### 9.3 최종 검증

- [ ] `go build ./...` 성공
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] `gofmt -l .` 출력 없음
- [ ] `golangci-lint run` 통과
- [ ] `pkg/framing` 커버리지 ≥ 기존 serial framing 커버리지
- [ ] `internal/node/framer.go` 커버리지 ≥ 85%
- [ ] Parity 테스트 포함 전체 회귀 없음
- [ ] SPEC-NODE-002 PR 에 결함 재현 시나리오 없음 (본 SPEC 은 신규 기능 + 리팩토링이므로)

### 9.4 완료 조건

- [ ] 모든 품질 게이트 통과
- [ ] CHANGELOG 업데이트
- [ ] framer 노드 사용 문서 작성
- [ ] SPEC status: `draft` → `completed`

---

## 10. 위험 요소 및 대응

### 10.1 위험: pkg/framing 이동 시 시리얼 에이전트 회귀

- **위험도**: 높음
- **시나리오**: Phase 0 의 이동 과정에서 framer 구현 미묘한 변경으로 LGCP 회귀 테스트 실패
- **대응**:
  - DDD 방법론 적용 - characterization test 먼저, 이동 후 반복 검증
  - framer 한 개씩 이동 (raw → ... → frame) 하여 문제 격리 용이
  - `TestFrameFramer_LGCPSamples` 를 pivot 테스트로 사용, 각 이동마다 실행
  - 이름 변경과 로직 변경을 섞지 않음 (이동 1차: 이름 유지, 2차: 이름 변경)

### 10.2 위험: Framer Read 의 소비 바이트 추적 실패

- **위험도**: 중간
- **시나리오**: 일부 framer 가 `bufio.Scanner` 를 내부에서 생성하면 소비된 바이트 수를 추적하기 어려움. 매 Process 호출마다 새 `bytes.Reader` 를 만들면 scanner 가 재초기화되어 버퍼 상태가 꼬임
- **대응**:
  - Phase 0 ANALYZE 에서 각 framer 의 `Read` 동작 정확히 파악
  - 필요 시 `pkg/framing` 에 "상태 보존 리더" API 추가 (예: `Framer.ReadFrom(buf []byte) (frame []byte, consumed int, err error)`)
  - Phase 1 의 GREEN 단계에서 문제 발견 시 `pkg/framing` API 확장 (backward compat 유지)

### 10.3 위험: 다중 스트림 버퍼 락 경합

- **위험도**: 낮음
- **시나리오**: 많은 스트림을 단일 뮤텍스로 보호하면 성능 저하
- **대응**:
  - 초기 구현은 단일 뮤텍스 (단순성 우선)
  - NFR7 의 처리량 벤치마크가 통과하지 못하면 후속 최적화로 `sync.RWMutex` 또는 per-stream lock 도입
  - 본 SPEC 의 수락 기준은 정확성 우선, 성능은 목표 수치 제공

### 10.4 위험: Parity 테스트 불일치

- **위험도**: 중간
- **시나리오**: 시리얼 에이전트 경로와 framer 노드 경로가 미묘하게 다른 프레임을 생성
- **대응**:
  - Phase 0 에서 두 경로가 같은 엔진을 공유하므로 원칙적으로 동일해야 함
  - 차이가 발견되면 바이트 소비 추적 로직 (결정 b1 관련) 의 문제일 가능성 높음
  - Phase 4 Parity 테스트를 Phase 1 보다 먼저 작성 가능하도록 테스트 케이스를 Phase 0 에서 먼저 정의

### 10.5 위험: TCP 서버 connection_id 의존성

- **위험도**: 낮음
- **시나리오**: framer 노드의 다중 스트림 기능이 TCP 서버 시나리오에서 동작하지 않음
- **대응**:
  - 결정 (f) 에 따라 별도 SPEC 으로 분리
  - 본 SPEC 의 acceptance.md 에 "single-stream TCP client" 와 "multi-stream with connection_id injected" 를 모두 포함
  - 문서에서 "TCP 서버 다중 연결 분리는 SPEC-NODE-003 완료 후 가능" 명시

---

## 11. 구현 순서 요약

```
Phase 0 (pkg/framing 승격 - DDD)
    ↓
Phase 1 (Framer 노드 단일 스트림 - TDD)
    ↓
Phase 2 (다중 스트림 버퍼 - TDD)
    ↓
Phase 3 (Registry 등록 - TDD)
    ↓
Phase 4 (Parity 테스트 - TDD/통합)
    ↓
Phase 5 (에러 포트 + 관측성 - TDD)
    ↓
Phase 6 (문서화 및 예제)
```

- Phase 0 은 이후 모든 Phase 의 전제 조건이며 관측 가능한 동작 보존이 핵심.
- Phase 1 ~ 3 는 기본 기능의 점진적 완성.
- Phase 4 는 이중 지원 (M5) 의 핵심 검증.
- Phase 5 는 에러 경로의 정리와 관측성 마무리.
- Phase 6 은 문서화 및 릴리스 준비.

---

## 12. 기술적 접근

### 12.1 패턴 일관성

본 SPEC 은 SPEC-AGENT-006 의 원칙 중 두 가지를 차용한다:

| SPEC-AGENT-006 | SPEC-NODE-002 |
|----------------|---------------|
| 에이전트가 자신의 connection key set 을 노출 | framer 노드가 자신의 에러 분류를 판단 |
| 옵셔널 인터페이스 (ConnectionConfigChecker) | 옵셔널 출력 포트 (error) |
| 기존 동작 보존 (characterization test) | Phase 0 의 DDD 접근 |

### 12.2 Go 코드 스타일

- 모든 신규 public API 에 godoc 주석 (한국어)
- error wrapping: `fmt.Errorf("framer node process: %w", err)`
- sentinel 에러는 `pkg/framing` 에 `errors.New` 로 정의하고 wrap 시 `%w` 사용
- `context.Context` 를 첫 인자로 (`Process(ctx, msg)`)
- table-driven test 우선
- `t.Parallel()` 적용 (독립 테스트)
- 뮤텍스 명명: 기존 BaseNode 패턴 유지 (`n.mu`)
- snake_case 파일 이름: `framer.go`, `framer_test.go`, `framer_parity_test.go`
- `pkg/framing` 파일 이름: `framing.go`, `raw.go`, `newline.go`, `length_prefix.go`, `fixed_size.go`, `stream.go`, `frame.go`

### 12.3 마이그레이션 전략

- 저장소 스키마 변경 없음
- 기존 플로우에 framer 노드가 없으므로 구 플로우는 그대로 동작
- 시리얼 에이전트 사용자는 `framing=frame` 방식을 계속 사용 가능
- framer 노드는 opt-in 기능

---

## 13. 완료 정의 (Definition of Done)

본 SPEC 의 구현이 완료되었다고 판정하는 기준:

- [ ] 모든 EARS 요구사항 (R1.1 ~ R7.6) 구현
- [ ] 모든 Phase (0 ~ 6) 완료
- [ ] `pkg/framing` 패키지 신설 및 시리얼 에이전트 리팩토링 완료
- [ ] framer 노드가 Registry 에 등록되고 플로우 JSON 에서 사용 가능
- [ ] 단일 스트림 및 다중 스트림 시나리오 모두 동작
- [ ] Parity 테스트 (LGCP 샘플 기반) 통과
- [ ] `TestFrameFramer_LGCPSamples` 회귀 없음
- [ ] `internal/node/framer.go` 테스트 커버리지 ≥ 85%
- [ ] `pkg/framing` 테스트 커버리지 ≥ 기존 `internal/agent/serial/framing.go` 수준
- [ ] `go test -race ./...` 전체 통과
- [ ] `go vet ./...` 통과
- [ ] `gofmt -l .` 출력 없음
- [ ] `golangci-lint run` 통과
- [ ] CHANGELOG.md 업데이트
- [ ] framer 노드 사용 문서 작성
- [ ] 후속 SPEC (TCP 서버 connection_id 주입) 의 필요성이 문서에 명시
- [ ] PR 머지 및 SPEC status: `draft` → `completed`
