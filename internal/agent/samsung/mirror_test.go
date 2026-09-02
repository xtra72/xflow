package samsung

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// fakeBus 는 in-memory MQTT 대체 버스이다. 여러 broker 핸들이 공유해 pub→sub 전달을
// 시뮬레이션한다. retained 메시지는 신규 구독 즉시 replay 되어 7a 재동기화를 검증한다.
// ---------------------------------------------------------------------------

type publishRecord struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

type subscription struct {
	pattern string
	handler func(topic string, payload []byte)
}

type fakeBus struct {
	mu         sync.Mutex
	subs       []subscription
	retained   map[string][]byte // 토픽별 최신 retained payload
	published  []publishRecord
	publishErr error // 설정 시 Publish 가 실패 (발행 실패 격리 테스트용)
}

func newFakeBus() *fakeBus {
	return &fakeBus{retained: make(map[string][]byte)}
}

// topicMatch 는 '+' 단일 세그먼트 와일드카드를 지원하는 토픽 매칭이다.
func topicMatch(pattern, topic string) bool {
	if pattern == topic {
		return true
	}
	ps := strings.Split(pattern, "/")
	ts := strings.Split(topic, "/")
	if len(ps) != len(ts) {
		return false
	}
	for i := range ps {
		if ps[i] == "+" {
			continue
		}
		if ps[i] != ts[i] {
			return false
		}
	}
	return true
}

func (bus *fakeBus) publish(topic string, qos byte, retained bool, payload []byte) error {
	bus.mu.Lock()
	if bus.publishErr != nil {
		err := bus.publishErr
		bus.mu.Unlock()
		return err
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	bus.published = append(bus.published, publishRecord{topic, qos, retained, cp})
	if retained {
		bus.retained[topic] = cp
	}
	// 핸들러를 스냅샷 후 락 밖에서 호출(재진입 deadlock 회피 — ack 경로가 재발행 가능).
	var targets []subscription
	for _, s := range bus.subs {
		if topicMatch(s.pattern, topic) {
			targets = append(targets, s)
		}
	}
	bus.mu.Unlock()

	for _, s := range targets {
		s.handler(topic, cp)
	}
	return nil
}

func (bus *fakeBus) subscribe(patterns []string, handler func(topic string, payload []byte)) {
	bus.mu.Lock()
	var replay []publishRecord
	for _, p := range patterns {
		bus.subs = append(bus.subs, subscription{pattern: p, handler: handler})
		// retained replay: 신규 구독에 기존 retained 메시지 전달(7a).
		for topic, payload := range bus.retained {
			if topicMatch(p, topic) {
				replay = append(replay, publishRecord{topic: topic, payload: payload})
			}
		}
	}
	bus.mu.Unlock()

	for _, r := range replay {
		handler(r.topic, r.payload)
	}
}

func (bus *fakeBus) publishesFor(pattern string) []publishRecord {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	var out []publishRecord
	for _, p := range bus.published {
		if topicMatch(pattern, p.topic) {
			out = append(out, p)
		}
	}
	return out
}

func (bus *fakeBus) setPublishErr(err error) {
	bus.mu.Lock()
	bus.publishErr = err
	bus.mu.Unlock()
}

// fakeBroker 는 fakeBus 를 공유하는 MirrorBroker 핸들이다.
type fakeBroker struct{ bus *fakeBus }

func (b *fakeBroker) Publish(topic string, qos byte, retained bool, payload []byte) error {
	return b.bus.publish(topic, qos, retained, payload)
}
func (b *fakeBroker) Subscribe(topics []string, handler func(string, []byte)) error {
	b.bus.subscribe(topics, handler)
	return nil
}
func (b *fakeBroker) Close() error { return nil }

// withFakeBus 는 mirrorBrokerFactory 를 fakeBus 기반으로 대체하고 복원 함수를 반환한다.
func withFakeBus(t *testing.T) *fakeBus {
	t.Helper()
	bus := newFakeBus()
	orig := mirrorBrokerFactory
	mirrorBrokerFactory = func(_ mirrorBrokerConn, _ *slog.Logger) (MirrorBroker, error) {
		return &fakeBroker{bus: bus}, nil
	}
	t.Cleanup(func() { mirrorBrokerFactory = orig })
	return bus
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("조건이 %v 내에 충족되지 않음", timeout)
}

// validFrame 은 지정 주소/전원/모드의 유효한 C0 14 프레임을 생성한다.
func validFrame(t *testing.T, addr NasaAddress, power byte, mode string) []byte {
	t.Helper()
	proto := NewNasaProtocol()
	frame, err := proto.Encode(&NasaMessage{
		SourceAddr:  addr,
		DestAddr:    NasaAddress{0x6A, 0xEE, 0xFF},
		CommandCode: CmdNotification,
		SequenceNum: 1,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{power}},
			{Index: MsgMode, Value: []byte{StringToMode[mode]}},
		},
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return frame
}

func gatewayConfig(gwID string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "gw",
		Name: "gateway",
		Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"transport_type":        "serial",
				"serial_port":           "/dev/null",
				"mirror_uplink_enabled": true,
				"mirror_broker":         "tcp://x:1883",
				"mirror_gateway_id":     gwID,
			},
		},
	}
}

