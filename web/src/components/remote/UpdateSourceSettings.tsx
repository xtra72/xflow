// UpdateSourceSettings 는 서버 저장 "업데이트 소스"(GitHub/자체 호스팅 + 채널)를
// 조회/편집하는 전역 설정 패널이다.
//
// 목적: 관리 서버가 원격 노드 프로그램을 업데이트할 때 사용할 다운로드 소스를 서버에
// 한 번 저장해 두고, 매번 설정하지 않고 필요시에만 변경한다(GET/PUT /remote/update-source).
// 저장된 소스는 원격 업데이트 명령에 자동 주입된다.
//
// 보안: 공개키(서명 검증용)는 노드 로컬 신뢰 앵커이므로 서버가 저장/전달하지 않는다.
// 여기서는 다운로드 위치(update_url)와 채널만 제어한다 — 무결성은 각 노드가 자기 로컬
// 공개키로 Ed25519 서명을 검증해 보장한다.

import { useEffect, useState } from 'react';
import { Github, HardDrive, Loader2, Pencil, Server } from 'lucide-react';

import { useSetUpdateSource, useUpdateSource } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { useUIStore } from '@/stores/uiStore';

/** GitHub 기본 릴리스 API 베이스 URL(백엔드 update.update_url 기본값과 동일). */
const GITHUB_DEFAULT_URL = 'https://api.github.com/repos/xtra72/xflow';

/**
 * 이 관리 서버의 릴리스 피드 베이스 URL 을 구성한다.
 * 백엔드는 `{origin}/api/v1/updates` 하위에 GitHub 호환 `/releases/latest` 등을
 * 서빙한다. 노드 다운로더는 https 를 요구하므로 origin 의 http 를 https 로 강제한다.
 */
