/**
 * Dev Laptop Sentinel Frontend Engine
 * Real-time hardware telemetry HUD, ghost process termination, port freeing, and docker maintenance.
 */

// State
let cpuHistory = [];
const MAX_HISTORY = 30;
let modalCallback = null;

// DOM Elements
const systemStatusPill = document.getElementById("system-status-pill");
const systemStatusText = document.getElementById("system-status-text");
const ghostsBadgeCount = document.getElementById("ghosts-badge-count");

// Top HUD Elements
const cpuTempVal = document.getElementById("cpu-temp-val");
const cpuLoadVal = document.getElementById("cpu-load-val");
const cpuFreqBadge = document.getElementById("cpu-freq-badge");
const cpuCoresGrid = document.getElementById("cpu-cores-grid");

const ramPressureBadge = document.getElementById("ram-pressure-badge");
const ramUsedVal = document.getElementById("ram-used-val");
const ramTotalVal = document.getElementById("ram-total-val");
const ramPctVal = document.getElementById("ram-pct-val");
const barRamUsed = document.getElementById("bar-ram-used");
const barRamCache = document.getElementById("bar-ram-cache");
const ramAppGb = document.getElementById("ram-app-gb");
const ramCacheGb = document.getElementById("ram-cache-gb");
const ramFreeGb = document.getElementById("ram-free-gb");
const swapUsedGb = document.getElementById("swap-used-gb");
const swapTotalGb = document.getElementById("swap-total-gb");
const barSwap = document.getElementById("bar-swap");

const batteryStatusBadge = document.getElementById("battery-status-badge");
const batPctVal = document.getElementById("bat-pct-val");
const batStateDesc = document.getElementById("bat-state-desc");
const batWattsVal = document.getElementById("bat-watts-val");
const batHealthVal = document.getElementById("bat-health-val");
const batWearVal = document.getElementById("bat-wear-val");
const batCyclesVal = document.getElementById("bat-cycles-val");
const batTipContainer = document.getElementById("bat-tip-container");

const nvmeTempVal = document.getElementById("nvme-temp-val");
const gpuTempVal = document.getElementById("gpu-temp-val");
const uptimeVal = document.getElementById("uptime-val");

// Tables & Counters
const tabProcsCounter = document.getElementById("tab-procs-counter");
const tabPortsCounter = document.getElementById("tab-ports-counter");
const tabDockerCounter = document.getElementById("tab-docker-counter");

const processesTbody = document.getElementById("processes-tbody");
const portsTbody = document.getElementById("ports-tbody");
const dockerTbody = document.getElementById("docker-tbody");
const cachesTbody = document.getElementById("caches-tbody");

const dockerRecImages = document.getElementById("docker-rec-images");
const dockerRecContainers = document.getElementById("docker-rec-containers");
const dockerRecBuild = document.getElementById("docker-rec-build");

// Toast and Modal
const toastContainer = document.getElementById("toast-container");
const confirmModal = document.getElementById("confirm-modal");
const modalTitle = document.getElementById("modal-title");
const modalDesc = document.getElementById("modal-desc");
const modalBtnConfirm = document.getElementById("modal-btn-confirm");
const modalBtnCancel = document.getElementById("modal-btn-cancel");

// Canvas chart
const cpuChartCanvas = document.getElementById("cpu-chart");
const chartCtx = cpuChartCanvas ? cpuChartCanvas.getContext("2d") : null;

// Initialize
document.addEventListener("DOMContentLoaded", () => {
  setupTabs();
  setupCanvas();
  setupActionButtons();

  // Initial fetch: fast HUD data first
  fetchTelemetry();
  fetchProcesses();
  fetchPorts();

  // Defer background Docker check so initial paint is instant
  setTimeout(fetchDocker, 1000);

  // Telemetry loop every 2s
  setInterval(fetchTelemetry, 2000);
  // Dev processes & ports refresh every 4s
  setInterval(() => {
    fetchProcesses();
    fetchPorts();
  }, 4000);
});

