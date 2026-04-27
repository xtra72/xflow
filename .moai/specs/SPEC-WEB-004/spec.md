---
id: SPEC-WEB-004
version: "1.0.0"
status: proposed
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
priority: high
dependencies:
  - SPEC-WEB-001
  - SPEC-WEB-002
  - SPEC-WEB-003
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-21 | 1.0.0 | 초기 SPEC 작성 - Web UI HTTPS/TLS 지원 (파일 경로 + 자체 서명 fallback, 핫 리로드, http_mode 3종) |

---

# SPEC-WEB-004: Web UI HTTPS 지원 및 TLS 설정

## 1. Environment (환경)

### 1.1 시스템 개요

xflowd 서버의 Web UI와 REST/WebSocket API는 현재 평문 HTTP로만 서비스되고 있다. 운영 환경에서 인증 토큰, 제어 명령, 로그 스트림이 평문으로 전달되므로 중간자 공격과 자격 증명 탈취에 취약하다. 본 SPEC은 기존 `internal/config.TLSConfig` 스켈레톤을 서버에 실제 연결하여 TLS 1.2 이상으로 Web UI와 API를 서빙하고, 파일 기반 인증서와 자체 서명 fallback, 핫 리로드, HTTP 모드 전환(비활성/병행/리다이렉트)을 통합 지원한다.

프론트엔드 WebSocket 클라이언트는 이미 `window.location.protocol`을 기반으로 `ws://` ↔ `wss://`를 자동 선택한다(`useChartChannel.ts`, `useChartChannels.ts`, `services/ws/wsClient.ts`). 본 SPEC에서는 백엔드 리스너가 TLS로 전환될 때 기존 프론트엔드가 무수정으로 동작함을 검증하고, 하드코딩된 `ws://` 사용처가 남아있는지 감사(audit)한다.

### 1.2 기술 환경

- **백엔드**: Go 1.25.6, `xflowd` 데몬 (`cmd/xflowd/main.go`)
- **HTTP 서버**: `internal/api/server.go` — `net.Listen("tcp", addr)` + `http.Server.Serve(ln)` 사용 (현재 평문 전용)
- **설정 스켈레톤**: `internal/config/types.go` — `TLSConfig{Enabled, CertFile, KeyFile}` 정의되어 있으나 서버가 참조하지 않음
- **프론트엔드**: `web/src/`, Vite 빌드, 이미 `wss://` 자동 전환 구현
- **파일 감시**: `github.com/fsnotify/fsnotify v1.9.0` — `go.mod`에 포함, 재사용
- **표준 라이브러리**: `crypto/tls`, `crypto/x509`, `crypto/ecdsa`, `crypto/rsa`, `crypto/rand`, `encoding/pem`
- **예시 설정**: `examples/config/xflow.yaml` — `server.tls` 블록 이미 존재
- **의존 SPEC**:
  - SPEC-WEB-001: Web UI 핵심 기능 (완료)
  - SPEC-WEB-002: WebSocket 모니터링 브로드캐스팅 (완료)
  - SPEC-WEB-003: Agent Observer 로그 스트리밍 (완료)

### 1.3 설계 원칙

- **기존 스키마 확장**: `TLSConfig` 구조체를 확장하며 기존 3개 필드(`Enabled`, `CertFile`, `KeyFile`)는 의미와 타입을 유지한다.
- **최소 변경 원칙**: `server.go`의 `Start()`에 TLS 분기를 추가하고, TLS 로직은 신규 파일(`tlsconfig.go`, `httpredirect.go`)에 격리한다.
- **백워드 호환성**: `tls.enabled=false`(기본값)인 경우 기존 평문 HTTP 동작과 바이트 수준으로 동일하다.
- **보안 우선**: TLS 1.2 미만 거부, HSTS 기본 활성화, 키 파일 권한 검증, OWASP 권고 사항 준수.
- **무중단 인증서 교체**: `tls.Config.GetCertificate` 콜백과 `atomic.Pointer[tls.Certificate]`를 이용해 핸드셰이크마다 최신 인증서를 사용한다.
- **운영 친화성**: 자체 서명 fallback으로 개발/테스트 환경을 즉시 지원하되, 프로덕션 경고 로그와 문서로 리스크를 명시한다.

