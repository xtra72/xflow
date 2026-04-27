---
id: SPEC-WEB-004
version: "1.0.0"
status: proposed
created: "2026-04-21"
updated: "2026-04-21"
author: xtra
---

# SPEC-WEB-004 구현 계획: Web UI HTTPS 지원 및 TLS 설정

## 1. 구현 전략

### 1.1 개발 방법론

`.moai/config/sections/quality.yaml`의 `development_mode: "hybrid"`를 따른다.

- **신규 파일** (TDD: RED-GREEN-REFACTOR):
  - `internal/api/tlsconfig.go`, `tlsconfig_test.go`
  - `internal/api/httpredirect.go`, `httpredirect_test.go`
- **기존 파일 수정** (DDD: ANALYZE-PRESERVE-IMPROVE):
  - `internal/api/server.go`: 캐릭터라이제이션 테스트로 현 평문 동작 고정 후 TLS 분기 추가
  - `internal/config/types.go`, `validate.go`: 기존 필드 시맨틱 유지하며 필드 확장
  - `examples/config/xflow.yaml`: 주석 블록 확장 (기능 영향 없음)

### 1.2 실행 순서 (마일스톤)

**우선순위 High (필수)**:
1. **Milestone M1 — 설정 확장 및 검증**: `TLSConfig` 확장, `validate.go` 규칙 추가, 기존 테스트 grean 유지
2. **Milestone M2 — TLS 로더 및 자체 서명**: `tlsconfig.go`의 cert 로드, self-signed 생성, `GetCertificate` 콜백
3. **Milestone M3 — 서버 통합 (HTTPS 단일 모드)**: `server.go`에서 `disabled` 모드 우선 구현, HSTS 미들웨어

**우선순위 High (필수)**:
4. **Milestone M4 — http_mode 확장**: `dual`, `redirect` 모드 구현, graceful shutdown
5. **Milestone M5 — 핫 리로드**: fsnotify 감시자, 디바운스, atomic swap

**우선순위 Medium (권장)**:
6. **Milestone M6 — 운영성**: 만료 경고 고루틴, 키 권한 WARN, 구조화 로깅
7. **Milestone M7 — 프론트엔드 감사 및 문서**: WS 경로 감사 기록, `docs/tls-setup.md`, README 업데이트 포인터

**우선순위 Low (선택, 본 SPEC OP-*)**:
8. **Milestone M8 — Prometheus 메트릭, 키 알고리즘 선택, TLS 1.3 전용 옵션**

M4는 M3 이후 실행하며 나머지 마일스톤은 M3 이후 독립적으로 병렬 가능하다.

---

## 2. Module별 구현 계획

### 2.1 Module 1: 설정 확장 및 검증 (M1)

**마일스톤**: 기존 `TLSConfig` 3필드를 의미 유지하며 신규 필드를 추가하고, `http_mode`/`http_port` 조합을 검증.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/config/types.go` | modify | `TLSConfig` 필드 확장: `MinVersion`, `HTTPMode`, `HTTPPort`, `AutoGenerate`, `ExpiryWarnDays`, `SelfSigned`, `Reload` |
| `internal/config/types.go` | modify | `SelfSignedConfig`, `ReloadConfig` 서브 구조체 신규 추가 |
| `internal/config/validate.go` | modify | `tls.enabled=true`일 때 `http_mode` enum, `http_port != server.port`, `expiry_warn_days > 0` 검증 |
| `internal/config/types_test.go` | modify | 신규 필드 기본값/역직렬화 테스트 추가 |
| `internal/config/validate_test.go` | modify | http_mode 충돌, enabled=false 시 경고 케이스 추가 |
| `examples/config/xflow.yaml` | modify | 주석 블록 확장 (HTTPS 예시) |

#### 제안 타입 정의 (예시)

```go
// internal/config/types.go (확장안)
type TLSConfig struct {
    Enabled        bool              `yaml:"enabled"         json:"enabled"`
    CertFile       string            `yaml:"cert_file"       json:"cert_file"`
    KeyFile        string            `yaml:"key_file"        json:"key_file"`
    MinVersion     string            `yaml:"min_version"     json:"min_version"`     // "1.2" | "1.3"
    HTTPMode       string            `yaml:"http_mode"       json:"http_mode"`       // "disabled" | "dual" | "redirect"
    HTTPPort       int               `yaml:"http_port"       json:"http_port"`
    AutoGenerate   bool              `yaml:"auto_generate"   json:"auto_generate"`
    ExpiryWarnDays int               `yaml:"expiry_warn_days" json:"expiry_warn_days"`
    SelfSigned     SelfSignedConfig  `yaml:"self_signed"     json:"self_signed"`
    Reload         ReloadConfig      `yaml:"reload"          json:"reload"`
}