// Setup Tabs
function setupTabs() {
  const tabs = document.querySelectorAll(".tab-btn");
  tabs.forEach(btn => {
    btn.addEventListener("click", () => {
      tabs.forEach(t => t.classList.remove("active"));
      document.querySelectorAll(".tab-panel").forEach(p => p.classList.remove("active"));
      
      btn.classList.add("active");
      const target = document.getElementById(btn.dataset.tab);
      if (target) target.classList.add("active");

      // Tab specific refresh
      if (btn.dataset.tab === "tab-docker") fetchDocker();
      if (btn.dataset.tab === "tab-caches") fetchCaches();
    });
  });
}

// Canvas Setup
function setupCanvas() {
  if (!cpuChartCanvas) return;
  const dpr = window.devicePixelRatio || 1;
  const rect = cpuChartCanvas.getBoundingClientRect();
  cpuChartCanvas.width = rect.width * dpr;
  cpuChartCanvas.height = 50 * dpr;
  if (chartCtx) chartCtx.scale(dpr, dpr);
}

// Render Telemetry
async function fetchTelemetry() {
  try {
    const res = await fetch("/api/telemetry");
    if (!res.ok) return;
    const data = await res.json();
    renderTelemetry(data);
  } catch (err) {
    console.error("Telemetry fetch error:", err);
  }
}

function renderTelemetry(data) {
  // 1. Overall Status
  if (systemStatusPill) {
    systemStatusPill.className = `status-pill status-${data.status.toLowerCase()}`;
    systemStatusText.textContent = data.status_text.toUpperCase();
  }

  // 2. CPU
  const cpu = data.cpu || {};
  const pkgTemp = cpu.package_temp || 50;
  cpuTempVal.textContent = `${pkgTemp}°C`;
  cpuLoadVal.textContent = `${cpu.load_percent}%`;

  if (pkgTemp >= 88) {
    cpuTempVal.className = "hero-val temp-crit";
  } else if (pkgTemp >= 78) {
    cpuTempVal.className = "hero-val temp-hot";
  } else if (pkgTemp >= 62) {
    cpuTempVal.className = "hero-val temp-warm";
  } else {
    cpuTempVal.className = "hero-val temp-healthy";
  }

  if (cpu.current_freq_mhz) {
    cpuFreqBadge.textContent = `${(cpu.current_freq_mhz / 1000).toFixed(2)} GHz`;
  }

  // Core bars
  if (cpu.load_per_core && cpuCoresGrid) {
    cpuCoresGrid.innerHTML = cpu.load_per_core.slice(0, 6).map((load, idx) => `
      <div class="core-pill">
        <div class="core-bar">
          <div class="core-fill" style="height: ${Math.max(8, load)}%;"></div>
        </div>
        <span>C${idx}: ${Math.round(load)}%</span>
      </div>
    `).join("");
  }

  // Add to chart
  cpuHistory.push(pkgTemp);
  if (cpuHistory.length > MAX_HISTORY) cpuHistory.shift();
  drawCpuChart();

  // 3. RAM
  const ram = data.ram || {};
  ramUsedVal.textContent = `${ram.used_gb} GB`;
  ramTotalVal.textContent = `${ram.total_gb} GB`;
  ramPctVal.textContent = `${ram.percent}%`;
  
  ramAppGb.textContent = `${ram.used_gb} GB`;
  ramCacheGb.textContent = `${ram.cached_gb} GB`;
  ramFreeGb.textContent = `${ram.available_gb} GB`;

  barRamUsed.style.width = `${ram.percent}%`;
  const cachePct = ((ram.cached_gb / ram.total_gb) * 100).toFixed(0);
  barRamCache.style.width = `${cachePct}%`;

  ramPressureBadge.textContent = ram.pressure;
  if (ram.pressure === "CRITICAL") {
    ramPressureBadge.className = "metric-badge badge-amber text-crimson";
  } else if (ram.pressure === "WARNING") {
    ramPressureBadge.className = "metric-badge badge-amber";
  } else {
    ramPressureBadge.className = "metric-badge badge-emerald";
  }

  swapUsedGb.textContent = `${ram.swap_used_gb} GB`;
  swapTotalGb.textContent = `${ram.swap_total_gb} GB`;
  barSwap.style.width = `${ram.swap_percent}%`;

  // 4. Battery
  const bat = data.battery || {};
  batPctVal.textContent = `${bat.percent}%`;
  batStateDesc.textContent = bat.is_plugged ? "Подключен к сети (AC)" : "Работа от аккумулятора";
  
  if (bat.is_plugged) {
    batteryStatusBadge.textContent = bat.status === "Charging" ? "Зарядка" : "Сеть (100%)";
    batteryStatusBadge.className = "metric-badge badge-blue";
  } else {
    batteryStatusBadge.textContent = "Разряд";
    batteryStatusBadge.className = "metric-badge badge-amber";
  }

  batWattsVal.textContent = `${bat.watts} W`;
  batHealthVal.textContent = `${bat.health_pct}%`;
  batWearVal.textContent = `${bat.wear_pct}%`;
  batCyclesVal.textContent = `${bat.cycle_count} циклов`;

  if (bat.tips && bat.tips.length > 0) {
    batTipContainer.innerHTML = `<strong>Рекомендация:</strong> ${bat.tips[0]}`;
    batTipContainer.style.display = "block";
  } else {
    batTipContainer.style.display = "none";
  }

  // 5. Storage & GPU
  if (data.nvme_temp !== null) {
    nvmeTempVal.textContent = `${data.nvme_temp}°C`;
  }
  const gpu = data.gpu || {};
  if (gpu.temp !== null && gpu.temp !== undefined) {
    gpuTempVal.textContent = `${gpu.temp}°C`;
  } else {
    gpuTempVal.textContent = gpu.mode === "intel" ? "Intel iGPU" : (gpu.mode || "Hybrid");
  }

  const gpuModeBadge = document.getElementById("gpu-mode-badge");
  if (gpuModeBadge) {
    const mode = gpu.mode || "unknown";
    if (mode === "on-demand") {
      gpuModeBadge.className = "metric-badge badge-emerald";
      gpuModeBadge.textContent = "On-Demand (Гибрид)";
    } else if (mode === "intel") {
      gpuModeBadge.className = "metric-badge badge-amber";
      gpuModeBadge.textContent = "Intel Only (iGPU)";
    } else if (mode === "nvidia") {
      gpuModeBadge.className = "metric-badge badge-violet";
      gpuModeBadge.textContent = "NVIDIA Only (dGPU)";
    } else {
      gpuModeBadge.textContent = mode;
    }
  }

  // Uptime
  if (data.uptime) {
    const hours = Math.floor((Date.now() / 1000 - data.uptime) / 3600);
    const mins = Math.floor(((Date.now() / 1000 - data.uptime) % 3600) / 60);
    uptimeVal.textContent = `${hours}ч ${mins}мин`;
  }
}

