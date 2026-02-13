---
id: SPEC-SYSAGENT-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-SYSAGENT-001
---

# SPEC-SYSAGENT-001 수락 기준

## Module 1: Event Agent

### AC-SYSAGENT-001-01: EventAgent SystemAgent 인터페이스 준수

```gherkin
Given EventAgent 인스턴스가 생성되었을 때
Then EventAgent는 SystemAgent 인터페이스를 구현해야 한다
And IsSystem()이 true를 반환해야 한다
And RequiresTransport()이 false를 반환해야 한다
```

### AC-SYSAGENT-001-02: 이벤트 발행 및 구독

```gherkin
Given EventAgent가 시작된 상태에서
And "flow.started" 토픽에 핸들러가 구독되어 있을 때
When Emit("flow.started", "flow-001")을 호출하면
Then 구독된 핸들러가 1회 호출되어야 한다
And Event의 Topic이 "flow.started"여야 한다
And Event의 Data가 "flow-001"이어야 한다
And Event의 Timestamp가 현재 시각에 근접해야 한다
```

### AC-SYSAGENT-001-03: 다중 구독자 이벤트 전달

```gherkin
Given EventAgent가 시작된 상태에서
And "agent.connected" 토픽에 핸들러 A와 핸들러 B가 각각 구독되어 있을 때
When Emit("agent.connected", "agent-mqtt-01")을 호출하면
Then 핸들러 A가 1회 호출되어야 한다
And 핸들러 B가 1회 호출되어야 한다
```

### AC-SYSAGENT-001-04: 와일드카드 패턴 구독 (단일 레벨)

```gherkin
Given EventAgent가 시작된 상태에서
And "flow.*" 패턴으로 핸들러가 구독되어 있을 때
When Emit("flow.started", data)를 호출하면
Then 핸들러가 호출되어야 한다
When Emit("flow.stopped", data)를 호출하면
Then 핸들러가 호출되어야 한다
When Emit("flow.sub.event", data)를 호출하면
Then 핸들러가 호출되지 않아야 한다 (단일 레벨이므로)
```

### AC-SYSAGENT-001-05: 와일드카드 패턴 구독 (다중 레벨)

```gherkin
Given EventAgent가 시작된 상태에서
And "agent.**" 패턴으로 핸들러가 구독되어 있을 때
When Emit("agent.connected", data)를 호출하면
Then 핸들러가 호출되어야 한다
When Emit("agent.mqtt.error", data)를 호출하면
Then 핸들러가 호출되어야 한다
When Emit("flow.started", data)를 호출하면
Then 핸들러가 호출되지 않아야 한다
```

### AC-SYSAGENT-001-06: 구독 해제

```gherkin
Given EventAgent가 시작된 상태에서
And "flow.started" 토픽에 핸들러가 구독되어 subID를 받았을 때
When Unsubscribe(subID)를 호출하면
Then nil error를 반환해야 한다
When 이후 Emit("flow.started", data)를 호출하면
Then 해제된 핸들러가 호출되지 않아야 한다
```

### AC-SYSAGENT-001-07: 존재하지 않는 구독 해제

```gherkin
Given EventAgent가 시작된 상태에서
When Unsubscribe("non-existent-id")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrSubscriptionNotFound)가 true여야 한다
```

### AC-SYSAGENT-001-08: 빈 토픽 발행 거부

```gherkin
Given EventAgent가 시작된 상태에서
When Emit("", data)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrTopicEmpty)가 true여야 한다
```

### AC-SYSAGENT-001-09: nil 핸들러 구독 거부

```gherkin
Given EventAgent가 시작된 상태에서
When Subscribe("flow.started", nil)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrHandlerNil)가 true여야 한다
```

### AC-SYSAGENT-001-10: 이벤트 버퍼 오버플로우

