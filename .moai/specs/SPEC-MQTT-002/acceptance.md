---
id: SPEC-MQTT-002
type: acceptance
version: "1.0.0"
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
---

# SPEC-MQTT-002 인수 기준: MQTT Agent client_id UUID 자동 생성

## 1. 인수 시나리오

### Scenario 1: client_id 미설정 시 UUID 자동 생성

```gherkin
Given MQTT 에이전트 설정에서 client_id가 지정되지 않은 경우
When parseMQTTSubscriberConfig()가 호출되면
Then 반환된 MQTTSubscriberConfig.ClientID는 "xflow-" 접두사로 시작해야 한다
And 접두사 이후의 문자열은 유효한 UUID v4 형식이어야 한다
And 전체 형식은 "xflow-{xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx}"이어야 한다
```

### Scenario 2: 사용자 설정 client_id 우선 적용

```gherkin
Given MQTT 에이전트 설정의 Transport.Options에 client_id가 "my-custom-client"로 설정된 경우
When parseMQTTSubscriberConfig()가 호출되면
Then 반환된 MQTTSubscriberConfig.ClientID는 "my-custom-client"이어야 한다
And UUID 기반 자동 생성은 적용되지 않아야 한다
```

### Scenario 3: 복수 에이전트 인스턴스의 고유 ID 생성

```gherkin
Given 두 개의 MQTT 에이전트가 client_id 설정 없이 생성되는 경우
When 각각의 parseMQTTSubscriberConfig()가 호출되면
Then 각 에이전트의 ClientID는 서로 다른 값이어야 한다
And 두 값 모두 "xflow-" 접두사와 유효한 UUID를 포함해야 한다
```

### Scenario 4: 빈 문자열 client_id 설정

```gherkin
Given MQTT 에이전트 설정의 Transport.Options에 client_id가 빈 문자열("")로 설정된 경우
When parseMQTTSubscriberConfig()가 호출되면
Then 빈 문자열은 유효한 설정으로 간주되지 않아야 한다
And UUID 기반 자동 생성된 client_id가 사용되어야 한다
```

### Scenario 5: 자동 생성된 client_id 로깅

```gherkin
Given MQTT 에이전트가 client_id 설정 없이 생성되는 경우
When 에이전트가 초기화되면
Then 자동 생성된 client_id 값이 Info 레벨 로그에 기록되어야 한다
And 로그 메시지에 생성된 client_id 전체 값이 포함되어야 한다
```

---

## 2. 엣지 케이스

### Edge Case 1: UUID 형식 검증

```gherkin
Given client_id가 자동 생성된 경우
When UUID 부분을 추출하면
Then uuid.Parse()로 파싱 가능한 유효한 UUID여야 한다
And UUID 버전은 4(랜덤 기반)이어야 한다
```

### Edge Case 2: client_id 접두사 일관성

```gherkin
Given 자동 생성된 client_id
When 값을 검사하면
Then 반드시 "xflow-" 리터럴 접두사로 시작해야 한다
And 접두사와 UUID 사이에 추가 구분자가 없어야 한다
```

---

## 3. Quality Gate 기준

### 3.1 테스트 커버리지

- `parseMQTTSubscriberConfig()` 함수의 client_id 관련 분기 커버리지 100%
- 기본값 생성 경로 + 사용자 오버라이드 경로 모두 테스트

### 3.2 검증 방법

| 검증 항목 | 방법 | 기준 |
|----------|------|------|
| UUID 형식 유효성 | `uuid.Parse()` 성공 여부 | 파싱 오류 없음 |
| 접두사 정확성 | `strings.HasPrefix(id, "xflow-")` | true |
| 고유성 | 100회 연속 생성 후 중복 검사 | 중복 없음 |
| 하위 호환성 | 사용자 설정 client_id 테스트 | 기존 값 그대로 적용 |
| 빈 문자열 처리 | 빈 문자열 설정 후 결과 검증 | UUID 기반 값 생성 |

### 3.3 Definition of Done

- [ ] 모든 인수 시나리오의 테스트가 통과한다
- [ ] `go test -race ./internal/agent/system/...` 통과한다
- [ ] 하드코딩된 `"xflow-mqtt-001"` 기본값이 코드에서 제거되었다
- [ ] `github.com/google/uuid` import가 추가되었다
- [ ] 사용자 설정 client_id 오버라이드 로직이 변경 없이 유지된다
- [ ] 자동 생성된 client_id가 로그에 기록된다

---

## 4. 추적성

| 시나리오 | 요구사항 | 구현 위치 |
|----------|----------|----------|
| Scenario 1 | REQ-1 | `parseMQTTSubscriberConfig()` 기본값 |
| Scenario 2 | REQ-2 | `opts["client_id"]` 오버라이드 로직 |
| Scenario 3 | REQ-1 | UUID 고유성 특성 |
| Scenario 4 | REQ-1, REQ-2 | 빈 문자열 가드 조건 |
| Scenario 5 | REQ-3 | 로깅 추가 |
