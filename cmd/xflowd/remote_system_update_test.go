package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/remote"
)

// fakeUpdateRunner 는 systemUpdateApplier 의 테스트 fake 이다.
type fakeUpdateRunner struct {
	result     remote.SystemUpdateResult
	err        error
	gotTarget  string
	gotChannel string
	callCount  int
}

func (f *fakeUpdateRunner) ApplyUpdate(_ context.Context, targetVersion, channel string) (remote.SystemUpdateResult, error) {
	f.callCount++
	f.gotTarget = targetVersion
	f.gotChannel = channel
	return f.result, f.err
}

func newTestSystemCommander(runner systemUpdateApplier, restart func()) *systemCommander {
	return &systemCommander{runner: runner, restart: restart, logger: slog.Default()}
}

func TestSystemCommander_UpdateSuccess_NoRestart(t *testing.T) {
	runner := &fakeUpdateRunner{result: remote.SystemUpdateResult{NewVersion: "v1.3.0", BackupPath: "/x.previous", AppliedAtMs: 1234}}
	restarted := false
	c := newTestSystemCommander(runner, func() { restarted = true })

	args, _ := json.Marshal(remote.SystemUpdateArgs{TargetVersion: "v1.3.0", Channel: "stable"})
	raw, err := c.Do(context.Background(), remote.ActionSystemUpdate, args)
	require.NoError(t, err)

	var res remote.SystemUpdateResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.Equal(t, "v1.3.0", res.NewVersion)
	assert.True(t, res.RestartRequired, "교체됐으나 미재시작 → restart_required")
	assert.False(t, res.Restarting)
	assert.False(t, restarted, "restart=false 면 재시작 안 함")
	assert.Equal(t, "v1.3.0", runner.gotTarget)
	assert.Equal(t, "stable", runner.gotChannel)
}

func TestSystemCommander_UpdateSuccess_WithRestart(t *testing.T) {
	runner := &fakeUpdateRunner{result: remote.SystemUpdateResult{NewVersion: "v1.3.0"}}
	restarted := false
	c := newTestSystemCommander(runner, func() { restarted = true })

	args, _ := json.Marshal(remote.SystemUpdateArgs{TargetVersion: "v1.3.0", Restart: true})
	raw, err := c.Do(context.Background(), remote.ActionSystemUpdate, args)
	require.NoError(t, err)

	var res remote.SystemUpdateResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.True(t, res.Restarting, "restart=true → restarting")
	assert.True(t, restarted, "restart 스케줄러가 호출되어야 한다")
}

func TestSystemCommander_RestartRequestedButUnsupported(t *testing.T) {
	runner := &fakeUpdateRunner{result: remote.SystemUpdateResult{NewVersion: "v1.3.0"}}
	// restart 미주입(nil) — 재시작 미지원.
	c := newTestSystemCommander(runner, nil)

	args, _ := json.Marshal(remote.SystemUpdateArgs{TargetVersion: "v1.3.0", Restart: true})
	raw, err := c.Do(context.Background(), remote.ActionSystemUpdate, args)
	require.NoError(t, err)
	var res remote.SystemUpdateResult
	require.NoError(t, json.Unmarshal(raw, &res))
	assert.True(t, res.RestartRequired)
	assert.False(t, res.Restarting, "미지원이면 restarting=false")
}

func TestSystemCommander_RunnerError(t *testing.T) {
	runner := &fakeUpdateRunner{err: errors.New("download failed")}
	c := newTestSystemCommander(runner, func() {})
	args, _ := json.Marshal(remote.SystemUpdateArgs{TargetVersion: "v1.3.0"})
	_, err := c.Do(context.Background(), remote.ActionSystemUpdate, args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download failed")
}

func TestSystemCommander_UnknownAction(t *testing.T) {
	c := newTestSystemCommander(&fakeUpdateRunner{}, func() {})
	_, err := c.Do(context.Background(), "reboot", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reboot")
}

func TestSystemCommander_EmptyArgsDefaultsToLatest(t *testing.T) {
	runner := &fakeUpdateRunner{result: remote.SystemUpdateResult{NewVersion: "v9.9.9"}}
	c := newTestSystemCommander(runner, func() {})
	_, err := c.Do(context.Background(), remote.ActionSystemUpdate, nil)
	require.NoError(t, err)
	assert.Equal(t, "", runner.gotTarget, "빈 args → target 빈 문자열(채널 최신)")
	assert.Equal(t, 1, runner.callCount)
}

func TestRemoteUpdateRunner_RequiresConfig(t *testing.T) {
	// update_url/public_key_path 미설정 시 명확한 오류.
	r := &remoteUpdateRunner{version: "v1.0.0", binaryPath: "/bin/xflowd"}
	_, err := r.ApplyUpdate(context.Background(), "v1.3.0", "stable")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update_url")
}
