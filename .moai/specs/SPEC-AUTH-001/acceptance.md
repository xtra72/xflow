---
id: SPEC-AUTH-001
type: acceptance
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
---

# SPEC-AUTH-001 인수 기준 (Acceptance Criteria)

## Module 1: JWT Token Service

### AC-AUTH-001-01: 액세스 토큰 생성 및 검증

```gherkin
기능: JWT 액세스 토큰 생성 및 검증
  배경:
    주어진 JWT 시크릿이 "test-secret-key-for-jwt-signing-minimum-32-bytes"로 설정되어 있고
    그리고 액세스 토큰 TTL이 15분으로 설정되어 있고
    그리고 발급자(issuer)가 "xflow"로 설정되어 있다

  시나리오: 유효한 액세스 토큰 생성
    주어진 사용자 ID가 "user-001"이고 역할이 "editor"일 때
    만약 GenerateAccessToken을 호출하면
    그러면 HS256으로 서명된 JWT 문자열이 반환되어야 한다
    그리고 토큰의 Claims에 UserID "user-001"이 포함되어야 한다
    그리고 토큰의 Claims에 Role "editor"가 포함되어야 한다
    그리고 토큰의 Claims에 TokenType "access"가 포함되어야 한다
    그리고 토큰의 만료 시간이 현재로부터 15분 후여야 한다
    그리고 토큰의 JTI가 유효한 UUID v4여야 한다

  시나리오: 액세스 토큰 검증 성공
    주어진 유효한 액세스 토큰이 생성되어 있을 때
    만약 ValidateToken을 호출하면
    그러면 Claims 구조체가 반환되어야 한다
    그리고 Claims의 UserID가 원본과 일치해야 한다
    그리고 Claims의 Role이 원본과 일치해야 한다

  시나리오: 만료된 토큰 검증 실패
    주어진 만료된 액세스 토큰이 있을 때
    만약 ValidateToken을 호출하면
    그러면 ErrTokenExpired 에러가 반환되어야 한다

  시나리오: 잘못된 서명의 토큰 검증 실패
    주어진 다른 시크릿으로 서명된 토큰이 있을 때
    만약 ValidateToken을 호출하면
    그러면 ErrTokenInvalid 에러가 반환되어야 한다

  시나리오: 변조된 토큰 검증 실패
    주어진 페이로드가 변조된 토큰이 있을 때
    만약 ValidateToken을 호출하면
    그러면 ErrTokenInvalid 에러가 반환되어야 한다
```

### AC-AUTH-001-02: 리프레시 토큰 생성 및 갱신

```gherkin
기능: JWT 리프레시 토큰 생성 및 토큰 쌍 갱신
  배경:
    주어진 리프레시 토큰 TTL이 168시간(7일)으로 설정되어 있다

  시나리오: 리프레시 토큰 생성
    주어진 사용자 ID가 "user-001"일 때
    만약 GenerateRefreshToken을 호출하면
    그러면 TokenType이 "refresh"인 JWT가 반환되어야 한다
    그리고 만료 시간이 현재로부터 7일 후여야 한다
    그리고 Role 필드가 비어 있어야 한다

  시나리오: 토큰 쌍 생성
    주어진 사용자 ID가 "user-001"이고 역할이 "admin"일 때
    만약 GenerateTokenPair를 호출하면
    그러면 액세스 토큰과 리프레시 토큰이 모두 반환되어야 한다
    그리고 두 토큰의 UserID가 동일해야 한다
    그리고 액세스 토큰의 만료가 리프레시 토큰보다 짧아야 한다

  시나리오: 토큰 쌍 갱신 성공
    주어진 유효한 리프레시 토큰이 있을 때
    만약 RefreshTokenPair를 호출하면
    그러면 새 액세스 토큰과 새 리프레시 토큰이 반환되어야 한다
    그리고 기존 리프레시 토큰이 블랙리스트에 추가되어야 한다
    그리고 새 토큰의 UserID가 기존과 동일해야 한다

  시나리오: 액세스 토큰으로 갱신 시도 실패
    주어진 액세스 토큰(TokenType이 "access")이 있을 때
    만약 RefreshTokenPair를 호출하면
    그러면 ErrRefreshTokenRequired 에러가 반환되어야 한다

  시나리오: 블랙리스트된 리프레시 토큰으로 갱신 실패
    주어진 블랙리스트에 추가된 리프레시 토큰이 있을 때
    만약 RefreshTokenPair를 호출하면
    그러면 ErrTokenBlacklisted 에러가 반환되어야 한다
```

