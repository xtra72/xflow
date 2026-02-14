package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Validate - Viper 인스턴스에 대한 유효성 검증 수행, 집계된 에러 반환
func Validate(v *viper.Viper) error {
	var ve ValidationErrors

	validatePort(v, &ve)
	validateStorageType(v, &ve)
	validateLogLevel(v, &ve)
	validatePositiveValues(v, &ve)
	validateDurations(v, &ve)
	validateTLS(v, &ve)
	validateProductionJWT(v, &ve)
	validatePostgresDSN(v, &ve)

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

// validatePostgresDSN - PostgreSQL 타입 선택 시 DSN 필수 검증
func validatePostgresDSN(v *viper.Viper, ve *ValidationErrors) {
	if v.GetString("storage.type") != "postgres" {
		return
	}

	if v.GetString("storage.postgres.dsn") == "" {
		ve.Add(fmt.Errorf("%w: storage.postgres.dsn (postgres 타입에서 필수)", ErrRequiredField))
	}
}
