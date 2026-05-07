// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11, M12, M13)
// manifest_test.go — DependencyManifest + CheckConstraint + ManifestFetcher + CompatibilityChecker 테스트.
//
// 보안 critical 모듈: 90%+ 커버리지 목표.
// 테스트 분류:
//   - DependencyManifest 구조 검증 (3 tests)
//   - CheckConstraint semver 비교 (5 tests)
//   - ManifestFetcher HTTPS + 서명 검증 (4 tests)
//   - CompatibilityChecker 호환성 검사 (4 tests)
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----- DependencyManifest 구조 검증 -----

// TestDependencyManifest_IsValid_TrueWhenComplete 는 모든 필수 필드가 채워졌을 때 valid 임을 검증한다.
func TestDependencyManifest_IsValid_TrueWhenComplete(t *testing.T) {
	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat: map[string]string{
			"xflowd": ">=v0.4.0",
		},
	}
	assert.True(t, m.IsValid())
}

// TestDependencyManifest_IsValid_FalseWhenMissingBinary 는 Binary 필드가 비어 있으면 invalid 임을 검증한다.
func TestDependencyManifest_IsValid_FalseWhenMissingBinary(t *testing.T) {
	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "",
	}
	assert.False(t, m.IsValid())
}

// TestDependencyManifest_IsValid_FalseWhenInvalidVersion 은 Version 이 semver 가 아닐 때 invalid 임을 검증한다.
func TestDependencyManifest_IsValid_FalseWhenInvalidVersion(t *testing.T) {
	m := DependencyManifest{
		Version: "not-a-semver",
		Binary:  "xflowd",
	}
	assert.False(t, m.IsValid())
}

// ----- CheckConstraint semver 비교 -----

// TestCheckConstraint_GreaterEqual_True 는 ">=" 제약을 만족하는 경우를 검증한다.
func TestCheckConstraint_GreaterEqual_True(t *testing.T) {
	err := CheckConstraint("v0.4.0", ">=v0.4.0")
	assert.NoError(t, err)

	err = CheckConstraint("v0.5.0", ">=v0.4.0")
	assert.NoError(t, err)
}

// TestCheckConstraint_GreaterEqual_False 는 ">=" 제약을 위반하는 경우 ErrUpdateIncompatibleVersion 을 반환함을 검증한다.
func TestCheckConstraint_GreaterEqual_False(t *testing.T) {
	err := CheckConstraint("v0.3.0", ">=v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))
}

// TestCheckConstraint_LessThan 은 "<" 연산자의 양/부정 모두를 검증한다.
func TestCheckConstraint_LessThan(t *testing.T) {
	// satisfied
	err := CheckConstraint("v0.3.0", "<v0.4.0")
	assert.NoError(t, err)

	// violated
	err = CheckConstraint("v0.4.0", "<v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))
}

// TestCheckConstraint_Equal_NotEqual 은 "==" 와 "!=" 연산자를 검증한다.
func TestCheckConstraint_Equal_NotEqual(t *testing.T) {
	// == satisfied
	require.NoError(t, CheckConstraint("v0.4.0", "==v0.4.0"))
	// == violated
	err := CheckConstraint("v0.4.1", "==v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))

	// != satisfied
	require.NoError(t, CheckConstraint("v0.5.0", "!=v0.4.0"))
	// != violated
	err = CheckConstraint("v0.4.0", "!=v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))
}

// TestCheckConstraint_LessEqual_Greater 는 "<=" 와 ">" 연산자를 검증한다 (잔여 분기 커버리지).
func TestCheckConstraint_LessEqual_Greater(t *testing.T) {
	// <= satisfied (equal)
	require.NoError(t, CheckConstraint("v0.4.0", "<=v0.4.0"))
	// <= satisfied (less)
	require.NoError(t, CheckConstraint("v0.3.0", "<=v0.4.0"))
	// <= violated
	err := CheckConstraint("v0.5.0", "<=v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))

	// > satisfied
	require.NoError(t, CheckConstraint("v0.5.0", ">v0.4.0"))
	// > violated (equal)
	err = CheckConstraint("v0.4.0", ">v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))
}

// TestCheckConstraint_InvalidFormat_Error 는 잘못된 제약 형식이 ErrUpdateInvalidInput 으로 거부됨을 검증한다.
func TestCheckConstraint_InvalidFormat_Error(t *testing.T) {
	cases := []string{
		"",                // 빈 문자열
		">>v0.4.0",        // 두 번 연산자
		"v0.4.0",          // 연산자 없음
		">=not-a-version", // semver 부분 invalid
		">=v0.4",          // 3-part 가 아님
		">= v0.4.0 garbage",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			err := CheckConstraint("v0.4.0", c)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput),
				"expected ErrUpdateInvalidInput for constraint %q, got %v", c, err)
		})
	}
}

// TestCheckConstraint_InvalidVersion_Error 는 currentVersion 자체가 invalid 일 때 ErrUpdateInvalidInput 을 반환함을 검증한다.
func TestCheckConstraint_InvalidVersion_Error(t *testing.T) {
	err := CheckConstraint("not-a-version", ">=v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// ----- ManifestFetcher HTTPS + 서명 검증 -----

// fetcherTestServer 는 manifest + signature 를 제공하는 httptest.Server 를 빌드한다.
//
// 반환된 함수는 호출 시 서명 / manifest 본문을 임의로 변조 가능 (테스트 시나리오용).
func fetcherTestServer(t *testing.T, manifestBytes, sigBytes []byte) (*httptest.Server, string, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(manifestBytes)
	})
	mux.HandleFunc("/manifest.json.sig", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(sigBytes)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv, srv.URL + "/manifest.json", srv.URL + "/manifest.json.sig"
}

// TestManifestFetcher_FetchAndVerify_Success 는 정상 manifest 를 검증하고 파싱하는 happy path 를 확인한다.
func TestManifestFetcher_FetchAndVerify_Success(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	dm := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat: map[string]string{
			"xflowd": ">=v0.4.0",
			"xflow":  ">=v0.3.0",
		},
	}
	manifestBytes, err := json.Marshal(dm)
	require.NoError(t, err)

	sigBytes := ed25519.Sign(priv, manifestBytes)

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)

	verifier, err := NewVerifier(pub)
	require.NoError(t, err)

	fetcher := &ManifestFetcher{
		HTTPClient: srv.Client(), // tls server 의 self-signed cert 를 신뢰
		Verifier:   verifier,
	}

	got, err := fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.NoError(t, err)
	assert.Equal(t, dm.Version, got.Version)
	assert.Equal(t, dm.Binary, got.Binary)
	assert.Equal(t, dm.Compat, got.Compat)
}

// TestManifestFetcher_FetchAndVerify_HTTPRejected 는 http:// (HTTPS 가 아닌) URL 을 거부함을 검증한다.
func TestManifestFetcher_FetchAndVerify_HTTPRejected(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	verifier, err := NewVerifier(pub)
	require.NoError(t, err)

	fetcher := &ManifestFetcher{
		HTTPClient: http.DefaultClient,
		Verifier:   verifier,
	}

	// http manifest URL
	_, err = fetcher.FetchAndVerify(context.Background(),
		"http://example.com/manifest.json",
		"https://example.com/manifest.json.sig")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))

	// http signature URL
	_, err = fetcher.FetchAndVerify(context.Background(),
		"https://example.com/manifest.json",
		"http://example.com/manifest.json.sig")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestManifestFetcher_FetchAndVerify_SignatureInvalid 는 변조된 서명을 거부함을 검증한다.
