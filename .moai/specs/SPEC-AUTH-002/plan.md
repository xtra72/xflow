---
id: SPEC-AUTH-002
type: plan
version: "1.0.0"
created: 2026-03-31
updated: 2026-03-31
author: xtra
---

# SPEC-AUTH-002 구현 계획

## 추적 태그

- `[SPEC-AUTH-002-M1]` - 설정 및 자격증명 관리
- `[SPEC-AUTH-002-M2]` - Auth 미들웨어 및 API
- `[SPEC-AUTH-002-M3]` - 로그인 UI 및 비밀번호 변경

---

## 1. 마일스톤 개요

### 최우선 목표 (Primary Goal)

**M1: 설정 및 자격증명 관리** `[SPEC-AUTH-002-M1]`
- `BasicAuthConfig` 타입 정의 및 설정 로딩
- 자격증명 파일(`users.yaml`) 로드/저장/검증
- bcrypt 비밀번호 해싱
- 최초 실행 시 기본 admin 계정 자동 생성

**M2-Core: Auth 미들웨어 및 로그인 API** `[SPEC-AUTH-002-M2]`
- Auth 미들웨어 구현 (JWT 토큰 검증)
- 로그인/로그아웃/토큰갱신 API 엔드포인트
- JWT 토큰 발행 및 검증 로직

### 보조 목표 (Secondary Goal)

**M3: 로그인 UI** `[SPEC-AUTH-002-M3]`
- 로그인 페이지 컴포넌트
- ProtectedRoute 래퍼
- 401 응답 시 리다이렉트
- authStore 업데이트

### 최종 목표 (Final Goal)

**M2-Ext: 비밀번호 변경 API** `[SPEC-AUTH-002-M2]`
- 비밀번호 변경 엔드포인트
- WebSocket 인증

**M3-Ext: 비밀번호 변경 UI** `[SPEC-AUTH-002-M3]`
- 비밀번호 변경 다이얼로그
- Header 메뉴에 비밀번호 변경 버튼

### 선택 목표 (Optional Goal)

- Refresh token 자동 갱신 (프론트엔드)
- 계정 잠금 기능

---

## 2. 태스크 분해

### M1: 설정 및 자격증명 관리

| 태스크 ID | 설명 | 파일 | 의존성 |
| --------- | ---- | ---- | ------ |
| M1-T1 | `BasicAuthConfig` 구조체 추가 | `internal/config/types.go` | 없음 |
| M1-T2 | `xflow.yaml`에 `server.basic_auth` 섹션 추가 | `internal/config/config.go` | M1-T1 |
| M1-T3 | `internal/auth/` 패키지 생성 | 새 패키지 | 없음 |
| M1-T4 | 자격증명 파일 로드/저장 구현 | `internal/auth/credentials.go` | M1-T3 |
| M1-T5 | bcrypt 해싱/검증 유틸 | `internal/auth/password.go` | M1-T3 |
| M1-T6 | 기본 admin 계정 자동 생성 로직 | `internal/auth/credentials.go` | M1-T4, M1-T5 |
| M1-T7 | 단위 테스트 | `internal/auth/*_test.go` | M1-T4~T6 |

### M2-Core: Auth 미들웨어 및 로그인 API

| 태스크 ID | 설명 | 파일 | 의존성 |
| --------- | ---- | ---- | ------ |
| M2-T1 | JWT 토큰 생성/검증 서비스 | `internal/auth/jwt.go` | M1 |
| M2-T2 | Auth 미들웨어 구현 (패스스루 교체) | `internal/api/middleware.go` | M2-T1 |
| M2-T3 | Auth 핸들러 생성 | `internal/api/handler/auth.go` | M2-T1 |
| M2-T4 | 로그인 엔드포인트 | `internal/api/handler/auth.go` | M2-T3, M1-T5 |
| M2-T5 | 로그아웃 엔드포인트 | `internal/api/handler/auth.go` | M2-T3 |
| M2-T6 | 토큰 갱신 엔드포인트 | `internal/api/handler/auth.go` | M2-T1 |
| M2-T7 | `/auth/me` 엔드포인트 | `internal/api/handler/auth.go` | M2-T3 |
| M2-T8 | 라우터에 auth 라우트 등록 | `internal/api/server.go` | M2-T3~T7 |
| M2-T9 | 단위 테스트 | `internal/auth/jwt_test.go`, `internal/api/handler/auth_test.go` | M2-T1~T8 |