type SelfSignedConfig struct {
    KeyType        string   `yaml:"key_type"        json:"key_type"`        // "ecdsa-p256" | "rsa-4096"
    ValidityDays   int      `yaml:"validity_days"   json:"validity_days"`
    AdditionalSANs []string `yaml:"additional_sans" json:"additional_sans"`
}

type ReloadConfig struct {
    Enabled    bool `yaml:"enabled"     json:"enabled"`
    DebounceMs int  `yaml:"debounce_ms" json:"debounce_ms"`
}
```

#### 기본값 (validate.go 내 Normalize/ApplyDefaults 패턴)

| 필드 | 기본값 |
|------|--------|
| `MinVersion` | `"1.2"` |
| `HTTPMode` | `"disabled"` |
| `HTTPPort` | `0` (비활성) |
| `AutoGenerate` | `false` |
| `ExpiryWarnDays` | `30` |
| `SelfSigned.KeyType` | `"ecdsa-p256"` |
| `SelfSigned.ValidityDays` | `365` |
| `SelfSigned.AdditionalSANs` | `nil` |
| `Reload.Enabled` | `true` |
| `Reload.DebounceMs` | `500` |

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| 기존 YAML 파일이 신규 필드 부재로 깨질 수 있음 | 중 | 모든 신규 필드에 zero-value → 기본값 normalization 적용, 기존 테스트 grean 유지 |
| `enabled=false`에서 `http_mode`가 잘못 채워진 경우 | 낮음 | 검증 시 WARN 로그만, 기동은 정상 진행 (ST-2) |

---

### 2.2 Module 2: TLS 로더 및 자체 서명 (M2)

**마일스톤**: 인증서 로드, 검증, 자체 서명 fallback, `GetCertificate` 콜백, 만료 경고를 통합하는 내부 API.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/api/tlsconfig.go` | new | `CertManager` 구조체, 로더, self-sign, GetCertificate 콜백, 만료 검사 |
| `internal/api/tlsconfig_test.go` | new | 테이블 드리븐 테스트 (로드 정상/실패, self-sign 생성, fingerprint, SAN, 만료) |
| `internal/api/tlsconfig_testdata/` | new | 테스트용 PEM (정상 cert/key, 만료 임박 cert, 파손 cert) |

#### 제안 인터페이스 (예시)

```go
// internal/api/tlsconfig.go (신규)

type CertManager struct {
    certFile string
    keyFile  string
    current  atomic.Pointer[tls.Certificate]  // 핸드셰이크마다 참조
    logger   *slog.Logger
    cfg      config.TLSConfig
}

func NewCertManager(cfg config.TLSConfig, logger *slog.Logger) (*CertManager, error) {
    // 1) cert/key 파일 존재 & 파싱 시도
    // 2) 없고 AutoGenerate=true이면 self-sign 후 디스크 기록
    // 3) 없고 AutoGenerate=false이면 에러 반환 (ST-1, UN-1)
    // 4) 키 파일 퍼미션 체크 → WARN (UN-5)
}

func (m *CertManager) BuildTLSConfig() *tls.Config {
    return &tls.Config{
        MinVersion:     minVersionFromString(m.cfg.MinVersion),
        GetCertificate: m.getCertificate,
    }
}

func (m *CertManager) getCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
    return m.current.Load(), nil
}

func (m *CertManager) Reload() error { /* EV-2, ST-3 */ }
func (m *CertManager) CertExpiryCheck() (remaining time.Duration, warn bool) { /* EV-3 */ }
```

#### 자체 서명 생성 알고리즘

1. `cert_file` 존재 & 파싱 가능 → skip (기존 파일 재사용)
2. `auto_generate=false` 이면서 파일 부재 → `ErrTLSCertMissing` 반환
3. `auto_generate=true` 이면서 파일 부재:
   - 키 생성: `KeyType="ecdsa-p256"`이면 `ecdsa.GenerateKey(elliptic.P256(), rand.Reader)`, `"rsa-4096"`이면 `rsa.GenerateKey(rand.Reader, 4096)`
   - `x509.Certificate` 템플릿: CN=`"xflow self-signed"`, NotBefore=now, NotAfter=now+ValidityDays, SAN=`["localhost","127.0.0.1","::1"]` + `server.host`(와일드카드/미지정 제외) + `AdditionalSANs`
   - `x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)` 자기서명
   - PEM 블록 인코딩 후 `cert_file`(0644), `key_file`(0600) 쓰기 (`os.WriteFile`)
   - SHA-256 fingerprint 로그: `WARN` 레벨, 메시지: `self-signed TLS certificate generated (see docs/tls-setup.md)` + `tls.cert_fingerprint`, SAN 목록, 만료일