// Draw CPU Sparkline
function drawCpuChart() {
  if (!chartCtx || !cpuChartCanvas || cpuHistory.length < 2) return;
  const w = cpuChartCanvas.getBoundingClientRect().width;
  const h = 50;
  chartCtx.clearRect(0, 0, w, h);

  const min = 35;
  const max = 95;
  const step = w / (MAX_HISTORY - 1);

  // Gradient
  const grad = chartCtx.createLinearGradient(0, 0, 0, h);
  grad.addColorStop(0, "rgba(0, 240, 255, 0.4)");
  grad.addColorStop(1, "rgba(0, 240, 255, 0.0)");

  chartCtx.beginPath();
  cpuHistory.forEach((temp, i) => {
    const x = i * step;
    const y = h - Math.max(4, Math.min(h - 4, ((temp - min) / (max - min)) * h));
    if (i === 0) chartCtx.moveTo(x, y);
    else chartCtx.lineTo(x, y);
  });

  chartCtx.strokeStyle = "#00f0ff";
  chartCtx.lineWidth = 2;
  chartCtx.stroke();

  // Area fill
  chartCtx.lineTo((cpuHistory.length - 1) * step, h);
  chartCtx.lineTo(0, h);
  chartCtx.closePath();
  chartCtx.fillStyle = grad;
  chartCtx.fill();
}

// Fetch Dev Processes
async function fetchProcesses() {
  try {
    const res = await fetch("/api/processes");
    if (!res.ok) return;
    const list = await res.json();
    renderProcesses(list);
  } catch (err) {
    console.error("Processes error:", err);
  }
}

