---
id: SPEC-AUTH-005
title: 사용자·역할 관리 및 서버 측 권한 강제 (RBAC 백엔드)
version: 0.1.0
status: draft
created: 2026-08-18
updated: 2026-08-18
author: xtra
priority: high
domain: auth
related_specs:
  - SPEC-AUTH-001
  - SPEC-AUTH-002
  - SPEC-AUTH-006
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-18 | xtra | 최초 작성 — 사용자 등록/역할 부여와 기능별 권한 강제를 정식화. 현재 `api.RequireRole` 이 인자를 버리는 패스스루라 인가가 전혀 강제되지 않는 결함을 해소하고, 관리자가 역할을 직접 만들어 권한을 조합할 수 있는 역할 CRUD 모델을 도입한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

XFlow 서버에 **사용자 관리**와 **기능 단위 권한 강제**를 도입한다. 세 축으로 구성된다.

1. **권한 모델**: `roles` / `role_permissions` 테이블과 `<resource>.<action>` 형식의 권한 키 카탈로그를 정의한다. 관리자가 역할을 생성·수정·삭제하고 권한을 조합할 수 있다.
2. **사용자 관리 API**: 사용자 등록·역할 부여·비밀번호 재설정·삭제를 REST API로 제공한다.
3. **인가 강제**: `api.RequirePermission` 미들웨어를 신설해 agents / devices / flows 를 포함한 보호 대상 라우트에 부착한다. 서버가 강제하는 것이 유일한 보안 경계이며, 웹 UI의 메뉴·버튼 숨김(SPEC-AUTH-006)은 보조 수단이다.

### 1.2 배경

#### 1.2.1 인가가 강제되지 않는 현재 상태

[`internal/api/middleware.go:366`](../../../internal/api/middleware.go) 의 `RequireRole` 은 인자를 받지 않고 버린다.

```go
// RequireRole 은 사용자가 필요한 역할을 가지고 있는지 확인한다.
// P1에서는 단순 패스스루이다.
func RequireRole(_ ...string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			return next(ctx)
		}
	}
}
```

`Auth` 미들웨어는 JWT를 검증하고 `ctx.UserID()` / `ctx.UserRole()` 을 채우지만, 그 값을 소비해 접근을 막는 지점이 없다. 결과적으로 `viewer` 로 로그인한 사용자도 `DELETE /api/v1/agents/{id}` 를 호출할 수 있다.

#### 1.2.2 이미 존재하는 자산 (재사용 대상)

| 자산 | 위치 | 상태 |
|------|------|------|
| `users` 테이블 (username·password_hash·role·timestamps) | `internal/storage/sqlite.go:105` | 존재. `role` 에 `CHECK (role IN ('admin','editor','viewer'))` 제약 |
| 사용자 조회/삽입 함수 6종 | `internal/storage/users_sqlite.go` | 존재. **역할 변경·삭제 함수 없음** |
| `CredentialsManager` (인증·비밀번호 변경·기본 admin 생성) | `internal/auth/credentials.go` | 존재 |
| JWT `Claims.Role` | `internal/auth/jwt.go:22` | 존재. 로그인 시 역할이 토큰에 실림 |
| `Auth` 미들웨어 (토큰 검증 → 컨텍스트 주입) | `internal/api/middleware.go:295` | 존재 |
| 라우트별 미들웨어 부착 (`g.GET(path, handler, mw...)`) | `internal/api/router.go:174` | 존재 |

즉 **인증은 완성되어 있고 인가만 비어 있다**. 본 SPEC은 빈 자리를 채우는 작업이며 인증 흐름은 건드리지 않는다.

#### 1.2.3 SPEC-AUTH-001 과의 경계

[SPEC-AUTH-001](../SPEC-AUTH-001/spec.md)(status: draft)은 JWT·RBAC·OAuth2·API 키·감사 로깅을 포괄하는 장기 구상이다. 그중 실제로 구현된 것은 `jwt.go` / `password.go` / `credentials.go` 뿐이고 `rbac.go` · `apikey.go` · `oauth.go` · `audit.go` 는 존재하지 않는다.

