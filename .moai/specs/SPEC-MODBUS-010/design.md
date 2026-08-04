# SPEC-MODBUS-010 기술 설계 (design.md)

> 대상: `internal/agent/modbusserver`. 본 문서는 아키텍처·데이터 모델·제어 흐름·동시성 모델을 확정한다. file:line 근거는 spec.md §2 / plan.md §2를 참조한다.

## 1. 아키텍처 개요

기존 서빙 경로:

```
Listener(TCP/RTU) → ModbusHandler.HandleConnection(handler.go:55)
    → DeviceManager.GetDevice(unitID)(handler.go:125)
    → Device.ReqHandler.HandleRequest / HandleWriteRequest(request.go:59/171)
    → registerStore(=*RegisterMap | *deviceView)   ← 여기까지 불변
```

백킹 도입 후: 백킹 디바이스만 `registerStore`를 `backedStore`로 교체한다. 순수 slave 디바이스는 오늘과 동일.

```
    → Device.ReqHandler.HandleRequest / HandleWriteRequest
    → registerStore
         ├─ 순수 slave:   *RegisterMap | *deviceView      (오늘과 동일)
         └─ 백킹:          *backedStore  ── decorates ──▶ *RegisterMap(내부 저장)
                                        └── uses ───────▶ ModbusTransport(upstream)
```

`backedStore`는 `registerStore` 인터페이스(request.go:17-24)를 구현하므로 `RequestHandler`는 백킹 여부를 몰라도 된다. `RequestHandler`의 **유일한** 변경은 예외 매핑 확장(§4).

## 2. 데이터 모델

### 2.1 BackingConfig (config.go 신규)

```
type BackingConfig struct {
    Transport    string          // "tcp" | "rtu" (기본 "tcp")
    Host         string          // TCP
    Port         int             // TCP
    Serial       SerialConfig    // RTU (config.go:42 재사용)
    UnitID       byte            // upstream 디바이스 unit id (가상 UnitID 와 독립)
    Mode         string          // "direct" | "indirect" (필수)
    PollInterval time.Duration   // indirect 전용, > 0
    Timeout      time.Duration   // upstream 요청 데드라인 + (indirect) stale 허용 한도
}
```

`DeviceConfig`(config.go:51-56)에 `Backing *BackingConfig` 추가. nil = 순수 slave.

### 2.2 backedStore (신규 파일, 예: backed_store.go)

```
type backedStore struct {
    inner     *RegisterMap            // 내부 저장(register_map.go, 자체 RWMutex 보유)
    transport modbus.ModbusTransport  // upstream (재사용)
    upUnitID  byte                    // upstream unit id
    mode      backingMode             // direct | indirect
    timeout   time.Duration
    lastOK    atomic.Int64            // indirect: 마지막 성공 폴 시각(UnixNano); direct 미사용
    // 폴러 취소는 디바이스/에이전트 레벨(agent.go)에서 소유
}

var _ registerStore = (*backedStore)(nil)  // 컴파일 타임 인터페이스 준수(request.go:27 선례)
```

`ErrGatewayTargetFailed`(errors.go 신규 sentinel)와 `ExceptionGatewayTargetFailed byte = 0x0B`(modbusserver 로컬 상수).

## 3. 제어 흐름 (mode별)

### 3.1 Direct 읽기 (예: ReadHoldingRegisters)

1. upstream 요청 PDU 조립(FC03 + start + qty; `internal/agent/modbus` 헬퍼 재사용).
2. `transport.SendAndReceive(ctxTimeout, upUnitID, pdu)`.
3. 성공: 응답 PDU 디코딩 → `inner.WriteHoldingRegisters(start, values)`(저장) → values 반환.
4. 실패(연결/타임아웃/예외): `return nil, ErrGatewayTargetFailed`.

### 3.2 Direct 쓰기 (예: WriteHoldingRegisters)

1. upstream 쓰기 PDU 조립(FC06/16).
2. `SendAndReceive` → 성공: `inner.WriteHoldingRegisters` 미러 → `ChangeSet` 반환.
3. 실패: `return nil, ErrGatewayTargetFailed` (inner 미변형).

### 3.3 Indirect 읽기

1. `elapsed = now - lastOK`.
2. `elapsed <= timeout`: `inner.ReadHoldingRegisters(start, qty)` 반환(stale 허용).
3. `elapsed > timeout`: `return nil, ErrGatewayTargetFailed`.

### 3.4 Indirect 쓰기

- Direct 쓰기와 동일(§3.2): 폴 캐시 우회, 즉시 upstream 전달. 미도달 시 `timeout` 무관 `ErrGatewayTargetFailed`.

