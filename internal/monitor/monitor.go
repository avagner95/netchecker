package monitor

import (
	"context"
	"database/sql"
	"github.com/jackpal/gateway"
	"netchecker/internal/config"
	"netchecker/internal/logging"
	"netchecker/internal/storage"
	"sync"
	"sync/atomic"
	"time"
)

func (s *targetState) push(sample ringSample) {
	if len(s.buf) == 0 {
		return
	}
	s.buf[s.next] = sample
	s.next++
	if s.next >= len(s.buf) {
		s.next = 0
	}
	if s.count < len(s.buf) {
		s.count++
	}
}

func (s *targetState) window(n int) []ringSample {
	if n <= 0 || s.count == 0 {
		return nil
	}
	if n > s.count {
		n = s.count
	}
	out := make([]ringSample, 0, n)
	idx := s.next - 1
	for i := 0; i < n; i++ {
		if idx < 0 {
			idx = len(s.buf) - 1
		}
		out = append(out, s.buf[idx])
		idx--
	}
	return out
}

func NewMonitor(store *storage.SQLiteStore, cfg config.Config) *Monitor {
	return &Monitor{
		store:        store,
		cfg:          cfg,
		states:       make(map[string]*targetState),
		traceQ:       make(chan traceJob, 64),
		pendingTrace: make(map[string]struct{}),
		traceWorkers: 2,
	}
}

func (m *Monitor) UpdateConfig(cfg config.Config) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	logging.Info("monitor", "config updated targets=%d traceOnStart=%t", len(cfg.Targets), cfg.Trace.OnStart)
}

func (m *Monitor) Status() Status {
	return Status{
		TraceWorkers:       m.traceWorkers,
		ActiveTraceWorkers: atomic.LoadInt64(&m.activeTrace),
		TraceQueueSize:     len(m.traceQ),
	}
}

func (m *Monitor) Run(ctx context.Context) error {
	if err := m.store.CleanupOlderThan(ctx, 24*time.Hour); err != nil {
		logging.Error("storage", "cleanup at start failed: %v", err)
	}

	logging.Info("monitor", "start trace workers=%d", m.traceWorkers)
	for i := 0; i < m.traceWorkers; i++ {
		go m.traceWorker(ctx, i+1)
	}

	logging.Info("monitor", "monitor loop started")

	for {
		select {
		case <-ctx.Done():
			logging.Info("monitor", "monitor loop stopped")
			return nil
		default:
		}

		cycleStart := time.Now()
		cfg := m.getConfigSnapshot()

		// retention occasionally
		if cycleStart.Minute()%30 == 0 && cycleStart.Second() < 2 {
			if err := m.store.CleanupOlderThan(ctx, 24*time.Hour); err != nil {
				logging.Error("storage", "cleanup failed: %v", err)
			} else {
				logging.Debug("storage", "cleanup ok retention=24h")
			}
		}

		m.ensureStatesAndEnqueueStartTraces(cfg)
		m.tickPing(ctx, cfg)

		interval := time.Duration(cfg.Ping.IntervalMs) * time.Millisecond
		if interval <= 0 {
			interval = 1 * time.Second
		}
		elapsed := time.Since(cycleStart)
		if sleep := interval - elapsed; sleep > 0 {
			select {
			case <-ctx.Done():
				logging.Info("monitor", "monitor loop stopped")
				return nil
			case <-time.After(sleep):
			}
		}
	}
}

// FIXED: no enqueue while holding stMu (prevents self-deadlock).
func (m *Monitor) ensureStatesAndEnqueueStartTraces(cfg config.Config) {
	maxN := maxInt(cfg.Trace.Loss.LastN, cfg.Trace.HighRTT.LastN)
	if maxN <= 0 {
		maxN = 20
	}
	if maxN > 200 {
		maxN = 200
	}

	type startJob struct {
		name    string
		address string
	}
	var toEnqueue []startJob

	m.stMu.Lock()

	// gateway state
	if cfg.Gateway.Enabled {
		if _, ok := m.states["gateway"]; !ok {
			m.states["gateway"] = &targetState{mode: modePING, buf: make([]ringSample, maxN)}
		}
	}

	for _, t := range cfg.Targets {
		if !t.Enabled || t.Address == "" {
			continue
		}
		st, ok := m.states[t.Address]
		if !ok {
			st = &targetState{mode: modePING, buf: make([]ringSample, maxN)}
			m.states[t.Address] = st

			if cfg.Trace.OnStart && t.TraceEnabled {
				st.mode = modePRETRACE
				toEnqueue = append(toEnqueue, startJob{name: pickName(t), address: t.Address})
			}
		} else {
			if len(st.buf) < maxN {
				old := st.window(st.count)
				st.buf = make([]ringSample, maxN)
				st.next, st.count = 0, 0
				for i := len(old) - 1; i >= 0; i-- {
					st.push(old[i])
				}
				logging.Debug("monitor", "resize ring buffer addr=%s newN=%d", t.Address, maxN)
			}
		}
	}

	m.stMu.Unlock()

	for _, j := range toEnqueue {
		logging.Info("monitor", "trace-on-start queued name=%q addr=%s", j.name, j.address)
		m.enqueueTraceIfPossible(j.name, j.address, "start")
	}
}