4. 쓰기 실패 시 `fmt.Errorf("tls: failed to persist self-signed cert: %w", err)` 반환 (UN-3)

#### 키 권한 체크

```go
info, _ := os.Stat(keyFile)
if info.Mode().Perm()&0077 != 0 {
    logger.Warn("tls key file permissions are too open (recommended 0600)",
        "key_file", keyFile, "mode", info.Mode().Perm())
}
// 절대 Chmod 자동 변경하지 않음
```

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| `rand.Reader` 성능 저하 (RSA 4096 수 초 소요) | 낮음 | 기본을 ECDSA P-256으로 설정, RSA는 옵션 |
| Windows에서 `0600` 퍼미션 의미 약함 | 낮음 | 경고 로그는 유지, 문서에 OS별 주석 |
| testdata 관리 (만료 임박 cert 재생성 필요) | 중 | 테스트에서 동적으로 생성하여 hardcoded PEM 의존성 제거 |

---

### 2.3 Module 3: 서버 통합 — HTTPS 단일 모드 (M3)

**마일스톤**: `server.go`의 `Start()`에 TLS 분기를 추가하고 `http_mode=disabled`까지 동작.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/api/server.go` | modify | `Start()` TLS 분기, HSTS 미들웨어 래핑, `s.certManager` 필드 추가 |
| `internal/api/server.go` | modify | `Stop()` graceful shutdown 로직 (ctx deadline 공유) |
| `internal/api/server_test.go` | modify | 기존 평문 테스트 grean 유지 (DDD 캐릭터라이제이션) |
| `internal/api/hsts_middleware.go` | new | `func WithHSTS(next http.Handler) http.Handler` |

#### 핵심 알고리즘 (의사코드)

```go
// internal/api/server.go Start() 발췌
func (s *Server) Start(ctx context.Context) error {
    addr := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
    handler := s.router.Handler()

    if s.config.TLS.Enabled {
        handler = WithHSTS(handler) // UB-3
        cm, err := NewCertManager(s.config.TLS, s.logger)
        if err != nil {
            return fmt.Errorf("tls init: %w", err) // ST-1, UN-1
        }
        s.certManager = cm

        s.httpServer = &http.Server{
            Handler:   handler,
            TLSConfig: cm.BuildTLSConfig(),
            // 기타 타임아웃 기존과 동일
        }

        // http_mode 분기 (Module 4 참조)
        // disabled: HTTPS만 리스닝
        ln, err := tls.Listen("tcp", addr, s.httpServer.TLSConfig)
        if err != nil { return err }
        return s.httpServer.Serve(ln)
    }

    // 기존 평문 경로 (바이트 수준 동일 동작 보장)
    ln, err := net.Listen("tcp", addr)
    if err != nil { return err }
    return s.httpServer.Serve(ln)
}
```

#### HSTS 미들웨어 (신규)

```go
// internal/api/hsts_middleware.go
func WithHSTS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Strict-Transport-Security",
            "max-age=31536000; includeSubDomains")
        next.ServeHTTP(w, r)
    })
}
```

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| 기존 `server_test.go`가 평문 의존 | 중 | `tls.enabled=false` 경로를 그대로 유지, 신규 테스트는 HTTPS 분기만 대상 |
| `httptest.NewTLSServer`와의 충돌 | 낮음 | 통합 테스트는 `httptest.NewUnstartedServer` + 직접 `StartTLS` 사용 |

---

### 2.4 Module 4: http_mode 확장 (M4)

**마일스톤**: `dual`, `redirect` 모드 구현, 두 리스너의 graceful shutdown.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/api/server.go` | modify | `dual`/`redirect` 분기, `errgroup`으로 두 리스너 병렬 실행 및 종료 |
| `internal/api/httpredirect.go` | new | HTTP → HTTPS 301 리다이렉트 핸들러 |
| `internal/api/httpredirect_test.go` | new | 리다이렉트 Location/Status/Query/Fragment 보존 테스트 |

#### 리다이렉트 핸들러 (제안)

