// 헤더 컴포넌트.
// 페이지 제목, 사용자 정보, WebSocket 연결 상태, 테마 선택, 로그아웃을 표시한다.
// 대시보드 라우트('/')에서는 DashboardPage 가 자체 헤더를 렌더하므로 null 을 반환한다.
// 디바이스 라우트('/devices')에서는 디바이스 추가 버튼을 표시한다.
//
// SPEC-WEB-006 v0.1.0 (M4): 우측 액션에 UpdateAvailableBadge 통합.
//   - useSystemVersion 으로 60초 폴링된 update_available/latest_version 사용.
//   - 비관리자(admin 이외)에서는 disabled.
//   - false→true 전이 시 useUpdateAvailableNotification 이 info 토스트 발화.
//   - 클릭 시 /admin/system 라우트로 이동 (Phase F 의 라우팅 등록 후 동작).
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import { useRemoteNodeDetail } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { useWebSocket } from '@/hooks/useWebSocket';
import { cn } from '@/lib/utils/cn';
import { resolveRemoteNodeLabel } from '@/lib/remote/nodeLabel';
import { useSystemVersion } from '@/services/api/systemUpdate';
import { ThemeSelector } from '@/components/theme/ThemeSelector';
import { ThemeEditorModal } from '@/components/theme/ThemeEditorModal';
import { UpdateAvailableBadge } from '@/components/system/UpdateAvailableBadge';
import type { ConnectionState } from '@/services/ws/wsClient';

/**
 * 원격 노드 컨텍스트 경로에서 instanceId 를 추출한다.
 *
 * 원격 노드 하위 경로(`/admin/remote/nodes/:instanceId/...`, 원격 플로우 편집기
 * 포함)에서만 instanceId 를 돌려준다. 그 외 경로는 null 을 반환한다.
 */
function extractRemoteInstanceId(pathname: string): string | null {
  const match = pathname.match(/^\/admin\/remote\/nodes\/([^/]+)/);
  return match ? decodeURIComponent(match[1]!) : null;
}

/** 라우트 경로에 따른 페이지 제목 번역 키 매핑 */
const PAGE_TITLE_KEYS: Record<string, string> = {
  '/': 'nav.dashboard',
  '/flows': 'nav.flows',
  '/agents': 'nav.agents',
  '/devices': 'nav.devices',
  '/nodes': 'nav.nodes',
  '/agent-types': 'nav.agentTypes',
  '/monitoring': 'nav.monitoring',
  '/settings': 'nav.settings',
  // 사용자 관리(역할 탭 포함). 본문에서 제목을 걷어냈으므로 여기서만 그린다.
  '/admin/users': 'nav.users',
};

/** WebSocket 연결 상태에 따른 표시 색상 */
const CONNECTION_STYLES: Record<ConnectionState, { dot: string; labelKey: string }> = {
  connected: {
    dot: 'bg-green-500',
    labelKey: 'status.connected',
  },
  connecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.connecting',
  },
  reconnecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.reconnecting',
  },
  disconnected: {
    dot: 'bg-red-500',
    labelKey: 'status.disconnected',
  },
};

/**
 * 앱 상단 헤더.
 * 현재 페이지 제목, 사용자 정보, 연결 상태, 테마 및 로그아웃 버튼을 표시한다.
 */
