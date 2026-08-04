# SPEC-MODBUS-010 코드베이스 분석 (research.md)

> 본 문서는 SPEC 작성 시 실제 소스에서 확인한 아키텍처 사실을 file:line 근거와 함께 기록한다(ANALYZE 단계). 모든 항목은 읽기로 직접 확인한 것이며 추정이 아니다.

## 1. 대상 패키지 현황

`internal/agent/modbusserver`(type id `modbus-gateway`)는 순수 slave다. 파일 구성(확인): agent.go(~57KB), config.go, device_manager.go, register_map.go, request.go, handler.go, listener.go, listener_rtu.go, observability.go, device_view.go, client_registry.go, errors.go, register.go. upstream(client/master) 통신 코드는 **전무**하다.

## 2. 서빙 경로 (검증된 호출 사슬)

1. 리스너가 프레임 수신 → `ModbusHandler.HandleConnection`(handler.go:55).
2. MBAP 파싱 후 `unitID` 추출(handler.go:78-81).
3. `dev := mh.deviceManager.GetDevice(unitID)`(handler.go:125).
4. `dev.ReqHandler`의 `HandleRequest`(request.go:59, 읽기 FC01~04) 또는 `HandleWriteRequest`(request.go:171, 쓰기 FC05/06/15/16).
5. 이들은 `rh.store`(=`registerStore`)의 `ReadCoils`/`ReadHoldingRegisters`/`WriteHoldingRegisters` 등 호출.

→ upstream 통신을 삽입할 최적 지점은 4-5 사이의 `registerStore`다.

## 3. 핵심 seam: registerStore 인터페이스

request.go:17-24에서 확인:

```
type registerStore interface {
    ReadCoils(start, quantity uint16) ([]bool, error)
    ReadDiscreteInputs(start, quantity uint16) ([]bool, error)
    ReadHoldingRegisters(start, quantity uint16) ([]uint16, error)
    ReadInputRegisters(start, quantity uint16) ([]uint16, error)
    WriteCoils(start uint16, values []bool) (*ChangeSet, error)
    WriteHoldingRegisters(start uint16, values []uint16) (*ChangeSet, error)
}
```

- `*RegisterMap`이 이를 만족(request.go:27 컴파일 타임 체크). `*deviceView`(공유 세그먼트)도 만족.
- `newRequestHandlerWithStore(store, logger)`(request.go:44)가 임의 store로 핸들러를 만든다 → 백킹 데코레이터 주입에 그대로 재사용 가능.

**결론**: `backedStore`가 이 6개 메서드를 구현하면 서빙 경로/`RequestHandler` 뼈대 변경 없이 삽입된다.

## 4. 예외 반환 현황과 0x0B 필요성

- `RequestHandler`는 store 읽기 오류를 일괄 `ExceptionIllegalDataAddress`(0x02)로 매핑(request.go:96,115,133,153). 쓰기도 유사(request.go:214,234,269,304).
- `makeExceptionPDU(fc, exCode)`(request.go:395)는 임의 exCode를 수용 → 프레이밍 변경 불필요.
- 예외 상수는 `internal/agent/modbus/protocol.go:55-58`에 `0x01`~`0x04`만 존재. **`0x0B` 부재**를 grep으로 확인(modbus 예외 계열에 0x0B 없음; century 패키지의 `0x0B`는 무관한 FCRead 상수).

**결론**: `0x0B`(Gateway Target Device Failed to Respond)을 `modbusserver`에 로컬 정의해야 한다(Non-Goal: internal/agent/modbus 미변경). 백킹 실패를 구분하려면 sentinel 오류 + `errors.Is` 필요(현재 모든 오류가 0x02로 뭉개짐).

## 5. RegisterMap 동시성 (검증)

