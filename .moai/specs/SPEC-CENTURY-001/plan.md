# SPEC-CENTURY-001: 구현 계획

> **SPEC ID**: SPEC-CENTURY-001
> **버전**: 0.2.0
> **개발 방법론**: Hybrid (v0.2.0 M6 도 TDD 적용 — 신규 transport_tcp 패키지)
> **상태**: Draft (M6 구현 진행 예정; v0.1.2 까지 Implemented)
> **커버리지 목표**: 85% 이상 (`.moai/config/sections/quality.yaml` 의 `hybrid_settings.min_coverage_new`)
> **달성 커버리지**: `internal/agent/century` 88.9%, `internal/node/century.go` 평균 87.4%
> **테스트 명령**: `go test -race ./internal/agent/century/...`, `go test -race ./internal/node/...`, `cd web && npm test`

## 변경 이력

| 날짜 | 버전 | 변경 |
|------|------|------|
| 2026-05-18 | 0.1.0 | 초안 작성 (M1~M5 마일스톤, 8개 리스크) |
| 2026-05-18 | 0.1.1 | M3 deliverable 에 다중 IDU 자동 발견 + WRITE 중복 제거(cycle tracker + writeDeduplicator) 추가. 리스크 R3 (다중 IDU 보류) 삭제 및 R3' (다중 IDU 검증 한계) 신설. R7' (cycle 경계 감지 오류) 신설 — 기존 R7/R8 은 R8/R9 로 번호 이동. |
| 2026-05-18 | 0.1.2 | M1-M5 구현 완료. 마일스톤 표에 상태(✓ Done) 및 인계 commit 추가. §10 "구현 완료" 신설 — 최종 metrics, commit chain, Known Limitations 명시. 상태 Draft → Implemented. |
| 2026-05-18 | 0.2.0 | M6 마일스톤 신설 (TCP transport — tcp-client + tcp-server, exponential backoff 재연결, transport-aware cycle_idle_timeout default). M5 까지 commit chain 보존, M6 는 신규. Risk register 에 R10/R11/R12 추가. 상태 Implemented → Draft. |

---

## 1. 마일스톤 개요

| 마일스톤 | 내용 | 우선순위 | 의존성 | 커버 REQ | 상태 | 인계 Commit |
|---------|------|---------|-------|----------|------|-------------|
| M1 | Foundation: 파일 스켈레톤 + CRC + frame scanner | Primary Goal | SPEC-SERIAL-001 트랜스포트 | REQ-CENTURY-003, REQ-CENTURY-004, REQ-CENTURY-005 | ✓ Done | bfdfaf0 |
| M2 | 디코더: reg 0x02/0x03/0x04 응답 + reg 0x04 write + ACK | Primary Goal | M1 | REQ-CENTURY-006, REQ-CENTURY-007, REQ-CENTURY-008, REQ-CENTURY-009, REQ-CENTURY-010, REQ-CENTURY-011, REQ-CENTURY-020, REQ-CENTURY-021, REQ-CENTURY-026 | ✓ Done | d33da37 |
| M3 | CenturyAgent + ring buffer + 디바이스 관리(다중 IDU 자동 발견) + WRITE 중복 제거 + 타입 등록 | Primary Goal | M2 | REQ-CENTURY-001, REQ-CENTURY-002, REQ-CENTURY-012, REQ-CENTURY-013, REQ-CENTURY-014, REQ-CENTURY-015, REQ-CENTURY-025, REQ-CENTURY-027 | ✓ Done | bad2e06 |
| M4 | 플로우 노드 4종 + Web UI 스키마 | Secondary Goal | M3 | REQ-CENTURY-016, REQ-CENTURY-017, REQ-CENTURY-018, REQ-CENTURY-019, REQ-CENTURY-022, REQ-CENTURY-023 | ✓ Done | 3f1b970 |
| M5 | Polish & QA: 예시 YAML + 문서 + 풀 커버리지 + 구조화 로그 | Final Goal | M4 | REQ-CENTURY-024, REQ-CENTURY-025 | ✓ Done | [M5 commit] |
| M6 | TCP Transport (v0.2.0): tcp-client + tcp-server, exponential backoff 재연결, transport-aware cycle_idle_timeout default, 회귀 보장 | Primary Goal (v0.2.0) | M5, SPEC-LGCNP TCP 패턴 참조 | REQ-CENTURY-028, REQ-CENTURY-029, REQ-CENTURY-030, REQ-CENTURY-031, REQ-CENTURY-032 | Planned | - |

**의존성 그래프**:

```
M1 (foundation)
 └→ M2 (decoders)
      └→ M3 (agent + devices) ───┐
                                  ├→ M4 (nodes + web) ──→ M5 (polish) ──→ M6 (TCP transport, v0.2.0)
                                  │
            (M3 와 M4 의 일부는 M2 완료 후 부분 병렬 가능:
             M3.3 디바이스 관리와 M4.1 노드 스켈레톤은 동시 진행 가능)

M6 는 M5 까지의 모든 결과물(agent.transportProvider abstraction, config.go, ring buffer, cycle tracker)
위에 transport_tcp.go + transport_serial.go (refactor 분리) 만 신규 추가. v0.1.2 의 회귀 영향 없음.
```

---

## 2. M1: Foundation (Primary Goal)

**목표**: 패키지 스켈레톤, CRC-16/ARC 구현, 프레임 스캐너. CAP-3 의 read response reg 0x02 프레임이 골든 픽스처로 CRC 검증·헤더 파싱·길이 추출까지 통과.

### 2.1 파일

- `internal/agent/century/crc.go` — CRC-16/ARC 함수 (init **`0x0000`**)
- `internal/agent/century/crc_test.go` — TDD: CAP-3 / CAP-1 / CAP-4 의 모든 프레임 CRC 검증 + Modbus init `0xFFFF` 회귀 거부 테스트
- `internal/agent/century/errors.go` — 센티널 에러:
  - `ErrCRCMismatch`, `ErrLengthMismatch`, `ErrHeaderInvalid`, `ErrPayloadPrefixInvalid`, `ErrRegisterLengthInvalid`
  - `ErrSerialPortRequired`, `ErrUnknownTransportType`, `ErrInvalidBaudRate` 등 설정 에러
