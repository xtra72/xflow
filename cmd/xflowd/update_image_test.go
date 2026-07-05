// @SPEC:SPEC-UPDATE-001 v0.1.0
// update_image_test.go — `xflowd update keygen` / `sign` 서브커맨드 동작 검증.
//
// 테스트 전략:
//   - cobra SetArgs/SetOut 패턴으로 인터랙션 격리
//   - t.TempDir() 의 더미 파일만 대상 (부수효과 없음)
//   - 생성된 .pub 으로 .sig 를 검증하여 keygen↔sign↔verifier 일관성 확인
package main

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/updater"
)

func TestKeygenWritesKeyAndPub(t *testing.T) {
	dir := t.TempDir()

	cmd := newUpdateKeygenCmd(defaultImageDeps())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--out-dir", dir, "--name", "xflow-release"})
	require.NoError(t, cmd.Execute())

	keyPath := filepath.Join(dir, "xflow-release.key")
	pubPath := filepath.Join(dir, "xflow-release.pub")

	// 개인키 권한 0600 확인.
	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// 공개키 존재 확인.
	_, err = os.Stat(pubPath)
	require.NoError(t, err)

	// 출력에 공개키 hex + 보안 경고 포함 확인.
	assert.Contains(t, out.String(), "공개키 (hex)")
	assert.Contains(t, out.String(), "보안 경고")

	// 생성된 키가 round-trip 가능한지 (private hex → load) 확인.
	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	priv, err := updater.LoadPrivateKeyFromHex(string(keyData))
	require.NoError(t, err)
	assert.NotNil(t, priv)

	// .pub 은 LoadPublicKeyFromFile 로 로드 가능해야 한다.
	pub, err := updater.LoadPublicKeyFromFile(pubPath)
	require.NoError(t, err)
	assert.NotNil(t, pub)
}

func TestKeygenRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()

	run := func() error {
		cmd := newUpdateKeygenCmd(defaultImageDeps())
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"--out-dir", dir, "--name", "rel"})
		return cmd.Execute()
	}

	// 1회차 성공.
	require.NoError(t, run())
	// 2회차는 --force 없이 거부.
	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "이미 존재")
}

func TestKeygenForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	run := func(force bool) error {
		cmd := newUpdateKeygenCmd(defaultImageDeps())
		cmd.SetOut(new(bytes.Buffer))
		args := []string{"--out-dir", dir, "--name", "rel"}
		if force {
			args = append(args, "--force")
		}
		cmd.SetArgs(args)
		return cmd.Execute()
	}

	require.NoError(t, run(false))
	keyPath := filepath.Join(dir, "rel.key")
	first, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	// --force 로 덮어쓰면 새 키가 생성되어 내용이 달라진다.
	require.NoError(t, run(true))
	second, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
}

func TestSignProducesVerifiableSig(t *testing.T) {
	dir := t.TempDir()

	// 1) 키쌍 생성.
	kg := newUpdateKeygenCmd(defaultImageDeps())
	kg.SetOut(new(bytes.Buffer))
	kg.SetArgs([]string{"--out-dir", dir, "--name", "rel"})
	require.NoError(t, kg.Execute())
	keyPath := filepath.Join(dir, "rel.key")
	pubPath := filepath.Join(dir, "rel.pub")

	// 2) 더미 바이너리 생성.
	binPath := filepath.Join(dir, "xflowd-linux-amd64")
	binContent := []byte("fake cross-compiled binary bytes \x00\x01\x02")
	require.NoError(t, os.WriteFile(binPath, binContent, 0o755))

	// 3) 서명.
	sign := newUpdateSignCmd(defaultImageDeps())
	sign.SetOut(new(bytes.Buffer))
	sign.SetArgs([]string{"--key", keyPath, binPath})
	require.NoError(t, sign.Execute())

	// 4) .sig 는 raw 64 bytes 여야 한다.
	sigPath := binPath + ".sig"
	sig, err := os.ReadFile(sigPath)
	require.NoError(t, err)
	assert.Len(t, sig, 64)

	// 5) 생성된 .pub 으로 Verifier 검증 PASS.
	pub, err := updater.LoadPublicKeyFromFile(pubPath)
	require.NoError(t, err)
	v, err := updater.NewVerifier(pub)
	require.NoError(t, err)
	assert.NoError(t, v.VerifySignature(binContent, sig))
}

func TestSignWithOutFlag(t *testing.T) {
	dir := t.TempDir()

	kg := newUpdateKeygenCmd(defaultImageDeps())
	kg.SetOut(new(bytes.Buffer))
	kg.SetArgs([]string{"--out-dir", dir, "--name", "rel"})
	require.NoError(t, kg.Execute())

	binPath := filepath.Join(dir, "bin")
	require.NoError(t, os.WriteFile(binPath, []byte("data"), 0o755))

	outSig := filepath.Join(dir, "custom.sig")
	sign := newUpdateSignCmd(defaultImageDeps())
	sign.SetOut(new(bytes.Buffer))
	sign.SetArgs([]string{"--key", filepath.Join(dir, "rel.key"), "--out", outSig, binPath})
	require.NoError(t, sign.Execute())

	_, err := os.Stat(outSig)
	require.NoError(t, err)
	// 기본 위치에는 생성되지 않아야 한다.
	_, err = os.Stat(binPath + ".sig")
	assert.True(t, os.IsNotExist(err))
}

func TestSignBadKeyPath(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "bin")
	require.NoError(t, os.WriteFile(binPath, []byte("data"), 0o755))

	sign := newUpdateSignCmd(defaultImageDeps())
	sign.SetOut(new(bytes.Buffer))
	// 존재하지 않는 경로 + hex 도 아님 → 에러.
	sign.SetArgs([]string{"--key", "/nonexistent/path/not-hex!!", binPath})
	err := sign.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "개인키 로딩 실패")
}

func TestSignMissingBinary(t *testing.T) {
	dir := t.TempDir()
	kg := newUpdateKeygenCmd(defaultImageDeps())
	kg.SetOut(new(bytes.Buffer))
	kg.SetArgs([]string{"--out-dir", dir, "--name", "rel"})
	require.NoError(t, kg.Execute())

	sign := newUpdateSignCmd(defaultImageDeps())
	sign.SetOut(new(bytes.Buffer))
	sign.SetArgs([]string{"--key", filepath.Join(dir, "rel.key"), filepath.Join(dir, "no-such-binary")})
	err := sign.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "바이너리 읽기 실패")
}

func TestSignWithHexKeyDirectly(t *testing.T) {
	dir := t.TempDir()

	// hex 개인키를 직접 --key 로 전달 (편의 경로).
	priv, _, err := updater.GenerateKeyPair()
	require.NoError(t, err)
	hexKey := updater.PrivateKeyToHex(priv)

	binPath := filepath.Join(dir, "bin")
	binContent := []byte("hex-key signed content")
	require.NoError(t, os.WriteFile(binPath, binContent, 0o755))

	sign := newUpdateSignCmd(defaultImageDeps())
	sign.SetOut(new(bytes.Buffer))
	sign.SetArgs([]string{"--key", hexKey, binPath})
	require.NoError(t, sign.Execute())

	sig, err := os.ReadFile(binPath + ".sig")
	require.NoError(t, err)

	v, err := updater.NewVerifier(priv.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	assert.NoError(t, v.VerifySignature(binContent, sig))
}
