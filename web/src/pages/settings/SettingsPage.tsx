// 설정 페이지.
// 프로필, 시스템, 테마, 언어 탭으로 구성된 설정 화면을 제공한다.
// viewer 역할은 시스템 설정 섹션이 비활성화된다.

import { useEffect, useState, type FormEvent } from 'react';
import {
  Eye,
  EyeOff,
  Globe,
  Lock,
  Monitor,
  Moon,
  Palette,
  Shield,
  Sun,
  User,
} from 'lucide-react';

import { useAuthStore } from '@/stores/authStore';
import { useUIStore } from '@/stores/uiStore';
import { useTheme } from '@/hooks/useTheme';
import { setLogLevel, getLogLevels, resetComponentLogLevel } from '@/services/api/monitorService';
import type { LogLevelInfo } from '@/services/api/monitorService';
import { cn } from '@/lib/utils/cn';

// --- 탭 정의 ---

type TabId = 'profile' | 'system' | 'theme' | 'language';

interface TabItem {
  /** 탭 식별자 */
  id: TabId;
  /** 탭 라벨 */
  label: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
}

const TABS: TabItem[] = [
  { id: 'profile', label: '프로필', icon: User },
  { id: 'system', label: '시스템', icon: Shield },
  { id: 'theme', label: '테마', icon: Palette },
  { id: 'language', label: '언어', icon: Globe },
];

// --- 테마 옵션 정의 ---

interface ThemeOption {
  value: 'light' | 'dark' | 'system';
  label: string;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
}

const THEME_OPTIONS: ThemeOption[] = [
  { value: 'light', label: '라이트', description: '밝은 테마를 사용합니다', icon: Sun },
  { value: 'dark', label: '다크', description: '어두운 테마를 사용합니다', icon: Moon },
  { value: 'system', label: '시스템', description: '시스템 설정을 따릅니다', icon: Monitor },
];

// --- 로그 레벨 옵션 ---

const LOG_LEVELS = ['debug', 'info', 'warn', 'error'] as const;

// --- 언어 옵션 ---

interface LanguageOption {
  code: string;
  label: string;
}

const LANGUAGES: LanguageOption[] = [
  { code: 'ko', label: '한국어' },
  { code: 'en', label: 'English' },
];

// --- 공통 스타일 ---

/** 입력 필드 기본 스타일 */
const inputClass = cn(
  'block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm',
  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
  'dark:border-gray-600 dark:bg-gray-700 dark:text-white',
  'disabled:cursor-not-allowed disabled:opacity-50',
);

/**
 * 시스템 설정 페이지.
 * 프로필, 시스템, 테마, 언어 4개 탭으로 구성되며
 * RBAC 기반으로 시스템 설정 접근을 제어한다.
 */
export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<TabId>('profile');

  return (
    <div className="space-y-6">
      {/* 페이지 헤더 */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">설정</h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          애플리케이션 환경을 구성합니다
        </p>
      </div>

      <div className="flex flex-col gap-6 md:flex-row">
        {/* 탭 네비게이션 - 모바일: 수평, 데스크톱: 수직 */}
        <nav
          className="flex gap-1 overflow-x-auto md:w-48 md:shrink-0 md:flex-col"
          aria-label="설정 탭"
        >
          {TABS.map((tab) => {
            const Icon = tab.icon;
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  'flex items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  'hover:bg-gray-100 dark:hover:bg-gray-700',
                  // 모바일: 수평 탭 스타일
                  isActive
                    ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400 md:border-l-2 md:border-blue-600'
                    : 'text-gray-700 dark:text-gray-300',
                )}
                aria-current={isActive ? 'page' : undefined}
              >
                <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span>{tab.label}</span>
              </button>
            );
          })}
        </nav>

        {/* 탭 콘텐츠 영역 */}
        <div className="min-w-0 flex-1">
          {activeTab === 'profile' && <ProfileTab />}
          {activeTab === 'system' && <SystemTab />}
          {activeTab === 'theme' && <ThemeTab />}
          {activeTab === 'language' && <LanguageTab />}
        </div>
      </div>
    </div>
  );
}

// ============================================================
// 프로필 탭
// ============================================================

