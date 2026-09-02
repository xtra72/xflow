// 대시보드 자산(도면 이미지 등) API 클라이언트.
//
// POST /dashboard-assets      → { id, mime, size }   (data-URL 업로드, 내용 해시 id)
// GET  /dashboard-assets/{id} → { id, mime, size, data_url }
//
// 왜 필요한가: 대시보드 snapshot PUT 은 256KB 상한이 있다. 도면 사진을 패널 config 에
// data-URL 로 박으면 그 한 장이 예산을 삼켜 **대시보드 저장 전체가 413 으로 실패**한다
// (히트맵뿐 아니라 다른 패널의 변경까지 유실). 이미지는 여기로 올리고 config 에는 id 만 남긴다.
//
// 인증이 Authorization 헤더 기반이라 <img src="/api/...">
// 직접 참조가 불가능하다(브라우저가 헤더를 싣지 않는다). 그래서 조회는 인증된 요청으로
// 받아 data-URL 을 얻고, 그 문자열을 src 에 넣는다.

import { get, post } from './client';

/** 업로드 응답(메타데이터). */
export interface DashboardAssetMeta {
  id: string;
  mime: string;
  size: number;
}

/** 조회 응답(data-URL 포함). */
interface DashboardAssetBody extends DashboardAssetMeta {
  data_url: string;
}

/**
 * 세션 내 자산 캐시(id → data-URL). 자산은 내용 주소화라 id 가 같으면 내용도 같다 —
 * 무효화가 필요 없고, 같은 도면을 쓰는 패널이 여러 개여도 요청은 한 번이다.
 *
 * in-flight Promise 를 함께 캐시해 동시 마운트되는 패널들이 같은 자산을 중복 요청하지 않는다.
 */
const assetCache = new Map<string, Promise<string>>();

/** data-URL 을 자산으로 업로드하고 id 를 반환한다(같은 내용이면 같은 id). */
export async function uploadDashboardAsset(dataUrl: string): Promise<DashboardAssetMeta> {
  const meta = await post<DashboardAssetMeta>('/dashboard-assets', { data_url: dataUrl });
  return meta;
}

/**
 * 자산 id 로 data-URL 을 가져온다(세션 캐시). 실패하면 캐시에서 제거해 다음 마운트에서
 * 다시 시도할 수 있게 한다 — 일시적 네트워크 오류가 영구 빈 이미지로 굳지 않는다.
 */
export function fetchDashboardAssetUrl(id: string): Promise<string> {
  const cached = assetCache.get(id);
  if (cached) return cached;
  const p = get<DashboardAssetBody>(`/dashboard-assets/${encodeURIComponent(id)}`)
    .then((body) => body.data_url)
    .catch((err) => {
      assetCache.delete(id);
      throw err;
    });
  assetCache.set(id, p);
  return p;
}

/** 테스트용 캐시 초기화. */
export function __clearDashboardAssetCache(): void {
  assetCache.clear();
}
