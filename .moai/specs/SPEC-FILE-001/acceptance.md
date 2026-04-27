---
id: SPEC-FILE-001
type: acceptance
spec_ref: SPEC-FILE-001
---

# SPEC-FILE-001 인수 기준

## Module 1: Text/Binary 모드 설정 및 연산

### AC-FILE-001: 텍스트 모드 파일 읽기

```gherkin
Scenario: 텍스트 모드에서 파일을 문자열로 읽는다
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 UTF-8 텍스트 파일 "test.txt"가 "Hello, World!"를 포함하고 있다
  When ReadFileText("test.txt")를 호출한다
  Then 반환값은 "Hello, World!" 문자열이다
  And 에러는 nil이다
  And fileStats.filesRead가 1 증가한다
```

### AC-FILE-002: 텍스트 모드 줄 단위 읽기

```gherkin
Scenario: 텍스트 모드에서 모든 줄을 슬라이스로 읽는다
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 "lines.txt"가 3줄("line1\nline2\nline3")을 포함하고 있다
  When ReadLines("lines.txt")를 호출한다
  Then 반환값은 ["line1", "line2", "line3"]이다
  And 에러는 nil이다

Scenario: 특정 줄 번호 읽기 (1-based)
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 "lines.txt"가 3줄을 포함하고 있다
  When ReadLine("lines.txt", 2)를 호출한다
  Then 반환값은 "line2"이다

Scenario: 줄 번호가 범위를 초과할 때 에러 반환
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 "lines.txt"가 3줄을 포함하고 있다
  When ReadLine("lines.txt", 10)을 호출한다
  Then ErrLineOutOfRange 에러가 반환된다
```

### AC-FILE-003: 바이너리 모드에서 텍스트 전용 연산 차단

```gherkin
Scenario: 바이너리 모드에서 ReadFileText 호출 시 에러
  Given File Agent가 mode="binary"로 초기화되었다 (기본값)
  When ReadFileText("test.txt")를 호출한다
  Then ErrTextModeRequired 에러가 반환된다

Scenario: 바이너리 모드에서 기존 ReadFile은 정상 동작
  Given File Agent가 mode="binary"로 초기화되었다
  And 샌드박스 내에 바이너리 파일 "data.bin"이 존재한다
  When ReadFile("data.bin")을 호출한다
  Then []byte 데이터가 정상 반환된다
  And 에러는 nil이다
```

### AC-FILE-004: 텍스트 모드 파일 쓰기

```gherkin
Scenario: 텍스트 모드에서 문자열을 파일에 쓴다
  Given File Agent가 mode="text"로 초기화되었다
  When WriteFileText("output.txt", "Hello!", 0644)를 호출한다
  Then 샌드박스 내에 "output.txt"가 "Hello!" 내용으로 생성된다
  And fileStats.filesWritten이 1 증가한다

Scenario: 텍스트 모드에서 한 줄 추가
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 "log.txt"가 "line1"을 포함하고 있다
  When AppendLine("log.txt", "line2")를 호출한다
  Then "log.txt"의 내용은 "line1\nline2\n"이다
```

---

## Module 2: Bridge Node 통합

### AC-FILE-010: Process 메서드 - 파일 읽기 명령

```gherkin
Scenario: Process로 read_file 명령을 실행한다
  Given File Agent가 초기화되었다
  And 샌드박스 내에 "data.txt"가 "test content"를 포함하고 있다
  When Process({"command":"read_file","params":{"path":"data.txt"}})를 호출한다
  Then JSON 응답의 success는 true이다
  And JSON 응답의 data에 base64 인코딩된 "test content"가 포함된다

Scenario: Process로 존재하지 않는 파일 읽기
  Given File Agent가 초기화되었다
  When Process({"command":"read_file","params":{"path":"nonexist.txt"}})를 호출한다
  Then JSON 응답의 success는 false이다
  And JSON 응답의 error에 "file not found"가 포함된다
```

### AC-FILE-011: Process 메서드 - 파일 쓰기 명령

```gherkin
Scenario: Process로 write_file 명령을 실행한다
  Given File Agent가 초기화되었다
  When Process({"command":"write_file","params":{"path":"new.txt","data":"aGVsbG8=","perm":420}})를 호출한다
  Then JSON 응답의 success는 true이다
  And 샌드박스 내에 "new.txt"가 "hello" 내용으로 생성된다

Scenario: Process로 알 수 없는 명령 실행
  Given File Agent가 초기화되었다
  When Process({"command":"unknown_cmd","params":{}})를 호출한다
  Then JSON 응답의 success는 false이다
  And JSON 응답의 error에 "unsupported command"가 포함된다
```