func serverConfig(gwID string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "srv",
		Name: "server",
		Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{
			Type: "mirror-mqtt",
			Options: map[string]any{
				"transport_type":    "mirror-mqtt",
				"mirror_broker":     "tcp://x:1883",
				"mirror_gateway_id": gwID,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Module 2: 서버 mirror 입력
// ---------------------------------------------------------------------------

// TestMirrorTransportRegistered 는 "mirror-mqtt" transport_type 이 config switch 와 팩토리에
// 등록되었는지 검증한다 (AC-2.1 / AC-8.3).
func TestMirrorTransportRegistered(t *testing.T) {
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type":    "mirror-mqtt",
		"mirror_broker":     "tcp://x:1883",
		"mirror_gateway_id": "gw01",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config(mirror-mqtt): %v", err)
	}
	if !cfg.MirrorMode {
		t.Error("mirror-mqtt transport_type 은 MirrorMode 를 활성화해야 함")
	}
	tr, err := NewNasaTransport("mirror-mqtt", nil)
	if err != nil {
		t.Fatalf("NewNasaTransport(mirror-mqtt): %v", err)
	}
	if _, ok := tr.(*mirrorTransport); !ok {
		t.Errorf("mirror-mqtt 팩토리는 *mirrorTransport 를 반환해야 함, got %T", tr)
	}
}

// TestMirrorConfigValidation 은 미러/업링크 모드에서 브로커/게이트웨이 식별자 누락 시
// 명시적 에러를 반환하는지 검증한다 (AC-2.3).
func TestMirrorConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
	}{
		{"mirror broker 누락", map[string]any{"transport_type": "mirror-mqtt", "mirror_gateway_id": "gw01"}},
		{"mirror gateway_id 누락", map[string]any{"transport_type": "mirror-mqtt", "mirror_broker": "tcp://x:1883"}},
		{"uplink broker 누락", map[string]any{"transport_type": "serial", "serial_port": "/dev/null", "mirror_uplink_enabled": true, "mirror_gateway_id": "gw01"}},
		{"uplink gateway_id 누락", map[string]any{"transport_type": "serial", "serial_port": "/dev/null", "mirror_uplink_enabled": true, "mirror_broker": "tcp://x:1883"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseHvacr01Config(tc.opts); err == nil {
				t.Fatal("누락 설정을 수용하면 안 됨(명시적 에러 필요)")
			}
		})
	}
}

// TestMirrorTransportChannelFeed 는 채널 급전형 transport 의 Feed→Receive 와 Close 시
// 언블록을 검증한다.
func TestMirrorTransportChannelFeed(t *testing.T) {
	mt := newMirrorTransport()
	_ = mt.Open()
	if !mt.Available() {
		t.Fatal("Open 후 Available 이어야 함")
	}
	mt.Feed([]byte{0x01, 0x02, 0x03})
	buf := make([]byte, 16)
	n, err := mt.Receive(buf)
	if err != nil || n != 3 {
		t.Fatalf("Receive: n=%d err=%v", n, err)
	}
	// Close 는 블록된 Receive 를 깨운다.
	done := make(chan struct{})
	go func() {
		_, _ = mt.Receive(buf)
		close(done)
	}()
	time.Sleep(5 * time.Millisecond)
	_ = mt.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close 후 Receive 가 언블록되지 않음")
	}
	if mt.Available() {
		t.Error("Close 후 Available 은 false 여야 함")
	}
}