- `RegisterMap`은 자체 `mu sync.RWMutex`(register_map.go:63)를 가진다.
- 읽기: `ReadTyped`:185, `ReadCoils`:282, `ReadHoldingRegisters`:314 등. 쓰기: `WriteTyped`:209, `WriteCoils`:351, `WriteHoldingRegisters`:365 등.

**결론**: 폴러 write와 handler read의 동시 접근은 기존 RWMutex로 안전. 신규 락 불필요(데이터 경로).

## 6. 트랜스포트 재사용 (검증)

- `modbusserver`는 이미 `internal/agent/modbus`를 import한다: request.go:7, handler.go:11(`modbus "github.com/xtra/xflow/internal/agent/modbus"`) — FC/예외/MBAP 상수 사용.
- `ModbusTransport` 인터페이스(modbus/transport.go:18-32): `Connect`/`SendAndReceive(ctx, unitID, pdu)`/`Close`/`IsConnected`. PDU-중립(ADU 프레이밍은 트랜스포트 소유).
- `ModbusTCPTransport`(transport.go:36), `NewModbusTCPTransport(host, port, timeout, logger)`(transport.go:59). `SendAndReceive`는 자체 `mu`로 직렬화(transport.go:99-100). RTU는 transport_rtu.go.

**결론**: 크로스-패키지 재사용 아키텍처가 이미 확립되어 있어, 백킹 트랜스포트는 신규 라이브러리/프로토콜 없이 이 인터페이스로 구성 가능하다.

## 7. 런타임 디바이스 관리 (이미 존재)

- `Process` 스위치(agent.go:286-338)에 `add_device`(329), `remove_device`(331), `get_device_status`(333), `list_devices`(325)가 이미 존재.
- `processAddDevice`(agent.go:1217): `register_map` 파싱 → `NewRegisterMap` → `NewRequestHandler(rm)` → `Device` → `deviceManager.AddDevice`. 여기에 백킹 분기를 추가하면 된다.
- `processRemoveDevice`(agent.go:1288): 마지막 디바이스 제거 방지 + `RemoveDevice`. 백킹이면 폴러 취소/트랜스포트 Close 추가.
- `DeviceManager.AddDevice`(device_manager.go:211)/`RemoveDevice`(device_manager.go:226): RWMutex 보호. 불변.

**결론**: 런타임 add/remove 인프라가 이미 있어 백킹 배선은 기존 훅에 얹으면 된다.

## 8. 에이전트 수명

- `Init`:134, `Start`:165, `Stop`:200, `Pause`:237, `Resume`:248, `Configure`:1430. `a.mu sync.RWMutex`(143/182/205 등에서 사용).

**결론**: 폴러 lifecycle을 `Start`/`Stop`과 정합시키고 `a.mu`로 폴러 핸들을 보호한다.

## 9. 미해결/후속 확정 항목 (run 단계 판단)

- upstream 요청 PDU 조립을 `internal/agent/modbus`의 write.go/protocol.go 헬퍼로 어디까지 재사용할지(export 범위) — run 단계에서 export 심볼 확인 후 확정. 재사용 불가 심볼은 modbusserver 측 최소 조립으로 대체(신규 라이브러리 금지 원칙 유지).
- RTU 백킹 시 동일 포트 트랜스포트 공유 여부 — 초기엔 디바이스별 트랜스포트(단순), 공유 최적화는 후속(design.md §7).
- 폴러가 폴링할 register 영역 집합의 결정(디바이스 RegisterMap 전 영역 vs 설정 명시) — 초기엔 디바이스에 정의된 영역 전체를 폴링. 설정 세분화는 후속 여지.

## 10. 잔여 위험 (Residual Risk)

- 실제 하드웨어 RTU 타이밍 정확성은 본 SPEC 범위 밖(§8 Non-Goal); fake upstream 기반 테스트로 로직만 검증한다.
- upstream 지연이 큰 direct 모드에서 리스너 goroutine 점유 위험 → `timeout` 데드라인으로 상한. 값 튜닝은 운영 설정 몫.
