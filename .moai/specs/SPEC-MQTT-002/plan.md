---
id: SPEC-MQTT-002
type: plan
version: "1.0.0"
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
---

# SPEC-MQTT-002 구현 계획: MQTT Agent client_id UUID 자동 생성

## 1. 작업 분해

### Primary Goal: 기본 client_id를 UUID 기반으로 변경

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | `parseMQTTSubscriberConfig()` 기본값 변경 | `internal/agent/system/mqtt_subscriber.go` | High |
| 2 | `uuid` 패키지 import 추가 | `internal/agent/system/mqtt_subscriber.go` | High |
| 3 | 자동 생성 client_id 로깅 추가 | `internal/agent/system/mqtt_subscriber.go` | Medium |
| 4 | 기존 테스트 업데이트 | `internal/agent/system/mqtt_subscriber_test.go` | High |
| 5 | 기본 client_id 관련 신규 테스트 추가 | `internal/agent/system/mqtt_subscriber_test.go` | High |

### Secondary Goal: 문서화 및 예제 업데이트

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 6 | 예제 YAML 파일 client_id 설명 보완 | `examples/agents/mqtt-sensor.yaml` (해당 시) | Low |

---

## 2. 기술 접근 방식

### 2.1 구현 변경사항

**변경 1: 기본값 생성 로직 (REQ-1)**

`parseMQTTSubscriberConfig()` 함수의 `MQTTSubscriberConfig` 초기화 블록에서:

- 변경 전: `ClientID: "xflow-mqtt-001"` (하드코딩된 정적 문자열)
- 변경 후: `ClientID: "xflow-" + uuid.New().String()` (호출 시마다 고유 UUID 생성)

**변경 2: Import 추가**

- `"github.com/google/uuid"` import 추가 (go.mod에 이미 등록됨, go.sum 변경 불필요)

**변경 3: 로깅 추가 (REQ-3)**

- `NewMQTTSubscriberAgent()` 또는 `parseMQTTSubscriberConfig()` 이후, 사용자 설정 여부에 따라 자동 생성된 client_id를 로그로 기록

### 2.2 기존 동작 보존 (REQ-2)

기존 오버라이드 로직은 변경하지 않는다:

```
if v, ok := opts["client_id"].(string); ok && v != "" {
    mc.ClientID = v
}
```

이 코드 블록은 그대로 유지되므로, 사용자가 `client_id`를 설정한 경우 자동 생성된 값을 덮어쓴다.

---

## 3. 테스트 전략

### 3.1 단위 테스트

| 테스트 케이스 | 검증 내용 | 유형 |
|--------------|----------|------|
| `TestParseMQTTSubscriberConfig_DefaultClientID` | client_id 미설정 시 `"xflow-"` 접두사로 시작하는 UUID 형식 ID 생성 확인 | 신규 |
| `TestParseMQTTSubscriberConfig_UniqueClientID` | 연속 호출 시 서로 다른 client_id 생성 확인 | 신규 |
| `TestParseMQTTSubscriberConfig_CustomClientID` | 사용자 설정 client_id가 우선 적용되는지 확인 | 기존 업데이트 |
| `TestParseMQTTSubscriberConfig_UUIDFormat` | 생성된 ID가 `"xflow-{valid-uuid}"` 형식인지 검증 | 신규 |

### 3.2 테스트 접근법

- **DDD 방식 (ANALYZE-PRESERVE-IMPROVE)**: 기존 테스트를 먼저 확인하고, 동작 보존 테스트를 추가한 후, 구현 변경
- 기존 `client_id` 관련 테스트가 하드코딩된 `"xflow-mqtt-001"` 값을 검증하는 경우, UUID 패턴 매칭으로 변경
- `uuid.MustParse()`를 사용하여 UUID 유효성 검증

---

## 4. 위험 분석

| 위험 | 영향도 | 확률 | 대응 |
|------|--------|------|------|
| MQTT 브로커가 긴 client_id를 거부 | 중간 | 낮음 | MQTT 3.1.1 이상에서 긴 ID 지원, 대부분 브로커 호환 |
| 에이전트 재시작 시 세션 복구 불가 | 낮음 | 중간 | CleanSession=true가 기본값이므로 세션 복구 불필요 |
| 기존 테스트의 하드코딩된 기댓값 실패 | 낮음 | 높음 | 테스트를 패턴 매칭 방식으로 업데이트 |

---

## 5. 마일스톤

### Primary Goal

- [ ] `parseMQTTSubscriberConfig()` 기본값을 UUID 기반으로 변경
- [ ] import 추가
- [ ] 기존 테스트 업데이트 및 신규 테스트 추가
- [ ] 전체 테스트 통과 확인 (`go test -race ./internal/agent/system/...`)

### Secondary Goal

- [ ] 자동 생성 client_id 로깅 추가
- [ ] 예제 파일 문서화 보완 (해당 시)

---

## 6. 추적성

| 요구사항 | 작업 | 마일스톤 |
|----------|------|----------|
| REQ-1 | 작업 1, 2 | Primary Goal |
| REQ-2 | (기존 동작 유지, 변경 불필요) | Primary Goal |
| REQ-3 | 작업 3 | Secondary Goal |