```gherkin
Given EventAgent의 버퍼 크기가 2로 설정되어 있을 때
And 느린 구독자가 등록되어 이벤트를 소비하지 않고 있을 때
When 3개의 이벤트를 연속 발행하면
Then 적어도 1개의 이벤트가 드롭되어야 한다
And EventAgent의 Stats()에서 EventsDropped가 0보다 커야 한다
```

### AC-SYSAGENT-001-11: EventAgent 통계

```gherkin
Given EventAgent가 시작된 상태에서
And "test" 토픽에 핸들러가 구독되어 있을 때
When Emit("test", data)를 3회 호출하면
Then Stats()에서 EventsEmitted가 3이어야 한다
And Stats()에서 EventsDelivered가 3이어야 한다
And Stats()에서 ActiveSubscriptions가 1이어야 한다
```

### AC-SYSAGENT-001-12: EventAgent 동시성 안전

```gherkin
Given EventAgent가 시작된 상태에서
When 10개의 goroutine이 동시에 Subscribe(), Emit(), Unsubscribe()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 모든 작업이 정상적으로 완료되어야 한다
```

---

## Module 2: Store Agent

### AC-SYSAGENT-001-13: StoreAgent SystemAgent 인터페이스 준수

```gherkin
Given StoreAgent 인스턴스가 생성되었을 때
Then StoreAgent는 SystemAgent 인터페이스를 구현해야 한다
And IsSystem()이 true를 반환해야 한다
And RequiresTransport()이 false를 반환해야 한다
```

### AC-SYSAGENT-001-14: 기본 키-값 Set/Get

```gherkin
Given StoreAgent가 시작된 상태에서
When Set("default", "key1", "value1")을 호출하면
Then nil error를 반환해야 한다
When Get("default", "key1")을 호출하면
Then ("value1", true)를 반환해야 한다
```

### AC-SYSAGENT-001-15: 존재하지 않는 키 Get

```gherkin
Given StoreAgent가 시작된 상태에서
When Get("default", "nonexistent")을 호출하면
Then (nil, false)를 반환해야 한다
```

### AC-SYSAGENT-001-16: 키 삭제

```gherkin
Given StoreAgent에 ("default", "key1", "value1")이 저장되어 있을 때
When Delete("default", "key1")을 호출하면
Then nil error를 반환해야 한다
When Get("default", "key1")을 호출하면
Then (nil, false)를 반환해야 한다
```

### AC-SYSAGENT-001-17: 존재하지 않는 키 삭제

```gherkin
Given StoreAgent가 시작된 상태에서
When Delete("default", "nonexistent")을 호출하면
Then nil error를 반환해야 한다 (에러 없이 무시)
```

### AC-SYSAGENT-001-18: 네임스페이스 격리

```gherkin
Given StoreAgent에 ("ns1", "key1", "value_a")와 ("ns2", "key1", "value_b")가 저장되어 있을 때
When Get("ns1", "key1")을 호출하면
Then ("value_a", true)를 반환해야 한다
When Get("ns2", "key1")을 호출하면
Then ("value_b", true)를 반환해야 한다
```

### AC-SYSAGENT-001-19: 빈 네임스페이스 기본값

```gherkin
Given StoreAgent가 시작된 상태에서
When Set("", "key1", "value1")을 호출하면
Then nil error를 반환해야 한다
When Get("default", "key1")을 호출하면
Then ("value1", true)를 반환해야 한다 (빈 문자열은 "default"로 매핑)
```

### AC-SYSAGENT-001-20: 잘못된 네임스페이스명 거부

```gherkin
Given StoreAgent가 시작된 상태에서
When Set("invalid namespace!", "key1", "value1")을 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrNamespaceInvalid)가 true여야 한다
```

### AC-SYSAGENT-001-21: TTL 만료

