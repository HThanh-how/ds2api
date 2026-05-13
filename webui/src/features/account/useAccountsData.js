import { useEffect, useState } from 'react'

export function useAccountsData({ apiFetch }) {
    const [queueStatus, setQueueStatus] = useState(null)
    const [keysExpanded, setKeysExpanded] = useState(false)

    const [accounts, setAccounts] = useState([])
    const [page, setPage] = useState(1)
    const [pageSize, setPageSize] = useState(10)
    const [totalPages, setTotalPages] = useState(1)
    const [totalAccounts, setTotalAccounts] = useState(0)
    const [loadingAccounts, setLoadingAccounts] = useState(false)

    const resolveAccountIdentifier = (acc) => {
        if (!acc || typeof acc !== 'object') return ''
        return String(acc.identifier || acc.email || acc.mobile || '').trim()
    }

    const [searchQuery, setSearchQuery] = useState('')

    const accountRowId = (row) => {
        if (!row || typeof row !== 'object') return ''
        return String(row.identifier || row.email || row.mobile || '').trim()
    }

    /**
     * @param {number} [targetPage]
     * @param {number} [targetPageSize]
     * @param {string} [targetQuery]
     * @param {Record<string, unknown>|null} [ensureItem] Row from POST /admin/accounts merged in if missing (stale GET / replicas).
     */
    const fetchAccounts = async (targetPage = page, targetPageSize = pageSize, targetQuery = searchQuery, ensureItem = null) => {
        setLoadingAccounts(true)
        try {
            let url = `/admin/accounts?page=${targetPage}&page_size=${targetPageSize}&_ts=${Date.now()}`
            if (targetQuery.trim()) url += `&q=${encodeURIComponent(targetQuery.trim())}`
            const res = await apiFetch(url)
            if (res.ok) {
                const data = await res.json()
                const cap = Number(data.page_size) || targetPageSize
                let items = [...(data.items || [])]
                const want = accountRowId(ensureItem)
                let mergedMissing = false
                if (want) {
                    const has = items.some((a) => accountRowId(a) === want)
                    if (!has) {
                        mergedMissing = true
                        items = [ensureItem, ...items.filter((a) => accountRowId(a) !== want)]
                        if (items.length > cap) {
                            items = items.slice(0, cap)
                        }
                    }
                }
                setAccounts(items)
                setTotalPages(data.total_pages || 1)
                const baseTotal = Number(data.total) || 0
                setTotalAccounts(mergedMissing ? Math.max(baseTotal, items.length) : baseTotal)
                setPage(data.page || 1)
            }
        } catch (e) {
            console.error('Failed to fetch accounts:', e)
        } finally {
            setLoadingAccounts(false)
        }
    }

    const changePageSize = (newSize) => {
        setPageSize(newSize)
        fetchAccounts(1, newSize)
    }

    const handleSearchChange = (query) => {
        setSearchQuery(query)
        fetchAccounts(1, pageSize, query)
    }

    /** After add, clear search and reload page 1; pass `item` from POST body if list GET is briefly stale. */
    const fetchAccountsFirstPageClearSearch = async (ensureItem = null) => {
        setSearchQuery('')
        await fetchAccounts(1, pageSize, '', ensureItem)
    }

    const fetchQueueStatus = async () => {
        try {
            const res = await apiFetch('/admin/queue/status')
            if (res.ok) {
                const data = await res.json()
                setQueueStatus(data)
            }
        } catch (e) {
            console.error('Failed to fetch queue status:', e)
        }
    }

    useEffect(() => {
        fetchAccounts()
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 5000)
        return () => clearInterval(interval)
    }, [])

    return {
        queueStatus,
        keysExpanded,
        setKeysExpanded,
        accounts,
        page,
        pageSize,
        totalPages,
        totalAccounts,
        loadingAccounts,
        fetchAccounts,
        changePageSize,
        resolveAccountIdentifier,
        searchQuery,
        handleSearchChange,
        fetchAccountsFirstPageClearSearch,
    }
}
