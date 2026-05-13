import { useEffect, useState, useCallback, useRef } from 'react'
import { useI18n } from '../../i18n'
import { RefreshCcw, Trash2, Loader2, Search, X, Filter, ChevronDown } from 'lucide-react'

const RANGE_PRESETS = [
    { id: '1h', label: '1H', ms: 3600000 },
    { id: '6h', label: '6H', ms: 21600000 },
    { id: '24h', label: '24H', ms: 86400000 },
    { id: '7d', label: '7D', ms: 604800000 },
    { id: '30d', label: '30D', ms: 2592000000 },
    { id: 'custom', label: 'Custom', ms: 0 },
]

const SURFACE_OPTIONS = ['', 'openai', 'responses', 'claude', 'gemini']

function toLocalDatetime(ts) {
    const d = new Date(ts)
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

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

    const [rangePreset, setRangePreset] = useState('24h')
    const [customFrom, setCustomFrom] = useState(() => toLocalDatetime(Date.now() - 86400000))
    const [customTo, setCustomTo] = useState(() => toLocalDatetime(Date.now()))
    const [filterCaller, setFilterCaller] = useState('')
    const [filterModel, setFilterModel] = useState('')
    const [filterSurface, setFilterSurface] = useState('')
    const [showFilters, setShowFilters] = useState(false)

    const debounceRef = useRef(null)

    const getTimeRange = useCallback(() => {
        if (rangePreset === 'custom') {
            return {
                from: new Date(customFrom).getTime() || (Date.now() - 86400000),
                to: new Date(customTo).getTime() || Date.now(),
            }
        }
        const preset = RANGE_PRESETS.find(p => p.id === rangePreset)
        const to = Date.now()
        return { from: to - (preset?.ms || 86400000), to }
    }, [rangePreset, customFrom, customTo])

    const activeFilterCount = [filterCaller, filterModel, filterSurface].filter(Boolean).length

    const loadData = useCallback(async ({ manual = false } = {}) => {
        if (manual) setRefreshing(true)
        else if (!logItems.length) setLoading(true)
        try {
            const { from, to } = getTimeRange()
            const base = `from=${from}&to=${to}`
            const extra = [
                filterCaller && `caller=${encodeURIComponent(filterCaller)}`,
                filterModel && `model=${encodeURIComponent(filterModel)}`,
                filterSurface && `surface=${encodeURIComponent(filterSurface)}`,
            ].filter(Boolean).join('&')
            const qs = extra ? `${base}&${extra}` : base
            const [logRes, sumRes, callerRes] = await Promise.all([
                apiFetch(`/admin/usage/log?${qs}&page=${page}&limit=50`),
                apiFetch(`/admin/usage/summary?${base}`),
                apiFetch(`/admin/usage/caller-summary?${base}`),
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
    }, [apiFetch, page, getTimeRange, filterCaller, filterModel, filterSurface, logItems.length, onMessage, t])

    useEffect(() => { loadData() }, [loadData])

    const debouncedReload = useCallback(() => {
        clearTimeout(debounceRef.current)
        debounceRef.current = setTimeout(() => {
            setPage(1)
            loadData({ manual: true })
        }, 400)
    }, [loadData])

    const handlePresetChange = (presetId) => {
        setRangePreset(presetId)
        setPage(1)
    }

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

    const clearAllFilters = () => {
        setFilterCaller('')
        setFilterModel('')
        setFilterSurface('')
        setRangePreset('24h')
        setPage(1)
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
        <div className="space-y-4">
            {/* Top bar: tabs + actions */}
            <div className="flex items-center justify-between flex-wrap gap-3">
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
                <div className="flex gap-2 items-center">
                    <button
                        onClick={() => setShowFilters(v => !v)}
                        className={`h-9 px-3 rounded-lg border text-sm font-medium flex items-center gap-1.5 transition-colors ${
                            showFilters || activeFilterCount > 0
                                ? 'border-primary bg-primary/10 text-primary'
                                : 'border-border bg-card text-muted-foreground hover:text-foreground'
                        }`}
                    >
                        <Filter className="w-3.5 h-3.5" />
                        Filters
                        {activeFilterCount > 0 && (
                            <span className="ml-1 w-5 h-5 rounded-full bg-primary text-primary-foreground text-[10px] font-bold flex items-center justify-center">
                                {activeFilterCount}
                            </span>
                        )}
                    </button>
                    <button
                        onClick={() => { setPage(1); loadData({ manual: true }) }}
                        disabled={refreshing}
                        className="h-9 w-9 rounded-lg border border-border bg-card text-muted-foreground hover:text-foreground flex items-center justify-center"
                        title="Refresh"
                    >
                        {refreshing ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCcw className="w-4 h-4" />}
                    </button>
                    <button
                        onClick={handleClear}
                        className="h-9 w-9 rounded-lg border border-border bg-card text-muted-foreground hover:text-destructive flex items-center justify-center"
                        title="Clear all"
                    >
                        <Trash2 className="w-4 h-4" />
                    </button>
                </div>
            </div>

            {/* Time range presets — always visible */}
            <div className="flex items-center gap-2 flex-wrap">
                <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider mr-1">Range:</span>
                {RANGE_PRESETS.map(p => (
                    <button
                        key={p.id}
                        onClick={() => handlePresetChange(p.id)}
                        className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                            rangePreset === p.id
                                ? 'bg-primary text-primary-foreground shadow-sm'
                                : 'bg-card border border-border text-muted-foreground hover:text-foreground hover:border-foreground/20'
                        }`}
                    >
                        {p.label}
                    </button>
                ))}
                {rangePreset === 'custom' && (
                    <div className="flex items-center gap-2 ml-2">
                        <input
                            type="datetime-local"
                            value={customFrom}
                            onChange={e => { setCustomFrom(e.target.value); debouncedReload() }}
                            className="px-2 py-1 text-xs bg-card border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring text-foreground"
                        />
                        <span className="text-xs text-muted-foreground">to</span>
                        <input
                            type="datetime-local"
                            value={customTo}
                            onChange={e => { setCustomTo(e.target.value); debouncedReload() }}
                            className="px-2 py-1 text-xs bg-card border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring text-foreground"
                        />
                    </div>
                )}
            </div>

            {/* Advanced filters panel */}
            {showFilters && (
                <div className="bg-card border border-border rounded-xl p-4 space-y-3 animate-in fade-in slide-in-from-top-2 duration-200">
                    <div className="flex items-center justify-between">
                        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-widest">Advanced Filters</span>
                        {activeFilterCount > 0 && (
                            <button
                                onClick={clearAllFilters}
                                className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors"
                            >
                                <X className="w-3 h-3" /> Clear all
                            </button>
                        )}
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                        <div className="space-y-1.5">
                            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Caller / API Key</label>
                            <div className="relative">
                                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                                <input
                                    type="text"
                                    value={filterCaller}
                                    onChange={e => { setFilterCaller(e.target.value); debouncedReload() }}
                                    placeholder="e.g. sk-ds2api-..."
                                    className="w-full pl-8 pr-3 py-2 text-xs bg-background border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring placeholder:text-muted-foreground/40"
                                />
                                {filterCaller && (
                                    <button onClick={() => { setFilterCaller(''); debouncedReload() }}
                                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                                        <X className="w-3 h-3" />
                                    </button>
                                )}
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Model</label>
                            <div className="relative">
                                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                                <input
                                    type="text"
                                    value={filterModel}
                                    onChange={e => { setFilterModel(e.target.value); debouncedReload() }}
                                    placeholder="e.g. deepseek-v4-flash"
                                    className="w-full pl-8 pr-3 py-2 text-xs bg-background border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring placeholder:text-muted-foreground/40"
                                />
                                {filterModel && (
                                    <button onClick={() => { setFilterModel(''); debouncedReload() }}
                                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                                        <X className="w-3 h-3" />
                                    </button>
                                )}
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Surface</label>
                            <div className="relative">
                                <select
                                    value={filterSurface}
                                    onChange={e => { setFilterSurface(e.target.value); debouncedReload() }}
                                    className="w-full px-3 py-2 text-xs bg-background border border-border rounded-lg focus:outline-none focus:ring-1 focus:ring-ring appearance-none text-foreground"
                                >
                                    <option value="">All surfaces</option>
                                    {SURFACE_OPTIONS.filter(Boolean).map(s => (
                                        <option key={s} value={s}>{s}</option>
                                    ))}
                                </select>
                                <ChevronDown className="absolute right-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground pointer-events-none" />
                            </div>
                        </div>
                    </div>
                </div>
            )}

            {/* Log tab */}
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
                                            <td className="px-4 py-3 font-mono text-xs max-w-[160px] truncate" title={entry.caller_id}>
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

            {/* Hourly tab */}
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

            {/* Caller tab */}
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
