---
id: SPEC-AUTH-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-AUTH-001: Authentication & Authorization System - JWT 인증, RBAC 인가, OAuth2, API 키, 감사 로깅

## 1. 개요 (Overview)

XFlow 플랫폼의 인증(Authentication) 및 인가(Authorization) 시스템을 정의한다. 본 패키지는 JWT 토큰 기반 사용자 인증, 역할 기반 접근 제어(RBAC), OAuth2 소셜 로그인, API 키 서비스 간 통신, 감사 로깅을 포함하는 엔터프라이즈급 보안 체계를 제공한다.

CLI(`xflow`)와 Web Dashboard 모두 `xflowd` 데몬 서버의 REST API를 통해 원격 접속하며, 이 모든 접근에 대해 본 패키지가 인증/인가를 수행한다. API Layer(`internal/api/`, SPEC-API-001)의 미들웨어가 본 패키지의 인터페이스를 소비하여 요청별 인증 검증과 권한 확인을 수행한다.

본 SPEC은 다음을 포함한다:

- **JWT Token Service** (`jwt.go`): 액세스/리프레시 토큰 생성, 검증, 갱신
- **Token Blacklist** (`blacklist.go`): Redis 기반 토큰 무효화, 인메모리 폴백
- **Password Service** (`password.go`): bcrypt 해싱, 비밀번호 정책 검증
- **RBAC** (`rbac.go`): 역할/권한 정의, 권한 매트릭스, 미들웨어 호환 검증
- **API Key Service** (`apikey.go`): API 키 생성, 검증, 폐기, 만료 관리
- **OAuth2 Integration** (`oauth.go`): Google/GitHub OAuth2, 계정 연동
- **User Model** (`user.go`): 사용자 구조체, 리포지토리 인터페이스
- **Auth Middleware Integration** (`middleware.go`): 다중 전략 인증 인터페이스
- **Audit Logger** (`audit.go`): 인증 이벤트 감사 로깅
- **Auth Info & Stats** (`info.go`): 인증 통계, 세션 추적
- **Error Types** (`errors.go`): 인증/인가 전용 에러 정의

---

## 2. 범위 (Scope)

### 2.1 IN SCOPE (본 SPEC 범위)

- JWT 액세스 토큰(15분) + 리프레시 토큰(7일) 쌍 생성/검증/갱신
- HS256 서명 알고리즘 기반 토큰 서명/검증
- Redis 기반 토큰 블랙리스트 (로그아웃, 비밀번호 변경 시 무효화)
- 인메모리 블랙리스트 폴백 (개발 환경, Redis 미사용 시)
- bcrypt 비밀번호 해싱 (cost factor 12) 및 비밀번호 정책
- RBAC 역할 정의 (Admin, Editor, Viewer) 및 권한 매트릭스
- 리소스 타입별 권한 (Flow, Agent, Node, Plugin, Config, User, System)
- 액션 타입별 권한 (Read, Write, Delete, Execute, Admin)
- API 키 생성/검증/폐기/만료 관리
- OAuth2 Google/GitHub 프로바이더 연동
- 사용자 모델 및 리포지토리 인터페이스
- 인증 미들웨어 인터페이스 (JWT Bearer + API Key 다중 전략)
- 인증 이벤트 감사 로깅
- 인증 관련 에러 타입 정의

### 2.2 OUT OF SCOPE (별도 SPEC)

- API 라우터 및 HTTP 핸들러 구현 (SPEC-API-001: `internal/api/`)
- 데이터베이스 스키마 및 마이그레이션 (SPEC-STORE-001: `internal/storage/`)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001: `internal/observe/`)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: `internal/config/`)
- TLS/HTTPS 서버 설정 (`internal/api/` 서버 레벨)
- 프론트엔드 로그인 UI (`web/`)

---

## 3. 용어 정의 (Terminology)

| 용어 | 정의 |
|------|------|
| Access Token | 짧은 수명(15분)의 JWT, API 요청 인증에 사용 |
| Refresh Token | 긴 수명(7일)의 JWT, 액세스 토큰 갱신에 사용 |
| Token Pair | 액세스 토큰 + 리프레시 토큰의 쌍 |
| Token Blacklist | 로그아웃/무효화된 토큰의 JTI 목록 |
| JTI | JWT Token Identifier, 토큰 고유 식별자 |
| RBAC | Role-Based Access Control, 역할 기반 접근 제어 |
| API Key | 서비스 간 통신 및 CLI 연동을 위한 장기 인증 키 |
| OAuth2 | 외부 인증 제공자(Google, GitHub)를 통한 위임 인증 |
| UserContext | 인증된 사용자의 요청 컨텍스트 (ID, 역할, 권한) |
| Timing-Safe Comparison | 사이드 채널 공격 방지를 위한 상수 시간 비교 |

---

## 4. 의존성 (Dependencies)

### 4.1 외부 의존성

