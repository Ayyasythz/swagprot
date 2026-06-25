"use strict";

const $ = (id) => document.getElementById(id);
// `cache` keeps each method's request, metadata, and last response keyed by
// full method name, so switching methods (and coming back) preserves your work.
const state = { canInvoke: false, defaultMetadata: [], services: [], current: null, cache: {} };

async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

// basePath is the directory the UI is served from (e.g. "/" or "/swagger/").
// API requests use paths relative to the document, so they work at root or
// under a mounted sub-path; only the WebSocket URL needs this explicitly.
function basePath() {
  const p = location.pathname;
  return p.endsWith("/") ? p : p.replace(/[^/]*$/, "");
}

// ---- theme --------------------------------------------------------------
const THEME_KEY = "swagprot-theme";

function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
}

function initTheme() {
  const stored = localStorage.getItem(THEME_KEY);
  const system = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  applyTheme(stored || system);
  $("themeToggle").addEventListener("click", () => {
    const next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
  });
  // Follow system changes only while the user hasn't picked a theme.
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (e) => {
    if (!localStorage.getItem(THEME_KEY)) applyTheme(e.matches ? "dark" : "light");
  });
}

// ---- bootstrap ----------------------------------------------------------
async function init() {
  initTheme();
  try {
    const cfg = await api("api/config");
    state.canInvoke = cfg.canInvoke;
    state.defaultMetadata = cfg.defaultMetadata || [];
    const badge = $("invokeBadge");
    badge.textContent = cfg.canInvoke ? "connected · invoke enabled" : "browse-only";
    badge.classList.toggle("live", cfg.canInvoke);
    state.services = await api("api/services");
    renderTree(state.services);
  } catch (e) {
    $("tree").innerHTML = `<p class="hint" style="padding:10px">Failed to load: ${escapeHtml(e.message)}</p>`;
  }
}

// ---- navigation tree ----------------------------------------------------
function renderTree(services, filter = "") {
  const tree = $("tree");
  tree.innerHTML = "";
  const f = filter.toLowerCase();
  let shown = 0;
  for (const svc of services) {
    const methods = svc.methods.filter(
      (m) => !f || m.fullName.toLowerCase().includes(f) || svc.fullName.toLowerCase().includes(f)
    );
    if (methods.length === 0) continue;
    shown += methods.length;
    const wrap = document.createElement("div");
    wrap.className = "svc";
    const name = document.createElement("div");
    name.className = "svc-name";
    name.textContent = svc.fullName;
    wrap.appendChild(name);
    for (const m of methods) {
      const k = kind(m);
      const el = document.createElement("div");
      el.className = "method";
      el.dataset.method = m.fullName;
      el.innerHTML = `<span class="glyph" title="${kindLabel(k)}">${glyphLetter(k)}</span><span class="mname">${escapeHtml(m.name)}</span>`;
      el.onclick = () => selectMethod(m.fullName, el);
      wrap.appendChild(el);
    }
    tree.appendChild(wrap);
  }
  if (shown === 0) tree.innerHTML = `<p class="hint" style="padding:10px">No methods match “${escapeHtml(filter)}”.</p>`;
}

function kind(m) {
  if (m.clientStreaming && m.serverStreaming) return "bidi";
  if (m.serverStreaming) return "server";
  if (m.clientStreaming) return "client";
  return "unary";
}
function glyphLetter(k) { return { unary: "U", server: "S", client: "C", bidi: "B" }[k]; }
function kindLabel(k) { return { unary: "unary", server: "server stream", client: "client stream", bidi: "bidi stream" }[k]; }

$("filter").addEventListener("input", (e) => renderTree(state.services, e.target.value));

// ---- method panel -------------------------------------------------------
// Snapshot the currently displayed method's editable fields and response so
// they can be restored when the user navigates back to it.
function saveCurrent() {
  if (!state.current) return;
  const statusEl = $("statusLine");
  state.cache[state.current.method.fullName] = {
    request: $("requestBody").value,
    metadata: $("metadata").value,
    responseHTML: $("response").innerHTML,
    statusText: statusEl.textContent,
    statusClass: statusEl.className,
    metaHTML: $("metaOutBody").textContent,
    metaHidden: $("metaOut").hidden,
  };
}

async function selectMethod(fullName, el) {
  if (state.current && state.current.method.fullName === fullName) return;
  saveCurrent();
  document.querySelectorAll(".method.active").forEach((n) => n.classList.remove("active"));
  if (el) el.classList.add("active");
  try {
    const detail = await api("api/method?name=" + encodeURIComponent(fullName));
    state.current = detail;
    showMethod(detail);
  } catch (e) {
    alert("Failed to load method: " + e.message);
  }
}

