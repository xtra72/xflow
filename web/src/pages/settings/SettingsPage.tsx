// 설정 페이지.
// 프로필, 시스템, 테마, 언어 탭으로 구성된 설정 화면을 제공한다.
// viewer 역할은 시스템 설정 섹션이 비활성화된다.

import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  ChevronDown,
  ChevronsUpDown,
  ChevronUp,
  Eye,
  EyeOff,
  Globe,
  Lock,
  Monitor,
  Moon,
  Palette,
  Search,
  Server,
  Shield,
  Sun,
  User,
} from 'lucide-react';

import { useAuthStore } from '@/stores/authStore';
import { useUIStore, type ThemeMode } from '@/stores/uiStore';
import { usePermission } from '@/hooks/usePermission';
import { useTheme } from '@/hooks/useTheme';
import {
  setLogLevel,
  getLogLevels,
  setComponentLogLevel,
  resetComponentLogLevel,
  getLogStyle,
  setLogStyle,
} from '@/services/api/monitorService';
import type { LogLevelInfo, LogStyle } from '@/services/api/monitorService';
import { SystemInfoCard } from '@/components/system/SystemInfoCard';
import { SystemRuntimeCard } from '@/components/system/SystemRuntimeCard';
import RemoteManagementTab from '@/pages/settings/RemoteManagementTab';
import { ScheduleLogStorageCard } from '@/pages/settings/ScheduleLogStorageCard';
import { ThemePaletteEditor } from '@/components/theme/ThemePaletteEditor';
import { cn } from '@/lib/utils/cn';
import { useTranslation, type Locale, type TranslationFn } from '@/lib/i18n';

// --- 탭 정의 ---

type TabId = 'profile' | 'system' | 'remote' | 'theme' | 'language';

interface TabItem {
  /** 탭 식별자 */
  id: TabId;
  /** 탭 라벨 i18n 키 (렌더 시 t()로 변환) */
  labelKey: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
}

const TABS: TabItem[] = [
  { id: 'profile', labelKey: 'settings.profile', icon: User },
  { id: 'system', labelKey: 'settings.system', icon: Shield },
  { id: 'remote', labelKey: 'settings.remote.tab', icon: Server },
  { id: 'theme', labelKey: 'settings.theme', icon: Palette },
  { id: 'language', labelKey: 'settings.language', icon: Globe },
];

// --- 테마 옵션 정의 ---

interface ThemeOption {
  value: ThemeMode;
  /** 라벨 i18n 키 (렌더 시 t()로 변환) */
  labelKey: string;
  /** 설명 i18n 키 (렌더 시 t()로 변환) */
  descKey: string;
  icon: React.ComponentType<{ className?: string }>;
}

const THEME_OPTIONS: ThemeOption[] = [
  { value: 'day', labelKey: 'settings.themeLight', descKey: 'settings.themeDayDesc', icon: Sun },
  { value: 'night', labelKey: 'settings.themeDark', descKey: 'settings.themeNightDesc', icon: Moon },
  { value: 'system', labelKey: 'settings.themeSystem', descKey: 'settings.themeSystemDesc', icon: Monitor },
];

// --- 로그 레벨 옵션 ---

const LOG_LEVELS = ['debug', 'info', 'warn', 'error'] as const;

// --- 로그 출력 식별자 표시 방식 옵션 ---

/**
 * 로그 표시 방식 옵션 메타데이터.
 *
 * `labelKey`/`hintKey` 는 i18n 키이며 렌더 시 t()로 변환한다
 * (컴포넌트 밖에서 t() 를 호출하지 않기 위해 키만 보관).
 */
const LOG_STYLES: { value: LogStyle; labelKey: string }[] = [
  { value: 'name', labelKey: 'settings.logStyleName' },
  { value: 'id', labelKey: 'settings.logStyleId' },
  { value: 'both', labelKey: 'settings.logStyleBoth' },
];

// --- 컴포넌트 종류(category) 매핑 ---

/**
 * 컴포넌트 종류 메타데이터.
 *
 * 컴포넌트명은 점(.) 구분 계층이며 첫 세그먼트가 종류이다
 * (예: `agent.mqtt-client` → `agent`, `flow.test-flow.node.modbus-reader` → `flow`).
 * `dotClass`는 종류를 가볍게 구분하기 위한 배지 점 색상으로, 하드코딩 색상 없이
 * 기존 디자인 토큰(`--color-*`)만 사용한다. Tailwind v4 JIT가 클래스를 인식하도록
 * 동적 조합이 아닌 정적 문자열로 선언한다.
 */
