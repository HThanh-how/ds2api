package monitor

import (
	"time"
)

var globalAlerter *Alerter

func SetAlerter(a *Alerter) {
	globalAlerter = a
}

func GetAlerter() *Alerter {
	return globalAlerter
}

var startTime = time.Now()

func UptimeSeconds() int64 {
	return int64(time.Since(startTime).Seconds())
}

func OnRequestComplete(surface, model string, statusCode int, elapsedMs int64, promptTokens, completionTokens, reasoningTokens int, retryCount int) {
	RecordRequest(surface, model, statusCode, elapsedMs, promptTokens, completionTokens, reasoningTokens)
	for i := 0; i < retryCount; i++ {
		RecordEmptyRetry()
	}
}

func OnEmptyRetry() {
	RecordEmptyRetry()
}

func OnAccountSwitchRetry() {
	RecordAccountSwitchRetry()
}

func OnAccountPoolChange(inUse, available, waiting, total int) {
	RecordAccountPool(inUse, available, waiting, total)
}

func OnAccountHealthChange(account string, consecutiveFailures, maxConsecutive int) {
	if a := GetAlerter(); a != nil {
		if consecutiveFailures >= maxConsecutive {
			a.AlertConsecutiveFailures(account, consecutiveFailures, maxConsecutive)
		}
	}
}

func OnAllAccountsDown() {
	if a := GetAlerter(); a != nil {
		a.AlertAllAccountsDown()
	}
}

func OnAccountRecovered(account string) {
	if a := GetAlerter(); a != nil {
		a.AlertAccountRecovered(account)
	}
}

func OnSessionCreationFailure(account, errMsg string) {
	if a := GetAlerter(); a != nil {
		a.AlertSessionCreationFailure(account, errMsg)
	}
}

func OnPowFailure(account string) {
	if a := GetAlerter(); a != nil {
		a.AlertPowFailure(account)
	}
}

func OnContentFilterBlock(account string) {
	if a := GetAlerter(); a != nil {
		a.AlertContentFilterBlock(account)
	}
}

func OnTokenRefreshFailure(account, errMsg string) {
	if a := GetAlerter(); a != nil {
		a.AlertTokenRefreshFailure(account, errMsg)
	}
}

func OnHighErrorRate(rate float64, windowSec int, threshold float64) {
	if a := GetAlerter(); a != nil {
		a.AlertHighErrorRate(rate, windowSec, threshold)
	}
}

type ErrorRateTracker struct {
	window     []bool
	idx        int
	size       int
	errorCount int
	totalCount int
}

func NewErrorRateTracker(windowSize int) *ErrorRateTracker {
	return &ErrorRateTracker{
		window: make([]bool, windowSize),
		size:   windowSize,
	}
}

func (t *ErrorRateTracker) Record(isError bool) {
	if t.size == 0 {
		return
	}
	oldIsError := t.window[t.idx]
	t.window[t.idx] = isError
	t.idx = (t.idx + 1) % t.size
	if oldIsError {
		t.errorCount--
	}
	if isError {
		t.errorCount++
	}
	t.totalCount++
	if t.totalCount > t.size {
		t.totalCount = t.size
	}
}

func (t *ErrorRateTracker) ErrorRate() float64 {
	if t.totalCount == 0 {
		return 0
	}
	return float64(t.errorCount) / float64(t.totalCount)
}
