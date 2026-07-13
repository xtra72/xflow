package socket

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// logFlag 는 소켓 에이전트의 내부 logMessages atomic 값을 읽는다 (테스트 전용).
func logFlag(a agent.Agent) bool {
	switch v := a.(type) {
	case *TCPServerAgent:
		return v.logMessages.Load()
	case *TCPClientAgent:
		return v.logMessages.Load()
	case *UDPServerAgent:
		return v.logMessages.Load()
	case *UDPClientAgent:
		return v.logMessages.Load()
	}
	return false
}

// TestConfigure_LogMessagesLiveToggle 는 log_messages 를 Configure 로 토글하면
// 에이전트 재생성 없이 즉시 반영되는지 검증한다 (재시작 없는 라이브 반영 회귀 방지).
//
// 회귀 배경: 이전 Configure 는 Transport.Options 를 재파싱하지 않아, UI 에서
// log_messages 를 켜도 running 에이전트에 반영되지 않았다 (needsRestart 대상도 아님).
func TestConfigure_LogMessagesLiveToggle(t *testing.T) {
	mkCfg := func(typ string, logMessages any) agent.AgentConfig {
		opts := map[string]any{"host": "127.0.0.1", "port": float64(19000)}
		if logMessages != nil {
			opts["log_messages"] = logMessages
		}
		return agent.AgentConfig{
			ID:        "test-" + typ,
			Name:      "Test " + typ,
			Type:      typ,
			Transport: agent.TransportConfig{Type: typ, Options: opts},
		}
	}

	cases := []struct {
		typ   string
		build func(agent.AgentConfig) (agent.Agent, error)
	}{
		{"tcp-server", NewTCPServerAgent},
		{"tcp-client", NewTCPClientAgent},
		{"udp-server", NewUDPServerAgent},
		{"udp-client", NewUDPClientAgent},
	}

	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			// log_messages 미지정 → 기본 false.
			a, err := tc.build(mkCfg(tc.typ, nil))
			if err != nil {
				t.Fatalf("build %s: %v", tc.typ, err)
			}
			if logFlag(a) {
				t.Fatalf("%s: 초기 logMessages 는 false 여야 함", tc.typ)
			}

			// Configure(log_messages=true) → 재시작 없이 즉시 true.
			if err := a.Configure(mkCfg(tc.typ, true)); err != nil {
				t.Fatalf("%s Configure(true): %v", tc.typ, err)
			}
			if !logFlag(a) {
				t.Errorf("%s: Configure(true) 후 logMessages 가 true 여야 함", tc.typ)
			}

			// Configure(log_messages=false) → 다시 false.
			if err := a.Configure(mkCfg(tc.typ, false)); err != nil {
				t.Fatalf("%s Configure(false): %v", tc.typ, err)
			}
			if logFlag(a) {
				t.Errorf("%s: Configure(false) 후 logMessages 가 false 여야 함", tc.typ)
			}
		})
	}
}

