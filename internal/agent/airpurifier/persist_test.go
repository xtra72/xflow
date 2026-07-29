package airpurifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// B7 — 로스터 영속화 라운드트립 (acceptance.md Module 2: Scenario 2.4 / 2.5 / 2.6)
//
// 영속화 테스트는 port 모드(브로커 배선 없음)를 사용하고 registry_path 를 t.TempDir() 로 둔다.
// 복원은 Init(=NewAirPurifierAgent) 에서 일어나므로 Start/브로커가 필요 없다.
// ---------------------------------------------------------------------------

// registryFilePath 는 registry_path 디렉터리 안의 로스터 저장 파일 경로를 반환한다.
func registryFilePath(dir string) string { return filepath.Join(dir, "device_registry.json") }

// Scenario 2.4: 로스터 영속화 라운드트립 (bridge 디바이스 복원 + config 디바이스 미저장).
func TestPersist_RosterRoundtrip(t *testing.T) {
	dir := t.TempDir()
	opts := portOpts()
	opts["registry_path"] = dir
	opts["devices"] = []any{map[string]any{"device_id": "ap-cfg"}} // config 시드(Source="config")

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","group_id":"platform-1"}`))
	require.NoError(t, err)
	require.NoError(t, ap.Stop(context.Background()))

	// 영속화 파일은 bridge 디바이스(ap-101)만 담고 config 디바이스(ap-cfg)는 담지 않는다.
	data, err := os.ReadFile(registryFilePath(dir))
	require.NoError(t, err)
	assert.Contains(t, string(data), "ap-101", "bridge 디바이스는 영속화된다")
	assert.NotContains(t, string(data), "ap-cfg", "config 디바이스는 영속화 파일에 저장되지 않는다")

	// 동일 설정으로 재시작 → ap-101 이 group_id 와 함께 복원된다.
	a2, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	dev, err := ap2.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "platform-1", dev.GroupID, "group_id 복원")
	assert.Equal(t, "bridge", dev.Source, "Source 보존(재시작 후 삭제 가능성 유지)")
	assert.False(t, dev.Online, "복원 디바이스는 Online=false")
}

// Scenario 2.5: 위치 계층 속성(station/place/index) 등록 및 라운드트립.
func TestPersist_LocationRoundtrip(t *testing.T) {
	dir := t.TempDir()
	opts := portOpts()
	opts["registry_path"] = dir

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","station":"ST-101","place":"승강장","index":3}`))
	require.NoError(t, err)

	// 등록 즉시 로스터에 반영된다.
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "ST-101", dev.Station)
	assert.Equal(t, "승강장", dev.Place)
	assert.Equal(t, 3, dev.Index)
	require.NoError(t, ap.Stop(context.Background()))

	// 재시작 후 station/place/index 가 그대로 복원된다.
	a2, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	dev2, err := ap2.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "ST-101", dev2.Station, "station 복원")
	assert.Equal(t, "승강장", dev2.Place, "place 복원")
	assert.Equal(t, 3, dev2.Index, "index 복원")
}

// Scenario 2.6: 위치 속성 미지정 하위호환 (v0.2.0 포맷 로드 → zero 값 복원, 오류 없이 Running).
func TestPersist_LegacyBackwardCompat(t *testing.T) {
	dir := t.TempDir()

	// station/place/index 키가 없던 v0.2.0 포맷 파일을 직접 기록한다.
	legacy := `{"ap-101":{"device_id":"ap-101","name":"레거시","group_id":"g1","source":"bridge"}}`
	require.NoError(t, os.WriteFile(registryFilePath(dir), []byte(legacy), 0644))

	opts := portOpts()
	opts["registry_path"] = dir

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err, "레거시 포맷 로드는 오류 없이 성공")
	ap := asAP(t, a)
	assert.Equal(t, lifecycle.StateRunning, ap.CurrentState(), "복원 후 Running 전이")

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "", dev.Station, "위치 속성 부재 → zero 값")
	assert.Equal(t, "", dev.Place)
	assert.Equal(t, 0, dev.Index)
	assert.Equal(t, "g1", dev.GroupID, "기존 필드는 복원")
	assert.Equal(t, "레거시", dev.Name)
	assert.Equal(t, "bridge", dev.Source)
}

// config 디바이스와 device_id 충돌 시 설정이 우선한다(config precedence, REQ-02-05).
func TestPersist_ConfigPrecedenceOnConflict(t *testing.T) {
	dir := t.TempDir()

	// 저장 파일에 ap-101 을 bridge, group_id "persisted-grp" 로 남긴다.
	persisted := `{"ap-101":{"device_id":"ap-101","name":"저장본","group_id":"persisted-grp","source":"bridge"}}`
	require.NoError(t, os.WriteFile(registryFilePath(dir), []byte(persisted), 0644))

	opts := portOpts()
	opts["registry_path"] = dir
	// 동일 device_id 를 config 로 선언(group_id "config-grp").
	opts["devices"] = []any{map[string]any{"device_id": "ap-101", "group_id": "config-grp"}}

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "config-grp", dev.GroupID, "설정 디바이스가 우선(덮어쓰지 않음)")
	assert.Equal(t, "config", dev.Source, "설정 Source 유지")
}

// registry_path 미설정 시 영속화가 비활성(no-op)이며 CRUD 는 정상 동작한다.
func TestPersist_NoRegistryPathDisabled(t *testing.T) {
	ap := asAP(t, mustNewPortAgent(t, portOpts()))
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101"}`))
	require.NoError(t, err, "registry_path 없이도 add_device 성공")
	assert.Nil(t, ap.registry, "저장소 미설정")
}

// GetPersistableDevices 는 bridge/auto 디바이스만 DeviceEntry 로 반환한다(config 제외).
func TestGetPersistableDevices_BridgeOnly(t *testing.T) {
	opts := portOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-cfg", "name": "설정디바이스"}}
	ap := asAP(t, mustNewPortAgent(t, opts))

	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","name":"표시명"}`))
	require.NoError(t, err)

	entries := ap.GetPersistableDevices()
	require.Len(t, entries, 1, "bridge 디바이스만 반환(config 제외)")
	e := entries[0]
	assert.Equal(t, "ap-101", e.Address, "Address = device_id")
	assert.Equal(t, "ap-101", e.Name, "Name = device_id(안정 식별자)")
	assert.Equal(t, "표시명", e.DisplayName, "DisplayName = 사용자 표시 이름")
	assert.Equal(t, "bridge", e.Source)
}

// mustNewPortAgent 는 port 모드 에이전트를 생성한다(에러 시 실패).
func mustNewPortAgent(t *testing.T, opts map[string]any) *AirPurifierAgent {
	t.Helper()
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	return asAP(t, a)
}
