// 원격 관리 클라이언트 설정 탭 (SPEC-REMOTE-001 원격 관리 클라이언트 설정 UI).
//
// 이 인스턴스(피관리 노드)가 관리 서버에 등록될 때 쓰는 client 모드 설정을 편집한다.
// 값은 서버의 config 오버라이드 레이어에 영속화된다(원본 config 파일 보존). mutable 키는
// 즉시 적용되고, 비-mutable 키는 "재시작 후 적용" 배지로 표시하며 재시작 후 반영된다.
// 시크릿(가입 토큰/부트스트랩 시크릿)은 현재값을 노출하지 않고 "설정됨" 여부만 보여주며,
// 새 값을 입력하지 않으면 기존값을 유지한다. admin 전용 화면이다.

import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Loader2, RotateCw, Save } from 'lucide-react';

import { useRemoteClientConfig, useUpdateRemoteClientConfig } from '@/hooks/useRemoteConfig';
import type { RemoteClientConfigUpdate } from '@/services/api/remoteConfigService';
import { useUIStore } from '@/stores/uiStore';
import { useTranslation } from '@/lib/i18n';
import { APIError } from '@/types/api';
import { cn } from '@/lib/utils/cn';

const INPUT_CLASS =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm ' +
  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 ' +
  'bg-(--color-bg-surface) text-(--color-text-primary) disabled:cursor-not-allowed disabled:opacity-50';

/** 폼 로컬 상태 — 모든 값을 문자열/불리언으로 보관하고 저장 시 변경분만 전송한다. */
interface Draft {
  mode: string;
  server_url: string;
  instance_id: string;
  auto_register: boolean;
  heartbeat_interval: string;
  enrollment_token: string; // 새 값 입력용(빈 값이면 미변경)
  bootstrap_secret: string;
  exposure_flows: string;
  exposure_agents: string;
  exposure_devices: string;
  require_secure: boolean;
  insecure_skip_verify: boolean;
  display_width: string;
  display_height: string;
}

