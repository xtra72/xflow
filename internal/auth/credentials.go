// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-2, M-3)
// credentials.go — SQLite 기반 자격증명 관리자.
//
// v0.1.x 의 yaml 파일 백엔드는 폐기되었다. 부팅 시 yaml 이 존재하면 1회성으로
// SQLite users 테이블로 마이그레이션 한 뒤 yaml 파일을 users.yaml.migrated 로 rename
// 한다 (UB-007 yaml 잔류 금지).
//
// 외부 API 시그니처 (Load, Save, Authenticate, ChangePassword, EnsureDefaultAdmin,
// GetUser) 는 유지된다. 호출자 (cmd/xflowd/main.go, internal/api/handler/auth.go) 는
// 영향을 받지 않는다.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/storage"
	"gopkg.in/yaml.v3"
)

// 자격증명 관련 에러.
var (
	ErrInvalidCredentials = errors.New("auth: 인증 실패")
	ErrUserNotFound       = errors.New("auth: 사용자를 찾을 수 없습니다")
)

// CredentialUser 는 외부 노출용 사용자 표현이다.
//
// v0.1.x 에서는 yaml 직렬화 태그가 있었으나, v0.2.0 부터 yaml 은 1회성 마이그레이션
// 입력에만 사용된다. 외부 호출자 (handler/auth.go) 는 yaml 태그를 사용하지 않으므로
// 호환성에 영향이 없다.
//
// CreatedAt / UpdatedAt 는 기존 RFC3339 문자열 형식을 유지한다 (외부 호환).
type CredentialUser struct {
	Username     string `yaml:"username"      json:"username"`
	PasswordHash string `yaml:"password_hash" json:"password_hash,omitempty"`
	Role         string `yaml:"role,omitempty" json:"role,omitempty"`
	CreatedAt    string `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt    string `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// credentialsFile 은 yaml 파일의 (마이그레이션 시점만) 구조이다.
type credentialsFile struct {
	Users []CredentialUser `yaml:"users"`
}

// CredentialsManager 는 SQLite 기반 자격증명 관리자이다.
//
// v0.1.x 의 in-memory users map 은 제거되고, 모든 조회는 SQLite users 테이블을
// 직접 참조한다. yamlPath 는 EnsureDefaultAdmin 의 1회성 마이그레이션 시에만 사용
// 된다.
type CredentialsManager struct {
	mu       sync.RWMutex
	db       *sql.DB
	yamlPath string // 빈 문자열이면 마이그레이션 건너뜀
	logger   *slog.Logger
}

// NewCredentialsManager 는 새 CredentialsManager 를 생성한다.
//
// db 는 main.go 가 OpenSQLiteDB 로 열어 소유한다 (Close 책임은 호출자).
// yamlPath 가 비어있지 않고 파일이 존재하면 EnsureDefaultAdmin 시점에 1회만
// 마이그레이션 후 yamlPath + ".migrated" 로 rename 된다.
//
// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-3)
// 외부 메서드 시그니처는 유지되지만 생성자 시그니처는 변경되었다 (yaml 파일 경로 →
// *sql.DB + yamlPath). 호출자 (cmd/xflowd/main.go) 를 동시에 업데이트해야 한다.
func NewCredentialsManager(db *sql.DB, yamlPath string) *CredentialsManager {
	return &CredentialsManager{
		db:       db,
		yamlPath: yamlPath,
		logger:   slog.Default(),
	}
}

// WithLogger 는 로거를 설정한다 (선택적).
func (cm *CredentialsManager) WithLogger(logger *slog.Logger) *CredentialsManager {
	if logger != nil {
		cm.logger = logger
	}
	return cm
}

// Load 는 v0.1.x 호환을 위해 유지되는 no-op 메서드이다.
//
// SQLite 기반에서는 모든 조회가 lazy (필요 시점에 직접 SELECT) 이므로 명시적 Load
// 단계가 불필요하다. 호출 자체는 멱등하게 nil 을 반환하여 기존 호출자 코드를
// 깨뜨리지 않는다.
//
// 외부 API 호환을 위해 시그니처는 유지된다 (return error).
func (cm *CredentialsManager) Load() error {
	// SQLite 백엔드는 lazy read 이므로 no-op.
	return nil
}

// Save 는 v0.1.x 호환을 위해 유지되는 no-op 메서드이다.
//
// SQLite 기반에서는 모든 write 가 즉시 영속화되므로 명시적 Save 단계가 불필요하다.
// 외부 API 호환을 위해 시그니처는 유지된다 (return error).
func (cm *CredentialsManager) Save() error {
	return nil
}

// Authenticate 는 username + password 를 검증하고 사용자 정보를 반환한다.
//
// 보안:
//   - 존재하지 않는 사용자와 비밀번호 오류를 구분하지 않는다 (REQ-N-002).
//   - 타이밍 공격 방지: 미발견 사용자에 대해서도 bcrypt 더미 비교 수행.
func (cm *CredentialsManager) Authenticate(username, password string) (*CredentialUser, error) {
	ctx := context.Background()
	cm.mu.RLock()
	row, err := storage.GetUserByUsername(ctx, cm.db, username)
	cm.mu.RUnlock()

	if err != nil {
		// 타이밍 공격 방지: 존재하지 않더라도 bcrypt 비교 수행
		_ = CheckPassword("$2a$10$invalidhashfortimingatk000000000000000000000000", password)
		return nil, ErrInvalidCredentials
	}

	if err := CheckPassword(row.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return userRowToCredential(row), nil
}

// ChangePassword 는 사용자의 비밀번호를 변경한다.
//
// 현재 비밀번호 검증 후 새 비밀번호로 교체한다 (bcrypt 해시 저장).
// 외부 시그니처 동일.
func (cm *CredentialsManager) ChangePassword(username, currentPassword, newPassword string) error {
	// 1. 현재 비밀번호 검증
	if _, err := cm.Authenticate(username, currentPassword); err != nil {
		return err
	}

	// 2. 새 비밀번호 해싱 (HashPassword 자체에서 길이 검증)
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	// 3. SQLite 갱신
	ctx := context.Background()
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if err := storage.UpdatePasswordHash(ctx, cm.db, username, hash); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("auth: 비밀번호 갱신 실패: %w", err)
	}
	return nil
}

// EnsureDefaultAdmin 는 부팅 시 1회 호출되어 자격증명 저장소를 정상 상태로 만든다.
//
// 동작 (M-3):
//  1. users 테이블이 비어있고 yamlPath 가 존재하는 yaml 파일이면 →
//     yaml 파싱 후 INSERT OR IGNORE 로 이관, 이후 yamlPath 를 ".migrated" 로 rename
//     (UB-007 yaml 잔류 금지). 로그: "auth: migrated N users from yaml to sqlite".
//  2. users 가 여전히 비어있으면 → admin/admin (role=admin) 자동 생성 + 경고 로그.
//  3. 그 외 → 정상 진행, 아무 작업도 하지 않음.
//
// 본 메서드는 멱등하다: 이미 마이그레이션이 완료되었거나 admin 이 존재하면 no-op.
func (cm *CredentialsManager) EnsureDefaultAdmin() error {
	ctx := context.Background()
	cm.mu.Lock()
	defer cm.mu.Unlock()

	count, err := storage.CountUsers(ctx, cm.db)
	if err != nil {
		return fmt.Errorf("auth: 사용자 수 조회 실패: %w", err)
	}

	// 1단계: yaml 마이그레이션 (users 비어있고 yaml 존재 시)
	if count == 0 && cm.yamlPath != "" {
		if migrated, err := cm.migrateFromYAML(ctx); err != nil {
			return err
		} else if migrated > 0 {
			cm.logger.Info("auth: migrated users from yaml to sqlite", "count", migrated)
			// 마이그레이션 후 user count 재조회
			count, err = storage.CountUsers(ctx, cm.db)
			if err != nil {
				return fmt.Errorf("auth: 사용자 수 재조회 실패: %w", err)
			}
		}
	}

	// 2단계: 여전히 비어있으면 admin/admin 자동 생성
	if count == 0 {
		hash, err := HashPassword("admin")
		if err != nil {
			return fmt.Errorf("auth: 기본 admin 비밀번호 해싱 실패: %w", err)
		}
		if err := storage.InsertUser(ctx, cm.db, "admin", hash, "admin", 0, 0); err != nil {
			return fmt.Errorf("auth: 기본 admin 생성 실패: %w", err)
		}
		cm.logger.Warn("기본 admin 계정이 생성되었습니다. 운영 환경에서는 반드시 비밀번호를 변경하세요.")
	}

	return nil
}

// migrateFromYAML 은 yamlPath 가 존재하면 yaml 의 사용자를 SQLite 로 INSERT OR
// IGNORE 후 yamlPath 를 ".migrated" 로 rename 한다.
//
// 반환값: 실제 INSERT 된 사용자 수.
//
// 실패 시:
//   - yaml 미존재          → (0, nil)        : 정상 (마이그레이션 스킵)
//   - yaml 읽기/파싱 실패  → (0, error)      : 호출자가 처리 (부팅 거부)
//   - rename 실패          → (n, error)      : 데이터는 SQLite 에 있으나 yaml 잔류 위험
//     (다음 부팅 재시도 시 INSERT OR IGNORE 가 멱등)
//
// 호출자가 cm.mu 를 보유한 상태로 호출해야 한다.
func (cm *CredentialsManager) migrateFromYAML(ctx context.Context) (int, error) {
	data, err := os.ReadFile(cm.yamlPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil // yaml 미존재 → 마이그레이션 스킵
		}
		return 0, fmt.Errorf("auth: yaml 읽기 실패: %w", err)
	}

	var cf credentialsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return 0, fmt.Errorf("auth: yaml 파싱 실패: %w", err)
	}

	migrated := 0
	for _, u := range cf.Users {
		if u.Username == "" || u.PasswordHash == "" {
			continue // 손상된 항목 스킵
		}
		role := u.Role
		if role == "" {
			role = "viewer"
		}
		// created_at / updated_at 은 yaml RFC3339 → epoch ms 변환. 파싱 실패 시 현재 시각.
		createdAtMs := parseRFC3339ToMillisOrNow(u.CreatedAt)
		updatedAtMs := parseRFC3339ToMillisOrNow(u.UpdatedAt)
		rows, err := storage.InsertUserIgnore(ctx, cm.db, u.Username, u.PasswordHash, role, createdAtMs, updatedAtMs)
		if err != nil {
			return migrated, fmt.Errorf("auth: yaml 사용자 이관 실패 (%s): %w", u.Username, err)
		}
		if rows == 1 {
			migrated++
		}
	}

	// 이관 성공 시 yaml 을 ".migrated" 로 rename — UB-007 yaml 잔류 금지.
	// 실 데이터는 이미 SQLite 에 있으므로 rename 실패해도 인증은 동작하며 다음 부팅에서
	// INSERT OR IGNORE 가 멱등하게 재시도된다.
	migratedPath := cm.yamlPath + ".migrated"
	if err := os.Rename(cm.yamlPath, migratedPath); err != nil {
		return migrated, fmt.Errorf("auth: yaml rename 실패: %w", err)
	}
	// 이관 완료 후 cm.yamlPath 를 초기화하여 본 인스턴스에서 다시 참조되지 않게 한다.
	cm.yamlPath = ""
	return migrated, nil
}