```gherkin
Given StoreAgent가 시작된 상태에서
When SetWithTTL("default", "temp_key", "temp_value", 100*time.Millisecond)을 호출하면
Then nil error를 반환해야 한다
When 즉시 Get("default", "temp_key")을 호출하면
Then ("temp_value", true)를 반환해야 한다
When 200ms 후 Get("default", "temp_key")을 호출하면
Then (nil, false)를 반환해야 한다 (만료됨)
```

### AC-SYSAGENT-001-22: 음수 TTL 거부

```gherkin
Given StoreAgent가 시작된 상태에서
When SetWithTTL("default", "key1", "value1", -1*time.Second)을 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrTTLNegative)가 true여야 한다
```

### AC-SYSAGENT-001-23: Watch 변경 감시

```gherkin
Given StoreAgent가 시작된 상태에서
And ("default", "watched_key") 키에 Watch가 등록되어 있을 때
When Set("default", "watched_key", "new_value")를 호출하면
Then WatchHandler가 호출되어야 한다
And WatchEvent의 Type이 WatchEventSet이어야 한다
And WatchEvent의 NewValue가 "new_value"여야 한다
```

### AC-SYSAGENT-001-24: Watch TTL 만료 이벤트

```gherkin
Given StoreAgent가 시작된 상태에서
And ("default", "ttl_key") 키에 Watch가 등록되어 있을 때
When SetWithTTL("default", "ttl_key", "value", 100*time.Millisecond)을 호출하고 200ms 대기하면
Then WatchHandler가 호출되어야 한다
And WatchEvent의 Type이 WatchEventExpire여야 한다
```

### AC-SYSAGENT-001-25: Keys 목록 조회

```gherkin
Given StoreAgent에 ("ns1", "a", 1), ("ns1", "b", 2), ("ns1", "c", 3)이 저장되어 있을 때
When Keys("ns1")을 호출하면
Then ["a", "b", "c"]를 반환해야 한다 (순서 무관)
And 길이가 3이어야 한다
```

### AC-SYSAGENT-001-26: Clear 네임스페이스 전체 삭제

```gherkin
Given StoreAgent에 ("ns1", "a", 1), ("ns1", "b", 2)가 저장되어 있을 때
When Clear("ns1")을 호출하면
Then Keys("ns1")이 빈 슬라이스를 반환해야 한다
```

### AC-SYSAGENT-001-27: StoreAgent 통계

```gherkin
Given StoreAgent가 시작된 상태에서
When Set("default", "k1", "v1") 후 Get("default", "k1"), Get("default", "missing")을 호출하면
Then Stats()에서 TotalKeys가 1이어야 한다
And Stats()에서 GetHits가 1이어야 한다
And Stats()에서 GetMisses가 1이어야 한다
```

### AC-SYSAGENT-001-28: StoreAgent 동시성 안전

```gherkin
Given StoreAgent가 시작된 상태에서
When 10개의 goroutine이 동시에 Set(), Get(), Delete()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 모든 작업이 정상적으로 완료되어야 한다
```

---

## Module 3: Timer Agent

### AC-SYSAGENT-001-29: TimerAgent SystemAgent 인터페이스 준수

```gherkin
Given TimerAgent 인스턴스가 생성되었을 때
Then TimerAgent는 SystemAgent 인터페이스를 구현해야 한다
And IsSystem()이 true를 반환해야 한다
And RequiresTransport()이 false를 반환해야 한다
```

### AC-SYSAGENT-001-30: Interval 타이머

```gherkin
Given TimerAgent가 시작된 상태에서
When SetInterval("test", 200*time.Millisecond, handler)를 호출하면
Then nil error를 반환하고 TimerID를 반환해야 한다
When 500ms 대기하면
Then handler가 2~3회 호출되어야 한다
And TimerTrigger의 TickCount가 순차적으로 증가해야 한다
```

### AC-SYSAGENT-001-31: Interval 최소 간격 위반

```gherkin
Given TimerAgent가 시작된 상태에서
When SetInterval("test", 50*time.Millisecond, handler)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrIntervalTooShort)가 true여야 한다
```

