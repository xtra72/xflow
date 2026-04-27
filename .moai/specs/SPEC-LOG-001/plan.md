---
id: SPEC-LOG-001
type: plan
version: "2.0.0"
spec_ref: SPEC-LOG-001
status: draft
---

# SPEC-LOG-001 v2.0.0 구현 계획

## 1. 구현 전략 개요

### 1.1 변경 범위

v2.0.0은 기존 ConsoleLoggerAgent에 두 가지 기능을 추가하는 **증분 개선**이다:

1. **content_mode 설정**: 메시지 전체 또는 payload만 출력하는 옵션
2. **binary 출력 포맷**: hex dump 형식의 바이너리 데이터 출력

v1.0.0의 LoggerAgent(internal/observe 래핑)는 변경하지 않는다.

### 1.2 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 기존 파일 수정이므로 DDD(ANALYZE-PRESERVE-IMPROVE) 적용
- 기존 테스트가 존재하는 경우 characterization test로 현재 동작 보존 확인
- 신규 기능에 대해서는 TDD(RED-GREEN-REFACTOR) 적용
- 85%+ 테스트 커버리지 유지
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.3 기술 스택

- **언어**: Go 1.23+
- **추가 표준 라이브러리**: `encoding/json` (payload 추출), `encoding/hex` (참조용), `fmt` (hex dump 포맷)
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **프론트엔드**: TypeScript (React, agentTypeMeta.ts / agentSchemas.ts)

### 1.4 수정 대상 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/agent/system/console_logger.go` | 수정 | ConsoleLoggerConfig 확장, extractContent/formatHexDump 추가, Process/PublishMessage 수정 |
| `internal/agent/system/console_logger_test.go` | 수정/생성 | content_mode 및 binary 포맷 테스트 추가 |
| `web/src/pages/agents/agentTypeMeta.ts` | 수정 | logger 설정 필드에 content_mode, format 옵션 추가 |
| `web/src/config/agentSchemas.ts` | 수정 | CONSOLE_LOGGER_FIELDS에 content_mode, binary 옵션 추가 |

---

## 2. 마일스톤

### Milestone 1: 기존 동작 보존 확인 (Primary Goal)

**목표**: v1.0.0 동작이 변경되지 않음을 보장

- [ ] 기존 console_logger_test.go 실행하여 모든 테스트 통과 확인
- [ ] characterization test 보강: Process()와 PublishMessage()의 현재 출력 형태 캡처
- [ ] `go test -race ./internal/agent/system/...` 통과 확인

### Milestone 2: content_mode 구현 (Primary Goal)

**목표**: "full" / "payload" 콘텐츠 모드 동작

- [ ] ConsoleLoggerConfig에 ContentMode 필드 추가
- [ ] parseConsoleLoggerConfig에서 content_mode 파싱 로직 추가
- [ ] extractContent 헬퍼 함수 구현
  - "full": 원본 반환
  - "payload": JSON 파싱 후 payload 키 추출, 실패 시 원본 반환
- [ ] Process() 수정: extractContent 적용
- [ ] PublishMessage() 수정: extractContent 적용
- [ ] Configure() 동작 확인: content_mode 변경 시 즉시 반영
- [ ] 테스트 작성:
  - content_mode="full" 기본 동작 (기존과 동일)
  - content_mode="payload" + 유효 JSON (payload 추출)
  - content_mode="payload" + 비JSON (fallback)
  - content_mode="payload" + JSON이지만 payload 키 없음 (fallback)
  - content_mode 미지정 시 기본값 "full"

### Milestone 3: binary 출력 포맷 구현 (Primary Goal)

**목표**: hex dump 형식의 바이너리 출력

- [ ] formatHexDump 함수 구현
  - 16바이트씩 행 분할
  - 오프셋(8자리 hex) + hex 바이트(두 그룹) + ASCII 표현
  - 비출력 문자는 '.' 대체
  - prefix 포함
- [ ] Process() 수정: format="binary" 시 slog 대신 formatHexDump 사용
- [ ] PublishMessage() 수정: format="binary" 시 hex dump 출력
  - topic="" (기본 로거): hex dump를 writer에 직접 기록
  - topic=filepath: hex dump를 파일에 기록
- [ ] binary + content_mode 조합 동작 확인
- [ ] 빈 데이터/nil 처리 확인 (패닉 방지)
- [ ] 테스트 작성:
  - format="binary" 기본 hex dump 출력
  - format="binary" + 16바이트 미만 데이터
  - format="binary" + 정확히 16바이트 데이터
  - format="binary" + 16바이트 초과 (다중 행)
  - format="binary" + 빈 데이터
  - format="binary" + content_mode="payload" 조합
  - format="binary" + PublishMessage(filepath) 파일 기록

