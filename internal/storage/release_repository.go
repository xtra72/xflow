// release_repository.go 는 관리 서버가 호스팅하는 프로그램 이미지(xflowd 바이너리 +
// Ed25519 .sig)의 버전 관리 저장소이다. 노드 측 updater 코드를 변경하지 않고도 동작하도록,
// GitHub-Releases 호환 피드(release_feed.go)가 본 저장소를 출처로 GitHub 형식 JSON 을 만든다.
//
// 설계 원칙:
//   - 메타데이터(버전/채널/asset 목록)는 공유 SQLite DB 에 영속한다(SettingsRepository 등과
//     동일한 authDashboardDB 핸들 재사용). 바이너리 본체와 .sig 는 디스크에 둔다(BLOB 미사용 —
//     대용량 스트리밍/메모리 효율).
//   - 공개키는 노드 로컬 신뢰 앵커이므로 서버가 보관하지 않는다(보안). 서버는 관리자가 미리
//     서명한 .sig 바이트와 바이너리 바이트만 저장·전달한다.
//   - 타임스탬프는 프로젝트 표준인 epoch ms(int64 UnixMilli)를 사용한다. 순수 헬퍼에서
//     time.Now() 를 호출하지 않고 호출자(핸들러)가 시각을 주입한다.
//
// 아키텍처 슬롯: 노드는 runtime.GOOS/GOARCH 를 보고한다. RPi armv6/armv7 은 모두
// GOARCH="arm" 으로 보고하므로 동일 슬롯(linux/arm)을 요청한다. 따라서 arch 차원은 fleet 이
// 사용하는 distinct (os,arch) 쌍이다(linux/amd64, linux/arm64, linux/arm, darwin/amd64,
// darwin/arm64). 업로드 API 는 임의 os/arch 문자열을 허용해 일반성을 유지한다.
package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록

	"github.com/xtra/xflow/internal/updater"
)

// 릴리즈 저장소 관련 에러.
var (
	// ErrReleaseNotFound 는 요청한 버전의 릴리즈가 없을 때 반환된다.
	ErrReleaseNotFound = errors.New("release not found")
	// ErrReleaseAssetNotFound 는 요청한 (version,os,arch) asset 이 없을 때 반환된다.
	ErrReleaseAssetNotFound = errors.New("release asset not found")
	// ErrInvalidReleaseVersion 은 버전 문자열이 semver(v 접두사) 형식이 아닐 때 반환된다.
	ErrInvalidReleaseVersion = errors.New("invalid release version (must be vMAJOR.MINOR.PATCH semver)")
	// ErrInvalidReleaseChannel 은 채널이 {stable,beta,nightly} 가 아닐 때 반환된다.
	ErrInvalidReleaseChannel = errors.New("invalid release channel (must be stable, beta, or nightly)")
	// ErrInvalidAssetFilename 은 파일명이 경로 순회(traversal)를 포함할 때 반환된다.
	ErrInvalidAssetFilename = errors.New("invalid asset filename (path traversal rejected)")
)

// ReleaseAssetRecord 는 단일 (os,arch) 바이너리 asset 의 메타데이터이다.
type ReleaseAssetRecord struct {
	OS           string // GOOS (예: linux, darwin)
	Arch         string // GOARCH (예: amd64, arm64, arm)
	Filename     string // 저장 파일명 (xflowd-{os}-{arch})
	Size         int64  // 바이너리 바이트 크기
	SHA256       string // 바이너리의 hex 인코딩 SHA256
	HasSig       bool   // .sig 동반 여부(관리자 사전 서명)
	UploadedAtMs int64  // 업로드 시각(epoch ms)
}

// ReleaseRecord 는 단일 릴리즈(버전)와 그 asset 목록이다.
type ReleaseRecord struct {
	Version       string // semver(v 접두사) 버전 문자열
	Channel       string // stable | beta | nightly
	Notes         string // 릴리즈 노트(선택)
	PublishedAtMs int64  // 게시 시각(epoch ms)
	Assets        []ReleaseAssetRecord
}

// ReleaseRepository 는 릴리즈 메타데이터(SQLite) + 바이너리/서명 파일(디스크)을 관리한다.
type ReleaseRepository struct {
	db          *sql.DB
	releasesDir string // {releasesDir}/{version}/ 하위에 바이너리·서명 파일 저장
}