### AC-SYSAGENT-001-32: Cron 스케줄

```gherkin
Given TimerAgent가 시작된 상태에서
When SetCron("test", "*/1 * * * *", handler)를 호출하면
Then nil error를 반환하고 TimerID를 반환해야 한다
And List()에 해당 타이머가 포함되어야 한다
And TimerInfo의 Type이 TimerTypeCron이어야 한다
```

### AC-SYSAGENT-001-33: 잘못된 Cron 표현식

```gherkin
Given TimerAgent가 시작된 상태에서
When SetCron("test", "invalid cron", handler)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidCronExpression)가 true여야 한다
```

### AC-SYSAGENT-001-34: Timeout 1회 실행

```gherkin
Given TimerAgent가 시작된 상태에서
When SetTimeout("test", 100*time.Millisecond, handler)를 호출하면
Then nil error를 반환하고 TimerID를 반환해야 한다
When 200ms 대기하면
Then handler가 정확히 1회 호출되어야 한다
And 추가 대기 후에도 더 이상 호출되지 않아야 한다
```

### AC-SYSAGENT-001-35: 타이머 취소

```gherkin
Given TimerAgent에 200ms 간격 Interval 타이머가 등록되어 있을 때
When Cancel(timerID)를 호출하면
Then nil error를 반환해야 한다
When 500ms 대기하면
Then handler가 더 이상 호출되지 않아야 한다
```

### AC-SYSAGENT-001-36: 존재하지 않는 타이머 취소

```gherkin
Given TimerAgent가 시작된 상태에서
When Cancel("non-existent-timer")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrTimerNotFound)가 true여야 한다
```

### AC-SYSAGENT-001-37: 빈 타이머 ID 거부

```gherkin
Given TimerAgent가 시작된 상태에서
When SetInterval("", 1*time.Second, handler)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrTimerIDEmpty)가 true여야 한다
```

### AC-SYSAGENT-001-38: Graceful Shutdown

```gherkin
Given TimerAgent에 여러 Interval/Cron/Timeout 타이머가 등록되어 있을 때
When Stop(ctx)를 호출하면
Then 모든 타이머가 취소되어야 한다
And List()가 빈 슬라이스를 반환해야 한다
And 이후 handler가 호출되지 않아야 한다
```

### AC-SYSAGENT-001-39: TimerAgent 통계

```gherkin
Given TimerAgent에 Interval 1개, Cron 1개가 등록되어 있을 때
Then Stats()에서 ActiveTimers가 2여야 한다
And Stats()에서 IntervalTimers가 1이어야 한다
And Stats()에서 CronTimers가 1이어야 한다
```

---

## Module 4: Error Types

### AC-SYSAGENT-001-40: Sentinel 에러 errors.Is() 호환성

```gherkin
Given ErrSubscriptionNotFound, ErrTopicEmpty, ErrHandlerNil, ErrInvalidPattern, ErrBufferFull,
    ErrNamespaceInvalid, ErrKeyEmpty, ErrWatchNotFound, ErrTTLNegative,
    ErrIntervalTooShort, ErrInvalidCronExpression, ErrTimerNotFound, ErrTimerIDEmpty,
    ErrPathOutsideSandbox, ErrSandboxNotConfigured, ErrFileNotFound, ErrWatchPathInvalid,
    ErrInvalidLogLevel, ErrComponentEmpty, ErrStreamClosed,
    ErrAgentTypeUnknown, ErrAlreadyInitialized, ErrNotInitialized 각각에 대해
When errors.Is(err, sentinel)를 호출하면
Then 동일한 sentinel 에러와 비교 시 true를 반환해야 한다
And 다른 sentinel 에러와 비교 시 false를 반환해야 한다
```

### AC-SYSAGENT-001-41: Sentinel 에러 fmt.Errorf 래핑 호환성

