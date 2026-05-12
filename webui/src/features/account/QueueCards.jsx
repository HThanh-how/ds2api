import { CheckCircle2, Server, ShieldCheck, AlertTriangle, Clock } from 'lucide-react'

export default function QueueCards({ queueStatus, t }) {
    if (!queueStatus) return null

    const accounts = queueStatus.accounts || []
    const hasDetail = accounts.length > 0

    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                    <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                        <CheckCircle2 className="w-16 h-16" />
                    </div>
                    <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.available')}</p>
                    <div className="mt-2 flex items-baseline gap-2">
                        <span className="text-3xl font-bold text-foreground">{queueStatus.available}</span>
                        <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                    </div>
                </div>
                <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                    <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                        <Server className="w-16 h-16" />
                    </div>
                    <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.inUse')}</p>
                    <div className="mt-2 flex items-baseline gap-2">
                        <span className="text-3xl font-bold text-foreground">{queueStatus.in_use}</span>
                        <span className="text-xs text-muted-foreground">{t('accountManager.threadsUnit')}</span>
                    </div>
                </div>
                <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                    <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                        <ShieldCheck className="w-16 h-16" />
                    </div>
                    <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.totalPool')}</p>
                    <div className="mt-2 flex items-baseline gap-2">
                        <span className="text-3xl font-bold text-foreground">{queueStatus.total}</span>
                        <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                    </div>
                </div>
            </div>

            {queueStatus.waiting > 0 && (
                <div className="flex items-center gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 text-amber-700 px-4 py-3 text-sm">
                    <Clock className="w-4 h-4" />
                    <span>{t('monitor.queueWaiting', { count: queueStatus.waiting, max: queueStatus.max_queue_size || '∞' })}</span>
                </div>
            )}

            {hasDetail && (
                <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
                    <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                            <thead className="border-b border-border bg-background/50">
                                <tr>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground">{t('monitor.accountColumn')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground">{t('monitor.slotsColumn')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground">{t('monitor.statusColumn')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground">{t('monitor.healthColumn')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground">{t('monitor.lastUsedColumn')}</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {accounts.map((acc, idx) => {
                                    const inUse = acc.in_use_slots || 0
                                    const max = acc.max_slots || 2
                                    const health = (acc.health_score ?? 100)
                                    const status = inUse >= max ? 'full' : inUse > 0 ? 'busy' : 'idle'
                                    const statusColors = {
                                        idle: 'bg-emerald-500/20 text-emerald-500 border-emerald-500/30',
                                        busy: 'bg-amber-500/20 text-amber-500 border-amber-500/30',
                                        full: 'bg-blue-500/20 text-blue-500 border-blue-500/30',
                                    }
                                    const statusLabels = {
                                        idle: t('monitor.statusIdle'),
                                        busy: t('monitor.statusBusy'),
                                        full: t('monitor.statusFull'),
                                    }
                                    const healthColor = health >= 80 ? 'text-emerald-500' : health >= 50 ? 'text-amber-500' : 'text-destructive'
                                    const lastUsed = acc.last_used_at
                                        ? new Date(acc.last_used_at).toLocaleTimeString()
                                        : '—'
                                    const hasError = acc.last_error ? true : false
                                    return (
                                        <tr key={acc.id || idx} className="hover:bg-background/50">
                                            <td className="px-4 py-3">
                                                <div className="flex items-center gap-2">
                                                    <span className="font-medium text-foreground">{acc.name || acc.id}</span>
                                                    {hasError && (
                                                        <AlertTriangle className="w-3.5 h-3.5 text-destructive" title={acc.last_error} />
                                                    )}
                                                </div>
                                                <div className="text-xs text-muted-foreground">{acc.id}</div>
                                            </td>
                                            <td className="px-4 py-3">
                                                <span className="font-mono text-xs">{inUse}/{max}</span>
                                                <div className="mt-1 w-16 h-1.5 bg-border rounded-full overflow-hidden">
                                                    <div className={`h-full rounded-full ${inUse >= max ? 'bg-blue-500' : inUse > 0 ? 'bg-amber-500' : 'bg-emerald-500'}`}
                                                        style={{ width: `${Math.min(100, (inUse / max) * 100)}%` }} />
                                                </div>
                                            </td>
                                            <td className="px-4 py-3">
                                                <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium border ${statusColors[status]}`}>
                                                    {statusLabels[status]}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3">
                                                <span className={`font-mono text-xs font-semibold ${healthColor}`}>
                                                    {health}%
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-xs text-muted-foreground">{lastUsed}</td>
                                        </tr>
                                    )
                                })}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}
        </div>
    )
}