| 패키지 | 버전 | 용도 |
|--------|------|------|
| `github.com/golang-jwt/jwt/v5` | v5.2+ | JWT 토큰 생성/검증 |
| `github.com/redis/go-redis/v9` | v9.4+ | 토큰 블랙리스트 저장소 |
| `golang.org/x/oauth2` | latest | OAuth2 클라이언트 (Google, GitHub) |
| `golang.org/x/crypto/bcrypt` | latest | 비밀번호 해싱 |
| `github.com/google/uuid` | v1.6+ | JTI, API 키 ID 생성 |

### 4.2 내부 의존성

| SPEC ID | 패키지 경로 | 관계 | 설명 |
|---------|------------|------|------|
| SPEC-CFG-001 | `internal/config/` | 소비 | JWT 시크릿, TTL, OAuth 설정 로딩 |
| SPEC-STORE-001 | `internal/storage/` | 소비 | 사용자 영속화, API 키 저장, 감사 로그 |
| SPEC-OBS-001 | `internal/observe/` | 소비 | 구조화된 로깅, 인증 메트릭 수집 |
| SPEC-LIFE-001 | `pkg/lifecycle/` | 소비 | Lifecycle 인터페이스 구현 |
| SPEC-ERR-001 | `pkg/xferr/` | 소비 | 에러 분류 패턴 참조 |

### 4.3 소비자 (Consumed By)

| SPEC ID | 패키지 경로 | 설명 |
|---------|------------|------|
| SPEC-API-001 | `internal/api/` | AuthMiddleware, RBACMiddleware, 인증 핸들러 |

---

## 5. 가정 사항 (Assumptions)

### 5.1 기술적 가정

- A1: `golang-jwt/jwt/v5`는 HS256 서명 알고리즘을 안전하게 구현하며, HMAC-SHA256 표준을 준수한다
- A2: Redis v7+의 `SET key value EX ttl` 명령은 원자적이며, TTL 기반 자동 만료를 보장한다
- A3: `bcrypt.GenerateFromPassword`는 cost factor 12에서 100ms 이내에 완료된다
- A4: `crypto/subtle.ConstantTimeCompare`는 타이밍 사이드 채널 공격을 방지한다
- A5: Redis 연결 실패 시 인메모리 폴백으로 전환하며, 데이터 유실 가능성을 감수한다 (개발 환경)
- A6: OAuth2 프로바이더(Google, GitHub)의 토큰 엔드포인트와 사용자 정보 엔드포인트는 안정적으로 제공된다

### 5.2 비즈니스 가정

- A7: 프로덕션 환경에서 JWT 시크릿은 반드시 설정되어야 하며, 미설정 시 서버 시작을 차단한다
- A8: 기본 역할 체계는 Admin/Editor/Viewer 3단계이며, 커스텀 역할 추가는 향후 확장 범위이다
- A9: API 키는 서비스 간 통신과 CLI 연동에 사용되며, 웹 대시보드 로그인에는 사용하지 않는다
- A10: OAuth2 최초 로그인 시 사용자가 자동 생성되며, 기본 역할은 Viewer이다
- A11: 비밀번호 변경 시 해당 사용자의 모든 기존 토큰을 블랙리스트에 추가한다
- A12: 감사 로그는 `internal/observe/` 패키지의 구조화된 로깅과 별도 DB 테이블 양쪽에 기록한다

---

## 6. Requirements (요구사항)

### Module 1: JWT Token Service (P0) - jwt.go

#### REQ-AUTH-001-01-01 (Ubiquitous) TokenService 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `TokenService` 인터페이스를 제공해야 한다:

- `GenerateAccessToken(userID string, role Role) (string, error)` - 액세스 토큰 생성
- `GenerateRefreshToken(userID string) (string, error)` - 리프레시 토큰 생성
- `GenerateTokenPair(userID string, role Role) (*TokenPair, error)` - 토큰 쌍 생성
- `ValidateToken(tokenString string) (*Claims, error)` - 토큰 검증 및 클레임 추출
- `RefreshTokenPair(refreshToken string) (*TokenPair, error)` - 리프레시 토큰으로 새 쌍 발급

#### REQ-AUTH-001-01-02 (Ubiquitous) JWT Claims 구조체

시스템은 **항상** 다음 필드를 포함하는 `Claims` 구조체를 제공해야 한다:

- `UserID string` - 사용자 고유 식별자
- `Username string` - 사용자명
- `Role Role` - 사용자 역할 (Admin, Editor, Viewer)
- `TokenType TokenType` - 토큰 유형 (Access, Refresh)
- `jwt.RegisteredClaims` 임베딩 (Issuer, ExpiresAt, IssuedAt, ID=JTI)

#### REQ-AUTH-001-01-03 (Event-Driven) 액세스 토큰 생성

**WHEN** `GenerateAccessToken(userID, role)` 호출 시, **THEN** 다음 사양의 JWT 액세스 토큰을 생성해야 한다:

- 서명 알고리즘: HS256
- 만료 시간: 설정값 `auth.jwt.access_ttl` (기본: 15분)
- JTI: UUID v4로 생성
- Issuer: 설정값 `auth.jwt.issuer`
- TokenType: Access
- 서명 키: 설정값 `auth.jwt.secret`

#### REQ-AUTH-001-01-04 (Event-Driven) 리프레시 토큰 생성

**WHEN** `GenerateRefreshToken(userID)` 호출 시, **THEN** 다음 사양의 JWT 리프레시 토큰을 생성해야 한다:

- 서명 알고리즘: HS256
- 만료 시간: 설정값 `auth.jwt.refresh_ttl` (기본: 7일 = 168시간)
- JTI: UUID v4로 생성
- TokenType: Refresh
- Role 필드는 비워둔다 (리프레시 토큰에는 역할 불포함)

#### REQ-AUTH-001-01-05 (Event-Driven) 토큰 검증

**WHEN** `ValidateToken(tokenString)` 호출 시, **THEN** 다음 검증을 순서대로 수행해야 한다:

1. JWT 파싱 및 서명 검증 (HS256, 설정된 시크릿 키)
2. 만료 시간 검증 (`exp` 클레임)
3. 발급자 검증 (`iss` 클레임)
4. 토큰 블랙리스트 확인 (JTI 기반)
5. 모든 검증 통과 시 `Claims` 반환

#### REQ-AUTH-001-01-06 (Event-Driven) 토큰 쌍 갱신

**WHEN** `RefreshTokenPair(refreshToken)` 호출 시, **THEN** 다음 절차를 수행해야 한다:

1. 리프레시 토큰 유효성 검증 (서명, 만료, 블랙리스트)
2. 토큰 타입이 Refresh인지 확인
3. 사용자 정보 조회 (최신 역할 정보 반영)
4. 기존 리프레시 토큰을 블랙리스트에 추가 (재사용 방지)
5. 새 액세스 토큰 + 새 리프레시 토큰 쌍 생성 반환

#### REQ-AUTH-001-01-07 (Unwanted) 프로덕션 시크릿 미설정 방지

시스템은 프로덕션 모드에서 JWT 시크릿이 설정되지 않은 경우 `ErrMissingJWTSecret` 에러를 반환하고 **서비스를 시작하지 않아야** 한다.

#### REQ-AUTH-001-01-08 (State-Driven) 개발 모드 기본 시크릿

**WHILE** 개발 모드(`mode: development`)인 동안, JWT 시크릿이 미설정되면 기본 개발용 시크릿을 사용하되 경고 로그를 출력해야 한다.

---

### Module 2: Token Blacklist (P0) - blacklist.go

#### REQ-AUTH-001-02-01 (Ubiquitous) Blacklist 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Blacklist` 인터페이스를 제공해야 한다:

- `Add(ctx context.Context, jti string, expiration time.Duration) error` - 토큰 JTI 추가
- `IsBlacklisted(ctx context.Context, jti string) (bool, error)` - 블랙리스트 여부 확인
- `AddUserTokens(ctx context.Context, userID string, expiration time.Duration) error` - 사용자의 모든 토큰 무효화

#### REQ-AUTH-001-02-02 (Ubiquitous) Redis 기반 블랙리스트 구현

시스템은 **항상** Redis를 기본 블랙리스트 저장소로 사용해야 한다:

- 키 형식: `blacklist:token:{jti}` (토큰 JTI 기반)
- 값: 블랙리스트 추가 시각 (Unix timestamp)
- TTL: 토큰 남은 유효 시간 (자동 만료로 메모리 관리)
- 사용자 토큰 무효화 키: `blacklist:user:{userID}` (값: 무효화 시각)

#### REQ-AUTH-001-02-03 (State-Driven) 인메모리 폴백

**WHILE** Redis 연결이 불가능한 동안, `sync.Map` 기반 인메모리 블랙리스트로 폴백해야 한다:

- 주기적 만료 항목 정리 (기본: 5분 간격 고루틴)
- Redis 복구 시 인메모리 데이터를 Redis로 동기화
- 개발 모드에서는 인메모리를 기본으로 사용

#### REQ-AUTH-001-02-04 (Event-Driven) 로그아웃 시 블랙리스트 추가

**WHEN** 사용자가 로그아웃 요청 시, **THEN** 해당 액세스 토큰과 리프레시 토큰의 JTI를 블랙리스트에 추가해야 한다.

#### REQ-AUTH-001-02-05 (Event-Driven) 비밀번호 변경 시 전체 토큰 무효화

**WHEN** 사용자가 비밀번호를 변경 시, **THEN** 해당 사용자의 모든 활성 토큰을 무효화해야 한다:

- `blacklist:user:{userID}` 키에 현재 시각 저장
- 토큰 검증 시 발급 시각이 무효화 시각보다 이전이면 거부

---

### Module 3: Password Service (P0) - password.go