func (m *Monitor) tickPing(ctx context.Context, cfg config.Config) {
	type pingItem struct {
		name    string
		address string
		isGW    bool
	}
	items := make([]pingItem, 0, 21)

	// gateway
	if cfg.Gateway.Enabled && m.stateMode("gateway") == modePING {
		if m.gwIP == "" {
			if ip, err := gateway.DiscoverGateway(); err == nil && ip != nil {
				m.gwIP = ip.String()
				m.gwFailStreak = 0
				logging.Info("gw", "discovered gateway ip=%s", m.gwIP)
			} else if err != nil {
				m.warnThrottled("gw", "discover gateway failed: %v", err)
			}
		}
		if m.gwIP != "" {
			items = append(items, pingItem{name: "gateway", address: m.gwIP, isGW: true})
		}
	}

	for _, t := range cfg.Targets {
		if !t.Enabled || t.Address == "" {
			continue
		}
		if m.stateMode(t.Address) != modePING {
			continue
		}
		items = append(items, pingItem{name: pickName(t), address: t.Address})
	}

	if len(items) > 20 {
		items = items[:20]
	}

	m.logPingThrottled(cfg.Ping.IntervalMs, len(items), m.gwIP)

	if len(items) == 0 {
		m.logTraceStatusThrottled()
		return
	}

	timeoutMs := cfg.Ping.TimeoutMs
	payload := cfg.Ping.Payload

	// parallel ping
	maxConc := len(items)
	if maxConc > 20 {
		maxConc = 20
	}
	sem := make(chan struct{}, maxConc)

	type res struct {
		item pingItem
		out  PingOut
		ts   int64
	}
	results := make(chan res, len(items))

	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it pingItem) {
			defer wg.Done()
			defer func() { <-sem }()
			out := PingOnce(ctx, it.address, timeoutMs, payload)
			results <- res{item: it, out: out, ts: time.Now().UnixMilli()}
		}(it)
	}

	wg.Wait()
	close(results)

	rows := make([]storage.Row, 0, len(items))

	for r := range results {
		// gateway re-resolve
		if r.item.isGW {
			if r.out.OK {
				m.gwFailStreak = 0
			} else {
				m.gwFailStreak++
				if m.gwFailStreak >= 3 {
					old := m.gwIP
					if ip, err := gateway.DiscoverGateway(); err == nil && ip != nil {
						m.gwIP = ip.String()
						logging.Warn("gw", "re-resolve after 3 fails old=%s new=%s", old, m.gwIP)
					} else if err != nil {
						logging.Error("gw", "re-resolve failed old=%s err=%v", old, err)
					}
					m.gwFailStreak = 0
				}
			}
		}

		rows = append(rows, toRow(r.ts, r.item.name, r.item.address, r.out))

		if !r.item.isGW {
			m.updateStateAndMaybeTriggerTrace(cfg, r.item.name, r.item.address, r.out)
		}
	}

	if err := m.store.InsertBatch(ctx, rows); err != nil {
		logging.Error("storage", "insert ping batch failed: %v", err)
	}

	m.logTraceStatusThrottled()
}