// `labelKey`는 i18n 키(`settings.category.*`)이며 렌더 시 t()로 변환한다.
// 컴포넌트 밖에서 t()를 호출하지 않기 위해 키만 보관한다.
const CATEGORY_META: Record<string, { labelKey: string; dotClass: string }> = {
  agent: { labelKey: 'settings.categoryLabel.agent', dotClass: 'bg-(--color-interactive-primary)' },
  flow: { labelKey: 'settings.categoryLabel.flow', dotClass: 'bg-(--color-status-running)' },
  node: { labelKey: 'settings.categoryLabel.node', dotClass: 'bg-(--color-status-info)' },
  remote: { labelKey: 'settings.categoryLabel.remote', dotClass: 'bg-(--color-status-warning)' },
  engine: { labelKey: 'settings.categoryLabel.engine', dotClass: 'bg-(--color-status-error)' },
  api: { labelKey: 'settings.categoryLabel.api', dotClass: 'bg-(--color-status-stopped)' },
  router: { labelKey: 'settings.categoryLabel.router', dotClass: 'bg-(--color-interactive-active)' },
  db: { labelKey: 'settings.categoryLabel.db', dotClass: 'bg-(--color-status-running)' },
  auth: { labelKey: 'settings.categoryLabel.auth', dotClass: 'bg-(--color-status-info)' },
  storage: { labelKey: 'settings.categoryLabel.storage', dotClass: 'bg-(--color-status-warning)' },
};

/** 매핑되지 않은 종류의 기본 배지 점 색상(중립 회색 토큰). */
const FALLBACK_DOT_CLASS = 'bg-(--color-status-stopped)';

/** 컴포넌트명에서 종류 키(첫 세그먼트)를 추출한다. */
function getCategoryKey(component: string): string {
  return component.split('.')[0] ?? component;
}

/**
 * 종류 키를 현지화된 종류 라벨로 변환한다. 매핑에 없으면 키를 그대로 표기한다.
 * t()를 인자로 받아 컴포넌트 밖 호출을 피한다.
 */
function getCategoryLabel(key: string, t: TranslationFn): string {
  const meta = CATEGORY_META[key];
  return meta ? t(meta.labelKey) : key;
}

/** 종류 키에 해당하는 배지 점 색상 클래스를 반환한다. */
function getCategoryDotClass(key: string): string {
  return CATEGORY_META[key]?.dotClass ?? FALLBACK_DOT_CLASS;
}

// --- 언어 옵션 ---

interface LanguageOption {
  code: Locale;
  /** 라벨 i18n 키 (렌더 시 t()로 변환) */
  labelKey: string;
}

const LANGUAGES: LanguageOption[] = [
  { code: 'ko', labelKey: 'settings.langKo' },
  { code: 'en', labelKey: 'settings.langEn' },
];

// --- 공통 스타일 ---

/** 입력 필드 기본 스타일 */
const inputClass = cn(
  'block w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) shadow-sm',
  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
  'disabled:cursor-not-allowed disabled:opacity-50',
);

/**
 * 시스템 설정 페이지.
 * 프로필, 시스템, 테마, 언어 4개 탭으로 구성되며
 * RBAC 기반으로 시스템 설정 접근을 제어한다.
 */
