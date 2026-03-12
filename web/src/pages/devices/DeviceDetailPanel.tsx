// 디바이스 상세 패널 컴포넌트.
// 디바이스 상태 속성, 명령 실행, 메타데이터 편집 기능을 제공한다.

import { useCallback, useState } from 'react';
import {
  AlertCircle,
  Check,
  Edit2,
  Loader2,
  MapPin,
  Play,
  Tag,
  X,
} from 'lucide-react';

import { useDevice, useExecuteCommand, useUpdateMetadata } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import type { CommandSpec, ParamSpec } from '@/types/device';

interface DeviceDetailPanelProps {
  deviceId: string;
}

/** 디바이스 행 확장 시 표시되는 상세 패널 */
export default function DeviceDetailPanel({ deviceId }: DeviceDetailPanelProps) {
  const { data: device, isLoading, error } = useDevice(deviceId);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-gray-400" />
        <span className="ml-2 text-sm text-gray-500 dark:text-gray-400">
          불러오는 중...
        </span>
      </div>
    );
  }

  if (error || !device) {
    return (
      <div className="flex items-center justify-center py-8 text-sm text-red-500 dark:text-red-400">
        <AlertCircle className="mr-2 h-4 w-4" />
        디바이스 정보를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="space-y-6 px-6 py-4">
      {/* Section 1: 상태 속성 */}
      {device.state?.properties && Object.keys(device.state.properties).length > 0 && (
        <StatePropertiesSection properties={device.state.properties} />
      )}

      {/* Section 2: 명령 */}
      {device.commands && device.commands.length > 0 && (
        <CommandsSection deviceId={deviceId} commands={device.commands} />
      )}

      {/* Section 3: 메타데이터 */}
      <MetadataSection
        deviceId={deviceId}
        metadata={{
          tags: device.metadata?.tags ?? [],
          location: device.metadata?.location ?? '',
          group: device.metadata?.group ?? '',
          labels: device.metadata?.labels ?? {},
        }}
      />
    </div>
  );
}

// ---- Section 1: 상태 속성 ----

/** 속성값 포맷팅 */
function formatPropertyValue(key: string, value: unknown): string {
  if (value === null || value === undefined) return '-';

  // 온도 관련 키는 C 단위 표시
  if (typeof value === 'number') {
    const lowerKey = key.toLowerCase();
    if (lowerKey.includes('temp')) return `${value}\u00B0C`;
    return String(value);
  }

  if (typeof value === 'boolean') return value ? 'ON' : 'OFF';

  return String(value);
}

