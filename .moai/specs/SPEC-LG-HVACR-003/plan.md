---
id: SPEC-LG-HVACR-003
type: plan
version: "1.0.0"
created: "2026-09-14"
updated: "2026-09-14"
---

# SPEC-LG-HVACR-003 구현 계획: LG HVACR-03 (PMBUSB00A) 제어 에이전트

## 1. 구현 전략

### 접근 방식

`lg_hvacr02`(2,436줄) 를 참조 구현으로 삼되, **복제할 축과 새로 쓸 축을 명확히 가른다**.

**복제 (구조를 그대로 따름)**
- 디바이스 관리: `devices` / `lastStates` / `lastEmitted` 3-맵 구조, offline watch, report gate, pinned devices
- 메시지 방출: `emitDeviceStatePayloadLocked` 의 페이로드 구조·dedup 규칙·링버퍼+msgCh 이중 push
- 명령 디스패처: `Process()` 의 switch 구조와 요청 DTO
- DeviceProvider / CommandExecutor 브리지 구조

**신규 (근본적으로 다름)**
- 수집: 캡처 루프(수동 수신) → **폴링 스케줄러**(능동 요청)
- 발견: 프레임 관측 → **FC02 비트맵 스캔**
- 제어: 서모스탯 사칭(SEQ 추적·에코 판별·버스 정숙 대기) → **정식 레지스터 쓰기**

제어는 오히려 **대폭 단순해진다**. `lg_hvacr02` 의 제어 복잡도 대부분(`buildThermostatPayloadForCommand`, `seqManager`/`unitSeqManager`, `isEcho`, `waitForBusQuiet`, `waitForStateChange`)은 역공학 우회를 위한 것이며, 공식 Modbus 경로에서는 전부 불필요하다.

### 핵심 설계 결정

1. **파일 접두 분리**: `lg` 패키지가 이미 21,898줄이므로 신규 파일은 `lg_hvacr03_*`(에이전트 층) / `lg_pmbus_*`(프로토콜 층) 접두로 일관 분리한다. 에이전트 층과 프로토콜 층을 접두로 구분하면 "어느 층의 파일인가"가 파일명에서 바로 읽힌다.

2. **순수 함수로 프로토콜 격리**: 주소 계산·인코딩·디코딩은 트랜스포트에 의존하지 않는 순수 함수로 둔다. 문서 §5 의 완성 프레임 12종을 **바이트 단위 골든 테스트**로 고정할 수 있어, 하드웨어 없이 프로토콜 정확성이 검증된다.

3. **트랜스포트 재사용**: `internal/agent/modbus` 의 `ModbusTransport` 인터페이스를 그대로 쓴다. `io.ReadWriteCloser` seam 덕분에 mock 시리얼로 전 경로를 시험할 수 있다.

4. **미검증 가정을 설정 구조체 필드로**: `temp_scale` / `address_base` / `fan_auto_code` 를 `Hvacr03Config` 필드로 두고, 인코더·디코더가 상수 대신 이 필드를 참조한다. 현장 교정이 YAML 수정으로 끝난다.

5. **단일 직렬화 경로**: 폴링과 제어가 같은 버스를 쓰므로 `txMu` 하나로 모든 Modbus 트랜잭션을 직렬화한다. RTU 트랜스포트가 자체 turnaround mutex 를 갖지만, 에이전트 층의 "read-back 까지 한 단위" 같은 복합 트랜잭션을 보호하려면 상위 락이 별도로 필요하다.

6. **기기 종류별 투영 게이트**: ERV / 하이드로킷 전용 레지스터는 0 으로 읽혀도 유효값이 아니다. 디바이스의 `device_type` 에 따라 투영 대상 속성을 제한하여, 문서 §5.1 의 "미사용(에어컨) = 0" 워드가 대시보드에 0 °C 로 표시되는 일을 막는다.

### 락 규약 (사전 경고)