// NewReleaseRepository 는 공유 *sql.DB 를 받아 releases/release_assets 테이블을 멱등하게
// 생성하고, releasesDir 을 보장한 뒤 저장소를 반환한다. db 의 수명은 호출자가 관리한다
// (Close 책임은 호출자 — authDashboardDB 공유 핸들 재사용).
func NewReleaseRepository(db *sql.DB, releasesDir string) (*ReleaseRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("release repository: db must not be nil")
	}
	if releasesDir == "" {
		return nil, fmt.Errorf("release repository: releasesDir must not be empty")
	}
	if err := os.MkdirAll(releasesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create releases directory: %w", err)
	}
	if err := migrateReleaseSchema(context.Background(), db); err != nil {
		return nil, err
	}
	return &ReleaseRepository{db: db, releasesDir: releasesDir}, nil
}

// migrateReleaseSchema 는 releases/release_assets 테이블을 멱등하게 생성한다.
// 기존 DB 에 안전하게 추가되며 재실행해도 무해하다(CREATE TABLE IF NOT EXISTS).
func migrateReleaseSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS releases (
		version         TEXT    PRIMARY KEY,
		channel         TEXT    NOT NULL DEFAULT 'stable',
		notes           TEXT,
		published_at_ms INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create releases table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS release_assets (
		version        TEXT    NOT NULL,
		os             TEXT    NOT NULL,
		arch           TEXT    NOT NULL,
		filename       TEXT    NOT NULL,
		size           INTEGER NOT NULL DEFAULT 0,
		sha256         TEXT    NOT NULL DEFAULT '',
		has_sig        INTEGER NOT NULL DEFAULT 0,
		uploaded_at_ms INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(version, os, arch)
	)`); err != nil {
		return fmt.Errorf("create release_assets table: %w", err)
	}
	return nil
}

// validChannel 은 채널 문자열이 허용 enum 인지 확인한다.
func validChannel(channel string) bool {
	switch channel {
	case "stable", "beta", "nightly":
		return true
	default:
		return false
	}
}

// UpsertRelease 는 릴리즈 행을 생성/갱신한다. version 은 semver 여야 하고 channel 은
// {stable,beta,nightly} 여야 한다. publishedAtMs 는 호출자가 부여한다(epoch ms).
func (r *ReleaseRepository) UpsertRelease(ctx context.Context, version, channel, notes string, publishedAtMs int64) error {
	if !updater.Version(version).IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidReleaseVersion, version)
	}
	if channel == "" {
		channel = "stable"
	}
	if !validChannel(channel) {
		return fmt.Errorf("%w: %q", ErrInvalidReleaseChannel, channel)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO releases (version, channel, notes, published_at_ms)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(version) DO UPDATE SET
			channel         = excluded.channel,
			notes           = excluded.notes,
			published_at_ms = excluded.published_at_ms
	`, version, channel, notes, publishedAtMs)
	if err != nil {
		return fmt.Errorf("upsert release: %w", err)
	}
	return nil
}