### M2-Ext: 비밀번호 변경 및 WebSocket

| 태스크 ID | 설명 | 파일 | 의존성 |
| --------- | ---- | ---- | ------ |
| M2-T10 | 비밀번호 변경 엔드포인트 | `internal/api/handler/auth.go` | M2-Core |
| M2-T11 | WebSocket 핸드셰이크 인증 | `internal/api/ws/*.go` | M2-T1 |
| M2-T12 | 단위 테스트 | `internal/api/handler/auth_test.go` | M2-T10~T11 |

### M3: 로그인 UI

| 태스크 ID | 설명 | 파일 | 의존성 |
| --------- | ---- | ---- | ------ |
| M3-T1 | `LoginRequest` 타입 수정 (email -> username) | `web/src/types/auth.ts` | 없음 |
| M3-T2 | authService 업데이트 | `web/src/services/api/authService.ts` | M3-T1 |
| M3-T3 | `LoginPage.tsx` 생성 | `web/src/pages/auth/LoginPage.tsx` | M3-T1, M3-T2 |
| M3-T4 | `ProtectedRoute.tsx` 생성 | `web/src/components/auth/ProtectedRoute.tsx` | 없음 |
| M3-T5 | `authStore.ts` 업데이트 | `web/src/stores/authStore.ts` | M3-T1 |
| M3-T6 | 라우터에 로그인/보호 경로 추가 | `web/src/App.tsx` 또는 라우터 파일 | M3-T3~T5 |
| M3-T7 | 401 인터셉터 리다이렉트 | `web/src/services/api/interceptors.ts` | M3-T5 |

### M3-Ext: 비밀번호 변경 UI

| 태스크 ID | 설명 | 파일 | 의존성 |
| --------- | ---- | ---- | ------ |
| M3-T8 | `ChangePasswordRequest` 타입 추가 | `web/src/types/auth.ts` | 없음 |
| M3-T9 | `changePassword()` API 함수 | `web/src/services/api/authService.ts` | M3-T8 |
| M3-T10 | `ChangePasswordDialog.tsx` 생성 | `web/src/pages/auth/ChangePasswordDialog.tsx` | M3-T8, M3-T9 |
| M3-T11 | Header에 비밀번호 변경 메뉴 추가 | `web/src/components/layout/Header.tsx` | M3-T10 |

---

## 3. 수정 대상 파일 목록

### 새로 생성하는 파일

| 파일 경로 | 설명 |
| --------- | ---- |
| `internal/auth/credentials.go` | 자격증명 파일 관리 |
| `internal/auth/credentials_test.go` | 자격증명 테스트 |
| `internal/auth/password.go` | bcrypt 해싱 유틸리티 |
| `internal/auth/password_test.go` | 비밀번호 해싱 테스트 |
| `internal/auth/jwt.go` | JWT 토큰 생성/검증 |
| `internal/auth/jwt_test.go` | JWT 테스트 |
| `internal/api/handler/auth.go` | 인증 API 핸들러 |
| `internal/api/handler/auth_test.go` | 인증 핸들러 테스트 |
| `web/src/pages/auth/LoginPage.tsx` | 로그인 페이지 |
| `web/src/pages/auth/ChangePasswordDialog.tsx` | 비밀번호 변경 다이얼로그 |
| `web/src/components/auth/ProtectedRoute.tsx` | 인증 보호 라우트 래퍼 |

### 수정하는 파일

