// 센서 배치 드래그 앤 드롭 오버레이 (SPEC-HEATMAP-PANEL-002 T6).
//
// 도면/히트맵 위에 absolute 로 겹치는 마커 오버레이(FacilityLinePanel 의 absolute 배치 패턴 참고).
// 배치된 센서는 정규화 좌표(0..1)를 % 로 환산해 마커로 표시하고(리사이즈 불변, AC-E4), 포인터
// 드래그로 좌표를 실시간 갱신한다(AC-03). 미배치 센서는 팔레트 칩으로 나열하고 도면 위로 끌어
// 놓으면 좌표가 부여된다(AC-04 배치-인). 마커의 제거(X) 버튼은 해당 센서의 좌표만 삭제한다(AC-04).
//
// 좌표 변환은 placement.ts 순수 함수(toNormalized/applySnap)에 위임하며, 컨테이너 실측은 오버레이
// 자신의 getBoundingClientRect 로 얻는다. 드롭 시 clamp 되어 도면 밖 좌표는 저장되지 않는다(AC-E2).
// 렌더/드래그는 좌표 오버레이 레이어에 국한되어 store 폴링/히트맵 렌더를 파괴하지 않는다(REQ-04, R3).
//
// 포인터 우선순위(도면 이동 오버레이와의 공존):
//   같은 편집 중에 도면 이동 오버레이(FloorPlanTransformOverlay)도 스테이지 전체를 덮는다. 둘 다
//   전면(全面) 레이어라 겹치는 순서가 곧 조작 우선순위다 — 마커가 아래에 깔리면 마커를 잡아도
//   도면이 끌려간다(보고된 "센서를 옮기려는데 전체가 이동됨"). 마커는 도면 위에 놓인 물건이므로
//   이 레이어가 위(z-24 > z-22)에 온다.
//   그러면서 **빈 자리 누름은 도면 이동으로 내려보내야** 하므로, 루트는 평소 pointer-events-none
//   이고 마커·팔레트만 이벤트를 받는다. 드래그가 시작되면 루트를 pointer-events-auto 로 되돌려
//   포인터가 마커를 벗어나도 이동/드롭 이벤트를 계속 받는다 — 팔레트 칩은 좌표가 생기는 순간
//   언마운트되므로(미배치 → 배치) 이벤트 수신자를 칩에 둘 수 없다.
//
// @spec SPEC-HEATMAP-PANEL-002

import { useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { toNormalized, applySnap, type NormalizedPos } from './placement';

/** 배치된 센서(센서 동일성 키 + 정규화 좌표). */
export interface PlacedSensor {
  key: string;
  pos: NormalizedPos;
}

interface SensorPlacementOverlayProps {
  /** 좌표가 배치된 센서 목록. */
  placed: PlacedSensor[];
  /** 좌표가 없는 미배치 센서 키 목록(팔레트에 표시). */
  unplaced: string[];
  /**
   * 센서 키 → 사람이 읽는 표시 라벨(선택). 좌표를 키잉하는 동일성 키는 기계용 문자열이므로
   * 마커/칩 텍스트는 이 맵으로 치환한다. 미제공/미등록 키는 키 자체를 표시한다(하위호환).
   */
  labels?: Record<string, string>;
  /** 좌표 갱신(드래그 미리보기 + 드롭 시 clamp 저장). */
  onPositionChange: (key: string, pos: NormalizedPos) => void;
  /** 마커 제거(해당 센서 좌표 항목만 삭제, 센서 자체는 store 유지). */
  onRemove: (key: string) => void;
  /** 그리드 스냅 간격(선택). */
  snap?: number;
  /** 마커 표시 크기(px, 기본 16). */
  markerSize?: number;
}

/** 마커 표시 크기 기본값(px). */
const DEFAULT_MARKER_SIZE = 16;

/** 센서 배치 드래그 오버레이. */
export default function SensorPlacementOverlay({
  placed,
  unplaced,
  labels,
  onPositionChange,
  onRemove,
  snap,
  markerSize = DEFAULT_MARKER_SIZE,
}: SensorPlacementOverlayProps) {
  const { t } = useTranslation();
  const overlayRef = useRef<HTMLDivElement>(null);
  // 드래그 중인 센서 키(배치/미배치 공통). null 이면 유휴.
  const [dragKey, setDragKey] = useState<string | null>(null);

  /** 표시 라벨(없으면 키 자체). 쓰기 키와 표시 텍스트를 분리한다. */
  const labelOf = (key: string): string => labels?.[key] ?? key;

  /** 포인터 client 좌표 → 오버레이 rect 기준 정규화 좌표(스냅 적용). rect 없으면 null. */
  function resolvePos(clientX: number, clientY: number): NormalizedPos | null {
    const rect = overlayRef.current?.getBoundingClientRect();
    if (!rect) return null;
    return applySnap(toNormalized(clientX, clientY, rect), snap);
  }

  function startDrag(key: string) {
    return (e: React.PointerEvent) => {
      e.preventDefault();
      setDragKey(key);
    };
  }

  function handleMove(e: React.PointerEvent) {
    if (dragKey === null) return;
    const pos = resolvePos(e.clientX, e.clientY);
    // 드래그 중 실시간 미리보기(AC-03). 좌표는 이미 clamp 됨(AC-E2).
    if (pos) onPositionChange(dragKey, pos);
  }

  function handleUp(e: React.PointerEvent) {
    if (dragKey === null) return;
    // 드롭 시 최종 좌표를 clamp 저장(도면 밖 금지, AC-E2).
    const pos = resolvePos(e.clientX, e.clientY);
    if (pos) onPositionChange(dragKey, pos);
    setDragKey(null);
  }

  return (
    <div
      ref={overlayRef}
      data-testid="sensor-placement-overlay"
      className={cn(
        // z-24: 도면 이동 오버레이(z-22) 위. 마커는 도면 위에 놓인 물건이므로 조작도 먼저 받는다.
        'absolute inset-0 z-[24]',
        // 유휴 시에는 빈 자리 누름이 아래(도면 이동)로 내려가야 하므로 루트가 이벤트를 받지 않는다.
        // 드래그 중에는 루트가 받아야 포인터가 마커를 벗어나도 이동/드롭이 이어진다.
        dragKey === null ? 'pointer-events-none' : 'pointer-events-auto',
      )}
      onPointerMove={handleMove}
      onPointerUp={handleUp}
      // 스테이지를 벗어난 채 손을 떼면 pointerup 이 오지 않아 드래그가 붙잡힌 채 남는다.
      // 벗어나는 순간을 드롭으로 처리한다 — 좌표는 어차피 [0,1] 로 clamp 되므로 경계에 놓인다.
      onPointerLeave={handleUp}
      onPointerCancel={handleUp}
    >
      {/* 배치된 센서 마커: 정규화 좌표를 % 로 환산해 배치(리사이즈 불변, AC-E4). */}
      {placed.map(({ key, pos }) => (
        <div
          key={key}
          data-testid={`sensor-marker-${key}`}
          data-sensor-key={key}
          className="group pointer-events-auto absolute -translate-x-1/2 -translate-y-1/2 touch-none"
          style={{ left: `${pos.x * 100}%`, top: `${pos.y * 100}%` }}
        >
          {/* 드래그 핸들(마커 원). */}
          <button
            type="button"
            data-testid={`sensor-marker-handle-${key}`}
            aria-label={t('dashboard.heatmap.editMarkerAria').replace('{key}', labelOf(key))}
            onPointerDown={startDrag(key)}
            className={cn(
              'block cursor-grab rounded-full border-2 border-white bg-blue-600 shadow ring-1 ring-black/20 active:cursor-grabbing dark:bg-blue-500',
              dragKey === key && 'ring-2 ring-blue-400',
            )}
            style={{ width: markerSize, height: markerSize }}
          />
          {/* 제거(X) 버튼: 해당 센서 좌표만 삭제(AC-04). */}
          <button
            type="button"
            data-testid={`sensor-remove-${key}`}
            aria-label={t('dashboard.heatmap.editRemoveAria').replace('{key}', labelOf(key))}
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => onRemove(key)}
            className="absolute -right-2 -top-2 rounded-full bg-red-500/90 p-0.5 text-white opacity-0 transition-opacity group-hover:opacity-100"
          >
            <X className="h-2.5 w-2.5" />
          </button>
          {/* 센서명 라벨. 시리즈를 구분하는 서술 표기(`key · metric{k=v}`)는 길어질 수 있으므로
              잘라서 그림을 덮지 않게 하고, 전체 값은 title 로 남긴다. */}
          <span
            title={labelOf(key)}
            className="pointer-events-none absolute left-1/2 top-full mt-1 max-w-[160px] -translate-x-1/2 truncate rounded bg-black/60 px-1 py-0.5 text-[10px] text-white"
          >
            {labelOf(key)}
          </span>
        </div>
      ))}

      {/* 미배치 센서 팔레트: 칩을 도면 위로 끌어 놓으면 좌표가 부여된다(AC-04 배치-인). */}
      {unplaced.length > 0 && (
        <div
          data-testid="sensor-unplaced-palette"
          className="pointer-events-auto absolute left-2 top-2 flex max-w-[60%] flex-wrap gap-1 rounded-md bg-black/50 p-1.5"
        >
          <span className="w-full text-[10px] font-medium text-white/80">
            {t('dashboard.heatmap.editUnplacedHint')}
          </span>
          {unplaced.map((key) => (
            <button
              key={key}
              type="button"
              data-testid={`sensor-unplaced-${key}`}
              aria-label={t('dashboard.heatmap.editPlaceAria').replace('{key}', labelOf(key))}
              title={labelOf(key)}
              onPointerDown={startDrag(key)}
              className="max-w-[180px] cursor-grab touch-none truncate rounded border border-white/40 bg-white/10 px-1.5 py-0.5 text-[10px] text-white active:cursor-grabbing"
            >
              {labelOf(key)}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