function showMethod(detail) {
  const m = detail.method;
  const k = kind(m);
  $("empty").hidden = true;
  $("panel").hidden = false;
  $("methodName").textContent = m.fullName;
  $("methodTypes").textContent = `${m.inputType}  →  ${m.outputType}`;

  const chip = $("streamTag");
  chip.textContent = kindLabel(k);
  chip.className = "chip " + k;

  const streaming = m.clientStreaming || m.serverStreaming;
  $("requestHint").innerHTML = streaming
    ? (m.clientStreaming
        ? "Client streaming — separate multiple request messages with a line containing only <code>---</code>."
        : "Server streaming — one request, multiple responses.")
    : "Edit the proto3-JSON request below.";

  renderSchema(detail.request);

  // Restore this method's saved request/response if we've shown it before,
  // otherwise start from a fresh skeleton and empty response.
  const cached = state.cache[m.fullName];
  if (cached) {
    $("requestBody").value = cached.request;
    $("metadata").value = cached.metadata;
    $("response").innerHTML = cached.responseHTML;
    $("statusLine").textContent = cached.statusText;
    $("statusLine").className = cached.statusClass;
    $("metaOutBody").textContent = cached.metaHTML;
    $("metaOut").hidden = cached.metaHidden;
  } else {
    $("requestBody").value = skeleton(detail.request.fields);
    $("metadata").value = state.defaultMetadata.join("\n");
    resetResponse();
  }

  $("invokeBtn").disabled = !state.canInvoke;
  $("invokeBtn").textContent = streaming ? "Invoke stream" : "Invoke";
  $("invokeBtn").title = state.canInvoke ? "" : "No target address configured (browse-only)";
  $("invokeBtn").onclick = () => (streaming ? invokeStream(m) : invokeUnary(m));
  // Reset clears this method's request back to a fresh skeleton.
  $("resetBtn").onclick = () => ($("requestBody").value = skeleton(detail.request.fields));
}

function resetResponse() {
  $("response").innerHTML = '<span class="placeholder">Response will appear here.</span>';
  setStatus("", "");
  $("metaOut").hidden = true;
}

// ---- request skeleton generation ---------------------------------------
function skeleton(fields) {
  return JSON.stringify(buildObject(fields), null, 2);
}

function buildObject(fields) {
  const obj = {};
  const seenOneof = new Set();
  for (const f of fields || []) {
    if (f.oneof) {
      if (seenOneof.has(f.oneof)) continue; // only the first option of each oneof
      seenOneof.add(f.oneof);
    }
    obj[f.jsonName || f.name] = sampleValue(f);
  }
  return obj;
}

function sampleValue(f) {
  if (f.type === "map") return {};
  if (f.repeated) return [scalarOrMessage(f)];
  return scalarOrMessage(f);
}

function scalarOrMessage(f) {
  switch (f.type) {
    case "message":
      if (f.wellKnown) return wellKnownSample(f.wellKnown);
      if (f.recursive || !f.fields) return {};
      return buildObject(f.fields);
    case "enum":
      return (f.enumValues && f.enumValues[0]) || "";
    case "bool":
      return false;
    case "string":
    case "bytes":
      return "";
    case "int32": case "uint32": case "float": case "double":
      return 0;
    case "int64": case "uint64":
      return "0"; // 64-bit ints are strings in proto3 JSON
    default:
      return null;
  }
}

function wellKnownSample(kind) {
  switch (kind) {
    case "timestamp": return "1970-01-01T00:00:00Z";
    case "duration": return "0s";
    case "fieldmask": return "";
    case "empty": return {};
    case "struct": return {};
    case "any": return { "@type": "type.googleapis.com/..." };
    case "wrapper": return null;
    default: return {};
  }
}

// ---- schema view --------------------------------------------------------
function renderSchema(req) {
  $("schemaView").innerHTML = req.fields && req.fields.length
    ? fieldsHtml(req.fields)
    : '<span class="hint">(no fields)</span>';
}

function fieldsHtml(fields) {
  let html = "";
  for (const f of fields || []) {
    const flags = [];
    if (f.repeated) flags.push("repeated");
    if (f.oneof) flags.push("oneof:" + f.oneof);
    if (f.optional) flags.push("optional");
    let typeName = f.type;
    if (f.type === "message") typeName = f.messageType + (f.wellKnown ? " (" + f.wellKnown + ")" : "");
    if (f.type === "enum") typeName = "enum {" + (f.enumValues || []).join(", ") + "}";
    if (f.type === "map") typeName = `map<${f.keyType}, ${f.valueType}>`;
    html += `<div class="field"><span class="fname">${escapeHtml(f.name)}</span>: <span class="ftype">${escapeHtml(typeName)}</span> ${flags.map((x) => `<span class="flag">${escapeHtml(x)}</span>`).join(" ")}</div>`;
    const nested = f.fields || f.valueFields;
    if (nested && nested.length) html += `<div class="nested">${fieldsHtml(nested)}</div>`;
  }
  return html;
}

