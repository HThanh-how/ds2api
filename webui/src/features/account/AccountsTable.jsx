import { useState } from 'react'
import { ChevronLeft, ChevronRight, Check, Copy, Pencil, Play, Plus, Trash2, FolderX, Clock, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import { formatCooldownTime } from './useAccountHealth'

const slotStatusStyle = {
    idle: 'bg-emerald-500/20 text-emerald-500 border-emerald-500/30',
    busy: 'bg-amber-500/20 text-amber-500 border-amber-500/30',
    full: 'bg-blue-500/20 text-blue-500 border-blue-500/30',
}
const healthBadgeStyle = {
    healthy: 'bg-emerald-500/20 text-emerald-500 border-emerald-500/30',
    muted: 'bg-red-500/20 text-red-500 border-red-500/30',
    rate_limited: 'bg-amber-500/20 text-amber-500 border-amber-500/30',
    login_failed: 'bg-gray-500/20 text-gray-400 border-gray-500/30',
    cooldown: 'bg-blue-500/20 text-blue-500 border-blue-500/30',
}
const healthBadgeLabel = {
    healthy: 'Healthy', muted: 'Muted', rate_limited: 'Rate Limited',
    login_failed: 'Login Failed', cooldown: 'Cooldown',
}

export default function AccountsTable({
    t,
    accounts,
    loadingAccounts,
    testing,
    testingAll,
    batchProgress,
    sessionCounts,
    deletingSessions,
    updatingProxy,
    totalAccounts,
    page,
    pageSize,
    totalPages,
    resolveAccountIdentifier,
    proxies,
    onTestAll,
    onShowAddAccount,
    onEditAccount,
    onTestAccount,
    onDeleteAccount,
    onDeleteAllSessions,
    onUpdateAccountProxy,
    onPrevPage,
    onNextPage,
    onPageSizeChange,
    searchQuery,
    onSearchChange,
    envBacked = false,
    healthData = {},
    queueStatus,
}) {
    const [copiedId, setCopiedId] = useState(null)
    const queueAccounts = queueStatus?.accounts || []
    const queueMap = {}
    for (const qa of queueAccounts) {
        queueMap[qa.id] = qa
    }

    const copyId = (id) => {
        navigator.clipboard.writeText(id).then(() => {
            setCopiedId(id)
            setTimeout(() => setCopiedId(null), 1500)
        })
    }

    return (
        <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
            <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-center justify-between gap-4">
                <div>
                    <h2 className="text-lg font-semibold">{t('accountManager.accountsTitle')}</h2>
                    <p className="text-sm text-muted-foreground">{t('accountManager.accountsDesc')}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                    <input
                        type="text"
                        value={searchQuery}
                        onChange={e => onSearchChange(e.target.value)}
                        placeholder={t('accountManager.searchPlaceholder')}
                        className="px-3 py-1.5 text-sm bg-muted border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring placeholder:text-muted-foreground"
                    />
                    <button
                        onClick={onTestAll}
                        disabled={testingAll || totalAccounts === 0}
                        className="flex items-center px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50"
                    >
                        {testingAll ? <span className="animate-spin mr-2">⟳</span> : <Play className="w-3 h-3 mr-2" />}
                        {t('accountManager.testAll')}
                    </button>
                    <button
                        onClick={onShowAddAccount}
                        className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors font-medium text-sm shadow-sm"
                    >
                        <Plus className="w-4 h-4" />
                        {t('accountManager.addAccount')}
                    </button>
                </div>
            </div>

            {testingAll && batchProgress.total > 0 && (
                <div className="p-4 border-b border-border bg-muted/30">
                    <div className="flex items-center justify-between text-sm mb-2">
                        <span className="font-medium">{t('accountManager.testingAllAccounts')}</span>
                        <span className="text-muted-foreground">{batchProgress.current} / {batchProgress.total}</span>
                    </div>
                    <div className="w-full bg-muted rounded-full h-2 overflow-hidden mb-4">
                        <div
                            className="bg-primary h-full transition-all duration-300"
                            style={{ width: `${(batchProgress.current / batchProgress.total) * 100}%` }}
                        />
                    </div>
                    {batchProgress.results.length > 0 && (
                        <div className="grid grid-cols-2 md:grid-cols-4 gap-2 max-h-32 overflow-y-auto custom-scrollbar">
                            {batchProgress.results.map((r, i) => (
                                <div key={i} className={clsx(
                                    "text-xs px-2 py-1 rounded border truncate",
                                    r.success ? "bg-emerald-500/10 border-emerald-500/20 text-emerald-500" : "bg-destructive/10 border-destructive/20 text-destructive"
                                )}>
                                    {r.success ? '✓' : '✗'} {r.id}
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            )}

            <div className="overflow-x-auto">
                <table className="w-full text-sm">
                    <thead className="border-b border-border bg-background/50">
                        <tr>
                            <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs w-[220px]">Account</th>
                            <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs w-[90px]">Slots</th>
                            <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs w-[80px]">Status</th>
                            <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs w-[180px]">Health</th>
                            <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">Info</th>
                            <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs w-[220px]">Actions</th>
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                        {loadingAccounts ? (
                            <tr><td colSpan={6} className="p-8 text-center text-muted-foreground">{t('actions.loading')}</td></tr>
                        ) : accounts.length === 0 ? (
                            <tr><td colSpan={6} className="p-8 text-center text-muted-foreground">{searchQuery ? t('accountManager.searchNoResults') : t('accountManager.noAccounts')}</td></tr>
                        ) : accounts.map((acc, i) => {
                            const id = resolveAccountIdentifier(acc)
                            const assignedProxy = proxies.find(proxy => proxy.id === acc.proxy_id)
                            const runtimeUnknown = envBacked && !acc.test_status
                            const isActive = acc.test_status === 'ok' || acc.has_token

                            const hd = healthData[id]
                            const hStatus = hd?.status || 'healthy'
                            const hScore = hd?.health_score ?? 100
                            const cooldown = formatCooldownTime(hd?.cooldown_until || hd?.mute_until)

                            const qa = queueMap[id]
                            const inUse = qa?.in_use_slots || 0
                            const maxSlots = qa?.max_slots || 2
                            const slotStatus = inUse >= maxSlots ? 'full' : inUse > 0 ? 'busy' : 'idle'
                            const slotLabel = slotStatus === 'idle' ? t('monitor.statusIdle') : slotStatus === 'busy' ? t('monitor.statusBusy') : t('monitor.statusFull')

                            const dotColor = hStatus === 'muted' ? "bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]"
                                : hStatus === 'rate_limited' ? "bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.5)]"
                                : hStatus === 'login_failed' ? "bg-gray-500 shadow-[0_0_8px_rgba(107,114,128,0.5)]"
                                : hStatus === 'cooldown' ? "bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.5)]"
                                : acc.test_status === 'failed' ? "bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]"
                                : isActive ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]"
                                : runtimeUnknown ? "bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.5)]" : "bg-amber-500"

                            const healthColor = hScore >= 80 ? 'text-emerald-500' : hScore >= 50 ? 'text-amber-500' : 'text-red-500'
                            const healthBarColor = hScore >= 80 ? 'bg-emerald-500' : hScore >= 50 ? 'bg-amber-500' : 'bg-red-500'

                                return (
                                <tr key={id ? `acc-${id}` : `acc-idx-${i}`} className="hover:bg-muted/30 transition-colors">
                                    {/* Account */}
                                    <td className="px-4 py-3">
                                        <div className="flex items-center gap-2.5">
                                            <div className={clsx("w-2 h-2 rounded-full shrink-0", dotColor)} />
                                            <div className="min-w-0">
                                                <div className="text-sm font-medium truncate">{acc.name || '-'}</div>
                                                <div
                                                    className="text-xs text-muted-foreground truncate flex items-center gap-1 cursor-pointer hover:text-primary transition-colors group"
                                                    onClick={() => copyId(id)}
                                                >
                                                    <span className="truncate">{id || '-'}</span>
                                                    {copiedId === id
                                                        ? <Check className="w-3 h-3 text-emerald-500 shrink-0" />
                                                        : <Copy className="w-3 h-3 opacity-0 group-hover:opacity-50 shrink-0 transition-opacity" />
                                                    }
                                                </div>
                                                {acc.remark && (
                                                    <div className="text-[10px] text-muted-foreground/70 truncate">{acc.remark}</div>
                                                )}
                                            </div>
                                        </div>
                                    </td>

                                    {/* Slots */}
                                    <td className="px-4 py-3">
                                        <div className="flex items-center gap-2">
                                            <span className="font-mono text-xs">{inUse}/{maxSlots}</span>
                                        </div>
                                        <div className="mt-1 w-16 h-1.5 bg-border rounded-full overflow-hidden">
                                            <div
                                                className={clsx("h-full rounded-full transition-all",
                                                    inUse >= maxSlots ? 'bg-blue-500' : inUse > 0 ? 'bg-amber-500' : 'bg-emerald-500'
                                                )}
                                                style={{ width: `${Math.min(100, (inUse / maxSlots) * 100)}%` }}
                                            />
                                        </div>
                                    </td>

                                    {/* Status */}
                                    <td className="px-4 py-3">
                                        <span className={clsx("inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium border", slotStatusStyle[slotStatus])}>
                                            {slotLabel}
                                        </span>
                                    </td>

                                    {/* Health */}
                                    <td className="px-4 py-3">
                                        <div className="flex flex-col gap-1">
                                            <div className="flex items-center gap-2">
                                                <span className={clsx("font-mono text-xs font-semibold", healthColor)}>
                                                    {Math.round(hScore)}%
                                                </span>
                                                <span className={clsx("inline-flex items-center px-1.5 py-0.5 rounded-full text-[10px] font-medium border",
                                                    healthBadgeStyle[hStatus] || healthBadgeStyle.healthy
                                                )}>
                                                    {healthBadgeLabel[hStatus] || 'Healthy'}
                                                </span>
                                            </div>
                                            <div className="w-20 h-1.5 bg-border rounded-full overflow-hidden">
                                                <div className={clsx("h-full rounded-full transition-all", healthBarColor)}
                                                    style={{ width: `${Math.min(100, hScore)}%` }} />
                                            </div>
                                            {cooldown && (
                                                <span className="text-[10px] text-red-400 flex items-center gap-1">
                                                    <Clock className="w-3 h-3" /> {cooldown}
                                                </span>
                                            )}
                                            {hd?.last_failure_reason && hStatus !== 'healthy' && (
                                                <span className="text-[10px] text-red-400 flex items-center gap-1 truncate max-w-[160px]" title={hd.last_failure_reason}>
                                                    <AlertTriangle className="w-3 h-3 shrink-0" /> {hd.last_failure_reason}
                                                </span>
                                            )}
                                        </div>
                                    </td>

                                    {/* Info badges */}
                                    <td className="px-4 py-3">
                                        <div className="flex items-center flex-wrap gap-1.5">
                                            {acc.token_preview && (
                                                <span className="font-mono bg-muted px-1.5 py-0.5 rounded text-[10px]">
                                                    {acc.token_preview}
                                                </span>
                                            )}
                                            {sessionCounts && sessionCounts[id] !== undefined && (
                                                <span className="font-mono bg-blue-500/10 text-blue-500 px-1.5 py-0.5 rounded text-[10px]">
                                                    {t('accountManager.sessionCount', { count: sessionCounts[id] })}
                                                </span>
                                            )}
                                            {sessionCounts && sessionCounts[id] !== undefined && sessionCounts[id] > 0 && (
                                                <button
                                                    onClick={() => onDeleteAllSessions(id)}
                                                    disabled={deletingSessions && deletingSessions[id]}
                                                    className="flex items-center gap-1 font-mono bg-red-500/10 text-red-500 hover:bg-red-500/20 px-1.5 py-0.5 rounded text-[10px] transition-colors disabled:opacity-50"
                                                    title={t('accountManager.deleteAllSessions')}
                                                >
                                                    {deletingSessions && deletingSessions[id]
                                                        ? <span className="animate-spin">⟳</span>
                                                        : <FolderX className="w-3 h-3" />
                                                    }
                                                </button>
                                            )}
                                            {acc.proxy_id && (
                                                <span className="font-mono bg-amber-500/10 text-amber-500 px-1.5 py-0.5 rounded text-[10px]">
                                                    {t('accountManager.proxyBadge', { name: assignedProxy ? (assignedProxy.name || `${assignedProxy.host}:${assignedProxy.port}`) : acc.proxy_id })}
                                                </span>
                                            )}
                                        </div>
                                    </td>

                                    {/* Actions */}
                                    <td className="px-4 py-3">
                                        <div className="flex items-center gap-1.5 justify-end">
                                            <select
                                                value={acc.proxy_id || ''}
                                                onChange={e => onUpdateAccountProxy(id, e.target.value)}
                                                disabled={updatingProxy?.[id]}
                                                className="max-w-[140px] px-2 py-1 text-[10px] bg-secondary border border-border rounded-md focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                                            >
                                                <option value="">{t('accountManager.proxyNone')}</option>
                                                {proxies.map(proxy => (
                                                    <option key={proxy.id} value={proxy.id}>
                                                        {proxy.name || `${proxy.host}:${proxy.port}`}
                                                    </option>
                                                ))}
                                            </select>
                                            <button
                                                onClick={() => onEditAccount(acc)}
                                                disabled={!id}
                                                className="p-1.5 text-muted-foreground hover:text-primary hover:bg-primary/10 rounded-md transition-colors disabled:opacity-40"
                                                title={id ? t('accountManager.editAccountTitle') : t('accountManager.invalidIdentifier')}
                                            >
                                                <Pencil className="w-3.5 h-3.5" />
                                            </button>
                                            <button
                                                onClick={() => onTestAccount(id)}
                                                disabled={testing[id]}
                                                className="px-2 py-1 text-[10px] font-medium border border-border rounded-md hover:bg-secondary transition-colors disabled:opacity-50"
                                            >
                                                {testing[id] ? t('actions.testing') : t('actions.test')}
                                            </button>
                                            <button
                                                onClick={() => onDeleteAccount(id)}
                                                className="p-1.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-md transition-colors"
                                            >
                                                <Trash2 className="w-3.5 h-3.5" />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            )
                        })}
                    </tbody>
                </table>
            </div>

            {totalPages > 1 && (
                <div className="p-4 border-t border-border flex items-center justify-between">
                    <div className="flex items-center gap-3">
                        <div className="text-sm text-muted-foreground">
                            {t('accountManager.pageInfo', { current: page, total: totalPages, count: totalAccounts })}
                        </div>
                        <select
                            value={pageSize}
                            onChange={e => onPageSizeChange(Number(e.target.value))}
                            className="text-sm border border-border rounded-md px-2 py-1 bg-background text-foreground"
                        >
                            {[10, 20, 50, 100, 500, 1000, 2000, 5000].map(s => (
                                <option key={s} value={s}>{s}</option>
                            ))}
                        </select>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            onClick={onPrevPage}
                            disabled={page <= 1 || loadingAccounts}
                            className="p-2 border border-border rounded-md hover:bg-secondary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            <ChevronLeft className="w-4 h-4" />
                        </button>
                        <span className="text-sm font-medium px-2">{page} / {totalPages}</span>
                        <button
                            onClick={onNextPage}
                            disabled={page >= totalPages || loadingAccounts}
                            className="p-2 border border-border rounded-md hover:bg-secondary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            <ChevronRight className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            )}
        </div>
    )
}
