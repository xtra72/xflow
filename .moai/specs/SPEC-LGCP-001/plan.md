# SPEC-LGCP-001: Implementation Plan

**SPEC Reference**: SPEC-LGCP-001
**Version**: 1.0.0
**Created**: 2026-03-24

## 1. Technical Approach

### 1.1 Core Strategy

프레임 파서를 먼저 구현하여 프로토콜 데이터의 정확한 분리를 검증한 후, 에이전트 쉘과 캡처 엔진을 순차적으로 구축한다.

### 1.2 Key Technical Decisions

**시리얼 트랜스포트 재사용**: 기존 `lgapSerialTransport`와 `LGAPTransport` 인터페이스를 그대로 재사용한다. LGCP 전용 transport를 새로 만들 필요 없이, 설정만 다른 인스턴스를 생성하면 된다.

**프레임 파서 설계**: `io.Reader` 기반의 스트림 파서로 구현한다. 내부적으로 `bufio.Reader`를 사용하여 바이트 단위 스캔과 블록 읽기를 효율적으로 처리한다.

**캡처 엔진 분리**: 파일 I/O와 통계를 담당하는 `CaptureEngine`을 독립 구조체로 분리하여 테스트 용이성을 확보한다.

**JSONL 포맷 선택 이유**: 한 줄 한 레코드 형식으로 파일 append가 안전하고, `jq`/`grep` 등 표준 도구로 바로 분석 가능하며, 파일 손상 시에도 개별 레코드 단위로 복구 가능하다.

### 1.3 Architecture Decisions

| 결정 | 선택 | 근거 |
|------|------|------|
| 패키지 위치 | `internal/agent/lg/` (기존 패키지 확장) | LGAP과 공통 인프라(transport, serial_opener) 공유 |
| 파일 접두사 | `lgcp_` | LGAP 파일과 명확히 구분 |
| CRC 구현 | 순수 Go (외부 라이브러리 미사용) | 프로젝트 의존성 최소화 정책 준수 |
| 파일 포맷 | JSONL | 스트리밍 write, 도구 호환성, 파일 손상 내성 |
| 프레임 버퍼 | `bufio.Reader` | 효율적인 바이트 스캔, 표준 라이브러리 |

## 2. Milestones

### M1: Frame Parser + CRC (Primary Goal)

**목표**: 프로토콜 프레임의 정확한 식별과 CRC 검증

**산출물**:
- `lgcp_crc.go`: `CalcLGCPCRC16(data []byte) uint16`, `VerifyLGCPCRC(frame []byte) bool`
- `lgcp_frame.go`: `LGCPFrameParser` 구조체, `NewLGCPFrameParser(r io.Reader)`, `ReadFrame() (*LGCPFrame, error)`
- `lgcp_crc_test.go`: 프로토콜 문서 예제 기반 CRC 검증
- `lgcp_frame_test.go`: 정상/부분/손상/연속 프레임 파싱 테스트

**의존성**: 없음 (독립적으로 구현 및 테스트 가능)

**검증 방법**: `references/protocols/lg_internal_control_protocol.md`의 예제 프레임 `56 2D 04 44 55 00 66 04 44 55 00 00 02 04 F8 1A ...` 로 CRC 및 파싱 라운드트립 검증

### M2: Agent Shell (Primary Goal)

**목표**: 에이전트 프레임워크 통합 및 설정 관리

**산출물**:
- `lgcp_config.go`: `LGCPConfig` 구조체, `parseLGCPConfig(opts map[string]any) (LGCPConfig, error)`
- `lgcp_errors.go`: `ErrLGCPFrameTimeout`, `ErrLGCPInvalidLength`, `ErrLGCPCRCFailed` 등
- `lgcp_agent.go`: `LGCPAgent` 구조체 (agent.Agent, agent.MessageReceiver, agent.StatefulAgent 인터페이스 구현)
- `lgcp_register.go`: `RegisterLGCPTypes(mgr *agent.DefaultManager) error`
- `lgcp_config_test.go`: 설정 파싱, 기본값, 유효성 검증 테스트

