# SPEC-STORE-001: 플로우 영속 스토리지 구현

## Context

현재 플로우는 메모리에만 저장된다. `FlowServiceAdapter.flowStore`는 `map[string]flow.Flow`이며,
서버 재시작 시 모든 미배포 플로우가 소실된다. 배포된 플로우도 `Engine.flows`에 런타임 전용으로 존재한다.

**목표**: 플로우를 파일(YAML), SQLite, PostgreSQL 3가지 백엔드로 영속 저장하고,
기존 `StorageConfig`와 연동하여 설정 기반으로 백엔드를 선택한다.

## 현재 아키텍처

- [flow_adapter.go](internal/api/service/flow_adapter.go): `flowStore map[string]flow.Flow` — 인메모리 저장
- [config/types.go](internal/config/types.go): `StorageConfig` — `Type`/`SQLitePath`/`PostgresDSN`/`PoolSize` (이미 정의됨)
- [config/defaults.go](internal/config/defaults.go): `storage.type: "sqlite"`, `storage.sqlite.path: "./data/xflow.db"` (이미 정의됨)
- [pkg/flow/serialize.go](pkg/flow/serialize.go): `FlowToYAML`/`FlowFromYAML`/`FlowFromJSON` (재사용)
- [pkg/flow/flow.go](pkg/flow/flow.go): `Flow` 인터페이스 — `ID()`, `Name()`, `Nodes()`, `Edges()` 등

## 설계 방침

- **FlowRepository 인터페이스**: CRUD + List 추상화로 백엔드 교체 용이
- **팩토리 패턴**: `StorageConfig.Type`에 따라 `"file"` / `"sqlite"` / `"postgres"` 선택
- **기존 직렬화 재사용**: `pkg/flow/serialize.go`의 YAML/JSON 변환 함수 활용
- **Pure Go SQLite**: `modernc.org/sqlite` (CGO 불필요)
- **PostgreSQL**: `github.com/jackc/pgx/v5` (커넥션 풀링 내장)
- **FlowServiceAdapter 통합**: `flowStore` 맵을 `FlowRepository`로 교체

## 변경 범위

| 파일 | 변경 | 설명 |
|------|------|------|
| [config/types.go](internal/config/types.go) | 수정 | `StorageConfig`에 `FileDirectory` 필드 추가, `Type` 주석에 `"file"` 추가 |
| [config/defaults.go](internal/config/defaults.go) | 수정 | `storage.file.directory` 기본값 추가 |
| `internal/storage/repository.go` | **신규** | `FlowRepository` 인터페이스 정의 |
| `internal/storage/factory.go` | **신규** | `NewRepository(cfg)` 팩토리 함수 |
| `internal/storage/file.go` | **신규** | 파일 기반 YAML 저장 구현 |
| `internal/storage/sqlite.go` | **신규** | SQLite 저장 구현 |
| `internal/storage/postgres.go` | **신규** | PostgreSQL 저장 구현 |
| `internal/storage/file_test.go` | **신규** | 파일 저장소 테스트 |
| `internal/storage/sqlite_test.go` | **신규** | SQLite 저장소 테스트 |
| `internal/storage/factory_test.go` | **신규** | 팩토리 테스트 |
| [flow_adapter.go](internal/api/service/flow_adapter.go) | 수정 | `flowStore` 맵 → `FlowRepository` 교체 |
| [main.go](cmd/xflowd/main.go) | 수정 | 스토리지 초기화 및 FlowServiceAdapter에 주입 |

## 구현 단계

### 1단계: FlowRepository 인터페이스 (`internal/storage/repository.go`)

```go
type FlowRepository interface {
    Save(ctx context.Context, f flow.Flow) error
    Get(ctx context.Context, id string) (flow.Flow, error)
    List(ctx context.Context) ([]flow.Flow, error)
    Delete(ctx context.Context, id string) error
    Close() error
}
```

### 2단계: 설정 확장 (`internal/config/`)

`StorageConfig`에 파일 디렉토리 필드 추가:

```go
type StorageConfig struct {
    Type          string // "file", "sqlite", "postgres"
    FileDirectory string // 파일 저장 디렉토리 (Type="file" 시)
    SQLitePath    string
    PostgresDSN   string
    PoolSize      int
}
```

기본값 추가: `storage.file.directory` → `"./data/flows"`

### 3단계: 파일 백엔드 (`internal/storage/file.go`)

- 디렉토리 내 `{flowID}.yaml` 파일로 저장
- `pkg/flow/serialize.go`의 `FlowToYAML`/`FlowFromYAML` 재사용
- `Save`: YAML 직렬화 → 파일 쓰기 (atomic write with temp file + rename)
- `Get`: 파일 읽기 → YAML 역직렬화
- `List`: 디렉토리 glob `*.yaml` → 각 파일 로드
- `Delete`: 파일 삭제

