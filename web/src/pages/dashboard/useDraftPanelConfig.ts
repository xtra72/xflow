// 패널 설정의 draft(편집 중) / committed(저장됨) 상태를 분리 관리하는 훅.
//
// @spec SPEC-PANEL-SETTINGS-001 (T9, REQ-14, AC-14)
//
// - draft: 편집 중 config/title. 옵션/색상/데이터소스 변경은 draft 만 갱신한다.
// - committed: 저장된 값(스토어). 저장 = draft→committed 승격(호출자가 스토어 반영),
//   취소 = draft 폐기·committed 로 복원(cancel).
// - 패널 전환(panelId 변경) 시에만 draft 를 committed 로 재초기화한다(같은 패널에서
//   외부 committed 변경으로 편집 중 draft 를 덮어쓰지 않는다 — 기존 동작 보존).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

export interface DraftPanelConfig {
  draftConfig: Record<string, unknown>;
  draftTitle: string;
  /** config 부분 갱신(patch merge). */
  patchConfig: (patch: Record<string, unknown>) => void;
  /** title 갱신. */
  setTitle: (title: string) => void;
  /** draft 를 committed 로 복원(취소). */
  cancel: () => void;
  /** draft 가 committed 와 다른지 여부. */
  isDirty: boolean;
}

export function useDraftPanelConfig(
  committedConfig: Record<string, unknown>,
  committedTitle: string,
  panelId: string,
): DraftPanelConfig {
  const [draftConfig, setDraftConfig] = useState<Record<string, unknown>>(
    () => committedConfig,
  );
  const [draftTitle, setDraftTitle] = useState<string>(() => committedTitle);

  const prevPanelIdRef = useRef(panelId);
  useEffect(() => {
    if (prevPanelIdRef.current !== panelId) {
      prevPanelIdRef.current = panelId;
      setDraftConfig(committedConfig);
      setDraftTitle(committedTitle);
    }
  }, [panelId, committedConfig, committedTitle]);

  const patchConfig = useCallback((patch: Record<string, unknown>) => {
    setDraftConfig((prev) => ({ ...prev, ...patch }));
  }, []);

  const setTitle = useCallback((title: string) => {
    setDraftTitle(title);
  }, []);

  const cancel = useCallback(() => {
    setDraftConfig(committedConfig);
    setDraftTitle(committedTitle);
  }, [committedConfig, committedTitle]);

  const isDirty = useMemo(
    () =>
      draftTitle !== committedTitle ||
      JSON.stringify(draftConfig) !== JSON.stringify(committedConfig),
    [draftConfig, draftTitle, committedConfig, committedTitle],
  );

  return { draftConfig, draftTitle, patchConfig, setTitle, cancel, isDirty };
}
