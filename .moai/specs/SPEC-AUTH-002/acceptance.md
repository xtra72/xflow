---
id: SPEC-AUTH-002
type: acceptance
version: "1.0.0"
created: 2026-03-31
updated: 2026-03-31
author: xtra
---

# SPEC-AUTH-002 인수 기준

## 추적 태그

- `[SPEC-AUTH-002-M1]` - 설정 및 자격증명 관리
- `[SPEC-AUTH-002-M2]` - Auth 미들웨어 및 API
- `[SPEC-AUTH-002-M3]` - 로그인 UI 및 비밀번호 변경

---

## 1. 테스트 시나리오

### TS-001: 인증 비활성화 시 패스스루 동작 `[SPEC-AUTH-002-M2]`

**Given** `server.basic_auth.enabled`가 `false`로 설정되어 있을 때
**When** 인증 토큰 없이 API 엔드포인트를 호출하면
**Then** 기존과 동일하게 200 OK 응답이 반환된다

**Given** `server.basic_auth` 설정이 아예 없을 때
**When** 인증 토큰 없이 API 엔드포인트를 호출하면
**Then** 기존과 동일하게 200 OK 응답이 반환된다 (하위 호환성 보장)

---

### TS-002: 로그인 성공 `[SPEC-AUTH-002-M2]`

**Given** 인증이 활성화되어 있고, `admin`/`admin` 계정이 자격증명 파일에 존재할 때
**When** `POST /api/v1/auth/login`에 `{"username": "admin", "password": "admin"}`을 전송하면
**Then** 응답은 200 OK이고, 응답 본문에 `access_token`, `refresh_token`, `expires_at`, `user` 필드가 포함된다
**And** `user.name`은 `"admin"`이고 `user.role`은 `"admin"`이다
**And** `access_token`은 유효한 JWT 토큰이다

---

### TS-003: 로그인 실패 `[SPEC-AUTH-002-M2]`

**Given** 인증이 활성화되어 있을 때
**When** `POST /api/v1/auth/login`에 잘못된 비밀번호를 전송하면
**Then** 응답은 401 Unauthorized이다
**And** 응답 본문에 "사용자가 없음"과 "비밀번호 틀림"을 구분하는 정보가 포함되지 않는다
**And** 인증 실패 로그가 기록된다

**Given** 인증이 활성화되어 있을 때
**When** `POST /api/v1/auth/login`에 존재하지 않는 사용자명을 전송하면
**Then** 응답은 401 Unauthorized이다
**And** 응답 형식은 잘못된 비밀번호 시와 동일하다 (REQ-N-002)

---

### TS-004: 보호된 엔드포인트 접근 `[SPEC-AUTH-002-M2]`

**Given** 인증이 활성화되어 있을 때
**When** 유효한 JWT 토큰을 `Authorization: Bearer <token>` 헤더에 포함하여 API를 호출하면
**Then** 정상적으로 200 OK 응답이 반환된다
**And** 컨텍스트에 `UserID`와 `UserRole`이 설정된다

**Given** 인증이 활성화되어 있을 때
**When** 토큰 없이 보호된 API 엔드포인트를 호출하면
**Then** 응답은 401 Unauthorized이다

**Given** 인증이 활성화되어 있을 때
**When** 만료된 JWT 토큰으로 API를 호출하면
**Then** 응답은 401 Unauthorized이다

---

### TS-005: 헬스/레디 엔드포인트 면제 `[SPEC-AUTH-002-M2]`

**Given** 인증이 활성화되어 있을 때
**When** 토큰 없이 `GET /health`를 호출하면
**Then** 응답은 200 OK이다

**Given** 인증이 활성화되어 있을 때
**When** 토큰 없이 `GET /ready`를 호출하면
**Then** 응답은 200 OK이다

---

### TS-006: 비밀번호 변경 `[SPEC-AUTH-002-M2]`

**Given** 인증된 사용자(`admin`)가 로그인한 상태일 때
**When** `PUT /api/v1/auth/password`에 `{"current_password": "admin", "new_password": "newpass123"}`을 전송하면
**Then** 응답은 200 OK이다
**And** 이후 기존 비밀번호(`admin`)로 로그인하면 401이 반환된다
**And** 새 비밀번호(`newpass123`)로 로그인하면 성공한다

**Given** 인증된 사용자가 로그인한 상태일 때
**When** 잘못된 현재 비밀번호로 비밀번호 변경을 요청하면
**Then** 응답은 400 Bad Request이다
**And** 비밀번호는 변경되지 않는다

---

### TS-007: 자격증명 파일 자동 생성 `[SPEC-AUTH-002-M1]`

**Given** `server.basic_auth.enabled`가 `true`이고 자격증명 파일이 존재하지 않을 때
**When** xflowd가 시작되면
**Then** 기본 `admin`/`admin` 계정이 포함된 자격증명 파일이 자동 생성된다
**And** 비밀번호는 bcrypt로 해싱되어 저장된다
**And** 경고 로그가 출력된다 ("기본 비밀번호를 변경하세요")

---

### TS-008: 로그아웃 `[SPEC-AUTH-002-M2]`

**Given** 유효한 JWT 토큰으로 인증된 상태일 때
**When** `POST /api/v1/auth/logout`을 호출하면
**Then** 응답은 200 OK이다
**And** 사용된 refresh token으로 `POST /api/v1/auth/refresh`를 호출하면 401이 반환된다

---

### TS-009: 토큰 갱신 `[SPEC-AUTH-002-M2]`

