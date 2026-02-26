import {Events} from "@wailsio/runtime";
import {App} from "../bindings/netchecker/internal/app";

let running = false;
let cfg = null;
let toastTimer = null;
let dirty = false;
const list = document.getElementById('targetsList');

// ===== Dashboard state =====
let dashTimer = null;
let lastBucketMs = 0;
const WINDOW_MS = 30 * 60 * 1000;
const POLL_MS = 10 * 1000;

// In-memory series: Map<name, Array<{t, rtt, errRate}>>
const seriesByName = new Map();
const enabled = new Map(); // name -> boolean

// ===== Helpers =====
function fmtMs(x) {
    if (!isFinite(x)) return "—";
    return `${Math.round(x)} ms`;
}

function fmtPct(x) {
    if (!isFinite(x)) return "—";
    return `${x.toFixed(1)}%`;
}

function timeHHMMSS(ms) {
    const d = new Date(ms);
    return d.toLocaleTimeString([], {hour: "2-digit", minute: "2-digit", second: "2-digit"});
}

function stateClass(summaryRow) {
    // Simple, cheap rules for UX:
    // - DOWN => bad
    // - loss >= 2% => warn, >= 5% => bad
    // - else ok
    if (!summaryRow.last_ok) return "state-bad";
    if (summaryRow.loss_pct >= 5) return "state-bad";
    if (summaryRow.loss_pct >= 2) return "state-warn";
    return "state-ok";
}

// ===== DOM refs =====
const dashCardsEl = () => document.getElementById("dash_cards");
const dashLegendEl = () => document.getElementById("dash_legend");
const dashLastUpdateEl = () => document.getElementById("dash_last_update");
const dashCanvas = () => document.getElementById("dash_chart");
const btnDashRefresh = () => document.getElementById("btnDashRefresh");

// ===== Init Dashboard =====
export function initDashboard() {
    // Button
    const btn = btnDashRefresh();
    if (btn) btn.addEventListener("click", () => dashPoll(true));

    // Resize chart
    window.addEventListener("resize", () => drawChart());

    // Initial load + start interval
    dashPoll(true);
    if (dashTimer) clearInterval(dashTimer);
    dashTimer = setInterval(() => dashPoll(false), POLL_MS);
}

// ===== Poll =====
async function dashPoll(fullReload) {
    try {
        if (fullReload) {
            lastBucketMs = 0;
            seriesByName.clear();
            enabled.clear();
            renderLegend(); // empty
            drawChart();
        }

        // Call backend
        const resp = await App.DashboardPoll(lastBucketMs);
        if (!resp) return;

        // Update "last update"
        if (dashLastUpdateEl()) {
            dashLastUpdateEl().textContent = `updated: ${timeHHMMSS(resp.now_ms)}`;
        }

        // Render summary cards
        renderCards(resp.summary);

        // Merge series
        mergeSeries(resp.series, resp.now_ms);

        // Update lastBucket
        if (resp.last_bucket_ms && resp.last_bucket_ms > lastBucketMs) {
            lastBucketMs = resp.last_bucket_ms;
        }

        // Render legend once we know names
        renderLegend();

        // Draw chart
        drawChart();
    } catch (e) {
        console.error("DashboardPoll failed:", e);
    }
}

