// registry_resolver_test.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T1)
//
// 본 테스트는 다음을 검증한다:
//   - ClassifyDeviceRef: UUID v4 / agent/name / composite / unknown 분류
//   - SplitAgentName / SplitComposite: 경계 조건
//   - inMemoryRegistry.GetByUID: UUID lookup 의 정확성과 멀티 provider 환경
//   - inMemoryRegistry.GetByAgentName: (agent, name) 쌍 lookup
//   - inMemoryRegistry.ResolveDevice: 통합 dispatcher 동작

package device

import (
	"errors"
	"testing"
	"time"
)

// fakeDeviceForResolver 는 ResolveDevice/GetByUID/GetByAgentName 테스트용 최소 Device.
type fakeDeviceForResolver struct {
	id        string
	uid       string
	name      string
	agentName string
}

func (d *fakeDeviceForResolver) ID() string               { return d.id }
func (d *fakeDeviceForResolver) UID() string              { return d.uid }
func (d *fakeDeviceForResolver) Name() string             { return d.name }
func (d *fakeDeviceForResolver) Type() DeviceType         { return DeviceTypeIndoor }
func (d *fakeDeviceForResolver) Protocol() string         { return "fake" }
func (d *fakeDeviceForResolver) AgentName() string        { return d.agentName }
func (d *fakeDeviceForResolver) Online() bool             { return true }
func (d *fakeDeviceForResolver) LastSeen() time.Time      { return time.Now() }
func (d *fakeDeviceForResolver) State() DeviceState       { return DeviceState{Online: true} }
func (d *fakeDeviceForResolver) Metadata() DeviceMetadata { return DeviceMetadata{} }
func (d *fakeDeviceForResolver) Source() string           { return "auto" }
func (d *fakeDeviceForResolver) Capabilities() []string   { return []string{} }

// fakeProvider 는 DeviceProvider 의 테스트용 구현이다.
type fakeProvider struct {
	devices []Device
}

