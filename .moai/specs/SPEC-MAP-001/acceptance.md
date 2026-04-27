---
id: SPEC-MAP-001
type: acceptance
version: "1.0.0"
created: "2026-03-11"
updated: "2026-03-11"
author: xtra
---

# SPEC-MAP-001: 수락 기준 - Mapping Node

## 1. 테스트 시나리오

### 시나리오 1: 키 매칭 성공 (정상 경로) [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.status_code |
  | mappings | {"0": "정상", "1": "경고", "2": "위험"} |
  | target   | status_text |
And 메시지의 payload.status_code 값이 "1" 이다
When Process(ctx, msg)가 호출되면
Then 반환된 메시지의 payload에 "status_text" 필드가 "경고"로 설정된다
And 에러가 반환되지 않는다
And 반환된 메시지 슬라이스 길이가 1이다
```

### 시나리오 2: 키 미발견 + 기본값 사용 [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.code   |
  | mappings | {"A": "알파", "B": "베타"} |
  | default  | 알 수 없음 |
And 메시지의 payload.code 값이 "Z" 이다
When Process(ctx, msg)가 호출되면
Then 반환된 메시지의 payload에 "code" 필드가 "알 수 없음"으로 설정된다
And 에러가 반환되지 않는다
```

### 시나리오 3: 키 미발견 + 기본값 없음 (에러) [R-MAP-003, R-MAP-006]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.code   |
  | mappings | {"A": "알파", "B": "베타"} |
And default가 설정되지 않았다
And 메시지의 payload.code 값이 "Z" 이다
When Process(ctx, msg)가 호출되면
Then ErrMappingKeyNotFound 에러가 반환된다
And 반환된 메시지 슬라이스가 nil이다
```

### 시나리오 4: 소스 필드 미발견 [R-MAP-004, R-MAP-006]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.nonexistent |
  | mappings | {"A": "알파"}         |
And 메시지의 payload에 "nonexistent" 필드가 존재하지 않는다
When Process(ctx, msg)가 호출되면
Then ErrMappingFieldNotFound 에러가 반환된다
And 반환된 메시지 슬라이스가 nil이다
```

### 시나리오 5: 숫자 키 변환 [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.level  |
  | mappings | {"42": "answer", "0": "zero"} |
And 메시지의 payload.level 값이 정수 42 (int 타입) 이다
When Process(ctx, msg)가 호출되면
Then 소스 값이 문자열 "42"로 변환되어 매핑 테이블에서 조회된다
And 반환된 메시지에 "answer" 값이 기록된다
```

### 시나리오 6: target 필드 지정 (새 필드에 기록) [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.sensor_id |
  | mappings | {"S001": "온도센서", "S002": "습도센서"} |
  | target   | sensor_name |
And 메시지의 payload.sensor_id 값이 "S001" 이다
When Process(ctx, msg)가 호출되면
Then 반환된 메시지의 payload에 "sensor_name" 필드가 "온도센서"로 설정된다
And 원본 payload.sensor_id 값 "S001"이 그대로 보존된다
```

### 시나리오 7: target 미지정 (소스 필드 덮어쓰기) [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.status |
  | mappings | {"ok": "정상", "error": "에러"} |
And target이 설정되지 않았다 (빈 문자열)
And 메시지의 payload.status 값이 "ok" 이다
When Process(ctx, msg)가 호출되면
Then 반환된 메시지의 payload.status 값이 "정상"으로 변경된다
```

### 시나리오 8: Configure 필수 필드 누락 [R-MAP-002]

```gherkin
Given MappingNode가 생성되었다
When Configure가 field 없이 호출되면:
  | mappings | {"A": "알파"} |
Then ErrInvalidConfig 에러가 반환된다

When Configure가 mappings 없이 호출되면:
  | field | $.payload.code |
Then ErrInvalidConfig 에러가 반환된다

When Configure가 빈 mappings로 호출되면:
  | field    | $.payload.code |
  | mappings | {} |
Then ErrInvalidConfig 에러가 반환된다
```

### 시나리오 9: Init 및 Shutdown 생명주기 [R-MAP-005]

```gherkin
Given MappingNode가 생성되었다
When Init(ctx)가 호출되면
Then 에러가 반환되지 않는다