### AC-AUTH-001-03: 프로덕션 모드 JWT 시크릿 검증

```gherkin
기능: 프로덕션 모드 JWT 시크릿 필수 검증
  시나리오: 프로덕션 모드에서 시크릿 미설정 시 시작 차단
    주어진 모드가 "production"이고
    그리고 auth.jwt.secret이 설정되지 않았을 때
    만약 NewTokenService를 호출하면
    그러면 ErrMissingJWTSecret 에러가 반환되어야 한다

  시나리오: 개발 모드에서 시크릿 미설정 시 기본값 사용
    주어진 모드가 "development"이고
    그리고 auth.jwt.secret이 설정되지 않았을 때
    만약 NewTokenService를 호출하면
    그러면 TokenService가 정상적으로 생성되어야 한다
    그리고 경고 로그가 출력되어야 한다
```

---

## Module 2: Token Blacklist

### AC-AUTH-001-04: Redis 기반 블랙리스트

```gherkin
기능: Redis 기반 토큰 블랙리스트
  배경:
    주어진 Redis 블랙리스트 저장소가 초기화되어 있다

  시나리오: 토큰 블랙리스트 추가 및 확인
    주어진 JTI가 "jti-abc-123"인 토큰이 있을 때
    만약 Add("jti-abc-123", 15분)를 호출하면
    그러면 IsBlacklisted("jti-abc-123")가 true를 반환해야 한다

  시나리오: 블랙리스트에 없는 토큰 확인
    주어진 JTI가 "jti-xyz-789"인 토큰이 블랙리스트에 없을 때
    만약 IsBlacklisted("jti-xyz-789")를 호출하면
    그러면 false를 반환해야 한다

  시나리오: TTL 만료 후 자동 제거
    주어진 TTL이 1초인 블랙리스트 항목이 있을 때
    만약 1초 이후 IsBlacklisted를 호출하면
    그러면 false를 반환해야 한다 (자동 만료)

  시나리오: 사용자 전체 토큰 무효화
    주어진 사용자 "user-001"이 여러 토큰을 발급받았을 때
    만약 AddUserTokens("user-001", 7일)를 호출하면
    그러면 해당 사용자의 무효화 시각이 기록되어야 한다
    그리고 무효화 시각 이전에 발급된 모든 토큰이 거부되어야 한다
```

### AC-AUTH-001-05: 인메모리 폴백 블랙리스트

```gherkin
기능: Redis 장애 시 인메모리 폴백
  시나리오: 인메모리 블랙리스트 기본 동작
    주어진 인메모리 블랙리스트가 초기화되어 있을 때
    만약 Add("jti-001", 15분)를 호출하면
    그러면 IsBlacklisted("jti-001")가 true를 반환해야 한다

  시나리오: 인메모리 주기적 정리
    주어진 TTL이 1초인 블랙리스트 항목이 100개 있을 때
    만약 정리 주기(5분) 후 정리 고루틴이 실행되면
    그러면 만료된 항목이 메모리에서 제거되어야 한다
```

---

## Module 3: Password Service

### AC-AUTH-001-06: 비밀번호 해싱 및 검증

```gherkin
기능: bcrypt 비밀번호 해싱 및 검증
  시나리오: 비밀번호 해싱 및 검증 라운드트립
    주어진 비밀번호가 "MySecureP@ss1"일 때
    만약 Hash를 호출하여 해시를 생성하면
    그러면 Verify(원본, 해시)가 nil(성공)을 반환해야 한다

  시나리오: 잘못된 비밀번호 검증 실패
    주어진 비밀번호 "MySecureP@ss1"의 해시가 있을 때
    만약 Verify("WrongPassword1", 해시)를 호출하면
    그러면 에러가 반환되어야 한다

  시나리오: 동일 비밀번호 해싱 시 다른 해시 생성
    주어진 동일한 비밀번호 "MySecureP@ss1"로 두 번 해싱하면
    그러면 두 해시 값이 서로 달라야 한다 (salt 차이)
```

### AC-AUTH-001-07: 비밀번호 강도 검증