### Milestone 4: Web UI 업데이트 (Secondary Goal)

**목표**: 프론트엔드 설정 UI에 새 옵션 노출

- [ ] agentTypeMeta.ts: logger configFields에 content_mode 추가, format description 갱신
- [ ] agentSchemas.ts: CONSOLE_LOGGER_FIELDS에 content_mode 필드 추가, format options에 'binary' 추가
- [ ] configExample 업데이트

### Milestone 5: 통합 검증 (Final Goal)

**목표**: 전체 시스템 정합성 확인

- [ ] 전체 테스트 스위트 실행: `go test -race ./internal/agent/system/...`
- [ ] 커버리지 85%+ 유지 확인
- [ ] `go vet ./...` 통과 확인
- [ ] 프론트엔드 빌드 확인

---

## 3. 기술적 접근

### 3.1 extractContent 구현 전략

```
extractContent(data []byte) []byte:
    if contentMode != "payload":
        return data

    var msg map[string]any
    if json.Unmarshal(data, &msg) != nil:
        return data  // JSON 파싱 실패 -> fallback

    payload, ok := msg["payload"]
    if !ok:
        return data  // payload 키 없음 -> fallback

    switch v := payload.(type):
    case string:
        return []byte(v)
    default:
        bytes, err := json.Marshal(v)
        if err != nil:
            return data
        return bytes
```

핵심 설계 결정:
- `encoding/json`의 표준 Unmarshal 사용 (성능이 중요하지 않은 로깅 경로)
- fallback 동작으로 graceful degradation 보장
- `map[string]any`로 유연한 JSON 구조 처리

### 3.2 formatHexDump 구현 전략

- `encoding/hex.Dump()`을 참조하되, prefix 포함 및 커스텀 포맷을 위해 자체 구현
- `strings.Builder`로 효율적 문자열 빌드
- 16바이트 단위 행 처리, 마지막 행은 패딩으로 정렬
- ASCII 범위(0x20~0x7e) 외 문자는 `.`으로 대체

### 3.3 slog 우회 전략

format="binary" 시 Process/PublishMessage에서:
- slog.Logger를 사용하지 않고 `a.writer`에 직접 hex dump 문자열을 기록
- 에이전트 메타 로깅(Start/Stop/Configure 등)은 여전히 slog를 통해 출력
- createLogger는 binary 포맷에서도 text handler 기반 slog를 생성 (메타 로깅용)

### 3.4 하위 호환성 보장

- content_mode 미지정 시 기본값 "full" -> 기존 동작 완전 동일
- format 미지정 시 기본값 "text" -> 기존 동작 완전 동일
- 기존 테스트 100% 통과 필수

---

## 4. 리스크 및 대응

| 리스크 | 영향 | 대응 방안 |
|--------|------|----------|
| JSON 파싱 성능 저하 | content_mode="payload" 시 매 메시지마다 JSON 파싱 | 로깅 에이전트는 성능 크리티컬 경로가 아니므로 허용. 필요시 lazy 파싱 최적화 |
| hex dump 대용량 데이터 | 큰 바이너리 메시지의 hex dump가 과도한 출력 생성 | 현재 스코프에서는 제한 없이 전체 출력. 향후 max_dump_size 옵션 고려 |
| 동시성 이슈 | contentMode 필드 접근 시 race condition | Configure()에서 mu.Lock으로 이미 보호됨. Process/PublishMessage에서 mu.RLock 하에 접근 |
| Web UI 호환성 | 새 필드가 기존 UI를 깨뜨릴 가능성 | 모든 새 필드에 기본값 설정, required: false |

---

## 5. 전문가 상담 권고

### Backend Expert 상담 권고

본 SPEC은 Go 백엔드 구현(ConsoleLoggerAgent 수정)을 포함하므로, 구현 시 **expert-backend** 상담을 권고한다:
- hex dump 포맷 함수의 최적 구현 방식
- JSON 파싱 fallback 전략의 edge case 검증
- 동시성 안전 관점에서 contentMode 필드 접근 패턴 검토

### Frontend Expert 상담 권고

Web UI 변경(agentTypeMeta.ts, agentSchemas.ts)이 포함되므로, **expert-frontend** 상담을 권고한다:
- select 필드의 옵션 추가가 기존 폼 레이아웃에 미치는 영향
- content_mode 필드의 적절한 UI 배치
