package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Validate - Viper 인스턴스에 대한 유효성 검증 수행, 집계된 에러 반환
func Validate(v *viper.Viper) error {
	var ve ValidationErrors

	validatePort(v, &ve)
	validateStorageType(v, &ve)
	validateLogLevel(v, &ve)
	validateLogFormat(v, &ve)
	validateLogOutput(v, &ve)
	validateIDStyle(v, &ve)
	validatePositiveValues(v, &ve)
	validateDurations(v, &ve)
	validateTLS(v, &ve)
	validateProductionJWT(v, &ve)
	validatePostgresDSN(v, &ve)
	validateWebUI(v, &ve)
	validateRemoteManagement(v, &ve)

	if ve.HasErrors() {
		return &ve
	}
	return nil
}

// validatePort - server.port가 1-65535 범위인지 검증
func validatePort(v *viper.Viper, ve *ValidationErrors) {
	port := v.GetInt("server.port")
	if port < 1 || port > 65535 {
		ve.Add(fmt.Errorf("%w: server.port=%d", ErrInvalidPort, port))
	}
}

// validateStorageType - storage.type이 "sqlite" 또는 "postgres"인지 검증
func validateStorageType(v *viper.Viper, ve *ValidationErrors) {
	st := v.GetString("storage.type")
	switch st {
	case "sqlite", "postgres":
		// 유효한 타입
	default:
		ve.Add(fmt.Errorf("%w: storage.type=%q", ErrInvalidStorageType, st))
	}
}

// validateLogLevel - observe.default_level이 유효한 로그 레벨인지 검증
func validateLogLevel(v *viper.Viper, ve *ValidationErrors) {
	level := v.GetString("observe.default_level")
	switch level {
	case "debug", "info", "warn", "error":
		// 유효한 레벨
	default:
		ve.Add(fmt.Errorf("%w: observe.default_level=%q", ErrInvalidLogLevel, level))
	}
}

// validateIDStyle - observe.id_style이 유효한 값인지 검증
// 빈 문자열은 기본 "both"로 처리되므로 허용한다 (구버전 설정 무회귀).
func validateIDStyle(v *viper.Viper, ve *ValidationErrors) {
	style := v.GetString("observe.id_style")
	switch style {
	case "", "name", "id", "both":
		// 유효한 스타일 (빈 값은 both 로 fallback)
	default:
		ve.Add(fmt.Errorf("%w: observe.id_style=%q", ErrInvalidIDStyle, style))
	}
}

// validatePositiveValues - 양수 필수 값들 검증
func validatePositiveValues(v *viper.Viper, ve *ValidationErrors) {
	positiveKeys := []string{
		"engine.backpressure_threshold",
		"engine.max_concurrent_flows",
		"script.vm_pool_size",
		"storage.pool_size",
	}

	for _, key := range positiveKeys {
		val := v.GetInt(key)
		if val <= 0 {
			ve.Add(fmt.Errorf("%w: %s=%d", ErrInvalidPositiveValue, key, val))
		}
	}
}

// validateDurations - 기간 문자열이 유효한 time.Duration인지 검증
func validateDurations(v *viper.Viper, ve *ValidationErrors) {
	durationKeys := []string{
		"auth.jwt.access_ttl",
		"auth.jwt.refresh_ttl",
		"script.timeout",
	}

	for _, key := range durationKeys {
		val := v.GetString(key)
		if val == "" {
			continue
		}
		if _, err := time.ParseDuration(val); err != nil {
			ve.Add(fmt.Errorf("%w: %s=%q", ErrInvalidDuration, key, val))
		}
	}
}

// validateTLS - TLS 활성화 시 인증서/키 파일 존재 여부 검증
func validateTLS(v *viper.Viper, ve *ValidationErrors) {
	if !v.GetBool("server.tls.enabled") {
		return
	}

	certFile := v.GetString("server.tls.cert_file")
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		ve.Add(fmt.Errorf("%w: server.tls.cert_file=%q", ErrFileNotFound, certFile))
	}

	keyFile := v.GetString("server.tls.key_file")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		ve.Add(fmt.Errorf("%w: server.tls.key_file=%q", ErrFileNotFound, keyFile))
	}
}

// validateProductionJWT - 프로덕션 모드에서 JWT 시크릿 필수 검증
func validateProductionJWT(v *viper.Viper, ve *ValidationErrors) {
	if v.GetString("server.mode") != "production" {
		return
	}

	if v.GetString("auth.jwt.secret") == "" {
		ve.Add(fmt.Errorf("%w: auth.jwt.secret (프로덕션 모드에서 필수)", ErrRequiredField))
	}
}

// validateLogFormat - observe.format이 "json" 또는 "text"인지 검증
func validateLogFormat(v *viper.Viper, ve *ValidationErrors) {
	format := v.GetString("observe.format")
	switch format {
	case "json", "text":
		// 유효한 포맷
	default:
		ve.Add(fmt.Errorf("%w: observe.format=%q", ErrInvalidLogFormat, format))
	}
}

