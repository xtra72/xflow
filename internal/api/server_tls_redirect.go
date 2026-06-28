package api

import (
	"errors"
	"net"
	"net/http"
	"sync"
)

// tlsRecordTypeHandshake 는 TLS 레코드의 첫 바이트(ContentType) 중 handshake(ClientHello)
// 를 나타내는 값이다. 평문 HTTP 요청의 첫 바이트('G','P','H' 등)와는 절대 겹치지 않으므로
// 이 1바이트만으로 TLS/평문 트래픽을 안전하게 구분할 수 있다.
const tlsRecordTypeHandshake = 0x16

// errListenerClosed 는 chanListener 가 닫힌 뒤 Accept 가 반환하는 센티넬 에러이다.
// http.Server.Serve 는 이 에러를 받으면 서빙을 종료한다. Start 의 serve 고루틴은
// 이 에러를 http.ErrServerClosed 와 동일하게 "정상 종료"로 취급해 errCh 로 전파하지 않는다.
var errListenerClosed = errors.New("api: split listener closed")

// peekedConn 은 첫 바이트를 미리 읽은 net.Conn 을 감싸서, 이후 Read 시
// 미리 읽은 바이트를 먼저 흘려보낸 뒤 원본 연결로 위임한다.
// 이를 통해 다운스트림(TLS 핸드셰이크 파서 또는 HTTP 파서)이 원본 스트림 전체를
// 손실 없이 그대로 보게 된다.
type peekedConn struct {
	net.Conn
	prefix []byte // 아직 소비되지 않은 미리 읽은 바이트
}

// Read 는 미리 읽어둔 prefix 를 먼저 비운 뒤 원본 연결로 위임한다.
func (c *peekedConn) Read(b []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}

// chanListener 는 버퍼드 채널로 연결을 공급받는 net.Listener 구현이다.
// 디스패처가 분류한 연결을 conns 채널로 밀어넣고, http.Server 는 Accept 로 꺼내 쓴다.
type chanListener struct {
	conns     chan net.Conn
	addr      net.Addr
	closeOnce sync.Once
	done      chan struct{}
}

// newChanListener 는 지정한 주소를 광고하는 chanListener 를 생성한다.
func newChanListener(addr net.Addr, buf int) *chanListener {
	return &chanListener{
		conns: make(chan net.Conn, buf),
		addr:  addr,
		done:  make(chan struct{}),
	}
}

// Accept 는 채널에서 다음 연결을 반환한다. 리스너가 닫히면(done) errListenerClosed 를 반환한다.
// conns 채널은 닫지 않고 done 으로만 종료를 신호하므로, 닫힌 뒤에는 항상 errListenerClosed 이다.
func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, errListenerClosed
	}
}

// Close 는 리스너를 닫는다(멱등). done 채널을 닫아 진행 중인 Accept 와
// push(select) 를 깨운다.
func (l *chanListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
	})
	return nil
}

// Addr 은 광고용 주소(실제 리스너의 주소)를 반환한다.
func (l *chanListener) Addr() net.Addr {
	return l.addr
}

// push 는 연결을 채널로 전달한다. 리스너가 이미 닫혔다면(done) 연결을 닫아
// 고루틴/연결 누수를 방지한다.
//
// done 닫힘을 먼저 확인하는 이유: select 는 송신/수신 케이스가 동시에 준비되면
// 무작위로 하나를 고른다. 리스너가 닫힌 뒤에도 conns 버퍼에 여유가 있으면
// 송신 케이스가 선택되어 닫힌 리스너에 연결이 쌓일 수 있으므로, 먼저 done 을
// 논블로킹으로 확인해 닫힘 상태를 우선 처리한다.
func (l *chanListener) push(c net.Conn) {
	select {
	case <-l.done:
		_ = c.Close()
		return
	default:
	}

	select {
	case l.conns <- c:
	case <-l.done:
		_ = c.Close()
	}
}

// tlsRedirectDispatcher 는 단일 TCP 리스너(ln)로 들어온 연결의 첫 바이트를 들여다보고,
// TLS ClientHello(0x16) 면 TLS 경로로, 그 외(평문 HTTP) 면 리다이렉트 경로로 분기한다.
// 두 경로는 각각 chanListener 로 노출되어 별도의 http.Server 가 서빙한다.
type tlsRedirectDispatcher struct {
	ln      net.Listener
	tlsLn   *chanListener
	redirLn *chanListener
	closeLn sync.Once
}

// chanListenerBufSize 는 분기 채널의 버퍼 크기이다. 순간적인 연결 버스트가
// peek 고루틴을 블로킹하지 않도록 적당한 여유를 둔다.
const chanListenerBufSize = 64

// newTLSRedirectDispatcher 는 실제 리스너를 감싸는 디스패처를 만들고 분기 리스너를 준비한다.
func newTLSRedirectDispatcher(ln net.Listener) *tlsRedirectDispatcher {
	return &tlsRedirectDispatcher{
		ln:      ln,
		tlsLn:   newChanListener(ln.Addr(), chanListenerBufSize),
		redirLn: newChanListener(ln.Addr(), chanListenerBufSize),
	}
}

// tlsListener 는 TLS 트래픽을 공급하는 리스너를 반환한다(httpServer.ServeTLS 용).
func (d *tlsRedirectDispatcher) tlsListener() net.Listener { return d.tlsLn }

// redirectListener 는 평문 HTTP 트래픽을 공급하는 리스너를 반환한다(redirectServer.Serve 용).
func (d *tlsRedirectDispatcher) redirectListener() net.Listener { return d.redirLn }

// run 은 Accept 루프를 돈다. 각 연결의 첫 바이트 peek 은 연결마다 별도 고루틴에서
// 수행하므로, 느리거나 idle 한 단일 클라이언트가 Accept 루프나 다른 연결의 분기를
// 가로막지 못한다. 블로킹이므로 호출자가 고루틴에서 실행해야 한다.
func (d *tlsRedirectDispatcher) run() {
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			// 실제 리스너가 닫혔거나 치명적 에러 → 분기 리스너를 닫아
			// 다운스트림 http.Server 들이 정상 종료되도록 한다.
			d.tlsLn.Close()
			d.redirLn.Close()
			return
		}
		go d.peekAndRoute(conn)
	}
}

// peekAndRoute 는 연결의 첫 바이트를 읽어 TLS/평문을 판별하고 해당 분기 리스너로 보낸다.
// peek 에 실패하면 연결을 닫는다.
func (d *tlsRedirectDispatcher) peekAndRoute(conn net.Conn) {
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		_ = conn.Close()
		return
	}

	pc := &peekedConn{Conn: conn, prefix: buf[:n]}
	if buf[0] == tlsRecordTypeHandshake {
		d.tlsLn.push(pc)
		return
	}
	d.redirLn.push(pc)
}

// Close 는 실제 리스너를 닫아 Accept 루프를 종료시킨다(멱등).
// Accept 루프가 깨어나면서 분기 리스너들도 닫는다.
func (d *tlsRedirectDispatcher) Close() error {
	d.closeLn.Do(func() {
		_ = d.ln.Close()
	})
	return nil
}

// newHTTPSRedirectHandler 는 평문 HTTP 요청을 동일 host:port 의 https:// 로
// 308 Permanent Redirect 시키는 핸들러를 반환한다.
// r.Host 는 클라이언트가 사용한 원래 host:port 를 보존하며, scheme 만 https 로 바꾼다.
// RequestURI() 는 path 와 query 를 그대로 보존한다.
func newHTTPSRedirectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}