function StatePropertiesSection({ properties }: { properties: Record<string, unknown> }) {
  const entries = Object.entries(properties);

  return (
    <div>
      <h4 className="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
        상태 속성
      </h4>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
        {entries.map(([key, value]) => (
          <div
            key={key}
            className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800"
          >
            <p className="text-xs text-gray-500 dark:text-gray-400">{key}</p>
            <p className="mt-0.5 text-sm font-medium text-gray-900 dark:text-white">
              {formatPropertyValue(key, value)}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---- Section 2: 명령 실행 ----

function CommandsSection({
  deviceId,
  commands,
}: {
  deviceId: string;
  commands: CommandSpec[];
}) {
  const [activeCommand, setActiveCommand] = useState<string | null>(null);

  return (
    <div>
      <h4 className="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
        명령
      </h4>
      <div className="space-y-2">
        {commands.map((cmd) => (
          <div key={cmd.name}>
            <div className="flex items-center justify-between rounded-lg border border-gray-200 bg-white px-4 py-2.5 dark:border-gray-600 dark:bg-gray-800">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-white">
                  {cmd.name}
                </p>
                {cmd.description && (
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {cmd.description}
                  </p>
                )}
              </div>
              <button
                type="button"
                onClick={() =>
                  setActiveCommand((prev) => (prev === cmd.name ? null : cmd.name))
                }
                className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
              >
                <Play className="h-3 w-3" />
                실행
              </button>
            </div>

            {activeCommand === cmd.name && (
              <CommandExecutionForm
                deviceId={deviceId}
                command={cmd}
                onClose={() => setActiveCommand(null)}
              />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

/** 명령 실행 폼 */
function CommandExecutionForm({
  deviceId,
  command,
  onClose,
}: {
  deviceId: string;
  command: CommandSpec;
  onClose: () => void;
}) {
  const [paramValues, setParamValues] = useState<Record<string, unknown>>({});
  const [result, setResult] = useState<{ success: boolean; data?: unknown; error?: string } | null>(
    null,
  );
  const executeMutation = useExecuteCommand();

  const handleParamChange = useCallback((name: string, value: unknown) => {
    setParamValues((prev) => ({ ...prev, [name]: value }));
  }, []);

  const handleSubmit = async () => {
    setResult(null);
    try {
      const data = await executeMutation.mutateAsync({
        id: deviceId,
        req: {
          command: command.name,
          params: Object.keys(paramValues).length > 0 ? paramValues : undefined,
        },
      });
      setResult({ success: true, data });
    } catch (err) {
      setResult({
        success: false,
        error: err instanceof Error ? err.message : '명령 실행에 실패했습니다.',
      });
    }
  };

  return (
    <div className="mt-1 rounded-lg border border-blue-200 bg-blue-50/50 p-4 dark:border-blue-800 dark:bg-blue-900/10">
      {/* 파라미터 입력 */}
      {command.params.length > 0 && (
        <div className="mb-3 space-y-3">
          {command.params.map((param) => (
            <ParamInput
              key={param.name}
              param={param}
              value={paramValues[param.name]}
              onChange={(val) => handleParamChange(param.name, val)}
            />
          ))}
        </div>
      )}

      {/* 실행 / 취소 버튼 */}
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={handleSubmit}
          disabled={executeMutation.isPending}
          className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          {executeMutation.isPending ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <Play className="h-3 w-3" />
          )}
          실행
        </button>
        <button
          type="button"
          onClick={onClose}
          className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
        >
          닫기
        </button>
      </div>

      {/* 실행 결과 */}
      {result && (
        <div
          className={cn(
            'mt-3 rounded-md p-3 text-xs',
            result.success
              ? 'border border-green-200 bg-green-50 text-green-700 dark:border-green-800 dark:bg-green-900/20 dark:text-green-400'
              : 'border border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400',
          )}
        >
          {result.success ? (
            <div className="flex items-start gap-1.5">
              <Check className="mt-0.5 h-3 w-3 shrink-0" />
              <span>
                명령이 실행되었습니다.
                {result.data != null && (
                  <pre className="mt-1 whitespace-pre-wrap text-xs">
                    {JSON.stringify(result.data, null, 2)}
                  </pre>
                )}
              </span>
            </div>
          ) : (
            <div className="flex items-start gap-1.5">
              <AlertCircle className="mt-0.5 h-3 w-3 shrink-0" />
              <span>{result.error}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/** 파라미터 입력 컴포넌트 (타입별 렌더링) */
function ParamInput({
  param,
  value,
  onChange,
}: {
  param: ParamSpec;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const inputBase =
    'block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:focus:border-blue-400';

  const label = (
    <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
      {param.name}
      {param.required && <span className="ml-0.5 text-red-500">*</span>}
    </label>
  );

  // enum -> select
  if (param.type === 'enum' && param.enum) {
    return (
      <div>
        {label}
        <select
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          className={inputBase}
        >
          <option value="">선택...</option>
          {param.enum.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      </div>
    );
  }

  // bool -> toggle
  if (param.type === 'bool') {
    return (
      <div className="flex items-center gap-2">
        {label}
        <button
          type="button"
          onClick={() => onChange(!(value as boolean))}
          className={cn(
            'relative inline-flex h-5 w-9 items-center rounded-full transition-colors',
            value ? 'bg-blue-600' : 'bg-gray-300 dark:bg-gray-600',
          )}
        >
          <span
            className={cn(
              'inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform',
              value ? 'translate-x-4.5' : 'translate-x-0.5',
            )}
          />
        </button>
      </div>
    );
  }

  // int / float -> number
  if (param.type === 'int' || param.type === 'float') {
    return (
      <div>
        {label}
        <input
          type="number"
          value={(value as number) ?? ''}
          min={param.min}
          max={param.max}
          step={param.type === 'float' ? 0.1 : 1}
          onChange={(e) => {
            const v = e.target.value;
            if (v === '') {
              onChange(undefined);
            } else {
              onChange(param.type === 'int' ? parseInt(v, 10) : parseFloat(v));
            }
          }}
          className={inputBase}
          placeholder={
            param.min != null && param.max != null
              ? `${param.min} ~ ${param.max}`
              : undefined
          }
        />
      </div>
    );
  }

  // string (default)
  return (
    <div>
      {label}
      <input
        type="text"
        value={(value as string) ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={inputBase}
      />
    </div>
  );
}

// ---- Section 3: 메타데이터 ----

interface MetadataSectionProps {
  deviceId: string;
  metadata: {
    tags: string[];
    location: string;
    group: string;
    labels: Record<string, string>;
  };
}

function MetadataSection({ deviceId, metadata }: MetadataSectionProps) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    location: metadata.location,
    group: metadata.group,
    tagsStr: metadata.tags.join(', '),
    labels: { ...metadata.labels },
  });
  const [newLabelKey, setNewLabelKey] = useState('');
  const [newLabelValue, setNewLabelValue] = useState('');
  const updateMutation = useUpdateMetadata();

  const handleSave = async () => {
    const tags = form.tagsStr
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);

    try {
      await updateMutation.mutateAsync({
        id: deviceId,
        metadata: {
          location: form.location,
          group: form.group,
          tags,
          labels: form.labels,
        },
      });
      setEditing(false);
    } catch {
      // 에러는 React Query에서 처리
    }
  };

  const handleAddLabel = () => {
    if (!newLabelKey.trim()) return;
    setForm((prev) => ({
      ...prev,
      labels: { ...prev.labels, [newLabelKey.trim()]: newLabelValue.trim() },
    }));
    setNewLabelKey('');
    setNewLabelValue('');
  };

  const handleRemoveLabel = (key: string) => {
    setForm((prev) => {
      const next = { ...prev.labels };
      delete next[key];
      return { ...prev, labels: next };
    });
  };

  const inputBase =
    'block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:focus:border-blue-400';

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h4 className="text-sm font-semibold text-gray-900 dark:text-white">
          메타데이터
        </h4>
        {!editing && (
          <button
            type="button"
            onClick={() => setEditing(true)}
            className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
          >
            <Edit2 className="h-3 w-3" />
            편집
          </button>
        )}
      </div>

      {!editing ? (
        /* 읽기 모드 */
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
              <MapPin className="h-3 w-3" />
              위치
            </p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.location || '-'}
            </p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="text-xs text-gray-500 dark:text-gray-400">그룹</p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.group || '-'}
            </p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800">
            <p className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
              <Tag className="h-3 w-3" />
              태그
            </p>
            <p className="mt-0.5 text-sm text-gray-900 dark:text-white">
              {metadata.tags.length > 0 ? metadata.tags.join(', ') : '-'}
            </p>
          </div>
          {Object.entries(metadata.labels).map(([k, v]) => (
            <div
              key={k}
              className="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-gray-600 dark:bg-gray-800"
            >
              <p className="text-xs text-gray-500 dark:text-gray-400">{k}</p>
              <p className="mt-0.5 text-sm text-gray-900 dark:text-white">{v}</p>
            </div>
          ))}
        </div>
      ) : (
        /* 편집 모드 */
        <div className="space-y-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                위치
              </label>
              <input
                type="text"
                value={form.location}
                onChange={(e) => setForm((prev) => ({ ...prev, location: e.target.value }))}
                className={inputBase}
                placeholder="예: 1층 로비"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                그룹
              </label>
              <input
                type="text"
                value={form.group}
                onChange={(e) => setForm((prev) => ({ ...prev, group: e.target.value }))}
                className={inputBase}
                placeholder="예: zone-a"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                태그 (콤마 구분)
              </label>
              <input
                type="text"
                value={form.tagsStr}
                onChange={(e) => setForm((prev) => ({ ...prev, tagsStr: e.target.value }))}
                className={inputBase}
                placeholder="예: hvac, 1층, 로비"
              />
            </div>
          </div>

          {/* 라벨 목록 */}
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
              라벨
            </label>
            {Object.entries(form.labels).length > 0 && (
              <div className="mb-2 space-y-1">
                {Object.entries(form.labels).map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2">
                    <span className="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-gray-700 dark:text-gray-300">
                      {k}: {v}
                    </span>
                    <button
                      type="button"
                      onClick={() => handleRemoveLabel(k)}
                      className="text-red-500 hover:text-red-700 dark:text-red-400"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={newLabelKey}
                onChange={(e) => setNewLabelKey(e.target.value)}
                className={cn(inputBase, 'w-32')}
                placeholder="키"
              />
              <input
                type="text"
                value={newLabelValue}
                onChange={(e) => setNewLabelValue(e.target.value)}
                className={cn(inputBase, 'w-32')}
                placeholder="값"
              />
              <button
                type="button"
                onClick={handleAddLabel}
                className="rounded-md border border-gray-300 px-2 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
              >
                추가
              </button>
            </div>
          </div>

          {/* 저장/취소 버튼 */}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleSave}
              disabled={updateMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {updateMutation.isPending && <Loader2 className="h-3 w-3 animate-spin" />}
              저장
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              취소
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
