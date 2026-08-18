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
    $("status").textContent = `ok · waitline ${m.queue_depth} · hold bin ${m.holdbin} · ${m.go}`;
  } catch (err) {
    $("status").className = "status bad";
    $("status").textContent = String(err);
  }
}

async function refreshCollectors() {
  const data = await api("/api/v1/collectors");
  $("collectors").innerHTML = (data.collectors || []).map((d) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(d.name)}</strong>
        <div class="muted">${escapeHtml(d.id)} · ${escapeHtml(d.url)}</div>
      </div>
      <button data-toggle="${d.id}" data-enabled="${d.enabled}">${d.enabled ? "Disable" : "Enable"}</button>
    </div>
  `).join("");
}

async function refreshRunlog() {
  const data = await api("/api/v1/runlog");
  $("runlog").innerHTML = (data.entries || []).map((e) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(e.kind)}</strong> ${e.status || ""} ${escapeHtml(e.type || "")}
        <div class="muted">${escapeHtml(e.forward_id)} · attempt ${e.attempt} · ${escapeHtml(e.note || e.error || "")}</div>
      </div>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshHold() {
  const data = await api("/api/v1/holdbin");
  $("holdbin").innerHTML = (data.items || []).map((it) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(it.reason)}</strong>
        <div class="muted">${escapeHtml(it.forward_id)}</div>
      </div>
      <button data-reissue="${it.forward_id}">Reissue</button>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshEcho() {
  const data = await api("/api/v1/echo/recent");
  $("echo").innerHTML = (data.received || []).map((r) => `
    <div class="row">
      <div>
        <strong>${escapeHtml(r.forward_id || "")}</strong>
        <div class="muted">${escapeHtml(JSON.stringify(r.body))}</div>
      </div>
    </div>
  `).join("") || '<div class="muted">empty</div>';
}

async function refreshCatalog() {
  const [tools, steps, programs] = await Promise.all([
    api("/api/v1/tools"),
    api("/api/v1/steps"),
    api("/api/v1/programs")
  ]);
  const t = (tools.tools || []).map((x) => `${x.id} ${x.state}`).join(", ");
  const s = (steps.steps || []).map((x) => x.id).join(", ");
  const p = (programs.process_programs || []).filter((x) => x.status === "qualified").map((x) => x.id).join(", ");
  $("catalog").innerHTML = `
    <div class="muted">tools: ${escapeHtml(t)}</div>
    <div class="muted">steps: ${escapeHtml(s)}</div>
    <div class="muted">process programs: ${escapeHtml(p)}</div>
  `;
}

async function refreshAll() {
  await refreshMeta();
  await Promise.all([refreshCollectors(), refreshRunlog(), refreshHold(), refreshEcho(), refreshCatalog()]);
}

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
}

$("col-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const prefixes = String(fd.get("kind_prefixes") || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  await api("/api/v1/collectors", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: fd.get("name"),
      url: fd.get("url"),
      secret: fd.get("secret"),
      kind_prefixes: prefixes,
      ordered: false,
      rate: 5,
      burst: 5
    })
  });
  ev.target.reset();
  await refreshCollectors();
});

$("event-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const fd = new FormData(ev.target);
  const payloadText = String(fd.get("payload") || "{}");
  let payload;
  try { payload = JSON.parse(payloadText); }
  catch (err) { $("event-result").textContent = "payload json: " + err; return; }
  const bodyObj = { type: fd.get("type"), payload };
  const body = JSON.stringify(bodyObj);
  const ts = Math.floor(Date.now() / 1000);
  const n = nonce();
  const canonical = `v1.${ts}.${n}.${await sha256Hex(body)}`;
  const sig = "v1=" + await hmacHex("dev-tool-secret", canonical);
  try {
    const res = await api("/api/v1/lots", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Wafer-Timestamp": String(ts),
        "X-Wafer-Nonce": n,
        "X-Wafer-Signature": sig,
        "Idempotency-Key": idemKey(),
        "X-Wafer-Tool-Key": "tool"
      },
      body
    });
    $("event-result").textContent = JSON.stringify(res, null, 2);
    setTimeout(refreshAll, 400);
  } catch (err) {
    $("event-result").textContent = String(err);
  }
});

$("refresh").addEventListener("click", refreshAll);

document.body.addEventListener("click", async (ev) => {
  const t = ev.target;
  if (!(t instanceof HTMLElement)) return;
  if (t.dataset.toggle) {
    const enabled = t.dataset.enabled !== "true";
    await api(`/api/v1/collectors/${t.dataset.toggle}/enable`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled })
    });
    await refreshCollectors();
  }
  if (t.dataset.reissue) {
    await api(`/api/v1/reissue/${t.dataset.reissue}`, { method: "POST" });
    await refreshAll();
  }
});

refreshAll();
setInterval(refreshMeta, 3000);
