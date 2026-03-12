# Plan: Device Auto-Registration via Agent Lifecycle Hooks

## Context

`DeviceRegistry`가 `main.go`에서 생성되지만 에이전트 시작/중지 시 `RegisterProvider`/`UnregisterProvider`가 호출되지 않아 REST API에서 디바이스가 노출되지 않는 문제. Agent Manager에 라이프사이클 훅을 추가하여 자동 등록을 구현한다.

## Approach

Agent Manager에 `OnStart`/`OnStop` 콜백 훅을 추가하고, NASAAgent/ModbusAgent에 `DeviceProvider()` 메서드를 추가한 뒤, `main.go`에서 연결한다. `internal/agent` 패키지에 `device` import 없이 Go duck typing으로 구현.

## Changes

### 1. `internal/agent/manager.go` - Manager 라이프사이클 훅 추가

- `DefaultManager` 구조체에 `onStart []func(Agent)`, `onStop []func(Agent)` 필드 추가
- `WithOnStart(fn func(Agent))`, `WithOnStop(fn func(Agent))` ManagerOption 함수 추가
- `Start()`: `agent.Start(ctx)` 성공 후 `onStart` 훅 호출
- `Stop()`: `agent.Stop(ctx)` 전에 `onStop` 훅 호출
- `Delete()`: agent 중지 전에 `onStop` 훅 호출
- `Shutdown()`: 각 agent 중지 전에 `onStop` 훅 호출

### 2. `internal/agent/samsung/agent.go` - NASAAgent DeviceProvider 메서드 추가

```go
func (a *NASAAgent) DeviceProvider() device.DeviceProvider {
    return NewNASADeviceProvider(a)
}
```

### 3. `internal/agent/modbus/agent.go` - ModbusAgent DeviceProvider 메서드 추가

```go
func (a *ModbusAgent) DeviceProvider() device.DeviceProvider {
    return NewModbusDeviceProvider(a)
}
```

### 4. `cmd/xflowd/main.go` - 라이프사이클 훅으로 연결

- `deviceRegistry` 생성을 `agentMgr` 생성 전으로 이동
- `WithOnStart` 훅: agent가 `DeviceProvider()` 메서드를 구현하면 `deviceRegistry.RegisterProvider(agent.Name(), provider)` 호출
- `WithOnStop` 훅: 동일 조건으로 `deviceRegistry.UnregisterProvider(agent.Name())` 호출

Duck typing 패턴 (main.go 내 로컬 인터페이스):
```go
type deviceProviderAgent interface {
    DeviceProvider() device.DeviceProvider
}
```

### 5. Tests

- `internal/agent/manager_test.go` - `WithOnStart`/`WithOnStop` 훅 호출 검증
- `internal/agent/samsung/agent.go` - 컴파일 타임 체크 (기존 `var _` 패턴)
- `internal/agent/modbus/agent.go` - 동일

## Files Modified

| File | Action |
|------|--------|
| `internal/agent/manager.go` | Add hook fields, options, and invocations |
| `internal/agent/manager_test.go` | Add lifecycle hook tests |
| `internal/agent/samsung/agent.go` | Add `DeviceProvider()` method |
| `internal/agent/modbus/agent.go` | Add `DeviceProvider()` method |
| `cmd/xflowd/main.go` | Move deviceRegistry, add hooks |

## Verification

1. `go build ./...` - 컴파일 확인
2. `go test -race ./internal/agent/...` - Manager 훅 테스트
3. `go test -race ./...` - 전체 테스트
4. 서버 시작 후 에이전트 생성 → 시작 → `GET /api/v1/devices` 로 디바이스 노출 확인