// TestMirrorConverges 는 게이트웨이 업링크 재생 시 서버 상태가 수렴하는지 검증한다
// (AC-2.2 / AC-E2E-1). online/offline·discovery replay 포함(AC-7.1).
func TestMirrorConverges(t *testing.T) {
	bus := withFakeBus(t)
	addr := NasaAddress{0x20, 0x00, 0x00}

	// 서버: mirror 입력 + Start(receiveLoop 기동, 구독).
	srv := mustAgent(t, serverConfig("gw01"))
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()

	// 게이트웨이: 업링크 tap 기동(broker 시작만, 로컬 receiveLoop 없이 직접 급전).
	gw := mustAgent(t, gatewayConfig("gw01"))
	gw.startMirror()

	// 게이트웨이가 프레임을 수신·디코드했다고 가정(RX 경로).
	gw.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	_ = bus // 발행은 tap 내부에서 수행됨

	// 서버측 상태가 게이트웨이와 동일하게 수렴해야 한다.
	eventually(t, 2*time.Second, func() bool {
		st, err := srv.GetDeviceState(addr)
		return err == nil && st != nil && st.Power
	})
	srvState, err := srv.GetDeviceState(addr)
	if err != nil {
		t.Fatalf("server GetDeviceState: %v", err)
	}
	if !srvState.Power || srvState.Mode != "cool" {
		t.Errorf("서버 상태 미수렴: power=%v mode=%q", srvState.Power, srvState.Mode)
	}
	// 게이트웨이 상태도 동일해야 한다(게이트웨이도 완전 상태 보유).
	gwState, err := gw.GetDeviceState(addr)
	if err != nil || !gwState.Power || gwState.Mode != "cool" {
		t.Errorf("게이트웨이 상태 불일치: %+v err=%v", gwState, err)
	}
}

// ---------------------------------------------------------------------------
// Module 3: 업링크 tap
// ---------------------------------------------------------------------------

// TestUplinkTapPublishesDecodedOnly 는 디코드 성공 프레임만 업링크됨을 검증한다
// (AC-3.1). CRC 불량 프레임은 발행되지 않는다.
func TestUplinkTapPublishesDecodedOnly(t *testing.T) {
	bus := withFakeBus(t)
	addr := NasaAddress{0x20, 0x00, 0x00}

	gw := mustAgent(t, gatewayConfig("gw01"))
	gw.startMirror()

	good := validFrame(t, addr, 0x01, "cool")
	bad := validFrame(t, addr, 0x01, "cool")
	bad[len(bad)-3] ^= 0xFF // CRC 영역 손상 → 디코드 실패

	gw.ingestFrameBytes(good)
	gw.ingestFrameBytes(bad)

	pubs := bus.publishesFor("xflow/hvacr/gw01/up/nasa")
	if len(pubs) != 1 {
		t.Fatalf("유효 프레임 1건만 업링크되어야 함, got %d", len(pubs))
	}
}

// TestUplinkTapDisabled 는 mirror 미설정 시 단독 동작(발행 없음)을 검증한다 (AC-3.2).
func TestUplinkTapDisabled(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "solo", Name: "solo", Type: "samsung_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: map[string]any{
			"transport_type": "serial", "serial_port": "/dev/null",
		}},
	}
	a := mustAgent(t, cfg)
	if a.mirror != nil {
		t.Fatal("mirror 미설정 시 a.mirror 는 nil 이어야 함(단독 동작)")
	}
	// tap 은 no-op 이어야 하며 패닉 없이 상태만 갱신한다.
	a.ingestFrameBytes(validFrame(t, NasaAddress{0x20, 0, 0}, 0x01, "cool"))
	st, err := a.GetDeviceState(NasaAddress{0x20, 0, 0})
	if err != nil || !st.Power {
		t.Errorf("단독 에이전트 상태 갱신 실패: %+v err=%v", st, err)
	}
}

