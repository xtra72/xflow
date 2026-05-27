# SPEC-LG-HVACR-001: 구현 계획

> **SPEC ID**: SPEC-LG-HVACR-001
> **개발 방법론**: Hybrid (TDD for new code, DDD for modifications)
> **상태**: Implemented

> **명명 규약**: 본 문서는 v1.0 rename (2026-05-27) 기준으로 작성된다.
> - 프로토콜 코드 식별자: `lg_icp01` (표시명 "LG ICP-01")
> - 에이전트 타입 식별자: `lg_hvacr01` (표시명 "LG HVACR-01")
> - 노드 타입 식별자: `lg_hvacr01`, `lg_hvacr01_status`, `lg_hvacr01_control`
> 구현 코드는 `internal/agent/lg/lg_hvacr01_*.go` (에이전트) + `internal/agent/lg/lg_icp01_*.go` (프로토콜) + `internal/node/lg_hvacr01.go` (노드).

---

## 1. 마일스톤 개요

| 마일스톤 | 내용 | 우선순위 | 의존성 |
|---------|------|---------|-------|
| M1 | 프레임 파서 | Primary Goal | 없음 |
| M2 | LG HVACR-01 에이전트 | Primary Goal | M1 |
| M3 | 디바이스 관리 | Primary Goal | M2 |
| M4 | 플로우 노드 | Secondary Goal | M2, M3 |
| M5 | Web UI 스키마 | Secondary Goal | M4 |
| M6 | 타입 등록 | Final Goal | M2, M4 |

---

## 2. M1: 프레임 파서 (Primary Goal)

### 2.1 파일: `internal/agent/lg/lg_icp01_frame.go`

**목표**: TYPE-A(20B)와 TYPE-B(40B) 두 유형의 프레임을 스트림에서 추출하고 검증하는 파서 구현.

**기술 접근**:

1. **스트림 파서 구조체** (`Icp01FrameParser`)
   - `io.Reader`에서 바이트를 읽어 프레임 경계를 탐지
   - STX 바이트 패턴 기반 분류:
     - `0x58` -> TYPE-A (다음 19바이트 읽기, 총 20바이트)
     - `0x81~0x85` -> TYPE-B (다음 39바이트 읽기, 총 40바이트)
   - 그 외 바이트는 스킵 (동기화 복구)

2. **프레임 구조체**
   - `Icp01ODUFrame`: TYPE-A 프레임 (Raw, SEQ, 체크섬 유효성, 파싱된 필드)
   - `Icp01IDUFrame`: TYPE-B 프레임 (Raw, IDU 주소/번호, 이중 기록 유효성, 온도값)
   - 공통 `Icp01Frame` 인터페이스 또는 래퍼로 통합 가능

3. **체크섬 검증 함수**
   - `VerifyODUChecksum(buf []byte) bool`: SEQ별 분기 (01/05=XOR, 04=SUM, 02/03=패스)
   - 프로토콜 분석 보고서 섹션 9의 Go 코드 참조

4. **이중 기록 검증 함수**
   - `VerifyIDURedundancy(buf []byte) bool`: `b[9]==b[29]` && `b[23]==b[36]`

5. **고정 바이트 구조 검증**
   - CMD 범위: `pkt[1]` in {0x02, 0x43}
   - IDU_INDEX: `pkt[20] == (pkt[0] - 0x81 + 1)`

6. **온도 디코딩 함수**
   - `DecodeIDUTemperatures(buf []byte) (set, room, inlet, outlet float32)`
   - `DecodeODUTemperatures(buf []byte) (tempA, tempB float32)` (SEQ=02 전용)

7. **물리 범위 검증**
   - 설정온도 16~30도C, 실내 0~50도C, 흡입/토출 0~70도C

**LGCP와의 차이점 (주의)**:
- LGCP는 STX+LEN 기반 가변 길이 -> LG ICP-01 은 STX 패턴 기반 고정 길이
- LGCP는 CRC-16 -> LG ICP-01 은 CRC 없음 (이중 기록 + 체크섬 혼합)
- 프레임 파서가 근본적으로 다른 구조이므로 LGCP 코드 복사가 아닌 신규 작성 필요