```go
// internal/api/httpredirect.go (신규)
func NewHTTPSRedirectHandler(httpsPort int) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        host := r.Host
        if i := strings.IndexByte(host, ':'); i >= 0 {
            host = host[:i]
        }
        target := "https://" + net.JoinHostPort(host, strconv.Itoa(httpsPort)) + r.URL.RequestURI()
        http.Redirect(w, r, target, http.StatusMovedPermanently)
    })
}
```

#### 이중 리스너 실행 (의사코드)

```go
// dual / redirect 모드에서
g, gctx := errgroup.WithContext(ctx)

// HTTPS 리스너
g.Go(func() error {
    return s.httpServer.ServeTLS(httpsLn, "", "") // GetCertificate 사용으로 빈 문자열 OK
})

// HTTP 리스너 (dual: 같은 핸들러 / redirect: 리다이렉트 핸들러)
httpHandler := handler
if s.config.TLS.HTTPMode == "redirect" {
    httpHandler = NewHTTPSRedirectHandler(s.config.Port)
}
s.httpPlainServer = &http.Server{Handler: httpHandler}
g.Go(func() error {
    return s.httpPlainServer.Serve(plainLn)
})

return g.Wait()
```

#### graceful shutdown

```go
func (s *Server) Stop(ctx context.Context) error {
    g, gctx := errgroup.WithContext(ctx)
    g.Go(func() error { return s.httpServer.Shutdown(gctx) })
    if s.httpPlainServer != nil {
        g.Go(func() error { return s.httpPlainServer.Shutdown(gctx) })
    }
    if s.certManager != nil {
        s.certManager.Close() // fsnotify watcher 종료
    }
    return g.Wait()
}
```

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| 포트 충돌 (`http_port == server.port`) | 높음 | `validate.go`에서 사전 차단 (UN-2 / AC8) |
| 두 리스너 중 하나 실패 시 나머지 정지 지연 | 중 | `errgroup`으로 context 취소 전파, 테스트로 검증 |

---

### 2.5 Module 5: 핫 리로드 (M5)

**마일스톤**: `fsnotify` 기반 인증서 변경 감지, 500ms 디바운스, atomic swap, 실패 시 기존 본 유지.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/api/tlsconfig.go` | modify | `CertManager.watchLoop(ctx)` 추가, `fsnotify.NewWatcher()` 사용 |
| `internal/api/tlsconfig_test.go` | modify | 핫 리로드 테스트 (신규 cert 파일 교체 → fingerprint 변경 검증) |

#### 핫 리로드 알고리즘

1. `CertManager.Start(ctx)`에서 `fsnotify.NewWatcher()` 생성
2. `cert_file`, `key_file`의 **부모 디렉토리**를 `Add()` — 편집기 save-in-place(rename) 이벤트 대응
3. 이벤트 루프:
   - `event.Op & (Write|Create|Rename) != 0`이면 `time.AfterFunc(debounceMs, m.Reload)` 호출
   - 기존 타이머가 대기 중이면 `timer.Reset(debounceMs)`로 재설정
4. `Reload()`:
   - `tls.LoadX509KeyPair(certFile, keyFile)` 시도
   - 성공: `m.current.Store(&cert)`, INFO 로그 (fingerprint, 만료일, trigger)
   - 실패: ERROR 로그, 기존 본 유지 (ST-3) — 재시도 자동으로 하지 않음
5. `ctx.Done()` 시 watcher Close, 타이머 Stop

#### 제안 코드 (골격)

```go
func (m *CertManager) Start(ctx context.Context) error {
    if !m.cfg.Reload.Enabled {
        return nil // ST-4
    }
    w, err := fsnotify.NewWatcher()
    if err != nil { return fmt.Errorf("fsnotify: %w", err) }
    m.watcher = w

    for _, p := range []string{m.certFile, m.keyFile} {
        if err := w.Add(filepath.Dir(p)); err != nil { /* log */ }
    }

    go m.watchLoop(ctx)
    return nil
}