export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState<TabId>('profile');
  const { t } = useTranslation();

  return (
    <div className="space-y-6">
      {/* 페이지 헤더 */}
      <div>
        <h2 className="text-2xl font-bold text-(--color-text-primary)">{t('settings.title')}</h2>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('settings.pageSubtitle')}
        </p>
      </div>

      <div className="flex flex-col gap-6 md:flex-row">
        {/* 탭 네비게이션 - 모바일: 수평, 데스크톱: 수직 */}
        <nav
          className="flex gap-1 overflow-x-auto md:w-48 md:shrink-0 md:flex-col"
          aria-label={t('settings.tabsLabel')}
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
                  'hover:bg-(--color-bg-elevated)',
                  // 모바일: 수평 탭 스타일
                  isActive
                    ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400 md:border-l-2 md:border-blue-600'
                    : 'text-(--color-text-secondary)',
                )}
                aria-current={isActive ? 'page' : undefined}
              >
                <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span>{t(tab.labelKey)}</span>
              </button>
            );
          })}
        </nav>

        {/* 탭 콘텐츠 영역 */}
        <div className="min-w-0 flex-1">
          {activeTab === 'profile' && <ProfileTab />}
          {activeTab === 'system' && <SystemTab />}
          {activeTab === 'remote' && <RemoteManagementTab />}
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
  const { t } = useTranslation();

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showPasswords, setShowPasswords] = useState(false);

  /** 역할 라벨 i18n 키 매핑 (공통 키 재사용) */
  const roleLabelKeys: Record<string, string> = {
    admin: 'common.admin',
    editor: 'common.editor',
    viewer: 'common.viewer',
  };

  /** 비밀번호 변경 폼 제출 */
  function handlePasswordSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();

    if (!currentPassword || !newPassword || !confirmPassword) {
      addNotification({ type: 'error', message: t('settings.allPasswordFieldsRequired') });
      return;
    }

    if (newPassword.length < 8) {
      addNotification({ type: 'error', message: t('settings.newPasswordTooShort') });
      return;
    }

    if (newPassword !== confirmPassword) {
      addNotification({ type: 'error', message: t('settings.newPasswordMismatch') });
      return;
    }

    // 백엔드 API 미구현 - 클라이언트 측 시뮬레이션
    addNotification({ type: 'success', message: t('settings.profileSaved') });
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
  }

  return (
    <div className="space-y-6">
      {/* 사용자 정보 카드 */}
      <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
        <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.userInfo')}</h3>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('settings.userInfoDesc')}
        </p>

        <dl className="mt-4 space-y-3">
          <div className="flex items-center gap-3">
            <dt className="w-20 shrink-0 text-sm font-medium text-(--color-text-muted)">
              {t('settings.name')}
            </dt>
            <dd className="text-sm text-(--color-text-primary)">{user?.name ?? '-'}</dd>
          </div>
          {/* Basic Auth에서는 이메일 필드 없음 */}
          <div className="flex items-center gap-3">
            <dt className="w-20 shrink-0 text-sm font-medium text-(--color-text-muted)">
              {t('settings.role')}
            </dt>
            <dd>
              <span
                className={cn(
                  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                  user?.role === 'admin'
                    ? 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400'
                    : user?.role === 'editor'
                      ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'bg-(--color-bg-sunken) text-(--color-text-primary)',
                )}
              >
                {(() => {
                  if (!user?.role) return '-';
                  const roleKey = roleLabelKeys[user.role];
                  return roleKey ? t(roleKey) : user.role;
                })()}
              </span>
            </dd>
          </div>
        </dl>
      </div>

      {/* 비밀번호 변경 카드 */}
      <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
        <div className="flex items-center gap-2">
          <Lock className="h-5 w-5 text-(--color-text-muted)" aria-hidden="true" />
          <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.changePassword')}</h3>
        </div>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('settings.changePasswordDesc')}
        </p>

        <form onSubmit={handlePasswordSubmit} className="mt-4 max-w-md space-y-4">
          {/* 현재 비밀번호 */}
          <div>
            <label
              htmlFor="current-password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('settings.currentPassword')}
            </label>
            <div className="relative">
              <input
                id="current-password"
                type={showPasswords ? 'text' : 'password'}
                autoComplete="current-password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                className={cn(inputClass, 'pr-10')}
                placeholder={t('settings.currentPasswordPlaceholder')}
              />
              <button
                type="button"
                onClick={() => setShowPasswords((prev) => !prev)}
                className="absolute inset-y-0 right-0 flex items-center pr-3 text-(--color-text-muted) hover:text-(--color-text-secondary)"
                aria-label={showPasswords ? t('auth.hidePassword') : t('auth.showPassword')}
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
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('settings.newPassword')}
            </label>
            <input
              id="new-password"
              type={showPasswords ? 'text' : 'password'}
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className={inputClass}
              placeholder={t('settings.newPasswordPlaceholder')}
            />
          </div>

          {/* 비밀번호 확인 */}
          <div>
            <label
              htmlFor="confirm-password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('settings.confirmPassword')}
            </label>
            <input
              id="confirm-password"
              type={showPasswords ? 'text' : 'password'}
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className={inputClass}
              placeholder={t('settings.confirmPasswordPlaceholder')}
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
            {t('settings.changePassword')}
          </button>
        </form>
      </div>
    </div>
  );
}

// ============================================================
// 시스템 탭
// ============================================================