#### REQ-AUTH-001-03-01 (Ubiquitous) PasswordService 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `PasswordService` 인터페이스를 제공해야 한다:

- `Hash(password string) (string, error)` - 비밀번호 해싱
- `Verify(password string, hash string) error` - 비밀번호 검증
- `ValidateStrength(password string) error` - 비밀번호 강도 검증

#### REQ-AUTH-001-03-02 (Ubiquitous) bcrypt 해싱

시스템은 **항상** `bcrypt` 알고리즘으로 비밀번호를 해싱해야 한다:

- Cost factor: 12 (고정)
- 평문 비밀번호는 해싱 직후 메모리에서 제거 (Go GC에 위임)

#### REQ-AUTH-001-03-03 (Ubiquitous) 비밀번호 정책

시스템은 **항상** 다음 비밀번호 정책을 적용해야 한다:

- 최소 길이: 8자
- 최대 길이: 128자 (bcrypt 입력 제한)
- 최소 1개의 대문자, 소문자, 숫자 포함

#### REQ-AUTH-001-03-04 (Unwanted) 타이밍 공격 방지

시스템은 비밀번호 검증 시 타이밍 사이드 채널 공격을 **허용하지 않아야** 한다. `bcrypt.CompareHashAndPassword`의 상수 시간 비교 특성을 활용한다.

---

### Module 4: RBAC - Role Based Access Control (P0) - rbac.go

#### REQ-AUTH-001-04-01 (Ubiquitous) Role 타입 정의

시스템은 **항상** 다음 역할 타입을 정의해야 한다:

```
type Role string

const (
    RoleAdmin  Role = "admin"
    RoleEditor Role = "editor"
    RoleViewer Role = "viewer"
)
```

#### REQ-AUTH-001-04-02 (Ubiquitous) Resource 및 Action 타입 정의

시스템은 **항상** 다음 리소스 타입과 액션 타입을 정의해야 한다:

```
type Resource string

const (
    ResourceFlow   Resource = "flow"
    ResourceAgent  Resource = "agent"
    ResourceNode   Resource = "node"
    ResourcePlugin Resource = "plugin"
    ResourceConfig Resource = "config"
    ResourceUser   Resource = "user"
    ResourceSystem Resource = "system"
)

type Action string

const (
    ActionRead    Action = "read"
    ActionWrite   Action = "write"
    ActionDelete  Action = "delete"
    ActionExecute Action = "execute"
    ActionAdmin   Action = "admin"
)
```

#### REQ-AUTH-001-04-03 (Ubiquitous) 권한 매트릭스

시스템은 **항상** 다음 권한 매트릭스를 적용해야 한다:

| 역할 | Flow | Agent | Node | Plugin | Config | User | System |
|------|------|-------|------|--------|--------|------|--------|
| Admin | RWDEA | RWDEA | RWDEA | RWDEA | RWDEA | RWDEA | RWDEA |
| Editor | RWDE | RWDE | RWE | RW | R | R | R |
| Viewer | R | R | R | R | R | - | R |

(R=Read, W=Write, D=Delete, E=Execute, A=Admin)

#### REQ-AUTH-001-04-04 (Ubiquitous) RBACService 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `RBACService` 인터페이스를 제공해야 한다:

- `HasPermission(role Role, resource Resource, action Action) bool` - 권한 보유 여부
- `GetPermissions(role Role) []Permission` - 역할의 전체 권한 목록
- `CheckAccess(role Role, resource Resource, action Action) error` - 접근 검증 (에러 반환)

#### REQ-AUTH-001-04-05 (Event-Driven) 권한 부족 시 거부

**WHEN** `CheckAccess`에서 권한이 부족한 경우, **THEN** `ErrInsufficientPermission` 에러를 반환해야 한다. 에러에는 요청된 역할, 리소스, 액션 정보가 포함된다.

---

### Module 5: API Key Service (P1) - apikey.go

#### REQ-AUTH-001-05-01 (Ubiquitous) APIKeyService 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `APIKeyService` 인터페이스를 제공해야 한다:

- `Generate(ctx context.Context, opts *APIKeyOptions) (*APIKeyResult, error)` - API 키 생성
- `Validate(ctx context.Context, key string) (*APIKeyInfo, error)` - API 키 검증
- `Revoke(ctx context.Context, keyID string) error` - API 키 폐기
- `List(ctx context.Context, userID string) ([]*APIKeyInfo, error)` - 사용자 API 키 목록
- `UpdateLastUsed(ctx context.Context, keyID string) error` - 마지막 사용 시각 갱신

#### REQ-AUTH-001-05-02 (Ubiquitous) API 키 형식

시스템은 **항상** 다음 형식의 API 키를 생성해야 한다:

- 형식: `xf_` + 32바이트 랜덤 값의 hex 인코딩 (총 68자)
- 저장: SHA-256 해시만 저장 (원본 키는 생성 시점에만 반환)
- 접두사 `xf_`는 키 식별 및 로그 감지에 활용