`lg_hvacr02` 에서 `a.Name()` 을 락 보유 중 호출해 자기 deadlock 을 일으킨 전례가 있다 ([lg_hvacr02_agent.go:2217](internal/agent/lg/lg_hvacr02_agent.go#L2217) 주석 참조). Go `RWMutex` 는 재귀 락을 금지하므로, **`a.mu` 보유 중에는 `a.Name()` / `a.ID()` / `a.Type()` 등 자체 락을 잡는 접근자를 호출하지 않고 `a.agentConfig.Name` 을 직접 읽는다.** 신규 코드에도 동일 규약을 적용한다.

---

## 2. 마일스톤

### M1: 프로토콜 층 — 주소 계산 · 인코딩 · 디코딩

**대상 파일 (신규)**
- `internal/agent/lg/lg_pmbus_register.go` — 주소 계산, 레지스터 상수
- `internal/agent/lg/lg_pmbus_codec.go` — 값 인코딩/디코딩
- `internal/agent/lg/lg_hvacr03_errors.go` — 센티널 에러

**작업 내용**

1. 주소 계산 순수 함수 (문서 §4.1 — 전부 0-base)
   - `pmbusCoilAddr(n, item uint16) uint16` → `n*16 + item`
   - `pmbusDiscreteAddr(n, item uint16) uint16` → `n*16 + item`
   - `pmbusHoldingAddr(n, item uint16) uint16` → `n*20 + item`
   - `pmbusInputAddr(n, item uint16) uint16` → `n*20 + item`
2. 항목 번호 상수 — 문서의 원문자(①~⑩)를 0-base 상수로. 1-base "번호 표기"(40001 등)는 **상수로 만들지 않는다** (혼동 원천 제거).
3. 코드 매핑 테이블
   - 모드: 0=cool, 1=dry, 2=fan, 3=auto, 4=heat → `hvac` 통일 ID
   - 풍량: 1=low, 2=medium, 3=high, `fan_auto_code`=auto → `hvac` 통일 ID
   - 역방향 매핑 (제어용)
4. 온도 인코딩/디코딩 — `temp_scale` 파라미터, **signed int16** 해석
5. 센티널 에러 정의
   - `ErrHvacr03ControlNotEnabled`, `ErrHvacr03DeviceNotConnected`
   - `ErrHvacr03TemperatureOutOfRange`, `ErrHvacr03InvalidAddress`
   - `ErrHvacr03MissingParam`, `ErrHvacr03UnknownTransportType`
   - `ErrHvacr03SerialPortRequired`, `ErrHvacr03PollIntervalTooShort`
   - `ErrHvacr03WriteRejected`, `ErrHvacr03UnsupportedForDeviceType`

**검증**: 문서 §4.6 조견표(N=0~15 전 행)와 계산 함수 출력 일치. 온도 음수(0xFFC4 = −6.0 °C) 디코딩.

---

### M2: PDU 빌더 — 문서 §5 프레임 골든 테스트

**대상 파일 (신규)**
- `internal/agent/lg/lg_pmbus_pdu.go` — FC01/02/03/04/05/06/16 PDU 빌더
- `internal/agent/lg/lg_pmbus_pdu_test.go` — 골든 테스트

**작업 내용**

1. 읽기 PDU 빌더: `buildReadPDU(fc byte, addr, quantity uint16) []byte`
2. 쓰기 PDU 빌더
   - `buildWriteCoilPDU(addr uint16, on bool) []byte` (ON=0xFF00 / OFF=0x0000)
   - `buildWriteRegisterPDU(addr, value uint16) []byte`
   - `buildWriteMultiplePDU(addr uint16, values []uint16) []byte`
3. 응답 파서
   - `parseBitResponse(pdu []byte, quantity uint16) ([]bool, error)`
   - `parseRegisterResponse(pdu []byte, quantity uint16) ([]uint16, error)`
   - Modbus 예외 응답(FC | 0x80) 처리

**검증 (핵심 게이트)**: 문서 §5 의 완성 프레임 **12종 전부**를 골든 테스트로 고정한다. `BuildRTUADU(slaveID, pdu)` 결과가 문서의 hex 와 바이트 단위로 일치해야 한다.

```
IDU 0 전원 ON      01 05 00 00 FF 00 8C 3A
IDU 0 전원 OFF     01 05 00 00 00 00 CD CA
IDU 3 전원 ON      01 05 00 30 FF 00 8C 35
IDU 0 난방 모드     01 06 00 00 00 04 88 09
IDU 0 풍량 강      01 06 00 01 00 03 98 0B
IDU 0 온도 24.0    01 06 00 02 00 F0 28 4E
IDU 3 온도 26.0    01 06 00 3E 01 04 E8 55
IDU 0 일괄(FC16)   01 10 00 00 00 03 06 00 04 00 03 00 F0 E7 04
IDU 0 상태 읽기     01 03 00 00 00 06 C5 C8
IDU 0 센서 읽기     01 04 00 00 00 06 70 08
IDU 0 상태비트     01 02 00 00 00 05 B8 09
IDU 0 코일 상태     01 01 00 00 00 0A BC 0D
전체 스캔          01 02 00 00 01 00 79 9A
```

> 이 테스트가 통과하면 CRC 다항식·엔디안·주소 0-base·온도 스케일이 **한 번에** 검증된다. 문서 §8 #1(온도 ×10)·#2(N 0-base)의 기본값 타당성도 여기서 확인된다.

응답 파싱은 문서 §5.1 의 응답 예(`01 04 0C 0000 00F5 00D2 00E8 0000 0000`)로 검증한다.

---

### M3: 설정 파싱

**대상 파일 (신규)**
- `internal/agent/lg/lg_hvacr03_config.go`
- `internal/agent/lg/lg_hvacr03_config_test.go`

**작업 내용**

1. `Hvacr03Config` 구조체 — SPEC §5 설정 스키마 전 필드
2. `parseHvacr03Config(opts map[string]any) (Hvacr03Config, error)`
   - 기본값 적용 (SPEC §5 주석의 기본값과 일치)
   - `transport_type` 검증: `rtu` / `tcp-client` 외 → `ErrHvacr03UnknownTransportType`
   - `rtu` 인데 `serial_port` 없음 → `ErrHvacr03SerialPortRequired`
   - `tcp-client` 인데 `tcp_host` 없음 → 에러
   - `slave_id` 범위 1~16 검증
   - **`poll_interval` < 5s → `ErrHvacr03PollIntervalTooShort`** (문서 §6.1)
   - `temp_scale` / `address_base` / `fan_auto_code` 파싱
   - `devices` 항목을 `agent.DeviceEntry` 로 변환, address 는 0~15 정수 문자열 검증
3. 타입 강제 변환 방어 — `lg_hvacr02` 의 `v.(string)` 무검사 단언 패턴은 잘못된 YAML 에 panic 을 낸다. 신규 코드는 `v, ok := x.(string)` 형태로 받고 실패 시 설정 오류를 반환한다.

---

### M4: 트랜스포트 배선 및 생명주기

**대상 파일 (신규)**
- `internal/agent/lg/lg_hvacr03_agent.go` — 구조체, Init/Start/Stop/Pause/Resume/Health/Info/Stats/State
- `internal/agent/lg/lg_hvacr03_transport.go` — 트랜스포트 생성 및 재연결

**작업 내용**

1. `Hvacr03Agent` 구조체 정의 — `lg_hvacr02` 필드 구성을 기준으로 하되 캡처 전용 필드(`frameBuilder`, `seqManager`, `unitSeqManager`, `lastSentFrame`, `isEcho` 관련)를 제거하고 폴링 필드를 추가
2. 인터페이스 구현 선언
   - `agent.Agent`, `agent.MessageReceiver`, `agent.StatefulAgent`
   - `agent.BufferInfoProvider`, `agent.TransportChecker`, `agent.FrameNotifier`
3. 트랜스포트 생성 — `transport_type` 에 따라 `modbus.NewModbusRTUTransport` 또는 `modbus.NewModbusTCPTransport`
4. `bgStarted atomic.Bool` 가드 — `lg_hvacr02` 의 고루틴 누적 방지 패턴을 그대로 적용 (누적 시 같은 디바이스 report 가 한 틱에 N건 중복 발행된다)
5. 재연결 루프 — 지수 백오프, 끊김 시 전 디바이스 오프라인 전이
6. `txMu` 단일 직렬화 락 + `sendAndReceive(pdu []byte) ([]byte, error)` 래퍼

---

### M5: 디바이스 발견 및 관리

**대상 파일 (신규)**
- `internal/agent/lg/lg_pmbus_device.go` — `PmbusDevice`, `PmbusDeviceState`, `toProperties`
- `internal/agent/lg/lg_hvacr03_device_mgmt.go` — 발견·등록·제거·오프라인 감시

**작업 내용**

1. `PmbusDevice` — `lg_hvacr02` 의 `Icp02Device` 구조 복제 (`Address`, `Label`, `Type`, `Online`, `LastSeen`, `Source`, `State`, `ReportEnabled`)
2. `PmbusDeviceState` — SPEC §4.4 의 전체 속성. 모든 필드 포인터 타입으로 "미관측"과 "0" 을 구분
3. FC02 전체 스캔 (`scanDevices`)
   - 1 트랜잭션으로 Discrete 0~255 읽기
   - N 마다 `N*16+0`(연결) / `+1`(알람) / `+2`(필터) 비트 추출
   - `address_base` 반영
4. `ensureDevice` / `registerConfigDevices` / `RegisterPinnedDevices`
5. `offlineWatchLoop` — `offline_timeout` 초과 시 오프라인 전이
6. `processListDevices` / `processRemoveDevice` / `processSetDevice` — NASA 통일 DTO 형식 복제

---

### M6: 폴링 엔진 및 상태 투영

**대상 파일 (신규)**
- `internal/agent/lg/lg_hvacr03_poll.go` — 폴링 스케줄러
- `internal/agent/lg/lg_hvacr03_emit.go` — 상태 방출

**작업 내용**

1. 폴링 스케줄러
   - `scanLoop` — `scan_interval` 주기 FC02 전체 스캔
   - `pollLoop` — `poll_interval` 주기로 온라인 디바이스 순회
   - 디바이스당 3 트랜잭션: FC01(코일 10bit) / FC03(Holding 6word) / FC04(Input 6word)
   - 타임아웃 시 해당 N 스킵, 사이클 계속 (REQ-POLL-004)
   - 오프라인 디바이스 스킵 (REQ-POLL-005)
2. 디코딩 → `PmbusDeviceState` 병합
3. `toProperties(devType)` — 기기 종류별 게이트 + 전원 OFF 시 운전 속성 생략
4. `emitDeviceStatePayloadLocked` — `lg_hvacr02` 페이로드 구조 복제
   - `report_enabled` 게이트 (lastEmitted 갱신 전 반환)
   - `trigger != "report"` 일 때만 dedup
   - `last_seen_ms`: report 는 now, 그 외는 관측 시각
   - 링버퍼 + msgCh 이중 push
5. `event_temp_threshold` 게이트 — 온도만 변경 + |Δ| < 임계 → suppress
6. `notifyLoop` — `report_interval` 주기 전체 방출

---

### M7: 제어

**대상 파일 (신규)**
- `internal/agent/lg/lg_hvacr03_control.go`
- `internal/agent/lg/lg_hvacr03_control_test.go`

**작업 내용**

1. `processControlCommand` 공통 전처리
   - `control_enabled` 검사 → `ErrHvacr03ControlNotEnabled`
   - `address` 파싱 및 0~15 범위 검증
   - **연결 상태 확인** (Discrete ①) → 미연결 시 `ErrHvacr03DeviceNotConnected`
2. 기본 제어 5종 (SPEC §4.7 REQ-CTRL-002)
   - `set_multiple` 은 **FC16 단일 트랜잭션** — 개별 FC06 분할 금지
3. 확장 제어 7종 (SPEC §4.7 REQ-CTRL-003)
   - `set_swing`, `clear_filter_alarm`, `set_lock`, `set_temp_limit`
   - `set_erv_mode`, `set_erv_rapid`, `set_erv_eco` — 기기 종류 미일치 시 `ErrHvacr03UnsupportedForDeviceType`
4. 온도 범위 검증 — `[16.0, 30.0]` + 현재 상·하한(Holding ④/⑤)
5. read-back 검증
   - `control_verify_delay` 대기 후 해당 레지스터 재읽기
   - 불일치 + 잠금 코일 설정됨 → 응답에 `locked_by` 진단 포함
   - 응답 형식: `{status, verified, expected, actual, locked_by?}`

---

### M8: 통합 — Provider · Executor · 등록

**대상 파일**
- `internal/agent/lg/lg_hvacr03_provider.go` (신규)
- `internal/agent/lg/lg_hvacr03_executor.go` (신규)
- `internal/agent/lg/lg_hvacr03_register.go` (신규)
- `internal/device/adapter/lg_pmbus.go` (신규)
- `cmd/xflowd/main.go` (수정)

**작업 내용**

1. `adapter.LGPmbusDeviceAdapter` — `LGIcp02DeviceAdapter` 구조를 따르되 `lgPmbusIndoorCommandSpecs()` 로 확장 명령 스펙 노출. 기존 어댑터를 재사용하지 않는 이유: `lgIcp02IndoorCommandSpecs()` 가 5종 명령으로 고정되어 있어 확장 7종을 표현할 수 없다.
2. `Hvacr03DeviceProvider` — `control_enabled` + 실내기일 때 `ControllableDevice` 반환
3. `newHvacr03Executor` — `Process()` 브리지
4. `RegisterLGHvacr03Types(mgr)` — 타입 ID `lg_hvacr03`
5. `cmd/xflowd/main.go` 배선 — 기존 `lg.RegisterLGHvacr02Types` 다음 줄

> **등록 누락 주의**: 타입 등록 함수를 정의만 하고 `main.go` 에 배선하지 않으면 에이전트가 조용히 사라진다 (기존 사례 있음). M8 완료 판정에 실제 기동 확인을 포함한다.

---

### M9: 프론트엔드 등록

**대상 파일 (수정)**

| 파일 | 수정 지점 |
|------|----------|
| `web/src/config/agentSchemas.ts` | 타입 옵션 목록 · 타입 배열 · 필드 맵 (3곳) + `LG_HVACR03_FIELDS` 정의 |
| `web/src/pages/agents/agentTypeMeta.ts` | 메타 항목 |
| `web/src/pages/agents/twoColumnConfigMap.ts` | 2열 설정 레이아웃 |
| `web/src/pages/agents/AgentTypesPage.tsx` | 카테고리 `'device'` |
| `web/src/pages/devices/DeviceListPage.tsx` | `REMOVABLE_AGENT_TYPES` · `REPORT_TOGGLE_AGENT_TYPES` (2곳) |

**작업 내용**

1. 위 지점 전부에 `lg_hvacr03` 추가
2. `LG_HVACR03_FIELDS` — SPEC §5 설정 스키마 전 필드의 폼 스키마
3. 등록 지점 누락 검출 — 전수 grep 으로 `lg_hvacr02` 가 나타나는 프론트 파일 목록과 `lg_hvacr03` 목록을 비교한다. `AgentDetailPanel.tsx` 의 `isLgIcp` 는 LGCP 계열 전용 분기이므로 **추가하지 않는다** (PMBUSB00A 는 hex 주소를 쓰지 않음).

---

### M10: 예제 및 문서

**대상 파일 (신규)**
- `examples/agents/lg_hvacr03-rtu.yaml` — RTU 기본 구성
- `examples/agents/lg_hvacr03-control.yaml` — 제어 활성 구성
- `internal/agent/lg/README.md` (수정) — `lg_hvacr03` 절 추가

**작업 내용**

1. 예제 YAML 2종
2. README 에 3계열 비교표 추가 (`lg_hvacr01` / `lg_hvacr02` / `lg_hvacr03`)
3. **현장 실측 체크리스트** 문서화 — 문서 §8 의 8개 항목 중 설정으로 교정 가능한 3개를 "설치 직후 확인" 절차로 기술

---

## 3. 마일스톤 의존 관계

```
M1 (프로토콜: 주소·코덱)
 └─▶ M2 (PDU 빌더 + 골든 테스트)   ← 핵심 검증 게이트
      └─▶ M3 (설정 파싱)
           └─▶ M4 (트랜스포트·생명주기)
                ├─▶ M5 (디바이스 발견·관리)
                │    └─▶ M6 (폴링·상태 투영)
                │         └─▶ M7 (제어)
                │              └─▶ M8 (Provider·Executor·등록)
                │                   └─▶ M9 (프론트엔드)
                │                        └─▶ M10 (예제·문서)
                └─(M5 와 병행 불가: 동일 에이전트 구조체 수정)
```

M1~M3 은 트랜스포트 없이 순수 함수·설정만 다루므로 하드웨어는 물론 mock 도 필요 없다. M2 의 골든 테스트가 통과하기 전에는 M4 이후로 진행하지 않는다 — 프로토콜이 틀린 채로 상위 층을 쌓으면 디버깅 비용이 폭증한다.

---

## 4. 시험 전략

### 하드웨어 없는 전 경로 검증

`ModbusRTUTransport` 의 `io.ReadWriteCloser` seam 과 `rtuSerialOpener` 함수 seam 덕분에 mock 시리얼로 전 경로를 시험할 수 있다. mock 은 요청 ADU 를 받아 미리 정의한 응답 ADU 를 돌려주는 스크립트 방식으로 구성한다.

| 층 | 시험 방식 | 하드웨어 |
|----|----------|:--------:|
| M1 주소·코덱 | 순수 함수 테이블 테스트 | 불필요 |
| M2 PDU | 문서 §5 골든 테스트 | 불필요 |
| M3 설정 | 파싱 단위 테스트 | 불필요 |
| M4 트랜스포트 | mock `io.ReadWriteCloser` | 불필요 |
| M5 발견 | mock FC02 응답 스크립트 | 불필요 |
| M6 폴링·투영 | mock 응답 + 페이로드 assert | 불필요 |
| M7 제어 | mock 요청 캡처 + read-back 스크립트 | 불필요 |
| M8 통합 | Provider/Executor 단위 테스트 | 불필요 |
| 현장 | 문서 §8 체크리스트 8종 | **필요** |

### 회귀 방지 시험

1. **페이로드 동형성**: `lg_hvacr03` 의 `device_state` 키 집합이 `lg_hvacr02` 의 기본 키 집합을 **포함**하는지 검사하는 테스트. 복제 의도가 코드 변경으로 무너지는 것을 막는다.
2. **CRC 격리**: `lg_hvacr03_*` / `lg_pmbus_*` 파일이 LGCP 체크섬 심볼을 참조하지 않음을 grep 으로 검증.
3. **등록 지점 대칭**: 백엔드 타입 등록과 프론트 등록 지점의 대칭성 검증.

### 커버리지 목표

TRUST 5 Tested 기준 85% 이상. 프로토콜 층(M1·M2)은 순수 함수이므로 **100%** 를 목표로 한다.

---

## 5. 미실측 범위의 처리

ERV(환기)·THERMA V(하이드로킷) 전용 레지스터는 현장 장비가 없어 실동작을 확인할 수 없다. 다음 방침을 적용한다.

1. 구현과 단위 시험은 문서 기준으로 **수행한다** (문서에 정의된 레지스터를 빠뜨리지 않음).
2. 해당 명령·속성에 `// 미실측: ERV 장비 확보 후 검증 필요` 주석을 남긴다.
3. 기기 종류 게이트로 에어컨 디바이스에는 노출하지 않아, 미검증 값이 대시보드에 새어 나오지 않게 한다.
4. README 현장 체크리스트에 미실측 항목을 명시한다.

---

## 6. 산출물

**신규 Go 파일 (16)**

```
internal/agent/lg/lg_pmbus_register.go
internal/agent/lg/lg_pmbus_codec.go
internal/agent/lg/lg_pmbus_pdu.go
internal/agent/lg/lg_pmbus_device.go
internal/agent/lg/lg_hvacr03_errors.go
internal/agent/lg/lg_hvacr03_config.go
internal/agent/lg/lg_hvacr03_agent.go
internal/agent/lg/lg_hvacr03_transport.go
internal/agent/lg/lg_hvacr03_device_mgmt.go
internal/agent/lg/lg_hvacr03_poll.go
internal/agent/lg/lg_hvacr03_emit.go
internal/agent/lg/lg_hvacr03_control.go
internal/agent/lg/lg_hvacr03_provider.go
internal/agent/lg/lg_hvacr03_executor.go
internal/agent/lg/lg_hvacr03_register.go
internal/device/adapter/lg_pmbus.go
```

**수정 파일 (7)**

```
cmd/xflowd/main.go
web/src/config/agentSchemas.ts
web/src/pages/agents/agentTypeMeta.ts
web/src/pages/agents/twoColumnConfigMap.ts
web/src/pages/agents/AgentTypesPage.tsx
web/src/pages/devices/DeviceListPage.tsx
internal/agent/lg/README.md
```

**예제 (2)**

```
examples/agents/lg_hvacr03-rtu.yaml
examples/agents/lg_hvacr03-control.yaml
```
