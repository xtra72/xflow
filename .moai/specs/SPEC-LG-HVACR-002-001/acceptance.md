# SPEC-LG-HVACR-002-001: Acceptance Criteria (v2.0.0 - Clean Transport Abstraction)

**SPEC Reference**: SPEC-LG-HVACR-002-001 (이전: SPEC-LGCP-001)
**Version**: 2.0.0
**Created**: 2026-04-06

> **명명 규약 (v2.0 rename, 2026-05-29 이후)**: 프로토콜 `lg_icp02` (LG ICP-02) / 에이전트 `lg_hvacr02` (LG HVACR-02).

---

## v1.0.0 Acceptance Criteria (기 구현 완료)

아래 항목은 v1.0.0에서 정의되어 이미 구현/검증 완료되었다. v2.0.0 변경으로 인해 regression이 발생하지 않아야 한다.

- AC-001 ~ AC-003: CRC-16 계산 및 검증
- AC-004 ~ AC-008: 프레임 파서 (정상, 가비지, 연속, 부분, 잘못된 LEN)
- AC-009 ~ AC-013: 캡처 엔진 (JSONL, 헤더, CRC 실패, 로테이션, 최대 파일)
- AC-014 ~ AC-018: 에이전트 라이프사이클 (시작, 정지, Pause/Resume, 통계, 재연결)
- AC-019 ~ AC-021: 에이전트 설정 (기본값, 필수 필드, 전체 필드)
- AC-022 ~ AC-024: 패시브 동작 (전송 금지, get_stats, 제어 명령 거부)
- AC-025 ~ AC-026: 타입 등록

---

## v2.0.0 Acceptance Criteria (신규)

## Module 8: Transport Type Configuration (REQ-LG-HVACR-002-001-13, REQ-LG-HVACR-002-001-18)

### AC-027: Transport Type 기본값

**Given** `transport_type` 설정이 없는 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** `TransportType` 이 `"serial"` 로 설정되어야 하며, 기존 serial 모드로 동작해야 한다

### AC-028: TCP Client Mode 설정

**Given** `transport_type: tcp-client`, `tcp_host: "192.168.1.100"`, `tcp_port: 8899` 가 설정된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** `TransportType` 이 `"tcp-client"`, `TCPHost` 이 `"192.168.1.100"`, `TCPPort` 이 `8899` 로 설정되어야 한다

### AC-029: TCP Server Mode 설정

**Given** `transport_type: tcp-server`, `tcp_port: 9900` 가 설정된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** `TransportType` 이 `"tcp-server"`, `TCPHost` 이 기본값 `"0.0.0.0"`, `TCPPort` 이 `9900` 으로 설정되어야 한다

### AC-030: 잘못된 Transport Type

**Given** `transport_type: unknown` 이 설정된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** 에러를 반환해야 한다

### AC-031: Serial Mode에서 serial_port 필수

**Given** `transport_type: serial` 이면서 `serial_port` 가 누락된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** `ErrHvacr02SerialPortRequired` 에러를 반환해야 한다 (기존 동작 유지)

### AC-032: TCP Client Mode에서 serial_port 불필요

**Given** `transport_type: tcp-client`, `tcp_host: "192.168.1.100"`, `tcp_port: 8899` 가 설정되고 `serial_port` 가 없는 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** 에러 없이 정상 파싱되어야 한다

### AC-033: TCP Client Mode에서 tcp_host/tcp_port 필수

**Given** `transport_type: tcp-client` 이면서 `tcp_port` 가 누락된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** 에러를 반환해야 한다

### AC-034: TCP Server Mode에서 tcp_port 필수

**Given** `transport_type: tcp-server` 이면서 `tcp_port` 가 누락된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** 에러를 반환해야 한다

### AC-035: TCP 타임아웃 기본값

**Given** `transport_type: tcp-client`, `tcp_host: "192.168.1.100"`, `tcp_port: 8899` 가 설정되고 타임아웃 미설정
**When** `parseHvacr02Config()` 를 호출하면
**Then** `TCPReadTimeout` 이 500ms, `TCPWriteTimeout` 이 1s, `TCPConnectTimeout` 이 5s 로 기본값이 적용되어야 한다

### AC-036: TCP 타임아웃 커스텀 값

**Given** `tcp_read_timeout: "2s"`, `tcp_write_timeout: "3s"`, `tcp_connect_timeout: "10s"` 가 설정된 YAML 옵션
**When** `parseHvacr02Config()` 를 호출하면
**Then** 각각 2s, 3s, 10s 로 파싱되어야 한다

---

## Module 9: TCP Client Transport (REQ-LG-HVACR-002-001-14)

### AC-037: TCP Client Open/Connect

**Given** `lgapTCPClientTransport` 가 host="127.0.0.1", port=테스트포트 로 설정되고 로컬 TCP 서버가 리스닝 중
**When** `Open()` 을 호출하면
**Then** TCP 연결이 수립되고 `Available()` 이 `true` 를 반환해야 한다