**테스트 전략 (TDD)**:
- 프로토콜 분석 보고서의 실제 캡처 데이터를 테스트 벡터로 사용
- TYPE-A SEQ=01 XOR 검증: `58 01 00 00 00 00 00 00 00 57 66 00 00 00 2e 00 19 01 43 1d`
- TYPE-B IDU#1 이중 기록 검증: b[09]=b[29]=0x52, b[23]=b[36]=0x6d
- 경계 조건: 불완전 프레임, 동기화 손실, 잘못된 STX

### 2.2 파일: `internal/agent/lg/lg_icp01_frame_test.go`

- Table-driven 테스트: 정상/비정상 프레임 벡터
- 체크섬 검증 테스트 (SEQ별)
- 이중 기록 불일치 테스트
- 온도 변환 정확도 테스트
- 물리 범위 초과 테스트
- 스트림 파서 동기화 복구 테스트

---

## 3. M2: LG HVACR-01 에이전트 (Primary Goal)

### 3.1 파일: `internal/agent/lg/lg_hvacr01_config.go`

**목표**: `Hvacr01Config` 구조체와 `parseHvacr01Config` 파싱 함수.

**주요 설정 필드**:
- `SerialPort` (필수)
- `BaudRate` (기본: **1200** -- LGCP의 9600과 다름)
- `DataBits` (기본: 8), `StopBits` (기본: 1), `Parity` (기본: "none")
- `TransportType` ("serial", "tcp-client", "tcp-server")
- `ReadTimeout` (기본: 500ms)
- `MsgChannelSize` (기본: 256)
- `ReconnectInterval`, `MaxReconnectBackoff`
- `AutoDiscovery` (기본: true)
- `OfflineTimeout` (기본: 30s)
- `NotifyInterval` (기본: 0 = 변경 시에만)
- `ControlEnabled` (기본: **false** -- 쓰기 명령 미확정)

**LGCP 패턴 참조**: `lgcp_config.go`의 `parseLGCPConfig`와 동일 구조, 보레이트 기본값만 1200으로 변경. `VerifyCRC` 대신 `VerifyRedundancy` (기본: true).

### 3.2 파일: `internal/agent/lg/lg_hvacr01_agent.go`

**목표**: 패시브 캡처 에이전트. LGCP 에이전트의 구조를 따르되, 프레임 파서와 디바이스 모델을 LG ICP-01 / LG HVACR-01 용으로 교체.

**구현 항목**:

1. **`Hvacr01Agent` 구조체**
   - `lifecycle.BaseLifecycle` 임베딩
   - `Hvacr01Config`, `LGAPTransport` (1200 bps)
   - 캡처 통계 (atomic): `framesCaptured`, `framesValid`, `framesInvalid`, `framesDropped`, `bytesReceived`
   - `oduFramesCaptured`, `iduFramesCaptured` (유형별 카운터)
   - 링 버퍼 (`recentFrames []icp01FrameRecord`)
   - 디바이스 맵 (`devices map[string]*Icp01Device`)
   - SEQ 추적 (`lastODUSeq int` -- 사이클 연속성 검증용)
   - 이전 온도값 (`prevTemps map[string]icp01TempSnapshot` -- 변화율 검증용)

2. **인터페이스 구현**
   - `agent.Agent`: Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
   - `agent.MessageReceiver`: ReceiveMessage
   - `agent.StatefulAgent`: State
   - `agent.BufferInfoProvider`: BufferInfo, FrameNotifyCh
   - `agent.TransportChecker`: TransportConnected

3. **캡처 루프** (`captureLoop`)
   - `Icp01FrameParser`로 프레임 읽기
   - TYPE-A: ODU 이벤트 생성, SEQ 순서 추적
   - TYPE-B: IDU 이벤트 생성, 온도 변환, 디바이스 상태 갱신
   - 링 버퍼 저장 + 프레임 알림 채널 신호

4. **Process 커맨드**
   - `get_stats`: 캡처 통계 (ODU/IDU별 카운터 포함)
   - `get_recent`: 최근 프레임 조회 (lastSeq 필터)
   - `drain`: 프레임 소비

5. **재연결 루프** (`reconnectLoop`)
   - LGCP와 동일한 지수 백오프 패턴

