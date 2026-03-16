---
id: SPEC-MODBUS-CLI-001
type: plan
version: "1.1.0"
created: "2026-03-16"
updated: "2026-03-16"
---

# SPEC-MODBUS-CLI-001: 구현 계획

## 기술 접근 방식

### 아키텍처 설계

기존 CLI 패턴을 그대로 따르는 단일 파일(`internal/cli/modbus.go`) 구현이다. 서버 측 변경 없이 기존 `POST /api/v1/agents/{id}/exec` 엔드포인트를 재사용하며, CLI 레이어에서 사용자 편의 추상화를 제공한다.

```
사용자 입력                  CLI 변환 레이어              기존 API
xflow modbus read ...  -->  에이전트 타입 감지     -->  POST /agents/{id}/exec
                            명령어 매핑                  {"command": "...", "params": {...}}
                            파라미터 변환
                            결과 포매팅
```

### 핵심 설계 결정

1. **단일 파일 구현**: `internal/cli/modbus.go`에 모든 커맨드를 집중. CLI 레이어는 비즈니스 로직이 아니라 UX 변환이므로 단일 파일이 적합
2. **에이전트 타입 자동 감지**: `GET /api/v1/agents/{id}` 한 번의 추가 호출로 Client/Server를 구분. 사용자가 에이전트 내부 구조를 알 필요 없음
3. **기존 함수 재사용**: `resolveAgentID`, `parseParamValue`, `PrintResult`, `DetailFormatter` 등 기존 CLI 유틸리티 활용
4. **플래그 기반 인터페이스**: `key=value` 대신 타입 안전한 Cobra 플래그 사용. 자동 완성, 도움말 생성, 기본값 제공의 이점

### 리스크 및 대응

| 리스크 | 영향 | 대응 방안 |
|--------|------|-----------|
| Agent API 응답 구조 변경 | 타입 감지 실패 | 타입 감지 함수에 방어 코드, 에러 메시지에 실제 타입 값 포함 |
| Client Agent에 device_id 누락 | 런타임 에러 | CLI 레벨에서 필수 플래그 검증 후 API 호출 |
| 레지스터 영역 별칭 충돌 | 잘못된 명령 전송 | 별칭 매핑을 상수 맵으로 관리, 테스트로 검증 |

---

## 마일스톤

### M1: 기반 구조 (Primary Goal) [Completed]

**범위**: 커맨드 트리 스캐폴딩 + 에이전트 타입 감지

**작업 내용**:
- `internal/cli/modbus.go` 파일 생성
- `newModbusCmd()` 함수: modbus 루트 커맨드 + 4개 서브커맨드 스캐폴딩
- `resolveModbusAgentType()` 함수: Agent API 호출 -> 타입 판별
- `root.go`에 `newModbusCmd(&client)` 등록
- 레지스터 영역 별칭 매핑 상수 정의

**영향 파일**:
- `internal/cli/modbus.go` (신규)
- `internal/cli/root.go` (1줄 추가)

**완료 기준**:
- `xflow modbus --help`가 서브커맨드 목록 출력
- `xflow modbus read --help`가 플래그 목록 출력
- 타입 감지 함수 단위 테스트 통과

### M2: read 명령어 (Primary Goal) [Completed]

**범위**: 레지스터 읽기 명령어 구현

**작업 내용**:
- Server 에이전트 읽기: 별칭 -> `get_holding_registers`/`get_input_registers`/`get_coils`/`get_discrete_inputs` 매핑
- Client 에이전트 읽기: 별칭 -> `read_registers` + `register` 파라미터 매핑
- `--device` 플래그 Client 전용 필수 검증
- `--force` 플래그 Client 전용 캐시 무시
- `--type` 플래그로 데이터 타입 전달
- 테이블 출력 포매팅 (address, raw_value, typed_value, type)

**영향 파일**:
- `internal/cli/modbus.go`

**의존성**: M1 완료 필수

**완료 기준**:
- Server 에이전트에서 4개 레지스터 영역 읽기 정상 동작
- Client 에이전트에서 device ID 지정 읽기 정상 동작
- 레지스터 별칭 모두 정상 변환
- 타입 지정 읽기 시 typed_values 출력

