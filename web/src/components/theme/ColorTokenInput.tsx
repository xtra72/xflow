// 개별 토큰 컬러 피커 + hex 입력 컴포넌트.

interface ColorTokenInputProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
}

export function ColorTokenInput({ label, value, onChange }: ColorTokenInputProps) {
  return (
    <div className="flex items-center gap-2">
      <input
        type="color"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 w-8 cursor-pointer rounded border border-gray-300 p-0.5 dark:border-gray-600"
      />
      <div className="flex-1">
        <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
        <input
          type="text"
          value={value}
          onChange={(e) => {
            const v = e.target.value;
            if (/^#[0-9a-fA-F]{0,6}$/.test(v)) onChange(v);
          }}
          className="w-full rounded border border-gray-200 bg-transparent px-2 py-0.5 text-xs font-mono dark:border-gray-700"
          placeholder="#000000"
        />
      </div>
    </div>
  );
}