#### REQ-AUTH-001-05-03 (Ubiquitous) API 키 메타데이터

시스템은 **항상** 다음 메타데이터를 API 키와 함께 저장해야 한다:

- `ID string` - 키 고유 식별자 (UUID)
- `Name string` - 키 이름 (사용자 지정)
- `KeyHash string` - SHA-256 해시
- `KeyPrefix string` - 키 접두사 8자 (표시용)
- `OwnerID string` - 소유자 사용자 ID
- `Role Role` - 키에 할당된 역할
- `CreatedAt time.Time` - 생성 시각
- `ExpiresAt *time.Time` - 만료 시각 (nil이면 무기한)
- `LastUsedAt *time.Time` - 마지막 사용 시각
- `Revoked bool` - 폐기 여부

#### REQ-AUTH-001-05-04 (Event-Driven) API 키 검증

**WHEN** `Validate(key)` 호출 시, **THEN** 다음 검증을 수행해야 한다:

1. 접두사(`xf_`) 확인
2. SHA-256 해시 계산 후 저장된 해시와 비교 (timing-safe)
3. 폐기 여부 확인
4. 만료 시각 확인
5. 검증 통과 시 `APIKeyInfo` 반환 및 `LastUsedAt` 갱신

#### REQ-AUTH-001-05-05 (Unwanted) 평문 API 키 저장 금지

시스템은 API 키 원본을 데이터베이스에 **저장하지 않아야** 한다. SHA-256 해시만 저장한다.

---

### Module 6: OAuth2 Integration (P2) - oauth.go

#### REQ-AUTH-001-06-01 (Ubiquitous) OAuth2Service 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `OAuth2Service` 인터페이스를 제공해야 한다:

- `GetAuthURL(provider string, state string) (string, error)` - 인증 URL 생성
- `HandleCallback(ctx context.Context, provider string, code string) (*TokenPair, error)` - 콜백 처리
- `GetUserInfo(ctx context.Context, provider string, token *oauth2.Token) (*OAuthUserInfo, error)` - 사용자 정보 조회

#### REQ-AUTH-001-06-02 (Ubiquitous) OAuth2 프로바이더 팩토리

시스템은 **항상** 프로바이더 팩토리 패턴을 사용하여 OAuth2 프로바이더를 등록/조회해야 한다:

- 기본 프로바이더: Google, GitHub
- 프로바이더별 설정: Client ID, Client Secret, Redirect URL, Scopes

#### REQ-AUTH-001-06-03 (Event-Driven) OAuth2 콜백 처리

**WHEN** OAuth2 콜백이 수신되면, **THEN** 다음 절차를 수행해야 한다:

1. Authorization Code를 Access Token으로 교환
2. 프로바이더에서 사용자 정보 조회 (이메일, 이름, 프로필)
3. 기존 사용자 매핑 확인 (이메일 기반)
4. 신규 사용자인 경우 자동 생성 (기본 역할: Viewer)
5. JWT 토큰 쌍 생성 및 반환

#### REQ-AUTH-001-06-04 (Optional) OAuth2 계정 연동

**가능하면** 기존 로컬 계정에 OAuth2 계정을 연동하는 기능을 제공해야 한다.

---

### Module 7: User Model (P0) - user.go

#### REQ-AUTH-001-07-01 (Ubiquitous) User 구조체

시스템은 **항상** 다음 필드를 포함하는 `User` 구조체를 제공해야 한다:

