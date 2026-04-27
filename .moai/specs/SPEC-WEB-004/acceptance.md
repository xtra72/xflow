---
id: SPEC-WEB-004
version: "1.0.0"
status: proposed
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
---

# SPEC-WEB-004 수용 기준 (Acceptance Criteria)

본 문서는 Given-When-Then 형식의 검증 가능한 인수 기준을 정의한다. 각 AC는 `spec.md`의 EARS 요구사항 ID와 매핑되며, `plan.md`의 테스트 매트릭스로 구현 경로가 지정된다.

---

## 1. Definition of Done

모든 AC(AC1~AC15)가 PASS이면 본 SPEC은 완료(completed) 상태로 전환된다. 추가 완료 조건:

- `golangci-lint run ./...` 0 warnings
- `go test -race ./...` 0 failures
- 영향받는 패키지 coverage ≥ 85% (quality.yaml 준수)
- `docs/tls-setup.md` 작성 완료
- `examples/config/xflow.yaml` HTTPS 예시 주석 최신화

---

## 2. Acceptance Criteria

### AC1: 기본 동작 유지 (tls.enabled=false)

**매핑 요구사항**: UB-1 (backward compat), ST-2

**Given**
- `server.tls.enabled: false`로 설정된 xflow.yaml

**When**
- `xflowd` 기동 후 `curl http://host:port/api/v1/health` 실행

**Then**
- HTTP 200 응답
- 응답 헤더에 `Strict-Transport-Security` 없음
- 기존 server_test.go 전체 grean
- 신규 TLS 코드 경로는 실행되지 않음 (로그에 `tls.enabled=false`)

---

### AC2: HTTPS 기본 동작 (파일 경로 제공)

**매핑 요구사항**: UB-1, UB-3, UB-4, EV-1

**Given**
- `tls.enabled: true`, `cert_file`/`key_file`에 유효한 PEM 인증서 존재
- `http_mode: "disabled"` (기본값)

**When**
- `xflowd` 기동 후 `curl -k https://host:port/api/v1/health` 실행

**Then**
- HTTP 200 응답
- 응답 헤더에 `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- TLS 핸드셰이크가 TLS 1.2 또는 1.3으로 완료 (`openssl s_client -connect host:port` 확인)
- 기동 로그에 `tls.cert_fingerprint`, `tls.expiry_date`, `tls.http_mode=disabled` 포함

---

### AC3: 자체 서명 인증서 자동 생성

**매핑 요구사항**: EV-1, UN-6

**Given**
- `tls.enabled: true`, `auto_generate: true`
- `cert_file=/tmp/xflow-test.crt`, `key_file=/tmp/xflow-test.key` 경로 존재하지 않음

**When**
- `xflowd` 기동

**Then**
- `/tmp/xflow-test.crt`와 `/tmp/xflow-test.key` 파일이 디스크에 생성됨
  - cert 퍼미션: `0644`, key 퍼미션: `0600`
- x509 파싱 시 SAN에 `localhost`, `127.0.0.1`, `::1`이 포함됨
- 유효기간: 기동 시각부터 `self_signed.validity_days`(기본 365)일
- 서버 로그 WARN 레벨:
  - `self-signed TLS certificate generated (see docs/tls-setup.md)`
  - `tls.cert_fingerprint=<sha256-hex>`, SAN 목록, `tls.expiry_date`
- HTTPS 서빙 성공 (`curl -k https://host:port/health` → 200)

---

### AC4: 인증서 부재 + auto_generate=false → 기동 거부

**매핑 요구사항**: ST-1, UN-1

**Given**
- `tls.enabled: true`, `auto_generate: false`
- `cert_file`, `key_file` 경로 존재하지 않음

**When**
- `xflowd` 기동 시도

**Then**
- 프로세스는 non-zero exit code로 종료
- stderr/log에 명확한 오류 메시지:
  - `tls init: cert file not found: <path>` 또는 동등한 메시지
  - 해결 안내: `set tls.auto_generate=true or provide valid cert/key paths`