```gherkin
Given ErrSubscriptionNotFound를 fmt.Errorf("wrapper: %w", ErrSubscriptionNotFound)로 래핑했을 때
When errors.Is(wrappedErr, ErrSubscriptionNotFound)를 호출하면
Then true를 반환해야 한다
```

---

## Module 5: File Agent

### AC-SYSAGENT-001-42: FileAgent SystemAgent 인터페이스 준수

```gherkin
Given FileAgent 인스턴스가 생성되었을 때
Then FileAgent는 SystemAgent 인터페이스를 구현해야 한다
And IsSystem()이 true를 반환해야 한다
And RequiresTransport()이 false를 반환해야 한다
```

### AC-SYSAGENT-001-43: 파일 쓰기 및 읽기

```gherkin
Given FileAgent가 시작되고 샌드박스 루트가 설정되어 있을 때
When Write("/sandbox/test.txt", []byte("hello"), 0644)를 호출하면
Then nil error를 반환해야 한다
When Read("/sandbox/test.txt")를 호출하면
Then []byte("hello")를 반환해야 한다
And nil error를 반환해야 한다
```

### AC-SYSAGENT-001-44: 파일 Append

```gherkin
Given FileAgent의 샌드박스에 "hello" 내용의 파일이 있을 때
When Append("/sandbox/test.txt", []byte(" world"))를 호출하면
Then nil error를 반환해야 한다
When Read("/sandbox/test.txt")를 호출하면
Then []byte("hello world")를 반환해야 한다
```

### AC-SYSAGENT-001-45: 샌드박스 외부 접근 거부

```gherkin
Given FileAgent의 샌드박스 루트가 "/sandbox/"일 때
When Read("/etc/passwd")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrPathOutsideSandbox)가 true여야 한다
```

### AC-SYSAGENT-001-46: 상대 경로 샌드박스 우회 거부

```gherkin
Given FileAgent의 샌드박스 루트가 "/sandbox/"일 때
When Read("/sandbox/../etc/passwd")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrPathOutsideSandbox)가 true여야 한다
```

### AC-SYSAGENT-001-47: 심볼릭 링크 샌드박스 검증

```gherkin
Given FileAgent의 샌드박스 내에 /sandbox/link -> /etc/ 심볼릭 링크가 있을 때
When Read("/sandbox/link/passwd")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrPathOutsideSandbox)가 true여야 한다
```

### AC-SYSAGENT-001-48: 존재하지 않는 파일 읽기

```gherkin
Given FileAgent의 샌드박스에 "nonexistent.txt"가 없을 때
When Read("/sandbox/nonexistent.txt")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrFileNotFound)가 true여야 한다
```

### AC-SYSAGENT-001-49: 디렉토리 감시

```gherkin
Given FileAgent가 시작된 상태에서
And WatchDir("/sandbox/watched/", handler)로 감시가 등록되어 있을 때
When "/sandbox/watched/new_file.txt" 파일을 생성하면
Then handler가 호출되어야 한다
And FileEvent의 Op가 FileOpCreate여야 한다
And FileEvent의 Path에 "new_file.txt"가 포함되어야 한다
```

### AC-SYSAGENT-001-50: 파일 존재 여부 확인

```gherkin
Given FileAgent의 샌드박스에 "exists.txt" 파일이 있을 때
When Exists("/sandbox/exists.txt")를 호출하면
Then true를 반환해야 한다
When Exists("/sandbox/not_exists.txt")를 호출하면
Then false를 반환해야 한다
```

### AC-SYSAGENT-001-51: FileAgent 통계

```gherkin
Given FileAgent가 시작된 상태에서
When Write("/sandbox/test.txt", []byte("hello"), 0644) 후 Read("/sandbox/test.txt")를 호출하면
Then Stats()에서 FilesWritten가 1이어야 한다
And Stats()에서 FilesRead가 1이어야 한다
And Stats()에서 BytesWritten가 5여야 한다
And Stats()에서 BytesRead가 5여야 한다
```