// TestUplinkPublishFailureIsolated 는 발행 실패가 로컬 상태 갱신을 차단하지 않음을
// 검증한다 (AC-3.3).
func TestUplinkPublishFailureIsolated(t *testing.T) {
	bus := withFakeBus(t)
	bus.setPublishErr(context.DeadlineExceeded)
	addr := NasaAddress{0x20, 0x00, 0x00}

	gw := mustAgent(t, gatewayConfig("gw01"))
	gw.startMirror()
	gw.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	// 발행이 실패해도 로컬 상태는 갱신되어야 한다.
	st, err := gw.GetDeviceState(addr)
	if err != nil || !st.Power {
		t.Fatalf("발행 실패 시에도 로컬 상태는 갱신되어야 함: %+v err=%v", st, err)
	}
	if gw.mirror.publishErrs.Load() == 0 {
		t.Error("발행 실패 카운터가 증가해야 함")
	}
}

// ---------------------------------------------------------------------------
// Module 4: 제어 역경로
// ---------------------------------------------------------------------------

// TestServerControlPublishesDownlink 는 서버 제어가 로컬 실행 없이 다운링크로 발행되는지
// 검증한다 (AC-4.1).
func TestServerControlPublishesDownlink(t *testing.T) {
	bus := withFakeBus(t)
	srv := mustAgent(t, serverConfig("gw01"))
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()

	// mirrorTransport 는 no-op Send 이므로 로컬 실행이 없음을 보장한다.
	reqJSON, _ := json.Marshal(map[string]any{
		"command": "set_power", "address": "200000", "params": map[string]any{"power": true},
	})
	resp, err := srv.Process(reqJSON)
	if err != nil {
		t.Fatalf("server Process: %v", err)
	}
	var r map[string]any
	_ = json.Unmarshal(resp, &r)
	if r["status"] != "accepted" {
		t.Errorf("서버 제어 응답은 accepted 여야 함: %v", r)
	}

	pubs := bus.publishesFor("xflow/hvacr/gw01/down/control")
	if len(pubs) != 1 {
		t.Fatalf("다운링크 제어 1건이 발행되어야 함, got %d", len(pubs))
	}
	if pubs[0].retained {
		t.Error("다운링크 제어는 retain=false 여야 함(REQ-07-03)")
	}
}

// TestGatewayDownlinkInvokesProcess 는 게이트웨이가 다운링크를 구독해 Process 로
// 실제 제어를 실행하는지 검증한다 (AC-4.2 / AC-E2E-2).
func TestGatewayDownlinkInvokesProcess(t *testing.T) {
	bus := withFakeBus(t)
	addr := NasaAddress{0x20, 0x00, 0x00}

	// 게이트웨이: mockTransport 주입으로 로컬 Send 캡처.
	gw := mustAgent(t, gatewayConfig("gw01"))
	mock := &mockTransport{available: true}
	gw.transport = mock
	gw.startMirror() // down/control 구독

	// 디바이스 온라인화(디코드 프레임 주입 → auto-discovery).
	gw.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	// 서버측이 다운링크를 발행(직접 bus.publish 로 시뮬레이션).
	ctrl, _ := json.Marshal(wireControl{
		Command: "set_multiple", Address: "200000",
		Params: map[string]any{"power": true, "target_temperature": 24.0},
	})
	_ = bus.publish("xflow/hvacr/gw01/down/control", 1, false, ctrl)

	// 게이트웨이가 Process→sendControlCommand→로컬 Send 를 수행해야 한다.
	eventually(t, time.Second, func() bool { return len(mock.getSentData()) > 0 })
	if len(mock.getSentData()) == 0 {
		t.Fatal("게이트웨이 로컬 transport 로 제어 프레임이 전송되지 않음")
	}
}

// ---------------------------------------------------------------------------
// Module 6/7: 토픽 구독 + QoS/retain + 재동기화
// ---------------------------------------------------------------------------

// TestServerSubscribesUplinkTopics 는 서버가 올바른 업링크 토픽을 구독하는지 검증한다
// (AC-6.2).
func TestServerSubscribesUplinkTopics(t *testing.T) {
	bus := withFakeBus(t)
	srv := mustAgent(t, serverConfig("gw01"))
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()

	bus.mu.Lock()
	found := false
	for _, s := range bus.subs {
		if s.pattern == "xflow/hvacr/gw01/up/nasa" {
			found = true
		}
	}
	bus.mu.Unlock()
	if !found {
		t.Error("서버는 .../gw01/up/nasa 를 구독해야 함")
	}
}