func (m *CertManager) watchLoop(ctx context.Context) {
    var timer *time.Timer
    debounce := time.Duration(m.cfg.Reload.DebounceMs) * time.Millisecond
    for {
        select {
        case <-ctx.Done():
            if timer != nil { timer.Stop() }
            m.watcher.Close()
            return
        case ev := <-m.watcher.Events:
            if !matchesTarget(ev.Name, m.certFile, m.keyFile) { continue }
            if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 { continue }
            if timer == nil {
                timer = time.AfterFunc(debounce, func() { _ = m.Reload() })
            } else {
                timer.Reset(debounce)
            }
        case err := <-m.watcher.Errors:
            m.logger.Error("tls watcher error", "err", err)
        }
    }
}
```

#### 리스크

| 리스크 | 심각도 | 완화 방안 |
|--------|--------|----------|
| fsnotify가 rename 전/후 이벤트를 별개로 발생 | 높음 | 부모 디렉토리 감시 + 500ms 디바운스 |
| 심볼릭 링크 사용 시(예: certbot) 링크 교체 감지 실패 | 중 | 문서에 심볼릭 링크 대신 실제 파일 권고, 또는 `filepath.EvalSymlinks` 후 감시 |
| 리로드 중 race condition | 낮음 | `atomic.Pointer` swap, GetCertificate는 lock-free |

---

### 2.6 Module 6: 운영성 (M6)

**마일스톤**: 만료 경고, 구조화 로깅 필드 통일, 키 권한 경고.

#### 변경 파일 목록

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `internal/api/tlsconfig.go` | modify | `expiryWarnLoop(ctx)` 고루틴 (24h ticker) |
| `internal/api/tlsconfig_test.go` | modify | 만료 임박 cert 로드 → WARN 로그 검증 (`slogtest` 또는 커스텀 Handler) |

#### 구조화 로그 필드 (slog keys)

| 필드 | 설명 | 예시 |
|------|------|------|
| `tls.enabled` | 활성화 여부 | `true` |
| `tls.cert_path` | 인증서 경로 | `/etc/xflow/tls/server.crt` |
| `tls.cert_fingerprint` | SHA-256 fingerprint (hex) | `ab:cd:...` |
| `tls.expiry_date` | RFC3339 만료일 | `2027-04-21T00:00:00Z` |
| `tls.days_until_expiry` | 만료까지 남은 일수 | `29` |
| `tls.reload_trigger` | fsnotify op 종류 | `Write` |
| `tls.http_mode` | 설정된 모드 | `dual` |

---

### 2.7 Module 7: 프론트엔드 감사 및 문서 (M7)

**마일스톤**: 프론트엔드 WS 경로 감사, 개발자 가이드 문서 작성.

#### 프론트엔드 WebSocket 감사 결과 (현재 상태)

| 파일 | 상태 | 조치 |
|------|------|------|
| `web/src/services/ws/wsClient.ts` (L266) | ✅ 이미 `wss:` 자동 전환 | 변경 불필요, 단위 테스트 유지 |
| `web/src/services/ws/chartChannel.ts` (L51 comment, L93) | ✅ 호출자(`useChartChannel[s]`)가 `wss://` URL 제공 | 변경 불필요 |
| `web/src/pages/dashboard/panels/charts/useChartChannel.ts` (L41-45) | ✅ `wss:`/`ws:` 자동 선택 | 변경 불필요 |
| `web/src/pages/dashboard/panels/charts/useChartChannels.ts` (L49-50) | ✅ `wss:`/`ws:` 자동 선택 | 변경 불필요 |
| `web/src/pages/dashboard/panels/charts/useChartChannels.test.tsx` | ✅ `ws://test` 고정 픽스처, `window.location` 미의존 | 변경 불필요 |
| `web/src/services/ws/chartChannel.test.ts` | ✅ `ws://localhost/...` 고정 URL | 변경 불필요 |

**결론**: 프론트엔드 코드는 이미 HTTPS 백엔드와 호환된다. 신규/추후 추가되는 WS 클라이언트는 동일한 `window.location.protocol` 기반 패턴을 따라야 함을 `docs/tls-setup.md`와 개발 가이드에 명시한다.

#### 백엔드 WebSocket 구현 감사 대상 (수정 불필요 확인용)

- `internal/api/ws/chart_channel.go`, `chart_channel_test.go`
- `internal/api/ws/debug_sink.go`, `log_writer.go`, `event_publisher.go`, `broadcaster.go`, `message.go`
- `internal/api/handler/websocket.go`

백엔드 WS 서버 측 업그레이드는 `net/http` 핸들러 위에서 동작하므로 TLS 리스너 전환 만으로 자동으로 `wss://` 지원. 별도 코드 변경 불필요.

#### 신규 문서

