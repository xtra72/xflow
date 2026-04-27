package agent

import "context"

// ConnAwareReceiver 는 연결 정보와 함께 메시지를 수신하는 에이전트의 선택적 인터페이스이다.
// TCP 서버처럼 다중 클라이언트를 지원하는 에이전트가 구현하여,
// 어떤 클라이언트로부터 데이터가 수신되었는지 식별할 수 있게 한다.
type ConnAwareReceiver interface {
	// ReceiveMessageFrom 은 메시지와 함께 원격 주소를 반환한다.
	// remoteAddr 는 "host:port" 형식이다.
	ReceiveMessageFrom(ctx context.Context) (data []byte, remoteAddr string, err error)
}
