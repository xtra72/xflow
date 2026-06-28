// conn.go 는 gorilla/websocket 연결을 remote.Conn 인터페이스로 래핑하고,
// ws.Message 봉투 송신 헬퍼를 제공한다. 클라이언트(dial)와 서버(upgrade) 양쪽이
// 동일한 Conn 추상화를 공유하여 전송 비의존 로직을 테스트 가능하게 한다.
package remote

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/xtra/xflow/internal/api/ws"
)

// gorillaConn 은 *websocket.Conn 을 remote.Conn 으로 어댑트한다.
//
// gorilla/websocket 은 동시 Writer 를 허용하지 않으므로(단일 writer), 쓰기를
// 뮤텍스로 직렬화한다. 읽기는 단일 고루틴에서만 호출된다(세션 읽기 루프).
type gorillaConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

var _ Conn = (*gorillaConn)(nil)

// newGorillaConn 은 *websocket.Conn 래퍼를 생성한다.
func newGorillaConn(conn *websocket.Conn) *gorillaConn {
	return &gorillaConn{conn: conn}
}

// NewGorillaConn 은 업그레이드된 *websocket.Conn 을 remote.Conn 으로 래핑한다.
// 관리 WS 핸들러(internal/api/handler/remote.go)가 서버 측 연결을 Server 로
// 넘길 때 사용한다.
func NewGorillaConn(conn *websocket.Conn) Conn {
	return newGorillaConn(conn)
}

// ReadMessage 는 다음 텍스트/바이너리 메시지 페이로드를 읽는다.
func (g *gorillaConn) ReadMessage() ([]byte, error) {
	_, data, err := g.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WriteMessage 는 텍스트 메시지로 데이터를 전송한다(쓰기 직렬화).
func (g *gorillaConn) WriteMessage(data []byte) error {
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	return g.conn.WriteMessage(websocket.TextMessage, data)
}

// Close 는 연결을 닫는다.
func (g *gorillaConn) Close() error {
	return g.conn.Close()
}

// writeEnvelope 는 ws.Message 봉투를 인코딩하여 Conn 으로 전송한다.
func writeEnvelope(conn Conn, msg *ws.Message) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	return conn.WriteMessage(data)
}