- `internal/agent/century/frame.go` — 구조체: `CenturyFrame{SrcAddr, DstAddr, FunctionCode, Payload, CRC, Raw}`, 헬퍼 메서드 (`Register()`, `Data()`, `IsAck()`, `Role()` ← src_addr 으로 master/slave 판정)
- `internal/agent/century/frame_scanner.go` — `FrameScanner` 구조체. 입력 `io.Reader`, 출력 `(*CenturyFrame, validationStage, error)`. 단계:
  1. 헤더 8B 읽기 → 검증
  2. `payload_length` 합리 범위 (`1 ≤ N ≤ MaxPayloadLength=256`)
  3. payload + CRC 읽기
  4. CRC 검증 → `validationStage="crc"` 까지 도달
  5. 페이로드 prefix (ACK 제외) 검증 → `validationStage="payload_prefix"`
- `internal/agent/century/frame_scanner_test.go` — TDD: 정상/단편화/노이즈 stream/잘못된 reserved/잘못된 fc/payload_length 범위 위반 등 table-driven
- `internal/agent/century/common.go` — 상수: function code (`FCResponse=0x06`, `FCRead=0x0B`, `FCWrite=0x0C`), 마스터/슬레이브 기본 주소, `MaxPayloadLength`, 검증 단계 enum

### 2.2 테스트 픽스처

`internal/agent/century/testdata/` 에 다음 hex 파일 보관:

- `cap1_off_cycle.hex` — CAP-1 한 사이클 (꺼짐, 9 프레임)
- `cap3_cool_start_cycle.hex` — CAP-3 한 사이클 (냉방 시작)
- `cap4_cool_steady_cycle.hex` — CAP-4 한 사이클 (냉방 정상상태)

각 hex 파일은 프로토콜 문서 §부록 A/B/C 의 raw bytes 를 그대로 base16 으로 인라인 저장. 테스트 로더는 공백·주석을 무시한다.

### 2.3 출구 기준 (Exit Criteria)

- [ ] `go test ./internal/agent/century/...` 통과 (CRC + frame_scanner 만)
- [ ] CAP-3 의 read response reg 0x02 프레임이 헤더 파싱 + CRC 검증 + payload prefix 추출까지 성공
- [ ] CRC 회귀: Modbus init `0xFFFF` 로 동일 프레임 검증이 **반드시 실패** 함을 명시 테스트로 보장
- [ ] 단편화·재동기화 케이스 6종 이상 통과

---

## 3. M2: Decoders (Primary Goal)

**목표**: reg 0x02/0x03/0x04 응답 + reg 0x04 write + ACK 의 5종 디코더. CAP-1/CAP-3/CAP-4 의 모든 프레임이 의미 있는 페이로드로 디코딩.

### 3.1 파일

- `internal/agent/century/message.go` — `MessagePayload`, `FieldMeta{Value, Raw, ConfirmationStatus}`, `ConfirmationStatus` enum (`confirmed`/`inferred`/`unknown`/`raw`)
- `internal/agent/century/decoder_reg02.go` — `decodeReg02Response(frame *CenturyFrame) (*MessagePayload, error)`. REQ-CENTURY-006 의 17B 전체 필드 노출
  - `mode` enum 디코딩: `0x00`→`"off"`, `0x01`→`"cooling"`, 그 외 `"mode_unknown_<hex>"` + `confirmation_status=unknown`
- `internal/agent/century/decoder_reg03.go` — `decodeReg03Response`. REQ-CENTURY-007, word0/word1 LE u16 ÷10
- `internal/agent/century/decoder_reg04.go` — 두 함수:
  - `decodeReg04Response(frame) (*MessagePayload, error)` — REQ-CENTURY-008 (14B)
  - `decodeReg04Write(frame) (*MessagePayload, error)` — REQ-CENTURY-009 (16B). `observation_mode="passive"` 마커 포함
- `internal/agent/century/decoder_ack.go` — `decodeAck(frame) (*MessagePayload, error)` — REQ-CENTURY-010 (1B payload)
- `internal/agent/century/register.go` — `Dispatch(frame *CenturyFrame) (*MessagePayload, error)` — REQ-CENTURY-005, REQ-CENTURY-011: 길이 → CRC (이미 frame_scanner 단계) → 헤더 → payload prefix → 디코더 매핑

### 3.2 테스트

- `decoder_reg02_test.go` — CAP-1 (off), CAP-3 (cooling start), CAP-4 (cooling steady) 의 reg 0x02 응답 모두 검증:
  - CAP-1: `mode="off"`, `fan=0`, `setpoint_c=25.0`
  - CAP-3: `mode="cooling"`, `fan=17`, `setpoint_c=25.0`
  - CAP-4: 동일 + `reg02_live_14=0x38` (CAP-3 의 `0x39` 와 미세 변동 확인)
- `decoder_reg03_test.go`:
  - CAP-1: `temp_evap_a_c=21.5`, `temp_evap_b_c=22.0`
  - CAP-3: 둘 다 19.5 (과도)
  - CAP-4: 9.0 / 8.5 (정상)
- `decoder_reg04_test.go`:
  - response: CAP-3 `status_bits=0x3B`, `temp_A_c=25.2`, `op_val_2=252`. CAP-4 `op_val_1=996`, `op_val_2=1248`
  - write: CAP-3 `mode_cmd="cooling"`, write_live_0/1=0, write_byte_14=0xC7. CAP-4 write_live_0=2, write_live_1=4, write_byte_14=0xC0, write_live_15 in [0x0F..0x11]