// ===== Cards =====
function renderCards(summary) {
    const root = dashCardsEl();
    if (!root) return;

    if (!Array.isArray(summary) || summary.length === 0) {
        root.innerHTML = `<div class="muted">No data for last 30 minutes</div>`;
        return;
    }

    // Ensure enabled defaults
    for (const r of summary) {
        if (!enabled.has(r.name)) enabled.set(r.name, true);
    }

    root.innerHTML = summary.map(r => {
        const cls = stateClass(r);
        const statusText = r.last_ok ? "UP" : "DOWN";
        const lastSeen = r.last_ok_ts_ms ? timeHHMMSS(r.last_ok_ts_ms) : "—";

        return `
      <div class="card-tile ${cls}">
        <div class="card-top">
          <div class="card-name" title="${escapeHtml(r.name)}">${escapeHtml(r.name)}</div>
          <div class="card-status">
            <span class="badge">${statusText}</span>
          </div>
        </div>

        <div class="muted" style="font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${escapeHtml(r.address)}">
          ${escapeHtml(r.address)}
        </div>

        <div class="card-kpis">
          <div class="kpi">
            <div class="k">Loss</div>
            <div class="v">${fmtPct(r.loss_pct)}</div>
          </div>
          <div class="kpi">
            <div class="k">Avg RTT</div>
            <div class="v">${fmtMs(r.avg_rtt_ms)}</div>
          </div>
          <div class="kpi">
            <div class="k">Max RTT</div>
            <div class="v">${fmtMs(r.max_rtt_ms)}</div>
          </div>
          <div class="kpi">
            <div class="k">Last OK</div>
            <div class="v">${lastSeen}</div>
          </div>
        </div>

        <div class="muted" style="font-size:12px; display:flex; gap:10px; justify-content:space-between;">
          <span>Samples: <b>${r.total}</b></span>
          <span>Err: <b>${r.errors}</b></span>
        </div>
      </div>
    `;
    }).join("");
}

