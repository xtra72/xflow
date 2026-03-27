# SPEC-LGCP-001: Acceptance Criteria

**SPEC Reference**: SPEC-LGCP-001
**Version**: 1.0.0
**Created**: 2026-03-24

## 1. CRC-16 Calculation (REQ-LGCP-001-04)

### AC-001: CRC 계산 정확성

**Given** 프로토콜 문서의 예제 프레임 데이터 `04 44 55 00 66 04 44 55 00 00 02 04 F8 1A 11 00 10 C0 18 00 1A C0 13 00 13 40 13 C0 16 00 18 40 18 80 29 C0 1D C0 91 9D 98`
**When** CRC-16/CCITT-FALSE 알고리즘으로 계산하고 0x5C56 XOR 마스크를 적용하면
**Then** 결과는 `0xDF35` (Big-endian: DF 35) 이어야 한다

### AC-002: CRC 검증 성공

**Given** CRC가 유효한 완전한 프레임 `56 2D 04 44 55 00 66 04 44 55 00 00 02 04 F8 1A ... DF 35`
**When** `VerifyLGCPCRC(frame)` 를 호출하면
**Then** `true` 를 반환해야 한다

### AC-003: CRC 검증 실패

**Given** CRC 바이트가 변조된 프레임
**When** `VerifyLGCPCRC(frame)` 를 호출하면
**Then** `false` 를 반환해야 한다

## 2. Frame Parser (REQ-LGCP-001-03, REQ-LGCP-001-05)

### AC-004: 정상 프레임 파싱

**Given** 프로토콜 예제 프레임 바이트 스트림
**When** `LGCPFrameParser.ReadFrame()` 을 호출하면
**Then** 다음 필드가 정확하게 파싱되어야 한다:
- `DA` = `44 55 00 66`
- `SA` = `44 55 00 00`
- `CMD` = `02 04`
- `SEQ0` = `0xF8`
- `PLEN` = `0x1A` (26 bytes)
- `SEQ1` = `0x98`
- `CRCValid` = `true`
- `ParseErr` = `nil`

### AC-005: 가비지 데이터 후 정상 프레임

**Given** STX 이전에 임의 바이트 `[AA BB CC DD]` 가 있고 그 뒤에 정상 프레임이 있는 스트림
**When** `ReadFrame()` 을 호출하면
**Then** 가비지 바이트를 건너뛰고 정상 프레임을 반환해야 한다

### AC-006: 연속 프레임 분리

**Given** 두 개의 완전한 프레임이 연속된 바이트 스트림
**When** `ReadFrame()` 을 두 번 호출하면
**Then** 각각 독립적인 프레임을 반환해야 하며, 두 프레임 모두 정확히 파싱되어야 한다

### AC-007: 부분 프레임 처리

**Given** STX와 LEN 이후 프레임이 불완전한 상태에서 EOF가 발생하는 스트림
**When** `ReadFrame()` 을 호출하면
**Then** `ParseErr` 가 non-nil인 `LGCPFrame` 을 반환하고, `Raw` 에 수신된 바이트가 포함되어야 한다

### AC-008: 잘못된 LEN 값 건너뛰기

**Given** STX=0x56 뒤에 LEN=0x05 (최소 프레임 크기 미만)인 스트림
**When** `ReadFrame()` 이 해당 바이트를 처리하면
**Then** 해당 STX를 무시하고 다음 STX를 탐색해야 한다

## 3. Capture Engine (REQ-LGCP-001-06, REQ-LGCP-001-07)

### AC-009: JSONL 파일 출력

**Given** 캡처 엔진이 시작되고 프레임이 수신되면
**When** `WriteRecord(frame)` 을 호출하면
**Then** 캡처 파일에 유효한 JSON 한 줄이 추가되어야 하며, `timestamp`, `seq`, `raw_hex`, `length`, `crc_valid` 필드가 포함되어야 한다

### AC-010: 파싱된 헤더 포함

**Given** 정상 파싱된 프레임
**When** 캡처 레코드가 기록되면
**Then** `parsed` 필드에 `da`, `sa`, `cmd`, `seq0`, `plen`, `payload_hex`, `seq1` 이 포함되어야 한다

### AC-011: CRC 실패 프레임 저장

**Given** CRC 검증이 실패한 프레임
**When** 캡처 레코드가 기록되면
**Then** `crc_valid` 는 `false` 이고, `raw_hex` 에 전체 원시 데이터가 포함되어야 한다

### AC-012: 파일 로테이션

**Given** 캡처 파일 크기가 `max_file_size` 에 도달하면
**When** 다음 레코드 기록 시
**Then** 새 캡처 파일이 생성되어야 하며, 파일 이름에 타임스탬프가 포함되어야 한다