- `decoder_ack_test.go` — payload `0x00` 의 ACK 검증
- 각 디코더는 `confirmation_status` 분류가 REQ-CENTURY-021 의 기준과 일치하는지 명시적 단언

### 3.3 출구 기준

- [ ] `go test -race ./internal/agent/century/...` 모든 디코더 테스트 통과
- [ ] CAP-3 한 사이클 전체(9 프레임) 가 디코더 디스패치를 통해 5종 메시지 타입으로 분류
- [ ] `confirmation_status` 필드가 모든 typed field 에 노출됨
- [ ] additive enum 검증: 가공된 `mode=0x02` 프레임이 `mode_unknown_02` 로 디코딩되고 에러 없이 처리됨

---

## 4. M3: Agent + Ring Buffer + Devices (Primary Goal)

**목표**: `CenturyAgent` 의 lifecycle, capture loop, ring buffer, 디바이스 자동 발견, 오프라인 감지, DeviceProvider, 에이전트 매니저 등록.

### 4.1 파일

- `internal/agent/century/common.go` 확장 — 기본값 상수 (`DefaultRingBufferSize=128`, `DefaultOfflineTimeout=5*time.Second`, `DefaultReadTimeout=200*time.Millisecond`)
- `internal/agent/century/config.go` — `CenturyConfig` 구조체 + `parseCenturyConfig(opts map[string]any) (CenturyConfig, error)` — REQ-CENTURY-002 의 모든 필드. hex 입력 허용 (`master_address: "0x0030"` 또는 `48` 모두 수용)
- `internal/agent/century/config_test.go` — 기본값, 필수 누락, 잘못된 enum, hex 문자열 파싱
- `internal/agent/century/ring_buffer.go` — 고정 크기 ring buffer. `Push(*CapturedFrame)`, `GetRecent(count int, lastSeq uint64) []*CapturedFrame`, `Drain() []*CapturedFrame`, `Stats() RingBufferStats`. 드롭 시 `framesDropped` 카운터 증가
- `internal/agent/century/ring_buffer_test.go` — 오버플로 → evict, drain → 비우기, 동시성(`-race`)
- `internal/agent/century/device.go` — `CenturyDevice`, `CenturyDeviceState`. **다중 IDU 지원**: `devices map[uint8]*CenturyDevice` (sub_dev_id 키). 통일 속성명 (`power`, `mode`, `fan_speed`, `target_temp`, `current_temp`) + Century 전용 (`temp_evap_a`, `temp_evap_b`, `op_val_1`, `op_val_2`, `status_bits`)
- `internal/agent/century/provider.go` — `CenturyDeviceProvider` 구현 (`device.DeviceProvider`). 모든 등록된 sub_dev_id 의 device 를 노출
- `internal/agent/century/cycle_tracker.go` — **신규 (REQ-CENTURY-027)**. polling cycle 경계 감지:
  - 1차 신호: 마지막 reg `0x04` 응답 또는 ACK 직후를 cycle 시작점으로 마킹
  - 2차 신호 (fallback): inter-frame idle > 100ms 시 새 cycle
  - `IsNewCycle(now time.Time, frame *CenturyFrame) bool` API
- `internal/agent/century/write_deduplicator.go` — **신규 (REQ-CENTURY-027)**. 현재 cycle 내 마지막 WRITE frame 의 raw payload hash 보관. `ShouldEmit(frame *CenturyFrame) bool`. `dedupe_writes=false` 시 항상 true 반환
- `internal/agent/century/agent.go` — `CenturyAgent`:
  - 임베딩: `lifecycle.BaseLifecycle`
  - 필드: config, transport(`io.ReadWriteCloser` 인터페이스 — 사실은 RX-only 로 사용), frameScanner, ringBuffer, `devices map[uint8]*CenturyDevice`, msgCh, frameNotifyCh, stats (atomic), logger, **cycleTracker, writeDeduplicator** (REQ-CENTURY-027)
  - 인터페이스 구현: REQ-CENTURY-001 + 4.2 의 전체 set
  - `Start()`: 트랜스포트 open → captureLoop goroutine 시작
  - `captureLoop()`: frameScanner 로부터 프레임 수신 → Dispatch → ring buffer 저장 → cycleTracker 갱신 → WRITE frame 인 경우 writeDeduplicator 적용 → 디바이스 상태 갱신(sub_dev_id 기반 자동 발견 포함) → frameNotifyCh 신호
  - **다중 IDU 자동 발견**: 새 `sub_dev_id` 가 frame 에서 관측될 때 자동으로 새 CenturyDevice 인스턴스 생성 (Source="auto")
  - `Process(msg)`: `get_stats` / `get_recent` / `drain` / `not_supported` (그 외). get_stats 응답에 `writes_deduped` 카운터 포함
  - **Write 경로 없음** — `Process()` 가 절대 `transport.Write()` 를 호출하지 않음을 코드 리뷰 시 명시
  - 오프라인 감지: ticker (1초 주기) 로 각 디바이스의 `lastSeen` 검사, `offline_timeout` 초과 시 전이 + 콜백
- `internal/agent/century/agent_test.go` — lifecycle Init→Start→Pause→Resume→Stop, get_stats / get_recent / drain 커맨드, 디바이스 자동 발견(단일 + 다중 sub_dev_id), 오프라인 감지 (모의 시간), WRITE 중복 제거(REQ-CENTURY-027 — dedupe_writes=true/false, cycle 경계, idle gap)
- `internal/agent/century/cycle_tracker_test.go` — **신규**. cycle 경계 감지 단위 테스트 (reg 0x04 응답 마커, 100ms idle fallback, 연속 sub_dev_id 변경)
- `internal/agent/century/write_deduplicator_test.go` — **신규**. 동일 raw payload 중복 1회 emit, cycle 경계 넘어가면 2회 emit, dedupe_writes=false 시 항상 emit
- `internal/agent/century/registration.go` — `RegisterCenturyTypes(mgr *agent.DefaultManager) error`
- `cmd/xflowd/main.go` 수정 — `century.RegisterCenturyTypes(agentMgr)` 호출 추가 (samsung/lg 등록 라인 인근)