```gherkin
기능: 비밀번호 강도 정책 검증
  시나리오 개요: 비밀번호 강도 검증
    주어진 비밀번호가 "<password>"일 때
    만약 ValidateStrength를 호출하면
    그러면 결과가 "<result>"이어야 한다

    예시:
      | password         | result              |
      | Ab1              | ErrPasswordTooWeak  |
      | abcdefgh         | ErrPasswordTooWeak  |
      | ABCDEFGH         | ErrPasswordTooWeak  |
      | 12345678         | ErrPasswordTooWeak  |
      | abcdEFGH         | ErrPasswordTooWeak  |
      | Abcdefg1         | nil (성공)           |
      | MySecureP@ss1    | nil (성공)           |
```

---

## Module 4: RBAC

### AC-AUTH-001-08: 역할 기반 접근 제어

```gherkin
기능: RBAC 권한 매트릭스 검증
  시나리오: Admin은 모든 리소스에 모든 액션 수행 가능
    주어진 역할이 "admin"일 때
    만약 HasPermission(admin, flow, read)를 호출하면
    그러면 true를 반환해야 한다
    만약 HasPermission(admin, user, admin)를 호출하면
    그러면 true를 반환해야 한다
    만약 HasPermission(admin, system, delete)를 호출하면
    그러면 true를 반환해야 한다

  시나리오: Editor는 CRUD+Execute 가능, Admin 액션 불가
    주어진 역할이 "editor"일 때
    만약 HasPermission(editor, flow, write)를 호출하면
    그러면 true를 반환해야 한다
    만약 HasPermission(editor, flow, execute)를 호출하면
    그러면 true를 반환해야 한다
    만약 HasPermission(editor, user, write)를 호출하면
    그러면 false를 반환해야 한다
    만약 HasPermission(editor, flow, admin)를 호출하면
    그러면 false를 반환해야 한다

  시나리오: Viewer는 읽기만 가능
    주어진 역할이 "viewer"일 때
    만약 HasPermission(viewer, flow, read)를 호출하면
    그러면 true를 반환해야 한다
    만약 HasPermission(viewer, flow, write)를 호출하면
    그러면 false를 반환해야 한다
    만약 HasPermission(viewer, user, read)를 호출하면
    그러면 false를 반환해야 한다

  시나리오: 권한 부족 시 에러 반환
    주어진 역할이 "viewer"이고 리소스가 "flow"이고 액션이 "write"일 때
    만약 CheckAccess를 호출하면
    그러면 ErrInsufficientPermission 에러가 반환되어야 한다
    그리고 에러 메시지에 요청된 역할, 리소스, 액션 정보가 포함되어야 한다
```

---

## Module 5: API Key Service

### AC-AUTH-001-09: API 키 생성 및 검증

```gherkin
기능: API 키 전체 라이프사이클
  배경:
    주어진 APIKeyService가 초기화되어 있고
    그리고 사용자 "user-001"이 존재한다

  시나리오: API 키 생성
    주어진 키 이름이 "my-ci-key"이고 역할이 "editor"일 때
    만약 Generate를 호출하면
    그러면 APIKeyResult가 반환되어야 한다
    그리고 Key가 "xf_" 접두사로 시작해야 한다
    그리고 Key의 전체 길이가 68자여야 한다
    그리고 Prefix가 Key의 처음 8자와 일치해야 한다

  시나리오: API 키 검증 성공
    주어진 유효한 API 키가 생성되어 있을 때
    만약 Validate(원본 키)를 호출하면
    그러면 APIKeyInfo가 반환되어야 한다
    그리고 OwnerID가 "user-001"이어야 한다
    그리고 Role이 "editor"여야 한다
    그리고 LastUsedAt이 갱신되어야 한다

  시나리오: 잘못된 API 키 검증 실패
    주어진 존재하지 않는 API 키가 있을 때
    만약 Validate를 호출하면
    그러면 ErrInvalidCredentials 에러가 반환되어야 한다

  시나리오: 접두사 없는 API 키 거부
    주어진 "xf_" 접두사 없는 키가 있을 때
    만약 Validate를 호출하면
    그러면 ErrInvalidCredentials 에러가 반환되어야 한다

  시나리오: API 키 폐기
    주어진 유효한 API 키가 있을 때
    만약 Revoke(keyID)를 호출하면
    그러면 성공을 반환해야 한다
    그리고 이후 Validate 시 ErrAPIKeyRevoked 에러가 반환되어야 한다

  시나리오: 만료된 API 키 거부
    주어진 만료 시각이 지난 API 키가 있을 때
    만약 Validate를 호출하면
    그러면 ErrAPIKeyExpired 에러가 반환되어야 한다

  시나리오: API 키 목록 조회
    주어진 사용자 "user-001"이 3개의 API 키를 생성했을 때
    만약 List("user-001")를 호출하면
    그러면 3개의 APIKeyInfo가 반환되어야 한다
    그리고 각 APIKeyInfo에 원본 키가 포함되지 않아야 한다
```