6. **변화율 검증** (선택 사항, 6계층 중 계층 5)
   - IDU별 이전 사이클 온도 저장
   - 현재값과 비교하여 2.0도C 초과 시 경고

### 3.3 파일: `internal/agent/lg/lg_hvacr01_agent_test.go`

- 에이전트 라이프사이클 테스트
- Process 커맨드 테스트
- 목(mock) 트랜스포트를 사용한 캡처 루프 테스트

---

## 4. M3: 디바이스 관리 (Primary Goal)

### 4.1 파일: `internal/agent/lg/lg_icp01_device.go`

**목표**: LG ICP-01 디바이스 모델과 상태 추적.

**구조체**:

1. **`Icp01Device`**
   - `Address string` (ODU: "odu", IDU: "81"~"85")
   - `Label string` (예: "outdoor", "indoor-1")
   - `Type string` ("outdoor" 또는 "indoor")
   - `Online bool`
   - `LastSeen time.Time`
   - `Source string` ("auto" 또는 "config")
   - `State *Icp01DeviceState`

2. **`Icp01DeviceState`** (IDU용)
   - `SetTemp *float64` (설정온도)
   - `RoomTemp *float64` (실내온도)
   - `InletTemp *float64` (흡입온도)
   - `OutletTemp *float64` (토출온도)
   - `OpMode *int` (운전 모드 바이트)
   - `StatusFlags *int` (상태 플래그)
   - `CMDCycle *string` ("A" 또는 "B")
   - `DevType *int`
   - `DeviceID *int`

3. **`Icp01ODUState`** (ODU용)
   - `OutdoorTempA *float64`
   - `OutdoorTempB *float64`
   - `CompressorFlag *int` (FLAG_A)

4. **`toProperties()`** -- NASA/LGCP 통일 속성명 사용
   - IDU: `power` (bit5), `mode` (통일 ID→문자열), `fan_speed` (통일 ID→문자열), `target_temp`, `current_temp`, `inlet_temp`, `outlet_temp`
   - ODU: `outdoor_temp`, `comp_suction_temp`, `comp_discharge_temp`, `condenser_temp_a`, `condenser_temp_b`, `avg_temp`

**DeviceProvider 구현**:
- `Hvacr01DeviceProvider` 어댑터 (LGCP의 `LGCPDeviceProvider` 패턴 참조)

---

## 5. M4: 플로우 노드 (Secondary Goal)

### 5.1 파일: `internal/node/lg_hvacr01.go`

**목표**: LGCP 노드(`internal/node/lgcp.go`)와 동일한 구조로 3개 노드 타입 구현.

**공통 기반**: `hvacr01NodeBase` (LGCP의 `lgcpNodeBase` 패턴)
- `AgentRef` 기반 에이전트 resolve
- `initAgent`에서 `*lg.Hvacr01Agent` 타입 확인
- `callAgentProcess` 타임아웃 래퍼

**노드 타입**:

1. **`Hvacr01StatusNode`** (SourceNode)
   - 폴링 기반 프레임 수신
   - `FrameNotifyCh` 지원 (에이전트 알림 즉시 반응)
   - `pollRecentBulk`: get_recent/drain 벌크 수신
   - `sourceCh` 채널로 개별 메시지 출력

2. **`Hvacr01ControlNode`** (플레이스홀더)
   - Process에서 항상 `{"status": "not_supported", "message": "LG ICP-01 control commands not yet discovered"}` 반환
   - `control_enabled` 설정이 true여도 실제 명령 전송 없음

3. **`Hvacr01Node`** (통합)
   - SourceNode: 폴링으로 상태 수신
   - Process: 제어 키 감지 시 미지원 응답, 그 외 상태 조회

**LGCP 노드와의 차이**:
- 에이전트 타입 체크가 `*lg.Hvacr01Agent`
- 제어 명령 미지원 (비활성)
- 메타데이터 키가 `node_source`, `node_id` (v1.11.0 부터 prefix-less)

---

## 6. M5: Web UI 스키마 (Secondary Goal)

### 6.1 파일: `web/src/config/agentSchemas.ts` (수정)