### 4.2 인터페이스 구현 체크리스트

- [ ] `agent.Agent` (Init, Start, Stop, Pause, Resume, Configure, Process, ID, Name, Type, Info, Stats, Health)
- [ ] `agent.MessageReceiver` — `ReceiveMessage() <-chan []byte`
- [ ] `agent.StatefulAgent` — `State() agent.AgentState`
- [ ] `agent.TransportChecker` — `TransportConnected() bool`
- [ ] `agent.BufferInfoProvider` — `BufferInfo() agent.BufferInfo`, `FrameNotifyCh() <-chan struct{}`
- [ ] `device.DeviceProvider` — via `CenturyDeviceProvider`

### 4.3 통계 카운터 (REQ-CENTURY-025)

```
framesCaptured            atomic.Uint64
framesValid               atomic.Uint64
framesInvalid             atomic.Uint64
framesDropped             atomic.Uint64
bytesReceived             atomic.Uint64

# 단계별 invalid 세분화
invalidLengthMismatch     atomic.Uint64
invalidCRCMismatch        atomic.Uint64
invalidHeaderInvalid      atomic.Uint64
invalidPayloadPrefix      atomic.Uint64
invalidRegisterLength     atomic.Uint64

# 디코딩별 카운터
reg02ResponseCount        atomic.Uint64
reg03ResponseCount        atomic.Uint64
reg04ResponseCount        atomic.Uint64
reg04WriteCount           atomic.Uint64
ackCount                  atomic.Uint64
unconfirmedFieldObservations atomic.Uint64

# WRITE 중복 제거 (REQ-CENTURY-027)
writesDeduped             atomic.Uint64
```

### 4.4 출구 기준

- [ ] `go test -race ./internal/agent/century/...` 전체 패키지 통과
- [ ] CAP-3 시뮬레이션 트랜스포트 입력 → 한 사이클 동안 5종 이벤트 모두 ring buffer 에 저장 확인
- [ ] `get_stats` 응답에 모든 카운터 노출
- [ ] `auto_discovery=true` 로 `sub_dev_id=0x3B` 자동 등록 (B1a 시나리오)
- [ ] **다중 IDU 자동 발견**: 합성 frame 으로 `sub_dev_id=0x3C` 추가 주입 시 첫 관측 polling cycle 내에 새 CenturyDevice 생성, 기존 device 와 독립 상태 유지 (B1b 시나리오)
- [ ] `offline_timeout=200ms` 모의 환경에서 200ms 무통신 후 디바이스 오프라인 전이
- [ ] **WRITE 중복 제거 (REQ-CENTURY-027)**: dedupe_writes=true 기본값에서 동일 raw payload 의 두 번째 WRITE frame 이 decoded event 로 emit 되지 않음. raw frame 노드는 두 frame 모두 emit. cycle 경계 넘어가면 dedup 무효. dedupe_writes=false 시 둘 다 emit
- [ ] 패키지 커버리지 ≥80% (M5 에서 ≥85% 로 끌어올림)

---

## 5. M4: 플로우 노드 + Web UI (Secondary Goal)

**목표**: 4종 노드(`century-status`, `century-control`, `century`, `century-raw-frame`) 구현, registry 등록, web schema 추가.

### 5.1 파일

- `internal/node/century.go`:
  - 에러: `ErrCenturyMissingAgentRef`, `ErrCenturyNoResolver`, `ErrCenturyAgentNotCentury`, `ErrCenturyProcessFailed`
  - `CenturyNodeConfig` (LGCNP 와 동형: `agent_ref`, `poll_interval`, `timeout`, `poll_command`, `recent_count`, `batch_size`)
  - `centuryNodeBase` — `lgcnpNodeBase` 패턴 그대로 차용, 타입 체크만 `*century.CenturyAgent`
  - `CenturyStatusNode` — SourceNode. `FrameNotifyCh` 지원, 폴링 `get_recent`/`drain` 으로 디코딩된 status 이벤트 송출
  - `CenturyControlNode` — Process 가 **항상** `{"status":"not_supported", "reason":"century_passive_only", ...}` 반환. 에이전트 Process 미호출
  - `CenturyNode` — 통합. 제어 키 감지(`power`/`mode`/`temperature`/`setpoint`/`fan_speed`) 시 not_supported, 아니면 status 응답
  - `CenturyRawFrameNode` — SourceNode. 에이전트 ring buffer 또는 별도 raw 채널에서 raw frame + 메타데이터 송출
  - 모든 노드는 Init-tolerance 패턴 적용(SPEC-SERIAL-001 REQ-SERIAL-016 + LGCNP v1.3)
- `internal/node/century_test.go` — 4종 노드 모두 단위 테스트. mock CenturyAgent 사용
- `internal/node/registry.go` 수정 — 등록 테이블에 4 행 추가:
  ```
  {"century-status",    NewCenturyStatusNode,    "io",    "Century HVAC 디바이스 상태 (패시브 캡처)"},
  {"century-control",   NewCenturyControlNode,   "io",    "Century HVAC 제어 (미지원, 패시브 전용)"},
  {"century",           NewCenturyNode,          "io",    "Century HVAC 상태 + 제어 통합"},
  {"century-raw-frame", NewCenturyRawFrameNode,  "io",    "Century HVAC 원시 프레임 캡처"},
  ```
- `web/src/config/agentSchemas.ts` 수정:
  - `AGENT_TYPES` 에 `{value:'century-hvac', label:'Century HVAC (passive)'}`
  - `CENTURY_HVAC_FIELDS` 상수 정의 (REQ-CENTURY-022 의 모든 필드)
  - `AGENT_CONFIG_FIELDS` 에 `'century-hvac': CENTURY_HVAC_FIELDS` 매핑