### AC-AUTH-001-10: API 키 보안

```gherkin
기능: API 키 보안 검증
  시나리오: 원본 키는 생성 시에만 반환
    주어진 API 키를 생성하면
    그러면 Generate 결과에만 원본 Key가 포함되어야 한다
    그리고 List 결과에는 원본 Key가 포함되지 않아야 한다
    그리고 데이터베이스에는 SHA-256 해시만 저장되어야 한다

  시나리오: 키 검증은 timing-safe 비교 사용
    주어진 유효한 API 키와 잘못된 API 키가 있을 때
    만약 각각 Validate를 호출하면
    그러면 두 검증의 처리 시간 차이가 1ms 이내여야 한다
```

---

## Module 6: OAuth2 Integration

### AC-AUTH-001-11: OAuth2 로그인 플로우

```gherkin
기능: OAuth2 소셜 로그인
  배경:
    주어진 Google OAuth2가 설정되어 있고
    그리고 GitHub OAuth2가 설정되어 있다

  시나리오: Google OAuth2 인증 URL 생성
    주어진 프로바이더가 "google"이고 state가 "random-state"일 때
    만약 GetAuthURL을 호출하면
    그러면 Google 인증 URL이 반환되어야 한다
    그리고 URL에 client_id 파라미터가 포함되어야 한다
    그리고 URL에 redirect_uri 파라미터가 포함되어야 한다
    그리고 URL에 state 파라미터가 "random-state"여야 한다

  시나리오: OAuth2 콜백으로 신규 사용자 생성
    주어진 OAuth2 콜백으로 인증 코드가 수신되었고
    그리고 해당 이메일의 사용자가 존재하지 않을 때
    만약 HandleCallback을 호출하면
    그러면 새 사용자가 생성되어야 한다
    그리고 기본 역할이 "viewer"여야 한다
    그리고 TokenPair가 반환되어야 한다

  시나리오: OAuth2 콜백으로 기존 사용자 로그인
    주어진 OAuth2 콜백으로 인증 코드가 수신되었고
    그리고 해당 이메일의 사용자가 이미 존재할 때
    만약 HandleCallback을 호출하면
    그러면 기존 사용자로 TokenPair가 반환되어야 한다
    그리고 새 사용자가 생성되지 않아야 한다

  시나리오: 지원하지 않는 프로바이더 거부
    주어진 프로바이더가 "facebook"일 때
    만약 GetAuthURL을 호출하면
    그러면 ErrOAuthProviderError 에러가 반환되어야 한다
```

---

## Module 7: User Model

### AC-AUTH-001-12: 사용자 관리

```gherkin
기능: 사용자 CRUD 및 비밀번호 변경
  배경:
    주어진 UserRepository가 초기화되어 있다

  시나리오: 중복 사용자 생성 방지
    주어진 Username이 "testuser"인 사용자가 이미 존재할 때
    만약 동일한 Username으로 Create를 호출하면
    그러면 ErrDuplicateUser 에러가 반환되어야 한다

  시나리오: 중복 이메일 생성 방지
    주어진 Email이 "test@example.com"인 사용자가 이미 존재할 때
    만약 동일한 Email로 Create를 호출하면
    그러면 ErrDuplicateUser 에러가 반환되어야 한다

  시나리오: 비활성 사용자 인증 거부
    주어진 Active가 false인 사용자가 있을 때
    만약 해당 사용자의 토큰으로 인증을 시도하면
    그러면 ErrUserDisabled 에러가 반환되어야 한다

  시나리오: 비밀번호 변경 성공
    주어진 사용자 "user-001"이 현재 비밀번호 "OldPass123"을 알고 있을 때
    만약 비밀번호를 "NewPass456"으로 변경하면
    그러면 새 비밀번호 해시가 저장되어야 한다
    그리고 해당 사용자의 모든 기존 토큰이 블랙리스트에 추가되어야 한다
    그리고 AuditPasswordChange 이벤트가 기록되어야 한다

  시나리오: 현재 비밀번호 불일치 시 변경 거부
    주어진 잘못된 현재 비밀번호를 입력했을 때
    만약 비밀번호 변경을 시도하면
    그러면 ErrInvalidCredentials 에러가 반환되어야 한다
```