| 파일 | 변경 유형 | 주요 변경점 |
|------|----------|------------|
| `docs/tls-setup.md` | new | 자체 서명 `openssl req -x509` 예시, Let's Encrypt certbot 힌트, 브라우저 신뢰 절차(Chrome/Firefox/Safari), systemd 배포 주석, 개발 환경 Vite 프록시 주의사항 |
| `examples/config/xflow.yaml` | modify | HTTPS 섹션 주석 확장 (샘플 값 포함) |
| `README.md` | modify (sync 단계) | Quickstart에 HTTPS 섹션 링크 추가 (Phase 3 `/moai sync`에서 실행) |

#### Vite 개발 프록시 정책 (문서화)

- 개발 환경(`npm run dev`)에서는 Vite가 HTTP로 동작하며, `vite.config.ts` 프록시 대상도 `http://localhost:8081` 백엔드
- 개발 중 백엔드를 HTTPS로 기동하려면:
  - **권장**: 백엔드를 평문 HTTP로 기동 (`tls.enabled=false`) — 개발 기본값
  - **대안**: Vite 프록시를 HTTPS 대상으로 변경 + `secure: false` (자체 서명 허용) — 문서에 스니펫 제공

---

### 2.8 Module 8: Optional 확장 (M8)

본 SPEC의 OP-* 요구사항을 선택적으로 구현. TDD 적용.

| 요구사항 | 구현 제안 | 테스트 |
|---------|----------|--------|
| OP-1 SAN 커스터마이징 | `SelfSignedConfig.AdditionalSANs` 반영 | self-sign 후 x509 파싱 → SAN 포함 확인 |
| OP-2 키 알고리즘 | `KeyType="rsa-4096"` 분기 | RSA 키 생성 성공, 공개키 타입 검증 |
| OP-3 TLS 1.3 전용 | `MinVersion="1.3"` → `tls.VersionTLS13` | TLS 1.2 클라이언트 거부 테스트 |
| OP-4 Prometheus 메트릭 | `prometheus.NewGaugeFunc` for expiry, `Counter` for handshake errors | `/metrics` 응답에 메트릭 노출 확인 |

---

## 3. 전체 파일 변경 요약

| 파일 경로 | 변경 종류 | 주요 변경점 |
|-----------|----------|------------|
| `internal/config/types.go` | modify | `TLSConfig` 확장, `SelfSignedConfig`/`ReloadConfig` 신규 |
| `internal/config/validate.go` | modify | `http_mode` enum, 포트 충돌, 기본값 Normalize |
| `internal/config/types_test.go` | modify | 신규 필드 역직렬화 케이스 |
| `internal/config/validate_test.go` | modify | UN-2, ST-2, 기본값 케이스 |
| `internal/api/tlsconfig.go` | new | `CertManager`, 로더, self-sign, GetCertificate, 핫 리로드, 만료 검사 |
| `internal/api/tlsconfig_test.go` | new | 테이블 드리븐 테스트 (로드/self-sign/reload/expiry) |
| `internal/api/hsts_middleware.go` | new | HSTS 응답 헤더 미들웨어 |
| `internal/api/hsts_middleware_test.go` | new | 헤더 존재/값 검증 |
| `internal/api/httpredirect.go` | new | HTTP→HTTPS 301 리다이렉트 핸들러 |
| `internal/api/httpredirect_test.go` | new | Location/Status/Query/Fragment 보존 |
| `internal/api/server.go` | modify | `Start()` TLS 분기, `dual`/`redirect` 리스너, `Stop()` 병렬 shutdown |
| `internal/api/server_test.go` | modify | 캐릭터라이제이션(평문 경로 grean 유지), TLS 통합 테스트 추가 |
| `examples/config/xflow.yaml` | modify | HTTPS 예시 블록 주석 확장 |
| `docs/tls-setup.md` | new | TLS 운영 가이드 |
| `README.md` | modify (sync) | Quickstart HTTPS 섹션 포인터 |

---

## 4. 테스트 전략

### 4.1 단위 테스트 (tlsconfig_test.go)

| 테스트 | 대상 | 방법 |
|--------|------|------|
| TestLoadValidCert | 정상 PEM 로드 | testdata 또는 동적 생성, `current.Load()` 검증 |
| TestLoadMissingAutoGenerateFalse | 파일 부재 + auto_generate=false | `NewCertManager` 에러 반환 (ST-1, UN-1, AC4) |
| TestSelfSignGenerate | 파일 부재 + auto_generate=true | 파일 생성 후 SAN/NotAfter/KeyType 검증 (AC3, UN-6) |
| TestSelfSignWriteFailure | 쓰기 불가 디렉토리 | 에러 반환 (UN-3) |
| TestReloadSuccess | Reload() 호출, 신규 cert fingerprint | atomic swap 확인 (EV-2) |
| TestReloadFailureKeepsOld | 파손 cert로 덮어씀 → Reload | 이전 본 유지 (ST-3, AC15) |
| TestKeyPermissionWarn | 0644 키 | WARN 로그 기록 (UN-5, AC14) |
| TestExpiryWarn | 20일 남은 cert | CertExpiryCheck 반환 + WARN (EV-3, AC12) |

