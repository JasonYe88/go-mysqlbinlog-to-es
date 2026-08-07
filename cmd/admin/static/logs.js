const $ = (id) => document.getElementById(id);

function toast(msg, isError = false) {
  const el = $("toast");
  el.textContent = msg;
  el.classList.toggle("error", !!isError);
  el.classList.remove("hidden");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add("hidden"), 4200);
}

function esc(s) {
  return String(s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll('"', "&quot;");
}

async function api(path) {
  const res = await fetch(path);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

async function loadLogs() {
  const source = $("logSource").value;
  const filter = $("logFilter").value.trim();
  const tail = $("logTail").value || "500";
  const qs = new URLSearchParams({ source, tail, filter });
  const data = await api(`/api/logs?${qs.toString()}`);
  const view = $("logView");
  const lines = data.lines || [];
  if (!lines.length) {
    view.textContent = `（暂无日志）source=${data.source || source}`;
    return;
  }
  view.innerHTML = lines
    .map((line) => {
      const lower = String(line).toLowerCase();
      let cls = "";
      if (lower.includes("[error]") || lower.includes(" err ") || lower.includes("error:")) cls = "err";
      else if (lower.includes("[warn]") || lower.includes("warning")) cls = "warn";
      return `<div class="${cls}">${esc(line)}</div>`;
    })
    .join("");
  view.scrollTop = view.scrollHeight;
}

let logTimer = null;
function startLogAutoRefresh() {
  clearInterval(logTimer);
  logTimer = setInterval(() => {
    if ($("logAuto")?.checked) {
      loadLogs().catch(() => {});
    }
  }, 3000);
}

$("btnLoadLogs").onclick = () => loadLogs().catch((e) => toast(e.message, true));
$("logSource").onchange = () => loadLogs().catch((e) => toast(e.message, true));
$("logFilter").onkeydown = (e) => {
  if (e.key === "Enter") loadLogs().catch((err) => toast(err.message, true));
};

loadLogs().catch((e) => toast(e.message, true));
startLogAutoRefresh();
