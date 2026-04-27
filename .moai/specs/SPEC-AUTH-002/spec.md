---
id: SPEC-AUTH-002
version: "1.0.0"
status: completed
created: 2026-03-31
updated: 2026-03-31
author: xtra
priority: high
related_specs:
  - SPEC-AUTH-001
  - SPEC-API-001
  - SPEC-WEB-001
tags:
  - authentication
  - web-ui
  - security
  - config
---

# SPEC-AUTH-002: Web UI 기본 인증 - 설정 파일 기반 로그인/비밀번호 변경

## 변경 이력

| 버전  | 날짜       | 작성자 | 변경 내용           |
| ----- | ---------- | ------ | ------------------- |
| 1.0.0 | 2026-03-31 | xtra   | 최초 작성 (draft)   |

---

## 1. 개요

xflow Web UI 접속 시 아이디/패스워드 기반 기본 인증 기능을 제공한다.
인증 활성화 여부는 `xflow.yaml` 설정 파일에서 제어하며, 인증이 활성화된 경우 별도의 자격증명 파일에 사용자 정보를 저장한다.
비밀번호 변경 기능을 지원하며, 최초 실행 시 기본 관리자 계정이 자동 생성된다.

**범위**: 이 SPEC은 단순한 기본 인증만을 다루며, SPEC-AUTH-001의 엔터프라이즈급 JWT/RBAC/OAuth2와는 별개의 독립된 기능이다.

## 2. 범위

### 포함

- 설정 파일 기반 인증 활성화/비활성화
- 별도 자격증명 파일(`users.yaml`)의 사용자 관리
- bcrypt 기반 비밀번호 해싱
- 로그인/로그아웃 API 엔드포인트
- 비밀번호 변경 API 엔드포인트
- Web UI 로그인 페이지
- Web UI 비밀번호 변경 다이얼로그
- JWT 세션 토큰 발행 및 검증
- WebSocket 연결 인증
- 최초 실행 시 기본 admin 계정 자동 생성

### 제외

- RBAC (역할 기반 접근 제어) - SPEC-AUTH-001 범위
- OAuth2/OIDC 외부 인증 - SPEC-AUTH-001 범위
- API Key 인증 - SPEC-AUTH-001 범위
- 다중 사용자 권한 관리 (admin/editor/viewer 구분)
- 비밀번호 찾기/재설정 이메일 발송
- 2FA (2단계 인증)

---

## 3. 환경 (Environment)

### 기존 인프라

- **HTTP 서버**: `internal/api/server.go` - 미들웨어 체인: Recovery -> RequestID -> Logger -> Compress -> CORS -> RateLimit -> Timeout -> Auth
- **Auth 미들웨어**: `internal/api/middleware.go` - `Auth()` 함수가 패스스루 플레이스홀더로 존재 (line 259)
- **Context 키**: `ctxKeyUserID`, `ctxKeyUserRole` 이미 정의됨
- **설정 시스템**: Viper 기반, `XFLOW_` 환경 변수 접두어, AutomaticEnv()
- **설정 타입**: `ServerConfig`에 `WebUI WebUIConfig` 필드 존재
- **AuthConfig**: JWT, APIKey, OAuth2 구조체 정의됨 (SPEC-AUTH-001용)

### 프론트엔드 인프라

- **authService.ts**: `login()`, `logout()`, `refreshToken()`, `getCurrentUser()` 함수 존재
- **types/auth.ts**: `AuthTokens`, `LoginRequest`, `LoginResponse`, `User` 타입 정의됨
- **interceptors.ts**: Auth 인터셉터 설정됨
- **stores/authStore.ts**: 인증 상태 관리 스토어 존재
- **로그인 페이지**: 아직 없음

### 기술 스택

- Backend: Go 1.23+, chi router, Viper config
- Frontend: React 19, TypeScript, Zustand, React Router
- 비밀번호 해싱: `golang.org/x/crypto/bcrypt`
- 토큰: JWT (`github.com/golang-jwt/jwt/v5`)

---

## 4. 가정 (Assumptions)

- A1: 단일 인스턴스 환경을 가정한다. 분산 세션 관리는 고려하지 않는다.
- A2: 사용자 수는 소규모(10명 이하)를 가정한다. 대규모 사용자 관리가 필요한 경우 SPEC-AUTH-001을 사용한다.
- A3: 자격증명 파일은 로컬 파일시스템에 저장되며, xflowd 프로세스가 읽기/쓰기 권한을 가진다.
- A4: HTTPS는 별도로 처리되며(TLS 설정 또는 리버스 프록시), 이 SPEC에서는 HTTP 레벨 인증만 다룬다.
- A5: 기존 `AuthConfig` (JWT, APIKey, OAuth2)는 SPEC-AUTH-001의 엔터프라이즈 인증용이며, 이 SPEC의 기본 인증과는 별도의 설정 경로를 사용한다.