### 4.2 단위 테스트 (httpredirect_test.go)

| 테스트 | 입력 | 기대 |
|--------|------|------|
| TestRedirectBasic | `http://host:8080/foo` | 301 + `Location: https://host:8443/foo` |
| TestRedirectQuery | `http://host:8080/a?x=1&y=2` | Location에 query 보존 (AC7) |
| TestRedirectHostWithPort | `Host: example.com:8080` | Location host = `example.com:8443` |

### 4.3 통합 테스트 (server_test.go 확장)

| 테스트 | 모드 | 검증 |
|--------|------|------|
| TestServerPlainHTTP | `enabled=false` | 기존 테스트 grean (AC1) |
| TestServerHTTPSOnly | `enabled=true`, `http_mode=disabled` | HTTPS 200 OK + HSTS 헤더 + HTTP 포트 connection refused (AC2, AC5, AC10) |
| TestServerDual | `http_mode=dual` | 두 리스너 모두 200 (AC6) |
| TestServerRedirect | `http_mode=redirect` | HTTP → 301 (AC7) |
| TestServerPortCollision | `http_port=server.port` | 검증 실패 (AC8) |
| TestServerTLS11Rejected | TLS 1.1 클라이언트 | handshake 실패 (AC13) |
| TestServerGracefulShutdown | dual 모드 → Stop() | 두 서버 모두 종료, 진행 중 요청 완료 대기 |

### 4.4 프론트엔드 테스트

| 테스트 | 대상 | 방법 |
|--------|------|------|
| 기존 `chartChannel.test.ts`, `useChartChannels.test.tsx` | WS 클라이언트 | 변경 없이 grean 유지 확인 |
| (선택) 신규 단위 테스트 | `deriveWsBaseUrl()` | `window.location.protocol='https:'` 주입 시 `wss:` 반환 (AC11) |

### 4.5 테스트 매트릭스

| 요구사항 ID | 단위 테스트 | 통합 테스트 | 수동 검증 |
|------------|------------|------------|----------|
| UB-1 | — | TestServerHTTPSOnly | curl https (AC2) |
| UB-2 | — | 프론트엔드 기존 테스트 | 브라우저 wss 연결 (AC11) |
| UB-3 | TestHSTSMiddleware | TestServerHTTPSOnly | curl -I (AC10) |
| UB-4 | TestMinVersion | TestServerTLS11Rejected | openssl s_client (AC13) |
| UB-5 | TestGetCertificate | — | — |
| EV-1 | TestSelfSignGenerate | TestServerHTTPSOnly(with auto) | 기동 로그 (AC3) |
| EV-2 | TestReloadSuccess | 파일 교체 후 fingerprint 변화 | — (AC9) |
| EV-3 | TestExpiryWarn | — | 로그 확인 (AC12) |
| EV-4 | TestRedirectBasic/Query | TestServerRedirect | curl -I (AC7) |
| EV-5 | — | TestServerDual | curl 두 포트 (AC6) |
| EV-6 | — | TestServerGracefulShutdown | — |
| ST-1 | TestLoadMissingAutoGenerateFalse | — | — (AC4) |
| ST-2 | TestValidateEnabledFalseIgnoresFields | — | — |
| ST-3 | TestReloadFailureKeepsOld | — | — (AC15) |
| ST-4 | TestReloadDisabledNoWatcher | — | — |
| UN-1 | TestLoadParsingError | — | — |
| UN-2 | TestValidatePortCollision | TestServerPortCollision | — (AC8) |
| UN-3 | TestSelfSignWriteFailure | — | — |
| UN-4 | — | TestServerTLS11Rejected | — (AC13) |
| UN-5 | TestKeyPermissionWarn | — | — (AC14) |
| UN-6 | TestSelfSignGenerate (로그 검증) | — | — |

---

## 5. Git 전략 및 커밋 가이드

### 5.1 브랜치

- 신규 feature 브랜치: `feature/spec-web-004-tls-https` (또는 `.moai/config`의 브랜치 정책을 따름)

