const state = {
  jobs: [],
  adapters: [],
  tasks: [],
  runs: [],
  records: [],
};

const $ = (selector) => document.querySelector(selector);

function emptyNode() {
  return $("#emptyTemplate").content.firstElementChild.cloneNode(true);
}

async function getJSON(path) {
  const response = await fetch(path);
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`${response.status} ${response.statusText}: ${body}`);
  }
  return response.json();
}

async function postJSON(path) {
  const response = await fetch(path, { method: "POST" });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`${response.status} ${response.statusText}: ${body}`);
  }
  return response.json();
}

async function refreshAll() {
  $("#refreshButton").disabled = true;
  const results = await Promise.allSettled([
    getJSON("/healthz"),
    getJSON("/v1/adapters"),
    getJSON("/v1/jobs"),
    getJSON("/v1/tasks?limit=50"),
    getJSON("/v1/runs?limit=8"),
    getJSON("/v1/records?limit=10"),
  ]);

  applyResult(results[0], renderHealth, renderHealthError);
  applyResult(results[1], renderAdapters, (error) => renderPanelError("#adapters", error));
  applyResult(results[2], renderJobs, (error) => {
    renderJobLoadError(error);
    $("#runButton").disabled = true;
  });
  applyResult(results[3], renderTasks, (error) => renderPanelError("#tasks", error));
  applyResult(results[4], renderRuns, (error) => renderPanelError("#runs", error));
  applyResult(results[5], renderRecords, (error) => renderPanelError("#records", error));
  $("#refreshButton").disabled = false;
}

function applyResult(result, onFulfilled, onRejected) {
  if (result.status === "fulfilled") {
    onFulfilled(result.value);
    return;
  }
  onRejected(result.reason);
}

function renderPanelError(selector, error) {
  const container = $(selector);
  container.innerHTML = "";
  const node = emptyNode();
  node.textContent = error.message;
  container.append(node);
}

function renderJobLoadError(error) {
  const select = $("#jobSelect");
  select.innerHTML = "";
  const option = document.createElement("option");
  option.value = "";
  option.textContent = error.message;
  select.append(option);
}

function renderHealth(health) {
  const database = health.database || {};
  $("#healthTitle").textContent = health.status === "ok" ? "Everything looks alive" : "Needs attention";
  $("#healthDetail").textContent =
    database.connected && database.initialized
      ? "API is up, PostgreSQL is connected, and required tables are initialized."
      : JSON.stringify(health, null, 2);
  $("#healthBadge").textContent = health.status || "Unknown";
  $("#healthBadge").className = `badge ${health.status === "ok" ? "ok" : "bad"}`;
}

function renderHealthError(error) {
  $("#healthTitle").textContent = "API is unreachable";
  $("#healthDetail").textContent = error.message;
  $("#healthBadge").textContent = "Error";
  $("#healthBadge").className = "badge bad";
}

function renderJobs(jobs) {
  const select = $("#jobSelect");
  select.innerHTML = "";
  for (const job of jobs) {
    const option = document.createElement("option");
    option.value = job.id;
    option.textContent = `${job.name || job.id} (${job.adapter})`;
    select.append(option);
  }
  $("#runButton").disabled = jobs.length === 0;
}

function renderAdapters(adapters) {
  const container = $("#adapters");
  container.innerHTML = "";
  if (!adapters.length) {
    container.append(emptyNode());
    return;
  }

  for (const adapter of adapters) {
    container.append(
      item({
        title: adapter.name,
        subtitle: adapter.kind,
        chips: [adapter.kind],
      }),
    );
  }
}

function renderTasks(tasks) {
  const container = $("#tasks");
  container.innerHTML = "";
  $("#taskCount").textContent = `${tasks.length}`;
  if (!tasks.length) {
    container.append(emptyNode());
    return;
  }

  for (const task of tasks) {
    container.append(
      item({
        title: task.name || task.key,
        subtitle: task.key,
        chips: [
          task.status,
          task.enabled ? "enabled" : "disabled",
          `${task.interval_seconds}s interval`,
          `${task.timeout_seconds}s timeout`,
        ],
      }),
    );
  }
}

function renderRuns(runs) {
  const container = $("#runs");
  container.innerHTML = "";
  if (!runs.length) {
    container.append(emptyNode());
    return;
  }

  for (const run of runs) {
    container.append(
      item({
        title: run.job_id,
        subtitle: run.summary || run.error || run.id,
        chips: [run.status, run.adapter, formatDate(run.started_at)],
      }),
    );
  }
}

function renderRecords(records) {
  const container = $("#records");
  container.innerHTML = "";
  if (!records.length) {
    container.append(emptyNode());
    return;
  }

  const table = document.createElement("table");
  table.innerHTML = `
    <thead>
      <tr>
        <th>Observed</th>
        <th>Channel</th>
        <th>Type</th>
        <th>Payload</th>
      </tr>
    </thead>
    <tbody></tbody>
  `;
  const tbody = table.querySelector("tbody");
  for (const record of records) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${escapeHTML(formatDate(record.observed_at))}</td>
      <td>${escapeHTML(record.channel)}</td>
      <td>${escapeHTML(record.record_type)}</td>
      <td><pre>${escapeHTML(JSON.stringify(record.payload, null, 2))}</pre></td>
    `;
    tbody.append(tr);
  }
  container.append(table);
}

function item({ title, subtitle, chips }) {
  const element = document.createElement("article");
  element.className = "item";
  const chipHTML = chips
    .filter(Boolean)
    .map((chip) => `<span class="chip ${escapeHTML(String(chip))}">${escapeHTML(String(chip))}</span>`)
    .join("");
  element.innerHTML = `
    <div class="item-header">
      <div>
        <div class="item-title">${escapeHTML(title || "Untitled")}</div>
        <p class="muted">${escapeHTML(subtitle || "")}</p>
      </div>
    </div>
    <div class="meta">${chipHTML}</div>
  `;
  return element;
}

async function runSelectedJob() {
  const jobID = $("#jobSelect").value;
  if (!jobID) return;

  $("#runButton").disabled = true;
  $("#runMessage").textContent = `Running ${jobID}...`;
  try {
    const run = await postJSON(`/v1/jobs/${encodeURIComponent(jobID)}/runs`);
    $("#runMessage").textContent = `Run ${run.status}: ${run.id}`;
    await refreshAll();
  } catch (error) {
    $("#runMessage").textContent = error.message;
  } finally {
    $("#runButton").disabled = false;
  }
}

function formatDate(value) {
  if (!value) return "";
  return new Intl.DateTimeFormat(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value));
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

$("#refreshButton").addEventListener("click", refreshAll);
$("#runButton").addEventListener("click", runSelectedJob);
refreshAll();
