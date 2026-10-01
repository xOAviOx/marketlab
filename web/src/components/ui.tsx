import * as Tooltip from '@radix-ui/react-tooltip'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cn } from '../lib/utils'

export function Button({ className, variant = 'default', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'primary' | 'danger' | 'ghost' }) {
  return <button className={cn('button', variant !== 'default' && `button-${variant}`, className)} {...props} />
}

export function Tip({ label, children }: { label: string; children: ReactNode }) {
  return <Tooltip.Root><Tooltip.Trigger asChild>{children}</Tooltip.Trigger><Tooltip.Portal><Tooltip.Content className="tooltip" sideOffset={7}>{label}<Tooltip.Arrow className="fill-line" /></Tooltip.Content></Tooltip.Portal></Tooltip.Root>
}

export function Panel({ title, aside, className, children }: { title: string; aside?: ReactNode; className?: string; children: ReactNode }) {
  return <section className={cn('panel', className)}><header className="panel-head"><h2>{title}</h2>{aside}</header>{children}</section>
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty-state"><span>∅</span><p>{children}</p></div>
}

export function Metric({ label, value, tone }: { label: string; value: string; tone?: 'up' | 'down' }) {
  return <div className="metric"><span>{label}</span><strong className={tone === 'up' ? 'up' : tone === 'down' ? 'down' : ''}>{value}</strong></div>
}