function renderProcesses(list) {
  tabProcsCounter.textContent = list.length;
  const ghosts = list.filter(p => p.is_ghost);
  ghostsBadgeCount.textContent = ghosts.length;

  if (list.length === 0) {
    processesTbody.innerHTML = `<tr><td colspan="8" class="text-center py-4 text-muted">Dev-процессы не обнаружены</td></tr>`;
    return;
  }

  processesTbody.innerHTML = list.map(p => {
    const catClass = `cat-${p.category}`;
    const ghostBadge = p.is_ghost 
      ? `<span class="metric-badge badge-amber">Призрак</span>` 
      : (p.is_service 
          ? `<span class="metric-badge badge-blue">Сервис</span>` 
          : `<span class="metric-badge badge-emerald">Активен</span>`);
    
    const displayTitle = p.title || p.name;
    const displayProject = p.project || "Системный / Утилита";
    const displaySub = p.subtitle || p.cmdline;

    return `
      <tr class="${p.is_ghost ? 'row-ghost' : ''}">
        <td><span class="category-tag ${catClass}">${escapeHtml(p.category.toUpperCase())}</span></td>
        <td>
          <div style="font-weight: 600; color: #f8fafc; font-size: 0.92rem; line-height: 1.25;">${escapeHtml(displayTitle)}</div>
          <div style="font-size: 0.74rem; color: #94a3b8; margin-top: 3px;">
            <span>${escapeHtml(p.name)}</span> • <span class="text-dim">PID ${p.pid}</span>
          </div>
        </td>
        <td class="cmd-cell" title="${escapeHtml(p.cmdline)}">
          <div style="color: #38bdf8; font-weight: 600; font-size: 0.82rem; margin-bottom: 2px;">${escapeHtml(displayProject)}</div>
          <div style="color: #cbd5e1; font-size: 0.76rem; text-overflow: ellipsis; overflow: hidden; white-space: nowrap; max-width: 320px;">
            ${escapeHtml(displaySub)}
          </div>
        </td>
        <td><strong>${p.memory_mb} MB</strong></td>
        <td>${p.cpu_percent}%</td>
        <td>${p.uptime_hours} ч</td>
        <td>${ghostBadge}</td>
        <td>
          <button class="btn btn-danger" onclick="requestKillProcess(${p.pid}, '${escapeHtml(displayTitle)}')">
            Завершить
          </button>
        </td>
      </tr>
    `;
  }).join("");
}

// Fetch Ports
async function fetchPorts() {
  try {
    const res = await fetch("/api/ports");
    if (!res.ok) return;
    const list = await res.json();
    renderPorts(list);
  } catch (err) {
    console.error("Ports error:", err);
  }
}

function renderPorts(list) {
  tabPortsCounter.textContent = list.length;
  if (list.length === 0) {
    portsTbody.innerHTML = `<tr><td colspan="7" class="text-center py-4 text-muted">Нет прослушиваемых портов</td></tr>`;
    return;
  }

  portsTbody.innerHTML = list.map(p => `
    <tr>
      <td><span class="port-pill">:${p.port}</span></td>
      <td><strong>${p.hint}</strong></td>
      <td>${p.name}</td>
      <td><span class="text-dim">${p.pid || 'System'}</span></td>
      <td>${p.memory_mb ? p.memory_mb + ' MB' : '--'}</td>
      <td class="cmd-cell" title="${escapeHtml(p.cmdline)}">${escapeHtml(p.cmdline || p.ip)}</td>
      <td>
        ${p.can_kill ? `
          <button class="btn btn-free-port" onclick="requestFreePort(${p.port})">
            Освободить
          </button>
        ` : `<span class="text-dim text-xs">Системный</span>`}
      </td>
    </tr>
  `).join("");
}

// Fetch Docker
async function fetchDocker() {
  try {
    const res = await fetch("/api/docker");
    if (!res.ok) return;
    const data = await res.json();
    renderDocker(data);
  } catch (err) {
    console.error("Docker error:", err);
  }
}