// TestUplinkQoSRetainPolicy 는 업링크/다운링크/스냅샷 발행의 QoS/retain 정책을 검증한다
// (AC-7.3).
func TestUplinkQoSRetainPolicy(t *testing.T) {
	bus := withFakeBus(t)
	addr := NasaAddress{0x20, 0x00, 0x00}

	cfg := gatewayConfig("gw01")
	cfg.Transport.Options["mirror_snapshot_enabled"] = true
	gw := mustAgent(t, cfg)
	gw.startMirror()
	gw.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	up := bus.publishesFor("xflow/hvacr/gw01/up/nasa")
	if len(up) != 1 || up[0].qos < 1 || up[0].retained {
		t.Errorf("업링크는 QoS>=1 retain=false 여야 함: %+v", up)
	}
	snap := bus.publishesFor("xflow/hvacr/gw01/up/snapshot/100000")
	// snapshot 토픽은 addr hex(200000) 이므로 정확히 조회.
	snap = bus.publishesFor("xflow/hvacr/gw01/up/snapshot/200000")
	if len(snap) != 1 || !snap[0].retained {
		t.Errorf("스냅샷은 retain=true 여야 함: %+v", snap)
	}
}

// TestResyncOnRestart 는 retain 스냅샷(7a)으로 재시작 서버가 상태를 즉시 복원하는지
// 검증한다 (AC-7.2).
func TestResyncOnRestart(t *testing.T) {
	_ = withFakeBus(t) // 공유 버스 활성화(retained replay 는 버스 내부에서 수행)
	addr := NasaAddress{0x20, 0x00, 0x00}

	// 게이트웨이가 스냅샷을 retained 로 발행.
	cfg := gatewayConfig("gw01")
	cfg.Transport.Options["mirror_snapshot_enabled"] = true
	gw := mustAgent(t, cfg)
	gw.startMirror()
	gw.ingestFrameBytes(validFrame(t, addr, 0x01, "cool"))

	// 이후 새 서버가 구독 → retained 스냅샷 즉시 수신 → 상태 복원.
	srv := mustAgent(t, serverConfigSnapshot("gw01"))
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()

	eventually(t, 2*time.Second, func() bool {
		st, err := srv.GetDeviceState(addr)
		return err == nil && st != nil && st.Power
	})
}

func serverConfigSnapshot(gwID string) agent.AgentConfig {
	cfg := serverConfig(gwID)
	cfg.Transport.Options["mirror_snapshot_enabled"] = true
	return cfg
}

// mustAgent 는 NewHvacr01Agent 를 호출하고 *Hvacr01Agent 로 캐스팅한다.
func mustAgent(t *testing.T, cfg agent.AgentConfig) *Hvacr01Agent {
	t.Helper()
	a, err := NewHvacr01Agent(cfg)
	if err != nil {
		t.Fatalf("NewHvacr01Agent: %v", err)
	}
	return a.(*Hvacr01Agent)
}

// ---------------------------------------------------------------------------
// M9: 미러 브로커 보안 (인증 + TLS)
// ---------------------------------------------------------------------------

// genTestCAPEM 은 테스트용 자체서명 CA 인증서 PEM 을 생성한다.
func genTestCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("키 생성: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "xflow-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("인증서 생성: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// TestBuildMirrorTLSConfig 는 CA 인증서 소스별 TLS 구성 동작을 검증한다 (M9).
//   - 빈 CA → 시스템 루트(RootCAs nil), MinVersion TLS12.
//   - 유효 PEM → RootCAs 채워짐.
//   - 파일 경로 PEM → RootCAs 채워짐.
//   - 잘못된 PEM → 에러.
func TestBuildMirrorTLSConfig(t *testing.T) {
	// 빈 CA: 시스템 루트 사용.
	cfg, err := buildMirrorTLSConfig("")
	if err != nil {
		t.Fatalf("빈 CA: 예상치 못한 에러: %v", err)
	}
	if cfg == nil || cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("빈 CA: MinVersion TLS1.2 config 여야 함: %+v", cfg)
	}
	if cfg.RootCAs != nil {
		t.Error("빈 CA: RootCAs 는 nil(시스템 루트)여야 함")
	}

	// 유효 PEM 문자열.
	caPEM := genTestCAPEM(t)
	cfg, err = buildMirrorTLSConfig(caPEM)
	if err != nil {
		t.Fatalf("PEM 문자열: 예상치 못한 에러: %v", err)
	}
	if cfg == nil || cfg.RootCAs == nil {
		t.Error("PEM 문자열: RootCAs 가 채워져야 함")
	}

	// 파일 경로.
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte(caPEM), 0o600); err != nil {
		t.Fatalf("CA 파일 쓰기: %v", err)
	}
	cfg, err = buildMirrorTLSConfig(path)
	if err != nil {
		t.Fatalf("파일 경로: 예상치 못한 에러: %v", err)
	}
	if cfg == nil || cfg.RootCAs == nil {
		t.Error("파일 경로: RootCAs 가 채워져야 함")
	}

	// 잘못된 PEM.
	if _, err := buildMirrorTLSConfig("-----BEGIN CERTIFICATE-----\nnot-valid\n-----END CERTIFICATE-----"); err == nil {
		t.Error("잘못된 PEM 은 에러를 반환해야 함")
	}
}

