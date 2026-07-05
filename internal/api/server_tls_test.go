package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/config"
)

// writeSelfSignedCert 는 127.0.0.1 용 self-signed 인증서/키를 temp 파일로 생성하고 경로를 반환한다.
func writeSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "xflow-test"},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(1<<31-1, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	return certFile, keyFile
}

// TLS 활성화 시 서버가 HTTPS 로 서빙하고 평문 HTTP 핸드셰이크는 실패한다.
func TestServer_Start_TLS(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}

	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	addr := s.ListenAddr()
	require.NotEmpty(t, addr)

	// HTTPS 요청 — self-signed 이므로 검증 생략.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(fmt.Sprintf("https://%s/health", addr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.TLS, "응답이 TLS 연결이어야 한다")

	// 같은 포트로 평문 HTTP 요청 → TLS 서버는 정상 200 을 주지 않는다.
	// (Go net/http 는 평문 요청에 400 "Client sent an HTTP request to an HTTPS server" 응답)
	plain := &http.Client{Timeout: 2 * time.Second}
	plainResp, plainErr := plain.Get(fmt.Sprintf("http://%s/health", addr))
	if plainErr == nil {
		defer plainResp.Body.Close()
		assert.NotEqual(t, http.StatusOK, plainResp.StatusCode,
			"TLS 서버에 평문 HTTP 는 정상 200 을 받으면 안 된다")
	}
}

// TLS 비활성(기본)이면 기존처럼 평문 HTTP 로 서빙한다(회귀).
func TestServer_Start_TLSDisabled_PlainHTTP(t *testing.T) {
	cfg := newTestServerConfig() // TLS.Enabled = false
	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	resp, err := http.Get(fmt.Sprintf("http://%s/health", s.ListenAddr()))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, resp.TLS, "TLS 비활성 시 평문 연결이어야 한다")
}

// 잘못된 cert/key 경로로 TLS 를 켜면 서버 시작이 에러를 반환한다.
func TestServer_Start_TLS_BadCert(t *testing.T) {
	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{
		Enabled:  true,
		CertFile: filepath.Join(t.TempDir(), "missing-cert.pem"),
		KeyFile:  filepath.Join(t.TempDir(), "missing-key.pem"),
	}
	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()

	// Start 는 ServeTLS 실패(파일 없음)를 errCh 로 반환한다.
	err := s.Start(context.Background())
	require.Error(t, err)
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
}