func TestManifestFetcher_FetchAndVerify_SignatureInvalid(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	dm := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflowd",
	}
	manifestBytes, err := json.Marshal(dm)
	require.NoError(t, err)

	// 정상 서명을 만든 뒤 변조
	sigBytes := ed25519.Sign(priv, manifestBytes)
	sigBytes[0] ^= 0xFF

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)

	verifier, err := NewVerifier(pub)
	require.NoError(t, err)
	fetcher := &ManifestFetcher{HTTPClient: srv.Client(), Verifier: verifier}

	_, err = fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid))
}

// TestManifestFetcher_FetchAndVerify_TooLarge 는 64KB 초과 manifest 를 거부함을 검증한다 (DoS 방어).
func TestManifestFetcher_FetchAndVerify_TooLarge(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 80KB filler manifest (cap 64KB)
	largeBlob := strings.Repeat("a", 80*1024)
	manifestBytes := []byte(`{"version":"v0.4.0","binary":"xflowd","filler":"` + largeBlob + `"}`)
	sigBytes := ed25519.Sign(priv, manifestBytes)

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)
	verifier, err := NewVerifier(pub)
	require.NoError(t, err)
	fetcher := &ManifestFetcher{HTTPClient: srv.Client(), Verifier: verifier}

	_, err = fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestManifestFetcher_FetchAndVerify_InvalidJSON 은 서명은 올바르지만 JSON 이 invalid 인 경우를 검증한다.
func TestManifestFetcher_FetchAndVerify_InvalidJSON(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestBytes := []byte("{not valid json}")
	sigBytes := ed25519.Sign(priv, manifestBytes)

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)
	verifier, err := NewVerifier(pub)
	require.NoError(t, err)
	fetcher := &ManifestFetcher{HTTPClient: srv.Client(), Verifier: verifier}

	_, err = fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestManifestFetcher_FetchAndVerify_InvalidStructure 는 JSON 은 올바르지만 구조가 invalid (Binary 필드 부재) 한 경우를 검증한다.
func TestManifestFetcher_FetchAndVerify_InvalidStructure(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Binary 필드 없음 → IsValid()=false
	manifestBytes := []byte(`{"version":"v0.4.0"}`)
	sigBytes := ed25519.Sign(priv, manifestBytes)

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)
	verifier, err := NewVerifier(pub)
	require.NoError(t, err)
	fetcher := &ManifestFetcher{HTTPClient: srv.Client(), Verifier: verifier}

	_, err = fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// ----- CompatibilityChecker 호환성 검사 -----

