package xsfm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// certMarker 는 PEM 인증서 블록 헤더 토큰이다 (소스에 리터럴로 두지 않기 위해 조립).
var certMarker = "-----BEGIN " + "CERTIFICATE-----"

// genCertPEM 은 테스트용 자체 서명 인증서 PEM 문자열을 생성한다.
func genCertPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestBuildTLSConfig(t *testing.T) {
	log := slog.Default()

	// TLS 미사용 → nil.
	c := newPahoMQTTClient(XSFMConfig{TLS: false}, log)
	tc, err := c.buildTLSConfig()
	require.NoError(t, err)
	assert.Nil(t, tc)

	// TLS 사용, CA 없음 → 기본 config.
	c = newPahoMQTTClient(XSFMConfig{TLS: true}, log)
	tc, err = c.buildTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, tc)
	assert.Equal(t, uint16(0x0303), tc.MinVersion) // TLS 1.2

	// TLS 사용, 인라인 PEM (유효) → RootCAs 설정.
	c = newPahoMQTTClient(XSFMConfig{TLS: true, CACert: genCertPEM(t)}, log)
	tc, err = c.buildTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, tc)
	assert.NotNil(t, tc.RootCAs)

	// TLS 사용, 잘못된 인라인 PEM → 에러 (헤더는 있으나 본문 파싱 실패).
	brokenPEM := certMarker + "\nbroken\n-----END CERTIFICATE-----"
	c = newPahoMQTTClient(XSFMConfig{TLS: true, CACert: brokenPEM}, log)
	_, err = c.buildTLSConfig()
	assert.Error(t, err)

	// TLS 사용, 파일 경로 (없는 파일) → 에러.
	c = newPahoMQTTClient(XSFMConfig{TLS: true, CACert: "/nonexistent/ca.pem"}, log)
	_, err = c.buildTLSConfig()
	assert.Error(t, err)
}

// 미연결 pahoMQTTClient 의 가드 동작 (Publish/Subscribe/IsConnected).
func TestPahoClient_Unconnected(t *testing.T) {
	c := newPahoMQTTClient(XSFMConfig{Broker: "tcp://127.0.0.1:1"}, slog.Default())
	assert.False(t, c.IsConnected())
	assert.ErrorIs(t, c.Publish("t", 1, []byte("x")), ErrNotConnected)

	// 미연결 Subscribe 는 재연결 복원용으로 기록만 하고 nil 반환.
	called := false
	require.NoError(t, c.Subscribe("xsfm/ap-1/state", 1, func(_ string, _ []byte) { called = true }))
	assert.False(t, called)
	c.mu.Lock()
	_, ok := c.subs["xsfm/ap-1/state"]
	c.mu.Unlock()
	assert.True(t, ok, "구독이 재연결 복원 레지스트리에 기록되어야 한다")

	// nil client Disconnect 는 no-op.
	c.Disconnect()
}

// Connect: 도달 불가 브로커 + auto_reconnect=false → 에러 (짧은 타임아웃).
func TestPahoClient_ConnectError(t *testing.T) {
	cfg := XSFMConfig{
		Broker:         "tcp://127.0.0.1:1",
		ClientID:       "test-cid",
		KeepAlive:      time.Second,
		ConnectTimeout: 200 * time.Millisecond,
		AutoReconnect:  false,
	}
	c := newPahoMQTTClient(cfg, slog.Default())
	assert.Error(t, c.Connect())
}

// Connect: 도달 불가 브로커 + auto_reconnect=true → degraded(nil) 반환 (백그라운드 재시도).
func TestPahoClient_ConnectDegraded(t *testing.T) {
	cfg := XSFMConfig{
		Broker:         "tcp://127.0.0.1:1",
		ClientID:       "test-cid",
		KeepAlive:      time.Second,
		ConnectTimeout: 200 * time.Millisecond,
		AutoReconnect:  true,
	}
	c := newPahoMQTTClient(cfg, slog.Default())
	assert.NoError(t, c.Connect())
	c.Disconnect()
}
