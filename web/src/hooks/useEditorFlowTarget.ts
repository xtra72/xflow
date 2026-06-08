// useEditorFlowTarget — EditorPage 의 플로우 데이터 소스/저장 대상 추상화
// (SPEC-REMOTE-001 M7, 그룹 I, REQ-I08).
//
// EditorPage 는 로컬 편집과 원격 편집을 동일한 React Flow 캔버스로 처리하되,
// 저장 대상만 다르다(REQ-I08 — "로컬 저장소 vs instance_id 노드 구분"):
//
//   - 로컬:  GET /flows/{id} 로드, PUT /flows/{id} 저장(기존 동작 그대로).
//   - 원격:  GET /remote/nodes/{instanceId}/flows 미러에서 단일 플로우 추출,
//            PATCH /remote/nodes/{instanceId}/flows/{flowId} 저장(신규는 POST).
//
// 본 훅은 그 seam 만 캡슐화하여 EditorPage 본문(캔버스/팔레트/단축키/DnD)을
// 건드리지 않고 데이터 소스를 교체할 수 있게 한다. 로컬 분기는 기존 useFlow +
// useUpdateFlow 를 동일 인자로 위임하므로 로컬 편집 동작은 회귀하지 않는다.
//
// 시크릿(REQ-I07): 원격 저장 시 omitMaskedSecrets 로 마스킹/미변경 시크릿 필드를
// 정의에서 생략한다(필드 부재 — 노드가 기존값 backfill). 미러 정의는 이미
// redaction 되어 시크릿 키가 부재하므로, 편집기에서 새로 입력하지 않는 한
// 시크릿은 자연히 생략된다.

import { useCallback, useMemo } from 'react';

import { useFlow, useUpdateFlow } from '@/hooks/useFlow';
import {
  useCreateRemoteFlow,
  useNodeMirror,
  useUpdateRemoteFlow,
} from '@/hooks/useRemote';
import { omitMaskedSecrets } from '@/lib/remote/secretOmission';
import type { FlowInfo } from '@/types/flow';
import type { MirroredResource } from '@/types/remote';

/** 편집기에 전달할 플로우 데이터(로컬 FlowInfo 와 호환되는 최소 형태). */
export interface EditorFlowData {
  id: string;
  name: string;
  /** nodes/edges/inputs/outputs 를 담은 정의(하이드레이션이 파싱). */
  config: Record<string, unknown>;
}

/** 저장 결과 — 신규 생성 시 노드 채번 id 를 운반한다(REQ-I01 후속 라우팅용). */
export interface EditorSaveResult {
  /** 노드가 채번/확정한 자원 식별자. */
  id: string;
}

/** useEditorFlowTarget 의 입력 식별자. */
export interface EditorFlowTargetParams {
  /** 편집 대상 플로우 id. 신규 원격 플로우면 undefined. */
  flowId?: string;
  /** 원격 편집 대상 노드. 로컬 편집이면 undefined. */
  instanceId?: string;
  /** 신규 원격 플로우 생성 모드 여부. */
  isNew?: boolean;
}

/** useEditorFlowTarget 반환 형태. */
export interface EditorFlowTarget {
  /** 편집기에 하이드레이트할 플로우 데이터. */
  flowData: EditorFlowData | undefined;
  isLoading: boolean;
  error: unknown;
  /** 원격 편집 대상인지 여부(UI 배지/저장 분기). */
  isRemote: boolean;
  /** 저장 진행 중 여부. */
  isSaving: boolean;
  /**
   * 편집된 정의를 저장한다. 로컬은 PUT, 원격은 PATCH(신규는 POST)로 위임한다.
   * 원격 신규 생성 시 노드 채번 id 를 결과로 돌려준다.
   */
  save: (definition: Record<string, unknown>, name: string) => Promise<EditorSaveResult>;
}

/** 미러 정의(JSON 문자열)를 파싱하여 nodes/edges 등 객체로 만든다. */
function parseDefinition(raw: string | undefined): Record<string, unknown> {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    // 손상된 정의는 빈 객체로 폴백한다(편집기는 빈 캔버스로 시작).
  }
  return {};
}

