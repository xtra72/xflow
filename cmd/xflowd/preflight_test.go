// preflight_test.go — `xflowd preflight` 명령의 unit/integration 테스트.
//
// 본 테스트는 SPEC-DEVICE-IDENTITY-001 Phase D § D-T5 의 D-AC7
// (정상 PASS) 와 D-AC8 (FAILED + actionable 메시지) 인수 기준을 검증한다.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPreflight_PASS_FreshEnvironment 는 모든 영속 파일이 부재한 첫 부팅
// 시나리오에서 preflight 가 PASS 함을 검증한다 (D-AC7 의 단순한 케이스).
func TestRunPreflight_PASS_FreshEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := runPreflight(stdout, stderr, &preflightFlags{
		configPath: "",
		dataDir:    tmpDir,
	})

	require.NoError(t, err, "preflight should PASS on fresh environment")
	out := stdout.String()
	assert.Contains(t, out, "Preflight PASSED")
	assert.Contains(t, out, "first boot")
}

// TestRunPreflight_FAIL_CompositeMetadataKeys 는 device_metadata.json 에
// composite key 가 잔존할 때 preflight 가 FAIL + actionable 메시지를 출력함을
// 검증한다 (D-AC8).
func TestRunPreflight_FAIL_CompositeMetadataKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// device_metadata.json 에 composite key 를 가진 메타데이터 작성.
	metaDir := filepath.Join(tmpDir, "device_metadata")
	require.NoError(t, os.MkdirAll(metaDir, 0755))
	metaFile := filepath.Join(metaDir, "device_metadata.json")
	metaPayload := map[string]map[string]any{
		"lg_icp01:81": {"name": "Indoor 1"},
		"lg_icp01:82": {"name": "Indoor 2"},
		"century_icp01:3b":  {"name": "Sensor A"},
	}
	data, err := json.Marshal(metaPayload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaFile, data, 0644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err = runPreflight(stdout, stderr, &preflightFlags{
		configPath: "",
		dataDir:    tmpDir,
	})

	require.Error(t, err, "preflight should FAIL with composite metadata keys")
	assert.EqualError(t, err, "preflight failed")

	out := stdout.String()
	assert.Contains(t, out, "[FAIL]", "FAIL marker should appear in output")
	assert.Contains(t, out, "composite keys", "actionable message should mention composite")
	assert.Contains(t, out, "xflowd migrate device-ids", "actionable message should provide migration command")
}

// TestRunPreflight_PASS_UUIDMetadataKeys 는 device_metadata.json 의 모든 key 가
// UUID 형식일 때 PASS 함을 검증한다 (D-AC7).
func TestRunPreflight_PASS_UUIDMetadataKeys(t *testing.T) {
	tmpDir := t.TempDir()

	// device_metadata.json 에 UUID key 만 작성.
	metaDir := filepath.Join(tmpDir, "device_metadata")
	require.NoError(t, os.MkdirAll(metaDir, 0755))
	metaFile := filepath.Join(metaDir, "device_metadata.json")
	metaPayload := map[string]map[string]any{
		"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d": {"name": "Indoor 1"},
		"b69cc779-6852-4c4d-ae3f-8d4d9b2c3d4e": {"name": "Indoor 2"},
	}
	data, err := json.Marshal(metaPayload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaFile, data, 0644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err = runPreflight(stdout, stderr, &preflightFlags{
		configPath: "",
		dataDir:    tmpDir,
	})

	require.NoError(t, err, "preflight should PASS with UUID-keyed metadata")
	out := stdout.String()
	assert.Contains(t, out, "Preflight PASSED")
	assert.Contains(t, out, "all UUID")
}

// TestRunPreflight_PASS_ExistingDeviceIDs 는 device_ids.json 이 존재하고
// 로드 가능할 때 PASS 함을 검증한다.
func TestRunPreflight_PASS_ExistingDeviceIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// 유효한 device_ids.json 작성.
	idsDir := filepath.Join(tmpDir, "device_ids")
	require.NoError(t, os.MkdirAll(idsDir, 0755))
	idsFile := filepath.Join(idsDir, "device_ids.json")
	idsPayload := map[string]string{
		"lg_icp01:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"century_icp01:3b":  "b69cc779-6852-4c4d-ae3f-8d4d9b2c3d4e",
	}
	data, err := json.Marshal(idsPayload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(idsFile, data, 0644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err = runPreflight(stdout, stderr, &preflightFlags{
		configPath: "",
		dataDir:    tmpDir,
	})

	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "device_ids repository OK")
	assert.Contains(t, out, "2 mappings loaded")
}

// TestRunPreflight_FAIL_CorruptedDeviceIDs 는 device_ids.json 이 손상되어
// 로드 불가능할 때 FAIL 함을 검증한다.
func TestRunPreflight_FAIL_CorruptedDeviceIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// 손상된 device_ids.json 작성 (잘못된 JSON).
	idsDir := filepath.Join(tmpDir, "device_ids")
	require.NoError(t, os.MkdirAll(idsDir, 0755))
	idsFile := filepath.Join(idsDir, "device_ids.json")
	require.NoError(t, os.WriteFile(idsFile, []byte("{not valid json"), 0644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := runPreflight(stdout, stderr, &preflightFlags{
		configPath: "",
		dataDir:    tmpDir,
	})

	require.Error(t, err)
	out := stdout.String()
	assert.Contains(t, out, "[FAIL]", "FAIL marker should appear for corrupted device_ids")
	assert.True(t,
		strings.Contains(out, "device_ids repository load failed") ||
			strings.Contains(out, "device_ids List failed"),
		"actionable message should mention device_ids load failure: got %q", out)
}

// TestNewPreflightCmd_Smoke 는 cobra 명령 객체가 정상 생성되는지 smoke test 한다.
func TestNewPreflightCmd_Smoke(t *testing.T) {
	cmd := newPreflightCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "preflight", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}