### AC-FILE-012: Process 메서드 - 텍스트 명령

```gherkin
Scenario: Process로 read_lines 명령을 실행한다
  Given File Agent가 mode="text"로 초기화되었다
  And 샌드박스 내에 "multi.txt"가 "a\nb\nc"를 포함하고 있다
  When Process({"command":"read_lines","params":{"path":"multi.txt"}})를 호출한다
  Then JSON 응답의 success는 true이다
  And JSON 응답의 data에 ["a","b","c"]가 포함된다

Scenario: 바이너리 모드에서 텍스트 명령 실행 시 에러
  Given File Agent가 mode="binary"로 초기화되었다
  When Process({"command":"read_lines","params":{"path":"test.txt"}})를 호출한다
  Then JSON 응답의 success는 false이다
  And JSON 응답의 error에 "text mode required"가 포함된다
```

### AC-FILE-013: MessageReceiver를 통한 이벤트 수신

```gherkin
Scenario: Bridge Node가 파일 이벤트를 수신한다
  Given File Agent가 초기화되었다
  And Bridge Node가 BridgeIn 방향으로 File Agent에 연결되었다
  And WatchDir로 "testdir/"를 감시하고 있다
  When "testdir/new_file.txt"가 생성된다
  Then Bridge Node의 수신 루프가 FileEvent를 JSON 메시지로 수신한다
  And 메시지의 event 필드는 "create"이다
  And 메시지의 path 필드에 "testdir/new_file.txt"가 포함된다

Scenario: 이벤트 채널이 가득 찼을 때 oldest 드롭
  Given File Agent가 event_buffer_size=2로 초기화되었다
  And 이벤트 채널에 이미 2개의 이벤트가 대기 중이다
  When 새로운 파일 이벤트가 발생한다
  Then 가장 오래된 이벤트가 드롭된다
  And 새 이벤트가 채널에 추가된다
  And 드롭 로그가 기록된다
```

### AC-FILE-014: Bridge 방향별 동작

```gherkin
Scenario: BridgeOut 방향에서 명령 실행
  Given Bridge Node가 BridgeOut 방향으로 File Agent에 연결되었다
  When 플로우에서 {"command":"file_exists","params":{"path":"test.txt"}} 메시지를 전송한다
  Then File Agent가 명령을 실행하고 결과를 반환한다

Scenario: BridgeRequestReply 방향에서 동기 응답
  Given Bridge Node가 BridgeRequestReply 방향으로 File Agent에 연결되었다
  When 플로우에서 {"command":"list_dir","params":{"path":"."}} 메시지를 전송한다
  Then File Agent가 디렉토리 목록을 동기적으로 응답한다
  And 응답 메시지에 파일 목록이 포함된다
```

---

## Module 3: 지정 파일 관리

### AC-FILE-020: 별칭 기반 파일 접근

```gherkin
Scenario: 별칭으로 파일을 읽는다
  Given File Agent가 target_files={"config":"etc/app.conf"}로 초기화되었다
  And 샌드박스 내에 "etc/app.conf"가 "key=value"를 포함하고 있다
  When Process({"command":"read_file","params":{"alias":"config"}})를 호출한다
  Then JSON 응답의 success는 true이다
  And data에 "key=value"가 포함된다

Scenario: 존재하지 않는 별칭 사용 시 에러
  Given File Agent가 target_files={"config":"etc/app.conf"}로 초기화되었다
  When Process({"command":"read_file","params":{"alias":"unknown"}})를 호출한다
  Then JSON 응답의 success는 false이다
  And JSON 응답의 error에 "alias not found"가 포함된다

Scenario: 별칭과 경로 동시 제공 시 경로 우선
  Given File Agent가 target_files={"config":"etc/app.conf"}로 초기화되었다
  When Process({"command":"read_file","params":{"alias":"config","path":"other.txt"}})를 호출한다
  Then "other.txt"의 내용이 반환된다 (path 우선)
```

### AC-FILE-021: 대상 파일 자동 감시

```gherkin
Scenario: target_files 디렉토리가 자동 감시된다
  Given File Agent가 target_files={"config":"etc/app.conf","data":"data/sensor.csv"}로 초기화되었다
  Then "etc/" 디렉토리와 "data/" 디렉토리에 WatchDir가 등록된다
  And fileStats.activeWatches가 2이다

Scenario: 중복 디렉토리는 한 번만 감시
  Given File Agent가 target_files={"a":"etc/a.txt","b":"etc/b.txt"}로 초기화되었다
  Then "etc/" 디렉토리에 WatchDir가 1회만 등록된다
```