/**
 * EditorPage 의 플로우 데이터 소스/저장 대상을 해석한다.
 *
 * instanceId 가 있으면 원격 편집, 없으면 로컬 편집이다. 두 분기 모두 항상
 * 동일한 React Query 훅 순서로 호출되어야 하므로(Rules of Hooks), 로컬/원격
 * 훅을 모두 호출하되 비활성 분기는 enabled=false / 빈 인자로 무력화한다.
 */
export function useEditorFlowTarget(
  params: EditorFlowTargetParams,
): EditorFlowTarget {
  const { flowId, instanceId, isNew = false } = params;
  const isRemote = !!instanceId;

  // --- 로컬 분기 ---
  // 원격이면 빈 id 로 호출해 useFlow 가 비활성(enabled=!!id=false)되게 한다.
  const localFlow = useFlow(isRemote ? '' : (flowId ?? ''));
  const updateLocal = useUpdateFlow();

  // --- 원격 분기 ---
  // 로컬이면 빈 instanceId 로 호출해 useNodeMirror 가 비활성되게 한다.
  const nodeFlows = useNodeMirror(isRemote ? instanceId : '', 'flow', isRemote);
  const createRemote = useCreateRemoteFlow();
  const updateRemote = useUpdateRemoteFlow();

  // 원격 단일 플로우(미러에서 flowId 매칭). 신규 생성이면 빈 정의로 시작한다.
  const remoteFlowData = useMemo<EditorFlowData | undefined>(() => {
    if (!isRemote) return undefined;
    if (isNew) {
      return { id: '', name: '', config: {} };
    }
    const rows: MirroredResource[] = nodeFlows.data ?? [];
    const match = rows.find((r) => r.id === flowId);
    if (!match) return undefined;
    return {
      id: match.id,
      name: match.name,
      config: parseDefinition(match.definition),
    };
  }, [isRemote, isNew, nodeFlows.data, flowId]);

  // 로컬 FlowInfo → EditorFlowData 정규화.
  const localFlowData = useMemo<EditorFlowData | undefined>(() => {
    if (isRemote) return undefined;
    const info = localFlow.data as FlowInfo | undefined;
    if (!info) return undefined;
    return {
      id: info.id,
      name: info.name,
      config: (info.config as Record<string, unknown>) ?? {},
    };
  }, [isRemote, localFlow.data]);

  // 원격 저장: 시크릿 생략 후 PATCH(기존) 또는 POST(신규).
  const saveRemote = useCallback(
    async (
      definition: Record<string, unknown>,
      name: string,
    ): Promise<EditorSaveResult> => {
      if (!instanceId) throw new Error('원격 저장 대상 노드가 없습니다');
      const cleaned = omitMaskedSecrets(definition);
      if (isNew || !flowId) {
        const created = await createRemote.mutateAsync({
          instanceID: instanceId,
          req: { name: name || 'untitled', definition: cleaned },
        });
        return { id: created.id };
      }
      await updateRemote.mutateAsync({
        instanceID: instanceId,
        flowID: flowId,
        req: { definition: cleaned },
      });
      return { id: flowId };
    },
    [instanceId, flowId, isNew, createRemote, updateRemote],
  );

  // 로컬 저장: 기존 useUpdateFlow 경로 그대로(정의를 definition 으로 전달).
  const saveLocal = useCallback(
    async (definition: Record<string, unknown>): Promise<EditorSaveResult> => {
      if (!flowId) throw new Error('로컬 저장 대상 플로우가 없습니다');
      await updateLocal.mutateAsync({ id: flowId, req: { definition } });
      return { id: flowId };
    },
    [flowId, updateLocal],
  );

  if (isRemote) {
    return {
      flowData: remoteFlowData,
      isLoading: !isNew && nodeFlows.isLoading,
      error: nodeFlows.error,
      isRemote: true,
      isSaving: createRemote.isPending || updateRemote.isPending,
      save: saveRemote,
    };
  }

  return {
    flowData: localFlowData,
    isLoading: localFlow.isLoading,
    error: localFlow.error,
    isRemote: false,
    isSaving: updateLocal.isPending,
    save: (definition) => saveLocal(definition),
  };
}