// TestCompatibilityChecker_Validate_AllSatisfied 는 모든 제약을 만족하는 환경에서 nil 을 반환함을 검증한다.
func TestCompatibilityChecker_Validate_AllSatisfied(t *testing.T) {
	versions := map[string]Version{
		"xflowd": "v0.4.0",
		"xflow":  "v0.3.0",
	}
	checker := &CompatibilityChecker{
		GetVersion: func(binary string) (Version, error) {
			v, ok := versions[binary]
			if !ok {
				return "", errors.New("not installed")
			}
			return v, nil
		},
	}

	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat: map[string]string{
			"xflowd": ">=v0.4.0",
			"xflow":  ">=v0.3.0",
		},
	}

	err := checker.Validate(m)
	assert.NoError(t, err)
}

// TestCompatibilityChecker_Validate_BinaryNotInstalled_SkipsConstraint 는 환경에 바이너리가 없으면 해당 제약을 건너뜀을 검증한다.
func TestCompatibilityChecker_Validate_BinaryNotInstalled_SkipsConstraint(t *testing.T) {
	checker := &CompatibilityChecker{
		GetVersion: func(_ string) (Version, error) {
			return "", errors.New("not installed")
		},
	}

	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat: map[string]string{
			"xflowd": ">=v999.0.0", // 위반이지만 환경에 xflowd 가 없으므로 스킵되어야 함
		},
	}

	err := checker.Validate(m)
	assert.NoError(t, err, "missing binary should skip the constraint")
}

// TestCompatibilityChecker_Validate_ConstraintViolated_ErrUpdateIncompatibleVersion 은 호환성 위반 시 sentinel error 를 반환함을 검증한다.
func TestCompatibilityChecker_Validate_ConstraintViolated_ErrUpdateIncompatibleVersion(t *testing.T) {
	versions := map[string]Version{
		"xflowd": "v0.3.0", // 제약 ">=v0.4.0" 위반
	}
	checker := &CompatibilityChecker{
		GetVersion: func(binary string) (Version, error) {
			return versions[binary], nil
		},
	}

	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat: map[string]string{
			"xflowd": ">=v0.4.0",
		},
	}

	err := checker.Validate(m)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateIncompatibleVersion))
}

// TestCompatibilityChecker_Validate_EmptyCompat_NoOp 은 Compat 이 빈 경우 항상 통과함을 검증한다.
func TestCompatibilityChecker_Validate_EmptyCompat_NoOp(t *testing.T) {
	checker := &CompatibilityChecker{
		GetVersion: func(_ string) (Version, error) {
			return "", errors.New("should not be called")
		},
	}

	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		// Compat 미설정 (nil map)
	}

	err := checker.Validate(m)
	assert.NoError(t, err)
}

// TestCompatibilityChecker_Validate_NilGetVersion_ErrUpdateInvalidInput 은
// GetVersion 이 nil 일 때 명시 실패를 검증한다 (silent skip 방지).
func TestCompatibilityChecker_Validate_NilGetVersion_ErrUpdateInvalidInput(t *testing.T) {
	checker := &CompatibilityChecker{GetVersion: nil}
	m := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflow-agent",
		Compat:  map[string]string{"xflowd": ">=v0.4.0"},
	}

	err := checker.Validate(m)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestManifestFetcher_FetchAndVerify_NilVerifier_ErrUpdateInvalidInput 은
// Verifier 가 nil 일 때 즉시 거부됨을 검증한다.
func TestManifestFetcher_FetchAndVerify_NilVerifier_ErrUpdateInvalidInput(t *testing.T) {
	fetcher := &ManifestFetcher{HTTPClient: http.DefaultClient, Verifier: nil}
	_, err := fetcher.FetchAndVerify(context.Background(),
		"https://example.com/m.json", "https://example.com/m.sig")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestManifestFetcher_FetchAndVerify_TrailingNewline_StillValid 은
// 일부 도구가 추가하는 트레일링 newline 이 있는 .sig 도 정상 검증됨을 확인한다.
func TestManifestFetcher_FetchAndVerify_TrailingNewline_StillValid(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	dm := DependencyManifest{
		Version: "v0.4.0",
		Binary:  "xflowd",
	}
	manifestBytes, err := json.Marshal(dm)
	require.NoError(t, err)

	// 정상 64-byte 서명에 trailing \n 추가 → 65 bytes 가 됨.
	sigBytes := append(ed25519.Sign(priv, manifestBytes), '\n')
	require.Len(t, sigBytes, 65)

	srv, manifestURL, sigURL := fetcherTestServer(t, manifestBytes, sigBytes)
	verifier, err := NewVerifier(pub)
	require.NoError(t, err)
	fetcher := &ManifestFetcher{HTTPClient: srv.Client(), Verifier: verifier}

	got, err := fetcher.FetchAndVerify(context.Background(), manifestURL, sigURL)
	require.NoError(t, err, "트레일링 newline 이 있는 정상 서명도 통과해야 함")
	assert.Equal(t, dm.Binary, got.Binary)
}
