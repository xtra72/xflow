// SPEC-TRIGGER-PANEL-001 M2~M5: 트리거 노드 대시보드 설정 패널.
//
// 특정 trigger 노드({flowId,nodeId})를 타겟팅하여 (1) 현재 스케줄/페이로드를
// useFlowNodes 로 읽어 렌더하고(SSOT 는 노드 config, 패널에 복제하지 않음),
// (2) TriggerScheduleEditor 로 6종 스케줄 CRUD 를 편집하며,
// (3) 패널-로컬 페이로드 카탈로그를 정의하고 스케줄에 스냅샷으로 주입하고(RD-11),
// (4) 저장 시 configureNode(LIVE) + updateFlow(PERSIST) dual-write 로 반영한다(RD-3).
//
// 노드가 실행 중이 아니면(live configure 404) persist-only 로 강등하고 사용자에게
// 통지하며 stopped 배지를 노출한다(RD-10). 동시 편집은 last-write-wins + 통지(RD-8).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlarmClock, CirclePlay, CircleStop, Plus, Save, Trash2 } from 'lucide-react';

import { TriggerScheduleEditor } from '@/components/property/TriggerScheduleEditor';
import { useFlowNodes } from '@/hooks/useFlow';
import { useNodeTypeInstances } from '@/hooks/useNodeTypeInstances';
import { configureNode } from '@/services/api/nodeService';
import { getFlow, updateFlow } from '@/services/api/flowService';
import { useUIStore } from '@/stores/uiStore';
import { APIError } from '@/types/api';
import { cn } from '@/lib/utils/cn';
import {
  buildFullTriggerConfig,
  detectConflict,
  findNodeConfigInDefinition,
  injectCatalogSnapshot,
  patchNodeConfigInDefinition,
  type PayloadCatalog,
  type SerializedSchedule,
} from './triggerPanelUtils';

interface TriggerConfigPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** config.payloadCatalog 를 안전하게 읽는다(없으면 빈 카탈로그). */
function readCatalog(config: Record<string, unknown>): PayloadCatalog {
  const raw = config.payloadCatalog;
  return raw && typeof raw === 'object' && !Array.isArray(raw)
    ? (raw as PayloadCatalog)
    : {};
}

/** 스케줄 요약 라벨(주입 셀렉터 목록 표시용). */
function scheduleLabel(schedule: SerializedSchedule, index: number): string {
  const type = typeof schedule.type === 'string' ? schedule.type : '?';
  return `#${index + 1} · ${type}`;
}

