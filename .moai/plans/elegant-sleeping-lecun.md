# Plan: Samsung NASA Agent - unsupported_msg_sets 설정 추가

## Context

Samsung NASA 프로토콜에서 수신되는 메시지 세트(NASAMessageSet) 중 에이전트가 해석하지 않는 인덱스가 많다.
현재는 switch에 없는 인덱스도 `RawMessageSets`에 무조건 저장되어 메모리를 차지한다.
설정에서 무시할 메시지 셋 인덱스 목록을 지정하면, 해당 인덱스를 수신 시 필터링하고 debug 로그만 남긴다.

---

## 변경 사항

### 1. NASAConfig 필드 추가 (`config.go`)

```go
type NASAConfig struct {
    // ... 기존 필드 ...
    UnsupportedMsgSets map[uint16]bool  // 필터링할 메시지 셋 인덱스 (O(1) 룩업)
}
```

### 2. 설정 파싱 추가 (`config.go`)

`parseNASAConfig()`에 `unsupported_msg_sets` 파싱 추가.
YAML에서 `0x4100` → int(16640)으로 파싱되므로 `toInt()` 재활용.

```yaml
# 사용 예
unsupported_msg_sets:
  - 0x4100
  - 0x4102
  - 0x4111
```

### 3. handleMessage 필터링 (`agent.go:984-988`)

`dev.State.UpdateFromMessageSets(msg.MessageSets)` 호출 전에 필터링:

```go
if dev.State != nil && len(msg.MessageSets) > 0 {
    sets := msg.MessageSets
    if len(a.nasaConfig.UnsupportedMsgSets) > 0 {
        sets = a.filterMessageSets(msg.MessageSets, srcAddr)
    }
    if len(sets) > 0 {
        // 이전 상태 저장 + UpdateFromMessageSets + 변경 감지
    }
}
```

`filterMessageSets` 메서드: unsupported 인덱스를 제외하고 debug 로그 남김.

### 4. State() 출력에 목록 추가 (`agent.go:1131`)

```go
return map[string]any{
    "device_count":          len(a.devices),
    "online_count":          onlineCount,
    "devices":               devices,
    "unsupported_msg_sets":  formatUnsupportedSets(a.nasaConfig.UnsupportedMsgSets),
}
```

### 5. 예제 YAML 업데이트

`examples/agents/samsung_hvacr01-tcp.yaml`에 unsupported_msg_sets 예시 추가.

---

## 파일 변경

| 파일 | 변경 |
|------|------|
| `internal/agent/samsung/config.go` | UnsupportedMsgSets 필드 + 파싱 |
| `internal/agent/samsung/agent.go` | filterMessageSets + handleMessage 필터링 + State 출력 |
| `internal/agent/samsung/agent_test.go` | 필터링 테스트 |
| `internal/agent/samsung/config_test.go` | 파싱 테스트 |
| `examples/agents/samsung_hvacr01-tcp.yaml` | 설정 예시 |

---

## 검증

1. `go test ./internal/agent/samsung/ -v -run "TestUnsupported|TestParseNASAConfig"`
2. `go build ./cmd/xflow/`
