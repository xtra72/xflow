---
id: SPEC-AUTH-001
type: plan
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
---

# SPEC-AUTH-001 구현 계획 (Implementation Plan)

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드 (TDD for new code): 모든 코드가 신규 작성이므로 RED-GREEN-REFACTOR 사이클을 적용한다.

- **RED**: 각 인터페이스에 대한 테스트를 먼저 작성
- **GREEN**: 테스트를 통과하는 최소 구현 작성
- **REFACTOR**: 코드 품질 개선 (중복 제거, 명확한 네이밍)

### 1.2 설계 원칙

- **인터페이스 우선**: 모든 공개 API를 인터페이스로 정의하여 테스트 용이성 확보
- **의존성 주입**: 구현체는 팩토리 함수를 통해 생성하며, 테스트 시 mock 주입
- **캡슐화**: 구현 구조체는 unexported, 인터페이스만 exported
- **보안 최우선**: OWASP 가이드라인 준수, 보안 테스트 필수
- **설정 외부화**: 모든 매직 넘버는 SPEC-CFG-001 설정값으로 외부화

---

## 2. 파일별 구현 계획

### 2.1 Phase 1: 기반 타입 (P0 - 최우선)

#### errors.go (~60줄)
- sentinel 에러 변수 14개 정의
- 에러 래핑 헬퍼 함수 (에러에 컨텍스트 추가)
- 테스트: errors_test.go에서 에러 식별 및 래핑 검증

#### rbac.go (~200줄)
- `Role`, `Resource`, `Action` 타입 상수 정의
- `Permission` 구조체
- 역할-권한 매트릭스 (`map[Role]map[Resource][]Action`)
- `RBACService` 인터페이스 + `rbacService` 구현체
- `HasPermission`, `GetPermissions`, `CheckAccess` 메서드
- 팩토리 함수: `NewRBACService() RBACService`
- 테스트: 모든 역할/리소스/액션 조합 검증 (테이블 기반 테스트)

#### user.go (~150줄)
- `User` 구조체 정의
- `UserRepository` 인터페이스 정의 (8개 메서드)
- `UserContext` 구조체 정의
- `TokenType` 타입 및 상수
- `TokenPair` 구조체
- 테스트: 구조체 생성 및 인터페이스 계약 검증

### 2.2 Phase 2: 핵심 인증 (P0)

#### password.go (~120줄)
- `PasswordService` 인터페이스 + `passwordService` 구현체
- `Hash`: bcrypt.GenerateFromPassword (cost 12)
- `Verify`: bcrypt.CompareHashAndPassword
- `ValidateStrength`: 길이, 대소문자, 숫자 검증
- 팩토리 함수: `NewPasswordService() PasswordService`
- 테스트: 해싱 라운드트립, 강도 검증 (유효/무효 케이스), 타이밍 일관성

#### jwt.go (~280줄)
- `Claims` 구조체 (jwt.RegisteredClaims 임베딩)
- `TokenService` 인터페이스 + `tokenService` 구현체
- 의존성: `Config` (시크릿, TTL, 발급자), `Blacklist`, `UserRepository`
- `GenerateAccessToken`: HS256 서명, Claims 구성, 토큰 문자열 반환
- `GenerateRefreshToken`: 리프레시 전용 Claims, 긴 TTL
- `GenerateTokenPair`: 액세스 + 리프레시 동시 생성
- `ValidateToken`: 파싱, 서명 검증, 만료, 발급자, 블랙리스트 확인
- `RefreshTokenPair`: 리프레시 검증 -> 기존 블랙리스트 -> 새 쌍 생성
- 프로덕션 모드 시크릿 검증 (ErrMissingJWTSecret)
- 팩토리 함수: `NewTokenService(cfg AuthConfig, bl Blacklist, repo UserRepository) (TokenService, error)`
- 테스트: 생성/검증 라운드트립, 만료 검증, 블랙리스트 검증, 갱신 플로우

