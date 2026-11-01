import { ToggleGroup } from 'radix-ui'
import { cn } from '@/lib/utils'

type Option<T extends string> = { value: T; label: React.ReactNode; title?: string }

/** Single-choice segmented control; an empty selection is ignored so one option always stays active. */
export function Segmented<T extends string>({ label, value, options, onChange, className }: { label: string; value: T; options: Option<T>[]; onChange: (value: T) => void; className?: string }) {
  return <ToggleGroup.Root type="single" aria-label={label} value={value} onValueChange={(v) => { if (v) onChange(v as T) }} className={cn('segmented', className)}>
    {options.map((o) => <ToggleGroup.Item key={o.value} value={o.value} title={o.title} className="segmented-item" onMouseDown={(e) => e.preventDefault()}>{o.label}</ToggleGroup.Item>)}
  </ToggleGroup.Root>
}
