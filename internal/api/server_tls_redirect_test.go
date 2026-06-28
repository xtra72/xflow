package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/config"
)

// noRedirectClient 는 리다이렉트를 따라가지 않는 평문 HTTP 클라이언트를 만든다.
// CheckRedirect 가 http.ErrUseLastResponse 를 반환하면 첫 응답(308)을 그대로 받는다.
func noRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// TLS 활성화 시 같은 포트로 들어온 평문 HTTP 요청은 308 Permanent Redirect 로
// 동일 host:port 의 https:// URL 로 리다이렉트되어야 한다.
func TestServer_TLS_PlainHTTP_Redirects308(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}

	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	addr := s.ListenAddr()
	require.NotEmpty(t, addr)

	client := noRedirectClient(5 * time.Second)
	resp, err := client.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
	assert.Equal(t, fmt.Sprintf("https://%s/health", addr), resp.Header.Get("Location"))
}

// 평문 HTTP 리다이렉트는 path 와 query 를 그대로 보존해야 한다.
func TestServer_TLS_PlainHTTP_PreservesPathQuery(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}

	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	addr := s.ListenAddr()

	client := noRedirectClient(5 * time.Second)
	resp, err := client.Get(fmt.Sprintf("http://%s/api/v1/flows?x=1", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
	assert.Equal(t, fmt.Sprintf("https://%s/api/v1/flows?x=1", addr), resp.Header.Get("Location"))
}

// TLS 활성화 시 HTTPS 요청은 평소처럼 정상 200 을 받아야 한다(리다이렉트 분기 영향 없음).
func TestServer_TLS_HTTPS_StillServes200(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}

	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	addr := s.ListenAddr()

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
}