// TestParseConfig_LogMessages 는 log_messages 옵션이 4개 소켓 에이전트 설정 파서
// (tcp/udp × server/client) 에 공통으로 반영되는지, 그리고 미지정 시 기본 false
// 인지 검증한다. SocketConfig 공통 필드로 정의되어 embedding 을 통해 모든 파서에
// 노출된다.
func TestParseConfig_LogMessages(t *testing.T) {
	base := map[string]any{"host": "127.0.0.1", "port": float64(9000)}

	t.Run("기본값 false", func(t *testing.T) {
		ts, err := ParseTCPServerConfig(base)
		if err != nil {
			t.Fatalf("ParseTCPServerConfig: %v", err)
		}
		if ts.LogMessages {
			t.Errorf("log_messages 미지정 시 기본 false 여야 함, got true")
		}
	})

	t.Run("log_messages=true 가 4개 파서에 반영됨", func(t *testing.T) {
		opts := map[string]any{"host": "127.0.0.1", "port": float64(9000), "log_messages": true}

		ts, err := ParseTCPServerConfig(opts)
		if err != nil {
			t.Fatalf("ParseTCPServerConfig: %v", err)
		}
		if !ts.LogMessages {
			t.Errorf("tcp-server: log_messages 가 true 여야 함")
		}

		tc, err := ParseTCPClientConfig(opts)
		if err != nil {
			t.Fatalf("ParseTCPClientConfig: %v", err)
		}
		if !tc.LogMessages {
			t.Errorf("tcp-client: log_messages 가 true 여야 함")
		}

		us, err := ParseUDPServerConfig(opts)
		if err != nil {
			t.Fatalf("ParseUDPServerConfig: %v", err)
		}
		if !us.LogMessages {
			t.Errorf("udp-server: log_messages 가 true 여야 함")
		}

		uc, err := ParseUDPClientConfig(opts)
		if err != nil {
			t.Fatalf("ParseUDPClientConfig: %v", err)
		}
		if !uc.LogMessages {
			t.Errorf("udp-client: log_messages 가 true 여야 함")
		}
	})

	t.Run("문자열 true 도 허용", func(t *testing.T) {
		opts := map[string]any{"host": "127.0.0.1", "port": float64(9000), "log_messages": "true"}
		ts, err := ParseTCPServerConfig(opts)
		if err != nil {
			t.Fatalf("ParseTCPServerConfig: %v", err)
		}
		if !ts.LogMessages {
			t.Errorf("문자열 \"true\" 도 log_messages=true 로 파싱되어야 함")
		}
	})
}

// TestLogPacket 는 logPacket 헬퍼의 동작을 검증한다:
//   - enabled=true 면 prefix + dir + addr + len + hex 를 INFO 로 로그
//   - enabled=false 또는 logger=nil 이면 no-op
//   - addr 이 빈 문자열이면 addr 필드 생략
func TestLogPacket(t *testing.T) {
	newLogger := func(buf *bytes.Buffer) *slog.Logger {
		return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	t.Run("enabled=true 면 hex 로그", func(t *testing.T) {
		var buf bytes.Buffer
		logPacket(newLogger(&buf), true, "tcp-server", "RX", "1.2.3.4:5", []byte{0x32, 0xAB})
		out := buf.String()
		if !strings.Contains(out, "tcp-server: RX") {
			t.Errorf("prefix + dir 이 로그에 있어야 함: %s", out)
		}
		if !strings.Contains(out, "32ab") {
			t.Errorf("payload hex 가 로그에 있어야 함: %s", out)
		}
		if !strings.Contains(out, "1.2.3.4:5") {
			t.Errorf("addr 가 로그에 있어야 함: %s", out)
		}
		if !strings.Contains(out, "\"len\":2") {
			t.Errorf("len 이 로그에 있어야 함: %s", out)
		}
	})

	t.Run("enabled=false 면 no-op", func(t *testing.T) {
		var buf bytes.Buffer
		logPacket(newLogger(&buf), false, "tcp-server", "TX", "1.2.3.4:5", []byte{0x01})
		if buf.Len() != 0 {
			t.Errorf("enabled=false 면 로그가 없어야 함: %s", buf.String())
		}
	})

	t.Run("logger=nil 이면 no-op (패닉 없음)", func(t *testing.T) {
		logPacket(nil, true, "udp-client", "RX", "", []byte{0x01})
	})

	t.Run("addr 가 비면 addr 필드 생략", func(t *testing.T) {
		var buf bytes.Buffer
		logPacket(newLogger(&buf), true, "tcp-client", "TX", "", []byte{0xFF})
		out := buf.String()
		if strings.Contains(out, "\"addr\"") {
			t.Errorf("addr 가 빈 문자열이면 addr 필드가 없어야 함: %s", out)
		}
		if !strings.Contains(out, "ff") {
			t.Errorf("payload hex 는 있어야 함: %s", out)
		}
	})
}