func (m *Monitor) traceWorker(ctx context.Context, workerID int) {
	logging.Info("trace", "worker started id=%d", workerID)

	for {
		select {
		case <-ctx.Done():
			logging.Info("trace", "worker stopped id=%d", workerID)
			return

		case job := <-m.traceQ:
			atomic.AddInt64(&m.activeTrace, 1)
			// left queue; allow future enqueue
			m.unmarkPendingTrace(job.address)

			m.setStateMode(job.address, modeTRACING)

			logging.Info("trace", "start worker=%d name=%q addr=%s reason=%s active=%d queued=%d",
				workerID, job.name, job.address, job.reason,
				atomic.LoadInt64(&m.activeTrace), len(m.traceQ))

			// small event
			if err := m.store.InsertBatch(ctx, []storage.Row{
				makeEventRow(time.Now().UnixMilli(), job.name, job.address, "trace_start:"+job.reason),
			}); err != nil {
				logging.Error("storage", "insert trace_start event failed: %v", err)
			}

			start := time.Now()
			out := TraceOnce(ctx, job.address)
			dur := time.Since(start)

			if out.Err == "timeout" {
				logging.Warn("trace", "trace timeout name=%q addr=%s", job.name, job.address)
			} else if out.Err == "canceled" {
				logging.Warn("trace", "trace canceled name=%q addr=%s", job.name, job.address)
			}

			// store output (bounded again at monitor-level)
			text := out.Text
			if len(text) > 64*1024 {
				text = text[:64*1024] + "\n...truncated..."
			}
			if err := m.store.InsertTrace(ctx, time.Now().UnixMilli(), job.name, job.address, job.reason, out.OK, text); err != nil {
				logging.Error("storage", "insert trace failed: %v", err)
			}

			endEvent := "trace_done"
			if !out.OK {
				endEvent = "trace_fail"
			}
			if err := m.store.InsertBatch(ctx, []storage.Row{
				makeEventRow(time.Now().UnixMilli(), job.name, job.address, endEvent),
			}); err != nil {
				logging.Error("storage", "insert trace end event failed: %v", err)
			}

			logging.Info("trace", "done worker=%d name=%q addr=%s ok=%t dur=%s out_len=%d active=%d queued=%d",
				workerID, job.name, job.address, out.OK, dur.String(), len(text),
				atomic.LoadInt64(&m.activeTrace), len(m.traceQ))

			// cooldown + resume ping
			cfg := m.getConfigSnapshot()
			cd := time.Duration(cfg.Trace.CooldownSec) * time.Second
			if cd <= 0 {
				cd = 5 * time.Minute
			}
			m.setCooldownAndMode(job.address, time.Now().Add(cd), modePING)

			atomic.AddInt64(&m.activeTrace, -1)
			m.logTraceStatusThrottled()
		}
	}
}

func (m *Monitor) updateStateAndMaybeTriggerTrace(cfg config.Config, name, address string, out PingOut) {
	// find target
	var target *config.Target
	for i := range cfg.Targets {
		if cfg.Targets[i].Address == address {
			target = &cfg.Targets[i]
			break
		}
	}
	if target == nil {
		logging.Debug("monitor", "unknown target addr=%s (skipping triggers)", address)
		return
	}

	rtt := 0
	if out.RTTms != nil {
		rtt = *out.RTTms
	}
	m.pushSample(address, ringSample{ok: out.OK, rttMs: rtt})

	if !target.TraceEnabled {
		return
	}
	if m.stateMode(address) != modePING {
		return
	}
	if !m.cooldownPassed(address) {
		return
	}

	if cfg.Trace.Loss.Enabled && cfg.Trace.Loss.LastN > 0 && cfg.Trace.Loss.Percent > 0 {
		lp := lossPercent(m.window(address, cfg.Trace.Loss.LastN))
		if lp >= cfg.Trace.Loss.Percent {
			logging.Info("trace", "trigger loss name=%q addr=%s loss=%d%% window=%d threshold=%d%%",
				name, address, lp, cfg.Trace.Loss.LastN, cfg.Trace.Loss.Percent)
			m.enqueueTraceIfPossible(name, address, "loss")
			return
		}
	}

	if cfg.Trace.HighRTT.Enabled && cfg.Trace.HighRTT.LastN > 0 && cfg.Trace.HighRTT.Percent > 0 && cfg.Trace.HighRTT.RTTms > 0 {
		hp := highRTTPercent(m.window(address, cfg.Trace.HighRTT.LastN), cfg.Trace.HighRTT.RTTms)
		if hp >= cfg.Trace.HighRTT.Percent {
			logging.Info("trace", "trigger high_rtt name=%q addr=%s high=%d%% window=%d rtt>%dms threshold=%d%%",
				name, address, hp, cfg.Trace.HighRTT.LastN, cfg.Trace.HighRTT.RTTms, cfg.Trace.HighRTT.Percent)
			m.enqueueTraceIfPossible(name, address, "high_rtt")
			return
		}
	}
}

func (m *Monitor) enqueueTraceIfPossible(name, address, reason string) {
	if !m.markPendingTrace(address) {
		logging.Debug("trace", "dedup skip name=%q addr=%s reason=%s", name, address, reason)
		return
	}

	m.setStateMode(address, modeTRACING)

	select {
	case m.traceQ <- traceJob{name: name, address: address, reason: reason}:
		logging.Info("trace", "enqueue name=%q addr=%s reason=%s queued=%d active=%d",
			name, address, reason, len(m.traceQ), atomic.LoadInt64(&m.activeTrace))
		return
	default:
		m.unmarkPendingTrace(address)
		m.setStateMode(address, modePING)
		logging.Warn("trace", "queue full rollback name=%q addr=%s reason=%s", name, address, reason)
	}
}