- 평문 HTTP로 폴백하지 않음 (평문 리스너도 열리지 않음)

---

### AC5: http_mode=disabled (HTTPS 단독)

**매핑 요구사항**: UB-1, EV-5

**Given**
- `tls.enabled: true`, `http_mode: "disabled"`, `server.port=8443`

**When**
- 기동 후 `curl http://host:8080/health` (평문 8080 포트 접근 시도)

**Then**
- `connection refused` 오류 (8080 포트에서 리스닝하지 않음)
- `curl -k https://host:8443/health` → 200 OK

---

### AC6: http_mode=dual (HTTPS + HTTP 병행)

**매핑 요구사항**: EV-5

**Given**
- `tls.enabled: true`, `http_mode: "dual"`, `server.port=8443`, `http_port=8080`

**When**
- `curl http://host:8080/health` 및 `curl -k https://host:8443/health` 실행

**Then**
- 두 요청 모두 HTTP 200 반환
- HTTPS 응답에만 `Strict-Transport-Security` 헤더 존재
- HTTP 응답에는 HSTS 헤더 없음 (평문 응답에 HSTS는 무의미)
- 두 리스너가 동일 라우터 체인을 공유 (동일 JSON body 반환)

---

### AC7: http_mode=redirect (HTTP→HTTPS 301)

**매핑 요구사항**: EV-4

**Given**
- `tls.enabled: true`, `http_mode: "redirect"`, `server.port=8443`, `http_port=8080`

**When**
- `curl -I "http://host:8080/foo/bar?x=1&y=2"` 실행

**Then**
- 응답 상태: `301 Moved Permanently`
- 응답 헤더: `Location: https://host:8443/foo/bar?x=1&y=2` (host, path, query 모두 보존)
- Body는 비어 있거나 최소 크기
- `curl -k -L "http://host:8080/foo?x=1"` (리다이렉트 따라감) → 최종 200 OK

**추가 검증**:
- Fragment(`#section`)는 HTTP 요청에 전달되지 않으므로 서버에서 재구성하지 않음 (클라이언트 측)

---

### AC8: 포트 충돌 검증 (http_port == server.port)

**매핑 요구사항**: UN-2

**Given**
- `tls.enabled: true`, `http_mode: "dual"` (또는 `"redirect"`), `server.port=8443`, `http_port=8443`

**When**
- `xflowd` 기동 시도

**Then**
- 프로세스는 config validation 단계에서 실패 (리스너 생성 이전)
- 오류 메시지: `config: server.tls.http_port must differ from server.port (both=8443)`
- non-zero exit code
- 어떤 포트도 바인딩되지 않음

---

### AC9: 인증서 핫 리로드 (무중단 교체)

**매핑 요구사항**: EV-2, UB-5

**Given**
- `tls.enabled: true`, `reload.enabled: true`, `reload.debounce_ms: 500`
- xflowd 기동 중, 초기 cert의 fingerprint `FP_OLD`
- 신규 cert/key 쌍 준비 완료 (`new.crt`, `new.key`)

**When**
- `cp new.crt <cert_file> && cp new.key <key_file>` 실행
- 1초 대기 후 `openssl s_client -connect host:port </dev/null | openssl x509 -fingerprint -sha256 -noout` 실행

**Then**
- 500ms 이내 reload 로그 기록: `tls certificate reloaded`, `tls.cert_fingerprint=<FP_NEW>`, `tls.reload_trigger=Write`
- 새 핸드셰이크는 `FP_NEW` 반환
- 서버 프로세스 PID 변경 없음 (재기동 없음)
- 기동 중인 기존 연결(keep-alive)은 끊기지 않고 유지
- 신규 fingerprint가 이전과 다름 (FP_NEW != FP_OLD)

---

### AC10: HSTS 헤더 추가 (HTTPS only)