- `web/src/config/nodeSchemas.ts` 수정 — 4 노드 스키마 + `agent_select` 옵션에 `century-hvac` 포함
- `web/src/config/__tests__/centurySchema.test.ts` (또는 동등) — 필드 가시성·기본값 검증

### 5.2 출구 기준

- [ ] `go test -race ./internal/node/...` 통과 (century 노드 포함)
- [ ] `cd web && npm test` 통과 (스키마 테스트 포함)
- [ ] Web UI 에서 century-hvac 에이전트 생성·삭제 정상 동작 (수동 smoke)
- [ ] 4 노드 모두 UI 에서 노드 추가 가능
- [ ] `century-control` 의 not_supported 응답이 디버그 노드로 전달되어 화면에 표시

---

## 6. M5: Polish & QA (Final Goal)

**목표**: 예시 YAML, README/SPEC 보강, 커버리지 ≥85%, 구조화 로그 안정화, `-race` clean.

### 6.1 작업 항목

- `examples/config/century-hvac-passive.yaml` — SPEC §4.5 의 예시 그대로 + 주석으로 패시브 전용임을 강조
- `docs/agents/century-hvac.md` (선택, 다른 에이전트 문서가 있으면 동일 위치) — 사용자용 운영 가이드:
  - 회선 tap 방법(반이중 RS-485 에 RX-only)
  - 미확정 필드 의미 발굴을 위한 캡처 시나리오 가이드(프로토콜 문서 §8.3 참조)
  - `century-raw-frame` 노드로 새 캡처 수집 → 후속 SPEC 개정 사이클
- 구조화 로그 audit:
  - INFO: 에이전트 시작/정지, 디바이스 자동 발견, 디바이스 온/오프라인
  - WARN: CRC 불일치 (`log_decode_errors=true`), 드롭 (`log_drops=true`)
  - DEBUG: 미확정 필드 새 관측값 (`log_unconfirmed_fields=true`)
  - 운영 환경 기본값(`false` 다수)에서 저널 범람이 없는지 확인
- 커버리지 측정 — `go test -race -cover ./internal/agent/century/... ./internal/node/...`
  - 부족한 분기에 대해 테이블 테스트 추가
  - 목표: 패키지별 ≥85%
- `go vet ./...` 경고 0
- `golangci-lint run` (프로젝트 lint 설정 따름) 경고 0
- SPEC 자신 수정 — 구현 중 발견된 사항이 있으면 spec.md 의 변경 이력에 v0.1.1 로 추가

### 6.2 출구 기준 (전체 SPEC 의 Definition of Done)

- [ ] M1 ~ M5 의 모든 출구 기준 충족
- [ ] `go test -race ./...` clean (회귀 영향 없음)
- [ ] 패키지 커버리지: `internal/agent/century` ≥85%, `internal/node` century 관련 함수 ≥85%
- [ ] 6 종 acceptance scenario 그룹(A~F) 모두 통과 (acceptance.md 참조 — F 는 WRITE 중복 처리)
- [ ] CAP-1, CAP-3, CAP-4 골든 픽스처가 모두 정확히 디코딩
- [ ] `xflowd` 데몬에 century-hvac 에이전트를 추가한 yaml 로 부팅 → smoke 캡처 → 디코딩 결과가 dashboard 에 표시

---

## 6.5 M6: TCP Transport (Primary Goal, v0.2.0)

**목표**: tcp-client + tcp-server transport 추가. lgcnp TCP 패턴 (lgapTCPClientTransport `net.DialTimeout`, lgapTCPServerTransport `net.Listen` + Accept loop) 을 차용하되 패시브 캡처(AC-B9 transport.Write 0회 불변식) 정책에 맞게 RX-only 로 적용. serial 회귀 완전 보존.

### 6.5.1 Deliverables

- `internal/agent/century/transport_serial.go` — **refactor**: M3 의 inline serial transport 를 별도 파일로 분리. 기존 동작 변경 없음.
- `internal/agent/century/transport_tcp.go` — **NEW**: tcp-client + tcp-server 구현
  - `tcpClientTransport` 구조체: `net.DialTimeout`, `SetReadDeadline`, RX-only `io.ReadCloser` wrapper
  - `tcpServerTransport` 구조체: `net.Listen`, single-active-connection Accept loop, RX-only
  - `reconnectWithBackoff(ctx, transport)` 헬퍼: exponential backoff (initial 5s, max 5min), context-aware sleep
  - Write 메서드 의도적으로 미구현 또는 `ErrTransportPassiveOnly` 반환 (AC-G8 보장)
- `internal/agent/century/transport_tcp_test.go` — **NEW**: 표준 `net.Listen` + `net.Dial` mock 으로 table-driven 검증
  - dial 성공/실패, EOF, read timeout, listen + accept, 두 번째 연결 거부, backoff doubling, context cancel
- `internal/agent/century/errors.go` 확장 — 신규 sentinel:
  - `ErrUnknownTransportType`
  - `ErrCenturyTCPPortRequired`
  - `ErrCenturyTCPHostRequired`
  - `ErrCenturyTCPDialFailed` (wrapping target)
  - `ErrTransportPassiveOnly` (TCP wrapper 가 Write 호출 거부)
- `internal/agent/century/config.go` 확장 — 신규 필드:
  - `TCPHost string`
  - `TCPPort int`
  - `TCPConnectTimeout time.Duration` (default 5s)
  - `TCPReadTimeout time.Duration` (default 3s)
  - `ReconnectInitial time.Duration` (default 5s)
  - `MaxReconnectBackoff time.Duration` (default 5min)
  - `CycleIdleTimeout time.Duration` (default transport-aware: serial=100ms, tcp-*=200ms)
  - `parseCenturyConfig` 의 validation 분기 확장: transport_type 별 필수 필드 검증
