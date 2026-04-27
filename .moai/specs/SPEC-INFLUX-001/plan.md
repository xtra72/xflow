---
id: SPEC-INFLUX-001
type: plan
version: "1.0.0"
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
---

# SPEC-INFLUX-001 구현 계획: InfluxDB 연동 에이전트 구현

## 1. 작업 분해

### Primary Goal: 클라이언트 추상화 및 코어 에이전트 구현

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | InfluxDB 클라이언트 추상화 인터페이스 정의 | `internal/agent/system/influxdb_client.go` | High |
| 2 | 설정 파싱 함수 구현 (`parseInfluxDBConfig`) | `internal/agent/system/influxdb_config.go` | High |
| 3 | InfluxDB 2.x 어댑터 구현 | `internal/agent/system/influxdb_v2.go` | High |
| 4 | InfluxDB 3.x 어댑터 구현 | `internal/agent/system/influxdb_v3.go` | High |
| 5 | 메인 에이전트 구현 (Agent + MessageReceiver) | `internal/agent/system/influxdb_agent.go` | High |
| 6 | 타입 등록 함수 구현 | `internal/agent/system/influxdb_register.go` | High |
| 7 | main.go에서 RegisterInfluxDBTypes 호출 추가 | `cmd/xflowd/main.go` | High |

### Secondary Goal: 테스트 작성

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 8 | 설정 파싱 테스트 | `internal/agent/system/influxdb_config_test.go` | High |
| 9 | 클라이언트 추상화 테스트 (목 기반) | `internal/agent/system/influxdb_client_test.go` | High |
| 10 | 에이전트 Process/ReceiveMessage 테스트 | `internal/agent/system/influxdb_agent_test.go` | High |

### Final Goal: 예제 및 문서화

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 11 | 예제 에이전트 설정 YAML | `examples/agents/influxdb-writer.yaml` | Medium |
| 12 | 예제 플로우 설정 YAML | `examples/flows/influxdb-metrics.yaml` | Medium |
| 13 | go.mod 의존성 추가 | `go.mod`, `go.sum` | High |

---

## 2. 기술 접근 방식

### 2.1 클라이언트 추상화 레이어 (REQ-3)

InfluxDB 2.x와 3.x의 API 차이를 숨기는 내부 인터페이스를 정의한다:

```go
type InfluxClient interface {
    Write(ctx context.Context, data []WriteData) error
    Query(ctx context.Context, query string, lang string) ([]map[string]any, error)
    Health(ctx context.Context) error
    Close() error
}
```

팩토리 함수 `NewInfluxClient(config InfluxDBConfig) (InfluxClient, error)`가 `version` 값에 따라 적절한 구현체를 반환한다.

### 2.2 InfluxDB 2.x 어댑터 (REQ-4)

- `influxdb-client-go/v2` 패키지 사용
- `influxdb2.NewClient(url, token)` 으로 클라이언트 생성
- `WriteAPIBlocking`으로 동기 쓰기 (안정성 우선)
- Flux 쿼리: `queryAPI.Query(ctx, fluxQuery)` -> `QueryTableResult` 파싱
- InfluxQL 쿼리: v1 호환 API 경로 활용 또는 Flux 래핑
- `"sql"` 요청 시 `ErrUnsupportedLanguage` 반환

### 2.3 InfluxDB 3.x 어댑터 (REQ-5)

- `influxdb3-go/v2` 패키지 사용
- `influxdb3.New(influxdb3.ClientConfig{...})` 으로 클라이언트 생성
- `client.Write(ctx, points)` 또는 Line Protocol 직접 쓰기
- SQL 쿼리: `client.Query(ctx, sqlQuery)` -> Arrow 레코드 -> `map[string]any` 변환
- InfluxQL 쿼리: `client.QueryWithType(ctx, query, influxdb3.InfluxQL)` 활용
- `"flux"` 요청 시 `ErrUnsupportedLanguage` 반환

### 2.4 요청 타입 판별 (REQ-8)

`Process(data []byte)` 내부에서:

1. JSON 언마샬링 시도
2. `map[string]any`로 파싱 -> `"query"` 키 존재 여부 확인
   - 존재: `QueryRequest`로 언마샬링 -> 쿼리 처리
   - 미존재: `"measurement"` 키 존재 확인
     - 존재: `WriteData`로 단건 쓰기
3. `[]any`로 파싱 시도 -> 배치 `[]WriteData`로 쓰기
4. 어떤 패턴에도 매칭되지 않으면 에러 반환

### 2.5 쿼리 결과 전달 (REQ-7)

- `recvCh chan []byte` 채널 (버퍼 크기: `config.BufferSize`)
- 쿼리 결과를 `[]map[string]any`로 변환 후 JSON 직렬화
- 직렬화된 `[]byte`를 `recvCh`에 전달
- `ReceiveMessage(ctx)`: 컨텍스트 취소 또는 `recvCh`에서 데이터 수신

### 2.6 기존 패턴 참조

MQTT 에이전트의 다음 패턴을 그대로 활용:

- `parseConfig()` 패턴: `Transport.Options` 맵에서 설정값 추출, 기본값 적용
- 채널 기반 메시지 전달: `recvCh` 채널로 비동기 결과 전달
- `RegisterXXXTypes()` 패턴: 에이전트 매니저에 팩토리 등록
- `AgentConfig` -> 내부 설정 구조체 변환

---

## 3. 테스트 전략

### 3.1 단위 테스트 (Hybrid: 신규 코드이므로 TDD 적용)

