// Package remote 는 SPEC-REMOTE-001 의 원격 관리 서버/클라이언트 기능을 구현한다.
//
// 본 패키지는 xflow 인스턴스(managed node)를 fleet 으로 등록·승인·원격 제어·
// 인벤토리 미러링하기 위한 전송/프로토콜/연결 라이프사이클을 제공한다. 전송은
// 클라이언트가 서버로 dial 하는 영속 WebSocket 이며, 기존
// internal/api/ws.Message 봉투를 재사용한다(REQ-REMOTE-N04).
//
// instance_id.go 는 노드(xflow 설치본)를 식별하는 영속 instance_id 를 다룬다
// (REQ-REMOTE-A03, spec §5.2). IoT 디바이스 UUID(DeviceIDRepository)와는 별개
// 네임스페이스이며 혼동해서는 안 된다(spec §1.4).
package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// instanceIDFileName 은 데이터 디렉토리 내 instance_id 영속 파일명이다.
const instanceIDFileName = "instance_id"

// instanceIDMu 는 동일 디렉토리에 대한 generate-once 영속을 직렬화하여
// 다중 고루틴/프로세스 내 race 로 인한 중복 생성을 방지한다.
//
// NOTE: 이 락은 프로세스 내 race 만 보호한다. 프로세스 간 race 는 파일 존재
// 재확인(double-check)으로 완화하며, 단일 노드당 단일 xflowd 프로세스를
// 가정한다(spec §A2 — 한 인스턴스는 단일 역할).
var instanceIDMu sync.Mutex

// ResolveInstanceID 는 노드의 영속 instance_id 를 결정한다(REQ-REMOTE-A03).
//
// 우선순위:
//  1. configOverride 가 비어 있지 않으면 그 값을 사용한다(config 가 소유; 파일을
//     생성·변경하지 않는다 — spec §5.2 remote_management.instance_id).
//  2. 그 외에는 dataDir/instance_id 파일에서 로드한다.
//  3. 파일이 없으면 새 UUID v4 를 1회 생성하여 atomic 하게 영속하고 반환한다.
//
// 반환값은 재시작/재접속 간 불변이다(파일 영속 보장).
func ResolveInstanceID(configOverride, dataDir string) (string, error) {
	// 1) config override 우선 — 영속하지 않는다(config 가 권위 소유).
	if trimmed := strings.TrimSpace(configOverride); trimmed != "" {
		return trimmed, nil
	}

	instanceIDMu.Lock()
	defer instanceIDMu.Unlock()

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("remote: create instance-id directory: %w", err)
	}

	path := filepath.Join(dataDir, instanceIDFileName)

	// 2) 기존 파일 로드.
	if id, ok, err := readInstanceIDFile(path); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	// 3) 신규 생성 + atomic 영속.
	id := uuid.New().String()
	if err := writeInstanceIDFile(path, id); err != nil {
		return "", err
	}
	return id, nil
}

// readInstanceIDFile 은 영속 파일에서 instance_id 를 읽는다.
// 반환: (id, 존재여부, 에러). 파일 미존재는 (",", false, nil) 로 반환한다.
func readInstanceIDFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("remote: read instance-id file: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		// 빈 파일은 미생성으로 간주(다음 단계에서 재생성).
		return "", false, nil
	}
	return id, true, nil
}

// writeInstanceIDFile 은 instance_id 를 atomic 하게(tmp + rename) 기록한다.
// DeviceIDFileRepository.saveToFile 패턴을 준용한다.
func writeInstanceIDFile(path, id string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(id), 0o600); err != nil {
		return fmt.Errorf("remote: write tmp instance-id file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remote: rename tmp instance-id file: %w", err)
	}
	return nil
}