export default function RemoteManagementTab() {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const { data, isLoading, isError, error } = useRemoteClientConfig();
  const updateMutation = useUpdateRemoteClientConfig();

  const [draft, setDraft] = useState<Draft | null>(null);

  // 서버 값 로드 시 드래프트 초기화(시크릿 입력란은 항상 빈 값 — 미변경 의미).
  useEffect(() => {
    if (!data) return;
    setDraft({
      mode: data.mode,
      server_url: data.server_url,
      instance_id: data.instance_id,
      auto_register: data.auto_register,
      heartbeat_interval: data.heartbeat_interval,
      enrollment_token: '',
      bootstrap_secret: '',
      exposure_flows: data.exposure.flows,
      exposure_agents: data.exposure.agents,
      exposure_devices: data.exposure.devices,
      require_secure: data.require_secure,
      insecure_skip_verify: data.insecure_skip_verify,
      display_width: String(data.display_width ?? 0),
      display_height: String(data.display_height ?? 0),
    });
  }, [data]);

  // 재시작 필요 키 집합(서버가 알려준 비-mutable 키).
  const restartKeys = useMemo(
    () => new Set(data?.restart_required_fields ?? []),
    [data?.restart_required_fields],
  );
  const needsRestart = (key: string) => restartKeys.has(key);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-12 text-(--color-text-muted)">
        <Loader2 className="h-4 w-4 animate-spin" /> {t('common.loading')}
      </div>
    );
  }

  // admin 아님(403) 등 조회 실패.
  if (isError) {
    const forbidden = error instanceof APIError && error.status === 403;
    return (
      <div className="flex flex-col items-center gap-2 py-12 text-center text-(--color-text-muted)">
        <AlertTriangle className="h-8 w-8 opacity-40" />
        <p className="text-sm">
          {forbidden ? t('settings.remote.adminOnly') : t('settings.remote.loadError')}
        </p>
      </div>
    );
  }

  if (!draft || !data) return null;

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((d) => (d ? { ...d, [key]: value } : d));

  async function handleSave() {
    if (!draft || !data) return;
    // 변경분만 담는다. 시크릿은 입력값이 있을 때만 전송(빈 값=미변경).
    const req: RemoteClientConfigUpdate = {};
    if (draft.mode !== data.mode) req.mode = draft.mode;
    if (draft.server_url !== data.server_url) req.server_url = draft.server_url;
    if (draft.instance_id !== data.instance_id) req.instance_id = draft.instance_id;
    if (draft.auto_register !== data.auto_register) req.auto_register = draft.auto_register;
    if (draft.heartbeat_interval !== data.heartbeat_interval)
      req.heartbeat_interval = draft.heartbeat_interval;
    if (draft.enrollment_token !== '') req.enrollment_token = draft.enrollment_token;
    if (draft.bootstrap_secret !== '') req.bootstrap_secret = draft.bootstrap_secret;
    if (draft.exposure_flows !== data.exposure.flows) req.exposure_flows = draft.exposure_flows;
    if (draft.exposure_agents !== data.exposure.agents) req.exposure_agents = draft.exposure_agents;
    if (draft.exposure_devices !== data.exposure.devices)
      req.exposure_devices = draft.exposure_devices;
    if (draft.require_secure !== data.require_secure) req.require_secure = draft.require_secure;
    if (draft.insecure_skip_verify !== data.insecure_skip_verify)
      req.insecure_skip_verify = draft.insecure_skip_verify;
    if (Number(draft.display_width) !== data.display_width)
      req.display_width = Number(draft.display_width) || 0;
    if (Number(draft.display_height) !== data.display_height)
      req.display_height = Number(draft.display_height) || 0;

    if (Object.keys(req).length === 0) {
      addNotification({ type: 'info', message: t('settings.remote.noChanges') });
      return;
    }

    try {
      const res = await updateMutation.mutateAsync(req);
      if (res.needs_restart.length > 0) {
        addNotification({
          type: 'success',
          message: t('settings.remote.savedRestart').replace('{n}', String(res.needs_restart.length)),
        });
      } else {
        addNotification({ type: 'success', message: t('settings.remote.saved') });
      }
    } catch (e) {
      addNotification({
        type: 'error',
        message:
          e instanceof APIError ? e.message : t('settings.remote.saveError'),
      });
    }
  }

  const restartBadge = (key: string) =>
    needsRestart(key) ? (
      <span className="ml-2 inline-flex items-center gap-1 rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
        <RotateCw className="h-2.5 w-2.5" />
        {t('settings.remote.restartRequired')}
      </span>
    ) : null;

  return (
    <div className="max-w-2xl space-y-6">
      <div>
        <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('settings.remote.title')}</h3>
        <p className="mt-1 text-sm text-(--color-text-muted)">{t('settings.remote.desc')}</p>
      </div>

      {/* 연결 */}
      <section className="space-y-3">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          {t('settings.remote.sectionConnection')}
        </h4>
        <Field label={t('settings.remote.mode')} badge={restartBadge('remote_management.mode')}>
          <select className={INPUT_CLASS} value={draft.mode} onChange={(e) => set('mode', e.target.value)}>
            <option value="disabled">disabled</option>
            <option value="client">client</option>
            <option value="server">server</option>
          </select>
        </Field>
        <Field label={t('settings.remote.serverUrl')} badge={restartBadge('remote_management.server_url')}>
          <input
            type="text"
            className={INPUT_CLASS}
            placeholder="wss://mgmt.example.com"
            value={draft.server_url}
            onChange={(e) => set('server_url', e.target.value)}
          />
        </Field>
        <Field label={t('settings.remote.instanceId')} badge={restartBadge('remote_management.instance_id')}>
          <input
            type="text"
            className={INPUT_CLASS}
            placeholder={t('settings.remote.instanceIdHint')}
            value={draft.instance_id}
            onChange={(e) => set('instance_id', e.target.value)}
          />
        </Field>
      </section>

      {/* 등록 */}
      <section className="space-y-3">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          {t('settings.remote.sectionRegistration')}
        </h4>
        <CheckboxField
          label={t('settings.remote.autoRegister')}
          badge={restartBadge('remote_management.auto_register')}
          checked={draft.auto_register}
          onChange={(v) => set('auto_register', v)}
        />
        <Field label={t('settings.remote.heartbeat')} badge={restartBadge('remote_management.heartbeat_interval')}>
          <input
            type="text"
            className={INPUT_CLASS}
            placeholder="30s"
            value={draft.heartbeat_interval}
            onChange={(e) => set('heartbeat_interval', e.target.value)}
          />
        </Field>
        <Field
          label={t('settings.remote.enrollmentToken')}
          badge={restartBadge('remote_management.enrollment_token')}
          hint={data.enrollment_token_set ? t('settings.remote.secretSet') : t('settings.remote.secretUnset')}
        >
          <input
            type="password"
            className={INPUT_CLASS}
            placeholder={t('settings.remote.secretPlaceholder')}
            value={draft.enrollment_token}
            onChange={(e) => set('enrollment_token', e.target.value)}
            autoComplete="new-password"
          />
        </Field>
        <Field
          label={t('settings.remote.bootstrapSecret')}
          badge={restartBadge('remote_management.bootstrap_secret')}
          hint={data.bootstrap_secret_set ? t('settings.remote.secretSet') : t('settings.remote.secretUnset')}
        >
          <input
            type="password"
            className={INPUT_CLASS}
            placeholder={t('settings.remote.secretPlaceholder')}
            value={draft.bootstrap_secret}
            onChange={(e) => set('bootstrap_secret', e.target.value)}
            autoComplete="new-password"
          />
        </Field>
      </section>

      {/* 노출 */}
      <section className="space-y-3">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          {t('settings.remote.sectionExposure')}
        </h4>
        <p className="text-xs text-(--color-text-muted)">{t('settings.remote.exposureHint')}</p>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Field label={t('settings.remote.exposureFlows')}>
            <input className={INPUT_CLASS} value={draft.exposure_flows} onChange={(e) => set('exposure_flows', e.target.value)} placeholder="none" />
          </Field>
          <Field label={t('settings.remote.exposureAgents')}>
            <input className={INPUT_CLASS} value={draft.exposure_agents} onChange={(e) => set('exposure_agents', e.target.value)} placeholder="none" />
          </Field>
          <Field label={t('settings.remote.exposureDevices')}>
            <input className={INPUT_CLASS} value={draft.exposure_devices} onChange={(e) => set('exposure_devices', e.target.value)} placeholder="none" />
          </Field>
        </div>
      </section>

      {/* 보안 */}
      <section className="space-y-3">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          {t('settings.remote.sectionSecurity')}
        </h4>
        <CheckboxField
          label={t('settings.remote.requireSecure')}
          badge={restartBadge('remote_management.require_secure')}
          checked={draft.require_secure}
          onChange={(v) => set('require_secure', v)}
        />
        <CheckboxField
          label={t('settings.remote.insecureSkipVerify')}
          badge={restartBadge('remote_management.insecure_skip_verify')}
          checked={draft.insecure_skip_verify}
          onChange={(v) => set('insecure_skip_verify', v)}
        />
      </section>

      {/* 저장 */}
      <div className="flex items-center justify-end gap-2 border-t border-(--color-border-default) pt-4">
        <button
          type="button"
          onClick={handleSave}
          disabled={updateMutation.isPending}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          {updateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {t('settings.remote.save')}
        </button>
      </div>
    </div>
  );
}

// --- 재사용 필드 래퍼 ---

function Field({
  label,
  badge,
  hint,
  children,
}: {
  label: string;
  badge?: React.ReactNode;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <label className="flex items-center text-xs font-medium text-(--color-text-secondary)">
        <span>{label}</span>
        {badge}
      </label>
      {children}
      {hint && <p className="text-[11px] text-(--color-text-muted)">{hint}</p>}
    </div>
  );
}

function CheckboxField({
  label,
  badge,
  checked,
  onChange,
}: {
  label: string;
  badge?: React.ReactNode;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className={cn('flex items-center gap-2 text-sm text-(--color-text-primary)')}>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
      />
      <span className="flex items-center">
        {label}
        {badge}
      </span>
    </label>
  );
}