```
type User struct {
    ID           string    // UUID
    Username     string    // 고유 사용자명
    Email        string    // 고유 이메일
    PasswordHash string    // bcrypt 해시
    Role         Role      // Admin, Editor, Viewer
    Active       bool      // 활성화 상태
    OAuthProvider *string  // OAuth2 프로바이더 (nullable)
    OAuthID      *string   // OAuth2 사용자 ID (nullable)
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

#### REQ-AUTH-001-07-02 (Ubiquitous) UserRepository 인터페이스

시스템은 **항상** 다음 메서드를 포함하는 `UserRepository` 인터페이스를 제공해야 한다:

- `Create(ctx context.Context, user *User) error`
- `GetByID(ctx context.Context, id string) (*User, error)`
- `GetByUsername(ctx context.Context, username string) (*User, error)`
- `GetByEmail(ctx context.Context, email string) (*User, error)`
- `GetByOAuth(ctx context.Context, provider, oauthID string) (*User, error)`
- `Update(ctx context.Context, user *User) error`
- `Delete(ctx context.Context, id string) error`
- `List(ctx context.Context, offset, limit int) ([]*User, int, error)`

#### REQ-AUTH-001-07-03 (Event-Driven) 비밀번호 변경

**WHEN** 사용자가 비밀번호를 변경하면, **THEN** 다음 절차를 수행해야 한다:

1. 현재 비밀번호 검증
2. 새 비밀번호 강도 검증
3. 새 비밀번호 bcrypt 해싱
4. 사용자 레코드 업데이트
5. 해당 사용자의 모든 기존 토큰 블랙리스트 추가

#### REQ-AUTH-001-07-04 (Unwanted) 중복 사용자 방지

시스템은 동일한 Username 또는 Email로 중복 사용자를 **생성하지 않아야** 한다. 중복 시 `ErrDuplicateUser`를 반환한다.

#### REQ-AUTH-001-07-05 (State-Driven) 비활성 사용자 접근 거부

**WHILE** 사용자가 비활성 상태(`Active: false`)인 동안, 시스템은 해당 사용자의 인증 요청을 `ErrUserDisabled` 에러로 거부해야 한다.

---

### Module 8: Auth Middleware Integration (P0) - middleware.go

#### REQ-AUTH-001-08-01 (Ubiquitous) Authenticator 인터페이스

시스템은 **항상** 다음 메서드를 포함하는 `Authenticator` 인터페이스를 제공해야 한다:

- `Authenticate(ctx context.Context, token string, tokenType string) (*UserContext, error)` - 인증 수행
- 지원 토큰 타입: `"bearer"` (JWT), `"apikey"` (API Key)

#### REQ-AUTH-001-08-02 (Ubiquitous) UserContext 구조체

시스템은 **항상** 다음 필드를 포함하는 `UserContext` 구조체를 제공해야 한다:

```
type UserContext struct {
    UserID      string
    Username    string
    Role        Role
    Permissions []Permission
    TokenType   string  // "jwt" 또는 "apikey"
    TokenID     string  // JTI 또는 API Key ID
}
```

#### REQ-AUTH-001-08-03 (Event-Driven) 다중 전략 인증

**WHEN** 인증 요청이 수신되면, **THEN** 다음 순서로 인증을 시도해야 한다:

1. `Authorization: Bearer <token>` 헤더 확인 -> JWT 검증
2. `X-API-Key: <key>` 헤더 확인 -> API 키 검증
3. 모든 방법 실패 시 `ErrInvalidCredentials` 반환

---

### Module 9: Audit Logger (P1) - audit.go

#### REQ-AUTH-001-09-01 (Ubiquitous) AuditService 인터페이스

시스템은 **항상** 다음 메서드를 포함하는 `AuditService` 인터페이스를 제공해야 한다:

- `LogEvent(ctx context.Context, event *AuditEvent) error` - 감사 이벤트 기록

#### REQ-AUTH-001-09-02 (Ubiquitous) AuditEvent 구조체

시스템은 **항상** 다음 필드를 포함하는 `AuditEvent` 구조체를 제공해야 한다:

```
type AuditEvent struct {
    ID        string        // 이벤트 고유 ID
    Timestamp time.Time     // 발생 시각
    UserID    string        // 사용자 ID (인증 전이면 빈 문자열)
    EventType AuditEventType // 이벤트 유형
    IP        string        // 클라이언트 IP
    UserAgent string        // 클라이언트 User-Agent
    Result    AuditResult   // 성공/실패
    Details   string        // 추가 세부 정보 (JSON)
}
```

#### REQ-AUTH-001-09-03 (Ubiquitous) 감사 이벤트 유형

시스템은 **항상** 다음 감사 이벤트 유형을 정의해야 한다:

- `AuditLogin` - 로그인 시도
- `AuditLogout` - 로그아웃
- `AuditTokenRefresh` - 토큰 갱신
- `AuditPasswordChange` - 비밀번호 변경
- `AuditRoleChange` - 역할 변경
- `AuditAPIKeyCreated` - API 키 생성
- `AuditAPIKeyRevoked` - API 키 폐기
- `AuditOAuthLogin` - OAuth2 로그인
- `AuditUserCreated` - 사용자 생성
- `AuditUserDisabled` - 사용자 비활성화

#### REQ-AUTH-001-09-04 (Event-Driven) 인증 이벤트 로깅

**WHEN** 인증 관련 이벤트(로그인, 로그아웃, 토큰 갱신, 비밀번호 변경 등)가 발생하면, **THEN** `AuditService.LogEvent`를 통해 감사 로그를 기록해야 한다:

- `internal/observe/` 패키지의 slog 구조화된 로깅으로 출력
- 별도 감사 로그 테이블에 영속적으로 저장

---

### Module 10: Auth Info & Stats (P1) - info.go

#### REQ-AUTH-001-10-01 (Ubiquitous) AuthInfo 구조체

시스템은 **항상** 다음 정보를 제공하는 `AuthInfo` 구조체를 제공해야 한다:

- `ActiveSessions int` - 활성 세션 수 (추정)
- `BlacklistedTokens int64` - 블랙리스트 토큰 수
- `APIKeyCount int` - 발급된 API 키 수
- `UserCount int` - 등록된 사용자 수

#### REQ-AUTH-001-10-02 (Ubiquitous) AuthStats 구조체

시스템은 **항상** 다음 통계를 제공하는 `AuthStats` 구조체를 제공해야 한다:

- `LoginAttempts int64` - 로그인 시도 횟수 (성공 + 실패)
- `LoginSuccesses int64` - 로그인 성공 횟수
- `LoginFailures int64` - 로그인 실패 횟수
- `TokenRefreshes int64` - 토큰 갱신 횟수
- `OAuthLogins int64` - OAuth 로그인 횟수
- `APIKeyValidations int64` - API 키 검증 횟수

---

### Module 11: Error Types (P0) - errors.go

#### REQ-AUTH-001-11-01 (Ubiquitous) 인증 에러 정의

시스템은 **항상** 다음 sentinel 에러를 정의해야 한다:

```go
var (
    ErrInvalidCredentials     = errors.New("auth: invalid credentials")
    ErrTokenExpired           = errors.New("auth: token expired")
    ErrTokenInvalid           = errors.New("auth: token invalid")
    ErrTokenBlacklisted       = errors.New("auth: token blacklisted")
    ErrRefreshTokenRequired   = errors.New("auth: refresh token required")
    ErrInsufficientPermission = errors.New("auth: insufficient permission")
    ErrUserNotFound           = errors.New("auth: user not found")
    ErrUserDisabled           = errors.New("auth: user disabled")
    ErrAPIKeyExpired          = errors.New("auth: api key expired")
    ErrAPIKeyRevoked          = errors.New("auth: api key revoked")
    ErrOAuthProviderError     = errors.New("auth: oauth provider error")
    ErrPasswordTooWeak        = errors.New("auth: password too weak")
    ErrDuplicateUser          = errors.New("auth: duplicate user")
    ErrMissingJWTSecret       = errors.New("auth: missing jwt secret in production")
)
```

---

## 7. 파일 구조 (File Structure)

```
internal/auth/
├── jwt.go              # JWT TokenService 구현
├── jwt_test.go         # JWT 서비스 테스트
├── blacklist.go        # Token Blacklist (Redis + 인메모리 폴백)
├── blacklist_test.go   # 블랙리스트 테스트
├── password.go         # PasswordService (bcrypt)
├── password_test.go    # 비밀번호 서비스 테스트
├── rbac.go             # RBAC 역할/권한/매트릭스
├── rbac_test.go        # RBAC 테스트
├── apikey.go           # API Key Service
├── apikey_test.go      # API 키 서비스 테스트
├── oauth.go            # OAuth2 Integration (Google, GitHub)
├── oauth_test.go       # OAuth2 테스트
├── user.go             # User 모델, UserRepository 인터페이스
├── user_test.go        # 사용자 모델 테스트
├── middleware.go        # Authenticator 인터페이스, UserContext
├── middleware_test.go   # 미들웨어 인터페이스 테스트
├── audit.go            # AuditService, AuditEvent
├── audit_test.go       # 감사 서비스 테스트
├── info.go             # AuthInfo, AuthStats
├── info_test.go        # 인증 정보/통계 테스트
└── errors.go           # Sentinel 에러 정의
```

---

## 8. 의사 코드 (Interface Definitions)

### 8.1 TokenService

```go
type TokenType string

