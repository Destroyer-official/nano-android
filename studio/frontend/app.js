(() => {
  const WORKSPACES = [
    { group: "Core", id: "dashboard", label: "Dashboard" },
    { group: "Core", id: "projects", label: "Project Explorer" },
    { group: "Core", id: "apk-inspector", label: "APK Inspector" },
    { group: "Execution", id: "runtime-studio", label: "Runtime Studio" },
    { group: "Execution", id: "live-device", label: "Live Device" },
    { group: "UI & A11y", id: "ui-inspector", label: "UI Inspector" },
    { group: "UI & A11y", id: "layout-explorer", label: "Layout Explorer" },
    { group: "UI & A11y", id: "visual-lab", label: "Visual Lab" },
    { group: "Testing", id: "test-lab", label: "Test Lab (12 Labs)" },
    { group: "Testing", id: "differential-lab", label: "Differential Lab" },
    { group: "Testing", id: "ai-lab", label: "AI Lab (Governance)" },
    { group: "Diagnostics", id: "profiler", label: "Profiler & Timeline" },
    { group: "Diagnostics", id: "network-inspector", label: "Network Inspector" },
    { group: "Diagnostics", id: "database-inspector", label: "Database Inspector" },
    { group: "Diagnostics", id: "security-center", label: "Security Center" },
    { group: "Infra", id: "device-manager", label: "Device Manager" },
    { group: "Infra", id: "build-center", label: "Build Center" },
    { group: "Infra", id: "reports", label: "Reports" },
    { group: "Infra", id: "settings", label: "Settings" },
  ];

  const params = new URLSearchParams(window.location.search);
  let authToken = params.get("token") || localStorage.getItem("nano_studio_token") || "";
  if (params.get("token")) {
    localStorage.setItem("nano_studio_token", authToken);
  }

  const state = {
    activeWorkspace: "dashboard",
    snapshot: null,
    projects: null,
    apks: [],
    apkInspect: null,
    apkCompat: null,
    executions: [],
    activeExecEvents: null,
    testsMeta: null,
    lastLabResult: null,
    reports: [],
    devices: null,
    workers: null,
    policies: null,
    git: null,
    visualBaselines: [],
    lastVisualDiff: null,
    graphResult: null,
    aiResult: null,
    aiProposals: [],
    pools: null,
    pluginsMeta: null,
    adaptersMeta: null,
    analyticsMeta: null,
    showOverlays: true,
    jobs: 0,
    notifs: 0,
    logs: [],
  };

  function logLine(msg, level = "INFO") {
    const ts = new Date().toISOString().slice(11, 19);
    state.logs.unshift(`[${ts}] [${level}] ${msg}`);
    if (state.logs.length > 200) state.logs.length = 200;
    const el = document.getElementById("drawerBody");
    if (el) el.innerText = state.logs.join("\n");
  }

  async function apiFetch(url, opts = {}) {
    const headers = Object.assign({}, opts.headers || {});
    if (authToken) headers["Authorization"] = `Bearer ${authToken}`;
    const res = await fetch(url, Object.assign({}, opts, { headers }));
    const txt = await res.text();
    let data = null;
    try { data = JSON.parse(txt); } catch { data = { raw: txt }; }
    if (!res.ok) {
      throw new Error((data && data.error) || `HTTP ${res.status}`);
    }
    return data;
  }

  function escapeHtml(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;")
      .replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  }

  function renderNav() {
    const rail = document.getElementById("navRail");
    let lastGroup = "";
    rail.innerHTML = "";
    for (const ws of WORKSPACES) {
      if (ws.group !== lastGroup) {
        lastGroup = ws.group;
        const g = document.createElement("div");
        g.className = "nav-group";
        g.textContent = ws.group;
        rail.appendChild(g);
      }
      const btn = document.createElement("button");
      btn.className = "nav-btn" + (state.activeWorkspace === ws.id ? " active" : "");
      btn.textContent = ws.label;
      btn.onclick = () => switchWorkspace(ws.id);
      rail.appendChild(btn);
    }
  }

  function refreshScreenImg() {
    const img = document.getElementById("screenImg");
    const q = new URLSearchParams({ refresh: "1", t: String(Date.now()) });
    if (authToken) q.set("token", authToken);
    img.src = `/api/screen.png?${q.toString()}`;
  }

  function renderASTOverlays(astRoot) {
    const boxHost = document.getElementById("astOverlays");
    boxHost.innerHTML = "";
    if (!state.showOverlays || !astRoot) return;
    const walk = (n) => {
      if (!n) return;
      if (Array.isArray(n.bounds) && n.bounds.length === 4 && n.visible) {
        const [x1, y1, x2, y2] = n.bounds;
        if (x2 > x1 && y2 > y1) {
          const div = document.createElement("div");
          div.className = "ast-overlay-box";
          div.style.left = `${(x1 / 1080) * 100}%`;
          div.style.top = `${(y1 / 2400) * 100}%`;
          div.style.width = `${((x2 - x1) / 1080) * 100}%`;
          div.style.height = `${((y2 - y1) / 2400) * 100}%`;
          boxHost.appendChild(div);
        }
      }
      if (Array.isArray(n.children)) n.children.forEach(walk);
    };
    walk(astRoot);
  }

  async function refreshAll() {
    try {
      const [obs, appsRes, projRes, devRes, polRes, workRes, gitRes, testsRes, repRes, baseRes, propRes] = await Promise.allSettled([
        apiFetch("/api/observe?refresh=1"),
        apiFetch("/api/v1/apks"),
        apiFetch("/api/v1/projects"),
        apiFetch("/api/v1/devices"),
        apiFetch("/api/v1/policies"),
        apiFetch("/api/v1/workers"),
        apiFetch("/api/v1/git"),
        apiFetch("/api/v1/tests"),
        apiFetch("/api/v1/reports"),
        apiFetch("/api/v1/visual/baselines"),
        apiFetch("/api/v1/ai/proposals"),
      ]);
      if (obs.status === "fulfilled") state.snapshot = obs.value;
      if (appsRes.status === "fulfilled") {
        state.apks = appsRes.value.items || [];
        populateArtifactSelect();
      }
      if (projRes.status === "fulfilled") state.projects = projRes.value;
      if (devRes.status === "fulfilled") state.devices = devRes.value;
      if (polRes.status === "fulfilled") state.policies = polRes.value;
      if (workRes.status === "fulfilled") state.workers = workRes.value;
      if (gitRes.status === "fulfilled") state.git = gitRes.value;
      if (testsRes.status === "fulfilled") state.testsMeta = testsRes.value;
      if (repRes.status === "fulfilled") state.reports = repRes.value.reports || [];
      if (baseRes.status === "fulfilled") state.visualBaselines = baseRes.value.baselines || [];
      if (propRes.status === "fulfilled") state.aiProposals = propRes.value.proposals || [];

      updateTopBadges();
      refreshScreenImg();
      renderASTOverlays(state.snapshot && state.snapshot.observe && state.snapshot.observe.ast);
      renderWorkspace();
    } catch (e) {
      logLine("Refresh error: " + e.message, "WARN");
    }
  }

  function populateArtifactSelect() {
    const sel = document.getElementById("artifactSelect");
    const prev = sel.value;
    sel.innerHTML = "";
    for (const a of state.apks) {
      const opt = document.createElement("option");
      opt.value = a.path;
      opt.dataset.id = a.name;
      opt.textContent = `${a.name} (${Math.max(1, Math.round(a.size_bytes / 1024))} KB)`;
      sel.appendChild(opt);
    }
    if (prev) sel.value = prev;
  }

  function updateTopBadges() {
    const last = state.snapshot && state.snapshot.last_launch;
    document.getElementById("activePkgBadge").textContent = last ? `${last.app_id || "app"} · ${last.result_status || last.status}` : "idle";
    if (state.git && state.git.available) {
      document.getElementById("gitPill").textContent = `git: ${state.git.branch || "detached"}`;
    }
    const execMode = (state.workers && state.workers.execution_mode) || "nano_jvm";
    document.getElementById("backendPill").textContent = `tier: ${execMode}`;
    document.getElementById("jobsBadge").textContent = `jobs: ${state.jobs}`;
    document.getElementById("notifBadge").textContent = `reports: ${state.reports.length}`;
  }

  function switchWorkspace(id) {
    state.activeWorkspace = id;
    const found = WORKSPACES.find((w) => w.id === id);
    document.getElementById("activeWorkspaceTitle").textContent = found ? found.label : id;
    renderNav();
    renderWorkspace();
  }

  function jsonBlock(obj) {
    return `<pre class="json">${escapeHtml(JSON.stringify(obj || {}, null, 2))}</pre>`;
  }

  function selectedArtifactName() {
    const sel = document.getElementById("artifactSelect");
    const opt = sel.options[sel.selectedIndex];
    return opt ? opt.dataset.id : (state.apks[0] && state.apks[0].name) || "minimal_valid.apk";
  }

  function selectedArtifactPath() {
    return document.getElementById("artifactSelect").value || "";
  }

  function renderWorkspace() {
    const host = document.getElementById("wsMain");
    const id = state.activeWorkspace;
    const obs = (state.snapshot && state.snapshot.observe) || {};
    const wm = (state.workers && state.workers.metrics) || {};

    switch (id) {
      case "dashboard":
        host.innerHTML = `
          <div class="grid">
            <div class="card"><h3>Active Tier</h3><div class="metric-big">${escapeHtml((state.workers && state.workers.execution_mode) || "nano_jvm")}</div><small>Canonical status disclosure active</small></div>
            <div class="card"><h3>Workspace Artifacts</h3><div class="metric-big">${state.apks.length}</div><small>${(state.projects && (state.projects.manifests || []).length) || 0} AndroidManifest files</small></div>
            <div class="card"><h3>JVM Heap Used</h3><div class="metric-big">${wm.used_mb ? wm.used_mb.toFixed(1) + " MB" : "0 MB"}</div><small>${wm.loaded_classes || 0} classes · ${wm.gc_cycles || 0} GC cycles</small></div>
            <div class="card"><h3>Audit Chain Integrity</h3><div class="metric-big">${state.policies && state.policies.audit_chain_valid ? "VERIFIED" : "CHECK"}</div><small>${(state.policies && state.policies.verified_entries) || 0} SHA-256 chained records</small></div>
          </div>
          <div class="card"><h3>Latest Launch & Session State</h3>${jsonBlock(state.snapshot && state.snapshot.last_launch)}</div>`;
        break;

      case "projects":
        host.innerHTML = `<div class="card"><h3>Project Explorer & Git Repository</h3>${jsonBlock({ git: state.git, project: state.projects })}</div>`;
        break;

      case "apk-inspector":
        host.innerHTML = `
          <div class="card">
            <h3>Tier-0 Static APK Intelligence <button id="btnInspectAPK" class="primary">Analyze Selected Artifact</button></h3>
            ${jsonBlock(state.apkInspect || { hint: "Click Analyze Selected Artifact to run manifest/DEX/resource/ELF/MASVS inspection" })}
          </div>
          <div class="card"><h3>API 26-37 Compatibility & Heatmap</h3>${jsonBlock(state.apkCompat || {})}</div>`;
        document.getElementById("btnInspectAPK").onclick = async () => {
          const name = encodeURIComponent(selectedArtifactName());
          state.jobs++; updateTopBadges();
          try {
            const [insp, cmp] = await Promise.all([
              apiFetch(`/api/v1/apks/${name}/inspect`),
              apiFetch(`/api/v1/apks/${name}/compatibility`),
            ]);
            state.apkInspect = insp;
            state.apkCompat = cmp;
            logLine(`Inspected ${selectedArtifactName()} (${insp.intel && insp.intel.sha256 ? insp.intel.sha256.slice(0, 12) : "ok"})`);
          } finally { state.jobs = Math.max(0, state.jobs - 1); updateTopBadges(); renderWorkspace(); }
        };
        break;

      case "runtime-studio":
        host.innerHTML = `
          <div class="card">
            <h3>Runtime Controls & Virtual Breakpoints
              <span style="display:flex;gap:6px;">
                <button id="btnProcDeath">Simulate Process Death</button>
                <button id="btnSnapshotSave">Snapshot UI</button>
              </span>
            </h3>
            <div style="display:flex;gap:6px;margin-bottom:8px;">
              <input id="bpTarget" type="text" placeholder="Lifecycle/API target (e.g. Activity.onCreate)" style="flex:1;" />
              <button id="btnAddBP" class="primary">Add Breakpoint</button>
            </div>
            ${jsonBlock({ virtual_uptime_ms: wm.virtual_uptime_ms, looper_queue_depth: wm.looper_queue_depth, actions: (state.snapshot && state.snapshot.actions) || [] })}
          </div>`;
        document.getElementById("btnAddBP").onclick = async () => {
          const tgt = document.getElementById("bpTarget").value.trim() || "Activity.onCreate";
          const execId = state.executions[0] && state.executions[0].id;
          if (!execId) return logLine("Launch an execution first before setting breakpoints", "WARN");
          const bp = await apiFetch(`/api/v1/executions/${execId}/breakpoint`, {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ kind: "lifecycle", target: tgt, enabled: true }),
          });
          logLine("Breakpoint registered: " + JSON.stringify(bp));
        };
        document.getElementById("btnSnapshotSave").onclick = async () => {
          const execId = state.executions[0] && state.executions[0].id;
          if (!execId) return refreshAll();
          await apiFetch(`/api/v1/executions/${execId}/snapshot`, {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: "studio_ui_snap" }),
          });
          logLine("Saved execution UI snapshot for " + execId);
          await refreshAll();
        };
        document.getElementById("btnProcDeath").onclick = async () => {
          const execId = state.executions[0] && state.executions[0].id;
          if (!execId) return logLine("Launch an execution first before simulating process death", "WARN");
          await apiFetch(`/api/v1/executions/${execId}/process_death`, { method: "POST" });
          logLine("Simulated Android process death & state restoration on " + execId);
          await refreshAll();
        };
        break;

      case "live-device":
        host.innerHTML = `
          <div class="card">
            <h3>Live Device & Execution Mode Switcher
              <span style="display:flex;gap:6px;">
                <button id="btnModeNano">Nano JVM</button>
                <button id="btnModeADB">ADB Device</button>
              </span>
            </h3>
            ${jsonBlock(state.devices)}
          </div>`;
        document.getElementById("btnModeNano").onclick = async () => { await apiFetch("/api/mode", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ mode: "nano_jvm" }) }); await refreshAll(); };
        document.getElementById("btnModeADB").onclick = async () => {
          try { await apiFetch("/api/mode", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ mode: "adb_device" }) }); await refreshAll(); }
          catch (e) { logLine("ADB switch refused: " + e.message, "WARN"); }
        };
        break;

      case "ui-inspector":
        host.innerHTML = `<div class="card"><h3>Accessibility & UI View AST (${(obs.visible_texts || []).length} visible texts)</h3>${jsonBlock(obs.ast || obs)}</div>`;
        break;

      case "layout-explorer":
        const layouts = (state.snapshot && state.snapshot.layouts) || [];
        host.innerHTML = `
          <div class="card">
            <h3>Binary AXML Layout Explorer (${layouts.length} layouts)</h3>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              ${layouts.map((l) => `<button class="btnPreviewLayout" data-layout="${escapeHtml(l.name)}">${escapeHtml(l.name)}</button>`).join("") || "<span class='pill'>No APK AXML layouts in current artifact</span>"}
            </div>
            ${jsonBlock(layouts)}
          </div>`;
        host.querySelectorAll(".btnPreviewLayout").forEach((b) => {
          b.onclick = async () => {
            await apiFetch("/api/preview_layout", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ layout: b.dataset.layout }) });
            await refreshAll();
          };
        });
        break;

      case "visual-lab":
        const vDiff = state.lastVisualDiff;
        host.innerHTML = `
          <div class="card">
            <h3>Visual Regression Lab (6 Modes & Approval Gate)</h3>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              <input id="visName" type="text" value="main_screen" placeholder="Baseline name" />
              <select id="visMode">
                <option value="pixel_exact">pixel_exact</option>
                <option value="tolerant_pixel" selected>tolerant_pixel</option>
                <option value="structural">structural</option>
                <option value="semantic">semantic</option>
                <option value="text">text</option>
                <option value="hybrid">hybrid</option>
              </select>
              <input id="visApprover" type="text" value="studio_reviewer" placeholder="Approver ID (required)" />
              <button id="btnVisApprove" class="primary">Approve Golden Baseline</button>
              <button id="btnVisCompare">Compare Live vs Baseline</button>
            </div>
            ${vDiff ? `<p style="margin-bottom:8px;"><strong>${escapeHtml(vDiff.summary)}</strong></p>
              ${vDiff.side_by_side_base64_png ? `<div style="margin-bottom:8px;"><small>Side-by-Side (Baseline | Candidate)</small><img src="${vDiff.side_by_side_base64_png}" style="width:100%;border:1px solid var(--border);border-radius:6px;" /></div>` : ""}
              <div class="row-imgs">
                ${vDiff.heatmap_base64_png ? `<div><small>Difference Heatmap</small><img src="${vDiff.heatmap_base64_png}" /></div>` : ""}
                ${vDiff.overlay_base64_png ? `<div><small>Overlay Blend</small><img src="${vDiff.overlay_base64_png}" /></div>` : ""}
              </div>` : ""}
            ${jsonBlock({ baselines: state.visualBaselines, diff: vDiff })}
          </div>`;
        document.getElementById("btnVisApprove").onclick = async () => {
          const meta = await apiFetch("/api/v1/visual/approve", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: document.getElementById("visName").value, profile_id: "clean_install", approved_by: document.getElementById("visApprover").value }),
          });
          logLine("Approved visual baseline: " + meta.name);
          await refreshAll();
        };
        document.getElementById("btnVisCompare").onclick = async () => {
          state.lastVisualDiff = await apiFetch("/api/v1/visual/compare", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: document.getElementById("visName").value, profile_id: "clean_install", mode: document.getElementById("visMode").value }),
          });
          await refreshAll();
        };
        break;

      case "test-lab":
        const labsList = (state.testsMeta && state.testsMeta.labs) || [];
        host.innerHTML = `
          <div class="card">
            <h3>12 Specialized Test Labs & Maestro Export <a id="btnExportYaml" class="pill" href="/api/export_maestro?token=${encodeURIComponent(authToken)}" target="_blank">Export Maestro YAML</a></h3>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              ${labsList.map((l) => `<button class="btnRunLab" data-lab="${escapeHtml(l.id)}">Run ${escapeHtml(l.name)}</button>`).join("")}
            </div>
            ${jsonBlock(state.lastLabResult || labsList)}
          </div>`;
        host.querySelectorAll(".btnRunLab").forEach((b) => {
          b.onclick = async () => {
            state.jobs++; updateTopBadges();
            try {
              state.lastLabResult = await apiFetch(`/api/v1/tests/${encodeURIComponent(b.dataset.lab)}/run`, {
                method: "POST", headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ artifact: selectedArtifactPath() }),
              });
              logLine(`Lab '${b.dataset.lab}' completed.`);
            } finally { state.jobs = Math.max(0, state.jobs - 1); await refreshAll(); }
          };
        });
        break;

      case "differential-lab":
        host.innerHTML = `
          <div class="card">
            <h3>11-Dimension Differential Comparison <button id="btnRunDiffLab" class="primary">Run Cross-Profile Differential</button></h3>
            ${jsonBlock(state.lastLabResult && state.lastLabResult.result && state.lastLabResult.result.lab_id === "differential" ? state.lastLabResult : { info: "Runs 11-dimension comparison (clean_install vs small_screen) and pinpoints earliest divergence." })}
          </div>`;
        document.getElementById("btnRunDiffLab").onclick = async () => {
          state.lastLabResult = await apiFetch("/api/v1/tests/differential/run", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ artifact: selectedArtifactPath(), profile_id: "small_screen" }),
          });
          renderWorkspace();
        };
        break;

      case "ai-lab":
        host.innerHTML = `
          <div class="card">
            <h3>Phase 5 AI Autonomous Engineer & Behavior Graph</h3>
            <p style="margin-bottom:8px;color:var(--muted);">Budget-bounded, prompt-injection-fenced (&lt;DF_UNTRUSTED_*&gt;), human-approved workflows.</p>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              <button id="btnGraphBuild" class="primary">Build Behavior Graph</button>
              <button id="btnGraphQuery">Query Graph</button>
              <button id="btnAIUnderstand">I1 Understand APK</button>
              <button id="btnAIExplore">I3 Autonomous Bug Hunter</button>
              <button id="btnAIGenTests">I2 Generate Tests</button>
              <button id="btnAICoordinate">Multi-Agent Coordinate</button>
            </div>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              <input id="aiImpactTargets" type="text" placeholder="Impact symbols (e.g. MainActivity,SQLiteDatabase)" style="flex:1;" />
              <button id="btnGraphImpact">Analyze Impact</button>
            </div>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              <input id="aiDiagTrace" type="text" placeholder="Paste crash trace or suspected API..." style="flex:1;" />
              <button id="btnAIDiagnose">I4 Diagnose & Propose Patch</button>
            </div>
            <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">
              <input id="aiBrokenSel" type="text" placeholder="Broken selector text or resource_id (Self-Heal)" style="flex:1;" />
              <button id="btnAISelfHeal">Self-Heal Selector</button>
              <button id="btnAIMemory">Inspect Memory</button>
            </div>
            ${jsonBlock(state.aiResult || state.graphResult || { info: "Run Build Behavior Graph or any AI workflow above." })}
          </div>
          <div class="card">
            <h3>Human Approval Queue (${state.aiProposals.length} proposals)</h3>
            <div style="display:flex;gap:6px;align-items:center;margin-bottom:8px;">
              <input id="aiApproverInput" type="text" value="studio_reviewer" placeholder="Approver ID (required)" />
            </div>
            ${state.aiProposals.map((p) => `
              <div style="border-top:1px solid var(--border);padding:8px 0;">
                <div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">
                  <strong>${escapeHtml(p.title)} [${escapeHtml(p.status)}]</strong>
                  <span style="display:flex;gap:6px;">
                    <button class="btnApproveProp" data-id="${escapeHtml(p.id)}">Approve & Save</button>
                    <button class="btnRejectProp" data-id="${escapeHtml(p.id)}">Reject</button>
                  </span>
                </div>
                <small style="color:var(--muted);">${escapeHtml(p.kind)} · ${escapeHtml(p.id)}</small>
              </div>
            `).join("") || "<p style='color:var(--muted);'>No AI proposals yet.</p>"}
          </div>`;
        const runAI = async (url, bodyObj, targetKey) => {
          state.jobs++; updateTopBadges();
          try {
            state[targetKey] = await apiFetch(url, {
              method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify(bodyObj),
            });
            logLine(`Completed ${url}`);
          } catch (e) {
            logLine(`${url} failed: ${e.message}`, "WARN");
          } finally { state.jobs = Math.max(0, state.jobs - 1); await refreshAll(); }
        };
        document.getElementById("btnGraphBuild").onclick = () => runAI("/api/v1/graph/build", { artifact: selectedArtifactPath(), scan_source: true }, "graphResult");
        document.getElementById("btnGraphQuery").onclick = () => runAI("/api/v1/graph/query", { limit: 100 }, "graphResult");
        document.getElementById("btnAIUnderstand").onclick = () => runAI("/api/v1/ai/understand", { artifact: selectedArtifactPath(), max_steps: 20 }, "aiResult");
        document.getElementById("btnAIExplore").onclick = () => runAI("/api/v1/ai/explore", { artifact: selectedArtifactPath(), max_steps: 10, max_screens: 8 }, "aiResult");
        document.getElementById("btnAIGenTests").onclick = () => runAI("/api/v1/ai/generate_tests", { artifact: selectedArtifactPath(), max_steps: 15 }, "aiResult");
        document.getElementById("btnAICoordinate").onclick = () => runAI("/api/v1/ai/coordinate", { artifact: selectedArtifactPath(), max_steps: 15, max_screens: 8 }, "aiResult");
        document.getElementById("btnGraphImpact").onclick = () => {
          const raw = document.getElementById("aiImpactTargets").value.trim();
          const changed = raw ? raw.split(",").map((s) => s.trim()).filter(Boolean) : ["MainActivity"];
          runAI("/api/v1/graph/impact", { artifact: selectedArtifactPath(), changed }, "graphResult");
        };
        document.getElementById("btnAIDiagnose").onclick = () => {
          const tr = document.getElementById("aiDiagTrace").value.trim() || "java.lang.NullPointerException: missing item";
          runAI("/api/v1/ai/diagnose", { trace: tr }, "aiResult");
        };
        document.getElementById("btnAISelfHeal").onclick = () => {
          const q = document.getElementById("aiBrokenSel").value.trim() || "Submit";
          runAI("/api/v1/ai/self_heal", { text: q, resource_id: q }, "aiResult");
        };
        document.getElementById("btnAIMemory").onclick = async () => {
          state.aiResult = await apiFetch("/api/v1/ai/memory");
          renderWorkspace();
        };
        host.querySelectorAll(".btnApproveProp").forEach((b) => {
          b.onclick = async () => {
            const by = document.getElementById("aiApproverInput").value.trim();
            try {
              await apiFetch("/api/v1/ai/proposals/approve", {
                method: "POST", headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: b.dataset.id, approved_by: by, apply: true }),
              });
              logLine(`Approved AI proposal ${b.dataset.id}`);
              await refreshAll();
            } catch (e) { logLine("Approve failed: " + e.message, "WARN"); }
          };
        });
        host.querySelectorAll(".btnRejectProp").forEach((b) => {
          b.onclick = async () => {
            const by = document.getElementById("aiApproverInput").value.trim();
            await apiFetch("/api/v1/ai/proposals/approve", {
              method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ id: b.dataset.id, approved_by: by || "reviewer", reject: true, reason: "rejected in studio" }),
            });
            await refreshAll();
          };
        });
        break;

      case "profiler":
        const tlEvents = (state.workers && state.workers.jvm_snapshot && state.workers.jvm_snapshot.timeline) || [];
        host.innerHTML = `
          <div class="card"><h3>Chronological Forensic Timeline (${tlEvents.length} events)</h3>${jsonBlock(tlEvents)}</div>
          <div class="card"><h3>Live JVM Heap, GC, Threads & Looper Telemetry</h3>${jsonBlock(state.workers)}</div>`;
        break;

      case "network-inspector":
        host.innerHTML = `
          <div class="card">
            <h3>Loopback Mock Network & Fault Injection Lab <button id="btnRunNetLab" class="primary">Run Network Lab Check</button></h3>
            ${jsonBlock(state.lastLabResult && state.lastLabResult.result && state.lastLabResult.result.lab_id === "network" ? state.lastLabResult : { info: "Verifies loopback mock routes, latency injection, and offline HTTP 503 resilience." })}
          </div>`;
        document.getElementById("btnRunNetLab").onclick = async () => {
          state.lastLabResult = await apiFetch("/api/v1/tests/network/run", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ artifact: selectedArtifactPath() }),
          });
          renderWorkspace();
        };
        break;

      case "database-inspector":
        host.innerHTML = `
          <div class="card">
            <h3>Live SQLite Tables, SharedPreferences & Staged Files</h3>
            ${jsonBlock({ storage: state.workers && state.workers.storage, staged_files: (state.snapshot && state.snapshot.staged_files) || [] })}
          </div>`;
        break;

      case "security-center":
        host.innerHTML = `
          <div class="card">
            <h3>OWASP MASVS v2.1.0, Policy Guard & Audit Chain <button id="btnRunSecLab" class="primary">Run Security Lab Audit</button></h3>
            ${jsonBlock({ policies: state.policies, last_security_lab: state.lastLabResult })}
          </div>`;
        document.getElementById("btnRunSecLab").onclick = async () => {
          state.lastLabResult = await apiFetch("/api/v1/tests/security/run", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ artifact: selectedArtifactPath() }),
          });
          await refreshAll();
        };
        break;

      case "device-manager":
        host.innerHTML = `
          <div class="card">
            <h3>ADB Devices, Remote Pools & 15 Scenario Profiles
              <span style="display:flex;gap:6px;">
                <button id="btnPoolRefresh">Refresh Pools</button>
                <button id="btnPoolShard" class="primary">Run Parallel Sharded Job</button>
              </span>
            </h3>
            ${jsonBlock({ local: state.devices, remote_pools: state.pools })}
          </div>
          <div class="card">
            <h3>Plugin SDK (Ed25519) & Ecosystem Adapters (12 Frameworks / 7 IDEs) <button id="btnLoadExt">Load SDK & Adapters</button></h3>
            ${jsonBlock({ plugins: state.pluginsMeta, adapters: state.adaptersMeta })}
          </div>`;
        document.getElementById("btnPoolRefresh").onclick = async () => {
          state.pools = await apiFetch("/api/v1/pools");
          renderWorkspace();
        };
        document.getElementById("btnPoolShard").onclick = async () => {
          state.pools = await apiFetch("/api/v1/pools/shard", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ actor: "studio_operator", max_shards: 2 }),
          });
          logLine("Completed parallel sharded pool job");
          renderWorkspace();
        };
        document.getElementById("btnLoadExt").onclick = async () => {
          const [pl, ad] = await Promise.all([apiFetch("/api/v1/plugins"), apiFetch("/api/v1/adapters")]);
          state.pluginsMeta = pl;
          state.adaptersMeta = ad;
          renderWorkspace();
        };
        break;

      case "build-center":
        host.innerHTML = `<div class="card"><h3>Gradle & Workspace Build Targets</h3>${jsonBlock(state.projects)}</div>`;
        break;

      case "reports":
        host.innerHTML = `
          <div class="card">
            <h3>Verifiable Signed Reports (22 Fields + Ed25519) & Analytics
              <span style="display:flex;gap:6px;">
                <button id="btnGenUnified" class="primary">Generate Signed Unified Report</button>
                <button id="btnLoadAnalytics">Load Analytics Trends</button>
              </span>
            </h3>
            ${state.reports.filter((r) => r.kind === "unified_signed").slice(0, 3).map((r) => `
              <div style="margin:6px 0;display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
                <strong>${escapeHtml(r.title)}</strong>
                <a class="pill" href="/api/v1/reports/${encodeURIComponent(r.id)}?format=html&token=${encodeURIComponent(authToken)}" target="_blank">HTML</a>
                <a class="pill" href="/api/v1/reports/${encodeURIComponent(r.id)}?format=pdf&token=${encodeURIComponent(authToken)}" target="_blank">PDF</a>
                <a class="pill" href="/api/v1/reports/${encodeURIComponent(r.id)}?format=junit&token=${encodeURIComponent(authToken)}" target="_blank">JUnit</a>
                <a class="pill" href="/api/v1/reports/${encodeURIComponent(r.id)}?format=sarif&token=${encodeURIComponent(authToken)}" target="_blank">SARIF</a>
                <a class="pill" href="/api/v1/reports/${encodeURIComponent(r.id)}?format=zip&token=${encodeURIComponent(authToken)}" target="_blank">Forensic ZIP</a>
                <button class="btnVerifyRep" data-id="${escapeHtml(r.id)}">Verify Ed25519</button>
              </div>
            `).join("")}
            ${jsonBlock({ analytics: state.analyticsMeta, reports: state.reports })}
          </div>`;
        host.querySelectorAll(".btnVerifyRep").forEach((b) => {
          b.onclick = async () => {
            const full = await apiFetch(`/api/v1/reports/${encodeURIComponent(b.dataset.id)}`);
            const vRes = await apiFetch("/api/v1/reports/verify", {
              method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify(full.payload),
            });
            logLine(`Verified report ${b.dataset.id}: sha256=${vRes.report_sha256}`);
          };
        });
        document.getElementById("btnGenUnified").onclick = async () => {
          await apiFetch("/api/v1/reports/unified", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ artifact: selectedArtifactPath(), actor: "studio_engineer" }),
          });
          await refreshAll();
        };
        document.getElementById("btnLoadAnalytics").onclick = async () => {
          state.analyticsMeta = await apiFetch("/api/v1/analytics");
          renderWorkspace();
        };
        break;

      case "settings":
        host.innerHTML = `
          <div class="card">
            <h3>Workstation Settings & Auth</h3>
            <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;">
              <input id="settingsTokenInput" type="text" value="${escapeHtml(authToken)}" placeholder="Bearer token" style="width:340px;" />
              <button id="btnSaveToken" class="primary">Apply Token & Reconnect WS</button>
            </div>
            <p style="color:var(--muted);">Shortcuts: Ctrl+K Command Palette · F5 Refresh Viewport</p>
          </div>`;
        document.getElementById("btnSaveToken").onclick = () => {
          authToken = document.getElementById("settingsTokenInput").value.trim();
          localStorage.setItem("nano_studio_token", authToken);
          connectWS();
          refreshAll();
        };
        break;
    }
  }

  function connectWS() {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${proto}//${window.location.host}/api/v1/ws${authToken ? `?token=${encodeURIComponent(authToken)}` : ""}`;
    try {
      const ws = new WebSocket(wsUrl);
      ws.onopen = () => {
        document.getElementById("wsDot").classList.remove("offline");
        logLine("WebSocket /api/v1/ws connected");
      };
      ws.onmessage = (ev) => {
        logLine("WS: " + ev.data);
        updateTopBadges();
      };
      ws.onclose = () => {
        document.getElementById("wsDot").classList.add("offline");
      };
    } catch (e) {
      logLine("WebSocket error: " + e.message, "WARN");
    }
  }

  function openPalette() {
    const modal = document.getElementById("paletteModal");
    modal.classList.add("open");
    const q = document.getElementById("paletteQuery");
    q.value = "";
    q.focus();
    renderPalette("");
  }

  function renderPalette(query) {
    const res = document.getElementById("paletteResults");
    const ql = query.toLowerCase();
    const matches = WORKSPACES.filter((w) => w.label.toLowerCase().includes(ql) || w.group.toLowerCase().includes(ql));
    res.innerHTML = matches.map((w) => `<div class="palette-item" data-ws="${w.id}"><span>${escapeHtml(w.label)}</span><small>${escapeHtml(w.group)}</small></div>`).join("");
    res.querySelectorAll(".palette-item").forEach((el) => {
      el.onclick = () => {
        document.getElementById("paletteModal").classList.remove("open");
        switchWorkspace(el.dataset.ws);
      };
    });
  }

  function bindEvents() {
    document.getElementById("btnObserve").onclick = refreshAll;
    document.getElementById("btnLaunch").onclick = async () => {
      const p = selectedArtifactPath();
      if (!p) return;
      state.jobs++; updateTopBadges();
      try {
        const execRec = await apiFetch("/api/v1/executions", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ artifact: p }),
        });
        state.executions.unshift(execRec);
        logLine(`Launched execution ${execRec.id}: ${execRec.activity}`);
      } catch (e) {
        logLine("Launch failed: " + e.message, "ERR");
      } finally { state.jobs = Math.max(0, state.jobs - 1); await refreshAll(); }
    };

    document.getElementById("btnStep300").onclick = async () => {
      try {
        await apiFetch("/api/step", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ delta_ms: 300 }) });
        logLine("Stepped virtual Looper +300ms");
        await refreshAll();
      } catch (e) {
        logLine("Looper step failed: " + e.message, "WARN");
      }
    };

    document.getElementById("apkUploadInput").onchange = async (e) => {
      const f = e.target.files[0];
      if (!f) return;
      const fd = new FormData();
      fd.append("apk", f);
      try {
        await apiFetch("/api/upload", { method: "POST", body: fd });
        logLine(`Uploaded ${f.name}`);
        await refreshAll();
      } catch (err) {
        logLine("Upload failed: " + err.message, "WARN");
      }
    };

    const frame = document.getElementById("phoneFrame");
    let pointerStart = null;
    const toDeviceXY = (e) => {
      const r = frame.getBoundingClientRect();
      return {
        x: Math.round(((e.clientX - r.left) / r.width) * 1080),
        y: Math.round(((e.clientY - r.top) / r.height) * 2400),
      };
    };
    frame.onmousemove = (e) => {
      const pt = toDeviceXY(e);
      document.getElementById("coordReadout").textContent = `x:${pt.x} y:${pt.y}`;
    };
    frame.onpointerdown = (e) => { pointerStart = toDeviceXY(e); };
    frame.onpointerup = async (e) => {
      if (!pointerStart) return;
      const end = toDeviceXY(e);
      const start = pointerStart;
      pointerStart = null;
      const dist = Math.hypot(end.x - start.x, end.y - start.y);
      const payload = dist >= 18
        ? { type: "swipe", start_x: start.x, start_y: start.y, end_x: end.x, end_y: end.y }
        : { type: "tap", x: end.x, y: end.y };
      try {
        const res = await apiFetch("/api/action", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        });
        document.getElementById("selectedNodeJson").textContent = JSON.stringify(res.action_result || {}, null, 2);
        const targetText = (res.action_result && res.action_result.resolved_text) || `(${end.x},${end.y})`;
        logLine(`Viewport ${payload.type} -> ${targetText}`);
        await refreshAll();
      } catch (err) {
        logLine("Viewport action failed: " + err.message, "WARN");
      }
    };
    frame.addEventListener("wheel", async (e) => {
      e.preventDefault();
      const dir = e.deltaY > 0 ? "DOWN" : "UP";
      try {
        await apiFetch("/api/action", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ type: "scroll", direction: dir }),
        });
        logLine(`Viewport scroll ${dir}`);
        await refreshAll();
      } catch (err) {
        logLine("Scroll failed: " + err.message, "WARN");
      }
    }, { passive: false });

    document.querySelectorAll("[data-act]").forEach((btn) => {
      btn.onclick = async () => {
        try {
          await apiFetch("/api/action", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ type: btn.dataset.act, direction: btn.dataset.dir || "" }),
          });
          logLine(`Action ${btn.dataset.act}${btn.dataset.dir ? " " + btn.dataset.dir : ""}`);
          await refreshAll();
        } catch (err) {
          logLine("Action failed: " + err.message, "WARN");
        }
      };
    });

    document.getElementById("btnSendType").onclick = async () => {
      const txt = document.getElementById("typeTextInput").value;
      try {
        await apiFetch("/api/action", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ type: "type", text: txt }) });
        logLine(`Typed ${txt.length} char(s)`);
        await refreshAll();
      } catch (err) {
        logLine("Type failed: " + err.message, "WARN");
      }
    };

    document.getElementById("btnToggleOverlays").onclick = () => {
      state.showOverlays = !state.showOverlays;
      renderASTOverlays(state.snapshot && state.snapshot.observe && state.snapshot.observe.ast);
    };

    document.getElementById("btnTheme").onclick = () => {
      const cur = document.documentElement.getAttribute("data-theme");
      document.documentElement.setAttribute("data-theme", cur === "light" ? "dark" : "light");
    };
    document.getElementById("btnDensity").onclick = () => {
      const cur = document.documentElement.getAttribute("data-density");
      document.documentElement.setAttribute("data-density", cur === "compact" ? "" : "compact");
    };
    document.getElementById("btnClearLog").onclick = () => { state.logs = []; document.getElementById("drawerBody").textContent = ""; };
    document.getElementById("btnRunConsole").onclick = async () => {
      const cmd = document.getElementById("consoleCmd").value.trim().toLowerCase();
      const mapUrl = { health: "/api/v1/health", devices: "/api/v1/devices", labs: "/api/v1/tests", policies: "/api/v1/policies", workers: "/api/v1/workers", git: "/api/v1/git" };
      if (!mapUrl[cmd]) return logLine(`Unknown console command '${cmd}'`, "WARN");
      const out = await apiFetch(mapUrl[cmd]);
      logLine(`$ ${cmd} -> ` + JSON.stringify(out));
    };

    document.getElementById("btnCmdPalette").onclick = openPalette;
    document.getElementById("paletteModal").onclick = (e) => { if (e.target.id === "paletteModal") e.target.classList.remove("open"); };
    document.getElementById("paletteQuery").oninput = (e) => renderPalette(e.target.value);
    window.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") { e.preventDefault(); openPalette(); }
      else if (e.key === "F5") { e.preventDefault(); refreshAll(); }
      else if (e.key === "Escape") { document.getElementById("paletteModal").classList.remove("open"); }
    });
  }

  renderNav();
  bindEvents();
  connectWS();
  refreshAll();
})();
