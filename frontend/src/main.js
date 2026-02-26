import {Events} from "@wailsio/runtime";
import {App} from "../bindings/netchecker/internal/app";

let running = false;
let cfg = null;
let toastTimer = null;

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
    console.log("Starting app");
    if (!running)
        await App.Start();
    else
        await App.Stop()
})
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
    console.log("mytargerts")
    console.log(targets)


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
        console.log(parts)
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
    console.log("mytargerts3")
    console.log(parts.join(''))
    document.getElementById("targetsList").innerHTML = parts.join('');
}


document.getElementById('addTarget').addEventListener('click', async () => {
    cfg.targets = cfg.targets || [];
    console.log(cfg.targets)
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
    console.log("Size:", ev.data);
    document.getElementById("db_size").innerText = "Size: "  + ev.data;

});

Events.On("app:running", (ev) => {
    running = ev.data;
    setActiveStatus(running);
});

window.addEventListener("DOMContentLoaded", async (event) => {
    running = await App.IsRunning();
    cfg = await  App.GetConfig()
    setActiveStatus(running);
    renderTargetsRows()
});

