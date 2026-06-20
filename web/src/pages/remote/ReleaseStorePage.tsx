// 릴리스 저장소 페이지 (관리 서버 호스팅 프로그램 이미지).
//
// 사이드바 `원격 관리` 하위 진입점이다. 아키텍처별 xflowd 이미지(바이너리 + 서명)를
// 이 관리 서버에 저장/조회/삭제한다. 업데이트 소스를 이 서버로 지정한 노드는 자기
// 아키텍처 이미지를 자동 다운로드한다.
//
// 권한/모드: admin 전용 라우트(router.tsx) + server 모드에서만 패널을 노출한다.
// 비-server 모드면 안내(RemoteNotServerNotice)를 표시한다.

import { ReleaseStorePanel } from '@/components/remote/ReleaseStorePanel';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import { useRemoteMode } from '@/hooks/useRemote';

export default function ReleaseStorePage(): React.JSX.Element {
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6" data-testid="release-store-page">
        <RemoteNotServerNotice />
      </div>
    );
  }

  return (
    <div data-testid="release-store-page">
      <ReleaseStorePanel />
    </div>
  );
}
