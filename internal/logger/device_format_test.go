// device_format_test.go (SPEC-DEVICE-IDENTITY-001 Phase D — D-T14)
//
// 본 테스트는 디바이스 로그 형식 헬퍼의 동작을 검증한다:
//   - FormatDevice: agent/name 형식 + fallback 우선순위 (Phase D — composite raw 금지)
//   - DeviceUIDAttr: zero-value 처리
//   - DeviceAttrs: 빈 필드 자동 생략

package logger

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// fakeDevice 는 device.Device 의 테스트용 최소 구현이다.
type fakeDevice struct {
	id        string
	uid       string
	name      string
	agentName string
}

func (d *fakeDevice) ID() string                      { return d.id }
func (d *fakeDevice) UID() string                     { return d.uid }
func (d *fakeDevice) Name() string                    { return d.name }
func (d *fakeDevice) Type() device.DeviceType         { return device.DeviceTypeIndoor }
func (d *fakeDevice) Protocol() string                { return "fake" }
func (d *fakeDevice) AgentName() string               { return d.agentName }
func (d *fakeDevice) Online() bool                    { return true }
func (d *fakeDevice) LastSeen() time.Time             { return time.Now() }
func (d *fakeDevice) State() device.DeviceState       { return device.DeviceState{Online: true} }
func (d *fakeDevice) Metadata() device.DeviceMetadata { return device.DeviceMetadata{} }
func (d *fakeDevice) Source() string                  { return "auto" }
func (d *fakeDevice) Capabilities() []string          { return []string{} }

func TestFormatDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    device.Device
		want string
	}{
		{
			name: "nil device",
			d:    nil,
			want: "unknown",
		},
		{
			name: "happy path agent/name",
			d:    &fakeDevice{agentName: "lg_hvacr01", name: "indoor-1", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lg_icp01:81"},
			want: "lg_hvacr01/indoor-1",
		},
		{
			name: "fallback to short uid when name empty",
			d:    &fakeDevice{agentName: "lg_hvacr01", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lg_icp01:81"},
			want: "lg_hvacr01/a58ba668",
		},
		{
			// Phase D § D-T14: composite raw 표시 금지 — agent 접두사 제거된 local_id 만 표시.
			name: "composite id fallback strips agent prefix",
			d:    &fakeDevice{agentName: "lg_hvacr01", id: "lg_icp01:81"},
			want: "lg_hvacr01/81",
		},
		{
			// Phase D: 첫 콜론만 stripping — 다중 콜론 composite (century 등) 의 잔여부.
			name: "multi-colon composite id strips only first prefix",
			d:    &fakeDevice{agentName: "century", id: "century:bus0:3b"},
			want: "century/bus0:3b",
		},
		{
			name: "no agent uses uid",
			d:    &fakeDevice{uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"},
			want: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		},
		{
			// Phase D: agent 부재 + composite id only 시에도 raw composite 노출 금지.
			name: "no agent composite id strips prefix",
			d:    &fakeDevice{id: "lg_icp01:81"},
			want: "81",
		},
		{
			name: "no agent no colon plain id",
			d:    &fakeDevice{id: "plain-id"},
			want: "plain-id",
		},
		{
			name: "all empty",
			d:    &fakeDevice{},
			want: "unknown",
		},
		{
			name: "uid shorter than 8 chars not truncated",
			d:    &fakeDevice{agentName: "lg_hvacr01", uid: "abc123"},
			want: "lg_hvacr01/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatDevice(tt.d); got != tt.want {
				t.Errorf("FormatDevice() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatDevice_NeverEmitsRawComposite — Phase D § D-T14: 어떤 입력에서도
// composite raw 형식 ("agent:local_id") 이 결과에 등장하지 않아야 한다.
func TestFormatDevice_NeverEmitsRawComposite(t *testing.T) {
	t.Parallel()

	cases := []*fakeDevice{
		{agentName: "lg_hvacr01", id: "lg_icp01:81"},
		{agentName: "century", id: "century:bus0:3b"},
		{id: "samsung:0x14"},
		{agentName: "lg_hvacr01", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lg_icp01:81"},
	}

	for _, d := range cases {
		got := FormatDevice(d)
		// composite 형식의 prefix ("agent:") 가 결과에 그대로 등장하면 위반.
		if d.agentName != "" && strings.Contains(got, d.agentName+":") {
			t.Errorf("FormatDevice(%+v) = %q leaked raw composite prefix", d, got)
		}
	}
}

func TestDeviceUIDAttr(t *testing.T) {
	t.Parallel()

	t.Run("uid present", func(t *testing.T) {
		t.Parallel()
		d := &fakeDevice{uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"}
		attr := DeviceUIDAttr(d)
		if attr.Key != "device_uid" {
			t.Errorf("key = %q, want device_uid", attr.Key)
		}
		if attr.Value.String() != d.uid {
			t.Errorf("value = %q, want %q", attr.Value.String(), d.uid)
		}
	})

	t.Run("uid empty returns zero attr", func(t *testing.T) {
		t.Parallel()
		d := &fakeDevice{}
		attr := DeviceUIDAttr(d)
		if attr.Key != "" {
			t.Errorf("expected zero-value attr (key=%q)", attr.Key)
		}
	})

	t.Run("nil device returns zero attr", func(t *testing.T) {
		t.Parallel()
		attr := DeviceUIDAttr(nil)
		if attr.Key != "" {
			t.Errorf("expected zero-value attr for nil device")
		}
	})
}

func TestDeviceAttrs(t *testing.T) {
	t.Parallel()

	t.Run("full device", func(t *testing.T) {
		t.Parallel()
		d := &fakeDevice{
			agentName: "lg_hvacr01",
			name:      "indoor-1",
			uid:       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		}
		attrs := DeviceAttrs(d)
		got := attrsToMap(attrs)
		if got["device"] != "lg_hvacr01/indoor-1" {
			t.Errorf("device = %q, want %q", got["device"], "lg_hvacr01/indoor-1")
		}
		if got["device_uid"] != d.uid {
			t.Errorf("device_uid = %q, want %q", got["device_uid"], d.uid)
		}
		if got["device_agent"] != "lg_hvacr01" {
			t.Errorf("device_agent = %q, want %q", got["device_agent"], "lg_hvacr01")
		}
		if got["device_name"] != "indoor-1" {
			t.Errorf("device_name = %q, want %q", got["device_name"], "indoor-1")
		}
	})

	t.Run("missing uid omits device_uid", func(t *testing.T) {
		t.Parallel()
		d := &fakeDevice{agentName: "lg_hvacr01", name: "indoor-1"}
		attrs := DeviceAttrs(d)
		got := attrsToMap(attrs)
		if _, ok := got["device_uid"]; ok {
			t.Error("device_uid attribute should be omitted when uid is empty")
		}
	})

	t.Run("nil device returns nil slice", func(t *testing.T) {
		t.Parallel()
		if attrs := DeviceAttrs(nil); attrs != nil {
			t.Errorf("expected nil slice for nil device, got %v", attrs)
		}
	})
}

// attrsToMap 는 테스트 보조 — slog.Attr 슬라이스를 map 으로 변환.
func attrsToMap(attrs []slog.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Value.String()
	}
	return m
}
