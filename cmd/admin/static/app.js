let config = null;
let activeRule = 0;
let columnsCache = [];
let editMode = "form";

const $ = (id) => document.getElementById(id);

function toast(msg, isError = false) {
  const el = $("toast");
  el.textContent = msg;
  el.classList.toggle("error", !!isError);
  el.classList.remove("hidden");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add("hidden"), 4200);
}

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function emptyRule() {
  return {
    schema: "",
    table: "",
    index: "",
    type: "_doc",
    parent: "",
    id: [],
    filter: [],
    pipeline: "",
    field: {},
  };
}

function fieldEntries(rule) {
  const field = rule.field || {};
  return Object.keys(field)
    .sort()
    .map((k) => ({ source: k, dest: field[k] }));
}

function collectConnection() {
  config.my_addr = $("my_addr").value.trim();
  config.my_user = $("my_user").value.trim();
  config.my_pass = $("my_pass").value;
  config.server_id = Number($("server_id").value || 0);
  config.es_addr = $("es_addr").value.trim();
  config.es_user = $("es_user").value.trim();
  config.es_pass = $("es_pass").value;
  config.data_dir = $("data_dir").value.trim() || "/app/var";
  if (!config.stat_addr) config.stat_addr = "0.0.0.0:12800";
  if (!config.stat_path) config.stat_path = "/metrics";
  if (!config.flavor) config.flavor = "mysql";
}

function fillConnection() {
  $("my_addr").value = config.my_addr || "";
  $("my_user").value = config.my_user || "";
  $("my_pass").value = config.my_pass || "";
  $("server_id").value = config.server_id || "";
  $("es_addr").value = config.es_addr || "";
  $("es_user").value = config.es_user || "";
  $("es_pass").value = config.es_pass || "";
  $("data_dir").value = config.data_dir || "/app/var";
}

function setEditMode(mode) {
  editMode = mode;
  $("modeForm").classList.toggle("hidden", mode !== "form");
  $("modeRaw").classList.toggle("hidden", mode !== "raw");
  $("tabForm").classList.toggle("active", mode === "form");
  $("tabRaw").classList.toggle("active", mode === "raw");
  if (mode === "raw") {
    loadRawToml().catch((e) => toast(e.message, true));
  }
}

function renderTabs() {
  const box = $("ruleTabs");
  box.innerHTML = "";
  (config.rule || []).forEach((r, i) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "tab" + (i === activeRule ? " active" : "");
    btn.textContent = r.table ? `${r.schema}.${r.table}` : `规则 ${i + 1}`;
    btn.onclick = () => {
      persistCurrentRuleForm();
      activeRule = i;
      renderTabs();
      renderRuleEditor();
    };
    box.appendChild(btn);
  });
}

function persistCurrentRuleForm() {
  const rule = config.rule?.[activeRule];
  if (!rule) return;
  if (!$("rule_schema")) return;
  rule.schema = $("rule_schema")?.value.trim() || "";
  rule.table = $("rule_table")?.value.trim() || "";
  rule.index = $("rule_index")?.value.trim() || "";
  rule.type = $("rule_type")?.value.trim() || "_doc";
  rule.pipeline = $("rule_pipeline")?.value.trim() || "";
  const filterRaw = $("rule_filter")?.value.trim() || "";
  rule.filter = filterRaw ? filterRaw.split(",").map((s) => s.trim()).filter(Boolean) : [];

  const rows = document.querySelectorAll("#mapBody tr");
  const field = {};
  rows.forEach((tr) => {
    const source = tr.querySelector(".src")?.value.trim();
    const dest = tr.querySelector(".dst")?.value.trim();
    const mod = tr.querySelector(".mod")?.value || "";
    if (!source) return;
    let value = dest || "";
    if (mod) value = `${value},${mod}`;
    field[source] = value;
  });
  rule.field = field;
}