const (
    TokenTypeAccess  TokenType = "access"
    TokenTypeRefresh TokenType = "refresh"
)

type TokenPair struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int64  `json:"expires_in"` // 초 단위
}

type Claims struct {
    UserID   string    `json:"user_id"`
    Username string    `json:"username"`
    Role     Role      `json:"role,omitempty"`
    Type     TokenType `json:"type"`
    jwt.RegisteredClaims
}

type TokenService interface {
    GenerateAccessToken(userID string, role Role) (string, error)
    GenerateRefreshToken(userID string) (string, error)
    GenerateTokenPair(userID string, role Role) (*TokenPair, error)
    ValidateToken(tokenString string) (*Claims, error)
    RefreshTokenPair(refreshToken string) (*TokenPair, error)
}
```

### 8.2 Blacklist

```go
type Blacklist interface {
    Add(ctx context.Context, jti string, expiration time.Duration) error
    IsBlacklisted(ctx context.Context, jti string) (bool, error)
    AddUserTokens(ctx context.Context, userID string, expiration time.Duration) error
}
```

### 8.3 RBACService

```go
type Permission struct {
    Resource Resource
    Action   Action
}

type RBACService interface {
    HasPermission(role Role, resource Resource, action Action) bool
    GetPermissions(role Role) []Permission
    CheckAccess(role Role, resource Resource, action Action) error
}
```

### 8.4 APIKeyService

```go
type APIKeyOptions struct {
    Name      string
    OwnerID   string
    Role      Role
    ExpiresIn *time.Duration // nil이면 무기한
}

type APIKeyResult struct {
    ID     string // UUID
    Key    string // 원본 키 (생성 시에만 반환)
    Prefix string // 키 접두사 8자
}

type APIKeyInfo struct {
    ID         string
    Name       string
    KeyPrefix  string
    OwnerID    string
    Role       Role
    CreatedAt  time.Time
    ExpiresAt  *time.Time
    LastUsedAt *time.Time
    Revoked    bool
}

