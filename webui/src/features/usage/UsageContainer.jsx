import { useEffect, useState, useCallback } from 'react'
import { useI18n } from '../../i18n'
import { RefreshCcw, Trash2, Loader2 } from 'lucide-react'

export default function UsageContainer({ authFetch, onMessage }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch
    const [activeTab, setActiveTab] = useState('log')
    const [logItems, setLogItems] = useState([])
    const [summaryItems, setSummaryItems] = useState([])
    const [callerItems, setCallerItems] = useState([])
    const [totalCount, setTotalCount] = useState(0)
    const [loading, setLoading] = useState(true)
    const [refreshing, setRefreshing] = useState(false)
    const [page, setPage] = useState(1)
    const [totalPages, setTotalPages] = useState(1)

    const loadData = useCallback(async ({ manual = false } = {}) => {
        if (manual) setRefreshing(true)
        else if (!logItems.length) setLoading(true)
        try {
            const to = Date.now()
            const from = to - 86400000 // 24h
            const params = `?from=${from}&to=${to}&page=${page}&limit=50`
            const [logRes, sumRes, callerRes] = await Promise.all([
                apiFetch(`/admin/usage/log${params}`),
                apiFetch(`/admin/usage/summary?from=${from}&to=${to}`),
                apiFetch(`/admin/usage/caller-summary?from=${from}&to=${to}`),
            ])
            const logData = await logRes.json()
            const sumData = await sumRes.json()
            const callerData = await callerRes.json()
            if (logRes.ok) {
                setLogItems(logData.items || [])
                setTotalCount(logData.total || 0)
                if (logData.total && logData.limit) {
                    setTotalPages(Math.ceil(logData.total / logData.limit))
                }
            }
            if (sumRes.ok) setSummaryItems(sumData.items || [])
            if (callerRes.ok) setCallerItems(callerData.items || [])
        } catch (_e) {
            onMessage?.('error', t('usage.loadFailed'))
        } finally {
            setLoading(false)
            setRefreshing(false)
        }
    }, [apiFetch, page, logItems.length, onMessage, t])

    useEffect(() => { loadData() }, [loadData])

    const handleClear = async () => {
        if (!confirm(t('usage.clearConfirm'))) return
        try {
            const res = await apiFetch('/admin/usage/log', { method: 'DELETE' })
            if (res.ok) {
                setLogItems([])
                setSummaryItems([])
                setCallerItems([])
                setTotalCount(0)
                onMessage?.('success', t('usage.cleared'))
            }
        } catch (_e) {
            onMessage?.('error', t('usage.clearFailed'))
        }
    }

    if (loading) {
        return (
            <div className="bg-card border border-border rounded-xl p-12 text-center">
                <Loader2 className="w-8 h-8 animate-spin mx-auto mb-4 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">{t('actions.loading')}</p>
            </div>
        )
    }

    const tabs = [
        { id: 'log', label: t('usage.logTab') },
        { id: 'hourly', label: t('usage.hourlyTab') },
        { id: 'caller', label: t('usage.callerTab') },
    ]

    const formatCost = (cost) => {
        if (cost == null || cost === 0) return '$0.00'
        if (cost < 0.01) return '<$0.01'
        return `$${cost.toFixed(4)}`
    }

    const formatTokens = (n) => {
        if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
        if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`
        return String(n)
    }

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <div className="flex gap-2">
                    {tabs.map(tab => (
                        <button
                            key={tab.id}
                            onClick={() => setActiveTab(tab.id)}
                            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${activeTab === tab.id
                                    ? 'bg-primary text-primary-foreground'
                                    : 'bg-card border border-border text-muted-foreground hover:text-foreground'
                                }`}
                        >
                            {tab.label}
                        </button>
                    ))}
                </div>
                <div className="flex gap-2">
                    <button
                        onClick={() => { setPage(1); loadData({ manual: true }) }}
                        disabled={refreshing}
                        className="h-9 w-9 rounded-lg border border-border bg-card text-muted-foreground hover:text-foreground flex items-center justify-center"
                        title={t('chatHistory.refresh')}
                    >
                        {refreshing ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCcw className="w-4 h-4" />}
                    </button>
                    <button
                        onClick={handleClear}
                        className="h-9 w-9 rounded-lg border border-border bg-card text-muted-foreground hover:text-destructive flex items-center justify-center"
                        title={t('chatHistory.clearAll')}
                    >
                        <Trash2 className="w-4 h-4" />
                    </button>
                </div>
            </div>

            {activeTab === 'log' && (
                <div className="bg-card border border-border rounded-xl overflow-hidden">
                    <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                            <thead className="border-b border-border bg-background/50">
                                <tr>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.time')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.caller')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.model')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.tokens')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.cost')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.latency')}</th>
                                    <th className="text-center px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.status')}</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {logItems.length === 0 ? (
                                    <tr>
                                        <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">
                                            {t('usage.noData')}
                                        </td>
                                    </tr>
                                ) : (
                                    logItems.map((entry) => (
                                        <tr key={entry.id} className="hover:bg-background/50">
                                            <td className="px-4 py-3 text-xs text-muted-foreground whitespace-nowrap">
                                                {new Date(entry.created_at).toLocaleTimeString()}
                                            </td>
                                            <td className="px-4 py-3 font-mono text-xs">
                                                {entry.caller_id || '—'}
                                            </td>
                                            <td className="px-4 py-3 text-xs">{entry.model || '—'}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">
                                                <span className="text-foreground">{formatTokens(entry.prompt_tokens)}</span>
                                                <span className="text-muted-foreground"> / </span>
                                                <span className="text-foreground">{formatTokens(entry.output_tokens)}</span>
                                            </td>
                                            <td className="px-4 py-3 text-xs text-right font-mono text-emerald-500">
                                                {formatCost(entry.total_cost)}
                                            </td>
                                            <td className="px-4 py-3 text-xs text-right text-muted-foreground">
                                                {entry.elapsed_ms}ms
                                            </td>
                                            <td className="px-4 py-3 text-center">
                                                <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${entry.status_code < 400
                                                        ? 'bg-emerald-500/10 text-emerald-500'
                                                        : 'bg-destructive/10 text-destructive'
                                                    }`}>
                                                    {entry.status_code}
                                                </span>
                                            </td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                    {totalPages > 1 && (
                        <div className="flex items-center justify-between px-4 py-3 border-t border-border">
                            <span className="text-xs text-muted-foreground">
                                {t('accountManager.pageInfo', { current: page, total: totalPages, count: totalCount })}
                            </span>
                            <div className="flex gap-1">
                                <button
                                    onClick={() => setPage(p => Math.max(1, p - 1))}
                                    disabled={page <= 1}
                                    className="px-3 py-1 text-xs rounded border border-border bg-background hover:bg-secondary disabled:opacity-50"
                                >
                                    ←
                                </button>
                                <button
                                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                                    disabled={page >= totalPages}
                                    className="px-3 py-1 text-xs rounded border border-border bg-background hover:bg-secondary disabled:opacity-50"
                                >
                                    →
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            )}

            {activeTab === 'hourly' && (
                <div className="bg-card border border-border rounded-xl overflow-hidden">
                    <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                            <thead className="border-b border-border bg-background/50">
                                <tr>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.hour')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.requests')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.tokensIn')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.tokensOut')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.cost')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.errors')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.avgLatency')}</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {summaryItems.length === 0 ? (
                                    <tr>
                                        <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">
                                            {t('usage.noData')}
                                        </td>
                                    </tr>
                                ) : (
                                    summaryItems.map((s) => (
                                        <tr key={s.hour} className="hover:bg-background/50">
                                            <td className="px-4 py-3 text-xs text-muted-foreground">{s.hour}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">{s.requests}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">{formatTokens(s.prompt_tokens)}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">{formatTokens(s.output_tokens)}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono text-emerald-500">{formatCost(s.total_cost)}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">
                                                {s.errors > 0 ? <span className="text-destructive">{s.errors}</span> : '0'}
                                            </td>
                                            <td className="px-4 py-3 text-xs text-right text-muted-foreground">{s.avg_latency_ms}ms</td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}

            {activeTab === 'caller' && (
                <div className="bg-card border border-border rounded-xl overflow-hidden">
                    <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                            <thead className="border-b border-border bg-background/50">
                                <tr>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.caller')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.requests')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.tokens')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.cost')}</th>
                                    <th className="text-right px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.errors')}</th>
                                    <th className="text-left px-4 py-3 font-medium text-muted-foreground text-xs">{t('usage.topModel')}</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {callerItems.length === 0 ? (
                                    <tr>
                                        <td colSpan={6} className="px-4 py-12 text-center text-muted-foreground">
                                            {t('usage.noData')}
                                        </td>
                                    </tr>
                                ) : (
                                    callerItems.map((c) => (
                                        <tr key={c.caller_id} className="hover:bg-background/50">
                                            <td className="px-4 py-3 font-mono text-xs">{c.caller_id || '—'}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">{c.requests}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">{formatTokens(c.total_tokens)}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono text-emerald-500">{formatCost(c.total_cost)}</td>
                                            <td className="px-4 py-3 text-xs text-right font-mono">
                                                {c.errors > 0 ? <span className="text-destructive">{c.errors}</span> : '0'}
                                            </td>
                                            <td className="px-4 py-3 text-xs text-muted-foreground">{c.top_model || '—'}</td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                </div>
            )}
        </div>
    )
}
