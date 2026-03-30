package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// 자격증명 관련 에러
var (
	ErrInvalidCredentials = errors.New("auth: 인증 실패")
	ErrUserNotFound       = errors.New("auth: 사용자를 찾을 수 없습니다")
)

// CredentialUser 는 자격증명 파일의 사용자 항목이다.
type CredentialUser struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
	Role         string `yaml:"role,omitempty"`
	CreatedAt    string `yaml:"created_at,omitempty"`
	UpdatedAt    string `yaml:"updated_at,omitempty"`
}

// CredentialsFile 는 users.yaml 파일의 구조이다.
type CredentialsFile struct {
	Users []CredentialUser `yaml:"users"`
}

// CredentialsManager 는 자격증명 파일을 관리한다.
type CredentialsManager struct {
	mu       sync.RWMutex
	filePath string
	users    map[string]*CredentialUser // username -> user
}

// NewCredentialsManager 는 새 CredentialsManager를 생성한다.
func NewCredentialsManager(filePath string) *CredentialsManager {
	return &CredentialsManager{
		filePath: filePath,
		users:    make(map[string]*CredentialUser),
	}
}

// Load 는 자격증명 파일을 읽어 메모리에 로드한다.
func (cm *CredentialsManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return fmt.Errorf("auth: 자격증명 파일 읽기 실패: %w", err)
	}

	var cf CredentialsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("auth: 자격증명 파일 파싱 실패: %w", err)
	}

	cm.users = make(map[string]*CredentialUser, len(cf.Users))
	for i := range cf.Users {
		cm.users[cf.Users[i].Username] = &cf.Users[i]
	}

	return nil
}

// Save 는 현재 메모리 상태를 자격증명 파일에 저장한다.
// 파일 권한은 0600 (소유자만 읽기/쓰기)으로 설정된다.
func (cm *CredentialsManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cf := CredentialsFile{
		Users: make([]CredentialUser, 0, len(cm.users)),
	}
	for _, u := range cm.users {
		cf.Users = append(cf.Users, *u)
	}

	data, err := yaml.Marshal(&cf)
	if err != nil {
		return fmt.Errorf("auth: 자격증명 직렬화 실패: %w", err)
	}

	if err := os.WriteFile(cm.filePath, data, 0600); err != nil {
		return fmt.Errorf("auth: 자격증명 파일 쓰기 실패: %w", err)
	}

	return nil
}

// Authenticate 는 사용자명과 비밀번호를 검증한다.
// 보안을 위해 "사용자 미발견"과 "비밀번호 오류"를 구분하지 않는다 (REQ-N-002).
func (cm *CredentialsManager) Authenticate(username, password string) (*CredentialUser, error) {
	cm.mu.RLock()
	user, exists := cm.users[username]
	cm.mu.RUnlock()

	if !exists {
		// 타이밍 공격 방지: 존재하지 않는 사용자라도 bcrypt 비교 수행
		_ = CheckPassword("$2a$10$invalidhashfortimingatk000000000000000000000000", password)
		return nil, ErrInvalidCredentials
	}

	if err := CheckPassword(user.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// ChangePassword 는 사용자의 비밀번호를 변경한다.
// 현재 비밀번호 검증 후 새 비밀번호로 교체한다.
func (cm *CredentialsManager) ChangePassword(username, currentPassword, newPassword string) error {
	// 현재 비밀번호 검증
	if _, err := cm.Authenticate(username, currentPassword); err != nil {
		return err
	}

	// 새 비밀번호 해싱
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	user, exists := cm.users[username]
	if !exists {
		cm.mu.Unlock()
		return ErrInvalidCredentials
	}
	user.PasswordHash = hash
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cm.mu.Unlock()

	// 파일에 저장
	return cm.Save()
}

// EnsureDefaultAdmin 는 자격증명 파일이 없을 때 기본 admin 사용자를 생성한다.
// 파일이 이미 존재하면 Load()만 수행한다.
func (cm *CredentialsManager) EnsureDefaultAdmin() error {
	// 파일 존재 여부 확인
	if _, err := os.Stat(cm.filePath); err == nil {
		// 파일이 존재하면 로드만 수행
		return cm.Load()
	}

	// 기본 admin 비밀번호 해싱
	hash, err := HashPassword("admin")
	if err != nil {
		return fmt.Errorf("auth: 기본 admin 비밀번호 해싱 실패: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cm.mu.Lock()
	cm.users["admin"] = &CredentialUser{
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	cm.mu.Unlock()

	slog.Warn("기본 admin 계정이 생성되었습니다. 운영 환경에서는 반드시 비밀번호를 변경하세요.",
		"credentials_file", cm.filePath,
	)

	return cm.Save()
}

// GetUser 는 사용자명으로 사용자를 조회한다.
func (cm *CredentialsManager) GetUser(username string) (*CredentialUser, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[username]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}
