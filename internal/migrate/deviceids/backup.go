// backup.go — 마이그레이션 전 메타데이터 + manifest (sha256) 백업 헬퍼.
//
// 백업 디렉토리 구조 (BackupDir 가 빈 문자열이면 <metadata-dir>/.backup-<ts>):
//
//	<backup-dir>/
//	  device_metadata.json         (원본 byte-perfect 복사)
//	  manifest.json                (sha256 합산 + per-key sha256)
//
// 운영자는 백업 디렉토리에서 device_metadata.json 을 원래 위치로 cp 만 하면
// 즉시 복원된다. manifest.json 은 검증 단계에서 변환 후 메타데이터의
// per-key sha256 가 보존되었는지 비교하는 데 사용된다 (acceptance MIG-AC1).
package deviceids

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// manifest 는 백업과 변환 후 검증을 위한 sha256 합산 + per-key sha256 표현이다.
//
// per-key sha256 는 key 가 변환된 (composite → UUID) 후에도 value (메타데이터
// JSON) 가 byte-perfect 보존되는지 확인하기 위함이다.
type manifest struct {
	// Generated 는 manifest 생성 시각 (RFC3339).
	Generated string `json:"generated"`

	// EntryCount 는 백업 시점의 entry 수.
	EntryCount int `json:"entry_count"`

	// SourceFile 은 백업된 원본 파일의 절대 경로.
	SourceFile string `json:"source_file"`

	// SourceSHA256 은 원본 파일 전체의 sha256 hex.
	SourceSHA256 string `json:"source_sha256"`

	// ValueSHA256Sum 은 모든 value 의 sha256 hex 를 key-sorted 순으로 이어붙인 후
	// 다시 sha256 한 값이다. key 가 변환되어도 value 만 보존되면 동일한 값이 된다.
	ValueSHA256Sum string `json:"value_sha256_sum"`

	// ValueSHA256ByKey 는 진단용으로 key → value sha256 의 매핑이다.
	ValueSHA256ByKey map[string]string `json:"value_sha256_by_key"`
}

// computeManifest 는 raw metadata 와 그 원본 byte stream 으로부터 manifest 를 생성한다.
//
// sourceBytes 는 device_metadata.json 의 원본 byte 이며, sha256 계산 + 백업 복사용.
func computeManifest(sourceFile string, sourceBytes []byte, m rawMetadata) manifest {
	// 전체 파일 sha256.
	sourceHash := sha256.Sum256(sourceBytes)

	// per-key sha256 — key-sorted 순회로 결정론.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	byKey := make(map[string]string, len(m))
	combined := sha256.New()
	for _, k := range keys {
		h := sha256.Sum256(m[k])
		hex := hex.EncodeToString(h[:])
		byKey[k] = hex
		_, _ = combined.Write([]byte(hex))
	}
	valueSum := hex.EncodeToString(combined.Sum(nil))

	return manifest{
		Generated:        time.Now().UTC().Format(time.RFC3339),
		EntryCount:       len(m),
		SourceFile:       sourceFile,
		SourceSHA256:     hex.EncodeToString(sourceHash[:]),
		ValueSHA256Sum:   valueSum,
		ValueSHA256ByKey: byKey,
	}
}

// computeValueHashSum 은 변환 후 메타데이터의 value sha256 합산을 계산한다.
//
// computeManifest 의 ValueSHA256Sum 과 비교하여 검증한다. key 가 변환되었어도
// value 의 sha256 합산은 동일하다 (key-sorted 순회는 변환 전후 동일한
// 메타데이터 집합에 대해 동일한 순서를 보장하지 않으므로 set 기반으로 계산).
func computeValueHashSum(m rawMetadata) string {
	// 변환 후에는 key 순서가 달라지므로, value sha256 의 hex 들을 sorted
	// 순서로 결합한다 (변환 전후 모두 동일 결과 보장).
	hashes := make([]string, 0, len(m))
	for _, v := range m {
		h := sha256.Sum256(v)
		hashes = append(hashes, hex.EncodeToString(h[:]))
	}
	sort.Strings(hashes)
	combined := sha256.New()
	for _, h := range hashes {
		_, _ = combined.Write([]byte(h))
	}
	return hex.EncodeToString(combined.Sum(nil))
}

// computeValueHashSumFromManifest 는 manifest 의 ValueSHA256ByKey 를 sorted hex 들로
// 다시 결합하여 변환 후와 비교 가능한 합산을 만든다.
func computeValueHashSumFromManifest(m manifest) string {
	hashes := make([]string, 0, len(m.ValueSHA256ByKey))
	for _, v := range m.ValueSHA256ByKey {
		hashes = append(hashes, v)
	}
	sort.Strings(hashes)
	combined := sha256.New()
	for _, h := range hashes {
		_, _ = combined.Write([]byte(h))
	}
	return hex.EncodeToString(combined.Sum(nil))
}

// performBackup 은 원본 device_metadata.json 과 manifest 를 backupDir 에 기록한다.
//
// 반환값:
//   - 백업 디렉토리 절대 경로
//   - manifest (검증 단계에서 사용)
//   - 에러
//
// backupDir 가 빈 문자열이면 metadataDir 아래 .backup-<timestamp> 를 사용한다.
// 디렉토리가 이미 존재하면 에러 (사고로 덮어쓰기 방지).
func performBackup(metadataDir, backupDir string, sourceBytes []byte, m rawMetadata) (string, manifest, error) {
	if backupDir == "" {
		ts := time.Now().UTC().Format("20060102T150405Z")
		backupDir = filepath.Join(metadataDir, ".backup-"+ts)
	}

	// 디렉토리 이미 존재 시 사고 방지로 abort.
	if _, err := os.Stat(backupDir); err == nil {
		return "", manifest{}, fmt.Errorf("backup-dir 이미 존재합니다 (덮어쓰기 방지): %s", backupDir)
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", manifest{}, fmt.Errorf("백업 디렉토리 생성 실패: %w", err)
	}

	sourceFile := filepath.Join(metadataDir, metadataFileName)

	// 원본 파일 byte-perfect 복사.
	backupFile := filepath.Join(backupDir, metadataFileName)
	if err := os.WriteFile(backupFile, sourceBytes, 0o644); err != nil {
		return "", manifest{}, fmt.Errorf("백업 파일 쓰기 실패: %w", err)
	}

	// manifest 생성 및 기록.
	man := computeManifest(sourceFile, sourceBytes, m)
	manifestBytes, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", manifest{}, fmt.Errorf("manifest 직렬화 실패: %w", err)
	}
	manifestPath := filepath.Join(backupDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return "", manifest{}, fmt.Errorf("manifest 파일 쓰기 실패: %w", err)
	}

	return backupDir, man, nil
}