---

## Module 6: Logger Agent

### AC-SYSAGENT-001-52: LoggerAgent SystemAgent 인터페이스 준수

```gherkin
Given LoggerAgent 인스턴스가 생성되었을 때
Then LoggerAgent는 SystemAgent 인터페이스를 구현해야 한다
And IsSystem()이 true를 반환해야 한다
And RequiresTransport()이 false를 반환해야 한다
```

### AC-SYSAGENT-001-53: 로그 작성

```gherkin
Given LoggerAgent가 시작된 상태에서
And "my-component"의 로그 레벨이 LogLevelInfo일 때
When Info("my-component", "test message", "key", "value")를 호출하면
Then slog에 Info 레벨 로그가 기록되어야 한다
When Debug("my-component", "debug message")를 호출하면
Then slog에 Debug 레벨 로그가 기록되지 않아야 한다 (현재 레벨 이하)
```

### AC-SYSAGENT-001-54: 로그 레벨 동적 변경

```gherkin
Given LoggerAgent가 시작된 상태에서
And "my-component"의 로그 레벨이 LogLevelInfo일 때
When SetLevel("my-component", LogLevelDebug)를 호출하면
Then nil error를 반환해야 한다
And GetLevel("my-component")이 LogLevelDebug를 반환해야 한다
When Debug("my-component", "now visible")를 호출하면
Then slog에 Debug 레벨 로그가 기록되어야 한다
```

### AC-SYSAGENT-001-55: 잘못된 로그 레벨 거부

```gherkin
Given LoggerAgent가 시작된 상태에서
When SetLevel("my-component", LogLevel("invalid"))를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidLogLevel)가 true여야 한다
```

### AC-SYSAGENT-001-56: 빈 컴포넌트명 거부

```gherkin
Given LoggerAgent가 시작된 상태에서
When Log("", LogLevelInfo, "test")를 호출하면
Then error가 nil이 아니어야 한다 (또는 내부적으로 기본 컴포넌트명 사용)
```

### AC-SYSAGENT-001-57: 로그 스트림 구독

```gherkin
Given LoggerAgent가 시작된 상태에서
And 로그 스트림 핸들러가 Subscribe()로 등록되어 있을 때
When Info("my-component", "stream test")를 호출하면
Then 스트림 핸들러가 호출되어야 한다
And LogEntry의 Component가 "my-component"여야 한다
And LogEntry의 Level이 LogLevelInfo여야 한다
And LogEntry의 Message가 "stream test"여야 한다
```

### AC-SYSAGENT-001-58: 로그 스트림 구독 해제

```gherkin
Given LoggerAgent에 로그 스트림이 구독되어 streamID를 받았을 때
When Unsubscribe(streamID)를 호출하면
Then nil error를 반환해야 한다
When 이후 Info("comp", "msg")를 호출하면
Then 해제된 스트림 핸들러가 호출되지 않아야 한다
```

### AC-SYSAGENT-001-59: LoggerAgent 통계

```gherkin
Given LoggerAgent가 시작된 상태에서
When Info("comp1", "msg") 2회, Error("comp2", "err") 1회 호출하면
Then Stats()에서 TotalLogs가 3이어야 한다
And Stats()에서 LogsByLevel[LogLevelInfo]가 2여야 한다
And Stats()에서 LogsByLevel[LogLevelError]가 1이어야 한다
And Stats()에서 Components가 2여야 한다
```

---

## Module 7: System Agent Manager

### AC-SYSAGENT-001-60: 초기화

```gherkin
Given SystemAgentManager가 생성되었을 때
When Initialize(SystemConfig{})를 호출하면
Then nil error를 반환해야 한다
And IsInitialized()이 true를 반환해야 한다
And Event()가 nil이 아니어야 한다
And Logger()가 nil이 아니어야 한다
And File()가 nil이 아니어야 한다
And Timer()가 nil이 아니어야 한다
And Store()가 nil이 아니어야 한다
```

