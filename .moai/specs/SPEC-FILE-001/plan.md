---
id: SPEC-FILE-001
type: plan
spec_ref: SPEC-FILE-001
---

# SPEC-FILE-001 구현 계획

## 1. 구현 전략

### 1.1 접근 방식

기존 `FileAgentImpl` 구조체를 확장하되, 새로운 기능은 별도 파일로 분리하여 관심사를 분리한다. Bridge 통합은 기존 Bridge Node 인프라를 활용하며 File Agent 측에서 필요한 인터페이스만 구현한다.

### 1.2 기술 스택

| 기술 | 버전 | 용도 |
|------|------|------|
| Go | 1.23+ | 구현 언어 |
| fsnotify | 기존 버전 | 디렉토리 감시 (이미 사용 중) |
| encoding/json | stdlib | Process 명령 직렬화/역직렬화 |
| bufio | stdlib | 줄 단위 파일 읽기 |
| golang.org/x/text | 최신 안정 | 인코딩 변환 (UTF-8 외 지원 시) |

### 1.3 DDD 방법론 적용 (ANALYZE-PRESERVE-IMPROVE)

- **ANALYZE**: 기존 file.go (489줄), file_errors.go, file_test.go 분석
- **PRESERVE**: 기존 FileOperator 인터페이스와 []byte 동작의 characterization 테스트 작성
- **IMPROVE**: 새 인터페이스 추가 및 Process 메서드 구현

---

## 2. 마일스톤

### Primary Goal: Module 5 + Module 1 (에러 타입 + Text/Binary 모드)

**우선순위: High**

작업 분해:
1. `file_errors.go`에 5개 센티넬 에러 추가
2. `file_text.go` 신규 생성 - `TextFileOperator` 인터페이스 구현
3. `FileAgentImpl`에 모드 설정 필드 추가 (`mode`, `encoding`)
4. Init에서 AgentConfig.Transport.Options의 모드/인코딩 파싱
5. `ReadFileText`, `WriteFileText`, `AppendFileText` 구현
6. `ReadLines`, `ReadLine`, `WriteLines`, `AppendLine` 구현
7. 바이너리 모드 기존 동작 변경 없음 검증
8. 테스트: `file_text_test.go`

영향 파일:
- `internal/agent/system/file_errors.go` (수정)
- `internal/agent/system/file_text.go` (신규)
- `internal/agent/system/file.go` (수정 - 필드 추가)
- `internal/agent/system/file_text_test.go` (신규)

### Secondary Goal: Module 2 (Bridge Node 통합)

**우선순위: High**

작업 분해:
1. `FileAgentImpl.Process(data []byte) ([]byte, error)` 오버라이드 구현
2. JSON 명령 디스패처 구현 (`file_bridge.go`)
3. 명령별 핸들러 매핑 (read_file, write_file 등 10개 명령)
4. `MessageReceiver` 인터페이스 구현 - 이벤트 채널 기반
5. `file_bridge.go`에 이벤트 메시지 포맷터 구현
6. `bridge_adapter_file.go` - File Agent 전용 BridgeAdapter 등록
7. BridgeAdapter `TransformToAgent`/`TransformFromAgent` 구현
8. Agent 타입 등록: `RegisterAdapter("file", ...)` 호출
9. 4방향 (In/Out/InOut/RequestReply) 동작 검증
10. 테스트: `file_bridge_test.go`, `bridge_adapter_file_test.go`

영향 파일:
- `internal/agent/system/file.go` (수정 - Process 오버라이드, MessageReceiver 추가)
- `internal/agent/system/file_bridge.go` (신규)
- `internal/node/bridge_adapter_file.go` (신규)
- `internal/agent/system/file_bridge_test.go` (신규)
- `internal/node/bridge_adapter_file_test.go` (신규)

### Tertiary Goal: Module 3 (지정 파일 관리)

**우선순위: Medium**

작업 분해:
1. `file_config.go` 신규 생성 - target_files 설정 파싱
2. 별칭-경로 맵 관리 (`targetFiles sync.Map`)
3. Init에서 target_files 설정 로드 및 샌드박스 검증
4. 명령 핸들러에 `alias` 파라미터 지원 추가
5. target_files 디렉토리 자동 WatchDir 등록
6. Stop 시 자동 해제
7. 테스트: `file_config_test.go`

영향 파일:
- `internal/agent/system/file_config.go` (신규)
- `internal/agent/system/file.go` (수정 - Init/Stop에 target_files 로직)
- `internal/agent/system/file_bridge.go` (수정 - alias 지원)
- `internal/agent/system/file_config_test.go` (신규)

### Final Goal: Module 4 (파일 이벤트 알림 via Bridge)

**우선순위: Medium**

