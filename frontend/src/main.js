import {Events} from "@wailsio/runtime";
import {App} from "../bindings/netchecker/internal/app";

let running = false;
let cfg = null;
let toastTimer = null;
let dirty = false;
const list = document.getElementById('targetsList');

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
function escapeHtml(s) {
    return String(s ?? '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", "&#39;");
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
    const statusEl = document.getElementById("exportCsvStatus");
    if (!btn) return;

    btn.addEventListener("click", async () => {
        try {
            btn.disabled = true;
            if (statusEl) statusEl.textContent = "Exporting...";

            // Вызов Go-метода (Wails bindings)
            const savedPath = await App.ExportAllToCSVGZWithDialog();

            if (!savedPath) {
                if (statusEl) statusEl.textContent = "Cancelled";
                return;
            }

            if (statusEl) statusEl.textContent = `Saved: ${savedPath}`;
        } catch (e) {
            console.error(e);
            if (statusEl) statusEl.textContent = `Export failed: ${e?.message || e}`;
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

});

