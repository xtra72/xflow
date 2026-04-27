---
id: SPEC-LOG-001
type: acceptance
version: "2.0.0"
spec_ref: SPEC-LOG-001
---

# SPEC-LOG-001 v2.0.0 수락 기준

> v1.0.0 수락 기준(Module 1~5)은 이미 구현 완료 상태이다.
> 본 문서는 v2.0.0에서 추가된 Module 6(Message Content Mode)과 Module 7(Binary Output Format)의 수락 기준을 정의한다.

---

## Module 6: Message Content Mode - 메시지 콘텐츠 모드

### AC-LOG-001-06-01: content_mode 설정 파싱

```gherkin
Given ConsoleLoggerAgent의 설정에 content_mode가 지정되어 있을 때
When parseConsoleLoggerConfig(cfg)가 호출되면
Then ConsoleLoggerConfig.ContentMode에 해당 값이 설정되어야 한다
```

```gherkin
Given ConsoleLoggerAgent의 설정에 content_mode가 미지정일 때
When parseConsoleLoggerConfig(cfg)가 호출되면
Then ConsoleLoggerConfig.ContentMode의 기본값은 "full"이어야 한다
```

### AC-LOG-001-06-02: content_mode="full" 동작 (하위 호환성)

```gherkin
Given content_mode="full"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"type":"sensor","payload":"temperature=25"}'일 때
When Process(data)가 호출되면
Then 출력에 수신 데이터 전체가 포함되어야 한다
And 출력 내용이 v1.0.0의 동작과 동일해야 한다
```

### AC-LOG-001-06-03: content_mode="payload" + 유효 JSON

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"type":"sensor","payload":"temperature=25"}'일 때
When Process(data)가 호출되면
Then 출력에 "temperature=25"만 포함되어야 한다
And "type" 또는 "sensor" 문자열은 출력에 포함되지 않아야 한다
```

### AC-LOG-001-06-04: content_mode="payload" + payload가 객체인 경우

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"payload":{"temp":25,"unit":"C"}}'일 때
When Process(data)가 호출되면
Then 출력에 payload 객체의 JSON 직렬화 결과가 포함되어야 한다
And 출력에 "temp" 키와 "unit" 키가 포함되어야 한다
```

### AC-LOG-001-06-05: content_mode="payload" + 비JSON 데이터 (fallback)

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 "plain text message"일 때
When Process(data)가 호출되면
Then 에러 없이 처리되어야 한다
And 출력에 원본 데이터 "plain text message"가 포함되어야 한다
```

### AC-LOG-001-06-06: content_mode="payload" + JSON이지만 payload 키 없음 (fallback)

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"type":"sensor","value":42}'일 때
When Process(data)가 호출되면
Then 에러 없이 처리되어야 한다
And 출력에 원본 JSON 전체가 포함되어야 한다
```

### AC-LOG-001-06-07: content_mode의 PublishMessage 적용

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And topic이 빈 문자열일 때
And payload가 '{"payload":"extracted-data"}'일 때
When PublishMessage("", 0, false, payload)가 호출되면
Then 출력에 "extracted-data"만 포함되어야 한다
```

### AC-LOG-001-06-08: content_mode의 PublishMessage 파일 기록 적용

```gherkin
Given content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And topic이 "/tmp/test-output.log" 파일 경로일 때
And payload가 '{"payload":"file-data"}'일 때
When PublishMessage("/tmp/test-output.log", 0, false, payload)가 호출되면
Then 파일에 "file-data"만 기록되어야 한다
```

### AC-LOG-001-06-09: Configure를 통한 content_mode 변경

```gherkin
Given content_mode="full"로 시작된 ConsoleLoggerAgent가 주어졌을 때
When Configure()로 content_mode="payload"로 변경한 뒤
And '{"payload":"new-mode"}'를 Process()로 전달하면
Then 출력에 "new-mode"만 포함되어야 한다
```

---

## Module 7: Binary Output Format - 바이너리 출력 포맷

### AC-LOG-001-07-01: format="binary" 기본 hex dump 출력

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And prefix가 "[logger]"일 때
And 수신 데이터가 "Hello World!" (12바이트)일 때
When Process(data)가 호출되면
Then 출력에 8자리 16진수 오프셋 "00000000"이 포함되어야 한다
And 출력에 hex 값 "48 65 6c 6c 6f 20 57 6f"가 포함되어야 한다
And 출력에 hex 값 "72 6c 64 21"이 포함되어야 한다
And 출력에 ASCII 표현 "|Hello World!|"가 포함되어야 한다
And 출력에 prefix "[logger]"가 포함되어야 한다
```

### AC-LOG-001-07-02: format="binary" 다중 행 출력

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 20바이트일 때
When Process(data)가 호출되면
Then 첫 번째 행의 오프셋은 "00000000"이어야 한다
And 두 번째 행의 오프셋은 "00000010"이어야 한다
And 첫 번째 행은 정확히 16바이트의 hex 값을 포함해야 한다
And 두 번째 행은 4바이트의 hex 값을 포함해야 한다
```

### AC-LOG-001-07-03: format="binary" 비출력 문자 처리

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터에 0x00, 0x01, 0xff 등 비출력 문자가 포함되어 있을 때
When Process(data)가 호출되면
Then ASCII 표현에서 비출력 문자는 '.'으로 대체되어야 한다
And hex 값은 정확하게 표시되어야 한다
```