func (m *Monitor) logPingThrottled(intervalMs, targets int, gw string) {
	now := time.Now()
	m.logMu.Lock()
	defer m.logMu.Unlock()
	if now.Sub(m.lastPingLog) < 10*time.Second {
		return
	}
	m.lastPingLog = now
	logging.Info("monitor", "[PING] interval=%dms targets=%d gw=%s", intervalMs, targets, gw)
}

func (m *Monitor) logTraceStatusThrottled() {
	now := time.Now()
	m.logMu.Lock()
	defer m.logMu.Unlock()
	if now.Sub(m.lastTraceLog) < 5*time.Second {
		return
	}
	m.lastTraceLog = now
	st := m.Status()
	logging.Info("monitor", "[TRACE] workers=%d active=%d queued=%d", st.TraceWorkers, st.ActiveTraceWorkers, st.TraceQueueSize)
}

func (m *Monitor) warnThrottled(component, msg string, args ...any) {
	now := time.Now()
	m.logMu.Lock()
	defer m.logMu.Unlock()
	if now.Sub(m.lastWarnLog) < 10*time.Second {
		return
	}
	m.lastWarnLog = now
	logging.Warn(component, msg, args...)
}

func lossPercent(samples []ringSample) int {
	if len(samples) == 0 {
		return 0
	}
	fail := 0
	for _, s := range samples {
		if !s.ok {
			fail++
		}
	}
	return int((100 * fail) / len(samples))
}

func highRTTPercent(samples []ringSample, thresholdMs int) int {
	if len(samples) == 0 {
		return 0
	}
	high := 0
	for _, s := range samples {
		if s.ok && s.rttMs > thresholdMs {
			high++
		}
	}
	return int((100 * high) / len(samples))
}

func toRow(ts int64, name, addr string, out PingOut) storage.Row {
	var ttl sql.NullInt64
	var rtt sql.NullInt64
	var errStr sql.NullString

	if out.TTL != nil {
		ttl = sql.NullInt64{Int64: int64(*out.TTL), Valid: true}
	}
	if out.RTTms != nil {
		rtt = sql.NullInt64{Int64: int64(*out.RTTms), Valid: true}
	}
	if !out.OK && out.Err != "" {
		errStr = sql.NullString{String: out.Err, Valid: true}
	}

	return storage.Row{
		TsMs:  ts,
		Name:  name,
		Addr:  addr,
		TTL:   ttl,
		RTTms: rtt,
		Error: errStr,
	}
}

func makeEventRow(ts int64, name, addr, event string) storage.Row {
	return storage.Row{
		TsMs:  ts,
		Name:  name,
		Addr:  addr,
		TTL:   sql.NullInt64{Valid: false},
		RTTms: sql.NullInt64{Valid: false},
		Error: sql.NullString{String: event, Valid: true},
	}
}

func pickName(t config.Target) string {
	if t.Name != "" {
		return t.Name
	}
	return t.Address
}

func (m *Monitor) getConfigSnapshot() config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// --- trace dedup helpers ---

func (m *Monitor) markPendingTrace(address string) bool {
	m.ptMu.Lock()
	defer m.ptMu.Unlock()
	if _, ok := m.pendingTrace[address]; ok {
		return false
	}
	m.pendingTrace[address] = struct{}{}
	return true
}

func (m *Monitor) unmarkPendingTrace(address string) {
	m.ptMu.Lock()
	delete(m.pendingTrace, address)
	m.ptMu.Unlock()
}

// --- state helpers ---

func (m *Monitor) stateMode(address string) mode {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		return modePING
	}
	return st.mode
}

func (m *Monitor) setStateMode(address string, md mode) {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		st = &targetState{mode: md, buf: make([]ringSample, 20)}
		m.states[address] = st
	} else {
		st.mode = md
	}
}

func (m *Monitor) setCooldownAndMode(address string, until time.Time, md mode) {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		st = &targetState{mode: md, buf: make([]ringSample, 20)}
		m.states[address] = st
	}
	st.cooldownUntil = until
	st.mode = md
}

func (m *Monitor) cooldownPassed(address string) bool {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		return true
	}
	return time.Now().After(st.cooldownUntil)
}

func (m *Monitor) pushSample(address string, s ringSample) {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		st = &targetState{mode: modePING, buf: make([]ringSample, 20)}
		m.states[address] = st
	}
	st.push(s)
}

func (m *Monitor) window(address string, n int) []ringSample {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[address]
	if !ok {
		return nil
	}
	return st.window(n)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
