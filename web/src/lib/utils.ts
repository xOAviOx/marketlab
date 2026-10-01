import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export const cn = (...inputs: ClassValue[]) => twMerge(clsx(inputs))
export const money = (n: number, digits = 2) => n.toLocaleString('en-US', { minimumFractionDigits: digits, maximumFractionDigits: digits })
export const signed = (n: number, suffix = '') => `${n >= 0 ? '+' : ''}${money(n)}${suffix}`
export const compact = (n: number) => Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 }).format(n)
export const stamp = (iso: string) => new Date(iso).toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