---

## 5. 요구사항 (Requirements)

### 5.1 편재 요구사항 (Ubiquitous)

- **REQ-U-001**: 시스템은 **항상** 비밀번호를 bcrypt 해싱하여 저장해야 한다. 평문 비밀번호를 파일이나 메모리에 보관하지 않아야 한다.
- **REQ-U-002**: 시스템은 **항상** 인증 관련 이벤트(로그인 성공/실패, 비밀번호 변경)를 구조화된 로그로 기록해야 한다.
- **REQ-U-003**: 시스템은 **항상** JWT 토큰에 만료 시간을 설정해야 한다.

### 5.2 이벤트 기반 요구사항 (Event-Driven)

- **REQ-E-001**: **WHEN** 사용자가 올바른 아이디/패스워드를 제출하면, **THEN** 시스템은 JWT access token과 refresh token을 발행한다.
- **REQ-E-002**: **WHEN** 사용자가 잘못된 아이디/패스워드를 제출하면, **THEN** 시스템은 401 Unauthorized를 반환하고 실패를 로그에 기록한다.
- **REQ-E-003**: **WHEN** 사용자가 로그아웃을 요청하면, **THEN** 시스템은 해당 세션의 refresh token을 무효화한다.
- **REQ-E-004**: **WHEN** 사용자가 유효한 현재 비밀번호와 새 비밀번호를 제출하면, **THEN** 시스템은 자격증명 파일의 비밀번호를 변경한다.
- **REQ-E-005**: **WHEN** xflowd가 최초 시작되고 인증이 활성화되었으나 자격증명 파일이 없으면, **THEN** 시스템은 기본 admin 계정(admin/admin)으로 자격증명 파일을 자동 생성하고 경고 로그를 출력한다.
- **REQ-E-006**: **WHEN** 프론트엔드가 401 응답을 수신하면, **THEN** 로그인 페이지로 리다이렉트한다.

### 5.3 상태 기반 요구사항 (State-Driven)

- **REQ-S-001**: **IF** `server.basic_auth.enabled`가 `true`이면, **THEN** 모든 API 엔드포인트(`/health`, `/ready` 제외)는 유효한 JWT 토큰을 요구한다.
- **REQ-S-002**: **IF** `server.basic_auth.enabled`가 `false`이거나 설정이 없으면, **THEN** Auth 미들웨어는 패스스루로 동작한다 (현재 동작 유지).
- **REQ-S-003**: **IF** 인증이 활성화된 상태에서 WebSocket 연결이 요청되면, **THEN** 초기 핸드셰이크 시 JWT 토큰 검증을 수행한다.

### 5.4 금지 요구사항 (Unwanted)

- **REQ-N-001**: 시스템은 평문 비밀번호를 로그에 기록**하지 않아야 한다**.
- **REQ-N-002**: 시스템은 로그인 실패 시 "사용자가 존재하지 않음"과 "비밀번호가 틀림"을 구분하여 응답**하지 않아야 한다** (타이밍 공격 방지).
- **REQ-N-003**: 시스템은 만료된 JWT 토큰으로 보호된 리소스에 접근을 허용**하지 않아야 한다**.

### 5.5 선택 요구사항 (Optional)

- **REQ-O-001**: **가능하면** 연속 로그인 실패 시 계정 잠금 기능을 제공한다 (예: 5회 실패 시 5분 잠금).
- **REQ-O-002**: **가능하면** JWT access token 만료 시 refresh token으로 자동 갱신 기능을 제공한다.

---

## 6. 명세 (Specifications)

### 모듈 1: 설정 및 자격증명 관리

#### 6.1.1 설정 구조 (`xflow.yaml`)

```yaml
server:
  basic_auth:
    enabled: true
    credentials_file: "/etc/xflow/users.yaml"  # 기본값: ~/.xflow/users.yaml
    jwt_secret: ""          # 비어있으면 자동 생성 (32바이트 랜덤)
    token_expiry: "24h"     # access token 만료
    refresh_expiry: "168h"  # refresh token 만료 (7일)
```

#### 6.1.2 자격증명 파일 형식 (`users.yaml`)

```yaml
users:
  - username: "admin"
    password_hash: "$2a$10$..."   # bcrypt hash
    created_at: "2026-03-31T00:00:00Z"
    updated_at: "2026-03-31T00:00:00Z"
```

#### 6.1.3 Go 타입 정의

- `BasicAuthConfig` 구조체를 `internal/config/types.go`의 `ServerConfig`에 추가
- `CredentialUser` 구조체: Username, PasswordHash, CreatedAt, UpdatedAt
- `CredentialsFile` 구조체: Users []CredentialUser