- `internal/agent/century/agent.go` 갱신:
  - `transportProvider` factory 가 `transport_type` 에 따라 분기 (serial / tcp-client / tcp-server)
  - tcp-client 경우 captureLoop 가 `reconnectWithBackoff` wrapper 를 통해 io.EOF / net.OpError / read timeout 발생 시 재연결
  - tcp-server 경우 accept loop + 단일 활성 연결 정책
  - context.Cancel 전파로 dial 중단 / backoff 중단 / accept loop 종료
- `internal/agent/century/cycle_tracker.go` 갱신 — `CycleIdleTimeout` 을 config 에서 주입받아 사용 (기존 hardcoded 100ms 제거, transport-aware default 적용)
- `examples/agents/century-hvac-tcp-client.yaml` — **NEW**
- `examples/agents/century-hvac-tcp-server.yaml` — **NEW**
- `web/src/config/agentSchemas.ts` 갱신:
  - `CENTURY_HVAC_FIELDS` 의 `transport_type` 옵션에 `tcp-client`/`tcp-server` 추가
  - `tcp_host`, `tcp_port`, `tcp_connect_timeout`, `tcp_read_timeout`, `reconnect_initial`, `max_reconnect_backoff`, `cycle_idle_timeout` 필드 추가
  - `visibleWhen` 으로 transport-conditional 표시 (serial 필드는 `transport_type==serial` 일 때만, tcp 필드는 `transport_type in [tcp-client, tcp-server]` 일 때만)
- `web/src/pages/agents/agentTypeMeta.ts` 갱신:
  - century-hvac 의 configFields 확장
  - configExample 에 TCP-client 예시 추가

### 6.5.2 Test scope

- **Group G acceptance scenarios** (acceptance.md 참조): AC-G1 ~ AC-G8 — TCP 동작, dial failure backoff, reconnect, read timeout, server accept, secondary connection rejection, transport-aware cycle_idle_timeout default, AC-B9 invariant under TCP
- **회귀**: serial 동작 완전 보존
  - `go test -race -count=3 ./internal/agent/century/... ./internal/node/...` 통과 — 기존 41 시나리오(그룹 A~F) 모두 PASS
  - 골든 픽스처(CAP-1/3/4) 디코딩 결과 변화 없음
  - serial 의 `cycle_idle_timeout` 기본값 100ms 유지 확인
- **Coverage 목표**: `transport_tcp.go` ≥85%, 전체 `internal/agent/century` 패키지 ≥87% 유지 (v0.1.2 의 88.9% 에서 큰 후퇴 없음)
- **fake/mock transport 패턴**: `net.Pipe()` 또는 `net.Listen("tcp", "127.0.0.1:0")` 로 실제 OS 소켓 사용 (lgcnp 테스트 패턴 참조)
- **TCP Write 0회 검증 (AC-G8)**: wrapper 의 Write 메서드 호출 카운터를 mock 으로 추가, 모든 TCP 시나리오에서 0 임을 단언

### 6.5.3 Exit criteria

- [ ] `go test -race -count=1 ./internal/agent/century/... ./internal/node/...` PASS
- [ ] `go vet ./internal/agent/century/...` clean
- [ ] `golangci-lint run ./internal/agent/century/...` clean
- [ ] REQ-CENTURY-028 ~ REQ-CENTURY-032 모두 Implemented (spec.md §4.6 추적성 표 Planned → Implemented)
- [ ] AC-G1 ~ AC-G8 모두 PASS (acceptance.md 의 Verification Results 부록 갱신)
- [ ] AC-B9 transport.Write 0회 불변식이 TCP 모드(tcp-client + tcp-server) 에서도 유지됨을 mock 으로 검증
- [ ] 예제 YAML 2종(`century-hvac-tcp-client.yaml`, `century-hvac-tcp-server.yaml`) 이 `agent.AgentConfigFromYAML` + `century.NewCenturyAgent` 로 round-trip 검증
- [ ] Web UI 에서 transport_type 변경 시 적절한 필드가 visible / hidden 되는 smoke 테스트 통과
- [ ] v0.1.2 의 모든 출구 기준 회귀 영향 없이 유지 (특히 AC-A~F 41 시나리오)

### 6.5.4 구현 순서

```
M6.1  transport_serial.go refactor (기존 inline 코드 분리, 동작 변경 없음)
      └→ go test -race ./internal/agent/century/... 회귀 확인
M6.2  errors.go 확장 + config.go 신규 필드 + parseCenturyConfig 분기
      └→ config_test.go 테이블 테스트 (transport-type 별 valid/invalid 케이스)
M6.3  transport_tcp.go: tcpClientTransport 구현 (Open/Close/Read, no Write)
      └→ transport_tcp_test.go: dial 성공/실패, EOF, read timeout, context cancel
M6.4  reconnectWithBackoff 구현 + agent.go captureLoop 통합
      └→ AC-G2, AC-G3 시나리오 검증
M6.5  transport_tcp.go: tcpServerTransport 구현 (Listen/Accept loop, single-active)
      └→ AC-G5, AC-G6 시나리오 검증
M6.6  cycle_tracker.go: CycleIdleTimeout 주입화 (transport-aware default)
      └→ AC-G7 시나리오 검증
M6.7  예시 YAML 2종 + web schema 확장
      └→ round-trip 검증 + UI smoke 테스트
M6.8  AC-B9 invariant 검증 (mock 으로 TCP wrapper 의 Write 0회)
      └→ AC-G8 시나리오 검증
M6.9  커버리지 측정 + 회귀 통과 확인
      └→ Exit criteria 모두 충족 → Implemented 전이
```

---

## 7. 리스크 및 대응