---

## Module 8: Auth Middleware Integration

### AC-AUTH-001-13: 다중 전략 인증

```gherkin
기능: 다중 인증 전략
  시나리오: JWT Bearer 토큰 인증 성공
    주어진 유효한 JWT 액세스 토큰이 있을 때
    만약 Authenticate(token, "bearer")를 호출하면
    그러면 UserContext가 반환되어야 한다
    그리고 TokenType이 "jwt"여야 한다
    그리고 UserID가 토큰의 UserID와 일치해야 한다

  시나리오: API Key 인증 성공
    주어진 유효한 API 키가 있을 때
    만약 Authenticate(key, "apikey")를 호출하면
    그러면 UserContext가 반환되어야 한다
    그리고 TokenType이 "apikey"여야 한다
    그리고 Permissions에 해당 역할의 권한이 포함되어야 한다

  시나리오: 모든 인증 방법 실패
    주어진 유효하지 않은 토큰이 있을 때
    만약 Authenticate를 호출하면
    그러면 ErrInvalidCredentials 에러가 반환되어야 한다
```

---

## Module 9: Audit Logger

### AC-AUTH-001-14: 감사 이벤트 로깅

```gherkin
기능: 인증 이벤트 감사 로깅
  시나리오: 로그인 성공 이벤트 기록
    주어진 사용자 "user-001"이 성공적으로 로그인했을 때
    만약 LogEvent를 호출하면
    그러면 AuditEvent가 기록되어야 한다
    그리고 EventType이 "login"이어야 한다
    그리고 Result가 "success"여야 한다
    그리고 UserID가 "user-001"이어야 한다
    그리고 IP와 UserAgent가 포함되어야 한다

  시나리오: 로그인 실패 이벤트 기록
    주어진 잘못된 비밀번호로 로그인 시도했을 때
    만약 LogEvent를 호출하면
    그러면 AuditEvent가 기록되어야 한다
    그리고 EventType이 "login"이어야 한다
    그리고 Result가 "failure"여야 한다

  시나리오: 모든 인증 이벤트 유형 기록 가능
    주어진 다음 이벤트 유형이 정의되어 있을 때:
      | 이벤트 유형        |
      | login              |
      | logout             |
      | token_refresh      |
      | password_change    |
      | role_change        |
      | apikey_created     |
      | apikey_revoked     |
      | oauth_login        |
      | user_created       |
      | user_disabled      |
    만약 각 유형으로 LogEvent를 호출하면
    그러면 모든 이벤트가 성공적으로 기록되어야 한다
```

---

## Module 10: Auth Info & Stats

### AC-AUTH-001-15: 인증 통계 수집

```gherkin
기능: 인증 통계 및 정보 수집
  시나리오: AuthInfo 조회
    주어진 인증 시스템이 운영 중일 때
    만약 AuthInfo를 조회하면
    그러면 ActiveSessions, BlacklistedTokens, APIKeyCount, UserCount가 반환되어야 한다
    그리고 모든 값이 0 이상이어야 한다

  시나리오: AuthStats 누적
    주어진 로그인 성공 3회, 실패 2회, 토큰 갱신 1회가 발생했을 때
    만약 AuthStats를 조회하면
    그러면 LoginAttempts가 5여야 한다
    그리고 LoginSuccesses가 3이어야 한다
    그리고 LoginFailures가 2여야 한다
    그리고 TokenRefreshes가 1이어야 한다
```

---

## Module 11: Error Types

### AC-AUTH-001-16: 에러 타입 식별