/** 트리거 설정 패널 */
export default function TriggerConfigPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  onTitleChange: _onTitleChange,
}: TriggerConfigPanelProps) {
  const flowId = typeof config.flowId === 'string' ? config.flowId : '';
  const nodeId = typeof config.nodeId === 'string' ? config.nodeId : '';
  const catalog = readCatalog(config);

  const { data: nodes, isLoading } = useFlowNodes(flowId);
  const { instances } = useNodeTypeInstances('trigger');
  const addNotification = useUIStore((s) => s.addNotification);

  // 대상 노드의 현재 config(SSOT). 스케줄/페이로드는 노드 config 가 원본이다.
  const targetNode = useMemo(
    () => (nodes ?? []).find((n) => n.node_id === nodeId),
    [nodes, nodeId],
  );

  // running 인스턴스 목록에 대상이 있으면 running, 없으면 stopped(persist-only).
  const isRunning = useMemo(
    () => instances.some((i) => i.flowId === flowId && i.nodeId === nodeId),
    [instances, flowId, nodeId],
  );

  // 로컬 편집 draft. 대상 노드 config 로부터 최초 1회 하이드레이트한다.
  const [schedules, setSchedules] = useState<SerializedSchedule[]>([]);
  const [saving, setSaving] = useState(false);
  // 동시 편집 감지 baseline(하이드레이션/저장 시점의 노드 config). FULL config 조립 기반.
  const baselineRef = useRef<Record<string, unknown>>({});
  const hydratedRef = useRef(false);

  // nodeId/flowId 변경 시 재하이드레이션 허용.
  useEffect(() => {
    hydratedRef.current = false;
  }, [nodeId, flowId]);

  useEffect(() => {
    if (!targetNode || hydratedRef.current) return;
    const cfg = (targetNode.config ?? {}) as Record<string, unknown>;
    setSchedules(Array.isArray(cfg.schedules) ? (cfg.schedules as SerializedSchedule[]) : []);
    baselineRef.current = cfg;
    hydratedRef.current = true;
  }, [targetNode]);

  const catalogNames = useMemo(() => Object.keys(catalog), [catalog]);

  // ---- 카탈로그 CRUD (패널 config, RD-9) ----

  const [newCatalogName, setNewCatalogName] = useState('');

  const commitCatalog = useCallback(
    (next: PayloadCatalog) => {
      onConfigChange?.({ payloadCatalog: next });
    },
    [onConfigChange],
  );

  const handleAddCatalog = useCallback(() => {
    const name = newCatalogName.trim();
    if (!name || name in catalog) return;
    commitCatalog({ ...catalog, [name]: {} });
    setNewCatalogName('');
  }, [newCatalogName, catalog, commitCatalog]);

  const handleRemoveCatalog = useCallback(
    (name: string) => {
      const next = { ...catalog };
      delete next[name];
      commitCatalog(next);
    },
    [catalog, commitCatalog],
  );

  const handleEditCatalog = useCallback(
    (name: string, value: string) => {
      try {
        const parsed = JSON.parse(value) as Record<string, unknown>;
        commitCatalog({ ...catalog, [name]: parsed });
      } catch {
        // 유효하지 않은 JSON 은 커밋하지 않는다(입력 중 상태 보존).
      }
    },
    [catalog, commitCatalog],
  );

  // ---- 스케줄별 카탈로그 스냅샷 주입 (RD-11) ----

  const handleInject = useCallback(
    (index: number, name: string) => {
      if (!name) return;
      setSchedules((prev) => injectCatalogSnapshot(prev, index, catalog, name));
    },
    [catalog],
  );

  // ---- dual-write 저장 (RD-3/RD-8/RD-10) ----

  const handleSave = useCallback(async () => {
    if (!flowId || !nodeId || saving) return;
    setSaving(true);

    // 카탈로그 주입은 이미 schedules 에 inline 반영되어 있다(스냅샷). FULL config 전송.
    const fullConfig = buildFullTriggerConfig(baselineRef.current, schedules);

    // 1) LIVE: 실행 중 노드에 즉시 반영. 404(미실행)이면 persist-only 로 강등.
    let persistOnly = false;
    try {
      await configureNode(flowId, nodeId, fullConfig);
    } catch (err) {
      if (err instanceof APIError && err.status === 404) {
        persistOnly = true;
      } else {
        addNotification({ type: 'error', message: '저장 실패 — 라이브 반영 중 오류가 발생했습니다.' });
        setSaving(false);
        return;
      }
    }

    // 2) PERSIST: patch-then-PUT. 다른 노드/와이어 보존, 동시 편집 감지.
    let conflict = false;
    try {
      const flow = await getFlow(flowId);
      const def = (flow.config ?? {}) as Record<string, unknown>;
      conflict = detectConflict(findNodeConfigInDefinition(def, nodeId), baselineRef.current);
      const patched = patchNodeConfigInDefinition(def, nodeId, fullConfig);
      await updateFlow(flowId, { definition: patched });
    } catch {
      addNotification({ type: 'error', message: '저장 실패 — 플로우 정의 지속화 중 오류가 발생했습니다.' });
      setSaving(false);
      return;
    }

    // 저장 성공: baseline 갱신 + 통지.
    baselineRef.current = fullConfig;
    addNotification({
      type: persistOnly ? 'warning' : 'success',
      message: persistOnly ? '노드 미실행 — 저장만 적용, 재배포 시 반영' : '저장 완료',
    });
    if (conflict) {
      // RD-8: last-write-wins 로 외부 편집을 덮어썼을 가능성을 통지(무통지 덮어쓰기 없음).
      addNotification({
        type: 'warning',
        message: '다른 편집이 감지되어 덮어썼습니다 (last-write-wins)',
      });
    }
    setSaving(false);
  }, [flowId, nodeId, saving, schedules, addNotification]);

  // ---- 렌더 ----

  const cardCls =
    'flex min-h-0 flex-1 flex-col gap-3 overflow-auto rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)';

  if (!flowId || !nodeId) {
    return (
      <div className={cn(cardCls, 'items-center justify-center')} data-testid="trigger-panel-untargeted">
        <AlarmClock className="mb-1 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">대상 트리거 노드가 지정되지 않았습니다.</p>
      </div>
    );
  }

  return (
    <div className={cardCls} data-testid="trigger-config-panel">
      {/* 헤더: 타이틀 + running/stopped 배지 */}
      <div className="flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <AlarmClock className="h-5 w-5 text-blue-500" />
          <span className="truncate text-base font-bold text-(--color-text-primary)">{title}</span>
        </div>
        {isRunning ? (
          <span
            data-testid="trigger-badge-running"
            className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-1 text-xs font-medium text-green-600 dark:bg-green-900/30 dark:text-green-400"
          >
            <CirclePlay className="h-3.5 w-3.5" /> 실행 중
          </span>
        ) : (
          <span
            data-testid="trigger-badge-stopped"
            className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-1 text-xs font-medium text-slate-500 dark:bg-slate-800 dark:text-slate-400"
          >
            <CircleStop className="h-3.5 w-3.5" /> 중지됨
          </span>
        )}
      </div>

      {/* not-running 안내(persist-only 모드) */}
      {!isRunning && (
        <p
          data-testid="trigger-persist-only-notice"
          className="shrink-0 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400"
        >
          노드 미실행 — 저장 시 지속화만 적용됩니다 (재배포 시 반영).
        </p>
      )}

      {isLoading ? (
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      ) : !targetNode ? (
        <p className="text-xs text-(--color-text-muted)" data-testid="trigger-node-not-found">
          대상 노드를 찾을 수 없습니다.
        </p>
      ) : (
        <>
          {/* 스케줄 편집 (6종 CRUD) — TriggerScheduleEditor 재사용(REQ-03-02) */}
          <section className="shrink-0 space-y-1.5">
            <h3 className="text-xs font-semibold text-(--color-text-secondary)">스케줄</h3>
            <TriggerScheduleEditor value={schedules} onChange={(v) => setSchedules(v as SerializedSchedule[])} />
          </section>

          {/* 페이로드 카탈로그 (패널-로컬, RD-9) */}
          <section className="shrink-0 space-y-1.5" data-testid="trigger-catalog-section">
            <h3 className="text-xs font-semibold text-(--color-text-secondary)">페이로드 카탈로그</h3>
            {catalogNames.length === 0 && (
              <p className="text-[11px] text-(--color-text-muted)">정의된 카탈로그 페이로드가 없습니다.</p>
            )}
            {catalogNames.map((name) => (
              <div key={name} className="rounded border border-(--color-border-default) p-2" data-testid={`catalog-item-${name}`}>
                <div className="mb-1 flex items-center justify-between">
                  <span className="text-xs font-medium text-(--color-text-primary)">{name}</span>
                  <button
                    type="button"
                    onClick={() => handleRemoveCatalog(name)}
                    className="rounded p-0.5 text-gray-400 transition-colors hover:text-red-500"
                    aria-label={`${name} 삭제`}
                    data-testid={`catalog-remove-${name}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
                <textarea
                  rows={3}
                  defaultValue={JSON.stringify(catalog[name] ?? {}, null, 2)}
                  onBlur={(e) => handleEditCatalog(name, e.target.value)}
                  data-testid={`catalog-edit-${name}`}
                  className="w-full rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 font-mono text-xs text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
                />
              </div>
            ))}
            <div className="flex items-center gap-1">
              <input
                type="text"
                value={newCatalogName}
                onChange={(e) => setNewCatalogName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleAddCatalog();
                  }
                }}
                placeholder="이름"
                data-testid="catalog-new-name"
                className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
              />
              <button
                type="button"
                onClick={handleAddCatalog}
                disabled={!newCatalogName.trim()}
                data-testid="catalog-add"
                className="inline-flex shrink-0 items-center gap-1 rounded border border-dashed border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 disabled:opacity-40"
              >
                <Plus className="h-3 w-3" /> 추가
              </button>
            </div>
          </section>

          {/* 스케줄별 카탈로그 주입 셀렉터 (스냅샷, RD-11 / REQ-03-03) */}
          {schedules.length > 0 && (
            <section className="shrink-0 space-y-1.5" data-testid="trigger-inject-section">
              <h3 className="text-xs font-semibold text-(--color-text-secondary)">스케줄별 카탈로그 주입</h3>
              {schedules.map((s, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <span className="w-24 shrink-0 truncate text-[11px] text-(--color-text-muted)">
                    {scheduleLabel(s, idx)}
                  </span>
                  <select
                    value=""
                    onChange={(e) => handleInject(idx, e.target.value)}
                    data-testid={`inject-select-${idx}`}
                    className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
                  >
                    <option value="">카탈로그 선택…</option>
                    {catalogNames.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                </div>
              ))}
            </section>
          )}

          {/* 저장 (dual-write) */}
          <div className="mt-auto flex shrink-0 justify-end pt-1">
            <button
              type="button"
              onClick={handleSave}
              disabled={saving}
              data-testid="trigger-save"
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              <Save className="h-4 w-4" />
              {saving ? '저장 중…' : '저장'}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
