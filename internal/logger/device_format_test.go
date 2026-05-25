// device_format_test.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T6)
//
// 본 테스트는 디바이스 로그 형식 헬퍼의 동작을 검증한다:
//   - FormatDevice: agent/name 형식 + fallback 우선순위
//   - DeviceUIDAttr: zero-value 처리
//   - DeviceAttrs: 빈 필드 자동 생략

package logger

import (
	"log/slog"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
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
			d:    &fakeDevice{agentName: "lgcnp", name: "indoor-1", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lgcnp:81"},
			want: "lgcnp/indoor-1",
		},
		{
			name: "fallback to short uid when name empty",
			d:    &fakeDevice{agentName: "lgcnp", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lgcnp:81"},
			want: "lgcnp/a58ba668",
		},
		{
			name: "fallback to id when name and uid empty",
			d:    &fakeDevice{agentName: "lgcnp", id: "lgcnp:81"},
			want: "lgcnp/lgcnp:81",
		},
		{
			name: "no agent uses uid",
			d:    &fakeDevice{uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"},
			want: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		},
		{
			name: "no agent no uid uses id",
			d:    &fakeDevice{id: "lgcnp:81"},
			want: "lgcnp:81",
		},
		{
			name: "all empty",
			d:    &fakeDevice{},
			want: "unknown",
		},
		{
			name: "uid shorter than 8 chars not truncated",
			d:    &fakeDevice{agentName: "lgcnp", uid: "abc123"},
			want: "lgcnp/abc123",
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
			agentName: "lgcnp",
			name:      "indoor-1",
			uid:       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		}
		attrs := DeviceAttrs(d)
		got := attrsToMap(attrs)
		if got["device"] != "lgcnp/indoor-1" {
			t.Errorf("device = %q, want %q", got["device"], "lgcnp/indoor-1")
		}
		if got["device_uid"] != d.uid {
			t.Errorf("device_uid = %q, want %q", got["device_uid"], d.uid)
		}
		if got["device_agent"] != "lgcnp" {
			t.Errorf("device_agent = %q, want %q", got["device_agent"], "lgcnp")
		}
		if got["device_name"] != "indoor-1" {
			t.Errorf("device_name = %q, want %q", got["device_name"], "indoor-1")
		}
	})

	t.Run("missing uid omits device_uid", func(t *testing.T) {
		t.Parallel()
		d := &fakeDevice{agentName: "lgcnp", name: "indoor-1"}
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

// TestFormatDevice_CompositeFallbackIncrementsMetric — SPEC-DEVICE-IDENTITY-001
// § B-T9: name 없이 composite id 로 fallback 되는 경로에서 메트릭 증가.
// 본 테스트는 패키지-레벨 메트릭을 변형하므로 t.Parallel 하지 않는다.
func TestFormatDevice_CompositeFallbackIncrementsMetric(t *testing.T) {
	baseline := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]

	// agent + composite id only (name and uid empty) → "agent/composite_id" fallback.
	d := &fakeDevice{agentName: "lgcnp", id: "lgcnp:81"}
	_ = FormatDevice(d)

	// composite-only fallback (no agent) → also tracked.
	d2 := &fakeDevice{id: "lgcnp:82"}
	_ = FormatDevice(d2)

	current := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]
	if delta := current - baseline; delta != 2 {
		t.Errorf("composite-fallback metric delta = %v, want 2", delta)
	}
}

// TestFormatDevice_HappyPathDoesNotIncrementMetric — 정상 agent/name 경로는
// 메트릭 증가 없음 (composite alias 사용이 아님).
func TestFormatDevice_HappyPathDoesNotIncrementMetric(t *testing.T) {
	baseline := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]

	d := &fakeDevice{agentName: "lgcnp", name: "indoor-1", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", id: "lgcnp:81"}
	_ = FormatDevice(d)

	// agent + uid (no name) — uid fallback, composite 아님.
	d2 := &fakeDevice{agentName: "lgcnp", uid: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"}
	_ = FormatDevice(d2)

	current := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]
	if delta := current - baseline; delta != 0 {
		t.Errorf("happy-path metric delta = %v, want 0 (no composite fallback)", delta)
	}
}

// TestFormatDevice_NonCompositeIDDoesNotIncrementMetric — id 에 콜론이 없으면
// composite 가 아니므로 메트릭 증가 없음 (휴리스틱 정확성).
func TestFormatDevice_NonCompositeIDDoesNotIncrementMetric(t *testing.T) {
	baseline := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]

	// id 가 콜론 없는 plain string — composite 휴리스틱에서 제외.
	d := &fakeDevice{agentName: "lgcnp", id: "plain-id-no-colon"}
	_ = FormatDevice(d)

	current := observe.CollectDeviceCompositeUse()[observe.CompositeUseSourceLog]
	if delta := current - baseline; delta != 0 {
		t.Errorf("non-composite id metric delta = %v, want 0", delta)
	}
}