작업 분해:
1. watchLoop에서 이벤트를 Bridge 메시지 채널로 포워딩
2. 이벤트 메시지 JSON 포맷 구현
3. alias 매핑 (target_files 등록 파일의 alias 자동 포함)
4. 이벤트 필터링 (watch_events 설정)
5. 이벤트 버퍼 (event_buffer_size 설정)
6. 통합 테스트

영향 파일:
- `internal/agent/system/file.go` (수정 - watchLoop 이벤트 포워딩)
- `internal/agent/system/file_bridge.go` (수정 - 이벤트 메시지 포맷)
- `internal/agent/system/file_config.go` (수정 - 필터링/버퍼 설정)

---

## 3. 아키텍처 설계 방향

### 3.1 파일 분리 전략

```
internal/agent/system/
  file.go           -- FileAgentImpl 핵심 (기존 + Process/MessageReceiver 확장)
  file_text.go      -- TextFileOperator 구현 (신규)
  file_bridge.go    -- JSON 명령 디스패처, 이벤트 포맷터 (신규)
  file_config.go    -- target_files 관리, 설정 파싱 (신규)
  file_errors.go    -- 센티넬 에러 (확장)

internal/node/
  bridge_adapter_file.go  -- File Agent BridgeAdapter (신규)
```

### 3.2 Process 명령 흐름

```
Bridge Node (BridgeOut/RequestReply)
  -> ag.Process(jsonCommand)
    -> FileAgentImpl.Process(data)
      -> parseCommand(data)
      -> dispatchCommand(cmd)
        -> ReadFile / WriteFile / ReadLines / ...
      -> marshalResponse(result, err)
    <- JSON 응답
```

### 3.3 이벤트 수신 흐름

```
fsnotify.Watcher
  -> watchLoop() (기존)
    -> FileEvent 생성
    -> 기존 handler 호출 (WatchDir 콜백)
    -> 이벤트 채널에 전달 (신규)

Bridge Node (BridgeIn/InOut)
  -> transport.Receive()
    -> agentTransportAdapter.Receive()
      -> FileAgentImpl.ReceiveMessage(ctx)
        -> 이벤트 채널에서 읽기
      <- JSON 이벤트 메시지
```

### 3.4 동시성 설계

- 이벤트 채널: `chan []byte` (버퍼 크기: event_buffer_size, 기본 256)
- target_files 맵: `sync.Map` (동시 읽기 최적화)
- 모드/인코딩: Init 이후 불변 (sync 불필요)
- Process 메서드: 기존 FileOperator 메서드가 이미 동시 안전

---

## 4. 리스크 분석

### 4.1 기술 리스크

| 리스크 | 영향도 | 대응 |
|--------|--------|------|
| 기존 FileOperator 테스트 회귀 | High | PRESERVE 단계에서 characterization 테스트 보강 |
| Process 메서드와 BaseAgent.Process 충돌 | High | FileAgentImpl에서 명시적 오버라이드, 컴파일 타임 검증 |
| 이벤트 채널 포화 (높은 파일 변경률) | Medium | 버퍼 크기 설정 가능, 오버플로우 시 oldest 드롭 + 로그 |
| 인코딩 변환 성능 | Low | UTF-8은 Go 네이티브, 비 UTF-8은 x/text 사용 |

### 4.2 통합 리스크

| 리스크 | 영향도 | 대응 |
|--------|--------|------|
| BridgeAdapter 등록이 기존 어댑터와 충돌 | Low | agent Type()이 "file"로 고유하므로 충돌 없음 |
| Bridge Node의 PollableAdapter 인터페이스 불일치 | Medium | File Agent는 PollableAdapter 미구현, 표준 수신 루프 사용 |
| AgentConfig 옵션 파싱 타입 안전성 | Medium | 타입 assertion에 ok 패턴 사용, 파싱 실패 시 기본값 적용 |

---

## 5. 의존 관계

```
Module 5 (에러 타입) ← 모든 모듈에서 참조
Module 1 (Text/Binary) ← Module 2 (Process에서 텍스트 명령 디스패치)
Module 2 (Bridge 통합) ← Module 4 (이벤트를 Bridge로 전달)
Module 3 (지정 파일) ← Module 4 (alias를 이벤트에 포함)
```

구현 순서: Module 5 -> Module 1 -> Module 2 -> Module 3 -> Module 4

---

## 6. 전문가 상담 권고

### expert-backend 상담 권고

이 SPEC는 다음 영역에서 expert-backend 상담이 유익할 수 있다:

- Process 메서드 JSON 명령 프로토콜 설계
- MessageReceiver와 이벤트 채널 동시성 패턴
- BridgeAdapter 등록 및 통합 패턴

### expert-testing 상담 권고

- 파일 시스템 의존 테스트의 격리 전략
- fsnotify 이벤트 기반 테스트의 타이밍 이슈 대응