/** Parse batch lines like: id = "id"  or  "dataJson.sku" = data.sku */
function parseFieldBatch(text) {
  const merged = {};
  const errors = [];
  const lines = String(text || "").split(/\r?\n/);
  lines.forEach((raw, idx) => {
    let line = raw.trim();
    if (!line || line.startsWith("#") || line.startsWith("//")) return;
    const eq = line.indexOf("=");
    if (eq < 0) {
      errors.push(`第 ${idx + 1} 行：缺少 =`);
      return;
    }
    let key = line.slice(0, eq).trim();
    let val = line.slice(eq + 1).trim();
    key = unquoteToml(key);
    val = unquoteToml(val);
    if (!key) {
      errors.push(`第 ${idx + 1} 行：源字段为空`);
      return;
    }
    merged[key] = val;
  });
  return { merged, errors };
}

function unquoteToml(s) {
  s = String(s || "").trim();
  if (
    (s.startsWith('"') && s.endsWith('"')) ||
    (s.startsWith("'") && s.endsWith("'"))
  ) {
    return s.slice(1, -1);
  }
  return s;
}

function applyFieldBatch() {
  const ta = $("batchField");
  if (!ta) return;
  const { merged, errors } = parseFieldBatch(ta.value);
  if (errors.length) {
    toast(errors.slice(0, 3).join("；") + (errors.length > 3 ? "…" : ""), true);
    return;
  }
  const keys = Object.keys(merged);
  if (!keys.length) {
    toast("没有可应用的映射行", true);
    return;
  }
  persistCurrentRuleForm();
  const rule = config.rule[activeRule];
  if (!rule.field) rule.field = {};
  keys.forEach((k) => {
    rule.field[k] = merged[k];
  });
  renderRuleEditor();
  toast(`已合并 ${keys.length} 条映射到当前规则`);
}