| 테스트 케이스 | 검증 내용 | 유형 |
|--------------|----------|------|
| `TestParseInfluxDBConfig_RequiredFields` | 필수 설정 누락 시 에러 반환 | TDD |
| `TestParseInfluxDBConfig_DefaultValues` | 선택 설정의 기본값 적용 | TDD |
| `TestParseInfluxDBConfig_V2Defaults` | v2 기본 쿼리 언어 "flux" | TDD |
| `TestParseInfluxDBConfig_V3Defaults` | v3 기본 쿼리 언어 "sql" | TDD |
| `TestParseInfluxDBConfig_InvalidVersion` | "2", "3" 외의 값 에러 | TDD |
| `TestNewInfluxClient_V2` | v2 설정 시 V2Client 생성 | TDD |
| `TestNewInfluxClient_V3` | v3 설정 시 V3Client 생성 | TDD |
| `TestInfluxDBAgent_ProcessWrite_Single` | 단건 쓰기 JSON 처리 | TDD |
| `TestInfluxDBAgent_ProcessWrite_Batch` | 배치 쓰기 JSON 처리 | TDD |
| `TestInfluxDBAgent_ProcessQuery` | 쿼리 요청 처리 및 결과 수신 | TDD |
| `TestInfluxDBAgent_RequestTypeDetection` | 쓰기/쿼리 자동 판별 | TDD |
| `TestInfluxDBAgent_UnsupportedLanguage_V2` | v2에서 SQL 요청 시 에러 | TDD |
| `TestInfluxDBAgent_UnsupportedLanguage_V3` | v3에서 Flux 요청 시 에러 | TDD |
| `TestInfluxDBAgent_Lifecycle` | Init/Start/Stop 생명주기 | TDD |
| `TestInfluxDBAgent_Stats` | 통계 정보 누적 확인 | TDD |
| `TestInfluxDBAgent_ReceiveMessage` | 쿼리 결과 채널 수신 | TDD |
| `TestInfluxDBAgent_Pause_Resume` | 일시 중지 및 재개 | TDD |

### 3.2 목(Mock) 전략

- `InfluxClient` 인터페이스를 목으로 구현하여 에이전트 테스트에 주입
- 실제 InfluxDB 서버 없이 단위 테스트 실행 가능
- 목 클라이언트에서 에러 주입으로 에러 처리 경로 검증

### 3.3 테스트 실행

```bash
go test -race -v ./internal/agent/system/... -run TestInfluxDB
go test -race -cover ./internal/agent/system/...
```

---

## 4. 위험 분석

| 위험 | 영향도 | 확률 | 대응 |
|------|--------|------|------|
| influxdb3-go 라이브러리 API 불안정 | 중간 | 중간 | 추상화 레이어로 격리, 어댑터만 수정하면 됨 |
| go.mod 의존성 충돌 | 낮음 | 낮음 | 의존성 호환성 사전 확인 (go mod tidy) |
| Apache Arrow Flight 의존성 크기 | 중간 | 중간 | v3 어댑터를 빌드 태그로 분리 가능 (향후 최적화) |
| 쿼리 결과 JSON 변환 시 데이터 손실 | 중간 | 낮음 | 타입별 변환 테스트 작성, 숫자 정밀도 보존 |
| recvCh 버퍼 풀 시 쿼리 결과 유실 | 중간 | 낮음 | 버퍼 크기 설정 가능, 풀 시 로그 경고 |
| v2/v3 InfluxQL 쿼리 동작 차이 | 낮음 | 중간 | 버전별 InfluxQL 호환성 테스트 작성 |

---

## 5. 마일스톤

### Primary Goal

- [ ] `go.mod`에 InfluxDB 클라이언트 의존성 추가
- [ ] `InfluxClient` 인터페이스 정의 (`influxdb_client.go`)
- [ ] 설정 파싱 구현 및 테스트 (`influxdb_config.go`, `influxdb_config_test.go`)
- [ ] v2 어댑터 구현 (`influxdb_v2.go`)
- [ ] v3 어댑터 구현 (`influxdb_v3.go`)
- [ ] 메인 에이전트 구현 (`influxdb_agent.go`)
- [ ] 타입 등록 및 main.go 연동 (`influxdb_register.go`)
- [ ] 클라이언트 추상화 테스트 (`influxdb_client_test.go`)
- [ ] 에이전트 테스트 (`influxdb_agent_test.go`)
- [ ] 전체 테스트 통과 (`go test -race ./internal/agent/system/...`)

### Secondary Goal

- [ ] 에이전트 통계 정보 구현 (REQ-11)
- [ ] 쿼리 언어 변환 최적화 (REQ-12)

### Final Goal

- [ ] 예제 에이전트 설정 YAML 작성
- [ ] 예제 플로우 설정 YAML 작성

---

## 6. 추적성

| 요구사항 | 작업 | 마일스톤 |
|----------|------|----------|
| REQ-1 | 작업 6, 7 | Primary Goal |
| REQ-2 | 작업 2, 8 | Primary Goal |
| REQ-3 | 작업 1, 9 | Primary Goal |
| REQ-4 | 작업 3 | Primary Goal |
| REQ-5 | 작업 4 | Primary Goal |
| REQ-6 | 작업 5, 10 | Primary Goal |
| REQ-7 | 작업 5, 10 | Primary Goal |
| REQ-8 | 작업 5, 10 | Primary Goal |
| REQ-9 | 작업 5, 10 | Primary Goal |
| REQ-10 | 작업 3, 4, 10 | Primary Goal |
| REQ-11 | 작업 5 | Secondary Goal |
| REQ-12 | 작업 3, 4 | Secondary Goal |