#### blacklist.go (~220줄)
- `Blacklist` 인터페이스
- `redisBlacklist` 구현체: Redis SET/GET/EXISTS, TTL 자동 만료
- `memoryBlacklist` 구현체: sync.Map, 주기적 정리 고루틴
- `AddUserTokens`: 사용자 단위 무효화 (user:{id} 키)
- 팩토리 함수: `NewRedisBlacklist(client *redis.Client) Blacklist`
- 팩토리 함수: `NewMemoryBlacklist() Blacklist`
- 테스트: 추가/확인 라운드트립, TTL 만료, 사용자 무효화, 동시성

#### middleware.go (~100줄)
- `Authenticator` 인터페이스
- `authenticator` 구현체: TokenService + APIKeyService 통합
- `Authenticate`: Bearer 토큰 -> JWT 검증, API 키 -> 키 검증, 순차 시도
- `UserContext` 생성 및 반환
- 팩토리 함수: `NewAuthenticator(ts TokenService, aks APIKeyService, rbac RBACService) Authenticator`
- 테스트: JWT 인증, API 키 인증, 실패 시나리오, 다중 전략

### 2.3 Phase 3: 확장 서비스 (P1)

#### apikey.go (~250줄)
- `APIKeyOptions`, `APIKeyResult`, `APIKeyInfo` 구조체
- `APIKeyService` 인터페이스 + `apiKeyService` 구현체
- `Generate`: crypto/rand 32바이트 생성, xf_ 접두사, SHA-256 해시 저장
- `Validate`: 접두사 확인, 해시 비교 (timing-safe), 만료/폐기 확인
- `Revoke`: 폐기 플래그 설정
- `List`: 사용자별 키 목록 조회
- `UpdateLastUsed`: 마지막 사용 시각 갱신
- 의존성: `APIKeyRepository` (storage에 위임), `RBACService`
- 테스트: 생성/검증 라운드트립, 폐기, 만료, 접두사 검증

#### audit.go (~130줄)
- `AuditEventType`, `AuditResult` 타입 및 상수
- `AuditEvent` 구조체
- `AuditService` 인터페이스 + `auditService` 구현체
- `LogEvent`: slog 구조화된 로깅 + DB 저장 (비동기)
- 의존성: `ComponentLogger` (SPEC-OBS-001), `AuditRepository`
- 팩토리 함수: `NewAuditService(logger ComponentLogger, repo AuditRepository) AuditService`
- 테스트: 이벤트 로깅 검증, 비동기 저장 검증

#### info.go (~80줄)
- `AuthInfo`, `AuthStats` 구조체
- `AuthInfoService` 인터페이스 + 구현체
- 의존성: Blacklist, APIKeyService, UserRepository
- 테스트: 통계 집계 검증

### 2.4 Phase 4: OAuth2 (P2)

#### oauth.go (~300줄)
- `OAuthUserInfo` 구조체
- `OAuth2Service` 인터페이스 + `oauth2Service` 구현체
- `OAuthProvider` 인터페이스 + Google/GitHub 프로바이더 구현
- 프로바이더 팩토리: `NewOAuthProvider(name string, cfg OAuthConfig) OAuthProvider`
- `GetAuthURL`: state 파라미터 + CSRF 보호
- `HandleCallback`: code -> token -> userinfo -> JWT pair
- `GetUserInfo`: 프로바이더별 사용자 정보 API 호출
- 계정 연동 로직 (이메일 기반 매핑)
- 테스트: Mock HTTP 서버로 OAuth 플로우 검증

---

## 3. 마일스톤 단계

### Milestone 1: 기반 타입 정의 (Primary Goal)

**범위**: errors.go, rbac.go, user.go

**목표**:
- 모든 에러 타입 정의 완료
- 역할/권한 타입 및 매트릭스 정의 완료
- User 모델 및 Repository 인터페이스 정의 완료
- 테스트 커버리지: 95%+

**완료 조건**:
- `go test ./internal/auth/... -count=1` 통과
- 모든 exported 타입에 대한 godoc 주석
- 순환 의존성 없음

### Milestone 2: 핵심 인증 서비스 (Primary Goal)

**범위**: password.go, jwt.go, blacklist.go, middleware.go