function renderRuleEditor() {
  const rule = config.rule?.[activeRule];
  const box = $("ruleEditor");
  if (!rule) {
    box.innerHTML = `<p class="muted">暂无规则，请点击「新增表规则」。</p>`;
    return;
  }

  const entries = fieldEntries(rule);
  box.innerHTML = `
    <div class="grid-2">
      <label>库 schema<input id="rule_schema" value="${esc(rule.schema || "")}" /></label>
      <label>表 table<input id="rule_table" value="${esc(rule.table || "")}" /></label>
      <label>ES index<input id="rule_index" value="${esc(rule.index || "")}" /></label>
      <label>ES type<input id="rule_type" value="${esc(rule.type || "_doc")}" /></label>
      <label>pipeline（可选）<input id="rule_pipeline" value="${esc(rule.pipeline || "")}" /></label>
      <label>filter（逗号分隔，可选）<input id="rule_filter" value="${esc((rule.filter || []).join(","))}" /></label>
    </div>
    <div class="row-between">
      <h3 style="margin:0;font-size:15px;">字段映射 [rule.field]</h3>
      <div class="inline-actions">
        <button type="button" class="ghost" id="btnAddMap">+ 映射行</button>
        <button type="button" class="danger" id="btnDelRule">删除本规则</button>
      </div>
    </div>
    <p class="hint">普通列：<code>title → es_title</code>；JSON 路径：<code>ext.sku → sku</code> 或 <code>ext.sku → data.sku</code></p>
    <table class="map-table">
      <thead>
        <tr>
          <th style="width:34%">MySQL 源（列或 JSON 路径）</th>
          <th style="width:34%">ES 目标字段</th>
          <th style="width:16%">修饰符</th>
          <th style="width:16%"></th>
        </tr>
      </thead>
      <tbody id="mapBody"></tbody>
    </table>
    <div class="batch-box">
      <h3 style="margin:0 0 8px;font-size:14px;">批量粘贴映射</h3>
      <p class="hint">每行一条，支持 <code>id = "id"</code> 或 <code>"dataJson.sku" = "data.sku"</code>；应用到当前规则（同 key 覆盖）。</p>
      <textarea id="batchField" placeholder='id = "id"&#10;"dataJson.sku" = "data.sku"&#10;"dataJson.skuName" = "skuName"'></textarea>
      <div class="batch-actions">
        <button type="button" class="primary" id="btnApplyBatch">应用到当前规则</button>
      </div>
    </div>
  `;

  const body = $("mapBody");
  const renderRows = (list) => {
    body.innerHTML = "";
    (list.length ? list : [{ source: "", dest: "" }]).forEach((item) => {
      let dest = item.dest || "";
      let mod = "";
      const parts = dest.split(",");
      if (parts.length >= 2) {
        dest = parts[0];
        mod = parts[1];
      }
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td><input class="src" value="${esc(item.source || "")}" placeholder="ext.sku 或 title" /></td>
        <td><input class="dst" value="${esc(dest)}" placeholder="sku 或 data.sku" /></td>
        <td>
          <select class="mod">
            <option value="">无</option>
            <option value="list" ${mod === "list" ? "selected" : ""}>list</option>
            <option value="date" ${mod === "date" ? "selected" : ""}>date</option>
          </select>
        </td>
        <td><button type="button" class="ghost btn-del-row">删除</button></td>
      `;
      tr.querySelector(".btn-del-row").onclick = () => tr.remove();
      body.appendChild(tr);
    });
  };
  renderRows(entries);

  $("btnAddMap").onclick = () => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><input class="src" placeholder="ext.sku 或 title" /></td>
      <td><input class="dst" placeholder="sku 或 data.sku" /></td>
      <td>
        <select class="mod">
          <option value="">无</option>
          <option value="list">list</option>
          <option value="date">date</option>
        </select>
      </td>
      <td><button type="button" class="ghost btn-del-row">删除</button></td>
    `;
    tr.querySelector(".btn-del-row").onclick = () => tr.remove();
    body.appendChild(tr);
  };

  $("btnDelRule").onclick = () => {
    if (!confirm("确认删除当前规则？")) return;
    config.rule.splice(activeRule, 1);
    activeRule = Math.max(0, activeRule - 1);
    renderTabs();
    renderRuleEditor();
  };

  $("btnApplyBatch").onclick = () => {
    try {
      applyFieldBatch();
    } catch (e) {
      toast(e.message || String(e), true);
    }
  };
}

function esc(s) {
  return String(s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll('"', "&quot;");
}

async function refreshSyncState() {
  const hint = $("syncStateHint");
  try {
    const st = await api("/api/sync-state");
    hint.textContent = st.hint || (st.has_master_info ? "已有位点" : "无位点（将全量）");
  } catch (e) {
    hint.textContent = "无法读取位点状态：" + (e.message || e);
  }
}

async function loadConfig() {
  config = await api("/api/config");
  if (!config.rule) config.rule = [];
  config.rule.forEach((r) => {
    if (!r.field) r.field = {};
  });
  if (!config.rule.length) {
    config.rule.push(emptyRule());
  }
  activeRule = 0;
  fillConnection();
  renderTabs();
  renderRuleEditor();
  await refreshSyncState();
  toast("配置已加载");
}

async function saveConfig() {
  collectConnection();
  persistCurrentRuleForm();
  const res = await api("/api/config", {
    method: "PUT",
    body: JSON.stringify(config),
  });
  toast(res.message || "已保存");
  await loadConfig();
  await refreshSyncState();
}

async function loadRawToml() {
  const data = await api("/api/config/raw");
  $("rawToml").value = data.content || "";
}

async function saveRawToml() {
  const content = $("rawToml").value;
  const res = await api("/api/config/raw", {
    method: "PUT",
    body: JSON.stringify({ content }),
  });
  toast(res.message || "原文已保存");
  await loadConfig();
  setEditMode("form");
}

async function restoreBackup() {
  const data = await api("/api/config/backups");
  const list = data.backups || [];
  if (!list.length) {
    toast("暂无备份可回退（保存过至少一次后会生成）", true);
    return;
  }
  const lines = list.map(
    (b) =>
      `${b.slot}. ${b.name}  (${b.mod_time || ""})  — ${
        b.slot === 1 ? "最近一次保存前" : b.slot === 3 ? "最旧" : "中间版本"
      }`
  );
  const pick = prompt(
    "可回退的备份（最多保留 3 个版本）：\n" +
      lines.join("\n") +
      "\n\n请输入要回退的版本号（1/2/3）：",
    "1"
  );
  if (pick == null) return;
  const slot = Number(pick.trim());
  if (![1, 2, 3].includes(slot) || !list.some((b) => b.slot === slot)) {
    toast("无效的版本号", true);
    return;
  }
  if (!confirm(`确认将 river.toml 回退为 bak.${slot}？\n当前配置会先备份再覆盖。`)) return;
  const res = await api("/api/config/restore", {
    method: "POST",
    body: JSON.stringify({ slot }),
  });
  toast(res.message || "已回退");
  await loadConfig();
  if (editMode === "raw") await loadRawToml();
}

async function restartSync() {
  const fullDump = !!$("chkFullDump")?.checked;
  const msg = fullDump
    ? "确认重启同步容器 go-mysqlbinlog-to-es？\n将删除位点 master.info，下次启动会全量 dump。"
    : "确认重启同步容器 go-mysqlbinlog-to-es？\n重启后会重新加载 river.toml。";
  if (!confirm(msg)) return;
  try {
    const res = await api("/api/restart", {
      method: "POST",
      body: JSON.stringify({ full_dump: fullDump }),
    });
    toast((res.message || "已重启") + (res.method ? ` [${res.method}]` : ""));
    $("chkFullDump").checked = false;
    setTimeout(() => refreshSyncState().catch(() => {}), 1500);
  } catch (e) {
    toast(e.message || String(e), true);
    alert("重启失败：\n" + (e.message || e));
  }
}

async function loadSchemas() {
  collectConnection();
  persistCurrentRuleForm();
  await api("/api/config", { method: "PUT", body: JSON.stringify(config) });

  const data = await api("/api/schemas");
  const sel = $("pickSchema");
  const cur = sel.value;
  sel.innerHTML = data.schemas.map((s) => `<option value="${esc(s)}">${esc(s)}</option>`).join("");
  if (cur && data.schemas.includes(cur)) sel.value = cur;
  else if (config.rule[activeRule]?.schema) sel.value = config.rule[activeRule].schema;
  await loadTables();
  toast(`已加载 ${data.schemas.length} 个库`);
}

async function loadTables() {
  const schema = $("pickSchema").value;
  if (!schema) return;
  const data = await api(`/api/tables?schema=${encodeURIComponent(schema)}`);
  const sel = $("pickTable");
  const cur = sel.value;
  sel.innerHTML = data.tables.map((t) => `<option value="${esc(t)}">${esc(t)}</option>`).join("");
  if (cur && data.tables.includes(cur)) sel.value = cur;
  else if (config.rule[activeRule]?.table) sel.value = config.rule[activeRule].table;
}

async function loadColumns() {
  const schema = $("pickSchema").value;
  const table = $("pickTable").value;
  if (!schema || !table) {
    toast("请先选择库和表", true);
    return;
  }
  const data = await api(
    `/api/columns?schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}&sample_json=1`
  );
  columnsCache = data.columns || [];
  renderColumns();
  toast(`已加载 ${columnsCache.length} 个字段`);
}

function renderColumns() {
  const box = $("columnsBox");
  if (!columnsCache.length) {
    box.innerHTML = `<p class="muted">暂无字段。先刷新库表并加载字段。</p>`;
    return;
  }
  box.innerHTML = "";
  columnsCache.forEach((col) => {
    const card = document.createElement("div");
    card.className = "col-card";
    const badge = col.is_json
      ? `<span class="badge json">JSON</span>`
      : `<span class="badge">${esc(col.type)}</span>`;
    card.innerHTML = `
      <div class="col-head">
        <div>
          <strong>${esc(col.name)}</strong>
          <div class="muted">${esc(col.type)}${col.key ? " · " + esc(col.key) : ""}</div>
        </div>
        <div class="inline-actions">
          ${badge}
          <button type="button" class="ghost btn-map-col">映射此列</button>
        </div>
      </div>
    `;
    card.querySelector(".btn-map-col").onclick = () => addMappingRow(col.name, col.name);
    if (col.is_json && col.json_keys?.length) {
      const keys = document.createElement("div");
      keys.className = "json-keys";
      col.json_keys.forEach((k) => {
        const chip = document.createElement("button");
        chip.type = "button";
        chip.className = "chip";
        chip.textContent = `${col.name}.${k}`;
        chip.title = "点击加入映射，默认拍平到同名路径";
        chip.onclick = () => addMappingRow(`${col.name}.${k}`, k.split(".").pop());
        keys.appendChild(chip);
      });
      card.appendChild(keys);
    } else if (col.is_json) {
      const tip = document.createElement("p");
      tip.className = "muted";
      tip.textContent = "未抽样到 JSON 子键（可能无数据或结构为空）";
      card.appendChild(tip);
    }
    box.appendChild(card);
  });
}

function addMappingRow(source, dest) {
  persistCurrentRuleForm();
  const rule = config.rule[activeRule];
  if (!rule.field) rule.field = {};
  if (!rule.field[source]) rule.field[source] = dest || source;
  renderRuleEditor();
  toast(`已加入映射：${source} → ${rule.field[source]}`);
}

function applyTableToRule() {
  const schema = $("pickSchema").value;
  const table = $("pickTable").value;
  if (!schema || !table) {
    toast("请先选择库和表", true);
    return;
  }
  persistCurrentRuleForm();
  const rule = config.rule[activeRule];
  rule.schema = schema;
  rule.table = table;
  if (!rule.index) rule.index = `${schema}_${table}`.toLowerCase();
  if (!rule.type) rule.type = "_doc";
  renderTabs();
  renderRuleEditor();
  toast(`已应用到当前规则：${schema}.${table}`);
}

function bindEvents() {
  $("btnReload").onclick = () => {
    if (editMode === "raw") loadRawToml().catch((e) => toast(e.message, true));
    else loadConfig().catch((e) => toast(e.message, true));
  };
  $("btnSave").onclick = () => {
    if (editMode === "raw") saveRawToml().catch((e) => toast(e.message, true));
    else saveConfig().catch((e) => toast(e.message, true));
  };
  $("btnRestore").onclick = () => restoreBackup().catch((e) => toast(e.message, true));
  $("btnRestart").onclick = () => restartSync().catch((e) => toast(e.message, true));
  $("btnOpenLogs").onclick = () => window.open("/logs.html", "_blank");
  $("tabForm").onclick = () => setEditMode("form");
  $("tabRaw").onclick = () => setEditMode("raw");
  $("btnRawReload").onclick = () => loadRawToml().then(() => toast("已从磁盘加载原文")).catch((e) => toast(e.message, true));
  $("btnRawSave").onclick = () => saveRawToml().catch((e) => toast(e.message, true));
  $("btnAddRule").onclick = () => {
    persistCurrentRuleForm();
    config.rule.push(emptyRule());
    activeRule = config.rule.length - 1;
    renderTabs();
    renderRuleEditor();
  };
  $("btnLoadSchemas").onclick = () => loadSchemas().catch((e) => toast(e.message, true));
  $("btnLoadColumns").onclick = () => loadColumns().catch((e) => toast(e.message, true));
  $("btnApplyTable").onclick = applyTableToRule;
  $("pickSchema").onchange = () => loadTables().catch((e) => toast(e.message, true));
}

bindEvents();
loadConfig().catch((e) => toast(e.message, true));