### AC-013: 최대 파일 수 관리

**Given** `max_files` 개의 캡처 파일이 존재하고 새 파일이 생성되면
**When** 로테이션이 실행되면
**Then** 가장 오래된 캡처 파일이 삭제되어야 한다

## 4. Agent Lifecycle (REQ-LGCP-001-08, REQ-LGCP-001-09)

### AC-014: 에이전트 시작

**Given** LGCP 에이전트가 설정과 함께 생성되면
**When** `Start(ctx)` 를 호출하면
**Then** 시리얼 트랜스포트가 열리고 캡처 루프 고루틴이 시작되어야 한다

### AC-015: 에이전트 정지

**Given** 캡처 중인 LGCP 에이전트
**When** `Stop(ctx)` 를 호출하면
**Then** 캡처 루프가 종료되고, 현재 캡처 파일이 정상적으로 닫혀야 하며 (flush), 트랜스포트가 닫혀야 한다

### AC-016: Pause/Resume

**Given** 캡처 중인 에이전트를 Pause 하면
**When** 시리얼 버스에서 데이터가 계속 수신되면
**Then** 데이터가 버퍼에 축적되어야 하며, Resume 후 축적된 데이터를 처리해야 한다

### AC-017: 캡처 통계

**Given** 에이전트가 여러 프레임을 캡처한 후
**When** `State()` 를 호출하면
**Then** `frames_captured`, `frames_valid`, `frames_invalid`, `bytes_read`, `current_file` 등의 통계가 포함되어야 한다

### AC-018: 재연결 로직

**Given** 트랜스포트 연결이 끊긴 상태
**When** 재연결 루프가 실행되면
**Then** 지수 백오프로 재연결을 시도하고, 성공 시 캡처 루프를 재시작해야 한다

## 5. Agent Configuration (REQ-LGCP-001-10)

### AC-019: 기본값 적용

**Given** `serial_port` 만 설정된 YAML 옵션
**When** `parseLGCPConfig()` 를 호출하면
**Then** `baud_rate=9600`, `data_bits=8`, `stop_bits=1`, `parity="none"`, `capture_dir="./captures"`, `max_file_size=104857600` 등 기본값이 적용되어야 한다

### AC-020: 필수 필드 검증

**Given** `serial_port` 가 누락된 옵션
**When** `parseLGCPConfig()` 를 호출하면
**Then** 에러를 반환해야 한다

### AC-021: 모든 필드 설정

**Given** 모든 설정 필드가 포함된 YAML 옵션
**When** `parseLGCPConfig()` 를 호출하면
**Then** 모든 값이 정확히 파싱되어야 한다

## 6. Passive Operation (REQ-LGCP-001-11)

### AC-022: 데이터 전송 금지

**Given** LGCP 에이전트가 실행 중일 때
**When** 에이전트 동작을 관찰하면
**Then** 시리얼 트랜스포트의 `Send()` 메서드가 호출되지 않아야 한다

### AC-023: get_stats 명령

**Given** 캡처 중인 에이전트에
**When** `Process({"command":"get_stats"})` 를 호출하면
**Then** 캡처 통계 JSON을 반환해야 한다

### AC-024: 제어 명령 거부

**Given** LGCP 에이전트에
**When** `Process({"command":"set_power"})` 같은 제어 명령을 호출하면
**Then** 에러를 반환해야 한다

## 7. Agent Type Registration (REQ-LGCP-001-01)

### AC-025: 타입 등록

**Given** 에이전트 매니저
**When** `RegisterLGCPTypes(mgr)` 를 호출하면
**Then** `"lgcp"` 타입이 등록되어야 하며, 해당 타입으로 에이전트 생성이 가능해야 한다

### AC-026: 중복 등록 방지

**Given** `"lgcp"` 타입이 이미 등록된 매니저
**When** `RegisterLGCPTypes(mgr)` 를 다시 호출하면
**Then** 에러를 반환해야 한다

## 8. Quality Gates (Definition of Done)

- [ ] 모든 단위 테스트 통과 (`go test -v -race ./internal/agent/lg/...`)
- [ ] CRC-16 계산이 프로토콜 문서 예제와 일치
- [ ] 프레임 파서가 정상/부분/손상 프레임을 올바르게 처리
- [ ] JSONL 캡처 파일이 `jq` 로 파싱 가능
- [ ] 파일 로테이션이 설정된 크기와 개수에 따라 정상 동작
- [ ] 에이전트 생명주기 (Start/Stop/Pause/Resume) 정상 동작
- [ ] 시리얼 버스에 데이터를 전송하지 않음 (패시브 보장)
- [ ] `go build ./cmd/xflowd/` 성공
- [ ] `go vet ./internal/agent/lg/...` 경고 없음