function thisServerFeedUrl(): string {
  const origin = window.location.origin.replace(/^http:\/\//, 'https://');
  return `${origin}/api/v1/updates`;
}

type SourceKind = 'github' | 'self' | 'this';

const CHANNELS = ['stable', 'beta', 'nightly'] as const;

export function UpdateSourceSettings(): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const sourceQuery = useUpdateSource();
  const setSource = useSetUpdateSource();

  const [editing, setEditing] = useState(false);
  const [kind, setKind] = useState<SourceKind>('github');
  const [url, setUrl] = useState('');
  const [channel, setChannel] = useState<string>('stable');

  const current = sourceQuery.data;
  const currentURL = current?.update_url ?? '';
  const currentChannel = current?.channel ?? '';
  // GitHub 기본값/빈 값이면 GitHub 소스로 간주한다.
  const isGithub = currentURL === '' || currentURL === GITHUB_DEFAULT_URL;
  // 이 관리 서버의 릴리스 피드를 가리키면 "이 관리 서버" 소스로 간주한다.
  const isThisServer = !isGithub && currentURL === thisServerFeedUrl();

  // 편집 시작 시 서버 값으로 초안을 초기화한다.
  useEffect(() => {
    if (!editing) return;
    if (isGithub) {
      setKind('github');
      setUrl('');
    } else if (isThisServer) {
      setKind('this');
      setUrl('');
    } else {
      setKind('self');
      setUrl(currentURL);
    }
    setChannel(currentChannel || 'stable');
  }, [editing, isGithub, isThisServer, currentURL, currentChannel]);

  const handleSave = (): void => {
    let update_url: string;
    if (kind === 'github') {
      update_url = GITHUB_DEFAULT_URL;
    } else if (kind === 'this') {
      update_url = thisServerFeedUrl();
    } else {
      update_url = url.trim();
      if (update_url === '' || !update_url.startsWith('https://')) {
        addNotification({ type: 'error', message: t('remote.updateSource.invalidUrl') });
        return;
      }
    }
    setSource.mutate(
      { update_url, channel },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.updateSource.saved') });
          setEditing(false);
        },
        onError: () =>
          addNotification({ type: 'error', message: t('remote.updateSource.saveFailed') }),
      },
    );
  };

  return (
    <section
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-3"
      data-testid="update-source-settings"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {isGithub ? (
            <Github className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
          ) : isThisServer ? (
            <HardDrive className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
          ) : (
            <Server className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
          )}
          <div className="min-w-0">
            <p className="text-xs font-semibold text-(--color-text-primary)">
              {t('remote.updateSource.title')}
            </p>
            <p className="truncate text-xs text-(--color-text-muted)" data-testid="update-source-current">
              {isGithub
                ? t('remote.updateSource.github')
                : isThisServer
                  ? t('remote.updateSource.thisServer')
                  : currentURL}
              {currentChannel ? ` · ${currentChannel}` : ''}
            </p>
          </div>
        </div>
        {!editing && (
          <button
            type="button"
            onClick={() => setEditing(true)}
            data-testid="update-source-edit"
            className="inline-flex shrink-0 items-center gap-1 rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2.5 py-1 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
          >
            <Pencil className="h-3 w-3" aria-hidden="true" />
            {t('remote.updateSource.change')}
          </button>
        )}
      </div>

      {editing && (
        <div className="mt-3 space-y-3 border-t border-(--color-border-default) pt-3">
          {/* 소스 종류 선택 */}
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => setKind('github')}
              data-testid="update-source-kind-github"
              className={`inline-flex flex-1 items-center justify-center gap-1.5 rounded border px-2.5 py-1.5 text-xs font-medium ${
                kind === 'github'
                  ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
                  : 'border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              <Github className="h-3.5 w-3.5" aria-hidden="true" />
              {t('remote.updateSource.github')}
            </button>
            <button
              type="button"
              onClick={() => setKind('this')}
              data-testid="update-source-kind-this"
              className={`inline-flex flex-1 items-center justify-center gap-1.5 rounded border px-2.5 py-1.5 text-xs font-medium ${
                kind === 'this'
                  ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
                  : 'border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              <HardDrive className="h-3.5 w-3.5" aria-hidden="true" />
              {t('remote.updateSource.thisServer')}
            </button>
            <button
              type="button"
              onClick={() => setKind('self')}
              data-testid="update-source-kind-self"
              className={`inline-flex flex-1 items-center justify-center gap-1.5 rounded border px-2.5 py-1.5 text-xs font-medium ${
                kind === 'self'
                  ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
                  : 'border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              <Server className="h-3.5 w-3.5" aria-hidden="true" />
              {t('remote.updateSource.selfHosting')}
            </button>
          </div>

          {/* 이 관리 서버: 릴리스 피드 URL 안내(https 강제) */}
          {kind === 'this' && (
            <div data-testid="update-source-this-info">
              <p className="truncate font-mono text-xs text-(--color-text-secondary)" title={thisServerFeedUrl()}>
                {thisServerFeedUrl()}
              </p>
              <p className="mt-1 text-xs text-(--color-text-muted)">
                {t('remote.updateSource.thisServerHint')}
              </p>
            </div>
          )}

          {/* 자체 호스팅 URL */}
          {kind === 'self' && (
            <div>
              <label
                htmlFor="update-source-url"
                className="mb-1 block text-xs text-(--color-text-muted)"
              >
                {t('remote.updateSource.urlLabel')}
              </label>
              <input
                id="update-source-url"
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://dl.example.com/xflow"
                data-testid="update-source-url"
                className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
              <p className="mt-1 text-xs text-(--color-text-muted)">
                {t('remote.updateSource.urlHint')}
              </p>
            </div>
          )}

          {/* 채널 */}
          <div>
            <label
              htmlFor="update-source-channel"
              className="mb-1 block text-xs text-(--color-text-muted)"
            >
              {t('remote.updateSource.channelLabel')}
            </label>
            <select
              id="update-source-channel"
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              data-testid="update-source-channel"
              className="rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary)"
            >
              {CHANNELS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>

          {/* 보안 안내: 공개키는 노드 로컬 */}
          <p className="rounded bg-(--color-bg-elevated) px-2 py-1.5 text-xs text-(--color-text-muted)">
            {t('remote.updateSource.securityNote')}
          </p>

          {/* 액션 */}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setEditing(false)}
              data-testid="update-source-cancel"
              className="rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={setSource.isPending}
              data-testid="update-source-save"
              className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {setSource.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.save')}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