```gherkin
기능: Sentinel 에러 식별
  시나리오 개요: 에러 타입 식별
    주어진 에러가 "<error_var>"일 때
    만약 errors.Is로 비교하면
    그러면 해당 에러와 일치해야 한다

    예시:
      | error_var                   |
      | ErrInvalidCredentials       |
      | ErrTokenExpired             |
      | ErrTokenInvalid             |
      | ErrTokenBlacklisted         |
      | ErrRefreshTokenRequired     |
      | ErrInsufficientPermission   |
      | ErrUserNotFound             |
      | ErrUserDisabled             |
      | ErrAPIKeyExpired            |
      | ErrAPIKeyRevoked            |
      | ErrOAuthProviderError       |
      | ErrPasswordTooWeak          |
      | ErrDuplicateUser            |
      | ErrMissingJWTSecret         |

  시나리오: 에러 래핑 및 unwrap
    주어진 ErrTokenExpired를 래핑한 에러가 있을 때
    만약 errors.Is로 ErrTokenExpired와 비교하면
    그러면 true를 반환해야 한다
```

---

## 성능 기준 (Performance Criteria)

### AC-AUTH-001-P01: 토큰 검증 성능

```gherkin
기능: 토큰 검증 성능 목표
  시나리오: 액세스 토큰 검증 1ms 이내
    주어진 유효한 액세스 토큰이 있을 때
    만약 ValidateToken을 1000회 반복 호출하면
    그러면 평균 응답 시간이 1ms 미만이어야 한다

  시나리오: 토큰 생성 5ms 이내
    만약 GenerateTokenPair를 1000회 반복 호출하면
    그러면 평균 응답 시간이 5ms 미만이어야 한다

  시나리오: RBAC 권한 확인 0.1ms 이내
    만약 HasPermission을 10000회 반복 호출하면
    그러면 평균 응답 시간이 0.1ms 미만이어야 한다
```

---

## 보안 기준 (Security Criteria)

### AC-AUTH-001-S01: 보안 검증

```gherkin
기능: 보안 요구사항 검증
  시나리오: 비밀번호 평문 저장 금지
    주어진 사용자를 생성하면
    그러면 데이터베이스에 PasswordHash 필드만 존재해야 한다
    그리고 PasswordHash가 "$2a$" 또는 "$2b$"로 시작하는 bcrypt 해시여야 한다

  시나리오: API 키 원본 저장 금지
    주어진 API 키를 생성하면
    그러면 데이터베이스에 KeyHash 필드만 존재해야 한다
    그리고 원본 키 문자열은 저장되지 않아야 한다

  시나리오: 로그에 시크릿 미노출
    주어진 JWT 시크릿이 "my-secret-key"일 때
    만약 인증 관련 로그를 출력하면
    그러면 로그에 시크릿 원본이 포함되지 않아야 한다

  시나리오: 동시성 안전 (race condition 없음)
    만약 10개 고루틴에서 동시에 토큰 생성/검증/블랙리스트 추가를 수행하면
    그러면 go test -race에서 데이터 레이스가 감지되지 않아야 한다

  시나리오: 로그아웃 시 즉시 토큰 무효화
    주어진 사용자가 로그아웃했을 때
    만약 이전 액세스 토큰으로 API를 호출하면
    그러면 ErrTokenBlacklisted 에러가 반환되어야 한다
```

---

## Definition of Done (완료 정의)

- [ ] 모든 인터페이스 정의 완료 (11개 모듈)
- [ ] 모든 구현체에 대한 팩토리 함수 제공
- [ ] 모든 exported 타입에 godoc 주석
- [ ] 테스트 커버리지 90% 이상
- [ ] `go test -race ./internal/auth/...` 통과
- [ ] `go vet ./internal/auth/...` 경고 없음
- [ ] `golangci-lint run ./internal/auth/...` 경고 없음
- [ ] 모든 AC 시나리오에 대응하는 테스트 존재
- [ ] 보안 시나리오 (AC-AUTH-001-S01) 전체 통과
- [ ] 성능 기준 (AC-AUTH-001-P01) 충족
- [ ] SPEC-CFG-001 설정 키 연동 확인
- [ ] SPEC-API-001 미들웨어 인터페이스 호환성 확인

---

*문서 버전: 1.0.0*
*최종 수정: 2026-02-13*
*작성: MoAI SPEC Builder*
