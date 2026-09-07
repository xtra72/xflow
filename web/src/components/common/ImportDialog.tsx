// 가져오기 대화 상자.
// 파일 선택(드래그 앤 드롭 포함)으로 JSON/YAML 파일을 파싱하고,
// 미리보기 후 플로우 또는 에이전트를 일괄 생성한다.
// 플로우 가져오기 시 참조된 에이전트가 서버에 없으면 자동 생성 옵션을 제공한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, AlertTriangle, Check, FileJson, KeyRound, Loader2, Upload, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import {
  parseImportFile,
  validateFlowImport,
  validateAgentImport,
  remapAgentNames,
  regenerateDefinitionIds,
  collectSecretFieldNames,
  REGENERATE_IDS_FLAG,
  type ImportItem,
  type RequiredAgent,
  type AgentResolutionState,
  type ExistingAgentOption,
} from '@/lib/utils/importParser';
import { getAgentConfigSchema } from '@/config/agentSchemas';
import { FormField } from '@/components/property/FormField';
import { useUIStore } from '@/stores/uiStore';
import type { ConfigField } from '@/types/node';
import { createFlow } from '@/services/api/flowService';
import { createAgent, getAgents } from '@/services/api/agentService';

/** 에이전트 타입의 스키마에서 sensitive:true 로 표시된 필드 이름 목록을 반환한다. */
function schemaSensitiveFieldNames(agentType: string | undefined): string[] {
  if (!agentType) return [];
  const schema = getAgentConfigSchema(agentType);
  if (!schema) return [];
  return schema.fields.filter((f) => f.sensitive).map((f) => f.name);
}

interface ImportDialogProps {
  open: boolean;
  onClose: () => void;
  type: 'flow' | 'agent';
  onImportSuccess: () => void;
}

/**
 * 가져오기 모달.
 * 파일 업로드 -> 파싱 -> 미리보기 -> 생성 플로우를 제공한다.
 */
