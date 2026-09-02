# SPEC-AUTH-005 구현 계획

## 1. 작업 분해

### M1 — 권한 카탈로그와 역할 저장소

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | 권한 키 카탈로그 상수 + 검증 함수 | `internal/auth/rbac.go` (신규) |
| 1.2 | 빌트인 역할 3종 정의(admin/editor/viewer 권한 집합) | `internal/auth/rbac.go` |
| 1.3 | `roles` / `role_permissions` 스키마 + 시드 | `internal/storage/sqlite.go` (`migrateRolesSchema`) |
| 1.4 | 역할 저장소 함수(List/Get/Insert/Update/Delete/ListPermissions) | `internal/storage/roles_sqlite.go` (신규) |

스키마:

```sql
CREATE TABLE IF NOT EXISTS roles (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL UNIQUE,
  description TEXT    NOT NULL DEFAULT '',
  builtin     INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id    INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission TEXT    NOT NULL,
  PRIMARY KEY (role_id, permission)
);
```

### M2 — `users.role` CHECK 제약 제거 (위험 구간)

SQLite는 CHECK 제약을 ALTER 로 제거할 수 없다. 다음 절차를 **단일 트랜잭션 + 멱등**으로 수행한다.

1. `PRAGMA table_info` / `sqlite_master` 의 DDL 문자열을 조회해 CHECK 제약 존재 여부 판정. 없으면 즉시 반환(멱등).
2. `users_new` 생성(동일 컬럼, CHECK 없음).
3. `INSERT INTO users_new SELECT * FROM users`.
4. `DROP TABLE users` → `ALTER TABLE users_new RENAME TO users`.
5. 인덱스·유니크 제약 재생성.

롤백: 트랜잭션 실패 시 전체 롤백되어 원본 `users` 가 유지된다. 실행 전 행 수를 기록하고 완료 후 대조해 손실이 없음을 확인한다.

### M3 — 사용자 저장소 확장

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `UpdateUserRole(ctx, db, username, role)` | `internal/storage/users_sqlite.go` |
| 3.2 | `DeleteUser(ctx, db, username)` | 동일 |
| 3.3 | `CountUsersByRole(ctx, db, role)` — 잠금 방지 불변식용 | 동일 |

### M4 — 인가 미들웨어

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 역할→권한 캐시(RWMutex + 무효화 훅) | `internal/auth/rbac.go` |
| 4.2 | `api.RequirePermission(perm string)` 미들웨어 | `internal/api/middleware.go` |
| 4.3 | 기존 `RequireRole` 처리 — 실제 검사로 대체하거나 Deprecated 주석 후 제거 | 동일 |
| 4.4 | 403 거부 시 구조화 로그 | 동일 |

`RequirePermission` 은 `Auth` 와 동일한 `enabled` 플래그를 공유해야 한다. 미들웨어 생성 지점(`cmd/xflowd` 또는 라우터 조립부)에서 플래그를 주입한다.

### M5 — 라우트 부착

agents / devices / flows 를 포함한 보호 대상 라우트에 권한 미들웨어를 부착한다. 매핑 원칙:

| 메서드 패턴 | 권한 |
|-------------|------|
| `GET /agents`, `GET /agents/{id}`, `/stats`, `/export` | `agent.read` |
| `POST /agents` | `agent.create` |
| `PUT /agents/{id}`, `PUT /agents/{id}/config` | `agent.update` |
| `DELETE /agents/{id}` | `agent.delete` |
| `POST /agents/{id}/{start,stop,restart,enable,disable}` | `agent.execute` |

devices / flows 도 동일 원칙을 적용한다. 부착 누락은 보안 구멍이므로, acceptance.md 의 커버리지 검사(보호 대상 라우트 전수 대조)로 검증한다.

### M6 — 사용자·역할 API 핸들러

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | 사용자 CRUD 핸들러 + DTO | `internal/api/handler/user.go`, `internal/api/dto/user.go` (신규) |
| 6.2 | 역할 CRUD 핸들러 + DTO | `internal/api/handler/role.go`, `internal/api/dto/role.go` (신규) |
| 6.3 | 권한 카탈로그 조회 | `internal/api/handler/role.go` |
| 6.4 | 잠금 방지 불변식 5종 | 6.1 / 6.2 내부 |
| 6.5 | `/auth/me` 에 `permissions` 추가 | `internal/api/handler/auth.go`, `internal/api/dto/auth.go` |
| 6.6 | 라우터 등록 | `cmd/xflowd/main.go` |

---

## 2. 기술 스택

- Go 1.x, 기존 `internal/api` 자체 라우터(외부 프레임워크 추가 없음)
- SQLite (`internal/storage`) — 신규 의존성 없음
- bcrypt: 기존 `internal/auth/password.go` 재사용

**새 외부 의존성 없음.** SPEC-AUTH-001 이 언급한 Redis 블랙리스트·OAuth2 라이브러리는 본 SPEC에서 도입하지 않는다.

---

## 3. 위험 분석

| 위험 | 영향 | 완화 |
|------|------|------|
| CHECK 제약 제거 중 테이블 재생성 실패 | 사용자 테이블 손실 = 로그인 불가 | 단일 트랜잭션, 멱등 판정, 행 수 대조, 마이그레이션 단위 테스트 |
| 라우트 부착 누락 | 특정 API가 무방비로 남음 | 보호 대상 라우트 전수 커버리지 테스트(AC-08) |
| 잠금(모든 admin 강등) | 관리 API 접근 영구 상실 | 불변식 5종 + 각각 단위 테스트 |
| 인증 비활성 배포 회귀 | 기존 사용자 전원 403 | `enabled=false` 패스스루 + 회귀 테스트(AC-10) |
| 캐시 무효화 누락 | 권한 변경이 반영되지 않음 | 무효화를 역할 쓰기 경로 단일 지점으로 강제 + 통합 테스트 |
| 역할 이름 변경 시 사용자 행 미갱신 | 사용자가 존재하지 않는 역할을 가리킴 | 이름 변경을 트랜잭션으로 묶어 `users.role` 동시 갱신 |

---

## 4. 구현 순서

M1 → M2 → M3 → M4 → M5 → M6

M2(마이그레이션)를 M1 직후에 두는 이유: 커스텀 역할을 가진 사용자를 저장할 수 없는 상태에서 M6의 사용자 API를 만들면, 통합 테스트가 CHECK 제약에 막혀 원인 파악이 어려워진다.

M5(라우트 부착)를 M6(관리 API)보다 앞에 두는 이유: 관리 API 자체도 `user.*` 권한으로 보호되어야 하므로 미들웨어가 먼저 동작해야 한다.

---

## 5. 검증 방법

각 마일스톤 완료 시:

```bash
go build ./... && go test ./internal/auth/... ./internal/storage/... ./internal/api/...
go vet ./...
```

전체 완료 시 acceptance.md 의 AC-01~AC-10 을 순서대로 검증한다.