### 3.5 Indirect 폴러 (agent.go 레벨 goroutine)

```
ticker := time.NewTicker(pollInterval)
for {
  select {
  case <-ctx.Done(): return          // remove_device / Stop 시 취소
  case <-ticker.C:
    pdu := 조립(설정된 영역들)
    resp, err := transport.SendAndReceive(ctxTimeout, upUnitID, pdu)
    if err == nil {
      inner.WriteXxx(...)            // RegisterMap 갱신(자체 RWMutex)
      lastOK.Store(now)             // 원자적
    }
    // 실패 시 lastOK 미갱신 → stale 판정이 자연히 처리
  }
}
```

## 4. 예외 매핑 (RequestHandler 변경)

현재(request.go:96 등): 모든 store 읽기 오류 → `makeExceptionPDU(fc, ExceptionIllegalDataAddress)`(0x02).

변경:

```
vals, err := rh.store.ReadHoldingRegisters(start, qty)
if err != nil {
    if errors.Is(err, ErrGatewayTargetFailed) {
        return makeExceptionPDU(fc, ExceptionGatewayTargetFailed) // 0x0B
    }
    return makeExceptionPDU(fc, ExceptionIllegalDataAddress)      // 0x02 (기존)
}
```

쓰기 경로(request.go:171~)도 동일 패턴. 순수 slave store는 `ErrGatewayTargetFailed`를 반환하지 않으므로 기존 동작 불변(하위 호환).

## 5. 동시성 모델

| 공유 상태             | 보호 메커니즘                                  | 접근자                                   |
|-----------------------|------------------------------------------------|------------------------------------------|
| `backedStore.inner` (RegisterMap) | 기존 `sync.RWMutex`(register_map.go:63) | 폴러 write, handler read/write           |
| upstream 트랜스포트    | 트랜스포트 자체 mutex(transport.go:99-100)      | direct-read/write, indirect-write, 폴러  |
| `lastOK`              | `atomic.Int64`(UnixNano)                        | 폴러 store, handler read 판정            |
| 폴러 핸들(취소 함수)   | `a.mu`(agent.go RWMutex)                        | Start/Stop/add_device/remove_device      |
| devices 맵            | `DeviceManager.mu`(device_manager.go:67)        | Add/Remove/Get (불변)                    |

핵심: RegisterMap과 트랜스포트가 이미 자체 락을 가지므로 신규 데코레이터는 락을 최소화한다. `a.mu`는 폴러 lifecycle에만 관여하고 데이터 경로에는 관여하지 않아 리스너 goroutine 블로킹을 피한다.

## 6. 배선 상세 (agent.go)

- `processAddDevice`(agent.go:1217): `req.Params["backing"]` 존재 시 → BackingConfig 파싱 → 트랜스포트 생성/connect → `backedStore` 구성 → `newRequestHandlerWithStore(backedStore, logger)`(request.go:44 재사용) → (indirect면) 폴러 goroutine 시작 + 취소함수 `a.mu` 하 등록 → `AddDevice`.
- `processRemoveDevice`(agent.go:1288): 백킹 디바이스면 폴러 취소 + `transport.Close` 후 `RemoveDevice`.
- `Start`(agent.go:165): 설정 device 중 `Backing != nil`인 것에 대해 connect/폴러-시작.
- `Stop`(agent.go:200): 모든 백킹 폴러 취소 + 트랜스포트 Close.

## 7. RTU 백킹 특수 케이스

동일 시리얼 포트를 여러 백킹 디바이스가 공유할 수 있다. 트랜스포트 자체 turnaround mutex(transport_rtu.go)로 직렬화되므로 정확성은 보장되나, 포트별 트랜스포트 공유 여부(디바이스마다 별도 vs 포트 단위 공유)는 run 단계에서 modbus-client의 기존 RTU 공유 패턴을 준용하여 확정한다. 초기 구현은 디바이스별 트랜스포트로 단순화하고, 동일 포트 공유 최적화는 후속 여지로 남긴다(YAGNI).

## 8. 테스트 전략

- **fake upstream**: `modbus.ModbusTransport`를 만족하는 테스트 더블(응답/끊김/지연 시나리오 주입).
- **direct**: 조회→저장→서빙, 끊김→0x0B, 쓰기 전달/미러.
- **indirect**: 폴러 1회+ 갱신, stale 이내 서빙, timeout 초과 0x0B, 쓰기 즉시 전달.
- **하위 호환**: 백킹 없는 디바이스 특성화(바이트 동일).
- **동시성**: 폴러+요청 동시 실행 `-race`, goroutine 누수(remove/Stop) 검증.