// TLS 비활성 시에는 디스패처 없이 평문 HTTP 200 을 그대로 서빙한다(회귀, 리다이렉트 없음).
func TestServer_TLSDisabled_NoRedirect(t *testing.T) {
	cfg := newTestServerConfig() // TLS.Enabled = false
	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()
	startTestServer(t, s)

	client := noRedirectClient(5 * time.Second)
	resp, err := client.Get(fmt.Sprintf("http://%s/health", s.ListenAddr()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, resp.TLS, "TLS 비활성 시 평문 연결이어야 한다")
}

// TLS 서버를 시작 후 정지하면 에러 없이 정상 종료되어야 한다(고루틴/포트 누수 없음).
func TestServer_TLS_ShutdownClean(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := newTestServerConfig()
	cfg.TLS = config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}

	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()

	// startTestServer 가 t.Cleanup 으로 Stop 을 호출하지만, 여기서는 명시적으로
	// Stop 을 호출해 반환 에러가 nil 인지 확인한다.
	ctxDone := make(chan struct{})
	go func() {
		_ = s.Start(context.Background())
		close(ctxDone)
	}()
	for i := 0; i < 100; i++ {
		if s.ListenAddr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEmpty(t, s.ListenAddr())

	// 평문 요청이 정상 308 을 주는지(디스패처 동작 확인) 후 정지.
	client := noRedirectClient(3 * time.Second)
	resp, err := client.Get(fmt.Sprintf("http://%s/health", s.ListenAddr()))
	require.NoError(t, err)
	resp.Body.Close()

	require.NoError(t, s.Stop(context.Background()))

	select {
	case <-ctxDone:
	case <-time.After(5 * time.Second):
		t.Fatal("TLS 서버 종료 타임아웃")
	}
}

// --- 디스패처 프리미티브 단위 테스트 (분기/엣지 케이스 커버리지) ---

// fakeConn 은 net.Conn 을 만족하는 테스트용 인메모리 연결이다.
// readData 를 첫 Read 로 흘려보내고, Close 여부를 기록한다.
type fakeConn struct {
	readData []byte
	readErr  error
	closed   bool
}

func (c *fakeConn) Read(b []byte) (int, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if len(c.readData) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.readData)
	c.readData = c.readData[n:]
	return n, nil
}
func (c *fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *fakeConn) Close() error                     { c.closed = true; return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// peekedConn 은 미리 읽은 prefix 를 먼저 흘려보낸 뒤 원본으로 위임해야 한다.
func TestPeekedConn_ReEmitsPrefixThenDelegates(t *testing.T) {
	base := &fakeConn{readData: []byte("ELLO")}
	pc := &peekedConn{Conn: base, prefix: []byte{'H'}}

	out := make([]byte, 16)
	// 첫 Read 는 prefix('H') 만 반환한다.
	n, err := pc.Read(out)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	assert.Equal(t, byte('H'), out[0])

	// 이후 Read 는 원본 데이터를 반환한다.
	n, err = pc.Read(out)
	require.NoError(t, err)
	assert.Equal(t, "ELLO", string(out[:n]))
}

// chanListener.Addr 은 광고 주소를 반환해야 한다.
func TestChanListener_Addr(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
	l := newChanListener(addr, 1)
	assert.Equal(t, addr.String(), l.Addr().String())
}

// 닫힌 chanListener.Accept 는 errListenerClosed 를 반환해야 한다.
func TestChanListener_AcceptAfterClose(t *testing.T) {
	l := newChanListener(&net.TCPAddr{}, 1)
	require.NoError(t, l.Close())
	require.NoError(t, l.Close()) // 멱등(double close) 안전

	_, err := l.Accept()
	assert.ErrorIs(t, err, errListenerClosed)
}

// 닫힌 chanListener 로 push 하면 연결을 닫아 누수를 막아야 한다.
func TestChanListener_PushAfterCloseClosesConn(t *testing.T) {
	l := newChanListener(&net.TCPAddr{}, 1)
	require.NoError(t, l.Close())

	c := &fakeConn{}
	l.push(c)
	assert.True(t, c.closed, "닫힌 리스너로 push 된 연결은 닫혀야 한다")
}

// peekAndRoute 는 peek 에러 시 연결을 닫아야 한다.
func TestDispatcher_PeekError_ClosesConn(t *testing.T) {
	// 실제 TCP 리스너 없이 디스패처를 구성한다(run 은 호출하지 않음).
	d := &tlsRedirectDispatcher{
		tlsLn:   newChanListener(&net.TCPAddr{}, 1),
		redirLn: newChanListener(&net.TCPAddr{}, 1),
	}
	c := &fakeConn{readErr: errors.New("peek boom")}
	d.peekAndRoute(c)
	assert.True(t, c.closed, "peek 실패 시 연결은 닫혀야 한다")
}

// peekAndRoute 는 첫 바이트로 TLS(0x16)/평문 트래픽을 올바른 분기 리스너로 라우팅해야 한다.
func TestDispatcher_PeekAndRoute_Classifies(t *testing.T) {
	d := &tlsRedirectDispatcher{
		tlsLn:   newChanListener(&net.TCPAddr{}, 1),
		redirLn: newChanListener(&net.TCPAddr{}, 1),
	}

	// TLS ClientHello(0x16) → tlsLn 으로 라우팅.
	tlsConn := &fakeConn{readData: []byte{tlsRecordTypeHandshake, 0x03, 0x01}}
	d.peekAndRoute(tlsConn)
	routed, err := d.tlsLn.Accept()
	require.NoError(t, err)
	require.NotNil(t, routed)

	// 평문 HTTP('G') → redirLn 으로 라우팅.
	httpConn := &fakeConn{readData: []byte("GET / HTTP/1.1")}
	d.peekAndRoute(httpConn)
	routed, err = d.redirLn.Accept()
	require.NoError(t, err)
	require.NotNil(t, routed)
}

// newHTTPSRedirectHandler 는 동일 host:port 의 https URL 로 308 을 보내고 path/query 를 보존해야 한다.
func TestHTTPSRedirectHandler(t *testing.T) {
	h := newHTTPSRedirectHandler()
	req, err := http.NewRequest(http.MethodGet, "http://example.com:8443/api/v1/flows?x=1", nil)
	require.NoError(t, err)
	req.Host = "example.com:8443"

	rec := &redirectRecorder{header: make(http.Header)}
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusPermanentRedirect, rec.code)
	assert.Equal(t, "https://example.com:8443/api/v1/flows?x=1", rec.header.Get("Location"))
}

// redirectRecorder 는 http.ResponseWriter 의 최소 구현으로 상태/헤더를 기록한다.
type redirectRecorder struct {
	header http.Header
	code   int
}

func (r *redirectRecorder) Header() http.Header         { return r.header }
func (r *redirectRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *redirectRecorder) WriteHeader(code int)        { r.code = code }
