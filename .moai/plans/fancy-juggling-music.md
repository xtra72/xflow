# Plan: Modbus 폴링을 에이전트에서 브릿지로 이전

## Context

현재 Modbus 폴링은 `ModbusAgent.pollLoop()`에서 수행된다. 에이전트가 자체 레지스터 맵을 기반으로 주기적으로 읽고 `msgCh`를 통해 브릿지에 전달한다.

**문제**: 에이전트에 폴 루틴을 두면, 다양한 구성의 레지스터 맵 읽기가 어렵다 (포트 분리로 가능하나 복잡). 하나의 에이전트(=하나의 Modbus 포트)에 여러 브릿지가 연결될 때, 각 브릿지가 자신만의 레지스터 맵과 폴링 간격으로 독립적으로 읽어야 한다.

**해결**: 폴링 책임을 브릿지로 이전한다. 브릿지가 자체 타이머로 에이전트에 `read_raw` 명령을 보내고, 응답을 어댑터로 변환하여 플로우에 전달한다.

---

## Implementation Steps

### Step 1: PollableAdapter 인터페이스 추가

**파일**: `internal/node/bridge_adapter.go`

```go
// ReadSpec 은 브릿지가 에이전트에게 요청할 단일 읽기 단위를 정의한다.
type ReadSpec struct {
    FunctionCode uint8  // Modbus FC (3=holding, 4=input)
    StartAddr    uint16 // 시작 레지스터 주소
    Quantity     uint16 // 읽을 레지스터 개수
    UnitID       uint8  // Modbus Unit ID
}

// ReadResult 는 에이전트로부터 받은 원시 읽기 결과이다.
type ReadResult struct {
    Spec ReadSpec
    Data []byte // 원시 바이트 (2 * Quantity 바이트)
}

// PollableAdapter 는 브릿지 주도 폴링을 지원하는 어댑터 인터페이스이다.
type PollableAdapter interface {
    // ReadSpecs 는 이 어댑터의 레지스터 맵을 읽기 단위 목록으로 변환한다.
    ReadSpecs() []ReadSpec

    // AssembleMessage 는 여러 ReadResult를 조합하여 플로우 Message를 생성한다.
    AssembleMessage(results []ReadResult) (message.Message, error)
}
```

### Step 2: ModbusAdapter에 PollableAdapter 구현

**파일**: `internal/node/adapter/modbus.go`

- `ReadSpecs()`: `registers []RegisterDef`를 Area/UnitID 기준으로 그룹핑하여 `[]ReadSpec` 반환
  - 같은 Area(holding/input)의 연속 레지스터는 하나의 ReadSpec으로 합칠 수 있음
  - 단순 구현: 각 RegisterDef를 개별 ReadSpec으로 변환 (Area→FC 매핑)
- `AssembleMessage()`: `[]ReadResult`의 바이트를 조합하여 기존 `TransformToFlow`와 동일한 메시지 생성
  - 각 ReadResult에서 RegisterDef에 해당하는 바이트를 추출하여 typed value로 변환

### Step 3: ModbusAgent에 `read_raw` 명령 추가

**파일**: `internal/agent/modbus/agent.go`

- Process()에 `read_raw` 케이스 추가
- 입력: `{ "command": "read_raw", "function_code": 3, "address": 0, "quantity": 10, "unit_id": 1 }`
- 출력: `{ "data": "<base64 encoded bytes>" }` 또는 에러
- 기존 `processReadRegisters()`의 단일 디바이스 읽기 로직 재사용
- 디바이스가 1개인 경우 해당 디바이스 사용, 여러 개인 경우 unit_id로 매칭

### Step 4: BridgeNode에 브릿지 주도 폴 루프 추가

**파일**: `internal/node/bridge.go`

- `startBridgePollLoop()` 메서드 추가:
  1. `adapter.(PollableAdapter).ReadSpecs()`로 읽기 사양 목록 획득
  2. `getPollingIntervalOverride()`로 폴링 간격 결정
  3. 타이머 루프:
     - 각 ReadSpec에 대해 `transport.Send()` → `read_raw` 명령 전송
     - 응답 바이트 수집하여 `[]ReadResult` 구성
     - `adapter.(PollableAdapter).AssembleMessage(results)`로 메시지 생성
     - `recvCh`로 전송
  4. context 취소 시 루프 종료

- `Init()` 라우팅 변경:
  ```
  if pollable, ok := adapter.(PollableAdapter); ok && len(pollable.ReadSpecs()) > 0 {
      go startBridgePollLoop(...)
  } else {
      go startReceiveLoop(...)
  }
  ```

- `SetPollInterval` 호출 제거 (브릿지가 직접 관리)

### Step 5: 테스트

- `bridge_adapter_test.go`: PollableAdapter 인터페이스 테스트
- `adapter/modbus_test.go`: ReadSpecs(), AssembleMessage() 단위 테스트
- `agent/modbus/agent_test.go`: read_raw 명령 테스트
- `bridge_test.go`: startBridgePollLoop 통합 테스트

---

## Critical Files

| File | Action | Description |
|------|--------|-------------|
| `internal/node/bridge_adapter.go` | Edit | ReadSpec, ReadResult 타입 + PollableAdapter 인터페이스 추가 |
| `internal/node/adapter/modbus.go` | Edit | PollableAdapter 구현 (ReadSpecs, AssembleMessage) |
| `internal/agent/modbus/agent.go` | Edit | `read_raw` Process 명령 추가 |
| `internal/node/bridge.go` | Edit | startBridgePollLoop() 추가, Init() 라우팅 변경 |

## Reused Code

- `internal/node/adapter/modbus.go`: `TransformToFlow()` 바이트→값 변환 로직 재사용
- `internal/agent/modbus/agent.go`: `processReadRegisters()` Modbus 읽기 로직 재사용
- `internal/node/bridge.go`: `getPollingIntervalOverride()` 폴링 간격 설정 재사용

## Verification

1. `go build ./...` - 빌드 성공
2. `go test -race ./internal/node/... ./internal/agent/modbus/...` - 테스트 통과
3. xflowd 실행 후 Modbus 브릿지 노드가 설정된 간격으로 read_raw 요청 전송 확인
4. 여러 브릿지가 같은 에이전트에 연결된 경우 각각 독립적 폴링 확인
