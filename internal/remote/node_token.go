// node_token.go 는 승인된 노드 토큰의 로컬 영속(파일)을 다룬다
// (@SPEC:SPEC-REMOTE-001 M2, REQ-C04/C05, F06).
//
// 토큰은 시크릿이므로 0600 권한으로 atomic(tmp + rename) 저장하고, 로그/커밋 대상이
// 아니다. instance_id 파일과 동일한 dataDir 에 nodeTokenFileName 으로 저장한다.
package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// nodeTokenFileName 은 데이터 디렉토리 내 노드 토큰 영속 파일명이다.
const nodeTokenFileName = "node_token"

// LoadNodeToken 은 영속된 노드 토큰을 로드한다(REQ-C05).
// 반환: (token, 존재여부, 에러). 파일 미존재는 ("", false, nil).
func LoadNodeToken(dataDir string) (string, bool, error) {
	path := filepath.Join(dataDir, nodeTokenFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("remote: read node token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

// SaveNodeToken 은 노드 토큰을 0600 권한으로 atomic 하게 영속한다(REQ-C04/F06).
func SaveNodeToken(dataDir, token string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("remote: create node token directory: %w", err)
	}
	path := filepath.Join(dataDir, nodeTokenFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token), 0o600); err != nil {
		return fmt.Errorf("remote: write tmp node token: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remote: rename tmp node token: %w", err)
	}
	return nil
}

// ClearNodeToken 은 영속된 노드 토큰을 삭제한다(폐기/거부 시 정리에 사용).
func ClearNodeToken(dataDir string) error {
	path := filepath.Join(dataDir, nodeTokenFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remote: remove node token: %w", err)
	}
	return nil
}