type APIKeyService interface {
    Generate(ctx context.Context, opts *APIKeyOptions) (*APIKeyResult, error)
    Validate(ctx context.Context, key string) (*APIKeyInfo, error)
    Revoke(ctx context.Context, keyID string) error
    List(ctx context.Context, userID string) ([]*APIKeyInfo, error)
    UpdateLastUsed(ctx context.Context, keyID string) error
}
```

### 8.5 Authenticator

```go
type UserContext struct {
    UserID      string
    Username    string
    Role        Role
    Permissions []Permission
    TokenType   string // "jwt" 또는 "apikey"
    TokenID     string
}

type Authenticator interface {
    Authenticate(ctx context.Context, token string, tokenType string) (*UserContext, error)
}
```

### 8.6 AuditService

```go
type AuditEventType string

const (
    AuditLogin          AuditEventType = "login"
    AuditLogout         AuditEventType = "logout"
    AuditTokenRefresh   AuditEventType = "token_refresh"
    AuditPasswordChange AuditEventType = "password_change"
    AuditRoleChange     AuditEventType = "role_change"
    AuditAPIKeyCreated  AuditEventType = "apikey_created"
    AuditAPIKeyRevoked  AuditEventType = "apikey_revoked"
    AuditOAuthLogin     AuditEventType = "oauth_login"
    AuditUserCreated    AuditEventType = "user_created"
    AuditUserDisabled   AuditEventType = "user_disabled"
)

type AuditResult string

const (
    AuditSuccess AuditResult = "success"
    AuditFailure AuditResult = "failure"
)

type AuditEvent struct {
    ID        string
    Timestamp time.Time
    UserID    string
    EventType AuditEventType
    IP        string
    UserAgent string
    Result    AuditResult
    Details   string
}

type AuditService interface {
    LogEvent(ctx context.Context, event *AuditEvent) error
}
```

---

## 9. 구현 우선순위 (Priority Matrix)

| 우선순위 | 모듈 | 설명 | 선행 조건 |
|---------|------|------|-----------|
| P0 | Module 11: errors.go | Sentinel 에러 정의 | 없음 |
| P0 | Module 4: rbac.go | 역할/권한 타입 및 매트릭스 | errors.go |
| P0 | Module 7: user.go | User 모델, Repository 인터페이스 | errors.go, rbac.go |
| P0 | Module 3: password.go | bcrypt 해싱, 비밀번호 정책 | errors.go |
| P0 | Module 1: jwt.go | JWT 토큰 생성/검증 | errors.go, rbac.go, SPEC-CFG-001 |
| P0 | Module 2: blacklist.go | 토큰 블랙리스트 | errors.go |
| P0 | Module 8: middleware.go | Authenticator 인터페이스 | jwt.go, rbac.go, errors.go |
| P1 | Module 5: apikey.go | API 키 서비스 | errors.go, rbac.go, user.go |
| P1 | Module 9: audit.go | 감사 로깅 | errors.go, SPEC-OBS-001 |
| P1 | Module 10: info.go | 인증 통계 | jwt.go, blacklist.go, apikey.go |
| P2 | Module 6: oauth.go | OAuth2 연동 | jwt.go, user.go, errors.go |

---

## 10. 성능 목표

| 지표 | 목표값 | 비고 |
|------|--------|------|
| 토큰 검증 (캐시) | < 1ms | 블랙리스트 Redis 조회 포함 |
| 토큰 생성 | < 5ms | HS256 서명 |
| 비밀번호 해싱 | < 200ms | bcrypt cost 12 |
| 비밀번호 검증 | < 200ms | bcrypt 상수 시간 비교 |
| API 키 검증 | < 2ms | SHA-256 해시 + DB 조회 |
| RBAC 권한 확인 | < 0.1ms | 인메모리 매트릭스 조회 |

---

## 11. 보안 고려사항

### 11.1 OWASP 준수

- **A01:2021 Broken Access Control**: RBAC 미들웨어로 모든 API 엔드포인트 보호
- **A02:2021 Cryptographic Failures**: bcrypt 해싱, HS256 JWT 서명, AES-256-GCM 설정 암호화
- **A07:2021 Identification and Authentication Failures**: 토큰 블랙리스트, 비밀번호 정책 강제

### 11.2 추가 보안 조치

- 비밀번호 평문 저장 금지 (bcrypt 해시만 저장)
- API 키 원본 저장 금지 (SHA-256 해시만 저장)
- 타이밍 사이드 채널 공격 방지 (상수 시간 비교)
- 로그아웃 시 토큰 즉시 무효화 (블랙리스트)
- 비밀번호 변경 시 전체 세션 무효화
- JWT 시크릿 미설정 시 프로덕션 시작 차단
- 모든 인증 결정은 감사 로그에 기록

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-13*
*작성: MoAI SPEC Builder*