| 파일 경로 | 변경 내용 |
| --------- | --------- |
| `internal/config/types.go` | `BasicAuthConfig` 구조체 추가, `ServerConfig`에 필드 추가 |
| `internal/config/config.go` | `server.basic_auth` 기본값 바인딩 |
| `internal/api/middleware.go` | `Auth()` 함수 구현 (패스스루 -> JWT 검증) |
| `internal/api/server.go` | Auth 미들웨어에 config 주입, auth 라우트 등록 |
| `internal/api/ws/*.go` | WebSocket 핸드셰이크 인증 추가 |
| `cmd/xflowd/main.go` | 자격증명 파일 초기화 호출 |
| `examples/config/xflow.yaml` | `server.basic_auth` 예제 추가 |
| `web/src/types/auth.ts` | `LoginRequest` 수정, `ChangePasswordRequest` 추가 |
| `web/src/services/api/authService.ts` | API 함수 업데이트 |
| `web/src/stores/authStore.ts` | 인증 활성화 상태, 토큰 관리 |
| `web/src/services/api/interceptors.ts` | 401 리다이렉트 로직 |
| `web/src/components/layout/Header.tsx` | 사용자 메뉴/비밀번호 변경 버튼 |
| `web/src/lib/i18n/ko.json` | 인증 관련 번역 키 추가 |
| `web/src/lib/i18n/en.json` | 인증 관련 번역 키 추가 |

---

## 4. 기술 접근 방식

### 4.1 아키텍처 설계

```
[Web UI] ---> [Auth Middleware] ---> [API Handlers]
   |               |                      |
   |               v                      v
   |          [JWT Service]          [Auth Handler]
   |               |                      |
   |               v                      v
   |          [Config]             [Credentials Manager]
   |                                      |
   |                                      v
   v                              [users.yaml file]
[Login Page]
```

### 4.2 핵심 설계 결정

**JWT 시크릿 관리**: 설정 파일에 `jwt_secret`이 비어있으면 서버 시작 시 32바이트 랜덤 시크릿을 생성하여 메모리에 보관. 재시작 시 기존 토큰이 무효화되지만, 간단한 배포에서는 허용 가능.

**자격증명 파일 동시 접근**: `sync.RWMutex`로 보호. 비밀번호 변경 시 파일 전체를 다시 쓴다.

**토큰 무효화**: 로그아웃 시 서버 메모리에 블랙리스트 유지 (재시작 시 초기화됨). 단순 배포에서는 충분함.

### 4.3 의존성 패키지

| 패키지 | 용도 | 비고 |
| ------ | ---- | ---- |
| `golang.org/x/crypto/bcrypt` | 비밀번호 해싱 | Go 공식 확장 |
| `github.com/golang-jwt/jwt/v5` | JWT 토큰 | 이미 사용 중이거나 추가 필요 |
| `gopkg.in/yaml.v3` | 자격증명 파일 파싱 | 이미 사용 중 |

---

## 5. 리스크 분석

| 리스크 | 영향도 | 발생 가능성 | 대응 방안 |
| ------ | ------ | ----------- | --------- |
| JWT 시크릿 미설정 시 재시작마다 토큰 무효화 | 중간 | 높음 | 기본 시크릿 자동 생성 + 설정 권장 문서화 |
| 자격증명 파일 권한 문제 | 높음 | 중간 | 시작 시 파일 권한 체크, 600 권한 권장 로그 |
| 기존 Auth 미들웨어 변경으로 인한 회귀 | 높음 | 낮음 | `enabled=false` 시 기존 패스스루 동작 보장, 기존 테스트 유지 |
| 프론트엔드 LoginRequest 타입 변경 | 중간 | 중간 | email -> username 전환 시 기존 코드 영향 분석 |
| 동시 비밀번호 변경 경합 | 낮음 | 낮음 | RWMutex로 파일 접근 직렬화 |

---

## 6. 전문가 상담 권고

이 SPEC은 다음 전문가 에이전트의 상담을 권장한다:

- **expert-backend**: Auth 미들웨어 구현, JWT 서비스 설계, API 엔드포인트 구현
- **expert-frontend**: 로그인 페이지 UI, ProtectedRoute 패턴, authStore 업데이트
- **expert-security**: bcrypt 설정, JWT 보안, 토큰 관리 검토
