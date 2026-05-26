// storage.go — device_metadata.json 과 device_ids.json 의 직렬화 IO 헬퍼.
//
// 두 파일 모두 JSON object 의 단순 map 형태이므로, 마이그레이션 도구는 storage
// 패키지 (DeviceMetadataFileRepository / DeviceIDFileRepository) 에 의존하지
// 않고 raw JSON 을 직접 다룬다. 이유:
//   - 도구는 운영 데몬과 독립적으로 실행되어야 한다 (운영 중에 도구가 storage
//     싱글톤을 점유할 위험 회피).
//   - 도구는 메타데이터의 의미를 해석할 필요가 없다 (key 만 변환).
//   - storage 패키지의 cache 동기화 메커니즘이 도구의 atomic rename 패턴과
//     충돌할 수 있다.
package deviceids

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// metadataFileName 은 device_metadata.json 의 파일명이다.
//
// storage.NewDeviceMetadataFileRepository 의 명명 규칙과 동일하게 유지해야 한다.
const metadataFileName = "device_metadata.json"

// idRepoFileName 은 device_ids.json 의 파일명이다.
const idRepoFileName = "device_ids.json"

// rawMetadata 는 device_metadata.json 의 raw map 표현이다.
//
// 값은 json.RawMessage 로 보관하여 마이그레이션 도중 메타데이터의 의미적
// 해석을 피한다 (key 만 변환하므로 value 는 byte-perfect 보존).
type rawMetadata map[string]json.RawMessage

// idMapping 은 device_ids.json 의 composite → UUID 매핑 표현이다.
//
// device_ids.json 의 key 는 "<agentName>:<unitID>" 형태이다 (storage 의
// deviceIDKey 와 동일).
type idMapping map[string]string

// loadMetadata 는 device_metadata.json 을 raw map 으로 로드한다.
//
// 파일이 없거나 비어 있으면 빈 map 을 반환한다 (idempotent: 신규 환경에서
// 도구를 실행해도 실패하지 않음).
func loadMetadata(path string) (rawMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return rawMetadata{}, nil
		}
		return nil, fmt.Errorf("metadata 파일 읽기 실패 (%s): %w", path, err)
	}
	if len(data) == 0 {
		return rawMetadata{}, nil
	}
	m := rawMetadata{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("metadata JSON 파싱 실패 (%s): %w", path, err)
	}
	return m, nil
}

// saveMetadataAtomic 은 rawMetadata 를 path 에 atomic 하게 쓴다.
//
// 임시 파일 (<path>.tmp) 에 먼저 쓰고 os.Rename 으로 교체한다. 같은
// 파일시스템에서 rename 은 POSIX 보장 atomic 이다. 실패 시 임시 파일을
// 삭제한다.
func saveMetadataAtomic(path string, m rawMetadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("metadata 디렉토리 생성 실패: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("metadata 직렬화 실패: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("metadata 임시 파일 쓰기 실패: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("metadata 임시 파일 atomic rename 실패: %w", err)
	}
	return nil
}

// loadIDMapping 은 device_ids.json 을 composite → UUID 매핑으로 로드한다.
//
// 파일이 없거나 비어 있으면 빈 mapping 을 반환한다 (도구는 graceful: orphan
// 경고만 출력하고 진행).
func loadIDMapping(path string) (idMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return idMapping{}, nil
		}
		return nil, fmt.Errorf("id-repo 파일 읽기 실패 (%s): %w", path, err)
	}
	if len(data) == 0 {
		return idMapping{}, nil
	}
	m := idMapping{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("id-repo JSON 파싱 실패 (%s): %w", path, err)
	}
	return m, nil
}