/** 시스템 탭: 로그 레벨 변경, API 서버 정보 표시. 쓰기 권한 없으면 비활성화. */
function SystemTab() {
  const { hasPermission } = usePermission();
  const addNotification = useUIStore((s) => s.addNotification);
  const { t } = useTranslation();
  const [logLevel, setLogLevelState] = useState<string>('info');
  const [logStyle, setLogStyleState] = useState<LogStyle>('both');
  const [isUpdating, setIsUpdating] = useState(false);
  const [isStyleUpdating, setIsStyleUpdating] = useState(false);

  // 마운트 시 현재 로그 표시 방식을 로드해 셀렉트를 초기화한다.
  // 컴포넌트별 로그 레벨 카드(loadLogLevels)와 동일한 then/catch 패턴을 따른다.
  useEffect(() => {
    getLogStyle()
      .then((info) => {
        setLogStyleState(info.style);
      })
      .catch(() => {
        // 조회 실패 시 기본값(both)을 유지한다.
      });
  }, []);

  // SPEC-AUTH-006 AC-10: 역할 이름 비교 → 권한 키 판정.
  //   이 플래그는 로그 레벨·로그 출력 방식 등 시스템 설정 변경 컨트롤을
  //   비활성화하는 데만 쓰이므로 system.update 미보유로 판정한다.
  //   변수명을 isViewer 로 두면 커스텀 역할에서 뜻이 어긋나므로 함께 바꾼다.
  const isReadOnly = !hasPermission('system.update');
  // API client(client.ts)는 상대경로 `/api/v1`를 사용하므로 백엔드는 브라우저가
  // 접속한 origin과 동일하다. window.location.origin을 그대로 표시하면 HTTP/HTTPS
  // 프로토콜도 현재 접속 상태를 정확히 반영한다(별도 변환 불필요).
  const apiUrl = window.location.origin;

  /** 로그 레벨 변경 핸들러 */
  async function handleLogLevelChange(level: string) {
    setIsUpdating(true);
    try {
      await setLogLevel(level);
      setLogLevelState(level);
      addNotification({
        type: 'success',
        message: t('settings.logLevelChangedNamed').replace('{level}', level),
      });
    } catch {
      addNotification({ type: 'error', message: t('settings.logLevelError') });
    } finally {
      setIsUpdating(false);
    }
  }

  /** 로그 출력 식별자 표시 방식 변경 핸들러 */
  async function handleLogStyleChange(style: LogStyle) {
    setIsStyleUpdating(true);
    try {
      await setLogStyle(style);
      setLogStyleState(style);
      addNotification({ type: 'success', message: t('settings.logStyleChanged') });
    } catch {
      addNotification({ type: 'error', message: t('settings.logStyleError') });
    } finally {
      setIsStyleUpdating(false);
    }
  }

  return (
    <div className="space-y-6">
      {/* 권한 부족 경고 배너 */}
      {isReadOnly && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded-md bg-yellow-50 p-4 text-sm text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400"
        >
          <Shield className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{t('settings.adminRequired')}</span>
        </div>
      )}

      {/* 로컬(self) 인스턴스 시스템 정보 카드 (SPEC-WEB-007) */}
      {/* 시스템 정보(OS/버전/업타임)를 로그·API 설정보다 먼저 배치한다. */}
      <SystemInfoCard />
      <SystemRuntimeCard />

      {/* 로그 레벨 설정 카드 */}
      <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
        <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.logLevel')}</h3>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('settings.logLevelDesc')}
        </p>

        <div className="mt-4 max-w-xs">
          <label
            htmlFor="log-level"
            className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
          >
            {t('settings.logLevelSelect')}
          </label>
          <select
            id="log-level"
            value={logLevel}
            onChange={(e) => handleLogLevelChange(e.target.value)}
            disabled={isReadOnly || isUpdating}
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

        {/* 로그 출력 식별자 표시 방식 (이름/ID/둘 다) */}
        <div className="mt-6 max-w-xs">
          <label
            htmlFor="log-style"
            className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
          >
            {t('settings.logStyle')}
          </label>
          <select
            id="log-style"
            value={logStyle}
            onChange={(e) => handleLogStyleChange(e.target.value as LogStyle)}
            disabled={isReadOnly || isStyleUpdating}
            className={cn(
              inputClass,
              'appearance-none bg-[url("data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2020%2020%22%20fill%3D%22%236b7280%22%3E%3Cpath%20fill-rule%3D%22evenodd%22%20d%3D%22M5.22%208.22a.75.75%200%20011.06%200L10%2011.94l3.72-3.72a.75.75%200%20111.06%201.06l-4.25%204.25a.75.75%200%2001-1.06%200L5.22%209.28a.75.75%200%20010-1.06z%22%20clip-rule%3D%22evenodd%22%2F%3E%3C%2Fsvg%3E")] bg-[length:1.25rem] bg-[right_0.5rem_center] bg-no-repeat pr-8',
            )}
          >
            {LOG_STYLES.map((option) => (
              <option key={option.value} value={option.value}>
                {t(option.labelKey)}
              </option>
            ))}
          </select>
          <p className="mt-1 text-sm text-(--color-text-muted)">{t('settings.logStyleHint')}</p>
        </div>
      </div>

      {/* 컴포넌트별 로그 레벨 오버라이드 카드 */}
      {/* isViewer prop 이름은 두 카드의 기존 계약이라 유지하고, 판정만 권한 키
          기반(isReadOnly)으로 바꿔 전달한다. */}
      <ComponentLogLevelOverrides isViewer={isReadOnly} addNotification={addNotification} />

      {/* 스케줄 로그 저장 방식 카드 (시스템 설정 쓰기 권한 없으면 비활성화) */}
      <ScheduleLogStorageCard isViewer={isReadOnly} />

      {/* API 서버 정보 카드 */}
      <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
        <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.apiServer')}</h3>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('settings.apiServerDesc')}
        </p>

        <div className="mt-4 max-w-md">
          <label
            htmlFor="api-url"
            className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
          >
            {t('settings.serverUrl')}
          </label>
          <input
            id="api-url"
            type="text"
            readOnly
            value={apiUrl}
            className={cn(inputClass, 'cursor-default bg-(--color-bg-sunken)')}
          />
        </div>
      </div>
    </div>
  );
}

// ---- 컴포넌트별 로그 레벨 오버라이드 ----

/** 오버라이드 행 모델 (컴포넌트명 + 종류 키 + 현재 레벨). */
interface OverrideRow {
  /** 전체 컴포넌트명 (예: `flow.test-flow.node.modbus-reader`). */
  component: string;
  /** 종류 키 (첫 세그먼트). */
  categoryKey: string;
  /** 현재 로그 레벨 (소문자). */
  level: string;
}

/** 정렬 대상 컬럼. */
type SortColumn = 'category' | 'component' | 'level';
/** 정렬 방향. */
type SortDirection = 'asc' | 'desc';

/** 종류 필터의 전체(미적용) 값. */
const CATEGORY_FILTER_ALL = 'all';

/** 알림 디스패처 타입 (uiStore.addNotification 시그니처). */
type NotifyFn = (n: {
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
}) => void;

