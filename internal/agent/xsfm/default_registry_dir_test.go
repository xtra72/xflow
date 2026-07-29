package xsfm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 기본 영속 경로 자동화 (SPEC-XSFM-001)
//
// SetDefaultRegistryDir 로 서버 데이터 디렉터리를 주입하면, registry_path/
// station_registry_path 설정이 비어 있어도 <dataDir>/xsfm/<agentID>/ 아래로
// 영속화가 기본 ON 이 된다. 우선순위: 설정 경로 > 기본 경로 > 인메모리.
//
// setter 는 패키지-레벨 싱글턴이므로 테스트 간 누수를 막기 위해 useDefaultRegistryDir
// 헬퍼가 t.Cleanup 으로 항상 "" 로 복원한다.
// ---------------------------------------------------------------------------

// useDefaultRegistryDir 는 기본 영속 디렉터리를 설정하고 테스트 종료 시 "" 로 복원한다.
func useDefaultRegistryDir(t *testing.T, dir string) {
	t.Helper()
	SetDefaultRegistryDir(dir)
	t.Cleanup(func() { SetDefaultRegistryDir("") })
}

// perAgentDir 는 baseAgentConfig 의 agentID("ap-agent-1") 기준 기본 영속 경로를 만든다.
func perAgentDir(base string) string {
	return filepath.Join(base, "xsfm", "ap-agent-1")
}

// 기본 디렉터리 설정 + 빈 설정 → 역사/위치가 station_registry.json 에 영속되고 복원된다.
func TestDefaultRegistryDir_StationPersistAndRestore(t *testing.T) {
	base := t.TempDir()
	useDefaultRegistryDir(t, base)

	// 빈 설정(station_registry_path 미지정) — 기본 경로로 유도되어야 한다.
	a1, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	// add_station + add_place → 역사/위치 등록.
	require.NoError(t, mustProcessOK(t, ap1, `{"command":"add_station","station":"ST-101","line":"line-2","order":5}`))
	require.NoError(t, mustProcessOK(t, ap1, `{"command":"add_place","station":"ST-101","place":"PL-A","display_name":"승강장 A","order":3}`))

	// 기본 경로 아래에 station_registry.json 이 기록되어야 한다.
	stationFile := filepath.Join(perAgentDir(base), "station_registry.json")
	data, err := os.ReadFile(stationFile)
	require.NoError(t, err, "빈 설정이라도 기본 경로에 station_registry.json 이 기록되어야 한다")
	assert.Contains(t, string(data), "ST-101")
	assert.Contains(t, string(data), "PL-A")

	// 동일 기본 디렉터리 + 동일 이름 + 빈 설정으로 재구성 → 역사/위치 복원.
	a2, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	entry, err := ap2.stations.GetStation("ST-101")
	require.NoError(t, err, "역사 복원")
	assert.Equal(t, "line-2", entry.Line)
	place, err := ap2.stations.GetPlace("ST-101", "PL-A")
	require.NoError(t, err, "위치 복원")
	assert.Equal(t, "승강장 A", place.DisplayName)
	assert.Equal(t, 3, place.Order)
}

// 기본 디렉터리 설정 + 빈 설정 → 디바이스가 device_registry.json 에 영속되고 복원된다.
func TestDefaultRegistryDir_DevicePersistAndRestore(t *testing.T) {
	base := t.TempDir()
	useDefaultRegistryDir(t, base)

	a1, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	require.NoError(t, mustProcessOK(t, ap1, `{"command":"add_device","device_id":"ap-101","group_id":"platform-1"}`))
	require.NoError(t, ap1.Stop(context.Background()))

	// 기본 경로 아래에 device_registry.json 이 기록되어야 한다.
	deviceFile := filepath.Join(perAgentDir(base), "device_registry.json")
	data, err := os.ReadFile(deviceFile)
	require.NoError(t, err, "빈 설정이라도 기본 경로에 device_registry.json 이 기록되어야 한다")
	assert.Contains(t, string(data), "ap-101")

	// 재구성 → 디바이스 복원.
	a2, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	dev, err := ap2.GetDevice("ap-101")
	require.NoError(t, err, "디바이스 복원")
	assert.Equal(t, "platform-1", dev.GroupID)
}

// 설정 경로가 기본 경로보다 우선한다(explicit > default): 파일은 설정 경로에만 기록된다.
func TestDefaultRegistryDir_ConfigPathWins(t *testing.T) {
	base := t.TempDir()     // 기본 경로 베이스
	explicit := t.TempDir() // 명시적 station_registry_path
	useDefaultRegistryDir(t, base)

	opts := portOpts()
	opts["station_registry_path"] = explicit // 설정 경로 명시

	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_station","station":"ST-101","line":"line-2","order":5}`))

	// 파일은 명시적 경로에 있어야 한다.
	_, err = os.ReadFile(filepath.Join(explicit, "station_registry.json"))
	require.NoError(t, err, "설정 경로에 station_registry.json 이 기록되어야 한다")

	// 기본 경로에는 기록되지 않아야 한다(설정이 우선).
	_, err = os.Stat(filepath.Join(perAgentDir(base), "station_registry.json"))
	assert.True(t, os.IsNotExist(err), "설정 경로가 우선하므로 기본 경로에는 파일이 없어야 한다")
}

// 기본 디렉터리 미설정 + 빈 설정 → 인메모리(파일 미기록) — 종전 동작 유지.
func TestDefaultRegistryDir_UnsetStaysInMemory(t *testing.T) {
	// SetDefaultRegistryDir 미호출(기본값 "") — 다른 테스트 누수 방지를 위해 명시적으로 "" 보장.
	SetDefaultRegistryDir("")

	a, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_station","station":"ST-101","line":"line-2"}`))
	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_device","device_id":"ap-101"}`))

	// 영속 저장소가 없어야 한다(인메모리).
	assert.Nil(t, ap.registry, "기본 경로 미설정 + 빈 설정 → 로스터 저장소 미설정")
	entry, err := ap.stations.GetStation("ST-101")
	require.NoError(t, err, "인메모리 CRUD 는 정상 동작")
	assert.Equal(t, "line-2", entry.Line)
}

// mustProcessOK 는 명령을 처리하고 status=ok 를 확인한다.
func mustProcessOK(t *testing.T, ap *XSFMAgent, cmd string) error {
	t.Helper()
	resp, err := ap.Process([]byte(cmd))
	if err != nil {
		return err
	}
	assertStatusOK(t, resp)
	return nil
}