### AC-SYSAGENT-001-61: 중복 초기화 거부

```gherkin
Given SystemAgentManager가 이미 초기화된 상태에서
When Initialize(SystemConfig{})를 다시 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrAlreadyInitialized)가 true여야 한다
```

### AC-SYSAGENT-001-62: 일괄 시작

```gherkin
Given SystemAgentManager가 초기화된 상태에서
When Start(ctx)를 호출하면
Then nil error를 반환해야 한다
And 모든 System Agent가 Running 상태여야 한다
```

### AC-SYSAGENT-001-63: 일괄 중지

```gherkin
Given SystemAgentManager의 모든 Agent가 Running 상태에서
When Stop(ctx)를 호출하면
Then nil error를 반환해야 한다
And 모든 System Agent가 Stopped 상태여야 한다
```

### AC-SYSAGENT-001-64: 시작 실패 롤백

```gherkin
Given SystemAgentManager가 초기화되었으나 3번째 Agent(Timer)가 시작 실패하도록 설정되어 있을 때
When Start(ctx)를 호출하면
Then error가 nil이 아니어야 한다
And 이미 시작된 Event, Store Agent가 역순으로 중지되어야 한다
```

### AC-SYSAGENT-001-65: 초기화 전 접근

```gherkin
Given SystemAgentManager가 초기화되지 않은 상태에서
When Event()를 호출하면
Then nil을 반환해야 한다 (또는 ErrNotInitialized 에러)
```

---

## Module 8: System Agent Info & Stats

### AC-SYSAGENT-001-66: SystemAgentInfo 확장

```gherkin
Given EventAgent가 시작된 상태에서
When Info()를 호출하면
Then AgentInfo의 기본 필드 (ID, Name, Type, State)가 설정되어야 한다
And SystemType이 SystemAgentEvent여야 한다
And AutoStart가 true여야 한다
```

### AC-SYSAGENT-001-67: Agent별 특화 통계 포함

```gherkin
Given 모든 System Agent가 시작된 상태에서
When 각 Agent의 Stats()를 호출하면
Then EventAgent는 EventsEmitted 필드를 포함해야 한다
And StoreAgent는 TotalKeys 필드를 포함해야 한다
And TimerAgent는 ActiveTimers 필드를 포함해야 한다
And FileAgent는 FilesRead 필드를 포함해야 한다
And LoggerAgent는 TotalLogs 필드를 포함해야 한다
```

---

## 품질 게이트

### QG-SYSAGENT-001-01: 테스트 커버리지

```gherkin
Given internal/agent/system/ 패키지의 모든 소스 파일에 대해
When go test -cover ./internal/agent/system/을 실행하면
Then 테스트 커버리지가 85% 이상이어야 한다
```

### QG-SYSAGENT-001-02: Race Condition

```gherkin
Given internal/agent/system/ 패키지의 모든 테스트에 대해
When go test -race ./internal/agent/system/을 실행하면
Then data race가 감지되지 않아야 한다
```

### QG-SYSAGENT-001-03: 린트

```gherkin
Given internal/agent/system/ 패키지의 모든 소스 파일에 대해
When golangci-lint run ./internal/agent/system/을 실행하면
Then 린트 에러가 0개여야 한다
```

### QG-SYSAGENT-001-04: vet

```gherkin
Given internal/agent/system/ 패키지의 모든 소스 파일에 대해
When go vet ./internal/agent/system/을 실행하면
Then 경고가 0개여야 한다
```

### QG-SYSAGENT-001-05: TypeRegistry 등록 검증

```gherkin
Given internal/agent/system 패키지가 import 되었을 때
Then TypeRegistry에 "event", "logger", "file", "timer", "store" 5개 타입이 등록되어야 한다
And 각 타입의 HasType()이 true를 반환해야 한다
```