| ID | 리스크 | 영향 | 대응 |
|----|--------|------|------|
| R1 | 미확정 필드 의미가 후속 캡처로 달라짐 | 페이로드 스키마 호환 깨짐 | `confirmation_status` 마커 + 캡처 위치 기반 명명(`reg02_byte_3`, `write_live_15`). 의미 확정 시 alias 필드를 추가하고 기존명 deprecated 유지 (REQ-CENTURY-026) |
| R2 | RS-485 회선 노이즈로 프레임 경계 손실 | `framesInvalid` 증가, 일부 이벤트 누락 | (a) 헤더 검증 후 1바이트 shift 재동기화, (b) CRC 검증으로 잘못된 경계 거부, (c) 통계 모니터링으로 회선 품질 감시 |
| R3 (revised) | 다중 IDU 코드 경로는 실제 다중 유닛 캡처가 없는 상태로 구현됨 | 다중 IDU 환경에서 회귀 위험 | (a) 단위 테스트로 sub_dev_id 다중 매핑 검증 (B1a/B1b), (b) 합성 frame 으로 sub_dev_id 격리 검증, (c) 실제 다중 IDU 환경에서는 점진적 roll-out (log_decode_errors=true 부터) 권장. 실제 캡처 확보 시 후속 SPEC 으로 ground truth acceptance 추가 |
| R4 | 모드 코드 `0x02` 이상 등장 | 디코더 미인식 | additive enum: `mode_unknown_<hex>` + `unknown` 마커. 후속 SPEC 으로 추가 (REQ-CENTURY-026) |
| R5 | 보레이트 미상 | 초기 캡처 실패 | 사용자 설정으로 노출. 운영 가이드에 보레이트 측정 방법(로직 애널라이저) 안내 |
| R6 | `payload_length` 가 합리적 범위를 벗어나는 비표준 프레임 | 메모리 폭발 가능성 | `MaxPayloadLength=256` 상한, 초과 시 1바이트 shift 재동기화 |
| R7 (NEW) | WRITE dedupe 의 cycle 경계 감지가 잘못되면 정상 신규 명령이 누락될 수 있음 | 마스터의 새 명령이 dedup 으로 무시되어 downstream 에 보이지 않음 | (a) cycle 경계 감지 로직 단위 테스트 (1차 신호: reg 0x04 응답 마커, 2차 신호: 100ms idle), (b) `log_drops=true` 시 dedup 된 frame 도 DEBUG 로깅하여 운영자가 진단 가능, (c) `dedupe_writes=false` fallback 옵션 노출, (d) `writesDeduped` 카운터로 정상 비율 모니터링 |
| R8 | LGCNP 와 동시에 같은 회선에 부착 | 잘못된 디코딩 시도 | 별도 에이전트 인스턴스로 분리 운용. 같은 시리얼 포트는 OS 수준에서 다중 오픈 차단 (SPEC-SERIAL-001 A2) |
| R9 | 송신 금지 정책 위반 (실수로 Write 호출) | RS-485 회선 충돌, 외부 컨트롤러와 마스터 권한 분쟁 | (a) `CenturyAgent.Process()` 의 Write 경로 부재를 코드 리뷰 시 명시 확인, (b) 트랜스포트를 `io.Reader` 래핑으로 노출하여 Write 메서드 자체를 가리는 옵션 검토 |
| R10 (NEW, v0.2.0) | TCP-server 단일 활성 연결 정책이 다중 컨버터 환경에서 제약 | 두 번째 이상 클라이언트가 즉시 거부되어 운영자가 단일 컨버터만 연결할 수 있음 | (a) v0.2.0 의 명시적 단일 연결 정책 (A11) 로 사용자에게 사전 고지, (b) INFO 로그로 두 번째 연결 거부 가시화, (c) 다중 컨버터 환경에서는 컨버터별 별도 century-hvac 에이전트 인스턴스를 다른 tcp_port 로 운영 권장, (d) v0.3.0 에서 다중 동시 연결 지원 검토 |
| R11 (NEW, v0.2.0) | `cycle_idle_timeout` 기본값이 transport-aware 로 변경되어 동작 변화 | serial 사용자는 영향 없음 (여전히 100ms). TCP 사용자에게는 default 200ms 적용 — 기존 v0.1.2 운영자 중 TCP 모드 시도 시 의도와 다른 default 가 적용될 수 있음 | (a) v0.1.2 는 TCP 미지원이었으므로 serial 사용자 회귀 없음, (b) 명시 설정 시 transport 와 무관하게 그 값 사용 (REQ-CENTURY-032), (c) Web UI 에서 default 가 transport 의존이라는 점을 hint 로 노출, (d) 운영자가 `writesDeduped` 카운터로 false dedup 정황을 모니터링 가능 |
| R12 (NEW, v0.2.0) | Exponential backoff max 가 5min 으로, 장애 동안 device offline 상태 5min 까지 지연 | 마스터/컨버터 장애 시 dashboard 의 device 상태 회복이 최대 5min 지연 | (a) `max_reconnect_backoff` 를 사용자가 환경에 맞게 단축 가능 (예: 30s), (b) `offline_timeout` 과 별개로 last_seen 을 통해 stale 표시로 운영자가 인지 가능, (c) monitoring (slog WARN 또는 Prometheus exporter v0.3.0) 으로 backoff 누적 상태 가시화, (d) 재연결 시 즉시 backoff 리셋되어 회복 후에는 정상 응답성 유지 |

---

## 8. 의존성

### 내부 패키지

- `internal/agent` — Agent, AgentConfig, AgentStats, DefaultManager, FrameNotifier, BufferInfo
- `internal/agent/serial` (선택) — 시리얼 트랜스포트 재사용. RX-only 사용
- `internal/device` — DeviceProvider 인터페이스, Device 모델
- `internal/node` — Node, SourceNode, BaseNode, NodeOption, AgentResolver, MultiSourceNode (선택)
- `pkg/lifecycle` — BaseLifecycle, 상태 전이
- `pkg/flow` — NodeDef, AgentRef
- `pkg/message` — Message 인터페이스, Payload

### 외부 의존성

- 추가 외부 의존성 없음
- `go.bug.st/serial` 은 SPEC-SERIAL-001 의 트랜스포트 계층을 통해 간접 사용

