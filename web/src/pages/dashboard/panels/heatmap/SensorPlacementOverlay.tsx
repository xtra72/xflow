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
// @spec SPEC-HEATMAP-PANEL-002

import { useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { toNormalized, applySnap, type NormalizedPos } from './placement';

/** 배치된 센서(키 + 정규화 좌표). */
export interface PlacedSensor {
  key: string;
  pos: NormalizedPos;
}

interface SensorPlacementOverlayProps {
  /** 좌표가 배치된 센서 목록. */
  placed: PlacedSensor[];
  /** 좌표가 없는 미배치 센서 키 목록(팔레트에 표시). */
  unplaced: string[];
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
  onPositionChange,
  onRemove,
  snap,
  markerSize = DEFAULT_MARKER_SIZE,
}: SensorPlacementOverlayProps) {
  const { t } = useTranslation();
  const overlayRef = useRef<HTMLDivElement>(null);
  // 드래그 중인 센서 키(배치/미배치 공통). null 이면 유휴.
  const [dragKey, setDragKey] = useState<string | null>(null);

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
      className="absolute inset-0 z-20"
      onPointerMove={handleMove}
      onPointerUp={handleUp}
    >
      {/* 배치된 센서 마커: 정규화 좌표를 % 로 환산해 배치(리사이즈 불변, AC-E4). */}
      {placed.map(({ key, pos }) => (
        <div
          key={key}
          data-testid={`sensor-marker-${key}`}
          data-sensor-key={key}
          className="group absolute -translate-x-1/2 -translate-y-1/2 touch-none"
          style={{ left: `${pos.x * 100}%`, top: `${pos.y * 100}%` }}
        >
          {/* 드래그 핸들(마커 원). */}
          <button
            type="button"
            data-testid={`sensor-marker-handle-${key}`}
            aria-label={t('dashboard.heatmap.editMarkerAria').replace('{key}', key)}
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
            aria-label={t('dashboard.heatmap.editRemoveAria').replace('{key}', key)}
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => onRemove(key)}
            className="absolute -right-2 -top-2 rounded-full bg-red-500/90 p-0.5 text-white opacity-0 transition-opacity group-hover:opacity-100"
          >
            <X className="h-2.5 w-2.5" />
          </button>
          {/* 센서명 라벨. */}
          <span className="pointer-events-none absolute left-1/2 top-full mt-1 -translate-x-1/2 whitespace-nowrap rounded bg-black/60 px-1 py-0.5 text-[10px] text-white">
            {key}
          </span>
        </div>
      ))}

      {/* 미배치 센서 팔레트: 칩을 도면 위로 끌어 놓으면 좌표가 부여된다(AC-04 배치-인). */}
      {unplaced.length > 0 && (
        <div
          data-testid="sensor-unplaced-palette"
          className="absolute left-2 top-2 flex max-w-[60%] flex-wrap gap-1 rounded-md bg-black/50 p-1.5"
        >
          <span className="w-full text-[10px] font-medium text-white/80">
            {t('dashboard.heatmap.editUnplacedHint')}
          </span>
          {unplaced.map((key) => (
            <button
              key={key}
              type="button"
              data-testid={`sensor-unplaced-${key}`}
              aria-label={t('dashboard.heatmap.editPlaceAria').replace('{key}', key)}
              onPointerDown={startDrag(key)}
              className="cursor-grab touch-none rounded border border-white/40 bg-white/10 px-1.5 py-0.5 text-[10px] text-white active:cursor-grabbing"
            >
              {key}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