func (p *fakeProvider) Devices() []Device { return p.devices }
func (p *fakeProvider) Device(id string) (Device, error) {
	for _, d := range p.devices {
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, ErrDeviceNotFound
}

// ---------------------------------------------------------------------------
// ClassifyDeviceRef
// ---------------------------------------------------------------------------

func TestClassifyDeviceRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
		want DeviceRefKind
	}{
		{"empty string", "", DeviceRefUnknown},
		{"valid uuid v4 lowercase", "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", DeviceRefUUID},
		{"valid uuid v4 uppercase", "A58BA668-5741-4B3C-9D2E-7F3C8A1B2C3D", DeviceRefUUID},
		{"agent/name", "lgcnp/indoor-1", DeviceRefAgentName},
		{"agent/name with multiple slashes", "lgcnp/zone1/indoor-1", DeviceRefAgentName},
		{"composite v0.x", "lgcnp:81", DeviceRefComposite},
		{"composite multi-colon", "century:bus0:3b", DeviceRefComposite},
		{"plain string", "indoor-1", DeviceRefUnknown},
		// 정규식은 v1~v5 의 모든 UUID variant 를 허용 (운영상 실 발급기에서
		// v4 만 쓰지만 미래 호환을 위해 permissive). v3 도 매칭됨.
		{"uuid v3 accepted (permissive)", "a58ba668-5741-3b3c-9d2e-7f3c8a1b2c3d", DeviceRefUUID},
		{"uuid wrong variant rejected", "a58ba668-5741-4b3c-0d2e-7f3c8a1b2c3d", DeviceRefUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyDeviceRef(tt.ref); got != tt.want {
				t.Errorf("ClassifyDeviceRef(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestDeviceRefKindString(t *testing.T) {
	t.Parallel()
	cases := map[DeviceRefKind]string{
		DeviceRefUUID:      "uuid",
		DeviceRefAgentName: "agent_name",
		DeviceRefComposite: "composite",
		DeviceRefUnknown:   "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("DeviceRefKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// SplitAgentName / SplitComposite
// ---------------------------------------------------------------------------

func TestSplitAgentName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ref       string
		wantAgent string
		wantName  string
		wantOK    bool
	}{
		{"normal", "lgcnp/indoor-1", "lgcnp", "indoor-1", true},
		{"multi-slash takes first", "lgcnp/zone1/indoor-1", "lgcnp", "zone1/indoor-1", true},
		{"no slash", "lgcnp", "", "", false},
		{"empty agent", "/indoor-1", "", "", false},
		{"empty name", "lgcnp/", "", "", false},
		{"empty input", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAgent, gotName, gotOK := SplitAgentName(tt.ref)
			if gotAgent != tt.wantAgent || gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("SplitAgentName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.ref, gotAgent, gotName, gotOK, tt.wantAgent, tt.wantName, tt.wantOK)
			}
		})
	}
}

func TestSplitComposite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ref         string
		wantAgent   string
		wantLocalID string
		wantOK      bool
	}{
		{"normal", "lgcnp:81", "lgcnp", "81", true},
		{"multi-colon", "century:bus0:3b", "century", "bus0:3b", true},
		{"no colon", "lgcnp", "", "", false},
		{"empty agent", ":81", "", "", false},
		{"empty localID", "lgcnp:", "", "", false},
		{"empty input", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAgent, gotLocal, gotOK := SplitComposite(tt.ref)
			if gotAgent != tt.wantAgent || gotLocal != tt.wantLocalID || gotOK != tt.wantOK {
				t.Errorf("SplitComposite(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.ref, gotAgent, gotLocal, gotOK, tt.wantAgent, tt.wantLocalID, tt.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetByUID / GetByAgentName / ResolveDevice on inMemoryRegistry
// ---------------------------------------------------------------------------

func makeTestRegistry(t *testing.T) (*inMemoryRegistry, []Device) {
	t.Helper()

	dev1 := &fakeDeviceForResolver{
		id:        "lgcnp:81",
		uid:       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		name:      "indoor-1",
		agentName: "lgcnp",
	}
	dev2 := &fakeDeviceForResolver{
		id:        "lgcnp:82",
		uid:       "b66cb779-6852-4c3e-8d2f-7f3c8a1b2c3e",
		name:      "indoor-2",
		agentName: "lgcnp",
	}
	dev3 := &fakeDeviceForResolver{
		id:        "samsung:200001",
		uid:       "c77dc88a-7963-4d3f-9e3f-8f4d9b2c3d4e",
		name:      "living-room",
		agentName: "samsung",
	}

	reg := &inMemoryRegistry{
		providers:     make(map[string]DeviceProvider),
		offlineAgents: make(map[string]bool),
		metadata:      make(map[string]DeviceMetadata),
	}
	reg.providers["lgcnp"] = &fakeProvider{devices: []Device{dev1, dev2}}
	reg.providers["samsung"] = &fakeProvider{devices: []Device{dev3}}

	return reg, []Device{dev1, dev2, dev3}
}

func TestInMemoryRegistry_GetByUID(t *testing.T) {
	t.Parallel()
	reg, devs := makeTestRegistry(t)

	t.Run("found", func(t *testing.T) {
		got, err := reg.GetByUID(devs[0].UID())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID() != devs[0].ID() {
			t.Errorf("got device %q, want %q", got.ID(), devs[0].ID())
		}
	})

	t.Run("found in second provider", func(t *testing.T) {
		got, err := reg.GetByUID(devs[2].UID())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID() != devs[2].ID() {
			t.Errorf("got device %q, want %q", got.ID(), devs[2].ID())
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := reg.GetByUID("00000000-0000-4000-8000-000000000000")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})

	t.Run("empty uid", func(t *testing.T) {
		_, err := reg.GetByUID("")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})
}

func TestInMemoryRegistry_GetByUID_OfflineAgent(t *testing.T) {
	t.Parallel()
	reg, devs := makeTestRegistry(t)

	// lgcnp 에이전트를 offline 으로 마킹.
	reg.offlineAgents["lgcnp"] = true

	got, err := reg.GetByUID(devs[0].UID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// offlineDeviceWrapper 가 적용되어 Online() == false.
	if got.Online() {
		t.Error("expected wrapped device to be offline")
	}
}

func TestInMemoryRegistry_GetByAgentName(t *testing.T) {
	t.Parallel()
	reg, devs := makeTestRegistry(t)

	t.Run("found", func(t *testing.T) {
		got, err := reg.GetByAgentName("lgcnp", "indoor-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UID() != devs[0].UID() {
			t.Errorf("got UID %q, want %q", got.UID(), devs[0].UID())
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		_, err := reg.GetByAgentName("nonexistent", "x")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})

	t.Run("name not found in agent", func(t *testing.T) {
		_, err := reg.GetByAgentName("lgcnp", "nonexistent")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})

	t.Run("empty agent", func(t *testing.T) {
		_, err := reg.GetByAgentName("", "indoor-1")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := reg.GetByAgentName("lgcnp", "")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
	})
}

func TestInMemoryRegistry_ResolveDevice(t *testing.T) {
	t.Parallel()
	reg, devs := makeTestRegistry(t)

	t.Run("uuid", func(t *testing.T) {
		got, kind, err := reg.ResolveDevice(devs[0].UID())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != DeviceRefUUID {
			t.Errorf("got kind %v, want DeviceRefUUID", kind)
		}
		if got.ID() != devs[0].ID() {
			t.Errorf("got device %q, want %q", got.ID(), devs[0].ID())
		}
	})

	t.Run("agent/name", func(t *testing.T) {
		got, kind, err := reg.ResolveDevice("lgcnp/indoor-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != DeviceRefAgentName {
			t.Errorf("got kind %v, want DeviceRefAgentName", kind)
		}
		if got.UID() != devs[0].UID() {
			t.Errorf("got UID %q, want %q", got.UID(), devs[0].UID())
		}
	})

	t.Run("composite", func(t *testing.T) {
		got, kind, err := reg.ResolveDevice("lgcnp:81")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind != DeviceRefComposite {
			t.Errorf("got kind %v, want DeviceRefComposite", kind)
		}
		if got.ID() != devs[0].ID() {
			t.Errorf("got device %q, want %q", got.ID(), devs[0].ID())
		}
	})

	t.Run("unknown format", func(t *testing.T) {
		_, kind, err := reg.ResolveDevice("plain-string")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
		if kind != DeviceRefUnknown {
			t.Errorf("got kind %v, want DeviceRefUnknown", kind)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, kind, err := reg.ResolveDevice("")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
		if kind != DeviceRefUnknown {
			t.Errorf("got kind %v, want DeviceRefUnknown", kind)
		}
	})

	t.Run("agent/name with empty name", func(t *testing.T) {
		_, kind, err := reg.ResolveDevice("lgcnp/")
		if !errors.Is(err, ErrDeviceNotFound) {
			t.Errorf("got %v, want ErrDeviceNotFound", err)
		}
		// "lgcnp/" 은 슬래시를 포함하므로 ClassifyDeviceRef 가 DeviceRefAgentName
		// 으로 분류한다. SplitAgentName 에서 false 가 반환되어 ErrDeviceNotFound.
		if kind != DeviceRefAgentName {
			t.Errorf("got kind %v, want DeviceRefAgentName", kind)
		}
	})
}

// ---------------------------------------------------------------------------
// 인터페이스 준수성: inMemoryRegistry 가 신규 DeviceRegistry 메서드를 모두 노출
// ---------------------------------------------------------------------------

func TestDeviceRegistry_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ DeviceRegistry = (*inMemoryRegistry)(nil)
}