// validateLogOutput - observe.output이 유효한 출력 대상인지 검증
func validateLogOutput(v *viper.Viper, ve *ValidationErrors) {
	output := v.GetString("observe.output")
	if output == "" {
		ve.Add(fmt.Errorf("%w: observe.output is empty", ErrInvalidLogOutput))
		return
	}

	// "stdout"은 유효한 출력 대상
	if output == "stdout" {
		return
	}

	// "stdout+파일경로" 형식 검증
	if strings.HasPrefix(output, "stdout+") {
		filePath := strings.TrimPrefix(output, "stdout+")
		if filePath == "" {
			ve.Add(fmt.Errorf("%w: observe.output=%q (file path after stdout+ is empty)", ErrInvalidLogOutput, output))
		}
		return
	}

	// "+"로 시작하는 잘못된 형식 검증 (예: "+/path")
	if strings.HasPrefix(output, "+") {
		ve.Add(fmt.Errorf("%w: observe.output=%q", ErrInvalidLogOutput, output))
		return
	}

	// 나머지는 파일 경로로 취급 (비어있지 않으면 유효)
}

// validatePostgresDSN - PostgreSQL 타입 선택 시 DSN 필수 검증
func validatePostgresDSN(v *viper.Viper, ve *ValidationErrors) {
	if v.GetString("storage.type") != "postgres" {
		return
	}

	if v.GetString("storage.postgres.dsn") == "" {
		ve.Add(fmt.Errorf("%w: storage.postgres.dsn (postgres 타입에서 필수)", ErrRequiredField))
	}
}

// validateWebUI - Web UI 활성화 시 디렉토리 설정 필수 검증
func validateWebUI(v *viper.Viper, ve *ValidationErrors) {
	if !v.GetBool("server.web_ui.enabled") {
		return
	}

	dir := v.GetString("server.web_ui.dir")
	if dir == "" {
		ve.Add(fmt.Errorf("%w: server.web_ui.dir (web_ui 활성화 시 필수)", ErrRequiredField))
	}
}

// validateRemoteManagement - 원격 관리 설정 검증 (@SPEC:SPEC-REMOTE-001 M1).
//
// 검증 항목:
//   - mode: server|client|disabled 만 허용 (REQ-A01).
//   - client 모드: server_url 필수 (REQ-A02).
//   - TLS 활성화 시: cert/key 파일 존재 (REQ-F01).
//
// disabled 모드(기본)는 추가 검증 없이 통과한다(회귀 안전 — REQ-N03).
func validateRemoteManagement(v *viper.Viper, ve *ValidationErrors) {
	mode := v.GetString("remote_management.mode")
	switch mode {
	case "disabled", "server", "client":
		// 유효한 모드.
	default:
		ve.Add(fmt.Errorf("%w: remote_management.mode=%q", ErrInvalidRemoteMode, mode))
		return
	}

	serverURL := v.GetString("remote_management.server_url")
	if mode == "client" {
		if serverURL == "" {
			ve.Add(fmt.Errorf("%w: remote_management.server_url (client 모드에서 필수)", ErrRequiredField))
		}
	}

	// 보안 전송 강제(M6, REQ-F01): require_secure=true 이고 non-dev 이면 평문 전송을
	// 거부한다. development 모드에서는 강제하지 않는다(로컬 개발 편의 — "non-dev" 한정).
	if v.GetBool("remote_management.require_secure") && v.GetString("server.mode") != "development" {
		switch mode {
		case "client":
			// 평문 ws:// (또는 비-wss 스킴)는 거부한다.
			if serverURL != "" && !strings.HasPrefix(strings.ToLower(serverURL), "wss://") {
				ve.Add(fmt.Errorf("%w: remote_management.server_url=%q (require_secure 시 wss:// 필수)",
					ErrInsecureTransport, serverURL))
			}
		case "server":
			// 관리 서버는 TLS 가 활성화되어야 한다(평문 ws 수락 금지).
			if !v.GetBool("remote_management.tls.enabled") {
				ve.Add(fmt.Errorf("%w: remote_management.tls.enabled=false (require_secure 시 server 모드 TLS 필수)",
					ErrInsecureTransport))
			}
		}
	}

	if v.GetBool("remote_management.tls.enabled") {
		certFile := v.GetString("remote_management.tls.cert_file")
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			ve.Add(fmt.Errorf("%w: remote_management.tls.cert_file=%q", ErrFileNotFound, certFile))
		}
		keyFile := v.GetString("remote_management.tls.key_file")
		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			ve.Add(fmt.Errorf("%w: remote_management.tls.key_file=%q", ErrFileNotFound, keyFile))
		}
	}
}