// ---- invocation ---------------------------------------------------------
function metadataLines() {
  return $("metadata").value.split("\n").map((s) => s.trim()).filter(Boolean);
}

async function invokeUnary(m) {
  setStatus("calling…", "pending");
  $("response").textContent = "";
  try {
    const res = await api("api/invoke", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method: m.fullName, request: $("requestBody").value, metadata: metadataLines() }),
    });
    showStatus(res.status);
    $("response").textContent = res.response || "(empty response)";
    showMeta(res.headers, res.trailers);
  } catch (e) {
    setStatus("error", "err");
    $("response").textContent = e.message;
  }
}

function appendFrame(out, n, body) {
  const div = document.createElement("div");
  div.className = "frame";
  div.innerHTML = `<span class="frame-num">// message #${n}</span>\n`;
  div.appendChild(document.createTextNode(body));
  out.appendChild(div);
  out.scrollTop = out.scrollHeight;
}

function streamRequests(m) {
  return m.clientStreaming
    ? $("requestBody").value.split(/^---$/m).map((s) => s.trim()).filter(Boolean)
    : [$("requestBody").value];
}

function invokeStream(m) {
  setStatus("streaming…", "pending");
  const out = $("response");
  out.textContent = "";
  const requests = streamRequests(m);
  let count = 0;
  let done = false; // a terminal status/error arrived over the socket

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  let ws;
  try {
    ws = new WebSocket(`${proto}//${location.host}${basePath()}api/stream`);
  } catch {
    streamBuffered(m, requests);
    return;
  }
  ws.onopen = () => ws.send(JSON.stringify({ method: m.fullName, requests, metadata: metadataLines() }));
  ws.onmessage = (ev) => {
    const e = JSON.parse(ev.data);
    if (e.type === "message") {
      appendFrame(out, ++count, e.data);
    } else if (e.type === "status") {
      done = true;
      showStatus(e.status, ` · ${count} message(s)`);
      showMeta(e.headers, e.trailers);
    } else if (e.type === "error") {
      done = true;
      setStatus("error", "err");
      out.appendChild(document.createTextNode(e.error));
    }
  };
  // If the socket never delivered a terminal frame (e.g. it couldn't upgrade
  // behind a fasthttp/Fiber adaptor or a buffering proxy), fall back to the
  // buffered HTTP endpoint so "try it out" still works.
  ws.onerror = () => { if (!done) { try { ws.close(); } catch {} } };
  ws.onclose = () => { if (!done) streamBuffered(m, requests); };
}

async function streamBuffered(m, requests) {
  setStatus("streaming (buffered)…", "pending");
  const out = $("response");
  out.textContent = "";
  try {
    const res = await api("api/invoke-stream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method: m.fullName, requests, metadata: metadataLines() }),
    });
    const msgs = res.messages || [];
    msgs.forEach((body, i) => appendFrame(out, i + 1, body));
    if (msgs.length === 0) out.innerHTML = '<span class="placeholder">(no messages)</span>';
    showStatus(res.status, ` · ${msgs.length} message(s) · buffered`);
    showMeta(res.headers, res.trailers);
  } catch (e) {
    setStatus("error", "err");
    out.textContent = e.message;
  }
}

function setStatus(text, cls) {
  const el = $("statusLine");
  el.textContent = text;
  el.className = "status" + (cls ? " " + cls : "");
}

function showStatus(status, suffix = "") {
  if (!status) return;
  const ok = status.code === "OK";
  setStatus(`${status.code} (${status.number})${status.message ? ": " + status.message : ""}${suffix}`, ok ? "ok" : "err");
}

function showMeta(headers, trailers) {
  const parts = [];
  if (headers && Object.keys(headers).length) parts.push("headers:\n" + dump(headers));
  if (trailers && Object.keys(trailers).length) parts.push("trailers:\n" + dump(trailers));
  if (parts.length) {
    $("metaOutBody").textContent = parts.join("\n\n");
    $("metaOut").hidden = false;
  } else {
    $("metaOut").hidden = true;
  }
}

function dump(obj) {
  return Object.entries(obj).map(([k, v]) => `  ${k}: ${v}`).join("\n");
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}

init();