**Given** 유효한 refresh token이 있을 때
**When** `POST /api/v1/auth/refresh`에 `{"refresh_token": "<token>"}`을 전송하면
**Then** 새로운 `access_token`과 `refresh_token`이 반환된다

**Given** 만료되거나 무효화된 refresh token으로
**When** `POST /api/v1/auth/refresh`를 호출하면
**Then** 응답은 401 Unauthorized이다

---

### TS-010: WebSocket 인증 `[SPEC-AUTH-002-M2]`

**Given** 인증이 활성화되어 있을 때
**When** 유효한 JWT 토큰을 쿼리 파라미터(`?token=<jwt>`)로 포함하여 WebSocket 연결을 시도하면
**Then** WebSocket 핸드셰이크가 성공한다

**Given** 인증이 활성화되어 있을 때
**When** 토큰 없이 WebSocket 연결을 시도하면
**Then** WebSocket 핸드셰이크가 실패하고 401 응답이 반환된다

---

### TS-011: 로그인 UI 동작 `[SPEC-AUTH-002-M3]`

**Given** 인증이 활성화되어 있고 사용자가 로그인하지 않은 상태일 때
**When** Web UI의 아무 페이지에 접근하면
**Then** 로그인 페이지(`/login`)로 리다이렉트된다

**Given** 로그인 페이지에서 올바른 자격증명을 입력한 후
**When** 로그인 버튼을 클릭하면
**Then** 원래 요청한 페이지로 리다이렉트된다
**And** 토큰이 로컬 스토리지에 저장된다

**Given** 인증이 비활성화된 상태일 때
**When** Web UI에 접근하면
**Then** 로그인 페이지가 표시되지 않고 바로 대시보드가 표시된다

---

### TS-012: 비밀번호 변경 UI 동작 `[SPEC-AUTH-002-M3]`

**Given** 로그인한 상태에서 Header의 사용자 메뉴를 클릭하면
**When** "비밀번호 변경" 메뉴를 선택하면
**Then** 비밀번호 변경 다이얼로그가 표시된다
**And** 현재 비밀번호, 새 비밀번호, 비밀번호 확인 입력 필드가 있다

**Given** 비밀번호 변경 다이얼로그에서 올바른 현재 비밀번호와 유효한 새 비밀번호를 입력한 후
**When** 변경 버튼을 클릭하면
**Then** 성공 메시지가 표시되고 다이얼로그가 닫힌다

---

## 2. 엣지 케이스

### EC-001: 자격증명 파일 손상

**Given** 자격증명 파일이 유효하지 않은 YAML 형식일 때
**When** xflowd가 시작되면
**Then** 명확한 에러 메시지와 함께 시작이 실패한다 (자동으로 덮어쓰지 않음)

### EC-002: 동시 비밀번호 변경

**Given** 두 세션에서 동시에 같은 사용자의 비밀번호 변경을 요청할 때
**When** 두 요청이 거의 동시에 도착하면
**Then** 하나만 성공하고 다른 하나는 올바르게 처리된다 (데이터 손상 없음)

### EC-003: JWT 시크릿 미설정 상태에서 서버 재시작

**Given** `jwt_secret`이 비어있어 자동 생성된 시크릿을 사용 중일 때
**When** 서버가 재시작되면
**Then** 기존 JWT 토큰은 모두 무효화된다
**And** 사용자는 다시 로그인해야 한다

### EC-004: 자격증명 파일 경로 권한 없음

**Given** 지정된 자격증명 파일 경로에 쓰기 권한이 없을 때
**When** xflowd가 시작되면
**Then** 명확한 에러 메시지와 함께 시작이 실패한다

### EC-005: 빈 비밀번호 또는 짧은 비밀번호

**Given** 비밀번호 변경 시
**When** 빈 문자열 또는 4자 미만의 비밀번호를 새 비밀번호로 제출하면
**Then** 400 Bad Request가 반환되고 비밀번호 요구사항 안내 메시지가 포함된다

---

## 3. 품질 게이트

### Definition of Done

- [ ] 모든 EARS 요구사항(REQ-U, REQ-E, REQ-S, REQ-N)이 구현됨
- [ ] `internal/auth/` 패키지 테스트 커버리지 85% 이상
- [ ] `internal/api/handler/auth.go` 테스트 커버리지 85% 이상
- [ ] `server.basic_auth.enabled: false` 시 기존 동작과 100% 동일 (회귀 없음)
- [ ] 기존 미들웨어 테스트 모두 통과
- [ ] 로그인 페이지가 반응형으로 구현됨
- [ ] i18n 키가 한국어/영어 모두 추가됨
- [ ] `examples/config/xflow.yaml`에 설정 예제 포함
- [ ] 비밀번호가 평문으로 로그에 기록되지 않음 (REQ-N-001 검증)

### 검증 방법

| 항목 | 검증 방법 |
| ---- | --------- |
| bcrypt 해싱 | 단위 테스트: 해싱 후 원문 복원 불가 확인 |
| JWT 토큰 검증 | 단위 테스트: 유효/만료/변조 토큰 각각 검증 |
| 패스스루 호환성 | 기존 미들웨어 통합 테스트 실행 |
| 로그인 UI | 수동 테스트 + 스크린샷 확인 |
| WebSocket 인증 | 통합 테스트: 토큰 유무에 따른 연결 성공/실패 |
| 자격증명 파일 자동 생성 | 통합 테스트: 빈 디렉토리에서 서버 시작 |
| 비밀번호 변경 | E2E 테스트: 변경 전후 로그인 검증 |