// ListReleases 는 모든 릴리즈를 semver 내림차순(최신 우선)으로 반환한다. 각 릴리즈는
// 자신의 asset 목록을 포함한다.
func (r *ReleaseRepository) ListReleases(ctx context.Context) ([]ReleaseRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT version, channel, COALESCE(notes, ''), published_at_ms FROM releases`)
	if err != nil {
		return nil, fmt.Errorf("query releases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var recs []ReleaseRecord
	for rows.Next() {
		var rec ReleaseRecord
		if err := rows.Scan(&rec.Version, &rec.Channel, &rec.Notes, &rec.PublishedAtMs); err != nil {
			return nil, fmt.Errorf("scan release: %w", err)
		}
		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate releases: %w", err)
	}

	// asset 목록을 각 릴리즈에 채운다.
	for i := range recs {
		assets, aerr := r.listAssets(ctx, recs[i].Version)
		if aerr != nil {
			return nil, aerr
		}
		recs[i].Assets = assets
	}

	sortReleasesDesc(recs)
	return recs, nil
}

// GetRelease 는 단일 릴리즈를 asset 목록과 함께 반환한다. 없으면 ErrReleaseNotFound.
func (r *ReleaseRepository) GetRelease(ctx context.Context, version string) (ReleaseRecord, error) {
	var rec ReleaseRecord
	err := r.db.QueryRowContext(ctx,
		`SELECT version, channel, COALESCE(notes, ''), published_at_ms FROM releases WHERE version = ?`,
		version).Scan(&rec.Version, &rec.Channel, &rec.Notes, &rec.PublishedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseRecord{}, ErrReleaseNotFound
	}
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("get release: %w", err)
	}
	assets, aerr := r.listAssets(ctx, version)
	if aerr != nil {
		return ReleaseRecord{}, aerr
	}
	rec.Assets = assets
	return rec, nil
}

// listAssets 는 한 버전의 asset 목록을 (os,arch) 오름차순으로 반환한다.
func (r *ReleaseRepository) listAssets(ctx context.Context, version string) ([]ReleaseAssetRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT os, arch, filename, size, sha256, has_sig, uploaded_at_ms
		FROM release_assets WHERE version = ?
		ORDER BY os ASC, arch ASC
	`, version)
	if err != nil {
		return nil, fmt.Errorf("query release assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	assets := make([]ReleaseAssetRecord, 0)
	for rows.Next() {
		var a ReleaseAssetRecord
		var hasSig int
		if err := rows.Scan(&a.OS, &a.Arch, &a.Filename, &a.Size, &a.SHA256, &hasSig, &a.UploadedAtMs); err != nil {
			return nil, fmt.Errorf("scan release asset: %w", err)
		}
		a.HasSig = hasSig != 0
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release assets: %w", err)
	}
	return assets, nil
}

// PutAsset 은 (version,os,arch) 의 바이너리와 서명을 디스크에 atomically(temp+rename, 0600)
// 기록하고, 바이너리의 SHA256 을 계산해 release_assets 행을 upsert(has_sig=true)한다.
// 릴리즈 행이 없으면 자동 생성한다(channel=stable, published_at=uploadedAtMs).
//
// uploadedAtMs 는 호출자가 부여한다(epoch ms). sig 가 nil 이면 .sig 미기록(has_sig=false).
func (r *ReleaseRepository) PutAsset(ctx context.Context, version, goos, arch string, binary io.Reader, sig io.Reader, uploadedAtMs int64) (ReleaseAssetRecord, error) {
	if !updater.Version(version).IsValid() {
		return ReleaseAssetRecord{}, fmt.Errorf("%w: %q", ErrInvalidReleaseVersion, version)
	}
	if goos == "" || arch == "" {
		return ReleaseAssetRecord{}, fmt.Errorf("release asset: os/arch must not be empty")
	}
	if binary == nil {
		return ReleaseAssetRecord{}, fmt.Errorf("release asset: binary reader must not be nil")
	}

	versionDir := filepath.Join(r.releasesDir, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return ReleaseAssetRecord{}, fmt.Errorf("create version directory: %w", err)
	}

	filename := updater.AssetName("xflowd", goos, arch)
	binPath := filepath.Join(versionDir, filename)
	sigPath := binPath + ".sig"

	// 바이너리를 임시 파일에 기록하며 SHA256 을 동시에 계산한다(스트리밍).
	hasher := sha256.New()
	size, err := writeAtomic(binPath, io.TeeReader(binary, hasher))
	if err != nil {
		return ReleaseAssetRecord{}, fmt.Errorf("write binary: %w", err)
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	hasSig := false
	if sig != nil {
		if _, serr := writeAtomic(sigPath, sig); serr != nil {
			// 바이너리는 기록되었으나 서명 기록 실패 — 일관성을 위해 바이너리도 정리한다.
			_ = os.Remove(binPath)
			return ReleaseAssetRecord{}, fmt.Errorf("write signature: %w", serr)
		}
		hasSig = true
	}

	// 릴리즈 행이 없으면 자동 생성한다(채널 stable, published_at=업로드 시각).
	if _, gerr := r.GetRelease(ctx, version); errors.Is(gerr, ErrReleaseNotFound) {
		if uerr := r.UpsertRelease(ctx, version, "stable", "", uploadedAtMs); uerr != nil {
			return ReleaseAssetRecord{}, uerr
		}
	} else if gerr != nil {
		return ReleaseAssetRecord{}, gerr
	}

	hasSigInt := 0
	if hasSig {
		hasSigInt = 1
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO release_assets (version, os, arch, filename, size, sha256, has_sig, uploaded_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(version, os, arch) DO UPDATE SET
			filename       = excluded.filename,
			size           = excluded.size,
			sha256         = excluded.sha256,
			has_sig        = excluded.has_sig,
			uploaded_at_ms = excluded.uploaded_at_ms
	`, version, goos, arch, filename, size, sum, hasSigInt, uploadedAtMs)
	if err != nil {
		return ReleaseAssetRecord{}, fmt.Errorf("upsert release asset: %w", err)
	}

	return ReleaseAssetRecord{
		OS:           goos,
		Arch:         arch,
		Filename:     filename,
		Size:         size,
		SHA256:       sum,
		HasSig:       hasSig,
		UploadedAtMs: uploadedAtMs,
	}, nil
}

// DeleteRelease 는 릴리즈 행 + asset 행들을 제거하고 버전 디렉토리를 삭제한다.
// 미존재 버전은 무해하게 통과한다(멱등).
func (r *ReleaseRepository) DeleteRelease(ctx context.Context, version string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM release_assets WHERE version = ?`, version); err != nil {
		return fmt.Errorf("delete release assets: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM releases WHERE version = ?`, version); err != nil {
		return fmt.Errorf("delete release: %w", err)
	}
	// 디스크 정리 — 버전 디렉토리는 releasesDir 하위로 한정한다(version 은 semver 검증된
	// 입력이 아닐 수 있으므로 경로 순회를 방지한다).
	if dir, ok := r.safeVersionDir(version); ok {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove version directory: %w", err)
		}
	}
	return nil
}

// DeleteAsset 은 (version,os,arch) 행과 디스크의 바이너리·서명 파일을 제거한다.
// 미존재는 무해하게 통과한다(멱등).
func (r *ReleaseRepository) DeleteAsset(ctx context.Context, version, goos, arch string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM release_assets WHERE version = ? AND os = ? AND arch = ?`,
		version, goos, arch); err != nil {
		return fmt.Errorf("delete release asset: %w", err)
	}
	if dir, ok := r.safeVersionDir(version); ok {
		filename := updater.AssetName("xflowd", goos, arch)
		_ = os.Remove(filepath.Join(dir, filename))
		_ = os.Remove(filepath.Join(dir, filename+".sig"))
	}
	return nil
}

// ResolveLatestStable 은 channel='stable' 중 semver 최고 버전을 반환한다. 없으면
// (zero, false, nil).
func (r *ReleaseRepository) ResolveLatestStable(ctx context.Context) (ReleaseRecord, bool, error) {
	all, err := r.ListReleases(ctx)
	if err != nil {
		return ReleaseRecord{}, false, err
	}
	// ListReleases 는 semver 내림차순이므로 첫 stable 이 최신이다.
	for _, rec := range all {
		if rec.Channel == "stable" {
			return rec, true, nil
		}
	}
	return ReleaseRecord{}, false, nil
}

// LatestVersionByArch 는 (os,arch) 슬롯별로 바이너리 asset 을 가진 최고-semver 버전을
// 매핑해 반환한다(아키텍처-aware 그룹 일괄 업데이트의 노드별 타깃 버전 해석에 사용).
//
// 키 형식은 정확히 `os + "/" + arch`(예: "linux/arm64")이다. 값은 해당 슬롯의 asset 을
// 보유한 릴리즈 중 semver 최고 버전이다. channel 이 비어 있지 않으면 그 채널 릴리즈만
// 고려하고, 비어 있으면 전 채널을 고려한다.
//
// 구현: ListReleases(semver 내림차순)를 순회하며, 각 릴리즈의 asset 슬롯이 아직 미설정
// 이면 그 버전을 기록한다(내림차순에서 처음 본 값 = 최신). 따라서 신규 버전이 일부
// 아키텍처를 누락하면 그 슬롯은 자연히 더 낮은(이전) 버전이 차지한다. 저장소가 비면
// 빈 맵을 반환한다.
func (r *ReleaseRepository) LatestVersionByArch(ctx context.Context, channel string) (map[string]string, error) {
	releases, err := r.ListReleases(ctx)
	if err != nil {
		return nil, err
	}
	// ListReleases 는 semver 내림차순이므로, 슬롯별 첫 등장이 곧 최신 버전이다.
	out := make(map[string]string)
	for _, rec := range releases {
		// 채널 필터: 비어 있지 않으면 일치 릴리즈만 고려한다.
		if channel != "" && rec.Channel != channel {
			continue
		}
		for _, a := range rec.Assets {
			key := a.OS + "/" + a.Arch
			if _, seen := out[key]; !seen {
				out[key] = rec.Version
			}
		}
	}
	return out, nil
}

// OpenAsset 은 저장된 파일(바이너리 또는 .sig)을 스트리밍용으로 연다. 호출자가 Close
// 책임을 진다. filename 은 경로 순회("/", "..")를 거부한다(보안). 없으면
// ErrReleaseAssetNotFound.
func (r *ReleaseRepository) OpenAsset(ctx context.Context, version, filename string) (io.ReadCloser, int64, error) {
	_ = ctx
	if !safeFilename(filename) {
		return nil, 0, fmt.Errorf("%w: %q", ErrInvalidAssetFilename, filename)
	}
	dir, ok := r.safeVersionDir(version)
	if !ok {
		return nil, 0, fmt.Errorf("%w: %q", ErrInvalidAssetFilename, version)
	}
	path := filepath.Join(dir, filename)
	f, err := os.Open(path) // #nosec G304 — version/filename 은 safeVersionDir/safeFilename 으로 검증됨
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrReleaseAssetNotFound
		}
		return nil, 0, fmt.Errorf("open asset: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("stat asset: %w", err)
	}
	return f, info.Size(), nil
}

// ChecksumFile 은 저장된 sha256 으로부터 checksum.txt 바이트를 즉석 생성한다. 각 바이너리
// asset 당 한 줄(`{hex}  {filename}` — 2칸 공백, GNU coreutils sha256sum 형식)을 만들며,
// .sig 라인은 포함하지 않는다(노드 Downloader 가 바이너리만 검증). asset 은 filename
// 오름차순으로 정렬해 결정적(deterministic) 출력을 보장한다. 없으면 ErrReleaseNotFound.
func (r *ReleaseRepository) ChecksumFile(ctx context.Context, version string) ([]byte, error) {
	rec, err := r.GetRelease(ctx, version)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(rec.Assets))
	for _, a := range rec.Assets {
		if a.SHA256 == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s  %s", a.SHA256, a.Filename))
	}
	sort.Strings(lines)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// safeVersionDir 은 version 으로부터 releasesDir 하위 디렉토리 경로를 안전하게 구성한다.
// version 이 경로 순회를 포함하면 (zero, false) 를 반환한다.
func (r *ReleaseRepository) safeVersionDir(version string) (string, bool) {
	if !safeFilename(version) {
		return "", false
	}
	return filepath.Join(r.releasesDir, version), true
}

// safeFilename 은 단일 경로 세그먼트가 경로 순회 문자를 포함하지 않는지 확인한다.
// "/", "\\", ".." 또는 빈 문자열을 거부한다.
func safeFilename(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return true
}

// writeAtomic 은 src 를 path 에 atomically(임시 파일 기록 후 rename, 0600) 기록하고
// 기록된 바이트 수를 반환한다. 동일 디렉토리에 임시 파일을 만들어 rename 이 같은 파일
// 시스템 내에서 일어나도록 보장한다.
func writeAtomic(path string, src io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// 실패 경로에서 임시 파일이 남지 않도록 정리한다(성공 시 rename 으로 사라짐).
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("chmod temp file: %w", err)
	}
	n, err := io.Copy(tmp, src)
	if err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("copy to temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, fmt.Errorf("rename temp file: %w", err)
	}
	return n, nil
}

// sortReleasesDesc 는 릴리즈를 semver 내림차순(최신 우선)으로 정렬한다. semver 비교는
// updater.Version.Compare 를 사용한다(비교 불가는 0 으로 처리되어 안정 정렬에 의존).
func sortReleasesDesc(recs []ReleaseRecord) {
	sort.SliceStable(recs, func(i, j int) bool {
		return updater.Version(recs[i].Version).Compare(updater.Version(recs[j].Version)) > 0
	})
}