본 SPEC은 그 구상 중 **RBAC + User Model 부분만** 현실 범위로 잘라내어 구현한다. SPEC-AUTH-001 은 장기 구상 문서로 유지되며, OAuth2 · API 키 · 감사 로깅은 본 SPEC의 범위가 아니다.

### 1.3 비범위 (Out of Scope)

- 웹 UI(사용자 관리 화면, 메뉴·버튼 게이팅) — **SPEC-AUTH-006**
- OAuth2 소셜 로그인, API 키 발급 — SPEC-AUTH-001 장기 구상으로 잔존
- 인증 이벤트 감사 로그 테이블 — 본 SPEC은 권한 거부를 구조화 로그로만 남긴다
- 리소스 인스턴스 단위 권한(특정 flow 1건만 편집 허용 등). 본 SPEC은 리소스 **타입** 단위까지만 다룬다
- PostgreSQL 저장소 대응 — `users` 테이블은 현재 SQLite 경로에만 존재하며 본 SPEC도 SQLite를 대상으로 한다
- 원격 노드(remote) 하위 API의 세분 권한 — `remote.*` 단일 키로만 다룬다

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 권한 모델과 역할 CRUD

시스템은 역할과 권한을 다음 구조로 영속한다.

**권한 키 형식**: `<resource>.<action>`

| 리소스 | 액션 | 비고 |
|--------|------|------|
| `agent` | read, create, update, delete, execute | execute = start/stop/restart/enable/disable |
| `device` | read, create, update, delete, execute | execute = 제어 명령 |
| `flow` | read, create, update, delete, execute | execute = 배포/시작/정지 |
| `dashboard` | read, update | 패널 레이아웃 저장 |
| `node` | read | 노드 타입 카탈로그 |
| `schedule` | read, create, update, delete | |
| `monitoring` | read | 로그·메트릭 조회 |
| `store` | read, update | 스토어 키/값 |
| `remote` | read, update | 원격 노드 관리 |
| `system` | read, update | 시스템 설정 |
| `user` | read, create, update, delete | 사용자 관리 |
| `role` | read, create, update, delete | 역할 관리 |

시스템은 이 카탈로그를 코드 상수로 보유하고 `GET /api/v1/permissions` 로 노출한다. 정의되지 않은 권한 키는 역할에 저장될 수 없다.

**빌트인 역할** (마이그레이션 시 시드, `builtin = 1`):

| 역할 | 권한 |
|------|------|
| `admin` | 전체 권한 |
| `editor` | 전 리소스 read + agent/device/flow/schedule/dashboard/store 의 create·update·execute (delete 제외, user·role·system 관리 제외) |
| `viewer` | 전 리소스 read 만 |

기존 `users.role` 값이 정확히 이 세 이름이므로 **사용자 데이터 마이그레이션은 불필요**하다.

관리자는 빌트인 외의 역할을 생성·수정·삭제할 수 있다. 역할 이름은 유일하며 소문자·숫자·하이픈만 허용한다.

### 2.2 [U2] (Ubiquitous) 사용자 관리 API

시스템은 다음 엔드포인트를 제공한다. 모두 `user.*` / `role.*` 권한을 요구한다.

| 메서드 | 경로 | 권한 | 설명 |
|--------|------|------|------|
| GET | `/api/v1/users` | `user.read` | 사용자 목록(비밀번호 해시 제외) |
| POST | `/api/v1/users` | `user.create` | 사용자 등록(username, password, role) |
| PUT | `/api/v1/users/{username}` | `user.update` | 역할 변경 |
| PUT | `/api/v1/users/{username}/password` | `user.update` | 관리자 비밀번호 재설정 |
| DELETE | `/api/v1/users/{username}` | `user.delete` | 사용자 삭제 |
| GET | `/api/v1/roles` | `role.read` | 역할 목록(권한 포함) |
| POST | `/api/v1/roles` | `role.create` | 역할 생성 |
| PUT | `/api/v1/roles/{name}` | `role.update` | 역할 권한 수정 |
| DELETE | `/api/v1/roles/{name}` | `role.delete` | 역할 삭제 |
| GET | `/api/v1/permissions` | 인증만 | 권한 키 카탈로그 |