#### 6.1.4 자격증명 파일 관리

- `internal/auth/credentials.go`: 자격증명 파일 로드/저장/검증
- 파일이 없을 때 기본 admin 계정 자동 생성
- bcrypt cost factor: 10 (기본값)

### 모듈 2: Auth 미들웨어 및 API

#### 6.2.1 Auth 미들웨어 구현

- `internal/api/middleware.go`의 기존 `Auth()` 함수를 구현
- `BasicAuthConfig`를 주입받아 동작 결정
  - `enabled=false`: 패스스루 (현재 동작 유지)
  - `enabled=true`: JWT 토큰 검증, `ctxKeyUserID`와 `ctxKeyUserRole` 설정
- `/health`, `/ready` 엔드포인트는 인증 면제

#### 6.2.2 API 엔드포인트

| 메서드 | 경로                    | 설명               | 인증 필요 |
| ------ | ----------------------- | ------------------ | --------- |
| POST   | `/api/v1/auth/login`    | 로그인             | 아니오    |
| POST   | `/api/v1/auth/logout`   | 로그아웃           | 예        |
| POST   | `/api/v1/auth/refresh`  | 토큰 갱신          | 아니오    |
| GET    | `/api/v1/auth/me`       | 현재 사용자 조회   | 예        |
| PUT    | `/api/v1/auth/password` | 비밀번호 변경      | 예        |

#### 6.2.3 JWT 토큰 구조

```json
{
  "sub": "admin",
  "role": "admin",
  "iat": 1711843200,
  "exp": 1711929600
}
```

#### 6.2.4 WebSocket 인증

- WebSocket 업그레이드 요청 시 `Authorization` 헤더 또는 `token` 쿼리 파라미터로 JWT 전달
- 기존 `internal/api/ws/` 패키지에서 핸드셰이크 시 토큰 검증 추가

### 모듈 3: 로그인 UI 및 비밀번호 변경

#### 6.3.1 로그인 페이지

- `web/src/pages/auth/LoginPage.tsx`: 로그인 폼 컴포넌트
- 아이디(username), 패스워드 입력 필드
- 로그인 버튼, 에러 메시지 표시
- 인증이 비활성화된 경우 이 페이지는 표시되지 않음

#### 6.3.2 인증 상태에 따른 라우팅

- `authStore.ts` 업데이트: 인증 활성화 여부 상태 추가
- 인증이 활성화된 경우 ProtectedRoute wrapper 적용
- 401 응답 시 로그인 페이지로 리다이렉트

#### 6.3.3 비밀번호 변경

- `web/src/pages/auth/ChangePasswordDialog.tsx`: 비밀번호 변경 다이얼로그
- 현재 비밀번호, 새 비밀번호, 새 비밀번호 확인 입력
- Header 영역에 비밀번호 변경 메뉴 추가

#### 6.3.4 types/auth.ts 업데이트

- `LoginRequest.email`을 `LoginRequest.username`으로 변경 (기본 인증은 이메일이 아닌 사용자명 사용)
- `ChangePasswordRequest` 타입 추가: `current_password`, `new_password`

---

## 7. 우선순위 매트릭스

| 우선순위 | 요구사항                         | 모듈 |
| -------- | -------------------------------- | ---- |
| 최우선   | 설정 구조 및 자격증명 파일 관리  | M1   |
| 최우선   | Auth 미들웨어 구현               | M2   |
| 최우선   | 로그인/로그아웃 API              | M2   |
| 최우선   | 로그인 페이지 UI                 | M3   |
| 높음     | 비밀번호 변경 API 및 UI          | M2+M3|
| 높음     | WebSocket 인증                   | M2   |
| 높음     | 기본 admin 계정 자동 생성        | M1   |
| 보통     | Refresh token 자동 갱신          | M2+M3|
| 낮음     | 계정 잠금 기능                   | M2   |

---

## 8. SPEC-AUTH-001과의 관계

SPEC-AUTH-002 (이 문서)는 SPEC-AUTH-001의 **선행 단계**가 아니라 **대체 옵션**이다.

- **SPEC-AUTH-002**: 소규모 배포용 기본 인증. 설정 파일 기반, 단순한 사용자 관리.
- **SPEC-AUTH-001**: 엔터프라이즈급 인증. JWT/RBAC/OAuth2, 데이터베이스 기반 사용자 관리.

두 시스템은 `server.basic_auth` vs `auth` 설정 경로로 독립적이며, 동시에 활성화할 수 없다.

---

## 추적 태그

- `[SPEC-AUTH-002-M1]` - 설정 및 자격증명 관리
- `[SPEC-AUTH-002-M2]` - Auth 미들웨어 및 API
- `[SPEC-AUTH-002-M3]` - 로그인 UI 및 비밀번호 변경