When Shutdown(ctx)가 호출되면
Then 에러가 반환되지 않는다
```

### 시나리오 10: 불리언 키 변환 [R-MAP-003]

```gherkin
Given MappingNode가 다음 설정으로 Configure되었다:
  | field    | $.payload.active |
  | mappings | {"true": "활성", "false": "비활성"} |
And 메시지의 payload.active 값이 불리언 true 이다
When Process(ctx, msg)가 호출되면
Then 소스 값이 문자열 "true"로 변환되어 매핑 테이블에서 조회된다
And 반환된 메시지에 "활성" 값이 기록된다
```

### 시나리오 11: 레지스트리 등록 확인 [R-MAP-007]

```gherkin
Given NewRegistry()로 레지스트리가 생성되었다
When Has("mapping")을 호출하면
Then true가 반환된다

When TypeMeta("mapping")을 호출하면
Then Category가 "processing"이고 Description이 "키 기반 값 매핑"이다

When Create(NodeDef{Type: "mapping"})를 호출하면
Then MappingNode 인스턴스가 반환된다
```

---

## 2. 품질 게이트

### 2.1 테스트 커버리지

| 파일 | 최소 커버리지 | 기대 커버리지 |
|------|-------------|-------------|
| `internal/node/mapping.go` | 85% | 90%+ |
| `internal/node/errors.go` | 기존 유지 | 기존 유지 |
| `internal/node/registry.go` | 기존 유지 | 기존 유지 |

### 2.2 코드 품질

- [x] `go vet ./internal/node/...` 경고 없음
- [x] `golangci-lint run ./internal/node/...` 오류 없음
- [x] `go test -race ./internal/node/...` 레이스 컨디션 없음
- [x] 인터페이스 컴파일 체크 통과: `var _ Node = (*MappingNode)(nil)`
- [x] 한국어 주석 및 테스트명 사용 (code_comments=ko)
- [x] TypeScript 컴파일 성공: `npx tsc --noEmit` (프론트엔드 변경 시)

### 2.3 TRUST 5 품질 검증

| 차원 | 검증 항목 | 기준 |
|------|----------|------|
| **Tested** | 단위 테스트 커버리지 | mapping.go 85%+ |
| **Tested** | 레이스 디텍터 | `go test -race` 통과 |
| **Readable** | 코드 주석 | 한국어, 공개 함수/타입 전수 문서화 |
| **Readable** | 네이밍 컨벤션 | 기존 노드 패턴 일관성 유지 |
| **Unified** | 코드 포맷팅 | `gofmt` 적용 |
| **Unified** | 린트 | `golangci-lint` 경고 없음 |
| **Secured** | 입력 검증 | Configure에서 필수 필드 검증 |
| **Secured** | 에러 처리 | 센티널 에러로 명시적 에러 반환 |
| **Trackable** | 커밋 메시지 | `feat(SPEC-MAP-001): ...` 형식 |
| **Trackable** | 요구사항 추적 | R-MAP-001 ~ R-MAP-010 매핑 |

---

## 3. Definition of Done

### 3.1 필수 완료 조건

- [x] `internal/node/mapping.go` 구현 완료
- [x] `internal/node/mapping_test.go` 작성 완료 (시나리오 1~11 전체 커버)
- [x] `internal/node/errors.go`에 센티널 에러 2개 추가
- [x] `internal/node/registry.go`에 mapping 빌트인 등록
- [x] `registry_test.go` 빌트인 수 검증 업데이트 (10 -> 11)
- [x] `go test -race ./internal/node/...` 전체 통과
- [x] `go vet ./internal/node/...` 경고 없음
- [x] mapping.go 테스트 커버리지 85% 이상

### 3.2 선택 완료 조건 (프론트엔드)

- [x] `web/src/config/nodeSchemas.ts`에 mapping 스키마 추가
- [x] `web/src/pages/nodes/nodeTypeMeta.ts`에 mapping 메타데이터 추가
- [x] `npx tsc --noEmit` 프론트엔드 컴파일 성공

### 3.3 최종 검증

```bash
# 백엔드 전체 테스트
go test -race -coverprofile=coverage.out ./internal/node/...

# 커버리지 확인
go tool cover -func=coverage.out | grep mapping

# 린트 검증
golangci-lint run ./internal/node/...

# 프론트엔드 타입 체크 (선택)
cd web && npx tsc --noEmit
```