**선행 조건**: Milestone 1 완료

**목표**:
- 비밀번호 해싱/검증 구현
- JWT 토큰 생성/검증/갱신 구현
- 토큰 블랙리스트 구현 (Redis + 인메모리)
- 다중 전략 인증 미들웨어 구현
- 테스트 커버리지: 90%+

**완료 조건**:
- 전체 인증 플로우 통합 테스트 통과
- 보안 시나리오 테스트 (만료, 블랙리스트, 비밀번호 정책)
- `go test -race` 통과 (동시성 안전)

### Milestone 3: 확장 서비스 (Secondary Goal)

**범위**: apikey.go, audit.go, info.go

**선행 조건**: Milestone 2 완료

**목표**:
- API 키 전체 라이프사이클 구현
- 감사 로깅 시스템 구현
- 인증 통계 수집 구현
- 테스트 커버리지: 90%+

**완료 조건**:
- API 키 생성/검증/폐기 통합 테스트 통과
- 감사 이벤트 로깅 검증
- 통계 집계 정확성 검증

### Milestone 4: OAuth2 연동 (Optional Goal)

**범위**: oauth.go

**선행 조건**: Milestone 2 완료

**목표**:
- Google OAuth2 연동 구현
- GitHub OAuth2 연동 구현
- 계정 연동 로직 구현
- 테스트 커버리지: 85%+

**완료 조건**:
- Mock OAuth 서버 기반 통합 테스트 통과
- 신규 사용자 자동 생성 검증
- 계정 연동 시나리오 검증

---

## 4. 의존성 그래프

```
errors.go (에러 타입 정의)
    │
    ├─── rbac.go (역할/권한 정의)
    │       │
    │       └─── user.go (User 모델, Repository)
    │               │
    │               ├─── jwt.go (TokenService)
    │               │       │
    │               │       └─── blacklist.go (Blacklist)
    │               │
    │               ├─── apikey.go (APIKeyService)
    │               │
    │               └─── oauth.go (OAuth2Service)
    │
    ├─── password.go (PasswordService)
    │
    ├─── middleware.go (Authenticator)
    │       │
    │       ├─── jwt.go
    │       └─── apikey.go
    │
    ├─── audit.go (AuditService)
    │       │
    │       └─── SPEC-OBS-001 (ComponentLogger)
    │
    └─── info.go (AuthInfoService)
            │
            ├─── blacklist.go
            ├─── apikey.go
            └─── user.go

외부 의존성:
    SPEC-CFG-001 (internal/config/) ──── jwt.go, oauth.go
    SPEC-STORE-001 (internal/storage/) ── user.go, apikey.go, audit.go
    SPEC-OBS-001 (internal/observe/) ──── audit.go, jwt.go
    SPEC-LIFE-001 (pkg/lifecycle/) ────── (전체 Lifecycle 인터페이스)
```

---

## 5. 위험 분석 (Risk Analysis)

### Risk 1: Redis 가용성

**설명**: 프로덕션 환경에서 Redis 장애 시 토큰 블랙리스트 동작 불가
**영향도**: High
**발생 가능성**: Medium
**대응 전략**:
- 인메모리 폴백 구현으로 서비스 연속성 보장
- Redis 재연결 시 인메모리 데이터 동기화
- 헬스체크 엔드포인트에서 Redis 상태 모니터링
- Redis Sentinel 또는 Cluster 모드 권장

### Risk 2: JWT 시크릿 유출

**설명**: JWT 서명 시크릿이 노출될 경우 임의의 유효 토큰 생성 가능
**영향도**: Critical
**발생 가능성**: Low
**대응 전략**:
- 환경 변수 또는 시크릿 매니저를 통한 시크릿 주입
- 로그에 시크릿 노출 방지 (마스킹)
- 정기적 시크릿 로테이션 절차 문서화
- 프로덕션 모드에서 시크릿 미설정 시 시작 차단

### Risk 3: bcrypt 성능

