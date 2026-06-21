package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunVerify_MalformedConfig(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("::: not valid yaml :::\n  - ["), 0o600))
	err := runVerify(bad)
	require.Error(t, err, "잘못된 설정은 verify 실패(비0)")
}

// writeScript 는 주어진 exit code 로 종료하는 실행 가능한 셸 스크립트를 만든다(스모크 테스트 후보 대역).
func writeScript(t *testing.T, exitCode int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "candidate")
	body := "#!/bin/sh\nexit " + strconv.Itoa(exitCode) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestSmokeTest_CandidatePasses(t *testing.T) {
	r := &remoteUpdateRunner{}
	candidate := writeScript(t, 0)
	require.NoError(t, r.smokeTest(context.Background(), candidate))
	// chmod 0755 적용 확인.
	info, err := os.Stat(candidate)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "실행 권한이 설정되어야 함")
}

func TestSmokeTest_CandidateFails(t *testing.T) {
	r := &remoteUpdateRunner{}
	candidate := writeScript(t, 1)
	err := r.smokeTest(context.Background(), candidate)
	require.Error(t, err, "후보 verify 비0 종료 시 스모크 실패")
	assert.Contains(t, err.Error(), "verify")
}
