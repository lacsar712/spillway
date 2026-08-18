const $ = (id) => document.getElementById(id);

async function hmacHex(secret, message) {
  const enc = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    enc.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const buf = await crypto.subtle.sign("HMAC", key, enc.encode(message));
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256Hex(text) {
  const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function nonce() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function idemKey() {
  return "idemp-" + nonce();
}

async function api(path, opts) {
  const res = await fetch(path, opts);
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) {
    throw new Error((data && data.error) || res.statusText);
  }
  return data;
}

async function refreshMeta() {
  try {
    const m = await api("/api/v1/meta");
    $("status").className = "status";
    $("status").textContent = `ok · queue ${m.queue_depth} · dlq ${m.dlq} · ${m.go}`;
  } catch (err) {
    $("status").className = "status bad";
    $("status").textContent = String(err);
  }
}

async function refreshReservoir() {
  const r = await api("/api/v1/reservoir");
  $("reservoir").innerHTML = `
    <div class="row"><div>level <strong>${r.level_m} m</strong></div><div class="muted">crest ${r.crest_m} · flood ${r.flood_m}</div></div>
    <div class="row"><div>inflow ${r.inflow_cms} m³/s</div><div class="muted">outflow ${r.outflow_cms} · tailwater ${r.tailwater_m} m</div></div>
  `;
}

async function refreshPlcs() {
  const data = await api("GET /api/v1/plcs".replace("GET ", ""));
  $("plcs").innerHTML = (data.plcs || []).map((d) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(d.name)}</strong>
        <div class="muted">${escapeHtml(d.id)} · ${escapeHtml(d.url)}</div>
      </div>
      <button data-toggle="${d.id}" data-enabled="${d.enabled}">${d.enabled ? "Disable" : "Enable"}</button>
    </div>
  `).join("");
}

async function refreshJournal() {
  const data = await api("/api/v1/journal");
  $("journal").innerHTML = (data.entries || []).map((e) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(e.kind)}</strong> ${e.status || ""} ${escapeHtml(e.type || "")}
        <div class="muted">${escapeHtml(e.delivery_id)} · attempt ${e.attempt} · ${escapeHtml(e.note || e.error || "")}</div>
      </div>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshDlq() {
  const data = await api("/api/v1/dlq");
  $("dlq").innerHTML = (data.items || []).map((it) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(it.reason)}</strong>
        <div class="muted">${escapeHtml(it.delivery_id)}</div>
      </div>
      <button data-replay="${it.delivery_id}">Replay</button>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshLoop() {
  const data = await api("/api/v1/loopback/recent");
  $("loop").innerHTML = (data.received || []).map((r) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(r.delivery_id || "")}</strong>
        <div class="muted">${escapeHtml(JSON.stringify(r.body))}</div>
      </div>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshAll() {
  await refreshMeta();
  await Promise.all([refreshReservoir(), refreshPlcs(), refreshJournal(), refreshDlq(), refreshLoop()]);
}

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
}

$("plc-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const prefixes = String(fd.get("type_prefixes") || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  await api("/api/v1/plcs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: fd.get("name"),
      url: fd.get("url"),
      secret: fd.get("secret"),
      type_prefixes: prefixes,
      ordered: false,
      rate: 5,
      burst: 5
    })
  });
  ev.target.reset();
  await refreshPlcs();
});

$("cmd-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const payloadText = String(fd.get("payload") || "{}");
  let payload;
  try { payload = JSON.parse(payloadText); }
  catch (err) { $("cmd-result").textContent = "payload json: " + err; return; }
  const bodyObj = { type: fd.get("type"), payload };
  const body = JSON.stringify(bodyObj);
  const ts = Math.floor(Date.now() / 1000);
  const n = nonce();
  const canonical = `v1.${ts}.${n}.${await sha256Hex(body)}`;
  const sig = "v1=" + await hmacHex("dev-ops-secret", canonical);
  try {
    const res = await api("/api/v1/commands", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Spill-Timestamp": String(ts),
        "X-Spill-Nonce": n,
        "X-Spill-Signature": sig,
        "Idempotency-Key": idemKey(),
        "X-Spill-Source-Key": "ops"
      },
      body
    });
    $("cmd-result").textContent = JSON.stringify(res, null, 2);
    setTimeout(refreshAll, 400);
  } catch (err) {
    $("cmd-result").textContent = String(err);
  }
});

$("refresh").addEventListener("click", refreshAll);

document.body.addEventListener("click", async (ev) => {
  const t = ev.target;
  if (!(t instanceof HTMLElement)) return;
  if (t.dataset.toggle) {
    const enabled = t.dataset.enabled !== "true";
    await api(`/api/v1/plcs/${t.dataset.toggle}/enable`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled })
    });
    await refreshPlcs();
  }
  if (t.dataset.replay) {
    await api(`/api/v1/replay/${t.dataset.replay}`, { method: "POST" });
    await refreshAll();
  }
});

refreshAll();
setInterval(refreshMeta, 3000);