**의존성**: M1 (프레임 파서)

### M3: Capture Engine (Primary Goal)

**목표**: 파일 기반 패킷 캡처 및 통계 추적

**산출물**:
- `lgcp_capture.go`: `CaptureEngine` 구조체
  - `NewCaptureEngine(config CaptureConfig) (*CaptureEngine, error)`
  - `WriteRecord(frame *LGCPFrame) error`
  - `RotateFile() error`
  - `Close() error`
  - `Stats() CaptureStats`
- `lgcp_capture_test.go`: JSONL 출력, 파일 로테이션, 통계 테스트
- `lgcp_agent_test.go`: 에이전트 + 캡처 엔진 통합 테스트

**의존성**: M1 (프레임 파서), M2 (에이전트 프레임워크)

### M4: Integration (Secondary Goal)

**목표**: 시스템 통합 및 사용 편의성

**산출물**:
- `cmd/xflowd/main.go`: `RegisterLGCPTypes()` 호출 추가
- `examples/agents/lgcp-capture.yaml`: 예제 설정 파일
- `web/src/config/agentSchemas.ts`: `LG_LGCP_FIELDS` 추가
- i18n 업데이트 (`en.json`, `ko.json`)

**의존성**: M2 (에이전트 등록)

### M5: Documentation (Optional Goal)

**목표**: 문서 정비 및 SPEC 완료

**산출물**:
- 프로토콜 참고 문서 업데이트
- SPEC 상태 업데이트 (Draft -> Done)

## 3. Risk Assessment

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 프로토콜 문서의 LEN 필드 해석이 정확하지 않을 수 있음 | 프레임 경계 오판별 | LEN 유효성 검사 범위를 넓게 설정, 부분 프레임도 저장하여 분석 가능하게 함 |
| 시리얼 버스에서 다른 프로토콜 트래픽이 혼재될 수 있음 | 가비지 데이터 증가 | STX 외의 바이트는 무시, 가비지 바이트 카운터로 모니터링 |
| CRC XOR 마스크(0x5C56)가 프레임 유형에 따라 다를 수 있음 | CRC 검증 실패 증가 | CRC 실패 프레임도 저장, 통계에서 CRC 실패율 모니터링 |
| LGAP과 동일 시리얼 포트를 사용할 수 없음 (별도 포트 필요) | 하드웨어 구성 제약 | 문서에 별도 포트 사용 안내, 향후 포트 공유 기능 검토 |

## 4. File Change Summary

| 파일 | 변경 유형 | 설명 |
|------|-----------|------|
| `internal/agent/lg/lgcp_crc.go` | 신규 | CRC-16/CCITT-FALSE 계산 |
| `internal/agent/lg/lgcp_frame.go` | 신규 | STX+LEN 기반 프레임 파서 |
| `internal/agent/lg/lgcp_capture.go` | 신규 | JSONL 파일 캡처 엔진 |
| `internal/agent/lg/lgcp_agent.go` | 신규 | 패시브 캡처 에이전트 |
| `internal/agent/lg/lgcp_config.go` | 신규 | 설정 구조체 및 파서 |
| `internal/agent/lg/lgcp_register.go` | 신규 | 타입 등록 함수 |
| `internal/agent/lg/lgcp_errors.go` | 신규 | 에러 변수 정의 |
| `internal/agent/lg/lgcp_*_test.go` | 신규 | 5개 테스트 파일 |
| `cmd/xflowd/main.go` | 수정 | RegisterLGCPTypes() 호출 추가 |
| `examples/agents/lgcp-capture.yaml` | 신규 | 예제 설정 파일 |
| `web/src/config/agentSchemas.ts` | 수정 | LG_LGCP_FIELDS 추가 |
| `web/src/lib/i18n/en.json` | 수정 | LGCP 관련 i18n 추가 |
| `web/src/lib/i18n/ko.json` | 수정 | LGCP 관련 i18n 추가 |