### 관련 SPEC

- **상위**: SPEC-AGENT-001 (에이전트 프레임워크), SPEC-AGENT-005 (deferred connection)
- **트랜스포트**: SPEC-SERIAL-001 (시리얼 트랜스포트)
- **패턴 참조**: SPEC-LGCNP-001 (패시브 캡처 + 다층 검증 + 노드 구성), SPEC-NASA-001 (파일 레이아웃 + 등록)
- **엔진**: SPEC-ENGINE-001 (`ReinitNodesForAgent` deferred connection 경로)
- **노드**: SPEC-NODE-002 (Framer 노드 — 직접 의존은 없으나, 노드 패턴 참조)

---

## 9. 구현 순서 요약

```
M1 (foundation)
  crc.go ─→ crc_test.go (TDD)
  frame_scanner.go ─→ frame_scanner_test.go (TDD)
  errors.go, common.go, frame.go
   │
   ↓
M2 (decoders)
  message.go ─→ ConfirmationStatus enum
  decoder_reg02.go / _reg03.go / _reg04.go / _ack.go ─→ 각 _test.go (TDD with CAP-1/3/4 fixtures)
  register.go (dispatch)
   │
   ↓
M3 (agent)
  config.go ─→ config_test.go
  ring_buffer.go ─→ ring_buffer_test.go
  device.go, provider.go
  agent.go ─→ agent_test.go
  registration.go ─→ cmd/xflowd/main.go 수정
   │
   ↓
M4 (nodes + web)
  internal/node/century.go (4 nodes) ─→ century_test.go
  internal/node/registry.go 수정
  web/src/config/agentSchemas.ts 수정
  web/src/config/nodeSchemas.ts 수정
   │
   ↓
M5 (polish)
  examples/config/century-hvac-passive.yaml
  (선택) docs/agents/century-hvac.md
  커버리지 보강, 로그 audit, go vet / lint clean
```

각 마일스톤 완료 시 `go test -race ./...` 로 회귀 확인. M3 완료 후 `xflowd` 부팅 smoke 테스트, M4 완료 후 web UI smoke 테스트.

---

## 10. 구현 완료 (Implementation Complete)

**SPEC v0.1.2 — Implemented (2026-05-18)**

### Commit Chain

| 단계 | Commit | 내용 |
|------|--------|------|
| Spec | `14ee853` | SPEC-CENTURY-001 v0.1.1 작성 (spec.md / plan.md / acceptance.md) |
| M1 | `bfdfaf0` | Foundation — CRC-16/ARC, frame scanner, errors, common, frame parser |
| M2 | `d33da37` | Decoders — reg 0x02/0x03/0x04 응답 + reg 0x04 write + ACK + dispatch |
| M3 | `bad2e06` | Agent + ring buffer + 디바이스 관리(다중 IDU) + WRITE dedupe + 타입 등록 + cmd/xflowd 통합 |
| M4 | `3f1b970` | 플로우 노드 4종 (status/control/combined/raw-frame) + Web UI 스키마 |
| M5 | [completed] | examples YAML 2종 + SPEC v0.1.2 갱신 + closure notes |
| M6 (예정, v0.2.0) | - | TCP transport — tcp-client + tcp-server + exponential backoff 재연결 + transport-aware cycle_idle_timeout default + 회귀 보장 |

### Final Metrics

- **Coverage**: `internal/agent/century` **88.9%** (목표 85% 통과), `internal/node/century.go` **평균 87.4%** (44 함수, 목표 85% 통과)
- **Tests**: `go test -race -count=3 ./internal/agent/century/... ./internal/node/...` 통과 (flake 없음)
- **Build**: `go build ./...` clean, `go vet ./internal/agent/century/... ./internal/node/...` clean
- **AC 시나리오**: 그룹 A~F 총 41 시나리오 모두 자동 테스트로 커버 (스펙 §4.6 참조)
- **REQ 구현**: 27 REQ-CENTURY-XXX 모두 Implemented (스펙 §4.6 참조)
- **Examples**: `examples/agents/century-hvac.yaml`, `examples/flows/century-status-flow.yaml` (project's own loaders 로 round-trip 검증 완료)
- **Binary**: `cmd/xflowd` 51 MB 빌드 성공, `RegisterCenturyTypes` 호출 포함 확인

### Known Limitations / v0.2.0 Deferrals

SPEC §5.9 "M5 Closure Notes" 참조. 핵심 항목:

1. CRC-failed raw frame 의 원시 노출 (현재 raw frame 노드는 검증된 ring buffer 만 사용)
2. 능동 폴링 / 송신 모드 (SPEC-CENTURY-002 로 분리 예정)
3. 실제 다중 IDU ground truth 캡처 확보 시 acceptance 보강
4. 미확정 필드 (`status_bits` 비트 매핑, `op_val_1/2` 단위) 의미 발굴
5. 모드 코드 `0x02` 이상 (난방/제습/송풍) additive enum 확장
6. Metrics export (Prometheus / OpenTelemetry) 직접 연계

### Smoke 검증 결과

- `xflow agent import -f examples/agents/century-hvac.yaml` 형식의 YAML 이 `internal/agent.AgentConfigFromYAML()` + `century.NewCenturyAgent()` 로 round-trip 성공 (id=century-living-room, transport.options=18 keys 정상 파싱)
- `xflow flow import -f examples/flows/century-status-flow.yaml` 형식의 flow YAML 이 `pkg/flow.LoadFlowFromFile()` 로 파싱 성공 (nodes=8, wires=8)

---

*Plan 버전: 0.2.0 (Draft, M6 진행 예정)*
*작성일: 2026-05-18 (v0.1.0), 갱신: 2026-05-18 (v0.1.1 — 다중 IDU + WRITE dedupe), 2026-05-18 (v0.1.2 — M1-M5 구현 완료), 2026-05-18 (v0.2.0 — M6 TCP transport 신설)*
*작성자: xtra*