### 1.4 범위 / 비범위(Non-Goals)

**IN SCOPE (본 SPEC 범위)**:
- `TLSConfig` 확장 및 설정 검증(`internal/config`)
- HTTPS 리스너 구성 (TLS 1.2+ 강제, HSTS 헤더 추가)
- 파일 경로 기반 인증서 로드 + 자체 서명 자동 생성 fallback
- fsnotify 기반 인증서/키 핫 리로드 (500ms 디바운스, 실패 시 기존 인증서 유지)
- `tls.http_mode` 3종: `disabled`, `dual`, `redirect`
- `wsClient.ts`, `useChartChannel`, `useChartChannels` 등 프론트엔드 WS 경로 감사 및 문서화
- `docs/tls-setup.md` 신규 문서 (openssl 자체 서명, Let's Encrypt 힌트, 브라우저 신뢰 절차)
- `examples/config/xflow.yaml` 주석 확장

**OUT OF SCOPE (비범위)**:
- ACME/Let's Encrypt 자동 발급 (별도 SPEC, Future Work)
- mTLS (클라이언트 인증서 검증)
- 사이드카/외부 TLS 종단 (Nginx/Envoy 위임)
- HSTS 외 추가 보안 헤더 (CSP, X-Frame-Options 등은 별도 SPEC)
- 인증서 투명성(CT) 로그 감시
- HTTP/3 (QUIC) 지원
- 사이퍼 스위트 개별 선택 UI
- 인증서 발급 API 엔드포인트
- CLI 플래그 신설 (설정 파일이 단일 소스)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| TLS | Transport Layer Security. 본 SPEC에서는 TLS 1.2/1.3만 허용 |
| HSTS | HTTP Strict Transport Security. 브라우저가 HTTPS 연결을 강제하도록 지시하는 응답 헤더 |
| Self-signed Certificate | CA 서명 없이 자기 키로 서명한 X.509 인증서. 브라우저는 기본 신뢰하지 않음 |
| GetCertificate Callback | `tls.Config.GetCertificate(*ClientHelloInfo) (*Certificate, error)` — 매 핸드셰이크마다 호출되는 콜백 |
| http_mode | TLS 활성화 시 부가 HTTP 리스너 동작 선택값. `disabled`/`dual`/`redirect` |
| Hot Reload | 서버 재기동 없이 디스크 상의 인증서 파일 변경을 감지하여 메모리의 TLS 인증서를 교체하는 동작 |
| SAN | Subject Alternative Name. 인증서에 포함되는 추가 호스트명/IP 목록 |
| Debounce | 짧은 시간 내 반복되는 이벤트(예: 편집기 저장)를 하나로 묶는 기법. 본 SPEC은 500ms 기본 |
| atomic Pointer | `sync/atomic.Pointer[T]` — 포인터를 원자적으로 swap 하여 동시성 안전하게 교체 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: `github.com/fsnotify/fsnotify v1.9.0`이 `go.mod`에 존재하며 macOS/Linux/Windows에서 `Write|Create|Rename` 이벤트를 제공한다.
- A-002: 운영자는 `cert_file`과 `key_file` 경로에 프로세스 사용자 권한으로 읽기(필요 시 쓰기)가 가능한 파일시스템 위치를 제공한다.
- A-003: Go 표준 라이브러리 `crypto/tls`가 `GetCertificate` 콜백과 TLS 1.3을 지원한다.
- A-004: 프론트엔드 빌드 산출물은 `server.port`와 동일한 리스너에서 서빙된다(리버스 프록시 없음).
- A-005: `useChartChannels.test.tsx`에서 사용하는 `ws://test/...` 고정 URL은 테스트 픽스처이며 `window.location`에 의존하지 않는다.
- A-006: Vite 개발 서버(`web/vite.config.ts`)는 HTTP로 동작하며 개발 중에는 백엔드도 HTTP로 기동한다는 관행을 유지한다 (문서로 명시).

### 3.2 운영 가정

- A-007: 자체 서명 인증서는 개발/사내 테스트용이며, 프로덕션은 CA 발급 인증서(파일 경로 제공)를 사용한다.
- A-008: 단일 xflowd 프로세스당 동시 HTTPS 연결은 최대 수백 개 수준으로 Go 기본 TLS 구현 성능으로 충분하다.
- A-009: 인증서 교체는 시간당 수회 이하로 드물게 발생하며 500ms 디바운스는 편집기 저장 flapping을 커버한다.
- A-010: 운영자는 키 파일 권한을 `0600`으로 설정하는 관행을 따르며, 경고 로그만으로 충분하다(기동 차단 아님).

---

## 4. Requirements (요구사항)

EARS 규칙 ID 체계: `UB` (Ubiquitous, 시스템이 항상 만족해야 하는 속성), `EV` (Event-driven, 이벤트 발생 시 반응), `ST` (State-driven, 특정 상태에서의 불변조건), `UN` (Unwanted, 금지/차단), `OP` (Optional, 향후/선택 기능).

### 4.1 Ubiquitous Requirements (항상 참)

#### UB-1 (Ubiquitous)
시스템은 `server.tls.enabled=true`일 때 `server.port`에서 **항상** HTTPS(TLS 1.2 이상)로 Web UI, REST API, WebSocket 엔드포인트를 서빙해야 한다.

#### UB-2 (Ubiquitous)
프론트엔드는 **항상** 현재 페이지 스킴(`window.location.protocol`)을 기반으로 `ws://` 또는 `wss://` WebSocket URL을 자동 선택해야 한다 (`useChartChannel`, `useChartChannels`, `services/ws/wsClient.ts` 이미 구현; 신규/기타 채널도 동일 규칙 준수).

#### UB-3 (Ubiquitous)
시스템은 HTTPS 활성화 상태에서 **항상** 응답 헤더 `Strict-Transport-Security: max-age=31536000; includeSubDomains`를 추가해야 한다.

#### UB-4 (Ubiquitous)
시스템은 **항상** 최소 TLS 1.2를 강제하며, 설정으로 최소 버전을 TLS 1.3으로 상향할 수 있어야 한다.

#### UB-5 (Ubiquitous)
서버는 **항상** `tls.Config.GetCertificate` 콜백 경로를 통해 매 TLS 핸드셰이크 시 메모리에 보유한 최신 인증서를 반환해야 한다.

### 4.2 Event-Driven Requirements (이벤트 기반)

#### EV-1 (Event-Driven)
**WHEN** 서버가 `tls.enabled=true`로 기동될 때, **THEN** `cert_file`과 `key_file`을 로드하여 파싱 및 유효성을 검증해야 한다. 파일이 존재하지 않고 `auto_generate=true`이면 자체 서명 인증서(기본 ECDSA P-256, 유효기간 365일, SAN = `localhost`, `127.0.0.1`, `::1`, `server.host`)를 생성하여 `cert_file`/`key_file` 경로에 저장하고 로드해야 한다.

#### EV-2 (Event-Driven)
**WHEN** 운영 중 `cert_file` 또는 `key_file`이 변경되면(`fsnotify` `Write|Create|Rename` 이벤트), **THEN** 500ms 디바운스 후 `tls.LoadX509KeyPair`로 재로드하고 성공 시 `atomic.Pointer[tls.Certificate]`를 swap하며 구조화된 로그(`tls.reload_trigger`, `tls.cert_fingerprint`)를 기록해야 한다.

#### EV-3 (Event-Driven)
**WHEN** 로드된 인증서의 `NotAfter`까지 남은 일수가 `tls.expiry_warn_days`(기본 30) 이하인 상태가 지속되면, **THEN** 서버는 하루 1회 WARN 레벨 로그(`tls.expiry_date`, 남은 일수)를 기록해야 한다.

#### EV-4 (Event-Driven)
**WHEN** `tls.http_mode=redirect`로 설정된 상태에서 `tls.http_port`로 HTTP 요청이 도착하면, **THEN** 서버는 동일 Host 및 Path, Query, Fragment를 유지한 `https://` URL로 `301 Moved Permanently` 응답을 반환해야 한다.

#### EV-5 (Event-Driven)
**WHEN** `tls.http_mode=dual`로 설정되면, **THEN** 서버는 HTTPS 리스너(`server.port`)와 평문 HTTP 리스너(`tls.http_port`)를 병렬 고루틴으로 실행하며 두 리스너는 동일 `router.Handler()` 체인을 공유해야 한다.

#### EV-6 (Event-Driven)
**WHEN** 서버가 graceful shutdown을 받으면, **THEN** HTTPS 서버와 부가 HTTP 서버(활성 시)를 `errgroup`으로 병렬 종료하며 `ctx` 데드라인을 공유해야 한다.

### 4.3 State-Driven Requirements (상태 기반)

#### ST-1 (State-Driven)
**IF** `tls.enabled=true`인 상태에서 유효한 인증서/키를 확보하지 못한 경우(파일 없음 + `auto_generate=false`, 또는 자체 서명 생성 실패) **THEN** 서버는 명확한 오류 메시지와 함께 기동을 거부해야 한다.

#### ST-2 (State-Driven)
**IF** `tls.enabled=false`인 상태 **THEN** `cert_file`, `key_file`, `http_mode`, `http_port`, `min_version`, `auto_generate` 등 부속 필드는 무시되어야 하며 설정 검증 시 값이 지정되어 있으면 WARN 로그만 기록해야 한다 (기동은 정상 진행).

#### ST-3 (State-Driven)
**IF** 핫 리로드 중 신규 인증서 파싱에 실패한 경우 **THEN** 기존 로드본을 계속 사용하며 ERROR 로그를 기록하고, 다음 `fsnotify` 변경 이벤트가 도착할 때까지 자동 재시도하지 않아야 한다.

#### ST-4 (State-Driven)
**IF** `tls.enabled=true`이고 `tls.reload.enabled=false`인 상태 **THEN** 서버는 fsnotify 감시자를 기동하지 않고 초기 로드된 인증서만 사용해야 한다.

### 4.4 Unwanted Behavior Requirements (금지/차단)

#### UN-1 (Unwanted)
시스템은 기동 시 cert 파일 파싱에 실패한 경우 **절대** 평문 HTTP로 폴백하지 않아야 하며, 명확한 오류 메시지와 함께 종료되어야 한다. 런타임 핫 리로드 실패 시에는 기존 로드본 유지(ST-3 참조).

#### UN-2 (Unwanted)
시스템은 `tls.http_mode`가 `dual` 또는 `redirect`인 상태에서 `tls.http_port`가 `server.port`와 동일하면 **절대** 기동해서는 안 되며, 설정 검증 단계에서 오류로 거부해야 한다.

#### UN-3 (Unwanted)
시스템은 자체 서명 인증서 자동 생성이 실패하면(디스크 쓰기 실패, 권한 부족, 키 생성 실패 등) **절대** 기동을 계속해서는 안 되며, 실패 원인을 로그로 남기고 종료해야 한다.

#### UN-4 (Unwanted)
시스템은 TLS 1.0/1.1 클라이언트의 연결 시도를 **절대** 수락하지 않아야 하며, 거부 사실은 DEBUG 레벨 감사 로그에만 기록해야 한다(과도한 로그 방지).

#### UN-5 (Unwanted)
시스템은 키 파일(`key_file`)의 퍼미션이 과도하게 개방(`mode & 0077 != 0`)된 경우에도 기동은 진행하되 **반드시** WARN 로그로 권장 퍼미션(`0600`)을 안내해야 한다. 절대 자동으로 퍼미션을 변경하지 않는다.

#### UN-6 (Unwanted)
시스템은 자체 서명 인증서를 생성한 경우 **절대** 로그를 침묵하지 않아야 하며, WARN 레벨로 SHA-256 fingerprint, SAN 목록, 유효기간, 신뢰 안내 문서(`docs/tls-setup.md`) 경로를 기록해야 한다.

### 4.5 Optional Requirements (선택/확장)

#### OP-1 (Optional)
가능하면 `tls.self_signed.additional_sans: []string`으로 자체 서명 인증서의 SAN 목록을 커스터마이징할 수 있어야 한다.

#### OP-2 (Optional)
가능하면 `tls.self_signed.key_type: "ecdsa-p256" | "rsa-4096"`으로 자체 서명 키 알고리즘을 선택할 수 있어야 한다. 기본값은 `ecdsa-p256`.

#### OP-3 (Optional)
가능하면 `tls.min_version: "1.2" | "1.3"`으로 최소 TLS 버전을 설정 가능해야 한다. 기본값은 `"1.2"`.

#### OP-4 (Optional)
가능하면 Prometheus 메트릭 `xflow_tls_cert_expiry_seconds`(만료까지 남은 초)와 `xflow_tls_handshake_errors_total`(핸드셰이크 실패 누적)을 노출할 수 있어야 한다.

#### OP-5 (Optional)
ACME/Let's Encrypt 자동 발급은 향후 작업(SPEC-WEB-005 후보)으로 분리하며, 본 SPEC의 파일 경로 기반 로더와 충돌하지 않도록 확장 가능한 구조로 설계한다.

---

## 5. Specifications (기술 사양)

### 5.1 YAML 설정 스키마 예시

확장된 `server.tls` 블록 (기존 3개 필드 유지 + 신규 필드 추가):

```yaml
server:
  port: 8443               # HTTPS 포트 (TLS 활성화 시)
  host: "0.0.0.0"
  mode: "production"
  tls:
    enabled: true          # (기존) TLS 활성화 스위치
    cert_file: "/etc/xflow/tls/server.crt"  # (기존) PEM 인증서 경로
    key_file:  "/etc/xflow/tls/server.key"  # (기존) PEM 키 경로

    min_version: "1.2"     # (신규) "1.2" (기본) | "1.3"
    http_mode:  "disabled" # (신규) "disabled" (기본) | "dual" | "redirect"
    http_port:  8080       # (신규) dual/redirect 모드에서 사용, server.port와 달라야 함

    auto_generate: false   # (신규) cert/key 부재 시 자체 서명 자동 생성 여부
    expiry_warn_days: 30   # (신규) 만료 경고 임계 일수

    self_signed:           # (신규) 자체 서명 세부 설정
      key_type: "ecdsa-p256"        # "ecdsa-p256" (기본) | "rsa-4096"
      validity_days: 365
      additional_sans: []           # 추가 SAN (예: ["xflow.internal", "10.0.0.5"])

    reload:                # (신규) 핫 리로드 세부 설정
      enabled: true        # fsnotify 감시자 기동 여부
      debounce_ms: 500     # 변경 이벤트 디바운스 (밀리초)
```

### 5.2 http_mode 값별 동작

| `http_mode` | HTTPS 리스너 | HTTP 리스너 | 동작 | 사용 예 |
|-------------|-------------|------------|------|---------|
| `disabled` (기본) | `server.port` 활성 | 비활성 | HTTPS 전용. HTTP 요청은 connection refused | 공개 프로덕션 |
| `dual`      | `server.port` 활성 | `tls.http_port` 활성 | 두 리스너가 동일 핸들러 체인을 공유, 병렬 서빙 | 내부 네트워크 + 외부 HTTPS 병행 |
| `redirect`  | `server.port` 활성 | `tls.http_port` 활성(리다이렉터) | HTTP 요청은 `301` + `Location: https://<Host>:<server.port>/<path>?<query>#<fragment>` | UX 친화적 공개 서비스 |

### 5.3 EARS 요구사항 ID 참조 매트릭스

| 요구사항 ID | 유형 | 구현 위치(예정) | 테스트 ID |
|------------|------|---------------|----------|
| UB-1 | Ubiquitous | `internal/api/server.go` Start() TLS 분기 | AC2, AC5 |
| UB-2 | Ubiquitous | `web/src/services/ws/wsClient.ts`, `useChartChannel[s].ts` (기존) | AC11 |
| UB-3 | Ubiquitous | 신규 HSTS 미들웨어 | AC10 |
| UB-4 | Ubiquitous | `internal/api/tlsconfig.go` MinVersion | AC13 |
| UB-5 | Ubiquitous | `internal/api/tlsconfig.go` GetCertificate | AC9 |
| EV-1 | Event-Driven | `internal/api/tlsconfig.go` loader + self-sign | AC2, AC3 |
| EV-2 | Event-Driven | `internal/api/tlsconfig.go` fsnotify reload | AC9, AC15 |
| EV-3 | Event-Driven | 만료 경고 고루틴 | AC12 |
| EV-4 | Event-Driven | `internal/api/httpredirect.go` | AC7 |
| EV-5 | Event-Driven | `server.go` dual listener goroutine | AC6 |
| EV-6 | Event-Driven | `server.Stop()` 병렬 shutdown | plan.md 테스트 매트릭스 참조 |
| ST-1 | State-Driven | `internal/config/validate.go` + loader | AC4 |
| ST-2 | State-Driven | `internal/config/validate.go` | plan.md 테스트 |
| ST-3 | State-Driven | fsnotify 핸들러 에러 처리 | AC15 |
| ST-4 | State-Driven | fsnotify watcher 시작 조건 | plan.md 테스트 |
| UN-1 | Unwanted | loader 에러 처리 | AC4 |
| UN-2 | Unwanted | `validate.go` 포트 충돌 검증 | AC8 |
| UN-3 | Unwanted | self-sign 에러 처리 | plan.md 테스트 |
| UN-4 | Unwanted | `tls.Config.MinVersion` | AC13 |
| UN-5 | Unwanted | loader 퍼미션 체크 | AC14 |
| UN-6 | Unwanted | self-sign 생성 후 WARN 로그 | AC3 |
| OP-1~5 | Optional | `SelfSignedConfig.AdditionalSANs` 등 | 선택 구현 |

### 5.4 Cross-SPEC 의존성

| 본 SPEC 항목 | 의존 SPEC/시스템 | 의존 내용 |
|-------------|----------------|----------|
| UB-1 (HTTPS 리스너) | SPEC-WEB-001 (완료) | `internal/api/server.go`, `router.Handler()` 체인 |
| UB-2 (WebSocket wss) | SPEC-WEB-002 (완료) | `internal/api/ws/*` 브로드캐스터 채널 |
| UB-2 (WebSocket wss) | SPEC-WEB-003 (완료) | 로그 스트리밍 채널의 프론트엔드 WS 사용 |
| EV-2 (핫 리로드) | `github.com/fsnotify/fsnotify v1.9.0` | `go.mod` 기존 종속성 재사용 |

### 5.5 보안 체크리스트

- TLS 1.2 미만 거부 (`tls.Config.MinVersion = tls.VersionTLS12` 또는 `VersionTLS13`)
- 약한 사이퍼 제외 (Go 1.25 기본 안전 사이퍼 유지)
- 키 파일 권한 검증 (`os.Stat` → `mode & 0077 == 0`)
- TLS 세션 티켓 키는 Go 기본 관리자 사용
- 자체 서명 인증서 발급 시 `crypto/rand.Reader`만 사용
- 인증서 fingerprint(SHA-256) 로그 기록 (운영자 수동 검증용)
- HTTP/2 유지 (`http.Server.TLSNextProto = nil` — Go 기본값 유지)

### 5.6 리스크 테이블

| 리스크 | 확률 | 영향 | 완화 방안 |
|-------|------|------|----------|
| 자체 서명 인증서 브라우저 경고로 운영자 혼란 | 높음 | 중 | WARN 로그 + `docs/tls-setup.md` 링크 제공, 기본값 `auto_generate=false` 유지, 프로덕션은 CA 인증서 권고 |
| fsnotify 다중 트리거(편집기 rename+write)로 반복 리로드 | 높음 | 낮음 | 500ms 디바운스 기본값, 리로드 실패 시 기존 본 유지 |
| Vite 개발 프록시가 HTTP 전용이어서 백엔드 HTTPS와 불일치 | 중 | 중 | `docs/tls-setup.md`에 개발 중 HTTP 모드 유지 권고 명시, 필요 시 `vite.config.ts`에 HTTPS proxy 토글 문서화 |
| 백워드 호환성 깨짐 (기존 YAML 깨짐) | 낮음 | 높음 | 신규 필드 전부 기본값 제공, `enabled=false` 시 기존 경로와 바이트 수준 동일 동작, 통합 테스트로 검증 |
| 키 파일 권한 과다 개방 방치 | 중 | 중 | WARN 로그로 즉시 안내, 문서에 `chmod 0600` 명시, 자동 변경은 하지 않음 |
| http_mode 오설정 (포트 충돌) | 중 | 높음 | 설정 검증 단계에서 즉시 거부 (UN-2) |

---

## 6. Stakeholders (이해관계자)

| 역할 | 관심 사항 |
|------|----------|
| 운영자 (DevOps) | 인증서 교체 절차, 핫 리로드 동작, 만료 경고, 문서화 |
| 보안 담당 | TLS 1.2+ 강제, HSTS, 키 권한 검증, OWASP 권고 준수 |
| 개발자 | 개발 환경에서 자체 서명으로 즉시 검증 가능, 기존 WS 코드 변경 없음 |
| 최종 사용자 | 신뢰 가능한 인증서 사용 시 브라우저 잠금 아이콘, 자격 증명 안전 전송 |

---

## 7. Acceptance Criteria 요약

상세 Given-When-Then 시나리오는 `acceptance.md` 참조. 핵심 15개 기준:

- AC1~AC8: TLS 설정 조합별 기동 동작 (기본/활성/자동생성/모드별)
- AC9, AC15: 핫 리로드 성공/실패 시나리오
- AC10, AC13: 보안(HSTS, TLS 1.1 거부)
- AC11: 프론트엔드 wss:// 연결
- AC12, AC14: 운영성(만료 경고, 퍼미션 경고)

---

## 8. Future Work (향후 작업)

- **SPEC-WEB-005 후보**: ACME/Let's Encrypt 자동 발급 및 갱신
- **SPEC-WEB-006 후보**: mTLS (클라이언트 인증서) — 에이전트/내부 클러스터 통신
- **SPEC-WEB-007 후보**: 추가 보안 헤더 (CSP, X-Frame-Options, Referrer-Policy)
- **SPEC-WEB-008 후보**: HTTP/3 (QUIC) 지원
- **SPEC-WEB-009 후보**: OCSP Stapling
- 설정 가능한 사이퍼 스위트 목록

---

*SPEC ID: SPEC-WEB-004*
*버전: 1.0.0*
*상태: proposed*
*최종 수정: 2026-04-21*