**설명**: bcrypt cost factor 12는 요청당 100-200ms 소요, 높은 동시 로그인 시 병목
**영향도**: Medium
**발생 가능성**: Medium
**대응 전략**:
- 로그인 요청에 레이트 리밋 적용 (SPEC-API-001)
- 고루틴 풀로 bcrypt 처리 제한
- 모니터링으로 지연 시간 추적

### Risk 4: OAuth2 프로바이더 장애

**설명**: Google/GitHub 인증 서비스 장애 시 OAuth 로그인 불가
**영향도**: Medium
**발생 가능성**: Low
**대응 전략**:
- 타임아웃 설정으로 무한 대기 방지
- 로컬 계정 로그인을 대체 수단으로 제공
- OAuth 프로바이더 상태를 헬스체크에 포함

### Risk 5: 토큰 재사용 공격

**설명**: 리프레시 토큰이 탈취되어 반복적으로 새 토큰 쌍 발급
**영향도**: High
**발생 가능성**: Medium
**대응 전략**:
- 리프레시 토큰 사용 시 즉시 블랙리스트 추가 (1회용)
- 새 리프레시 토큰 발급 (Rotation)
- 비정상 갱신 패턴 감지 및 전체 세션 무효화

### Risk 6: API 키 무차별 대입

**설명**: API 키 추측을 통한 무차별 대입 공격
**영향도**: High
**발생 가능성**: Low
**대응 전략**:
- 32바이트 랜덤 (256비트 엔트로피)으로 추측 불가능
- API 키 검증 실패 시 레이트 리밋 적용
- 실패 패턴 감지 및 IP 차단
- SHA-256 해시 저장으로 DB 유출 시에도 원본 복원 불가

### Risk 7: 감사 로그 저장 부하

**설명**: 대량의 인증 이벤트 발생 시 감사 로그 저장소에 부하
**영향도**: Low
**발생 가능성**: Medium
**대응 전략**:
- 감사 로그 저장을 비동기(채널 기반)로 수행
- 배치 삽입으로 DB 쓰기 최적화
- 오래된 감사 로그 아카이빙 정책 수립

---

## 6. 파일 의존성 매트릭스

| 파일 | errors | rbac | user | password | jwt | blacklist | apikey | oauth | middleware | audit | info |
|------|--------|------|------|----------|-----|-----------|--------|-------|------------|-------|------|
| errors.go | - | | | | | | | | | | |
| rbac.go | R | - | | | | | | | | | |
| user.go | R | R | - | | | | | | | | |
| password.go | R | | | - | | | | | | | |
| jwt.go | R | R | R | | - | R | | | | | |
| blacklist.go | R | | | | | - | | | | | |
| apikey.go | R | R | R | | | | - | | | | |
| oauth.go | R | R | R | | R | | | - | | | |
| middleware.go | R | R | | | R | | R | | - | | |
| audit.go | R | | | | | | | | | - | |
| info.go | | | R | | | R | R | | | | - |

(R = 읽기 의존)

---

## 7. 테스트 전략

### 7.1 단위 테스트

- 모든 인터페이스에 대한 mock 생성
- 테이블 기반 테스트 (table-driven tests) 사용
- 보안 시나리오 필수 테스트 (만료, 블랙리스트, 권한 거부)
- `go test -race` 로 동시성 안전 검증

### 7.2 통합 테스트

- Redis 블랙리스트: testcontainers 또는 miniredis 사용
- 전체 인증 플로우: 로그인 -> 토큰 발급 -> API 호출 -> 토큰 갱신 -> 로그아웃
- OAuth2: httptest.Server로 Mock OAuth 프로바이더

### 7.3 커버리지 목표

| 파일 | 목표 커버리지 |
|------|-------------|
| errors.go | 100% |
| rbac.go | 95%+ |
| user.go | 90%+ |
| password.go | 95%+ |
| jwt.go | 90%+ |
| blacklist.go | 90%+ |
| middleware.go | 90%+ |
| apikey.go | 90%+ |
| audit.go | 85%+ |
| info.go | 85%+ |
| oauth.go | 85%+ |
| **전체** | **90%+** |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-13*
*작성: MoAI SPEC Builder*