### M3: write 명령어 (Primary Goal) [Completed]

**범위**: 레지스터 쓰기 명령어 구현

**작업 내용**:
- 단일 값 / 다중 값 판별 로직 (`--values` 파싱)
- Server 에이전트: `set_register`/`set_registers`/`set_coil`/`set_coils` 매핑
- Client 에이전트: `write_register`/`write_registers`/`write_coil`/`write_coils` 매핑
- `--device` 플래그 Client 전용 필수 검증
- `--type` 플래그로 데이터 타입 전달
- 쓰기 결과 출력 포매팅

**영향 파일**:
- `internal/cli/modbus.go`

**의존성**: M1 완료 필수

**완료 기준**:
- 단일/다중 값 쓰기 정상 동작
- coils, holding 영역 쓰기 정상 동작
- 데이터 타입 지정 쓰기 정상 동작

### M4: status/map 명령어 + 테스트 (Secondary Goal) [Completed]

**범위**: 보조 명령어 및 전체 테스트

**작업 내용**:
- `status` 서브커맨드: `get_status` Process 명령 전송 + 결과 출력
- `map` 서브커맨드: `get_map` Process 명령 전송 + Server 전용 검증
- `internal/cli/modbus_test.go` 테이블 기반 테스트 작성:
  - 타입 감지 테스트 (client, server, non-modbus, 에러)
  - read 명령 테스트 (각 레지스터 영역 x 각 에이전트 타입)
  - write 명령 테스트 (단일/다중 x holding/coils)
  - status/map 테스트
  - 에러 케이스 (device 누락, 잘못된 레지스터, non-modbus 에이전트)

**영향 파일**:
- `internal/cli/modbus.go`
- `internal/cli/modbus_test.go` (신규)

**의존성**: M1, M2, M3 완료 필수

**완료 기준**:
- status/map 명령어 정상 동작
- Client 에이전트에서 map 명령 에러 정상 반환
- 테스트 커버리지 85% 이상
- 모든 테스트 통과 (`go test -race ./internal/cli/ -run TestModbus`)

### M5: Interactive 모드 호환 (Bugfix) [Completed]

**범위**: REPL 모드에서 modbus 서브커맨드 플래그 리셋 수정

**작업 내용**:
- `resetFlags()` 함수를 재귀적으로 모든 서브커맨드 플래그를 리셋하도록 개선
- `Changed` 속성도 함께 리셋하여 Cobra 플래그 상태 완전 초기화
- `resetCommandFlags()` 헬퍼 함수 추출
- `TestREPL_ModbusSubcommandFlagReset` 테스트 추가

**영향 파일**:
- `internal/cli/interactive.go` (수정: resetFlags → resetCommandFlags 재귀)
- `internal/cli/interactive_test.go` (테스트 추가)

**의존성**: M1-M4 완료 필수

**완료 기준**:
- REPL에서 `modbus read` 반복 실행 시 플래그 기본값 정상 복원
- `Changed` 플래그가 실행 간 false로 리셋
- 기존 interactive 테스트 전체 통과

---

## 구현 가이드라인

### 코딩 패턴

1. **기존 agent.go 패턴 따르기**: `newAgentExecCmd`의 구조를 참고하되, 플래그 기반으로 변환
2. **에러 처리**: `ErrInvalidInput()` 래퍼 사용, API 에러는 그대로 전파
3. **테스트**: `httptest.NewServer`를 사용한 mock API 서버 기반 테이블 주도 테스트
4. **주석**: 모든 exported 함수에 GoDoc 주석 (한국어)

### 파일 구조

```
internal/cli/
  modbus.go          # newModbusCmd, read/write/status/map 서브커맨드, 타입 감지, 별칭 매핑
  modbus_test.go     # 테이블 주도 테스트
  root.go            # 1줄 수정: newModbusCmd 등록
```

### 커밋 전략

```
feat(cli): add modbus subcommand scaffolding and agent type detection (M1)
feat(cli): implement modbus read command with register alias mapping (M2)
feat(cli): implement modbus write command with single/multi value support (M3)
feat(cli): add modbus status/map commands and comprehensive tests (M4)
```
