import { CheckCircle2, Server, ShieldCheck, HeartPulse, Clock } from 'lucide-react'

export default function QueueCards({ queueStatus, t, healthData = {} }) {
    if (!queueStatus) return null

    const totalAccounts = queueStatus.total || 0
    const healthyCount = Object.values(healthData).filter(h => h.status === 'healthy' || h.health_score > 50).length
    const unhealthyCount = totalAccounts - healthyCount

    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
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
                <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                    <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                        <HeartPulse className="w-16 h-16" />
                    </div>
                    <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.healthyAccounts')}</p>
                    <div className="mt-2 flex items-baseline gap-2">
                        <span className={`text-3xl font-bold ${unhealthyCount > 0 ? 'text-amber-500' : 'text-emerald-500'}`}>{healthyCount}</span>
                        <span className="text-xs text-muted-foreground">/ {totalAccounts}</span>
                    </div>
                </div>
            </div>

            {queueStatus.waiting > 0 && (
                <div className="flex items-center gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 text-amber-700 px-4 py-3 text-sm">
                    <Clock className="w-4 h-4" />
                    <span>{t('monitor.queueWaiting', { count: queueStatus.waiting, max: queueStatus.max_queue_size || '∞' })}</span>
                </div>
            )}
        </div>
    )
}