/** 프로필 탭: 사용자 정보 표시 및 비밀번호 변경 폼 */
function ProfileTab() {
  const user = useAuthStore((s) => s.user);
  const addNotification = useUIStore((s) => s.addNotification);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showPasswords, setShowPasswords] = useState(false);

  /** 역할 라벨 매핑 */
  const roleLabels: Record<string, string> = {
    admin: '관리자',
    editor: '편집자',
    viewer: '뷰어',
  };

  /** 비밀번호 변경 폼 제출 */
  function handlePasswordSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();

    if (!currentPassword || !newPassword || !confirmPassword) {
      addNotification({ type: 'error', message: '모든 비밀번호 필드를 입력해 주세요' });
      return;
    }

    if (newPassword.length < 8) {
      addNotification({ type: 'error', message: '새 비밀번호는 8자 이상이어야 합니다' });
      return;
    }

    if (newPassword !== confirmPassword) {
      addNotification({ type: 'error', message: '새 비밀번호가 일치하지 않습니다' });
      return;
    }

    // 백엔드 API 미구현 - 클라이언트 측 시뮬레이션
    addNotification({ type: 'success', message: '프로필이 저장되었습니다' });
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
  }

  return (
    <div className="space-y-6">
      {/* 사용자 정보 카드 */}
      <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">사용자 정보</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          현재 로그인된 계정 정보입니다
        </p>

        <dl className="mt-4 space-y-3">
          <div className="flex items-center gap-3">
            <dt className="w-20 shrink-0 text-sm font-medium text-gray-500 dark:text-gray-400">
              이름
            </dt>
            <dd className="text-sm text-gray-900 dark:text-white">{user?.name ?? '-'}</dd>
          </div>
          <div className="flex items-center gap-3">
            <dt className="w-20 shrink-0 text-sm font-medium text-gray-500 dark:text-gray-400">
              이메일
            </dt>
            <dd className="text-sm text-gray-900 dark:text-white">{user?.email ?? '-'}</dd>
          </div>
          <div className="flex items-center gap-3">
            <dt className="w-20 shrink-0 text-sm font-medium text-gray-500 dark:text-gray-400">
              역할
            </dt>
            <dd>
              <span
                className={cn(
                  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                  user?.role === 'admin'
                    ? 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400'
                    : user?.role === 'editor'
                      ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
                )}
              >
                {user?.role ? (roleLabels[user.role] ?? user.role) : '-'}
              </span>
            </dd>
          </div>
        </dl>
      </div>

      {/* 비밀번호 변경 카드 */}
      <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
        <div className="flex items-center gap-2">
          <Lock className="h-5 w-5 text-gray-400" aria-hidden="true" />
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">비밀번호 변경</h3>
        </div>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          계정 보안을 위해 주기적으로 비밀번호를 변경하세요
        </p>

        <form onSubmit={handlePasswordSubmit} className="mt-4 max-w-md space-y-4">
          {/* 현재 비밀번호 */}
          <div>
            <label
              htmlFor="current-password"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              현재 비밀번호
            </label>
            <div className="relative">
              <input
                id="current-password"
                type={showPasswords ? 'text' : 'password'}
                autoComplete="current-password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                className={cn(inputClass, 'pr-10')}
                placeholder="현재 비밀번호 입력"
              />
              <button
                type="button"
                onClick={() => setShowPasswords((prev) => !prev)}
                className="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                aria-label={showPasswords ? '비밀번호 숨기기' : '비밀번호 표시'}
              >
                {showPasswords ? (
                  <EyeOff className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <Eye className="h-4 w-4" aria-hidden="true" />
                )}
              </button>
            </div>
          </div>

          {/* 새 비밀번호 */}
          <div>
            <label
              htmlFor="new-password"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              새 비밀번호
            </label>
            <input
              id="new-password"
              type={showPasswords ? 'text' : 'password'}
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className={inputClass}
              placeholder="새 비밀번호 입력 (8자 이상)"
            />
          </div>

          {/* 비밀번호 확인 */}
          <div>
            <label
              htmlFor="confirm-password"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              비밀번호 확인
            </label>
            <input
              id="confirm-password"
              type={showPasswords ? 'text' : 'password'}
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className={inputClass}
              placeholder="새 비밀번호 다시 입력"
            />
          </div>

          <button
            type="submit"
            className={cn(
              'rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white',
              'hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2',
              'dark:focus:ring-offset-gray-800 transition-colors',
            )}
          >
            비밀번호 변경
          </button>
        </form>
      </div>
    </div>
  );
}

// ============================================================
// 시스템 탭
// ============================================================