### 4단계: SQLite 백엔드 (`internal/storage/sqlite.go`)

- `modernc.org/sqlite` 드라이버 사용 (Pure Go, CGO 불필요)
- 테이블: `flows (id TEXT PRIMARY KEY, name TEXT, data BLOB, created_at, updated_at)`
- `data` 컬럼: JSON 직렬화된 플로우 정의 (`FlowToJSON` 활용)
- `database/sql` 표준 인터페이스 사용
- `Save`: INSERT OR REPLACE
- `Get`: SELECT by id → `FlowFromJSON` 역직렬화
- `List`: SELECT all → 각 행 역직렬화
- 자동 마이그레이션: `Open` 시 CREATE TABLE IF NOT EXISTS

### 5단계: PostgreSQL 백엔드 (`internal/storage/postgres.go`)

- `github.com/jackc/pgx/v5/pgxpool` 커넥션 풀 사용
- 동일 테이블 스키마: `flows (id, name, data JSONB, created_at, updated_at)`
- JSONB 타입으로 쿼리 가능한 저장
- `Save`: INSERT ON CONFLICT DO UPDATE
- `Get`/`List`/`Delete`: 표준 SQL 쿼리

### 6단계: 팩토리 (`internal/storage/factory.go`)

```go
func NewRepository(ctx context.Context, cfg config.StorageConfig) (FlowRepository, error) {
    switch cfg.Type {
    case "file":
        return NewFileRepository(cfg.FileDirectory)
    case "sqlite":
        return NewSQLiteRepository(ctx, cfg.SQLitePath)
    case "postgres":
        return NewPostgresRepository(ctx, cfg.PostgresDSN, cfg.PoolSize)
    default:
        return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
    }
}
```

### 7단계: FlowServiceAdapter 통합

`FlowServiceAdapter` 구조체 변경:

```go
type FlowServiceAdapter struct {
    engine *engine.Engine
    repo   storage.FlowRepository  // flowStore 맵 대체
    logger *slog.Logger
}
```

- `mu sync.RWMutex`와 `flowStore map` 제거
- 모든 CRUD 메서드를 `repo` 호출로 전환
- `CreateFlow` → `repo.Save()`
- `GetFlow` → `repo.Get()` (없으면 엔진에서 조회)
- `ListFlows` → `repo.List()` + 엔진 배포 목록 병합
- `UpdateFlow` → `repo.Get()` + 수정 + `repo.Save()`
- `DeleteFlow` → `repo.Delete()` (엔진에서도 정지/제거)
- `DeployFlow` → `repo.Get()` + `repo.Delete()` + `engine.DeployFlow()`

### 8단계: 서버 와이어링 (`cmd/xflowd/main.go`)

```go
// Storage 초기화
storageCfg := config.StorageConfig{
    Type:          viper.GetString("storage.type"),
    FileDirectory: viper.GetString("storage.file.directory"),
    SQLitePath:    viper.GetString("storage.sqlite.path"),
    PostgresDSN:   viper.GetString("storage.postgres.dsn"),
    PoolSize:      viper.GetInt("storage.pool_size"),
}
repo, err := storage.NewRepository(ctx, storageCfg)
// ...
defer repo.Close()

flowSvc := service.NewFlowServiceAdapter(eng, repo, apiLogger.Logger())
```

### 9단계: 테스트

- `TestFileRepository_CRUD`: 파일 저장소 CRUD 전체 사이클
- `TestSQLiteRepository_CRUD`: SQLite 저장소 CRUD 전체 사이클
- `TestFactoryNewRepository`: 설정별 올바른 백엔드 생성 확인
- `TestFlowServiceAdapter_WithRepository`: 통합 테스트 (리포지토리 연동)

## 검증

```bash
# 단위 테스트
go test -race -cover ./internal/storage/...
go test -race -cover ./internal/api/service/...

# 빌드 확인
go vet ./...
go build ./cmd/xflowd ./cmd/xflow

# 수동 검증 (파일 모드)
# config: storage.type=file, storage.file.directory=./data/flows
xflow flow create -f examples/flows/nasa-monitoring.yaml
ls ./data/flows/  # YAML 파일 확인
# 서버 재시작 후
xflow flow list   # 플로우가 유지되는지 확인

# 수동 검증 (SQLite 모드)
# config: storage.type=sqlite
xflow flow create -f examples/flows/nasa-monitoring.yaml
sqlite3 ./data/xflow.db "SELECT id, name FROM flows"
```