또한 기존 `GET /api/v1/auth/me` 응답에 **`permissions` 배열을 추가**한다. 웹 UI가 이 값으로 메뉴·버튼을 게이팅한다(SPEC-AUTH-006). 기존 필드는 그대로 두어 하위 호환을 지킨다.

비밀번호는 기존 `internal/auth/password.go` 의 bcrypt 해싱을 그대로 사용하며 어떤 응답에도 해시를 포함하지 않는다.

### 2.3 [E1] (Event-driven) 요청 인가 강제

**When** 인증이 활성화된 상태에서 보호 대상 라우트로 요청이 도착하면, **the system shall** 요청자의 역할에 부여된 권한 집합을 조회하여 해당 라우트가 요구하는 권한 키를 보유했는지 검사하고, 보유하지 않으면 `403 Forbidden` 으로 거부한다.

- 권한 집합은 **요청 시점에 역할 이름으로 조회**한다. JWT에는 역할 이름만 담기므로, 역할의 권한을 수정하면 토큰 재발급 없이 다음 요청부터 즉시 반영된다.
- 조회 비용을 줄이기 위해 역할→권한 매핑을 인메모리 캐시에 보관하고, 역할·권한 변경 시 캐시를 무효화한다.
- 부착 대상은 agents(14) · devices · flows 를 포함한 보호 대상 라우트다. 부착 완료 여부는 acceptance.md 의 커버리지 검사로 검증한다.
- 거부 시 구조화 로그에 `username`, `permission`, `method`, `path` 를 남긴다. 응답 본문에는 어떤 권한이 부족한지 노출하지 않는다.

### 2.4 [UB1] (Unwanted-Behavior) 잠금 방지 불변식

시스템은 관리자가 스스로를 잠가버리는 상태를 만들 수 없어야 한다. 다음 요청은 `409 Conflict` 로 거부한다.

1. `user.delete` / `role.update` 권한을 보유한 마지막 사용자의 삭제 또는 강등
2. 자기 자신의 삭제
3. 빌트인 역할(`admin`, `editor`, `viewer`)의 삭제
4. `admin` 역할의 권한 수정(항상 전체 권한 유지)
5. 한 명 이상의 사용자가 사용 중인 역할의 삭제

각 거부는 원인을 식별할 수 있는 에러 코드를 응답에 포함한다.

### 2.5 [S1] (State-Driven) 인증 비활성 상태

**While** `basic_auth.enabled = false` 인 동안, **the system shall** 권한 검사를 수행하지 않고 모든 요청을 통과시킨다.

현재 `Auth` 미들웨어가 이 상태에서 패스스루하므로 컨텍스트에 역할이 없다. 이때 권한 검사를 수행하면 인증 없이 쓰던 기존 배포가 전부 403이 되어 회귀가 발생한다. `RequirePermission` 은 `Auth` 와 동일한 활성화 플래그를 공유하며, 비활성 시 패스스루 미들웨어를 반환한다.

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 권한 모델 | `internal/auth/rbac.go`(신규), `internal/storage/roles_sqlite.go`(신규), `internal/storage/sqlite.go`(스키마) | AC-01, AC-02 |
| U1 역할 CRUD | `internal/api/handler/role.go`(신규) | AC-03 |
| U2 사용자 관리 API | `internal/api/handler/user.go`(신규), `internal/storage/users_sqlite.go`(확장) | AC-04, AC-05 |
| U2 `/auth/me` 확장 | `internal/api/handler/auth.go`, `internal/api/dto/auth.go` | AC-06 |
| E1 인가 강제 | `internal/api/middleware.go`, 각 핸들러 `RegisterRoutes` | AC-07, AC-08 |
| UB1 잠금 방지 | `internal/api/handler/user.go`, `internal/api/handler/role.go` | AC-09 |
| S1 인증 비활성 | `internal/api/middleware.go` | AC-10 |

