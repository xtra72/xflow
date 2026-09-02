// 패널 설정 페이지.
// 기존 모달(PanelSettingsDialog)을 딥링크 라우트(/panels/:panelId/settings)로
// 승격한 래퍼다. 라우트 파라미터에서 panelId 를 취득하고, 해당 패널이 활성
// 대시보드에 없으면(딥링크 오타/삭제된 패널) 대시보드로 리다이렉트한다.
// 설정 본문(모든 타입별 섹션, 저장/삭제, uiStore 연동)은 그대로 재사용한다.

import { Navigate, useNavigate, useParams } from 'react-router';

import { useUIStore } from '@/stores/uiStore';

import PanelSettingsDialog from './PanelSettingsDialog';

/** 패널 설정 딥링크 페이지 (AppLayout 하위 전체화면 라우트). */
export default function PanelSettingsPage() {
  const { panelId } = useParams<{ panelId: string }>();
  const navigate = useNavigate();

  // 활성 대시보드에서 해당 패널 존재 여부 확인 (PanelSettingsDialog 와 동일 조회 경로).
  const panelExists = useUIStore((s) => {
    const activePage = s.dashboardPages.find((p) => p.id === s.activeDashboardId);
    return activePage?.panels.some((p) => p.id === panelId) ?? false;
  });

  // 패널 부재 시 대시보드로 리다이렉트(딥링크 진입 후 새로고침/삭제 대응).
  if (!panelId || !panelExists) {
    return <Navigate to="/" replace />;
  }

  return <PanelSettingsDialog panelId={panelId} onClose={() => navigate('/')} />;
}