### 5.2 커밋 단위 (제안)

| 순서 | 커밋 메시지 | 포함 파일 |
|------|------------|----------|
| 1 | `feat(config): TLSConfig 확장 및 검증 규칙 추가` | `internal/config/*` |
| 2 | `feat(api): CertManager 및 자체 서명 인증서 로더 추가` | `internal/api/tlsconfig*.go` |
| 3 | `feat(api): HSTS 미들웨어 추가` | `internal/api/hsts_*` |
| 4 | `feat(api): HTTPS 리스너 및 tls.http_mode=disabled 지원` | `internal/api/server.go`, `server_test.go` |
| 5 | `feat(api): tls.http_mode=dual/redirect 지원 및 graceful shutdown` | `internal/api/httpredirect*`, `server.go` |
| 6 | `feat(api): 인증서 핫 리로드 (fsnotify + debounce)` | `internal/api/tlsconfig.go` |
| 7 | `feat(api): 인증서 만료 경고 및 키 권한 검증` | `internal/api/tlsconfig.go` |
| 8 | `docs(tls): TLS 운영 가이드 및 예시 설정 확장` | `docs/tls-setup.md`, `examples/config/xflow.yaml` |

각 커밋은 TRUST 5 기준(테스트 통과, linter clean, 85%+ coverage)을 만족해야 한다.

---

## 6. 마이그레이션 및 배포 영향

### 6.1 기존 사용자 영향

- `tls.enabled=false`(기본값) 사용자: **영향 없음**. 기동 옵션, 로그, 응답 바이트까지 동일.
- `examples/config/xflow.yaml`에 이미 `tls` 블록이 주석으로 존재 → YAML 역직렬화 깨짐 없음.
- 신규 필드 부재 시 기본값으로 normalize → 기존 설정 파일 수정 불필요.

### 6.2 API 브레이킹 체인지

- 없음. 전송 계층만 변경되며 REST/WebSocket API 인터페이스는 동일.
- HTTP 클라이언트는 운영자가 TLS 활성화 시 base URL을 `http://` → `https://`로 업데이트해야 함 (배포 노트에 명시).

### 6.3 DB 마이그레이션

- 없음.

### 6.4 운영 배포 절차

1. cert/key 파일을 `/etc/xflow/tls/` 등에 배치 (CA 발급 또는 `auto_generate` 사용)
2. `xflow.yaml`에 `server.tls.enabled: true`와 경로 설정
3. `xflowd` 재시작 (downtime 필요; 인증서 교체는 이후 무중단)
4. 로그 확인: `tls.cert_fingerprint`, `tls.expiry_date`, `tls.http_mode`
5. 브라우저/curl로 `https://host:port/health` 확인

---

## 7. 보안 고려사항

| 항목 | 적용 |
|------|------|
| TLS 1.0/1.1 거부 | `tls.Config.MinVersion` 강제 (UN-4) |
| 약한 사이퍼 | Go 1.25 기본 안전 사이퍼 세트 유지 |
| 키 파일 권한 | `0600` 미만이면 WARN (UN-5) |
| 자체 서명 경고 | WARN 로그 + 문서 링크 (UN-6) |
| HSTS | 기본 활성 (`max-age=31536000; includeSubDomains`) |
| TLS 세션 티켓 | Go 기본 관리 (자동 회전) |
| 키 생성 난수원 | `crypto/rand.Reader`만 사용 |
| 메모리 내 키 보호 | Go GC 범위 내, mlock 미적용 (범위 외) |
| HTTP/2 | Go 기본 유지 (`TLSNextProto=nil`) |

---

## 8. 향후 작업 (Future Work)

- SPEC-WEB-005 (가제): ACME/Let's Encrypt 자동 발급 — 본 SPEC의 `CertManager` 인터페이스를 확장
- SPEC-WEB-006 (가제): mTLS (클라이언트 인증서) — `tls.Config.ClientAuth`, agent/cluster 통신
- SPEC-WEB-007 (가제): CSP, X-Frame-Options, Referrer-Policy 등 추가 보안 헤더
- SPEC-WEB-008 (가제): HTTP/3 (QUIC) 지원
- SPEC-WEB-009 (가제): OCSP Stapling

---

## 9. 수용 기준 요약

상세 시나리오는 `acceptance.md` 참조. AC1~AC15 총 15개.

---

*SPEC ID: SPEC-WEB-004*
*버전: 1.0.0*
*상태: proposed*
*최종 수정: 2026-04-21*