### AC-038: TCP Client Send/Receive

**Given** 연결된 `lgapTCPClientTransport`
**When** `Send([]byte{0x56, 0x2D, ...})` 로 LG ICP-02 프레임 바이트를 전송하면
**Then** 원격 서버에서 동일한 바이트를 수신할 수 있어야 한다

### AC-039: TCP Client Receive 데이터

**Given** 연결된 `lgapTCPClientTransport` 와 원격 서버가 데이터를 전송
**When** `Receive(buf)` 를 호출하면
**Then** 원격 서버에서 전송한 바이트를 수신하고 읽은 바이트 수를 반환해야 한다

### AC-040: TCP Client Close

**Given** 연결된 `lgapTCPClientTransport`
**When** `Close()` 를 호출하면
**Then** TCP 연결이 종료되고 `Available()` 이 `false` 를 반환해야 한다

### AC-041: TCP Client 연결 끊김 감지

**Given** 연결된 `lgapTCPClientTransport` 에서 원격 서버가 연결을 종료
**When** `Receive(buf)` 또는 `Send(data)` 를 호출하면
**Then** 에러를 반환하고 `Available()` 이 `false` 로 전환되어야 한다

### AC-042: TCP Client 연결 타임아웃

**Given** `lgapTCPClientTransport` 가 응답하지 않는 호스트를 대상으로 설정 (connectTimeout=1s)
**When** `Open()` 을 호출하면
**Then** connectTimeout 이후 에러를 반환해야 한다

### AC-043: TCP Client Read Deadline

**Given** 연결된 `lgapTCPClientTransport` (readTimeout=100ms) 에서 원격 서버가 데이터를 보내지 않음
**When** `Receive(buf)` 를 호출하면
**Then** readTimeout 이후 타임아웃 에러를 반환해야 한다

### AC-044: TCP Client 동시 접근 안전성

**Given** `lgapTCPClientTransport` 인스턴스
**When** 여러 고루틴에서 동시에 Send와 Receive를 수행하면
**Then** `go test -race` 에서 race condition이 감지되지 않아야 한다

---

## Module 10: TCP Server Transport (REQ-LG-HVACR-002-001-15, REQ-LG-HVACR-002-001-21)

### AC-045: TCP Server Open/Listen

**Given** `lgapTCPServerTransport` 가 host="127.0.0.1", port=테스트포트 로 설정
**When** `Open()` 을 호출하면
**Then** 지정된 포트에서 TCP 리스닝이 시작되어야 한다

### AC-046: TCP Server Accept Connection

**Given** 리스닝 중인 `lgapTCPServerTransport`
**When** 외부 TCP 클라이언트가 접속하면
**Then** 연결이 수락되고 `Available()` 이 `true` 를 반환해야 한다

### AC-047: TCP Server Send/Receive

**Given** 클라이언트가 접속된 `lgapTCPServerTransport`
**When** `Send(data)` 와 `Receive(buf)` 를 호출하면
**Then** 접속된 클라이언트와 양방향 데이터 전송이 가능해야 한다

### AC-048: TCP Server 연결 교체 (최신 우선)

**Given** 클라이언트A가 접속된 `lgapTCPServerTransport`
**When** 클라이언트B가 새로 접속하면
**Then** 클라이언트A 연결이 닫히고 클라이언트B가 활성 연결이 되어야 하며, `Available()` 이 `true` 를 유지해야 한다

### AC-049: TCP Server 연결 없이 Send 시 에러

**Given** 리스닝 중이지만 접속된 클라이언트가 없는 `lgapTCPServerTransport`
**When** `Send(data)` 를 호출하면
**Then** 에러를 반환해야 한다

### AC-050: TCP Server Close

**Given** 클라이언트가 접속된 `lgapTCPServerTransport`
**When** `Close()` 를 호출하면
**Then** 리스너와 활성 연결이 모두 종료되고 Accept 루프가 종료되어야 한다

### AC-051: TCP Server 클라이언트 연결 끊김

**Given** 클라이언트가 접속된 `lgapTCPServerTransport` 에서 클라이언트가 연결을 종료
**When** `Receive(buf)` 를 호출하면
**Then** 에러를 반환하고 `Available()` 이 `false` 로 전환되어야 한다

### AC-052: TCP Server 동시 접근 안전성

**Given** `lgapTCPServerTransport` 인스턴스
**When** Accept 루프, Send, Receive 가 동시에 실행되면
**Then** `go test -race` 에서 race condition이 감지되지 않아야 한다

---

## Module 11: Transport Factory (REQ-LG-HVACR-002-001-19)

### AC-053: Serial Transport 생성

**Given** `transport_type: "serial"` (또는 미설정) 이고 `serial_port: "/dev/ttyUSB1"` 이 설정된 config
**When** Transport Factory 를 실행하면
**Then** `lgapSerialTransport` 인스턴스가 생성되어야 한다

