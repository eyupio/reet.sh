import type { MachineHealth, Reading } from './api'

export function value(reading?: Reading): number | null { return reading?.value != null && Number.isFinite(reading.value) ? reading.value : null }
export function percent(total?: Reading, available?: Reading): number | null { const t = value(total), a = value(available); return t != null && t > 0 && a != null && a >= 0 && a <= t ? (t - a) / t * 100 : null }
export function bytes(amount: number | null): string { if (amount == null) return '—'; if (amount < 1024) return `${Math.round(amount)} B`; const units = ['KiB', 'MiB', 'GiB', 'TiB']; let n = amount / 1024, i = 0; while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ } return `${n.toFixed(n < 10 ? 1 : 0)} ${units[i]}` }
export function uptime(seconds: number | null): string { if (seconds == null) return '—'; const minutes = Math.floor(seconds / 60), hours = Math.floor(minutes / 60), days = Math.floor(hours / 24); return days ? `${days}d ${hours % 24}h` : hours ? `${hours}h ${minutes % 60}m` : `${minutes}m` }
export function fresh(health: MachineHealth | undefined, serverNow: number): boolean { const c = health?.connection; return !!(c?.connected && c.reportFresh && c.reportAt && serverNow - c.reportAt <= 90000) }
export function age(at: number | undefined, now: number): string { if (!at) return 'not yet observed'; const seconds = Math.max(0, Math.floor((now - at) / 1000)); return seconds < 60 ? `${seconds}s ago` : seconds < 3600 ? `${Math.floor(seconds / 60)}m ago` : `${Math.floor(seconds / 3600)}h ago` }
export function missing(reading?: Reading): string { return reading?.reason ? `Unavailable: ${reading.reason.replaceAll('_', ' ')}` : 'Not yet observed' }