### AC-LOG-001-07-04: format="binary" 빈 데이터

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 빈 바이트 슬라이스([]byte{})일 때
When Process(data)가 호출되면
Then 패닉이 발생하지 않아야 한다
And 에러 없이 정상 반환해야 한다
```

### AC-LOG-001-07-05: format="binary" nil 데이터

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 nil일 때
When Process(data)가 호출되면
Then 패닉이 발생하지 않아야 한다
And 에러 없이 정상 반환해야 한다
```

### AC-LOG-001-07-06: format="binary" slog 비사용 확인

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
When Process(data)가 호출되면
Then slog의 TextHandler/JSONHandler 형식 출력이 아니어야 한다
And hex dump 형식으로 writer에 직접 기록되어야 한다
```

### AC-LOG-001-07-07: format="binary" + PublishMessage(topic="")

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And topic이 빈 문자열일 때
When PublishMessage("", 0, false, payload)가 호출되면
Then hex dump 형식으로 기본 writer에 출력되어야 한다
```

### AC-LOG-001-07-08: format="binary" + PublishMessage(topic=filepath)

```gherkin
Given format="binary"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And topic이 "/tmp/binary-output.log" 파일 경로일 때
When PublishMessage("/tmp/binary-output.log", 0, false, payload)가 호출되면
Then hex dump 형식으로 해당 파일에 기록되어야 한다
```

### AC-LOG-001-07-09: format="binary" + content_mode="payload" 조합

```gherkin
Given format="binary"이고 content_mode="payload"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"payload":"ABC"}'일 때
When Process(data)가 호출되면
Then "ABC" (3바이트)에 대한 hex dump가 출력되어야 한다
And hex 값 "41 42 43"이 포함되어야 한다
And ASCII 표현 "|ABC|"가 포함되어야 한다
And 전체 JSON이 아닌 payload 값만 hex dump 되어야 한다
```

### AC-LOG-001-07-10: format="binary" + content_mode="full" 조합

```gherkin
Given format="binary"이고 content_mode="full"로 설정된 ConsoleLoggerAgent가 주어졌을 때
And 수신 데이터가 '{"payload":"ABC"}'일 때
When Process(data)가 호출되면
Then 전체 JSON 문자열에 대한 hex dump가 출력되어야 한다
And JSON의 모든 문자에 대한 hex 값이 포함되어야 한다
```

### AC-LOG-001-07-11: format 옵션에 "binary" 추가 확인

```gherkin
Given ConsoleLoggerAgent의 설정에 format="binary"가 지정되어 있을 때
When parseConsoleLoggerConfig(cfg)가 호출되면
Then ConsoleLoggerConfig.Format에 "binary"가 설정되어야 한다
And 에이전트가 정상적으로 초기화되어야 한다
```

---

## Web UI 수락 기준

### AC-LOG-001-WEB-01: agentTypeMeta.ts 필드 추가

```gherkin
Given web/src/pages/agents/agentTypeMeta.ts 파일이 주어졌을 때
Then logger 항목의 configFields에 content_mode 필드가 존재해야 한다
And content_mode 필드의 type은 'select'이어야 한다
And content_mode 필드의 default는 'full'이어야 한다
And format 필드의 description에 'binary'가 포함되어야 한다
```

### AC-LOG-001-WEB-02: agentSchemas.ts 스키마 추가

```gherkin
Given web/src/config/agentSchemas.ts 파일이 주어졌을 때
Then CONSOLE_LOGGER_FIELDS에 content_mode 필드가 존재해야 한다
And content_mode 필드의 options에 'full'과 'payload'가 포함되어야 한다
And format 필드의 options에 'text', 'json', 'binary'가 포함되어야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] Module 6 수락 기준 전체 통과 (AC-LOG-001-06-01 ~ 06-09)
- [ ] Module 7 수락 기준 전체 통과 (AC-LOG-001-07-01 ~ 07-11)
- [ ] Web UI 수락 기준 전체 통과 (AC-LOG-001-WEB-01 ~ WEB-02)
- [ ] 기존 v1.0.0 테스트 전체 통과 (회귀 없음)
- [ ] `go test -race ./internal/agent/system/...` 통과
- [ ] 테스트 커버리지 85%+ 유지
- [ ] `go vet ./...` 경고 없음
- [ ] 프론트엔드 빌드 성공

### 검증 방법

| 검증 항목 | 도구 | 기준 |
|-----------|------|------|
| 단위 테스트 | `go test -race -cover` | 전체 통과, 커버리지 85%+ |
| 정적 분석 | `go vet` | 경고 0건 |
| 동시성 검증 | `go test -race` | 데이터 레이스 0건 |
| 하위 호환성 | 기존 테스트 스위트 | 100% 통과 |
| 프론트엔드 | TypeScript 컴파일 | 에러 0건 |