### AC-FILE-022: 샌드박스 외부 경로 거부

```gherkin
Scenario: target_files에 샌드박스 외부 경로가 있으면 Init 실패
  Given 샌드박스 루트가 "/sandbox"이다
  When File Agent를 target_files={"bad":"../../etc/passwd"}로 초기화한다
  Then Init은 ErrPathOutsideSandbox 에러를 반환한다
  And File Agent는 초기화되지 않는다
```

---

## Module 4: 파일 이벤트 알림 via Bridge

### AC-FILE-030: 이벤트 메시지 포맷

```gherkin
Scenario: 파일 생성 이벤트가 올바른 포맷으로 전달된다
  Given File Agent가 초기화되고 WatchDir이 등록되었다
  When 감시 디렉토리에 "new_file.txt"가 생성된다
  Then 이벤트 메시지의 event 필드는 "create"이다
  And path 필드는 샌드박스 기준 상대 경로이다
  And timestamp 필드는 ISO 8601 포맷이다

Scenario: target_files에 등록된 파일의 이벤트에 alias 포함
  Given File Agent가 target_files={"config":"etc/app.conf"}로 초기화되었다
  And "etc/" 디렉토리가 감시 중이다
  When "etc/app.conf"가 수정된다
  Then 이벤트 메시지에 alias="config" 필드가 포함된다

Scenario: target_files에 없는 파일의 이벤트에는 alias 미포함
  Given File Agent가 target_files={"config":"etc/app.conf"}로 초기화되었다
  And "etc/" 디렉토리가 감시 중이다
  When "etc/other.txt"가 수정된다
  Then 이벤트 메시지에 alias 필드가 포함되지 않는다
```

### AC-FILE-031: 이벤트 필터링

```gherkin
Scenario: watch_events 필터로 특정 이벤트만 전달
  Given File Agent가 watch_events=["create","remove"]로 초기화되었다
  When 파일 생성 이벤트가 발생한다
  Then 이벤트가 채널에 전달된다

  When 파일 수정(write) 이벤트가 발생한다
  Then 이벤트가 필터링되어 채널에 전달되지 않는다

Scenario: watch_events 미설정 시 모든 이벤트 전달
  Given File Agent가 watch_events 설정 없이 초기화되었다
  When create, write, remove 이벤트가 순서대로 발생한다
  Then 3개 모두 채널에 전달된다
```

---

## Module 5: 에러 타입

### AC-FILE-040: 새로운 에러 타입 존재 확인

```gherkin
Scenario: 모든 확장 에러 타입이 정의되어 있다
  Given file_errors.go 파일을 검사한다
  Then ErrUnsupportedCommand 센티넬 에러가 정의되어 있다
  And ErrAliasNotFound 센티넬 에러가 정의되어 있다
  And ErrInvalidCommandFormat 센티넬 에러가 정의되어 있다
  And ErrTextModeRequired 센티넬 에러가 정의되어 있다
  And ErrLineOutOfRange 센티넬 에러가 정의되어 있다

Scenario: 에러 메시지에 "system/file:" 접두사가 포함된다
  Given 각 에러 타입을 확인한다
  Then 모든 에러의 Error() 문자열은 "system/file:"로 시작한다
```

### AC-FILE-041: 에러 JSON 응답 포맷

```gherkin
Scenario: Process 에러가 JSON 응답으로 래핑된다
  Given File Agent가 초기화되었다
  When 잘못된 JSON 명령을 Process에 전달한다
  Then 반환값은 유효한 JSON이다
  And JSON의 success 필드는 false이다
  And JSON의 error 필드에 에러 메시지가 포함된다
  And JSON의 data 필드는 null이다
```

---

## 품질 게이트

### 커버리지

- 모든 신규 파일: 85% 이상 테스트 커버리지
- 기존 file.go 수정 부분: characterization 테스트로 회귀 방지

### 성능 기준

- Process 명령 처리 지연: 1KB 파일 기준 P95 < 5ms (디스크 I/O 제외)
- 이벤트 전달 지연: fsnotify 이벤트 수신부터 채널 전달까지 P95 < 1ms
- ReadLines: 10,000줄 파일 읽기 P95 < 50ms

### Definition of Done

- [ ] 모든 인수 시나리오 통과
- [ ] go test -race 통과 (동시성 안전)
- [ ] golangci-lint 경고 0건
- [ ] 기존 file_test.go 테스트 회귀 없음
- [ ] 컴파일 타임 인터페이스 검증 (var _ = ...)