function renderDocker(data) {
  const runningBadge = document.getElementById("docker-running-badge");
  const stoppedBadge = document.getElementById("docker-stopped-badge");

  if (!data.available) {
    dockerTbody.innerHTML = `<tr><td colspan="7" class="text-center py-4 text-muted">${data.reclaimable_summary}</td></tr>`;
    return;
  }

  tabDockerCounter.textContent = data.containers.length;
  if (runningBadge) runningBadge.textContent = `${data.running_count || 0} Запущено`;
  if (stoppedBadge) stoppedBadge.textContent = `${data.stopped_count || 0} Остановлено`;

  const df = data.disk || {};
  dockerRecImages.textContent = df.images_reclaimable || "0 B";
  dockerRecContainers.textContent = df.containers_reclaimable || "0 B";
  dockerRecBuild.textContent = df.build_cache_reclaimable || "0 B";

  if (data.containers.length === 0) {
    dockerTbody.innerHTML = `<tr><td colspan="7" class="text-center py-4 text-muted">Контейнеры отсутствуют</td></tr>`;
    return;
  }

  dockerTbody.innerHTML = data.containers.map(c => {
    const stats = c.stats || {};
    const statusBadge = c.is_running 
      ? `<span class="metric-badge badge-emerald" style="display:inline-flex;align-items:center;gap:4px;"><span class="pulsing-dot" style="width:6px;height:6px;"></span>Запущен</span>` 
      : `<span class="metric-badge badge-amber">Остановлен</span>`;
    
    const cpuVal = stats.cpu || "--";
    const memVal = stats.mem || "--";

    return `
      <tr class="${c.is_running ? 'row-active-container' : ''}">
        <td><strong>${escapeHtml(c.name)}</strong> <span class="text-dim">(${escapeHtml(c.id)})</span></td>
        <td class="cmd-cell" title="${escapeHtml(c.image)}">${escapeHtml(c.image)}</td>
        <td>${statusBadge}</td>
        <td><strong class="${c.is_running ? 'text-cyan' : 'text-dim'}">${escapeHtml(cpuVal)}</strong></td>
        <td><span class="${c.is_running ? 'text-violet' : 'text-dim'}">${escapeHtml(memVal)}</span></td>
        <td class="cmd-cell" title="${escapeHtml(c.ports || '--')}">${escapeHtml(c.ports || '--')}</td>
        <td>
          <div style="display:flex;gap:6px;">
            ${c.is_running ? `
              <button class="btn btn-warning btn-sm" onclick="requestStopContainer('${c.id}', '${escapeHtml(c.name)}')">
                Стоп
              </button>
              <button class="btn btn-secondary btn-sm" title="Перезапустить" onclick="requestRestartContainer('${c.id}', '${escapeHtml(c.name)}')">
                Перезапуск
              </button>
            ` : `
              <button class="btn btn-secondary btn-sm" onclick="requestStartContainer('${c.id}', '${escapeHtml(c.name)}')">
                Старт
              </button>
            `}
            <button class="btn btn-secondary btn-sm" title="Посмотреть логи" onclick="viewContainerLogs('${c.id}', '${escapeHtml(c.name)}')">
              Логи
            </button>
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

// Fetch Caches
async function fetchCaches() {
  try {
    const res = await fetch("/api/caches");
    if (!res.ok) return;
    const list = await res.json();
    renderCaches(list);
  } catch (err) {
    console.error("Caches error:", err);
  }
}

function renderCaches(list) {
  if (list.length === 0) {
    cachesTbody.innerHTML = `<tr><td colspan="5" class="text-center py-4 text-muted">Кэши не найдены</td></tr>`;
    return;
  }

  cachesTbody.innerHTML = list.map(c => `
    <tr>
      <td><strong>${escapeHtml(c.name)}</strong></td>
      <td>${escapeHtml(c.desc)}</td>
      <td class="cmd-cell" title="${escapeHtml(c.path)}">${escapeHtml(c.path)}</td>
      <td><span class="stat-value text-cyan" style="font-size: 1rem;">${c.size_display}</span></td>
      <td>
        <button class="btn btn-secondary" onclick="requestCleanCache('${escapeHtml(c.name)}')">
          Очистить
        </button>
      </td>
    </tr>
  `).join("");
}

// User Actions & Modals
function setupActionButtons() {
  document.getElementById("btn-refresh").addEventListener("click", () => {
    fetchTelemetry();
    fetchProcesses();
    fetchPorts();
    fetchDocker();
    fetchCaches();
    showToast("Данные обновлены", "success");
  });

  document.getElementById("btn-quick-kill-ghosts").addEventListener("click", () => {
    killGhostsAction();
  });

  document.getElementById("btn-kill-all-ghosts").addEventListener("click", () => {
    killGhostsAction();
  });

  document.getElementById("btn-quick-prune-docker").addEventListener("click", () => {
    pruneDockerAction("stopped");
  });

  document.getElementById("btn-prune-stopped").addEventListener("click", () => {
    pruneDockerAction("stopped");
  });

  document.getElementById("btn-prune-images").addEventListener("click", () => {
    pruneDockerAction("images");
  });

  document.getElementById("btn-prune-system").addEventListener("click", () => {
    showConfirm(
      "Полная очистка Docker",
      "Будут удалены все остановленные контейнеры, неиспользуемые сети и образы. Продолжить?",
      () => pruneDockerAction("system")
    );
  });

  modalBtnCancel.addEventListener("click", () => {
    confirmModal.classList.remove("open");
    modalCallback = null;
  });

  modalBtnConfirm.addEventListener("click", () => {
    confirmModal.classList.remove("open");
    if (modalCallback) modalCallback();
    modalCallback = null;
  });

  const logsModalClose = document.getElementById("logs-modal-close");
  const logsModal = document.getElementById("logs-modal");
  if (logsModalClose && logsModal) {
    logsModalClose.addEventListener("click", () => {
      logsModal.classList.remove("open");
    });
    logsModal.addEventListener("click", (e) => {
      if (e.target === logsModal) logsModal.classList.remove("open");
    });
  }
}

function showConfirm(title, desc, onConfirm) {
  modalTitle.textContent = title;
  modalDesc.textContent = desc;
  modalCallback = onConfirm;
  confirmModal.classList.add("open");
}

function showToast(message, type = "success") {
  const toast = document.createElement("div");
  toast.className = `toast toast-${type}`;
  const iconSvg = type === 'success'
    ? '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>'
    : '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>';
  toast.innerHTML = `<span class="toast-icon">${iconSvg}</span> <div>${escapeHtml(message)}</div>`;
  toastContainer.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = "0";
    setTimeout(() => toast.remove(), 300);
  }, 3500);
}

// Actions Execution
window.requestKillProcess = function(pid, name) {
  showConfirm(
    "Завершение процесса",
    `Вы действительно хотите завершить процесс ${name} (PID: ${pid})?`,
    async () => {
      try {
        const res = await fetch("/api/processes/kill", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ pid, force: false })
        });
        const data = await res.json();
        if (res.ok) {
          showToast(data.message, "success");
          fetchProcesses();
        } else {
          showToast(data.detail || "Ошибка завершения", "error");
        }
      } catch (err) {
        showToast("Сетевая ошибка", "error");
      }
    }
  );
};

window.requestFreePort = function(port) {
  showConfirm(
    "Освобождение порта",
    `Завершить процесс, занимающий порт ${port}?`,
    async () => {
      try {
        const res = await fetch("/api/ports/free", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ port })
        });
        const data = await res.json();
        if (res.ok) {
          showToast(data.message, "success");
          fetchPorts();
        } else {
          showToast(data.detail || "Не удалось освободить", "error");
        }
      } catch (err) {
        showToast("Сетевая ошибка", "error");
      }
    }
  );
};

window.requestCleanCache = function(name) {
  showConfirm(
    "Очистка кэша",
    `Очистить кэш ${name}? Это безопасно, но следующие сборки могут загрузить пакеты заново.`,
    async () => {
      try {
        const res = await fetch("/api/caches/clean", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name })
        });
        const data = await res.json();
        if (res.ok) {
          showToast(data.message, "success");
          fetchCaches();
        } else {
          showToast(data.detail || "Ошибка очистки", "error");
        }
      } catch (err) {
        showToast("Ошибка при очистке", "error");
      }
    }
  );
};

async function killGhostsAction() {
  try {
    const res = await fetch("/api/processes/kill_ghosts", { method: "POST" });
    const data = await res.json();
    if (res.ok) {
      showToast(data.message, "success");
      fetchProcesses();
      fetchTelemetry();
    } else {
      showToast("Ошибка завершения призраков", "error");
    }
  } catch (err) {
    showToast("Сетевая ошибка", "error");
  }
}

async function pruneDockerAction(mode) {
  try {
    const res = await fetch("/api/docker/prune", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode })
    });
    const data = await res.json();
    if (res.ok) {
      showToast(`Docker ${mode} prune выполнен!`, "success");
      fetchDocker();
    } else {
      showToast(data.detail || "Ошибка Docker", "error");
    }
  } catch (err) {
    showToast("Ошибка Docker", "error");
  }
}

window.requestStartContainer = async function(id, name) {
  try {
    const res = await fetch("/api/docker/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id })
    });
    const data = await res.json();
    if (res.ok) {
      showToast(data.message || `Контейнер ${name} запущен!`, "success");
      fetchDocker();
    } else {
      showToast(data.detail || "Ошибка запуска контейнера", "error");
    }
  } catch (err) {
    showToast("Сетевая ошибка", "error");
  }
};

window.requestStopContainer = function(id, name) {
  showConfirm(
    "Остановка контейнера",
    `Остановить контейнер ${name} (${id})?`,
    async () => {
      try {
        const res = await fetch("/api/docker/stop", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ id })
        });
        const data = await res.json();
        if (res.ok) {
          showToast(data.message || `Контейнер ${name} остановлен`, "success");
          fetchDocker();
        } else {
          showToast(data.detail || "Ошибка остановки", "error");
        }
      } catch (err) {
        showToast("Сетевая ошибка", "error");
      }
    }
  );
};

window.requestRestartContainer = function(id, name) {
  showConfirm(
    "Перезапуск контейнера",
    `Перезапустить контейнер ${name} (${id})?`,
    async () => {
      try {
        const res = await fetch("/api/docker/restart", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ id })
        });
        const data = await res.json();
        if (res.ok) {
          showToast(data.message || `Контейнер ${name} перезапущен`, "success");
          fetchDocker();
        } else {
          showToast(data.detail || "Ошибка перезапуска", "error");
        }
      } catch (err) {
        showToast("Сетевая ошибка", "error");
      }
    }
  );
};

window.viewContainerLogs = async function(id, name) {
  const modal = document.getElementById("logs-modal");
  const title = document.getElementById("logs-modal-title");
  const content = document.getElementById("logs-modal-content");
  if (!modal || !content) return;

  title.textContent = `Логи: ${name} (${id})`;
  content.textContent = "Загрузка логов...";
  modal.classList.add("open");

  try {
    const res = await fetch(`/api/docker/logs/${id}`);
    const data = await res.json();
    if (res.ok) {
      content.textContent = data.logs || "Логи пусты";
    } else {
      content.textContent = `Ошибка: ${data.detail || 'Не удалось получить логи'}`;
    }
  } catch (err) {
    content.textContent = "Сетевая ошибка при получении логов";
  }
};

function escapeHtml(str) {
  if (!str) return "";
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

// GPU Mode Switcher
window.openGpuModal = function() {
  const modal = document.getElementById("gpu-modal");
  if (modal) modal.classList.add("active");
  const cmdBox = document.getElementById("gpu-switch-cmd-box");
  if (cmdBox) cmdBox.style.display = "none";
};

window.closeGpuModal = function() {
  const modal = document.getElementById("gpu-modal");
  if (modal) modal.classList.remove("active");
};

window.applyGpuSwitch = async function() {
  const selected = document.querySelector('input[name="gpu-mode-choice"]:checked');
  if (!selected) return;
  const targetMode = selected.value;
  const btn = document.getElementById("btn-apply-gpu");
  const cmdBox = document.getElementById("gpu-switch-cmd-box");

  if (btn) {
    btn.disabled = true;
    btn.textContent = "Переключение...";
  }

  try {
    const res = await fetch("/api/gpu/switch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode: targetMode })
    });
    const data = await res.json();
    if (data.success) {
      showToast(data.message, "success");
      closeGpuModal();
      fetchTelemetry();
    } else {
      if (cmdBox && data.command) {
        cmdBox.style.display = "block";
        cmdBox.innerHTML = `<strong>Требуются права root:</strong><br>Скопируйте и выполните команду в терминале:<br><code style="color:#00f0ff; user-select: all; display:block; margin-top:5px; padding:6px; background:rgba(0,0,0,0.4); border-radius:4px;">${escapeHtml(data.command)}</code>`;
      }
      showToast(data.message || "Требуется авторизация в терминале", "warning");
    }
  } catch (err) {
    showToast("Ошибка смены режима: " + err, "error");
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = "Применить режим";
    }
  }
};
