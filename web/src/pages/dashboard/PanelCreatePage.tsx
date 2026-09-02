// 패널 생성 페이지.
// 기존 모달(AddPanelDialog)을 딥링크 라우트(/panels/new)로 승격한 래퍼다.
// 생성 완료/취소 시 대시보드(/)로 복귀한다. AddPanelDialog 본문(위저드 step,
// uiStore 연동)은 그대로 재사용하며, 여기서는 route navigate 만 주입한다.

import { useNavigate } from 'react-router';

import AddPanelDialog from './AddPanelDialog';

/** 패널 생성 딥링크 페이지 (AppLayout 하위 전체화면 라우트). */
export default function PanelCreatePage() {
  const navigate = useNavigate();
  // open 은 항상 true — 페이지 진입 자체가 "열림"이며, 닫기/생성완료는 대시보드로 이동.
  return <AddPanelDialog open onClose={() => navigate('/')} />;
}