/** 시스템 탭: 로그 레벨 변경, API 서버 정보 표시. viewer 역할 비활성화. */
function SystemTab() {
  const user = useAuthStore((s) => s.user);
  const addNotification = useUIStore((s) => s.addNotification);
  const [logLevel, setLogLevelState] = useState<string>('info');
  const [isUpdating, setIsUpdating] = useState(false);

  const isViewer = user?.role === 'viewer';
  const apiUrl = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

  /** 로그 레벨 변경 핸들러 */
  async function handleLogLevelChange(level: string) {
    setIsUpdating(true);
    try {
      await setLogLevel(level);
      setLogLevelState(level);
      addNotification({ type: 'success', message: `로그 레벨이 "${level}"로 변경되었습니다` });
    } catch {
      addNotification({ type: 'error', message: '로그 레벨 변경에 실패했습니다' });
    } finally {
      setIsUpdating(false);
    }
  }

  return (
    <div className="space-y-6">
      {/* RBAC 경고 배너 */}
      {isViewer && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded-md bg-yellow-50 p-4 text-sm text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400"
        >
          <Shield className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span>관리자 권한이 필요합니다</span>
        </div>
      )}

      {/* 로그 레벨 설정 카드 */}
      <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">로그 레벨</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          서버 런타임 로그 레벨을 변경합니다
        </p>

        <div className="mt-4 max-w-xs">
          <label
            htmlFor="log-level"
            className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            레벨 선택
          </label>
          <select
            id="log-level"
            value={logLevel}
            onChange={(e) => handleLogLevelChange(e.target.value)}
            disabled={isViewer || isUpdating}
            className={cn(
              inputClass,
              'appearance-none bg-[url("data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2020%2020%22%20fill%3D%22%236b7280%22%3E%3Cpath%20fill-rule%3D%22evenodd%22%20d%3D%22M5.22%208.22a.75.75%200%20011.06%200L10%2011.94l3.72-3.72a.75.75%200%20111.06%201.06l-4.25%204.25a.75.75%200%2001-1.06%200L5.22%209.28a.75.75%200%20010-1.06z%22%20clip-rule%3D%22evenodd%22%2F%3E%3C%2Fsvg%3E")] bg-[length:1.25rem] bg-[right_0.5rem_center] bg-no-repeat pr-8',
            )}
          >
            {LOG_LEVELS.map((level) => (
              <option key={level} value={level}>
                {level.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 컴포넌트별 로그 레벨 오버라이드 카드 */}
      <ComponentLogLevelOverrides isViewer={isViewer} addNotification={addNotification} />

      {/* API 서버 정보 카드 */}
      <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">API 서버</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          현재 연결된 백엔드 API 서버 정보입니다
        </p>

        <div className="mt-4 max-w-md">
          <label
            htmlFor="api-url"
            className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            서버 URL
          </label>
          <input
            id="api-url"
            type="text"
            readOnly
            value={apiUrl}
            className={cn(inputClass, 'cursor-default bg-gray-50 dark:bg-gray-900')}
          />
        </div>
      </div>
    </div>
  );
}

// ---- 컴포넌트별 로그 레벨 오버라이드 ----

/** 컴포넌트별 로그 레벨 오버라이드 목록 표시 및 리셋 기능 */
function ComponentLogLevelOverrides({
  isViewer,
  addNotification,
}: {
  isViewer: boolean;
  addNotification: (n: { type: 'success' | 'error' | 'info' | 'warning'; message: string }) => void;
}) {
  const [logLevelInfo, setLogLevelInfo] = useState<LogLevelInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [resettingComponent, setResettingComponent] = useState<string | null>(null);

  /** 로그 레벨 정보 로드 */
  function loadLogLevels() {
    setIsLoading(true);
    getLogLevels()
      .then((info) => {
        setLogLevelInfo(info);
      })
      .catch(() => {
        setLogLevelInfo(null);
      })
      .finally(() => {
        setIsLoading(false);
      });
  }

  useEffect(() => {
    loadLogLevels();
  }, []);

  /** 컴포넌트 로그 레벨 리셋 핸들러 */
  async function handleReset(component: string) {
    setResettingComponent(component);
    try {
      await resetComponentLogLevel(component);
      addNotification({ type: 'success', message: `"${component}" 로그 레벨이 리셋되었습니다` });
      loadLogLevels();
    } catch {
      addNotification({ type: 'error', message: '로그 레벨 리셋에 실패했습니다' });
    } finally {
      setResettingComponent(null);
    }
  }

  const overrides = logLevelInfo?.components
    ? Object.entries(logLevelInfo.components)
    : [];

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
        컴포넌트별 로그 레벨
      </h3>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
        개별 컴포넌트에 설정된 로그 레벨 오버라이드 목록입니다
      </p>

      <div className="mt-4">
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <div
                key={i}
                className="h-10 animate-pulse rounded bg-gray-200 dark:bg-gray-700"
              />
            ))}
          </div>
        ) : overrides.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">
            설정된 오버라이드가 없습니다
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="pb-2 pr-4 font-medium text-gray-500 dark:text-gray-400">
                    컴포넌트
                  </th>
                  <th className="pb-2 pr-4 font-medium text-gray-500 dark:text-gray-400">
                    레벨
                  </th>
                  <th className="pb-2 font-medium text-gray-500 dark:text-gray-400">
                    액션
                  </th>
                </tr>
              </thead>
              <tbody>
                {overrides.map(([component, level]) => (
                  <tr
                    key={component}
                    className="border-b border-gray-100 last:border-0 dark:border-gray-700/50"
                  >
                    <td className="py-2 pr-4 text-gray-900 dark:text-white">
                      {component}
                    </td>
                    <td className="py-2 pr-4 font-mono text-gray-900 dark:text-white">
                      {level.toUpperCase()}
                    </td>
                    <td className="py-2">
                      <button
                        type="button"
                        onClick={() => handleReset(component)}
                        disabled={isViewer || resettingComponent === component}
                        className={cn(
                          'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                          'border border-gray-300 text-gray-700 hover:bg-gray-50',
                          'dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700',
                          'disabled:cursor-not-allowed disabled:opacity-50',
                        )}
                      >
                        {resettingComponent === component ? '리셋 중...' : '리셋'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// 테마 탭
// ============================================================

/** 테마 탭: 라이트/다크/시스템 테마 선택. 변경 시 즉시 적용. */
function ThemeTab() {
  const { theme, setTheme } = useTheme();

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-white">테마 설정</h3>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
        애플리케이션 외관을 설정합니다. 변경 사항은 즉시 적용됩니다.
      </p>

      <div className="mt-6 grid gap-3 sm:grid-cols-3">
        {THEME_OPTIONS.map((option) => {
          const Icon = option.icon;
          const isSelected = theme === option.value;
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => setTheme(option.value)}
              className={cn(
                'flex flex-col items-center gap-3 rounded-lg border-2 p-4 transition-colors',
                isSelected
                  ? 'border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-900/20'
                  : 'border-gray-200 bg-white hover:border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:hover:border-gray-500',
              )}
              aria-pressed={isSelected}
            >
              <Icon
                className={cn(
                  'h-8 w-8',
                  isSelected
                    ? 'text-blue-600 dark:text-blue-400'
                    : 'text-gray-400 dark:text-gray-500',
                )}
                aria-hidden="true"
              />
              <div className="text-center">
                <div
                  className={cn(
                    'text-sm font-medium',
                    isSelected
                      ? 'text-blue-600 dark:text-blue-400'
                      : 'text-gray-900 dark:text-white',
                  )}
                >
                  {option.label}
                </div>
                <div className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {option.description}
                </div>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ============================================================
// 언어 탭
// ============================================================

/** localStorage 키: 사용자 언어 설정 */
const LANGUAGE_STORAGE_KEY = 'xflow-language';

/** 언어 탭: 한국어/영어 선택. localStorage에 저장. */
function LanguageTab() {
  const addNotification = useUIStore((s) => s.addNotification);
  const [language, setLanguage] = useState<string>(
    () => localStorage.getItem(LANGUAGE_STORAGE_KEY) ?? 'ko',
  );

  /** 언어 변경 핸들러 */
  function handleLanguageChange(code: string) {
    setLanguage(code);
    localStorage.setItem(LANGUAGE_STORAGE_KEY, code);
    const selected = LANGUAGES.find((l) => l.code === code);
    addNotification({
      type: 'success',
      message: `언어가 "${selected?.label ?? code}"(으)로 변경되었습니다`,
    });
  }

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-white">언어 설정</h3>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
        인터페이스 표시 언어를 선택합니다
      </p>

      <div className="mt-6 space-y-2 max-w-sm">
        {LANGUAGES.map((lang) => {
          const isSelected = language === lang.code;
          return (
            <label
              key={lang.code}
              className={cn(
                'flex cursor-pointer items-center gap-3 rounded-lg border-2 px-4 py-3 transition-colors',
                isSelected
                  ? 'border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-900/20'
                  : 'border-gray-200 hover:border-gray-300 dark:border-gray-600 dark:hover:border-gray-500',
              )}
            >
              <input
                type="radio"
                name="language"
                value={lang.code}
                checked={isSelected}
                onChange={() => handleLanguageChange(lang.code)}
                className="h-4 w-4 text-blue-600 focus:ring-blue-500 dark:focus:ring-offset-gray-800"
              />
              <span
                className={cn(
                  'text-sm font-medium',
                  isSelected
                    ? 'text-blue-600 dark:text-blue-400'
                    : 'text-gray-900 dark:text-white',
                )}
              >
                {lang.label}
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}