**매핑 요구사항**: UB-3

**Given**
- `tls.enabled: true`, `http_mode: "dual"`, `http_port: 8080`

**When**
- `curl -I -k https://host:8443/health` 및 `curl -I http://host:8080/health` 실행

**Then**
- HTTPS 응답 헤더에 **반드시** 존재: `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- HTTP 응답 헤더에는 **존재하지 않음** (평문 응답에 HSTS는 RFC 6797 권고에 따라 무효)

---

### AC11: 프론트엔드 wss:// 연결

**매핑 요구사항**: UB-2

**Given**
- 백엔드 `tls.enabled: true`, HTTPS로 Web UI 서빙
- 브라우저에서 `https://host:8443/dashboard` 로드

**When**
- Dashboard 차트 패널이 WebSocket 채널을 열 때 `useChartChannel.ts` `deriveWsBaseUrl()` 호출

**Then**
- `window.location.protocol === 'https:'` → `wss:` 반환
- WebSocket URL: `wss://host:8443/ws/chart/<channel>` 로 연결 성공
- 브라우저 개발자 도구 Network 탭에서 WebSocket 스테이터스 101 확인
- 기존 `web/src/services/ws/chartChannel.test.ts`, `useChartChannels.test.tsx` 전체 grean 유지 (변경 없음)

---

### AC12: 인증서 만료 경고 (30일 이내)

**매핑 요구사항**: EV-3, OP-3

**Given**
- `tls.enabled: true`
- 로드된 인증서의 `NotAfter`가 현재 시각 + 20일

**When**
- xflowd 기동 후 만료 검사 루프가 첫 틱 수행

**Then**
- WARN 레벨 로그 기록:
  - 메시지: `TLS certificate expires soon`
  - 필드: `tls.cert_path`, `tls.expiry_date`(RFC3339), `tls.days_until_expiry=20`, `tls.expiry_warn_days=30`
- 동일 경고는 24시간 내 1회만 기록 (로그 홍수 방지)
- 다음날 동일 조건이면 다시 1회 기록

---

### AC13: TLS 1.1 클라이언트 거부

**매핑 요구사항**: UB-4, UN-4

**Given**
- `tls.enabled: true`, `min_version: "1.2"` (기본값)

**When**
- `openssl s_client -connect host:8443 -tls1_1 </dev/null` 실행

**Then**
- 핸드셰이크 실패: `alert protocol version` 또는 동등한 에러
- HTTP 레이어까지 요청이 도달하지 않음
- 서버 로그(DEBUG 레벨)에 handshake 실패 기록 (감사용)
- WARN/ERROR 레벨로는 기록하지 않음 (운영 로그 오염 방지)

**추가 검증**:
- `-tls1_2` 및 `-tls1_3` 은 정상 동작

---

### AC14: 키 파일 퍼미션 경고

**매핑 요구사항**: UN-5

**Given**
- `tls.enabled: true`, 유효한 cert/key 파일 존재
- 키 파일 퍼미션이 `0644` (world-readable)

**When**
- xflowd 기동

**Then**
- 기동은 정상 진행 (에러 아님)
- WARN 레벨 로그:
  - 메시지: `tls key file permissions are too open (recommended 0600)`
  - 필드: `tls.key_file=<path>`, `mode=0644`
- 서버는 키 파일 퍼미션을 **자동 변경하지 않음** (운영자가 명시적으로 수정해야 함)

**추가 검증**:
- 키 파일 퍼미션이 `0600`이면 경고 없음

---

### AC15: 핫 리로드 실패 시 기존 인증서 유지

**매핑 요구사항**: ST-3, UN-1

**Given**
- 서버 기동 중, 현재 fingerprint `FP_OLD` 사용
- 신규 cert 파일로 덮어쓰지만 내용이 손상(PEM 파싱 불가)

**When**
- `echo "garbage" > <cert_file>` 실행
- 500ms 디바운스 후 reload 시도