**추가 항목**:
- `AGENT_TYPES` 배열에 `{ value: 'lg_hvacr01', label: 'LG HVACR-01' }` 추가
- `LG_HVACR01_FIELDS` 상수 정의:
  - serial_port (필수)
  - baud_rate (기본: 1200)
  - transport_type (serial/tcp-client/tcp-server)
  - auto_discovery (기본: true)
  - offline_timeout (기본: "30s")
  - notify_interval (기본: "0s")
  - devices (선택: 고정 설치 디바이스 목록)
- `AGENT_CONFIG_FIELDS`에 `'lg_hvacr01': LG_HVACR01_FIELDS` 매핑

### 6.2 파일: `web/src/config/nodeSchemas.ts` (수정)

**추가 항목**:
- `lg_hvacr01_status`: agent_ref(agent_select, options: ['lg_hvacr01']), poll_interval, timeout, poll_command, recent_count, batch_size
- `lg_hvacr01_control`: agent_ref(agent_select, options: ['lg_hvacr01']), timeout -- 비활성 안내 문구
- `lg_hvacr01`: agent_ref, poll_interval, timeout, poll_command, recent_count, batch_size

---

## 7. M6: 타입 등록 (Final Goal)

### 7.1 파일: `internal/agent/lg/lg_hvacr01_register.go` (신규)

```go
func RegisterHvacr01Types(mgr *agent.DefaultManager) error {
    return mgr.RegisterType("lg_hvacr01", func(config agent.AgentConfig) (agent.Agent, error) {
        return NewHvacr01Agent(config)
    })
}
```

### 7.2 파일: `internal/node/registry.go` (수정)

노드 등록 테이블에 추가:
```go
{"lg_hvacr01_status", NewHvacr01StatusNode, "io", "LG HVACR-01 디바이스 상태 조회"},
{"lg_hvacr01_control", NewHvacr01ControlNode, "io", "LG HVACR-01 디바이스 제어 (미지원)"},
{"lg_hvacr01", NewHvacr01Node, "io", "LG HVACR-01 상태 조회 + 제어 통합"},
```

### 7.3 에이전트 매니저 초기화 (수정)

에이전트 매니저가 `RegisterHvacr01Types`를 호출하도록 초기화 코드에 추가.

---

## 8. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|-------|------|------|
| 1200 bps 저속으로 인한 타임아웃 | 프레임 수신 지연 | ReadTimeout을 충분히 길게 설정 (1s 이상 권장) |
| TYPE-A/TYPE-B 동기화 손실 | 잘못된 프레임 경계 | 유효하지 않은 STX 바이트 스킵, 다음 유효 STX까지 스캔 |
| 이중 기록 불일치 빈도 | 유효 프레임 손실 | 통계 모니터링, 불일치율이 높으면 물리 계층 점검 알림 |
| IDU 6대 이상 구성 | 0x86+ 주소 미확인 | 확장 가능하도록 IDU_MAX를 설정으로 분리 |
| 제어 명령 미확정 | 향후 확장 필요 | control_enabled 플래그로 플레이스홀더 유지 |
| ODU SEQ=02/03 b[18:20] 미확정 | 일부 센서값 의미 불명 | 원시 바이트를 이벤트에 포함, 향후 분석 가능 |

---

## 9. 의존성

### 내부 패키지 의존성

- `internal/agent`: Agent, AgentConfig, AgentStats, DefaultManager, FrameNotifier
- `internal/agent/lg`: LGAPTransport (기존 시리얼 트랜스포트 재사용)
- `internal/device`: DeviceProvider 인터페이스
- `internal/node`: Node, SourceNode, BaseNode, NodeOption, AgentResolver
- `pkg/lifecycle`: BaseLifecycle, 상태 전이
- `pkg/flow`: NodeDef, AgentRef
- `pkg/message`: Message 인터페이스

### 외부 의존성

- 추가 외부 의존성 없음 (기존 프로젝트 의존성으로 충분)

---

## 10. 구현 순서 요약

```
M1 (프레임 파서)
 |
 v
M2 (에이전트) ---> M3 (디바이스 관리)
 |                    |
 v                    v
M4 (플로우 노드) <---+
 |
 v
M5 (Web UI 스키마)
 |
 v
M6 (타입 등록 + 초기화 연결)
```

각 마일스톤 완료 후 `go test -race ./...` 실행으로 회귀 확인.