// GetUser 는 username 으로 사용자를 조회한다. 없으면 ErrUserNotFound.
func (cm *CredentialsManager) GetUser(username string) (*CredentialUser, error) {
	ctx := context.Background()
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	row, err := storage.GetUserByUsername(ctx, cm.db, username)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: 사용자 조회 실패: %w", err)
	}
	return userRowToCredential(row), nil
}

// userRowToCredential 은 storage.UserRow 를 외부 노출용 CredentialUser 로 변환한다.
// epoch ms → RFC3339 변환을 통해 외부 표현 일관성을 유지한다.
func userRowToCredential(u *storage.UserRow) *CredentialUser {
	return &CredentialUser{
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		CreatedAt:    millisToRFC3339(u.CreatedAt),
		UpdatedAt:    millisToRFC3339(u.UpdatedAt),
	}
}

// parseRFC3339ToMillisOrNow 는 RFC3339 문자열을 epoch ms 로 변환한다. 실패 시 현재 시각.
func parseRFC3339ToMillisOrNow(s string) int64 {
	if s == "" {
		return time.Now().UnixMilli()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UnixMilli()
	}
	return t.UnixMilli()
}

// millisToRFC3339 는 epoch ms 를 UTC RFC3339 문자열로 변환한다. 0 이면 빈 문자열.
func millisToRFC3339(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