**Then**
- ERROR 레벨 로그:
  - 메시지: `tls certificate reload failed, keeping previous cert`
  - 필드: `tls.cert_path`, `err=<파싱 에러 메시지>`
- `atomic.Pointer`는 swap되지 않음 → 새 핸드셰이크는 여전히 `FP_OLD` 반환
- 서버는 계속 정상 동작 (HTTPS 응답 유지)
- 다음 변경 이벤트가 도착할 때까지 자동 재시도 없음
- cert 파일을 정상 PEM으로 다시 교체하면 정상 reload 수행 (복구 가능)

---

## 3. 추가 운영 검증 (선택)

### AC16 (Optional): 만료된 인증서 로드 시 기동 실패 여부

**주의**: 본 SPEC 범위 외. Go `crypto/tls`는 `NotAfter` 과거인 인증서도 로드하며, 클라이언트가 거부한다. 본 SPEC은 기동을 차단하지 않으나 로그에 `tls.days_until_expiry=<음수>` 형식으로 경고만 기록한다.

### AC17 (Optional): reload.enabled=false일 때 파일 교체 무시

**매핑 요구사항**: ST-4

**Given**
- `reload.enabled: false`

**When**
- cert 파일 교체

**Then**
- 리로드 동작하지 않음 (fsnotify watcher가 기동되지 않음)
- 기동 로그: `tls reload disabled`

---

## 4. 테스트 자동화 요약

| AC | 자동화 수준 | 도구 |
|----|------------|------|
| AC1 | 완전 자동 | Go 기존 server_test.go |
| AC2 | 완전 자동 | httptest + `net/http.Client` |
| AC3 | 완전 자동 | tmp dir + `NewCertManager` 통합 테스트 |
| AC4 | 완전 자동 | NewCertManager 에러 검증 |
| AC5 | 완전 자동 | net.Dial to 8080 → refused 확인 |
| AC6 | 완전 자동 | 두 클라이언트 병렬 요청 |
| AC7 | 완전 자동 | httptest redirect + Location 파싱 |
| AC8 | 완전 자동 | validate.go 단위 테스트 |
| AC9 | 완전 자동 (테스트) / 수동 검증 권장 | tmp cert 생성 + `os.Rename` + polling fingerprint |
| AC10 | 완전 자동 | HTTP Header assertion |
| AC11 | 기존 테스트 유지 + 수동 브라우저 검증 | Vitest (기존) + 수동 Chrome DevTools |
| AC12 | 완전 자동 | 시간 주입 (`time.Now` 대체) |
| AC13 | 완전 자동 | `tls.Config.MaxVersion=VersionTLS11` 클라이언트 |
| AC14 | 완전 자동 | tmp key with 0644 + log hook |
| AC15 | 완전 자동 | 파손 PEM 쓰기 + reload 호출 |

---

## 5. 수용 기준 실행 환경

- **OS**: macOS (Darwin 25.x), Linux (Ubuntu 24.04+)
  - Windows는 기본 지원이나 fsnotify 차이로 AC9는 별도 수동 검증 권장
- **Go**: 1.25.6
- **브라우저**(AC11): Chrome 120+, Firefox 123+, Safari 17+ (최신 안정판)
- **openssl**: 3.0+ (AC13)
- **curl**: 8.0+

---

## 6. 회귀 방지 (Regression Guard)

본 SPEC 완료 후 다음 항목을 CI 파이프라인에 포함하여 회귀를 방지한다:

- `go test ./internal/config/... ./internal/api/...` (race detector 포함)
- `golangci-lint run ./...`
- 프론트엔드 `npm run test` (기존 테스트 + 신규 AC11 커버)
- `examples/config/xflow.yaml`에 대한 YAML 스키마 lint (존재 시)

---

*SPEC ID: SPEC-WEB-004*
*버전: 1.0.0*
*상태: proposed*
*최종 수정: 2026-04-21*