export default function ImportDialog({ open, onClose, type, onImportSuccess }: ImportDialogProps) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const agentFileInputRef = useRef<HTMLInputElement>(null);

  // 상태 관리
  const [items, setItems] = useState<ImportItem[]>([]);
  const [errors, setErrors] = useState<string[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);

  // 누락 에이전트 상태 (플로우 가져오기 전용)
  const [missingAgents, setMissingAgents] = useState<RequiredAgent[]>([]);
  const [agentResolutions, setAgentResolutions] = useState<Map<number, AgentResolutionState>>(new Map());
  const [sameTypeAgents, setSameTypeAgents] = useState<Map<number, ExistingAgentOption[]>>(new Map());
  const [isAgentDragOver, setIsAgentDragOver] = useState(false);

  // 비밀 값 입력 상태 (플로우 가져오기 전용).
  // 키 = 에이전트 이름, 값 = { 필드 이름 -> 입력 값 }.
  const [secretInputs, setSecretInputs] = useState<Map<string, Record<string, string>>>(new Map());

  const addNotification = useUIStore((s) => s.addNotification);

  // 모달이 열릴 때 상태 초기화
  useEffect(() => {
    if (open) {
      setItems([]);
      setErrors([]);
      setIsDragOver(false);
      setIsImporting(false);
      setImportError(null);
      setFileName(null);
      setMissingAgents([]);
      setAgentResolutions(new Map());
      setSameTypeAgents(new Map());
      setIsAgentDragOver(false);
      setSecretInputs(new Map());
    }
  }, [open]);

  // Escape 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isImporting) onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose, isImporting]);

  /** 배경 클릭 시 모달 닫기 */
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !isImporting) onClose();
    },
    [onClose, isImporting],
  );

  /** 파일 처리 로직 */
  const processFile = async (file: File) => {
    setErrors([]);
    setItems([]);
    setImportError(null);
    setFileName(file.name);
    setMissingAgents([]);
    setAgentResolutions(new Map());
    setSameTypeAgents(new Map());
    setSecretInputs(new Map());

    try {
      const data = await parseImportFile(file, t);
      const result = type === 'flow'
        ? validateFlowImport(data, t)
        : validateAgentImport(data, t);

      setErrors(result.errors);
      setItems(result.items);

      // 플로우 가져오기 시 누락 에이전트 확인
      if (type === 'flow' && result.items.length > 0) {
        const allRequired = result.items.flatMap(item => item.requiredAgents ?? []);
        if (allRequired.length > 0) {
          try {
            const { data: existingAgents } = await getAgents({ page: 1, size: 100 });
            const existingNames = new Set(existingAgents.map(a => a.name));

            // 이름 기준 중복 제거
            const uniqueMap = new Map<string, RequiredAgent>();
            for (const agent of allRequired) {
              if (!existingNames.has(agent.name) && !uniqueMap.has(agent.name)) {
                uniqueMap.set(agent.name, agent);
              }
            }

            const missing = Array.from(uniqueMap.values());
            setMissingAgents(missing);

            // 같은 타입의 기존 에이전트를 찾아 드롭다운 옵션으로 제공
            const typeMap = new Map<number, ExistingAgentOption[]>();
            const resolutions = new Map<number, AgentResolutionState>();

            for (let idx = 0; idx < missing.length; idx++) {
              const agent = missing[idx]!;
              if (agent.type) {
                const agentType = agent.type;
                const matched = existingAgents
                  .filter(ea => ea.type === agentType)
                  .map(ea => ({ name: ea.name, type: ea.type, status: ea.status }));
                if (matched.length > 0) {
                  typeMap.set(idx, matched);
                  // 같은 타입 에이전트가 있으면 기본값: 대체 (첫 번째 선택)
                  resolutions.set(idx, {
                    resolution: 'substitute',
                    substituteAgentName: matched[0]!.name,
                  });
                } else {
                  // 같은 타입 에이전트가 없으면 기본값: 새로 생성
                  resolutions.set(idx, { resolution: 'create' });
                }
              } else {
                // 타입 정보 없음 -> 건너뛰기
                resolutions.set(idx, { resolution: 'skip' });
              }
            }

            setSameTypeAgents(typeMap);
            setAgentResolutions(resolutions);
          } catch {
            // 에이전트 조회 실패 시 무시하고 계속 진행
          }
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : t('import.parseFailed');
      setErrors([message]);
    }
  };

  /** 파일 선택 핸들러 */
  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) processFile(file);
    // 같은 파일 재선택 허용
    e.target.value = '';
  };

  /** 드래그 앤 드롭 핸들러 */
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) processFile(file);
  };

  /** 항목 이름 편집 핸들러 */
  const handleNameChange = (index: number, newName: string) => {
    setItems((prev) =>
      prev.map((item, i) => (i === index ? { ...item, editedName: newName } : item)),
    );
  };

  /** 누락 에이전트 해결 방식 변경 */
  const handleResolutionChange = (index: number, value: string) => {
    setAgentResolutions(prev => {
      const next = new Map(prev);
      if (value === 'create') {
        next.set(index, { resolution: 'create' });
      } else if (value === 'skip') {
        next.set(index, { resolution: 'skip' });
      } else {
        // 기존 에이전트로 대체
        next.set(index, { resolution: 'substitute', substituteAgentName: value });
      }
      return next;
    });
  };

  /**
   * 생성 예정(resolution === 'create')인 누락 에이전트별로 사용자 재입력이
   * 필요한 비밀 필드 목록을 계산한다. 대체/건너뛰기로 해결되는 에이전트는
   * 새로 생성하지 않으므로 비밀 입력 대상에서 제외된다.
   */
  const secretPrompts = useMemo(() => {
    if (type !== 'flow') return [];
    const prompts: { agentIndex: number; agent: RequiredAgent; fields: string[] }[] = [];
    for (let idx = 0; idx < missingAgents.length; idx++) {
      const agent = missingAgents[idx]!;
      const state = agentResolutions.get(idx);
      // 생성될 에이전트만 대상 (타입 정보 필수).
      if (state?.resolution !== 'create' || !agent.type) continue;
      const fields = collectSecretFieldNames(agent, schemaSensitiveFieldNames(agent.type));
      if (fields.length > 0) {
        prompts.push({ agentIndex: idx, agent, fields });
      }
    }
    return prompts;
  }, [type, missingAgents, agentResolutions]);

  /** 비밀 필드 입력 값 변경 핸들러 (agentName + fieldName). */
  const handleSecretChange = (agentName: string, fieldName: string, value: string) => {
    setSecretInputs((prev) => {
      const next = new Map(prev);
      const cur = { ...(next.get(agentName) ?? {}) };
      cur[fieldName] = value;
      next.set(agentName, cur);
      return next;
    });
  };

  /** 에이전트 파일로 누락 에이전트 보완 */
  const processAgentFiles = async (files: FileList) => {
    // 파싱 결과 수집
    const parsed: RequiredAgent[] = [];
    for (const file of Array.from(files)) {
      try {
        const data = await parseImportFile(file, t);
        const result = validateAgentImport(data, t);
        for (const item of result.items) {
          parsed.push({ name: item.name, type: item.type, config: item.config });
        }
      } catch {
        // 개별 파일 파싱 실패 시 무시
      }
    }
    if (parsed.length === 0) return;

    // immutable 상태 업데이트
    setMissingAgents(prev => {
      const updated = [...prev];
      const newIndices: number[] = [];

      for (const agent of parsed) {
        const idx = updated.findIndex(a => a.name === agent.name);
        if (idx >= 0) {
          updated[idx] = agent;
          newIndices.push(idx);
        } else {
          newIndices.push(updated.length);
          updated.push(agent);
        }
      }

      // 매칭된 에이전트는 자동으로 '새로 생성'으로 설정
      setAgentResolutions(prev => {
        const next = new Map(prev);
        for (const idx of newIndices) {
          next.set(idx, { resolution: 'create' });
        }
        return next;
      });

      return updated;
    });
  };

  /** 가져오기 실행 */
  const handleImport = async () => {
    setIsImporting(true);
    setImportError(null);

    try {
      // 1단계: 누락 에이전트 해결 (생성 또는 대체 테이블 구성)
      const remapTable: Record<string, string> = {};
      // 비밀 값을 비워둔 채 생성된 에이전트 이름 (가져오기 후 경고용).
      const agentsMissingSecrets: string[] = [];

      if (type === 'flow' && missingAgents.length > 0) {
        for (const [index, state] of agentResolutions) {
          const agent = missingAgents[index];
          if (!agent) continue;

          if (state.resolution === 'create' && agent.type) {
            // 사용자가 입력한 비밀 값을 config 에 병합한다(빈 값은 무시).
            const required = collectSecretFieldNames(
              agent,
              schemaSensitiveFieldNames(agent.type),
            );
            const entered = secretInputs.get(agent.name) ?? {};
            const mergedConfig: Record<string, unknown> = { ...(agent.config ?? {}) };
            for (const fieldName of required) {
              const v = entered[fieldName];
              if (v != null && v !== '') {
                mergedConfig[fieldName] = v;
              }
            }
            // 입력되지 않은 비밀 필드가 남아 있으면 경고 대상에 추가.
            const stillMissing = required.some((f) => {
              const v = mergedConfig[f];
              return v === undefined || v === null || v === '';
            });
            if (stillMissing) {
              agentsMissingSecrets.push(agent.name);
            }

            await createAgent({
              name: agent.name,
              type: agent.type,
              config: Object.keys(mergedConfig).length > 0 ? mergedConfig : undefined,
            });
          } else if (state.resolution === 'substitute' && state.substituteAgentName) {
            remapTable[agent.name] = state.substituteAgentName;
          }
          // skip: 아무것도 하지 않음
        }
      }

      // 2단계: 플로우/에이전트 생성
      for (const item of items) {
        const name = item.editedName.trim() || item.name;

        if (type === 'flow') {
          let definition = (item.definition as Record<string, unknown>) ?? {};
          // 노드/엣지 id 를 새 UUID 로 재생성 — 같은 플로우를 여러 번 가져와도
          // 에디터/미리보기 안에서 id 가 충돌하지 않게 한다.
          definition = regenerateDefinitionIds(definition);
          // 대체 테이블이 있으면 플로우 정의 내 에이전트 이름을 치환
          if (Object.keys(remapTable).length > 0) {
            definition = remapAgentNames(definition, remapTable);
          }
          await createFlow({
            name,
            description: item.description,
            definition,
            // belt-and-suspenders: 백엔드에도 id 재생성을 요청한다("둘 다").
            [REGENERATE_IDS_FLAG]: true,
          });
        } else {
          await createAgent({
            name,
            type: item.type ?? '',
            config: item.config,
          });
        }
      }

      // 비밀 값을 비워둔 에이전트가 있으면 경고 알림으로 안내(가져오기는 계속 진행).
      if (agentsMissingSecrets.length > 0) {
        addNotification({
          type: 'warning',
          message: t('import.secretsMissingWarning').replace(
            '{agents}',
            agentsMissingSecrets.join(', '),
          ),
        });
      }

      onImportSuccess();
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : t('import.importFailed');
      setImportError(message);
    } finally {
      setIsImporting(false);
    }
  };

  /** 유효한 항목이 있고 에러가 없으면 확인 가능 */
  const canImport = items.length > 0 && errors.length === 0 && !isImporting;

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="import-dialog-title"
    >
      <div className="mx-4 flex w-full max-w-lg flex-col rounded-lg bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-6 py-4">
          <h2
            id="import-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            {type === 'flow' ? t('import.title.flow') : t('import.title.agent')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) disabled:opacity-50"
            aria-label={t('common.close')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 px-6 py-4">
          {/* 파일 드롭 영역 */}
          <div
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={cn(
              'flex cursor-pointer flex-col items-center gap-2 rounded-lg border-2 border-dashed px-6 py-8 text-center transition-colors',
              isDragOver
                ? 'border-blue-400 bg-blue-50 dark:border-blue-500 dark:bg-blue-900/20'
                : 'border-(--color-border-strong) bg-(--color-bg-secondary) hover:border-(--color-text-muted)',
            )}
          >
            <Upload className="h-8 w-8 text-(--color-text-muted)" />
            <div>
              <p className="text-sm font-medium text-(--color-text-secondary)">
                {t('import.dropzone')}
              </p>
              <p className="mt-1 text-xs text-(--color-text-muted)">
                {t('import.dropzoneHint')}
              </p>
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,.yaml,.yml"
              onChange={handleFileChange}
              className="hidden"
            />
          </div>

          {/* 선택된 파일 이름 */}
          {fileName && (
            <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
              <FileJson className="h-4 w-4" />
              <span>{fileName}</span>
            </div>
          )}

          {/* 유효성 오류 표시 */}
          {errors.length > 0 && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
                <div className="space-y-1">
                  {errors.map((err, i) => (
                    <p key={i} className="text-sm text-red-700 dark:text-red-300">
                      {err}
                    </p>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* 미리보기 목록 */}
          {items.length > 0 && (
            <div className="space-y-2">
              <p className="text-sm font-medium text-(--color-text-secondary)">
                {t('import.preview').replace('{count}', String(items.length))}
              </p>
              <div className="max-h-48 space-y-2 overflow-y-auto">
                {items.map((item, index) => (
                  <div
                    key={index}
                    className="flex items-center gap-3 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-3"
                  >
                    <Check className="h-4 w-4 shrink-0 text-green-500" />
                    <div className="min-w-0 flex-1">
                      <input
                        type="text"
                        value={item.editedName}
                        onChange={(e) => handleNameChange(index, e.target.value)}
                        className="w-full rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:focus:border-blue-400"
                      />
                      {item.type && (
                        <p className="mt-1 text-xs text-(--color-text-muted)">
                          {t('import.typePrefix')} {item.type}
                        </p>
                      )}
                      {item.description && (
                        <p className="mt-1 truncate text-xs text-(--color-text-muted)">
                          {item.description}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 누락된 에이전트 목록 (플로우 가져오기 전용) */}
          {type === 'flow' && missingAgents.length > 0 && (
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <p className="text-sm font-medium text-amber-700 dark:text-amber-400">
                  {t('import.missingAgents').replace('{count}', String(missingAgents.length))}
                </p>
              </div>
              <p className="text-xs text-(--color-text-muted)">
                {t('import.missingAgentsDesc')}
              </p>
              <div className="max-h-40 space-y-1.5 overflow-y-auto">
                {missingAgents.map((agent, index) => {
                  const options = sameTypeAgents.get(index);
                  const resolution = agentResolutions.get(index);

                  return (
                    <div
                      key={agent.name}
                      className={cn(
                        'flex items-center gap-2.5 rounded-md border p-2.5',
                        agent.type
                          ? 'border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-900/20'
                          : 'border-(--color-border-default) bg-(--color-bg-secondary)',
                      )}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="text-sm font-medium text-(--color-text-primary)">
                            {agent.name}
                          </span>
                          {agent.type && (
                            <span className="text-xs text-(--color-text-muted)">
                              ({agent.type})
                            </span>
                          )}
                        </div>

                        {/* 타입 정보가 있고 같은 타입의 에이전트가 존재하면 드롭다운 표시 */}
                        {agent.type && options && options.length > 0 ? (
                          <select
                            value={
                              resolution?.resolution === 'create'
                                ? '__create__'
                                : resolution?.resolution === 'skip'
                                  ? '__skip__'
                                  : resolution?.substituteAgentName ?? '__create__'
                            }
                            onChange={(e) => {
                              const val = e.target.value;
                              if (val === '__create__') {
                                handleResolutionChange(index, 'create');
                              } else if (val === '__skip__') {
                                handleResolutionChange(index, 'skip');
                              } else {
                                handleResolutionChange(index, val);
                              }
                            }}
                            className="mt-1.5 w-full rounded border border-amber-300 bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary) focus:border-amber-500 focus:outline-none focus:ring-1 focus:ring-amber-500 dark:border-amber-700"
                          >
                            {options.map((opt) => (
                              <option key={opt.name} value={opt.name}>
                                {opt.name} ({t('import.substitute')})
                              </option>
                            ))}
                            <option value="__create__">{t('import.create')}</option>
                            <option value="__skip__">{t('import.skip')}</option>
                          </select>
                        ) : agent.type ? (
                          /* 타입 정보가 있지만 같은 타입의 에이전트가 없으면 자동 생성 표시 */
                          <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                            {t('import.autoCreate')}
                          </p>
                        ) : (
                          /* 타입 정보 없음 */
                          <p className="mt-1 text-xs text-red-500 dark:text-red-400">
                            {t('import.noTypeInfo')}
                          </p>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
              {/* 에이전트 파일로 추가 */}
              <div
                onDragEnter={(e) => { e.preventDefault(); e.stopPropagation(); setIsAgentDragOver(true); }}
                onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); }}
                onDragLeave={(e) => { e.preventDefault(); e.stopPropagation(); setIsAgentDragOver(false); }}
                onDrop={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  setIsAgentDragOver(false);
                  if (e.dataTransfer.files.length > 0) processAgentFiles(e.dataTransfer.files);
                }}
                onClick={() => agentFileInputRef.current?.click()}
                className={cn(
                  'flex cursor-pointer items-center justify-center gap-1.5 rounded-md border-2 border-dashed px-3 py-3 text-xs transition-colors',
                  isAgentDragOver
                    ? 'border-amber-400 bg-amber-100 text-amber-700 dark:border-amber-500 dark:bg-amber-900/30 dark:text-amber-300'
                    : 'border-amber-300 text-amber-600 hover:bg-amber-50 dark:border-amber-700 dark:text-amber-400 dark:hover:bg-amber-900/20',
                )}
              >
                <Upload className="h-3.5 w-3.5" />
                {t('import.addAgentFile')}
                <input
                  ref={agentFileInputRef}
                  type="file"
                  accept=".json,.yaml,.yml"
                  multiple
                  onChange={(e) => {
                    if (e.target.files && e.target.files.length > 0) processAgentFiles(e.target.files);
                    e.target.value = '';
                  }}
                  className="hidden"
                />
              </div>
            </div>
          )}

          {/* 비밀 값 입력 (플로우 가져오기 전용).
              생성 예정 에이전트 중 비밀 필드(비밀번호/토큰 등) 가 제거되었거나
              누락된 경우, 생성 전에 사용자에게 값을 재입력받는다. */}
          {type === 'flow' && secretPrompts.length > 0 && (
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <KeyRound className="h-4 w-4 text-blue-500" />
                <p className="text-sm font-medium text-(--color-text-primary)">
                  {t('import.secretInput').replace('{count}', String(secretPrompts.length))}
                </p>
              </div>
              <p className="text-xs text-(--color-text-muted)">
                {t('import.secretInputDesc')}
              </p>
              <div className="max-h-56 space-y-2 overflow-y-auto">
                {secretPrompts.map(({ agent, fields }) => (
                  <div
                    key={agent.name}
                    className="space-y-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-3"
                  >
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-medium text-(--color-text-primary)">
                        {agent.name}
                      </span>
                      <span className="text-xs text-(--color-text-muted)">
                        ({agent.type})
                      </span>
                    </div>
                    {fields.map((fieldName) => {
                      // FormField 의 password 렌더링을 재사용하기 위해 sensitive 필드를 합성한다.
                      const secretField: ConfigField = {
                        name: fieldName,
                        type: 'string',
                        label: fieldName,
                        sensitive: true,
                      };
                      return (
                        <FormField
                          key={fieldName}
                          field={secretField}
                          value={secretInputs.get(agent.name)?.[fieldName] ?? ''}
                          onChange={(v) => handleSecretChange(agent.name, fieldName, String(v ?? ''))}
                        />
                      );
                    })}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 가져오기 오류 */}
          {importError && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
                <p className="text-sm text-red-700 dark:text-red-300">{importError}</p>
              </div>
            </div>
          )}
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onClick={handleImport}
            disabled={!canImport}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {isImporting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('import.importing')}
              </>
            ) : (
              <>
                <Upload className="h-4 w-4" />
                {t('import.button').replace('{count}', String(items.length))}
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