---

## 4. 설계 결정

### 4.1 역할을 문자열로 유지 (역할 id 정규화하지 않음)

`users.role` 을 `role_id` 외래키로 정규화하는 대신 **역할 이름 문자열을 유지**한다.

- JWT `Claims.Role`, `ctx.UserRole()`, 웹의 `User.role` 이 모두 문자열을 전제로 이미 동작한다. 정규화하면 이 경로가 전부 바뀐다.
- 역할 이름은 유일 제약이 있으므로 참조 무결성은 이름으로도 성립한다.
- 대가: 역할 이름 변경 시 사용자 행을 함께 갱신해야 한다. 역할 이름 변경은 트랜잭션 안에서 `users.role` 을 함께 갱신하는 것으로 처리한다.

### 4.2 `users.role` CHECK 제약 제거 (마이그레이션 위험 지점)

현재 스키마:

```sql
role TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'editor', 'viewer'))
```

이 제약이 살아 있으면 커스텀 역할을 가진 사용자를 저장할 수 없다. SQLite는 `ALTER TABLE ... DROP CONSTRAINT` 를 지원하지 않으므로 **테이블 재생성 절차**(신규 테이블 생성 → 데이터 복사 → 원본 삭제 → 이름 변경)가 필요하다. 단일 트랜잭션으로 수행하고 멱등하게(이미 제약이 없으면 건너뜀) 구현한다. 상세 절차와 롤백 계획은 plan.md 에 둔다.

### 4.3 권한 해석 시점: 토큰이 아니라 요청 시점

JWT에 권한 배열을 담으면 역할 권한을 수정해도 기존 토큰이 만료될 때까지 옛 권한으로 동작한다(액세스 토큰 수명만큼 권한 회수가 지연). 따라서 토큰에는 역할 이름만 두고 권한은 요청 시 조회한다. 캐시 무효화는 역할 변경 지점 한 곳에서만 일어나므로 일관성 유지가 단순하다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 인가 오버헤드 | 캐시 적중 시 요청당 추가 지연 1ms 미만 |
| 하위 호환 | 인증 비활성 배포는 동작 변화 없음. 기존 admin/editor/viewer 사용자는 재설정 없이 동일 권한 유지 |
| 보안 | 비밀번호 해시는 어떤 응답에도 포함되지 않음. 403 응답은 부족한 권한 키를 노출하지 않음 |
| 테스트 | 신규 코드 커버리지 85% 이상(quality.yaml 기준) |
| 마이그레이션 | 재실행 안전(멱등). 실패 시 기존 `users` 테이블이 손상되지 않음 |

---

## 6. 가정 및 제약

1. 대상 저장소는 SQLite. PostgreSQL 경로에는 `users` 테이블이 없으므로 본 SPEC 범위 밖이다.
2. 사용자 수는 수십 명 규모로 가정한다. 역할→권한 캐시는 전체를 메모리에 올린다.
3. 역할 이름은 소문자·숫자·하이픈, 1~32자.
4. 세션 무효화(권한 강등 시 기존 토큰 즉시 폐기)는 본 SPEC 범위 밖이다. 권한은 요청 시 조회하므로 강등은 다음 요청부터 적용되며, 토큰 자체는 유효하게 남는다.
5. 웹 UI가 없는 상태에서도 API만으로 사용자·역할 관리가 완결되어야 한다(SPEC-AUTH-006 지연 시에도 운영 가능).
