# SPEC-AUTH-005 인수 조건

모든 시나리오는 `basic_auth.enabled = true` 를 전제로 한다(AC-10 제외).

---

## AC-01 — 빌트인 역할 시드

**Given** `roles` 테이블이 없는 기존 데이터베이스가 있고
**When** 서버가 기동해 스키마 마이그레이션을 수행하면
**Then** `roles` 에 `admin` / `editor` / `viewer` 3개 행이 `builtin = 1` 로 생성되고, `role_permissions` 에 각 역할의 권한이 채워지며, `admin` 은 카탈로그의 전체 권한을 보유한다.
**And** 마이그레이션을 다시 실행해도 행이 중복 생성되지 않는다(멱등).

## AC-02 — users.role CHECK 제약 제거

**Given** `users.role` 에 `CHECK (role IN ('admin','editor','viewer'))` 제약이 있는 기존 데이터베이스에 사용자 3명이 저장되어 있고
**When** 마이그레이션을 수행하면
**Then** 제약이 제거되고 사용자 3명의 username·password_hash·role·timestamps 가 모두 보존된다.
**And** 마이그레이션 후 `role = 'operator'` 인 사용자를 삽입해도 오류가 발생하지 않는다.
**And** 마이그레이션을 재실행해도 데이터가 변하지 않는다.

## AC-03 — 역할 생성과 권한 조합

**Given** `role.create` 권한을 가진 관리자가 로그인해 있고
**When** `POST /api/v1/roles` 로 `{"name":"operator","permissions":["agent.read","agent.execute","device.read"]}` 를 보내면
**Then** 201 이 반환되고 `GET /api/v1/roles` 응답에 해당 역할이 권한 3개와 함께 나타난다.
**And** 카탈로그에 없는 권한 키(`agent.launch`)를 포함해 요청하면 400 으로 거부되고 역할은 생성되지 않는다.

## AC-04 — 사용자 등록과 역할 부여

**Given** `user.create` 권한을 가진 관리자가 로그인해 있고 `operator` 역할이 존재할 때
**When** `POST /api/v1/users` 로 `{"username":"kim","password":"<8자 이상>","role":"operator"}` 를 보내면
**Then** 201 이 반환되고 응답 본문에 `password_hash` 가 포함되지 않는다.
**And** `kim` 계정으로 로그인하면 성공하고, `GET /api/v1/auth/me` 의 `permissions` 가 `operator` 역할의 권한 3개와 일치한다.

## AC-05 — 역할 변경 즉시 반영

**Given** `kim` 이 `operator` 역할로 로그인해 액세스 토큰을 보유한 상태에서
**When** 관리자가 `PUT /api/v1/users/kim` 으로 역할을 `viewer` 로 변경하면
**Then** `kim` 이 **기존 토큰 그대로** `POST /api/v1/agents/{id}/start` 를 호출해도 403 으로 거부된다(권한은 요청 시점에 조회되므로 토큰 재발급이 필요 없다).
**And** `GET /api/v1/agents` 는 여전히 200 이다.

## AC-06 — /auth/me 하위 호환

**Given** 기존 클라이언트가 `GET /api/v1/auth/me` 를 호출할 때
**When** 응답을 파싱하면
**Then** 기존 필드(username, role 등)가 이전과 동일한 위치·형식으로 존재하고, `permissions` 배열이 추가되어 있다.

## AC-07 — 권한 없는 요청 차단

**Given** `viewer` 역할 사용자가 로그인해 있고
**When** 다음을 각각 호출하면
**Then** 표와 같이 응답한다.

| 요청 | 기대 |
|------|------|
| `GET /api/v1/agents` | 200 |
| `POST /api/v1/agents` | 403 |
| `PUT /api/v1/agents/{id}` | 403 |
| `DELETE /api/v1/agents/{id}` | 403 |
| `POST /api/v1/agents/{id}/start` | 403 |
| `GET /api/v1/users` | 403 |

**And** 403 응답 본문에 부족한 권한 키가 노출되지 않는다.
**And** 서버 로그에 username·permission·method·path 가 남는다.

## AC-08 — 보호 대상 라우트 커버리지

**Given** agents / devices / flows 핸들러가 등록하는 모든 라우트 목록이 있고
**When** 라우트 등록 결과를 검사하면
**Then** 각 라우트에 권한 미들웨어가 부착되어 있으며 부착 누락이 0건이다.
**And** 이 검사는 자동화 테스트로 수행되어, 이후 새 라우트를 추가하고 권한을 지정하지 않으면 테스트가 실패한다.

## AC-09 — 잠금 방지 불변식

**Given** 관리 권한을 가진 사용자가 `admin` 1명뿐일 때
**When** 아래를 시도하면
**Then** 모두 409 로 거부되고 상태가 변하지 않는다.

| 시도 | 기대 |
|------|------|
| 마지막 admin 삭제 | 409 |
| 마지막 admin 을 viewer 로 강등 | 409 |
| 자기 자신 삭제 | 409 |
| 빌트인 역할 `viewer` 삭제 | 409 |
| `admin` 역할의 권한 축소 | 409 |
| 사용자가 배정된 `operator` 역할 삭제 | 409 |

## AC-10 — 인증 비활성 회귀 없음

**Given** `basic_auth.enabled = false` 로 서버를 기동하고
**When** 토큰 없이 `POST /api/v1/agents` 및 `DELETE /api/v1/agents/{id}` 를 호출하면
**Then** 권한 검사로 인한 403 이 발생하지 않고 인증 도입 이전과 동일하게 동작한다.

---

## 품질 게이트

| 항목 | 기준 |
|------|------|
| 빌드 | `go build ./...` 성공 |
| 테스트 | `go test ./...` 전체 통과 |
| 정적 분석 | `go vet ./...` 무출력 |
| 커버리지 | 신규 패키지(`internal/auth/rbac.go`, `internal/storage/roles_sqlite.go`, `internal/api/handler/{user,role}.go`) 85% 이상 |
| 회귀 | 기존 인증 테스트(`internal/auth/...`, `internal/api/handler/auth_test.go`) 전부 통과 |

## 엣지 케이스

| 상황 | 기대 동작 |
|------|-----------|
| 존재하지 않는 역할을 사용자에게 부여 | 400, 사용자 상태 불변 |
| 중복 username 등록 | 409 |
| 8자 미만 비밀번호 | 400 |
| 역할 이름 대문자/공백 포함 | 400 |
| 역할 이름 변경 | 해당 역할을 쓰던 사용자의 `users.role` 도 함께 갱신 |
| 권한 캐시 적재 전 첫 요청 | 정상 처리(캐시 미스 시 DB 조회) |
| 토큰의 역할이 삭제된 역할을 가리킴 | 403(권한 없음으로 처리, 500 아님) |