### AC-054: TCP Client Transport 생성

**Given** `transport_type: "tcp-client"`, `tcp_host: "192.168.1.100"`, `tcp_port: 8899` 가 설정된 config
**When** Transport Factory 를 실행하면
**Then** `lgapTCPClientTransport` 인스턴스가 생성되어야 한다

### AC-055: TCP Server Transport 생성

**Given** `transport_type: "tcp-server"`, `tcp_port: 9900` 이 설정된 config
**When** Transport Factory 를 실행하면
**Then** `lgapTCPServerTransport` 인스턴스가 생성되어야 한다

### AC-056: 알 수 없는 Transport Type 에러

**Given** `transport_type: "bluetooth"` 가 설정된 config
**When** Transport Factory 를 실행하면
**Then** 에러를 반환해야 한다

---

## Module 12: Integration (REQ-LG-HVACR-002-001-16, REQ-LG-HVACR-002-001-17)

### AC-057: captureLoop + TCP Client Transport

**Given** TCP Client transport 로 생성된 LG HVACR-02 에이전트, 로컬 TCP 서버가 LG ICP-02 프레임을 전송
**When** 에이전트를 Start 하고 TCP 서버에서 완전한 LG ICP-02 프레임 바이트를 전송하면
**Then** captureLoop 이 프레임을 파싱하여 msgCh에 `type: "lg_icp02_frame"` 이벤트를 전달해야 한다

### AC-058: captureLoop + TCP Server Transport

**Given** TCP Server transport 로 생성된 LG HVACR-02 에이전트, TCP 클라이언트가 접속하여 LG ICP-02 프레임을 전송
**When** 에이전트를 Start 하고 TCP 클라이언트에서 완전한 LG ICP-02 프레임 바이트를 전송하면
**Then** captureLoop 이 프레임을 파싱하여 msgCh에 `type: "lg_icp02_frame"` 이벤트를 전달해야 한다

### AC-059: Process() 명령이 TCP Transport 로 전송

**Given** TCP Client transport 로 생성된 LG HVACR-02 에이전트 (control_enabled: true)
**When** `Process({"command":"set_power", "address":"44550066", "params":{"power":true}})` 를 호출하면
**Then** 빌드된 LG ICP-02 프레임 바이트가 TCP 연결을 통해 원격 서버로 전송되어야 한다

### AC-060: reconnectLoop + TCP Client

**Given** TCP Client transport 로 생성된 LG HVACR-02 에이전트, 원격 서버가 연결을 종료
**When** reconnectLoop 이 `Available()` == false 를 감지하면
**Then** `transport.Close()` → `transport.Open()` 을 호출하여 재연결을 시도해야 한다

### AC-061: reconnectLoop + TCP Server

**Given** TCP Server transport 로 생성된 LG HVACR-02 에이전트, 클라이언트가 연결을 종료
**When** 새로운 TCP 클라이언트가 접속하면
**Then** Accept 루프가 새 연결을 수락하고 `Available()` 이 `true` 로 전환되어야 한다

### AC-062: 기존 Serial 테스트 전체 통과

**Given** v2.0.0 코드 변경이 완료된 상태
**When** `go test -race ./internal/agent/lg/...` 를 실행하면
**Then** v1.1.0에서 통과하던 모든 테스트가 여전히 통과해야 한다

### AC-063: 기존 API 응답 형식 유지

**Given** Serial 모드로 실행 중인 LG HVACR-02 에이전트
**When** `Process({"command":"get_stats"})` 를 호출하면
**Then** v1.1.0과 동일한 JSON 응답 형식을 반환해야 한다

### AC-064: 기존 YAML 설정 호환

**Given** v1.1.0에서 사용하던 `transport_type` 필드 없는 YAML 설정 파일
**When** v2.0.0 에이전트로 로딩하면
**Then** `serial` 모드로 정상 동작하며, 기존과 동일한 캡처 결과를 생성해야 한다

---

## Quality Gates (Definition of Done)

- [ ] 기존 v1.0.0/v1.1.0 테스트 전체 통과 (`go test -v -race ./internal/agent/lg/...`)
- [ ] 신규 TCP Client Transport 테스트 전체 통과
- [ ] 신규 TCP Server Transport 테스트 전체 통과
- [ ] Transport Factory 테스트 전체 통과
- [ ] TCP 통합 테스트 (captureLoop + TCP transport) 통과
- [ ] 동시 접근 안전성 검증 (`go test -race`)
- [ ] CRC-16 계산이 프로토콜 문서 예제와 일치 (기존 검증 유지)
- [ ] lgapSerialTransport 코드 미수정 확인
- [ ] LGAPTransport 인터페이스 미변경 확인
- [ ] LG HVACR-02 에이전트 captureLoop/transportReader/Process 내부 구조 미변경 확인
- [ ] `go build ./cmd/xflowd/` 성공
- [ ] `go vet ./internal/agent/lg/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 유지