// TestMirrorSecurityConfigParsing 은 인증/TLS 옵션 4종이 config 로 파싱되는지 검증한다 (M9).
func TestMirrorSecurityConfigParsing(t *testing.T) {
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type":    "mirror-mqtt",
		"mirror_broker":     "ssl://x:8883",
		"mirror_gateway_id": "gw01",
		"mirror_username":   "user1",
		"mirror_password":   "secret",
		"mirror_tls":        true,
		"mirror_ca_cert":    "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config: %v", err)
	}
	if cfg.MirrorUsername != "user1" {
		t.Errorf("MirrorUsername=%q, want user1", cfg.MirrorUsername)
	}
	if cfg.MirrorPassword != "secret" {
		t.Errorf("MirrorPassword=%q, want secret", cfg.MirrorPassword)
	}
	if !cfg.MirrorTLS {
		t.Error("MirrorTLS 는 true 여야 함")
	}
	if !strings.Contains(cfg.MirrorCACert, "BEGIN CERTIFICATE") {
		t.Errorf("MirrorCACert 미파싱: %q", cfg.MirrorCACert)
	}

	// 미설정 시 기존 동작 보존(빈 값/false).
	def, err := parseHvacr01Config(map[string]any{
		"transport_type":    "mirror-mqtt",
		"mirror_broker":     "tcp://x:1883",
		"mirror_gateway_id": "gw01",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config(default): %v", err)
	}
	if def.MirrorUsername != "" || def.MirrorPassword != "" || def.MirrorTLS || def.MirrorCACert != "" {
		t.Errorf("미설정 시 보안 필드는 비어 있어야 함: %+v", def)
	}
}

// TestMirrorSecurityConnPropagation 은 config 의 보안 필드가 브로커 팩토리 conn 으로
// 전달되는지 검증한다 (M9). 실제 paho 클라이언트 옵션은 내부 상태라 직접 단언이 어려우므로
// 팩토리에 전달되는 mirrorBrokerConn 을 캡처해 검증한다.
func TestMirrorSecurityConnPropagation(t *testing.T) {
	var captured mirrorBrokerConn
	orig := mirrorBrokerFactory
	mirrorBrokerFactory = func(conn mirrorBrokerConn, _ *slog.Logger) (MirrorBroker, error) {
		captured = conn
		return &fakeBroker{bus: newFakeBus()}, nil
	}
	t.Cleanup(func() { mirrorBrokerFactory = orig })

	cfg := gatewayConfig("gw01")
	cfg.Transport.Options["mirror_username"] = "user1"
	cfg.Transport.Options["mirror_password"] = "secret"
	cfg.Transport.Options["mirror_tls"] = true
	cfg.Transport.Options["mirror_ca_cert"] = genTestCAPEM(t)
	gw := mustAgent(t, cfg)
	gw.startMirror()

	if captured.username != "user1" || captured.password != "secret" {
		t.Errorf("자격증명 미전달: %+v", captured)
	}
	if !captured.tls {
		t.Error("tls 플래그 미전달")
	}
	if !strings.Contains(captured.caCert, "BEGIN CERTIFICATE") {
		t.Errorf("caCert 미전달: %q", captured.caCert)
	}
}