export default function Header() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, authEnabled } = useAuth();
  const { state: wsState } = useWebSocket();

  // SPEC-WEB-006 (M4): 시스템 버전 폴링 결과 — UpdateAvailableBadge 가 소비.
  // useSystemVersion 은 React Query cache 로 dedupe 되므로 다른 곳에서도
  // 호출되어 있으면 추가 네트워크 요청은 발생하지 않는다.
  const { data: versionInfo } = useSystemVersion();
  // 관리자만 시스템 업데이트 페이지 접근 가능. authEnabled=false 일 때는
  // 인증 자체가 비활성이므로 항상 활성화한다 (단일-사용자 dev 모드).
  const updateBadgeDisabled = authEnabled ? user?.role !== 'admin' : false;
  const [editorOpen, setEditorOpen] = useState(false);

  // 원격 노드 컨텍스트 제목 — 원격 노드 하위 경로(원격 플로우 편집기 포함)에서는
  // 사이드바 브랜드("XFlow")와 중복되는 폴백 대신 대상 노드 이름을 보여준다.
  // 호스트명은 노드 상세에서 해석하고, 미조회/지연 시 단축 instanceId 로 폴백한다
  // (원시 UUID 전체는 제목에 노출하지 않음). 상세 조회는 렌더를 막지 않는다.
  const remoteInstanceId = extractRemoteInstanceId(location.pathname);
  const { data: remoteNodeDetail } = useRemoteNodeDetail(
    remoteInstanceId ?? '',
    remoteInstanceId !== null,
  );
  const remoteTitle = remoteInstanceId
    ? `${t('remote.header.titlePrefix')} · ${resolveRemoteNodeLabel(
        remoteNodeDetail?.hostname,
        remoteInstanceId,
      )}`
    : null;

  // 대시보드 라우트 여부. 대시보드 관리 컨트롤은 이 헤더가 아니라 DashboardPage 의
  // 자체 헤더와 대시보드 관리 화면(SPEC-DASHBOARD-004 M7)이 제공한다.
  const isDashboardRoute = location.pathname === '/';

  // 현재 라우트에서 페이지 제목 결정. 원격 노드 컨텍스트에서는 노드 이름을
  // 우선 사용한다(폴백 'XFlow' 가 사이드바 브랜드와 중복되지 않도록).
  const pageTitle = remoteTitle ?? derivePageTitle(location.pathname, t);
  const connectionStyle = CONNECTION_STYLES[wsState];

  // 대시보드 라우트에서는 DashboardPage가 자체 헤더를 렌더링하므로 앱 헤더를 숨긴다.
  if (isDashboardRoute) return null;

  return (
    <header className="flex h-(--header-height) shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
      {/* 좌측: 페이지 제목 (대시보드 라우트는 위에서 이미 null 로 반환된다) */}
      <h1 className="text-lg font-semibold text-(--color-text-primary)">{pageTitle}</h1>

      {/* 우측 액션 영역 */}
      <div className="flex items-center gap-4">
        {/* WebSocket 연결 상태 */}
        <div className="flex items-center gap-2" title={`WebSocket: ${t(connectionStyle.labelKey)}`}>
          <span
            className={cn('inline-block h-2 w-2 rounded-full', connectionStyle.dot)}
            aria-hidden="true"
          />
          <span className="text-xs text-(--color-text-muted)">{t(connectionStyle.labelKey)}</span>
        </div>

        {/* SPEC-WEB-006 (M4): 시스템 업데이트 가용 배지 */}
        <UpdateAvailableBadge
          available={versionInfo?.update_available ?? false}
          latestVersion={versionInfo?.latest_version ?? null}
          disabled={updateBadgeDisabled}
          onClick={() => {
            // Phase F 에서 라우트 등록 후 정상 동작. 미등록 상태에서는
            // 단순 navigate 호출이 noop 처리된다.
            navigate('/admin/system');
          }}
        />

        {/* 테마 선택 */}
        <ThemeSelector onOpenEditor={() => setEditorOpen(true)} />

        {/* 커스텀 테마 에디터 */}
        <ThemeEditorModal isOpen={editorOpen} onClose={() => setEditorOpen(false)} />

        {/* 사용자 메뉴는 사이드바 하단으로 이동했다(SPEC-AUTH-006).
            Header 는 대시보드 라우트에서 null 을 반환하므로, 메뉴가 대시보드
            하나만 남은 사용자에게는 로그아웃 경로가 사라지기 때문이다. */}
      </div>
    </header>
  );
}

/**
 * 라우트 경로에서 페이지 제목을 추출한다.
 * 에디터 경로(/editor/:flowId)의 경우 번역된 '플로우'(nav.editor)를 반환한다.
 */
function derivePageTitle(pathname: string, t: (key: string) => string): string {
  // 정적 경로 매핑에서 확인
  if (pathname in PAGE_TITLE_KEYS) {
    return t(PAGE_TITLE_KEYS[pathname]!);
  }

  // 에디터 경로 패턴 (/editor/:flowId)
  if (pathname.startsWith('/editor/')) {
    return t('nav.editor');
  }

  return 'XFlow';
}