/** 레벨 심각도 인덱스 (정렬용). 알 수 없는 레벨은 맨 뒤로 보낸다. */
function levelRank(level: string): number {
  const idx = LOG_LEVELS.indexOf(level as (typeof LOG_LEVELS)[number]);
  return idx === -1 ? LOG_LEVELS.length : idx;
}

/** 정렬 비교자. 컬럼/방향에 따라 두 행을 비교한다. */
function compareRows(a: OverrideRow, b: OverrideRow, column: SortColumn, t: TranslationFn): number {
  switch (column) {
    case 'category': {
      // 현지화된 종류 라벨 기준 비교. 동률이면 컴포넌트명으로 안정 정렬한다.
      const byLabel = getCategoryLabel(a.categoryKey, t).localeCompare(
        getCategoryLabel(b.categoryKey, t),
        'ko',
      );
      return byLabel !== 0 ? byLabel : a.component.localeCompare(b.component, 'ko');
    }
    case 'level': {
      const byLevel = levelRank(a.level) - levelRank(b.level);
      return byLevel !== 0 ? byLevel : a.component.localeCompare(b.component, 'ko');
    }
    case 'component':
    default:
      return a.component.localeCompare(b.component, 'ko');
  }
}

/**
 * Promise.allSettled 결과를 성공/실패 개수로 요약한다.
 * 일괄 작업 후 사용자에게 부분 실패를 명확히 알리기 위함이다.
 */
function summarizeSettled(results: PromiseSettledResult<unknown>[]): {
  fulfilled: number;
  rejected: number;
} {
  let fulfilled = 0;
  let rejected = 0;
  for (const r of results) {
    if (r.status === 'fulfilled') fulfilled += 1;
    else rejected += 1;
  }
  return { fulfilled, rejected };
}

/** 정렬 가능한 테이블 헤더 셀. 클릭으로 정렬 토글, aria-sort 부여. */
function SortableHeader({
  label,
  column,
  sortColumn,
  sortDirection,
  onSort,
  className,
}: {
  label: string;
  column: SortColumn;
  sortColumn: SortColumn;
  sortDirection: SortDirection;
  onSort: (column: SortColumn) => void;
  className?: string;
}) {
  const isActive = sortColumn === column;
  const ariaSort: React.AriaAttributes['aria-sort'] = isActive
    ? sortDirection === 'asc'
      ? 'ascending'
      : 'descending'
    : 'none';

  return (
    <th
      scope="col"
      aria-sort={ariaSort}
      className={cn('pb-2 pr-4 font-medium text-(--color-text-muted)', className)}
    >
      <button
        type="button"
        onClick={() => onSort(column)}
        className="inline-flex items-center gap-1 hover:text-(--color-text-secondary)"
      >
        <span>{label}</span>
        {isActive ? (
          sortDirection === 'asc' ? (
            <ChevronUp className="h-3.5 w-3.5" aria-hidden="true" />
          ) : (
            <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
          )
        ) : (
          <ChevronsUpDown
            className="h-3.5 w-3.5 opacity-50"
            aria-hidden="true"
          />
        )}
      </button>
    </th>
  );
}

/** 종류 배지: 색상 점 + 현지화된 라벨. */
function CategoryBadge({ categoryKey, t }: { categoryKey: string; t: TranslationFn }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-0.5 text-xs font-medium text-(--color-text-secondary)">
      <span
        className={cn('h-2 w-2 shrink-0 rounded-full', getCategoryDotClass(categoryKey))}
        aria-hidden="true"
      />
      {getCategoryLabel(categoryKey, t)}
    </span>
  );
}

/**
 * 컴포넌트별 로그 레벨 오버라이드 관리.
 *
 * 종류(category) 배지 표시, 레벨 직접 변경, 정렬, 종류/이름 필터,
 * 선택 기반 일괄 변경/리셋을 제공한다. viewer 역할은 전체 비활성화된다.
 */
