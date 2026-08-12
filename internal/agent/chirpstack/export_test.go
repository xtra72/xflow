package chirpstack

// HandleUplinkForTest 는 외부 테스트 패키지(chirpstack_test)가 업링크 수신 경로를
// 구동할 수 있도록 노출하는 테스트 전용 seam 이다.
//
// 본 파일은 _test.go 이므로 프로덕션 빌드에 포함되지 않는다 — 공개 API 표면을 넓히지
// 않으면서, MQTT 브로커 없이 inventory 노드까지의 경계 통합 테스트를 가능하게 한다
// (Go 표준 export_test.go 관례).
func (a *ChirpStackAgent) HandleUplinkForTest(raw []byte, topic string) {
	a.handleUplink(raw, topic)
}