function escapeHtml(s) {
    return String(s ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}

// ===== Series merge (bucketed 1s points) =====
function mergeSeries(points, nowMs) {
    if (!Array.isArray(points) || points.length === 0) return;

    const cutoff = nowMs - WINDOW_MS;

    for (const p of points) {
        // Ensure series list
        if (!seriesByName.has(p.name)) seriesByName.set(p.name, []);
        if (!enabled.has(p.name)) enabled.set(p.name, true);

        const arr = seriesByName.get(p.name);
        const errRate = p.total > 0 ? (p.errors / p.total) : 0;

        // store avg RTT + err rate for bucket
        arr.push({t: p.bucket_ms, rtt: p.avg_rtt_ms, err: errRate});
    }

    // Trim old
    for (const [name, arr] of seriesByName.entries()) {
        let i = 0;
        while (i < arr.length && arr[i].t < cutoff) i++;
        if (i > 0) arr.splice(0, i);

        // Also de-duplicate buckets if backend ever repeats a bucket
        // (keep last occurrence)
        const dedup = new Map();
        for (const pt of arr) dedup.set(pt.t, pt);
        const merged = Array.from(dedup.values()).sort((a, b) => a.t - b.t);
        seriesByName.set(name, merged);
    }
}

// ===== Legend (toggle series) =====
function renderLegend() {
    const root = dashLegendEl();
    if (!root) return;

    const names = Array.from(seriesByName.keys()).sort();
    if (names.length === 0) {
        root.innerHTML = "";
        return;
    }

    root.innerHTML = names.map((name, idx) => {
        const on = enabled.get(name) !== false;
        return `
      <div class="legend-item ${on ? "" : "off"}" data-name="${escapeHtml(name)}">
        <span class="legend-dot" style="opacity:.8"></span>
        ${escapeHtml(name)}
      </div>
    `;
    }).join("");

    // Click handlers (event delegation)
    root.onclick = (e) => {
        const el = e.target.closest(".legend-item");
        if (!el) return;
        const name = el.getAttribute("data-name");
        enabled.set(name, !(enabled.get(name) !== false));
        renderLegend();
        drawChart();
    };
}

// ===== Chart drawing (simple canvas) =====
// Note: no external libs -> minimal overhead.
function drawChart() {
    const canvas = dashCanvas();
    if (!canvas) return;

    const parent = canvas.parentElement;
    const w = parent.clientWidth;
    const h = parent.clientHeight;
    if (w <= 10 || h <= 10) return;

    // HiDPI
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(h * dpr);
    canvas.style.width = `${w}px`;
    canvas.style.height = `${h}px`;

    const ctx = canvas.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    // Background
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = "rgba(0,0,0,0.12)";
    ctx.fillRect(0, 0, w, h);

    // Gather enabled series points
    const now = Date.now();
    const from = now - WINDOW_MS;

    const enabledNames = Array.from(seriesByName.keys()).filter(n => enabled.get(n) !== false);
    if (enabledNames.length === 0) return;

    // Determine max RTT (for scale)
    let maxRtt = 10;
    for (const name of enabledNames) {
        const arr = seriesByName.get(name) || [];
        for (const pt of arr) {
            if (pt.t >= from && pt.rtt > maxRtt) maxRtt = pt.rtt;
        }
    }
    // Add headroom
    maxRtt = Math.max(10, Math.round(maxRtt * 1.2));

    // Grid lines (very light)
    ctx.strokeStyle = "rgba(255,255,255,0.06)";
    ctx.lineWidth = 1;
    const gridY = 4;
    for (let i = 0; i <= gridY; i++) {
        const y = Math.round((h - 1) * (i / gridY));
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(w, y);
        ctx.stroke();
    }

    // Draw series (each with different lightness — simple)
    enabledNames.forEach((name, idx) => {
        const arr = seriesByName.get(name) || [];
        if (arr.length < 2) return;

        // A simple color scheme using alpha variations (no heavy palette)
        const base = 0.28 + (idx % 6) * 0.08;
        ctx.strokeStyle = `rgba(24,177,154,${Math.min(0.85, base + 0.25)})`;
        ctx.lineWidth = 1.5;

        let started = false;
        ctx.beginPath();

        for (const pt of arr) {
            const t = pt.t;
            if (t < from) continue;

            const x = (t - from) / WINDOW_MS * w;
            const y = h - (pt.rtt / maxRtt) * h;

            // Break line when errors dominate this bucket
            if (pt.err >= 0.5) {
                // draw a small marker for loss bucket
                ctx.fillStyle = "rgba(255,95,86,0.55)";
                ctx.fillRect(x - 1, y - 1, 3, 3);
                started = false;
                continue;
            }

            if (!started) {
                ctx.moveTo(x, y);
                started = true;
            } else {
                ctx.lineTo(x, y);
            }
        }
        ctx.stroke();
    });

    // Axis labels (minimal)
    ctx.fillStyle = "rgba(255,255,255,0.55)";
    ctx.font = "12px -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Arial";
    ctx.fillText(`0 ms`, 8, h - 8);
    ctx.fillText(`${maxRtt} ms`, 8, 14);
}
list?.addEventListener('blur', (e) => {
    const cell = e.target;
    if (!cell?.dataset?.field) return;

    const row = cell.closest?.('[data-kind="target"]');
    if (!row) return;

    const idx = Number(row.dataset.idx);
    if (!Number.isFinite(idx)) return;

    const field = cell.dataset.field;
    const text = (cell.textContent || '').trim();

    if (field === 'name') {
        cfg.targets[idx].name = text;
    } else if (field === 'address') {
        cfg.targets[idx].address = text;

        const res = validateAddress(text);
        if (cfg.targets[idx].enabled && !res.ok) row.classList.add('row-error');
        else row.classList.remove('row-error');
    } else {
        return;
    }

    setDirty(true);
}, true);
list.addEventListener('change', (e) => {

    const input = e.target;
    if (input.tagName !== 'INPUT') return;
    if (input.id === 'gw_on'){
        cfg.gateway = cfg.gateway || {};
        cfg.gateway.enabled = !!e.target.checked;
        setDirty(true);
    }
    else{
        const row = input.closest('[data-kind="target"]');
        if (!row) return;

        const idx = Number(row.dataset.idx);
        console.log(idx)
        console.log(cfg.targets[idx])

        if (!Number.isFinite(idx)) return;

        if (input.id.startsWith('t_on_')) {
            cfg.targets[idx].enabled = input.checked;
        }

        if (input.id.startsWith('t_trace_')) {
            cfg.targets[idx].traceEnabled = input.checked;
        }

        setDirty(true);
    }

});

function setActiveStatus(status) {
    let btnText = status ? "Stop" : "Start";
    let StsText = status ? "Running" : "Stopped";
    let StsStatus = status ? "ok" : "muted";
    document.getElementById("btnStartStop").innerText = btnText;
    document.getElementById("app_sts_text").innerText = StsText;
    document.getElementById("app_sts_run").classList.remove('ok', 'muted')
    document.getElementById("app_sts_run").classList.add(StsStatus)
}

document.getElementById('btnStartStop').addEventListener('click', async () => {

    if (!running)
        await App.Start();
    else
        await App.Stop()
})
document.getElementById('btnSave')?.addEventListener('click', async () => {
    if (!App) return;

    const errs = validateConfigForSave();
    if (errs.length) {
        toast(`Fix settings: ${errs[0]}`);
        console.log(errs.join('\n'))
        return;
    }
    console.log('Saving...');
    const ok = await App.SaveConfig(cfg);
    if (ok) {
        console.log('Saved');
        setDirty(false);
        toast('Saved');
        cfg = await App.GetConfig();

    } else {
        console.log('Save failed');
        toast('Save failed');
    }
});
function toast(msg) {
    clearTimeout(toastTimer);
    let el = document.getElementById('toast');
    if (!el) {
        el = document.createElement('div');
        el.id = 'toast';
        el.className = 'toast';
        document.body.appendChild(el);
    }
    el.textContent = msg;
    el.style.display = 'block';
    toastTimer = setTimeout(() => {
        el.style.display = 'none';
    }, 2200);
}
function setDirty(v) {
    dirty = !!v;
    console.log(dirty)

    const dot = document.getElementById('dirtyDot');
    if (dot) dot.style.display = dirty ? 'inline-block' : 'none';
}

function setFieldsEnabled(containerId, enabled) {
    const box = document.getElementById(containerId);
    if (!box) return;
    box.querySelectorAll('input, select, textarea, button').forEach(el => {
        el.disabled = !enabled;
    });
    box.querySelectorAll('.form-row').forEach(r => {
        if (!enabled) r.classList.add('disabled');
        else r.classList.remove('disabled');
    });
}
function validateConfigForSave() {
    document.querySelectorAll('[data-kind="target"].row-error').forEach(el => el.classList.remove('row-error'));

    const errors = [];
    const targets = cfg?.targets || [];

    targets.forEach((t, idx) => {
        if (!t?.enabled) return;

        const res = validateAddress(t.address);
        if (!res.ok) {
            let msg = `Target #${idx + 1}: invalid address`;
            if (res.reason === 'empty') msg = `Target #${idx + 1}: address is empty`;
            if (res.reason === 'protocol') msg = `Target #${idx + 1}: remove "http://..."`;
            if (res.reason === 'bad_chars') msg = `Target #${idx + 1}: no spaces or "/" allowed`;
            if (res.reason === 'too_long') msg = `Target #${idx + 1}: address too long`;

            errors.push(msg);

            const row = document.querySelector(`[data-kind="target"][data-idx="${idx}"]`);
            row?.classList.add('row-error');
        }
    });

    const p = cfg.ping || {};
    if (p.intervalMs < 250) errors.push('Ping: interval must be >= 250ms');
    if (p.timeoutMs < 200 || p.timeoutMs > 10000) errors.push('Ping: timeout must be 200..10000ms');
    if (p.payload < 0 || p.payload > 1472) errors.push('Ping: payload must be 0..1472 bytes');

    const tr = cfg.trace || {};
    if (tr.cooldownSec < 0 || tr.cooldownSec > 3600) errors.push('Trace: cooldown must be 0..3600 sec');

    const loss = tr.loss || {};
    if (loss.enabled) {
        if (loss.percent < 1 || loss.percent > 100) errors.push('Trace Loss: percent must be 1..100');
        if (loss.lastN < 1 || loss.lastN > 200) errors.push('Trace Loss: lastN must be 1..200');
    }

    const high = tr.highRtt || tr.highRTT || {};
    if (high.enabled) {
        if (high.rttMs < 1 || high.rttMs > 60000) errors.push('Trace High RTT: rttMs must be 1..60000');
        if (high.percent < 1 || high.percent > 100) errors.push('Trace High RTT: percent must be 1..100');
        if (high.lastN < 1 || high.lastN > 200) errors.push('Trace High RTT: lastN must be 1..200');
    }
    return errors;
}

function validateAddress(addrRaw) {
    const addr = (addrRaw || '').trim();
    if (!addr) return { ok: false, reason: 'empty' };
    if (addr.length > 253) return { ok: false, reason: 'too_long' };
    if (/[\/\s]/.test(addr)) return { ok: false, reason: 'bad_chars' };
    if (addr.includes('://')) return { ok: false, reason: 'protocol' };

    if (isValidIPv4(addr)) return { ok: true };
    if (isValidHostname(addr)) return { ok: true };

    return { ok: false, reason: 'format' };
}
function isValidIPv4(s) {
    const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(s);
    if (!m) return false;
    for (let i = 1; i <= 4; i++) {
        const n = Number(m[i]);
        if (!Number.isInteger(n) || n < 0 || n > 255) return false;
    }
    return true;
}

// relaxed hostname validation (labels 1..63, total <=253, no leading/trailing dash)
function isValidHostname(s) {
    if (s.length < 1 || s.length > 253) return false;
    if (s.includes('..')) return false;
    if (/[\/\s]/.test(s)) return false;
    if (s.includes('://')) return false;

    if (s.endsWith('.')) s = s.slice(0, -1);
    if (!s) return false;

    const labels = s.split('.');
    for (const label of labels) {
        if (!label.length || label.length > 63) return false;
        if (label.startsWith('-') || label.endsWith('-')) return false;
        if (!/^[a-zA-Z0-9_-]+$/.test(label)) return false;
    }
    return true;
}

function toggleHTML({checked, disabled, id}) {
    return `
    <label class="toggle small">
      <input id="${id}" type="checkbox" ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''}/>
      <span class="slider"></span>
    </label>
  `;
}


function renderTargetsRows() {
    const targets = cfg?.targets || [];
    const parts = [];



    parts.push(`
    <div class="trow trow-compact trow-flex" data-kind="gateway">
      <div class="center">
        ${toggleHTML({checked: !!cfg?.gateway?.enabled, disabled: false, id: 'gw_on'})}
      </div>
      <div class="center">
        ${toggleHTML({checked: false, disabled: true, id: 'gw_trace'})}
      </div>
      <div class="cell editable" contenteditable="false" style="opacity:.6;">gateway</div>
      <div class="cell editable" contenteditable="false" style="opacity:.6;">(auto)</div>
      <div class="center">
        <button class="icon-btn danger" title="Delete" disabled>✕</button>
      </div>
    </div>
  `);

    targets.forEach((t, idx) => {
        const nameText = (t.name ?? '').trim();
        const addrText = (t.address ?? '').trim();
        parts.push(`
      <div class="trow trow-compact trow-flex" data-kind="target" data-idx="${idx}">
        <div class="center">
          ${toggleHTML({checked: !!t.enabled, disabled: false, id: `t_on_${idx}`})}
        </div>
        <div class="center">
          ${toggleHTML({checked: !!t.traceEnabled, disabled: false, id: `t_trace_${idx}`})}
        </div>

        <div class="cell editable" contenteditable="true"
             data-field="name"
             spellcheck="false"
             title="Edit name">${escapeHtml(nameText)}</div>

        <div class="cell editable" contenteditable="true"
             data-field="address"
             spellcheck="false"
             title="Edit address/hostname">${escapeHtml(addrText)}</div>

        <div class="center">
          <button class="icon-btn danger" title="Delete" data-action="delete" data-idx="${idx}">✕</button>
        </div>
      </div>
    `);
    });
    document.getElementById("targetsList").innerHTML = parts.join('');
}


document.getElementById('addTarget').addEventListener('click', async () => {
    cfg.targets = cfg.targets || [];

    if (cfg.targets.length >= 20) {
        toast('Max 20 targets');
        return;
    }
    cfg.targets.push({ enabled: true, traceEnabled: false, name: '', address: '' });
    renderTargetsRows();
})
document.getElementById('targetsList').addEventListener('click', (e) => {
    const btn = e.target?.closest?.('[data-action="delete"]');
    if (!btn) return;
    const idx = Number(btn.dataset.idx);
    if (!Number.isFinite(idx)) return;
    cfg.targets.splice(idx, 1);
    renderTargetsRows();
});


Events.On("app:size", (ev) => {

    document.getElementById("db_size").innerText = "Size: "  + ev.data;

});

Events.On("app:running", (ev) => {
    running = ev.data;
    setActiveStatus(running);
});


document.getElementById('loss_enabled')?.addEventListener('change', (e) => {
    const en = !!e.target.checked;
    cfg.trace.loss.enabled = en;
    setFieldsEnabled('loss_fields', en);
    setDirty(true);
});

document.getElementById('rtt_enabled')?.addEventListener('change', (e) => {
    const en = !!e.target.checked;
    cfg.trace.highRtt.enabled = en;
    setFieldsEnabled('rtt_fields', en);
    setDirty(true);
});

const num = (v) => {
    const n = parseInt(String(v ?? ''), 10);
    return Number.isFinite(n) ? n : 0;
};
const bindNum = (id, setter) => {
    const el = document.getElementById(id);

    el?.addEventListener('input', () => {
        setter(num(el.value));
        setDirty(true);
    });
};

const bindBool = (id, setter) => {
    const el = document.getElementById(id);
    el?.addEventListener('change', () => {
        setter(!!el.checked);
        setDirty(true);
    });
};
function setValue(id, v) {
    const el = document.getElementById(id);
    if (el) el.value = String(v ?? '');
}
function initExportCSV() {
    const btn = document.getElementById("btnExportCsv");

    if (!btn) return;

    btn.addEventListener("click", async () => {
        try {
            btn.disabled = true;
            toast("Exporting...");

            // Вызов Go-метода (Wails bindings)
            const savedPath = await App.ExportAllToCSVGZWithDialog();

            if (!savedPath) {
                toast("Cancelled");
                return;
            }

            toast`Saved: ${savedPath}`;
        } catch (e) {
            console.error(e);
            toast(`Export failed: ${e?.message || e}`);
        } finally {
            btn.disabled = false;
        }
    });
}

window.addEventListener("DOMContentLoaded", async (event) => {
    running = await App.IsRunning();
    cfg = await  App.GetConfig()
    setFieldsEnabled('loss_fields', !!cfg.trace.loss.enabled);
    setFieldsEnabled('rtt_fields', !!cfg.trace.highRtt.enabled);




    setValue('ping_interval', cfg?.ping?.intervalMs);
    setValue('ping_timeout',  cfg?.ping?.timeoutMs);
    setValue('ping_payload',  cfg?.ping?.payload);

    // trace
    const traceOnStart = document.getElementById('trace_onstart');
    if (traceOnStart) traceOnStart.checked = !!cfg?.trace?.onStart;

    setValue('trace_cooldown', cfg?.trace?.cooldownSec);

    // loss
    const lossEnabled = document.getElementById('loss_enabled');
    if (lossEnabled) lossEnabled.checked = !!cfg?.trace?.loss?.enabled;

    setValue('loss_percent', cfg?.trace?.loss?.percent);
    setValue('loss_lastn',   cfg?.trace?.loss?.lastN);

    // high rtt
    const rttEnabled = document.getElementById('rtt_enabled');
    if (rttEnabled) rttEnabled.checked = !!cfg?.trace?.highRtt?.enabled;

    setValue('rtt_threshold', cfg?.trace?.highRtt?.rttMs);
    setValue('rtt_percent',   cfg?.trace?.highRtt?.percent);
    setValue('rtt_lastn',     cfg?.trace?.highRtt?.lastN);

    // теперь можно включать/выключать поля
    setFieldsEnabled('loss_fields', !!cfg?.trace?.loss?.enabled);
    setFieldsEnabled('rtt_fields',  !!cfg?.trace?.highRtt?.enabled);

    bindNum('ping_interval', (n) => cfg.ping.intervalMs = n);
    bindNum('ping_timeout', (n) => cfg.ping.timeoutMs = n);
    bindNum('ping_payload', (n) => cfg.ping.payload = n);

    // Trace global

    bindBool('trace_onstart', (b) => cfg.trace.onStart = b);
    bindNum('trace_cooldown', (n) => cfg.trace.cooldownSec = n);

    // Loss trigger
    bindNum('loss_percent', (n) => cfg.trace.loss.percent = n);
    bindNum('loss_lastn', (n) => cfg.trace.loss.lastN = n);

    // High RTT trigger
    bindNum('rtt_threshold', (n) => cfg.trace.highRtt.rttMs = n);
    bindNum('rtt_percent', (n) => cfg.trace.highRtt.percent = n);
    bindNum('rtt_lastn', (n) => cfg.trace.highRtt.lastN = n);
    setActiveStatus(running);
    renderTargetsRows()
    initExportCSV();
    initDashboard();

});