function ComponentLogLevelOverrides({
  isViewer,
  addNotification,
}: {
  isViewer: boolean;
  addNotification: NotifyFn;
}) {
  const { t } = useTranslation();
  const [logLevelInfo, setLogLevelInfo] = useState<LogLevelInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // 정렬 상태
  const [sortColumn, setSortColumn] = useState<SortColumn>('component');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  // 필터 상태
  const [categoryFilter, setCategoryFilter] = useState<string>(CATEGORY_FILTER_ALL);
  const [nameFilter, setNameFilter] = useState('');

  // 선택 상태 (일괄 처리 대상)
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // 진행 상태: 개별 행 변경/리셋 중인 컴포넌트 집합 + 일괄 처리 플래그
  const [pending, setPending] = useState<Set<string>>(new Set());
  const [isBulkProcessing, setIsBulkProcessing] = useState(false);

  // 일괄 변경용 선택 레벨
  const [bulkLevel, setBulkLevel] = useState<string>(LOG_LEVELS[0]);

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

  /** 전체 오버라이드 행 (필터/정렬 전 원본). */
  const allRows = useMemo<OverrideRow[]>(() => {
    if (!logLevelInfo?.components) return [];
    return Object.entries(logLevelInfo.components).map(([component, level]) => ({
      component,
      categoryKey: getCategoryKey(component),
      level,
    }));
  }, [logLevelInfo]);

  /** 존재하는 종류 키 목록 (종류 필터 옵션 구성용). */
  const availableCategories = useMemo<string[]>(() => {
    const keys = new Set(allRows.map((r) => r.categoryKey));
    return Array.from(keys).sort((a, b) =>
      getCategoryLabel(a, t).localeCompare(getCategoryLabel(b, t), 'ko'),
    );
  }, [allRows, t]);

  /** 필터 + 정렬을 적용한 표시 행. */
  const visibleRows = useMemo<OverrideRow[]>(() => {
    const query = nameFilter.trim().toLowerCase();
    const filtered = allRows.filter((r) => {
      if (categoryFilter !== CATEGORY_FILTER_ALL && r.categoryKey !== categoryFilter) {
        return false;
      }
      if (query && !r.component.toLowerCase().includes(query)) {
        return false;
      }
      return true;
    });
    const sorted = [...filtered].sort((a, b) => compareRows(a, b, sortColumn, t));
    return sortDirection === 'asc' ? sorted : sorted.reverse();
  }, [allRows, categoryFilter, nameFilter, sortColumn, sortDirection, t]);

  /** 정렬 토글: 같은 컬럼이면 방향 반전, 다른 컬럼이면 오름차순으로 시작. */
  function handleSort(column: SortColumn) {
    if (column === sortColumn) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortColumn(column);
      setSortDirection('asc');
    }
  }

  /** 단일 컴포넌트 레벨 직접 변경. */
  async function handleLevelChange(component: string, level: string) {
    setPending((prev) => new Set(prev).add(component));
    try {
      await setComponentLogLevel(component, level);
      addNotification({
        type: 'success',
        message: t('settings.logLevelChangedFor')
          .replace('{component}', component)
          .replace('{level}', level),
      });
      loadLogLevels();
    } catch {
      addNotification({ type: 'error', message: t('settings.logLevelError') });
    } finally {
      setPending((prev) => {
        const next = new Set(prev);
        next.delete(component);
        return next;
      });
    }
  }

  /** 단일 컴포넌트 리셋. */
  async function handleReset(component: string) {
    setPending((prev) => new Set(prev).add(component));
    try {
      await resetComponentLogLevel(component);
      addNotification({
        type: 'success',
        message: t('settings.logLevelResetFor').replace('{component}', component),
      });
      loadLogLevels();
    } catch {
      addNotification({ type: 'error', message: t('settings.logLevelResetError') });
    } finally {
      setPending((prev) => {
        const next = new Set(prev);
        next.delete(component);
        return next;
      });
    }
  }

  /** 단일 행 선택 토글. */
  function toggleSelected(component: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(component)) next.delete(component);
      else next.add(component);
      return next;
    });
  }

  /** 표시 중인 행 전체 선택/해제 토글. */
  function toggleSelectAllVisible() {
    setSelected((prev) => {
      const next = new Set(prev);
      const allVisibleSelected = visibleRows.every((r) => next.has(r.component));
      if (allVisibleSelected) {
        for (const r of visibleRows) next.delete(r.component);
      } else {
        for (const r of visibleRows) next.add(r.component);
      }
      return next;
    });
  }

  /** 일괄 레벨 변경: 선택된 모든 컴포넌트에 적용. */
  async function handleBulkApply() {
    const targets = Array.from(selected);
    if (targets.length === 0) return;
    setIsBulkProcessing(true);
    try {
      const results = await Promise.allSettled(
        targets.map((c) => setComponentLogLevel(c, bulkLevel)),
      );
      const { fulfilled, rejected } = summarizeSettled(results);
      addNotification({
        type: rejected === 0 ? 'success' : 'warning',
        message:
          rejected === 0
            ? t('settings.bulkApplied')
                .replace('{count}', String(fulfilled))
                .replace('{level}', bulkLevel)
            : t('settings.bulkApplyPartial')
                .replace('{ok}', String(fulfilled))
                .replace('{fail}', String(rejected)),
      });
    } finally {
      setIsBulkProcessing(false);
      setSelected(new Set());
      loadLogLevels();
    }
  }

  /** 일괄 리셋: 선택된 모든 컴포넌트의 오버라이드 제거. */
  async function handleBulkReset() {
    const targets = Array.from(selected);
    if (targets.length === 0) return;
    setIsBulkProcessing(true);
    try {
      const results = await Promise.allSettled(
        targets.map((c) => resetComponentLogLevel(c)),
      );
      const { fulfilled, rejected } = summarizeSettled(results);
      addNotification({
        type: rejected === 0 ? 'success' : 'warning',
        message:
          rejected === 0
            ? t('settings.bulkResetDone').replace('{count}', String(fulfilled))
            : t('settings.bulkResetPartial')
                .replace('{ok}', String(fulfilled))
                .replace('{fail}', String(rejected)),
      });
    } finally {
      setIsBulkProcessing(false);
      setSelected(new Set());
      loadLogLevels();
    }
  }

  const hasOverrides = allRows.length > 0;
  const selectedCount = selected.size;
  const allVisibleSelected =
    visibleRows.length > 0 && visibleRows.every((r) => selected.has(r.component));
  const someVisibleSelected =
    !allVisibleSelected && visibleRows.some((r) => selected.has(r.component));
  // 컨트롤 비활성화 여부 (viewer 또는 일괄 처리 중).
  const controlsDisabled = isViewer || isBulkProcessing;

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
      <h3 className="text-lg font-semibold text-(--color-text-primary)">
        {t('settings.componentLogLevel')}
      </h3>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('settings.componentLogLevelDesc')}
      </p>

      <div className="mt-4">
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <div key={i} className="h-10 animate-pulse rounded bg-(--color-bg-elevated)" />
            ))}
          </div>
        ) : !hasOverrides ? (
          <p className="text-sm text-(--color-text-muted)">{t('settings.noOverrides')}</p>
        ) : (
          <div className="space-y-4">
            {/* 필터 영역: 종류 필터 + 이름 검색 */}
            <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
              <div className="sm:w-48">
                <label
                  htmlFor="log-category-filter"
                  className="mb-1 block text-xs font-medium text-(--color-text-muted)"
                >
                  {t('settings.category')}
                </label>
                <select
                  id="log-category-filter"
                  value={categoryFilter}
                  onChange={(e) => setCategoryFilter(e.target.value)}
                  className={cn(inputClass, 'py-1.5 text-sm')}
                >
                  <option value={CATEGORY_FILTER_ALL}>{t('settings.categoryAll')}</option>
                  {availableCategories.map((key) => (
                    <option key={key} value={key}>
                      {getCategoryLabel(key, t)}
                    </option>
                  ))}
                </select>
              </div>

              <div className="flex-1">
                <label
                  htmlFor="log-name-filter"
                  className="mb-1 block text-xs font-medium text-(--color-text-muted)"
                >
                  {t('settings.componentSearch')}
                </label>
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute inset-y-0 left-2 my-auto h-4 w-4 text-(--color-text-muted)"
                    aria-hidden="true"
                  />
                  <input
                    id="log-name-filter"
                    type="text"
                    value={nameFilter}
                    onChange={(e) => setNameFilter(e.target.value)}
                    placeholder={t('settings.componentSearchPlaceholder')}
                    className={cn(inputClass, 'py-1.5 pl-8 text-sm')}
                  />
                </div>
              </div>
            </div>

            {/* 일괄 작업 바: 선택된 컴포넌트에 레벨 일괄 적용 / 리셋 */}
            <div className="flex flex-col gap-2 rounded-md border border-(--color-border-subtle) bg-(--color-bg-sunken) p-3 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-xs text-(--color-text-muted)">
                {selectedCount > 0
                  ? t('settings.selectedCount').replace('{count}', String(selectedCount))
                  : t('settings.selectHint')}
              </span>
              <div className="flex flex-wrap items-center gap-2">
                <select
                  aria-label={t('settings.bulkLevelLabel')}
                  value={bulkLevel}
                  onChange={(e) => setBulkLevel(e.target.value)}
                  disabled={controlsDisabled || selectedCount === 0}
                  className={cn(inputClass, 'w-auto py-1.5 text-sm')}
                >
                  {LOG_LEVELS.map((level) => (
                    <option key={level} value={level}>
                      {level.toUpperCase()}
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={handleBulkApply}
                  disabled={controlsDisabled || selectedCount === 0}
                  className={cn(
                    'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                    'bg-blue-600 text-white hover:bg-blue-700',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  {isBulkProcessing ? t('settings.processing') : t('settings.apply')}
                </button>
                <button
                  type="button"
                  onClick={handleBulkReset}
                  disabled={controlsDisabled || selectedCount === 0}
                  className={cn(
                    'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                    'border border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  {t('settings.bulkReset')}
                </button>
              </div>
            </div>

            {/* 필터 결과가 0건일 때의 빈 상태 */}
            {visibleRows.length === 0 ? (
              <p className="py-4 text-center text-sm text-(--color-text-muted)">
                {t('settings.noMatchingComponents')}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead>
                    <tr className="border-b border-(--color-border-default)">
                      <th scope="col" className="pb-2 pr-2">
                        <input
                          type="checkbox"
                          aria-label={t('settings.selectAllVisible')}
                          checked={allVisibleSelected}
                          ref={(el) => {
                            if (el) el.indeterminate = someVisibleSelected;
                          }}
                          onChange={toggleSelectAllVisible}
                          disabled={controlsDisabled}
                          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
                        />
                      </th>
                      <SortableHeader
                        label={t('settings.category')}
                        column="category"
                        sortColumn={sortColumn}
                        sortDirection={sortDirection}
                        onSort={handleSort}
                      />
                      <SortableHeader
                        label={t('settings.column')}
                        column="component"
                        sortColumn={sortColumn}
                        sortDirection={sortDirection}
                        onSort={handleSort}
                      />
                      <SortableHeader
                        label={t('settings.level')}
                        column="level"
                        sortColumn={sortColumn}
                        sortDirection={sortDirection}
                        onSort={handleSort}
                      />
                      <th
                        scope="col"
                        className="pb-2 font-medium text-(--color-text-muted)"
                      >
                        {t('common.actions')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleRows.map((row) => {
                      const isRowPending = pending.has(row.component);
                      const rowDisabled = isViewer || isRowPending || isBulkProcessing;
                      return (
                        <tr
                          key={row.component}
                          className="border-b border-(--color-border-subtle) last:border-0"
                        >
                          <td className="py-2 pr-2">
                            <input
                              type="checkbox"
                              aria-label={t('settings.rowSelect').replace('{component}', row.component)}
                              checked={selected.has(row.component)}
                              onChange={() => toggleSelected(row.component)}
                              disabled={controlsDisabled}
                              className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
                            />
                          </td>
                          <td className="py-2 pr-4">
                            <CategoryBadge categoryKey={row.categoryKey} t={t} />
                          </td>
                          <td className="py-2 pr-4 break-all text-(--color-text-primary)">
                            {row.component}
                          </td>
                          <td className="py-2 pr-4">
                            <select
                              aria-label={t('settings.rowLogLevel').replace('{component}', row.component)}
                              value={row.level}
                              onChange={(e) => handleLevelChange(row.component, e.target.value)}
                              disabled={rowDisabled}
                              className={cn(inputClass, 'w-auto py-1 font-mono text-xs')}
                            >
                              {LOG_LEVELS.map((level) => (
                                <option key={level} value={level}>
                                  {level.toUpperCase()}
                                </option>
                              ))}
                            </select>
                          </td>
                          <td className="py-2">
                            <button
                              type="button"
                              onClick={() => handleReset(row.component)}
                              disabled={rowDisabled}
                              className={cn(
                                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                                'border border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                                'disabled:cursor-not-allowed disabled:opacity-50',
                              )}
                            >
                              {isRowPending ? t('settings.processing') : t('settings.reset')}
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// 테마 탭
// ============================================================

/**
 * 테마 탭: 모드(라이트/다크/시스템) 선택 + 팔레트 컬러 테이블 편집.
 *
 * 앱 전체에서 테마를 바꿀 수 있는 유일한 화면이다(헤더·대시보드의 테마
 * 드롭다운은 제거됨). 모드 선택과 팔레트 편집 모두 즉시 적용된다.
 */
function ThemeTab() {
  const { theme, resolvedPreset, setTheme } = useTheme();
  const { t } = useTranslation();

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
      <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.themeTitle')}</h3>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('settings.themeSubtitle')}
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
                  : 'border-(--color-border-default) bg-(--color-bg-surface) hover:border-(--color-border-strong)',
              )}
              aria-pressed={isSelected}
            >
              <Icon
                className={cn(
                  'h-8 w-8',
                  isSelected
                    ? 'text-blue-600 dark:text-blue-400'
                    : 'text-(--color-text-muted)',
                )}
                aria-hidden="true"
              />
              <div className="text-center">
                <div
                  className={cn(
                    'text-sm font-medium',
                    isSelected
                      ? 'text-blue-600 dark:text-blue-400'
                      : 'text-(--color-text-primary)',
                  )}
                >
                  {t(option.labelKey)}
                </div>
                <div className="mt-0.5 text-xs text-(--color-text-muted)">
                  {t(option.descKey)}
                </div>
              </div>
            </button>
          );
        })}
      </div>

      {/* 팔레트 컬러 테이블 — 라이트/다크 각각을 편집한다. */}
      <div className="mt-8">
        <h4 className="text-base font-semibold text-(--color-text-primary)">
          {t('settings.palette.title')}
        </h4>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {theme === 'system'
            ? t('settings.palette.systemHint')
            : t('settings.palette.subtitle')}
        </p>

        <ThemePaletteEditor activePreset={resolvedPreset} />
      </div>
    </div>
  );
}

// ============================================================
// 언어 탭
// ============================================================

/** 언어 탭: 한국어/영어 선택. i18n 시스템(useTranslation)과 연동한다. */
function LanguageTab() {
  const addNotification = useUIStore((s) => s.addNotification);
  // i18n 의 locale/setLocale 을 직접 사용한다.
  // setLocale 이 'xflow-locale' 저장과 t() 갱신을 모두 처리하므로
  // 별도의 로컬 state·localStorage 조작은 두지 않는다.
  const { locale, setLocale, t } = useTranslation();

  /** 언어 변경 핸들러 */
  function handleLanguageChange(code: Locale) {
    setLocale(code);
    addNotification({
      type: 'success',
      message: t('settings.langChanged'),
    });
  }

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-6 shadow">
      <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.languageTitle')}</h3>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('settings.languageSubtitle')}
      </p>

      <div className="mt-6 space-y-2 max-w-sm">
        {LANGUAGES.map((lang) => {
          const isSelected = locale === lang.code;
          return (
            <label
              key={lang.code}
              className={cn(
                'flex cursor-pointer items-center gap-3 rounded-lg border-2 px-4 py-3 transition-colors',
                isSelected
                  ? 'border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-900/20'
                  : 'border-(--color-border-default) hover:border-(--color-border-strong)',
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
                    : 'text-(--color-text-primary)',
                )}
              >
                {t(lang.labelKey)}
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}
