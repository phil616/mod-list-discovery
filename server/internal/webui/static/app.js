"use strict";
const U = window.UI,
  content = document.querySelector("#content");
let csrf = "",
  settings = {},
  servers = [];
const enc = encodeURIComponent;
const labels = {
  dashboard: "工作台",
  servers: "我的服务器",
  artifacts: "MOD 文件库",
  releases: "环境版本",
  clients: "玩家设备与邀请",
  settings: "系统设置",
  guide: "入门指南",
};
const requirement = {
  required: "必须安装",
  optional: "玩家自选",
  forbidden: "禁止安装",
};
const published = (r) => r.status === "published";
const serverName = (id) => servers.find((s) => s.id === id)?.display_name || id;
const query = new URLSearchParams(location.search);
function serverURL(id) {
  return settings.publicBaseUrl.replace(/\/$/, "") + "/" + id;
}
async function api(path, method = "GET", body) {
  let response;
  try {
    response = await fetch("/api/v1/admin/" + path, {
      method,
      headers: {
        "X-CSRF-Token": csrf,
        ...(body && !(body instanceof FormData)
          ? { "Content-Type": "application/json" }
          : {}),
      },
      body:
        body instanceof FormData
          ? body
          : body
            ? JSON.stringify(body)
            : undefined,
    });
  } catch {
    throw new Error("无法连接管理服务，请检查网络连接后重试。");
  }
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("服务返回了无法识别的响应，请稍后重试。");
  }
  if (!response.ok) {
    if (response.status === 401 && path !== "login") location.assign("/login");
    const e = new Error("操作未完成，请刷新数据后重试。");
    e.code = data.error;
    e.detail = data.message;
    throw e;
  }
  return data;
}
function go(url, message) {
  if (message) sessionStorage.setItem("mld.notice", message);
  location.assign(url);
}
async function refresh(message) {
  await render();
  if (message) U.toast(message);
}
function heading(title, description, ...actions) {
  content.append(
    U.box(
      "page-heading",
      U.box("", U.el("h1", title), U.el("p", description)),
      U.box("d-flex gap-2", ...actions),
    ),
  );
}
function section(title, meta = "") {
  content.append(
    U.box("section-heading", U.el("h2", title), U.el("span", meta)),
  );
}
function status(r) {
  return U.badge(
    published(r) ? "已封存" : "草稿",
    published(r) ? "success" : "warning",
  );
}
function visibility(s) {
  return U.badge(
    s.visibility === "private" ? "需要邀请码" : "公开访问",
    s.visibility === "private" ? "primary" : "success",
  );
}
function help(parent, title, text) {
  const p = U.panel(title, "", parent);
  p.card.classList.add("help-panel");
  p.body.append(U.el("p", text), U.link("查看完整使用指南 →", "/admin/guide"));
  return p;
}
function serverChoices(onlyPrivate = false) {
  return servers
    .filter((s) => !onlyPrivate || s.visibility === "private")
    .map((s) => [s.id, `${s.display_name}（${s.id}）`]);
}
function workflow(parent, states, links) {
  const cards = U.box("workflow");
  const names = ["创建服务器", "上传 MOD 文件", "搭配环境版本", "启用并分享"];
  const desc = [
    "为一群玩家建立专属的 MOD 清单。",
    "上传玩家需要安装的 Fabric JAR。",
    "选择文件，设置必须安装或玩家自选。",
    "把完整的一版设为当前使用，再把地址发给玩家。",
  ];
  const first = states.findIndex((v) => !v);
  names.forEach((name, i) => {
    const card = U.box(
      `card step-card ${states[i] ? "done" : i === first ? "current" : ""}`,
      U.box("step-number", U.el("span", states[i] ? "✓" : String(i + 1))),
      U.el("h3", name),
      U.el("p", desc[i]),
      U.link(states[i] ? "查看 →" : "开始这一步 →", links[i]),
    );
    if (states[i]) card.append(U.badge("已完成", "success"));
    cards.append(card);
  });
  parent.append(cards);
}
async function dashboard() {
  const [overview, releases, channels] = await Promise.all([
    api("overview"),
    api("releases"),
    Promise.all(servers.map((s) => api(`servers/${enc(s.id)}/channels`))),
  ]);
  heading(
    "每位玩家，都用对的 MOD。",
    "从准备文件到交给玩家，在这里管理每一版游戏环境。",
    U.button("＋ 创建服务器", () => serverDialog()),
  );
  const hasActive = channels.flat().some((c) => c.active_release_id);
  const welcome = U.box(
    "welcome",
    U.box(
      "",
      U.el("span", "GET STARTED · 从这里开始", { className: "eyebrow" }),
      U.el(
        "h2",
        hasActive
          ? "环境已就绪，接下来邀请玩家加入。"
          : "四个步骤，准备好玩家的游戏环境。",
      ),
      U.el(
        "p",
        "这里管理的是玩家需要的 MOD 清单，不是 Minecraft 游戏服务器。你上传文件、搭配一版并启用，玩家通过一个地址完成检查与同步。",
      ),
    ),
    U.box("welcome-art", U.box("stack", U.el("span", "▦"))),
  );
  content.append(welcome);
  const stats = U.box("stat-grid");
  [
    ["◈", "我的服务器", overview.servers],
    ["▦", "已上传 MOD 文件", overview.artifacts],
    ["◷", "环境版本", overview.releases],
    ["♧", "已登记设备", overview.client_devices],
  ].forEach(([icon, label, value]) =>
    stats.append(
      U.box(
        "card stat-card",
        U.box("stat-icon", U.el("span", icon)),
        U.box(
          "",
          U.el("div", label, { className: "stat-label" }),
          U.el("div", String(value), { className: "stat-value" }),
        ),
      ),
    ),
  );
  content.append(stats);
  section("你的上手清单", "按顺序完成，随时可以回来继续");
  workflow(
    content,
    [
      servers.length > 0,
      overview.artifacts > 0,
      releases.some(published),
      hasActive,
    ],
    [
      "/admin/servers",
      "/admin/artifacts",
      "/admin/releases",
      servers[0] ? "/admin/servers/" + enc(servers[0].id) : "/admin/servers",
    ],
  );
  section("我的服务器", `${servers.length} 个服务器`);
  if (!servers.length) {
    const split = U.box("split");
    const card = U.box(
      "card",
      U.empty(
        "先给你的服务器起个名字",
        "例如“朋友们的生存服”。这会建立一份独立的 MOD 清单，不会启动或改变游戏服务器。",
        U.button("创建第一个服务器", () => serverDialog()),
      ),
    );
    split.append(card);
    help(
      split,
      "上传文件，不等于发给玩家",
      "文件库只是仓库。还要把文件加入一份环境版本，并启用这一版，玩家才能获取对应清单。",
    );
    content.append(split);
  } else serverCards(servers, channels, releases);
}
function serverCards(rows, channels = [], releases = []) {
  const grid = U.box("server-grid");
  rows.forEach((s, index) => {
    const active = channels[index]?.find(
      (c) => c.name === "stable",
    )?.active_release_id;
    const release = releases.find((r) => r.id === active);
    const card = U.box("card server-card"),
      body = U.box("card-body");
    body.append(
      U.box(
        "server-title",
        U.box("", U.el("h3", s.display_name), U.el("p", `地址名称：${s.id}`)),
        visibility(s),
      ),
    );
    body.append(
      U.el("p", s.description || "独立管理这个服务器的玩家 MOD 环境。", {
        className: "subtle mb-0",
      }),
    );
    if (channels.length)
      body.append(
        U.el(
          "div",
          active
            ? "当前使用：" + (release?.name || `第 ${active} 版`)
            : "尚未启用环境版本 · 完成配置后再分享给玩家",
          { className: "server-meta" },
        ),
      );
    body.append(
      U.box(
        "d-flex justify-content-between align-items-center",
        U.link("管理这个服务器 →", "/admin/servers/" + enc(s.id)),
        U.button("编辑", () => serverDialog(s), "btn-sm btn-outline-secondary"),
      ),
    );
    card.append(body);
    grid.append(card);
  });
  content.append(grid);
}
function serverDialog(existing) {
  const m = U.modal(
    existing ? "编辑服务器信息" : "创建你的服务器",
    "这里的服务器是一份玩家 MOD 环境配置。无需填写游戏服务器 IP，也不会自动读取游戏服务器的 mods 目录。",
  );
  const fields = [
    {
      name: "displayName",
      label: "玩家看到的名称",
      help: "给它一个容易辨认的名字，例如“朋友们的生存服”。",
      placeholder: "朋友们的生存服",
      value: existing?.display_name || "",
    },
  ];
  if (!existing)
    fields.push({
      name: "id",
      label: "地址中的名称",
      help: "用于生成分享地址，创建后不可修改。1–64 个字符，只能用小写字母、数字和短横线；不能使用 admin、api 等系统名称。",
      value: "my-server",
      pattern: "[a-z0-9][a-z0-9-]{0,63}",
    });
  fields.push(
    {
      name: "visibility",
      label: "谁可以获取这份清单？",
      choices: [
        ["public", "公开：知道地址的人都可以使用"],
        ["private", "私密：玩家必须先输入邀请码"],
      ],
      value: existing?.visibility || "public",
      help: "公开适合开放社区；私密适合朋友服。邀请码只授权 MOD 同步，不是 Minecraft 游戏服务器的白名单。",
    },
    {
      name: "description",
      label: "给玩家的介绍",
      type: "textarea",
      value: existing?.description || "",
      required: false,
      placeholder: "例如：适合轻量生存体验，需要安装 Fabric。",
      help: "选填，可以说明玩法或这套 MOD 的用途。",
    },
  );
  const f = U.form(
    m.body,
    fields,
    existing ? "保存服务器信息" : "创建服务器，继续上传 MOD",
    async (b) => {
      if (
        !existing &&
        ["admin", "api", "assets", "health", "healthz", "metrics"].includes(
          b.id,
        )
      )
        throw new Error(
          "这个地址名称被系统保留，请换一个，例如 friends-survival。",
        );
      await api(
        existing ? "servers/" + enc(existing.id) : "servers",
        existing ? "PATCH" : "POST",
        b,
      );
      m.close();
      go(
        "/admin/servers/" + enc(existing?.id || b.id),
        existing
          ? "服务器信息已保存。"
          : "服务器已创建。下一步：上传玩家需要的 MOD 文件。",
      );
    },
  );
  if (!existing) {
    const preview = U.info("玩家将使用这个地址", serverURL(f.refs.id.value));
    f.refs.id.addEventListener(
      "input",
      () =>
        (preview.lastChild.textContent = serverURL(
          f.refs.id.value || "my-server",
        )),
    );
    f.refs.id.closest(".field").after(preview);
  }
  f.actions.prepend(U.button("取消", m.close, "btn-outline-secondary"));
}
async function serverList() {
  heading(
    "我的服务器",
    "每个服务器拥有独立的 MOD 清单、环境版本和玩家授权。",
    U.button("＋ 创建服务器", () => serverDialog()),
  );
  if (!servers.length) {
    content.append(
      U.box(
        "card",
        U.empty(
          "还没有服务器",
          "创建一个配置，再上传 MOD、搭配环境版本，最后把地址分享给玩家。",
          U.button("创建第一个服务器", () => serverDialog()),
        ),
      ),
    );
    return;
  }
  const [channels, releases] = await Promise.all([
    Promise.all(servers.map((s) => api(`servers/${enc(s.id)}/channels`))),
    api("releases"),
  ]);
  serverCards(servers, channels, releases);
}
async function serverDetail(id) {
  const s = servers.find((s) => s.id === id);
  if (!s) throw new Error("这个服务器不存在，请从“我的服务器”重新选择。");
  const [releases, channels, artifacts] = await Promise.all([
    api(`servers/${enc(id)}/releases`),
    api(`servers/${enc(id)}/channels`),
    api("artifacts"),
  ]);
  const stable = channels.find((c) => c.name === "stable"),
    active = releases.find((r) => r.id === stable?.active_release_id);
  heading(
    s.display_name,
    "准备这台服务器的玩家环境，并把可用的地址交给玩家。",
    U.button("编辑信息", () => serverDialog(s), "btn-outline-secondary"),
    U.button("＋ 配置新一版", () => createRelease(id)),
  );
  workflow(
    content,
    [true, artifacts.length > 0, releases.some(published), !!active],
    [
      "/admin/servers",
      `/admin/artifacts?server=${enc(id)}`,
      `/admin/releases?server=${enc(id)}`,
      "#share",
    ],
  );
  const split = U.box("split");
  const main = U.box(""),
    side = U.box("");
  split.append(main, side);
  content.append(split);
  const current = U.panel(
    "玩家当前使用的环境",
    "默认分发通道叫 stable，可以理解为“正式版”。",
    main,
  );
  current.body.append(
    active
      ? U.kv([
          ["当前环境版本", active.name],
          ["启用范围", "stable · 正式版"],
          [
            "访问方式",
            s.visibility === "private"
              ? "邀请码授权后使用"
              : "知道地址即可使用",
          ],
        ])
      : U.info(
          "暂时还没有可用环境",
          "先创建草稿并加入 MOD，检查后封存，再把这一版设为玩家当前使用。",
          "warning",
        ),
  );
  current.body.append(
    U.button(active ? "更换版本 / 回到旧版" : "选择已封存版本并启用", () =>
      activateDialog(id, releases, channels),
    ),
  );
  current.body.lastChild.disabled = !releases.some(published);
  if (!releases.some(published))
    current.body.append(
      U.el("p", "还没有可选的已封存版本。请先完成下面的环境配置。", {
        className: "form-text",
      }),
    );
  const share = U.panel(
    "分享给玩家",
    "这是 MOD 同步地址，不是游戏服务器连接地址。",
    main,
  );
  share.card.classList.add("mt-4");
  share.card.id = "share";
  share.body.append(U.copy(serverURL(id), "复制地址"));
  if (!active)
    share.body.append(
      U.info(
        "还需要先启用一版环境",
        "现在访问这个地址还拿不到可用清单。完成上面的启用操作后，再邀请玩家。",
        "warning",
      ),
    );
  share.body.append(
    U.el(
      "p",
      "玩家操作：安装本客户端 MOD 与 Fabric API → 在标题界面打开 Mod List Discovery → 添加这个地址 → 检查 → 确认同步 → 重启游戏。",
      { className: "subtle" },
    ),
  );
  if (s.visibility === "private")
    share.body.append(
      U.link("为玩家生成邀请码 →", `/admin/clients?server=${enc(id)}`),
    );
  help(
    side,
    "为什么要先封存，再启用？",
    "封存会把这一版的文件清单固定下来，便于复现和回滚。启用才会让玩家获取这一版。以后更新 MOD，需要配置新的草稿，而不是修改历史版本。",
  );
  const details = U.panel("服务器资料", "", side);
  details.card.classList.add("mt-4");
  details.body.append(
    U.kv([
      ["地址名称", id],
      ["访问方式", s.visibility === "private" ? "私密" : "公开"],
      ["简介", s.description || "未填写"],
    ]),
  );
  details.body.append(
    U.advanced(
      "删除服务器",
      U.el(
        "p",
        "删除会移除草稿、邀请码及设备记录。已有封存版本的服务器不能删除，以保留发布历史。",
        { className: "subtle" },
      ),
      U.button(
        "删除这个服务器",
        async () => {
          if (
            await U.confirm(
              "删除“" + s.display_name + "”？",
              "草稿、邀请码和已登记设备将一起删除。已有封存版本时，服务器会拒绝此操作。",
              "确认删除",
              true,
            )
          ) {
            await api(`servers/${enc(id)}?confirm=true`, "DELETE");
            go("/admin/servers", "服务器已删除。");
          }
        },
        "btn-sm btn-outline-danger",
      ),
    ),
  );
  section("这个服务器的环境版本", "先编辑草稿，确认后封存");
  releaseTable(content, releases);
}
async function activateDialog(serverId, releases, channels, preferredId) {
  const available = releases.filter(published);
  if (!available.length)
    throw new Error("还没有已封存版本，请先创建并封存一版环境。");
  const m = U.modal(
    "选择玩家使用的环境版本",
    "启用后，玩家下次检查就会看到这份清单；已运行的游戏不会被自动改动。选择较早的一版就是回滚。",
  );
  const f = U.form(
    m.body,
    [
      {
        name: "releaseId",
        label: "玩家应使用哪一版？",
        choices: available.map((r) => [r.id, r.name]),
        value:
          preferredId ||
          channels.find((c) => c.name === "stable")?.active_release_id ||
          available[0].id,
        help: "这里只显示当前服务器的已封存版本，不需要记住版本编号。",
      },
      {
        name: "channel",
        label: "分发通道名称",
        value: "stable",
        pattern: "[a-z0-9][a-z0-9-]{0,63}",
        help: "通常保留 stable（正式版）。其他通道只有把客户端 Profile 配置为同名通道时才生效，不会改变默认分享地址使用的版本。",
      },
    ],
    "启用所选版本",
    async (b) => {
      await api(`servers/${enc(serverId)}/channels/${enc(b.channel)}`, "PUT", {
        releaseId: Number(b.releaseId),
      });
      m.close();
      go(
        "/admin/servers/" + enc(serverId) + "#share",
        "已启用所选版本。可以复制地址发给玩家了。",
      );
    },
  );
  const field = f.refs.channel.closest(".field");
  f.container.append(U.advanced("高级：使用其他分发通道", field));
  f.actions.prepend(U.button("取消", m.close, "btn-outline-secondary"));
}
async function artifactsView() {
  const rows = await api("artifacts");
  heading(
    "MOD 文件库",
    "先把玩家需要的 Fabric JAR 放到这里。上传只保存文件，不会直接改变玩家的环境。",
    U.button("从链接导入", importDialog, "btn-outline-secondary"),
    U.button("＋ 上传 MOD 文件", uploadDialog),
  );
  content.append(
    U.info(
      "接下来要做什么？",
      "上传完成后，到“环境版本”创建一份草稿，从文件库选择 MOD。通常无需填写文件校验码，系统会自动识别名称和版本。",
    ),
  );
  U.searchTable(
    content,
    rows,
    [
      {
        label: "MOD 名称 / 版本",
        render: (r) => U.textPair(r.mod_name || r.mod_id, r.mod_version),
      },
      { label: "文件大小", render: (r) => U.bytes(r.size) },
      {
        label: "适用位置",
        render: (r) =>
          U.badge(
            r.mod_environment === "server"
              ? "仅游戏服务端"
              : r.mod_environment === "client"
                ? "玩家客户端"
                : "客户端与服务端",
            r.mod_environment === "server" ? "warning" : "success",
          ),
      },
      { label: "上传时间", render: (r) => U.date(r.created_at) },
      {
        label: "",
        render: (r) =>
          U.box(
            "row-actions",
            U.button(
              "查看详情",
              () => artifactDetails(r.sha256),
              "btn-sm btn-outline-secondary",
            ),
          ),
      },
    ],
    (r) => [r.mod_id, r.mod_name, r.mod_version, r.filename].join(" "),
    "先上传第一个 MOD",
    "请选择适用于 Minecraft 26.2 的 Fabric JAR。这里只管理 MOD，不支持资源包、光影包或配置文件。",
    U.button("选择 JAR 文件", uploadDialog),
  );
  const next = U.box(
    "mt-4 d-flex justify-content-between align-items-center",
    U.el("span", "文件准备好了？把它们组合成玩家要使用的一版。", {
      className: "subtle",
    }),
    U.link(
      "去搭配环境版本 →",
      "/admin/releases" +
        (query.get("server") ? "?server=" + enc(query.get("server")) : ""),
    ),
  );
  content.append(next);
}
function uploadDialog() {
  const m = U.modal(
    "上传 MOD 文件",
    "选择 Fabric MOD 的 .jar 文件。系统会自动识别名称、版本和适用环境；相同内容只保存一份。",
  );
  U.form(
    m.body,
    [
      {
        name: "file",
        label: "Fabric MOD 文件",
        type: "file",
        accept: ".jar,application/java-archive",
        help: "一次上传一个 JAR。默认上限 512 MB，实际限制以服务端配置为准。请确认文件支持 Minecraft 26.2，并且你有权分发。",
      },
    ],
    "上传并识别 MOD",
    async (b) => {
      const body = new FormData();
      body.append("file", b.file);
      await api("artifacts/upload", "POST", body);
      m.close();
      await refresh("上传成功。下一步：把这个 MOD 加入环境版本。");
    },
  );
}
function importDialog() {
  const m = U.modal(
    "从下载链接导入 MOD",
    "服务端会下载并保存文件，之后玩家从你自己的文件库获取它。",
  );
  U.form(
    m.body,
    [
      {
        name: "url",
        label: "JAR 文件的直接下载链接",
        type: "url",
        placeholder: "https://example.com/downloads/example.jar",
        help: "请填可以直接下载 JAR 的 HTTPS 链接，不是 MOD 介绍网页。默认禁止内网地址，不能在链接中包含账号密码。",
      },
    ],
    "下载并导入文件",
    async (b) => {
      await api("artifacts/import", "POST", b);
      m.close();
      await refresh("文件已导入。现在可以把它加入环境版本。");
    },
  );
}
async function artifactDetails(hash) {
  const { artifact: a, releases } = await api("artifacts/" + enc(hash));
  const m = U.modal(
    a.mod_name || a.mod_id,
    "这个文件可以被多个环境版本复用。替换 MOD 时请上传新文件，并创建新版本。",
  );
  m.body.append(
    U.kv([
      ["MOD 版本", a.mod_version],
      ["文件名", a.filename],
      ["大小", U.bytes(a.size)],
      [
        "适用位置",
        a.mod_environment === "server"
          ? "仅游戏服务端，不可用于玩家环境"
          : a.mod_environment === "client"
            ? "玩家客户端"
            : "客户端与服务端",
      ],
      ["内部名称（Mod ID）", a.mod_id],
    ]),
  );
  const refs = U.panel(
    "正在使用这个文件的环境版本",
    "被任何草稿或封存版本引用的文件都不能删除。",
    m.body,
  );
  refs.card.classList.add("mt-4");
  if (releases.length)
    releases.forEach((r) =>
      refs.body.append(
        U.box(
          "mb-2 d-flex gap-2",
          U.link(
            `${serverName(r.server_id)} / ${r.name}`,
            "/admin/releases/" + r.id,
          ),
          status(r),
        ),
      ),
    );
  else
    refs.body.append(
      U.el("p", "尚未加入任何环境版本。", { className: "subtle mb-0" }),
    );
  let metadata = {};
  try {
    metadata = JSON.parse(a.fabric_metadata);
  } catch {
    metadata = {};
  }
  const dependencies = U.box("");
  for (const [key, title] of [
    ["depends", "需要的其他 MOD 或运行环境"],
    ["conflicts", "声明的冲突"],
    ["breaks", "不兼容的组合"],
  ]) {
    dependencies.append(U.el("h3", title, { className: "fs-6 mt-3" }));
    const entries = Object.entries(metadata[key] || {});
    dependencies.append(
      entries.length
        ? U.kv(
            entries.map(([id, predicate]) => [
              id,
              Array.isArray(predicate)
                ? predicate.join(" 或 ")
                : String(predicate),
            ]),
          )
        : U.el("p", "文件未声明此项。", { className: "subtle" }),
    );
  }
  m.body.append(
    U.advanced(
      "高级：文件校验码与依赖说明",
      U.el(
        "p",
        "SHA256 是系统计算的文件指纹，用于确认下载内容没有改变。搭配环境时无需手动复制。",
        { className: "subtle" },
      ),
      U.copy(hash, "复制校验码"),
      dependencies,
    ),
  );
  m.body.append(
    U.box(
      "form-actions",
      U.button(
        "删除未使用的文件",
        async () => {
          await m.close();
          if (
            await U.confirm(
              "删除这个 MOD 文件？",
              `${a.mod_name || a.mod_id} ${a.mod_version} 将从文件库删除。已有引用时删除会被拒绝。`,
              "确认删除",
              true,
            )
          ) {
            await api("artifacts/" + enc(hash), "DELETE");
            await refresh("未使用的文件已删除。");
          }
        },
        "btn-outline-danger",
      ),
      U.link("去搭配环境版本", "/admin/releases", "btn btn-primary"),
    ),
  );
}
function releaseTable(parent, rows) {
  U.searchTable(
    parent,
    rows,
    [
      {
        label: "环境版本",
        render: (r) => U.link(r.name, "/admin/releases/" + r.id),
      },
      { label: "所属服务器", render: (r) => serverName(r.server_id) },
      { label: "状态", render: status },
      {
        label: "下一步",
        render: (r) =>
          U.link(
            published(r) ? "查看并启用 →" : "继续配置 →",
            "/admin/releases/" + r.id,
          ),
      },
    ],
    (r) => `${r.name} ${serverName(r.server_id)}`,
    "还没有环境版本",
    "环境版本是你为玩家搭配的一组 MOD。先创建草稿，再选择文件。",
    U.button("创建环境版本", () => createRelease(query.get("server"))),
  );
}
async function releasesView() {
  heading(
    "环境版本",
    "把 MOD 搭配成一版可分发的环境。草稿可以修改，封存后可启用给玩家。",
    U.button("＋ 创建环境版本", () => createRelease(query.get("server"))),
  );
  content.append(
    U.info(
      "保存、封存、启用有什么区别？",
      "保存草稿只保留你的配置；封存会锁定这一版；启用到正式版后，玩家下次检查才会收到更新。",
    ),
  );
  const rows = await api("releases");
  const filter = U.field({
    name: "server",
    label: "查看哪个服务器？",
    choices: [["", "全部服务器"], ...serverChoices()],
    value: query.get("server") || "",
    required: false,
  });
  content.append(filter.wrapper);
  const list = U.box("");
  content.append(list);
  const draw = () => {
    list.replaceChildren();
    releaseTable(
      list,
      rows.filter(
        (r) => !filter.input.value || r.server_id === filter.input.value,
      ),
    );
  };
  filter.input.addEventListener("change", draw);
  draw();
}
const runtimeFields = (r) => [
  {
    name: "minecraft",
    label: "Minecraft 版本",
    value: r?.minecraft_version || "26.2",
    help: "玩家需要使用这个游戏版本。目前配套客户端面向 Minecraft 26.2。",
  },
  {
    name: "fabricLoader",
    label: "Fabric Loader 版本范围",
    value: r?.loader_constraint || ">=0.19.0",
    help: "加载 MOD 的运行程序。>= 表示不低于这个版本，通常保留默认值。",
  },
  {
    name: "java",
    label: "Java 版本范围",
    value: r?.java_constraint || ">=25",
    help: "启动游戏所需的 Java。通常由启动器安装，Minecraft 26.2 使用 Java 25。",
  },
  {
    name: "minDiscoveryVersion",
    label: "同步客户端最低版本",
    value: r?.min_client_version || "0.1.0",
    help: "指 Mod List Discovery 的版本，不是 Minecraft 或其他 MOD 的版本。",
  },
];
function createRelease(serverId) {
  if (!servers.length) {
    go("/admin/servers", "请先创建服务器，再搭配环境版本。");
    return;
  }
  const m = U.modal(
    "搭配新一版环境",
    "创建后先保存为草稿，你可以反复调整，不会影响玩家。",
  );
  const f = U.form(
    m.body,
    [
      {
        name: "serverId",
        label: "用于哪个服务器？",
        choices: serverChoices(),
        value: serverId || servers[0].id,
      },
      {
        name: "name",
        label: "这一版的名称",
        placeholder: "例如：生存服 · 首次开放",
        help: "起一个容易区分的名字，例如“国庆更新”或“修复地图冲突”。",
      },
      ...runtimeFields(),
    ],
    "创建草稿，开始选择 MOD",
    async (b) => {
      const r = await api("servers/" + enc(b.serverId) + "/releases", "POST", {
        name: b.name,
        minecraft: b.minecraft,
        fabricLoader: b.fabricLoader,
        java: b.java,
        minDiscoveryVersion: b.minDiscoveryVersion,
      });
      m.close();
      go("/admin/releases/" + r.id, "草稿已创建。现在选择玩家需要的 MOD。");
    },
  );
  const advanced = U.advanced("高级：游戏与运行环境（通常无需修改）");
  for (const key of [
    "minecraft",
    "fabricLoader",
    "java",
    "minDiscoveryVersion",
  ])
    advanced.lastChild.append(f.refs[key].closest(".field"));
  f.container.append(advanced);
}
async function releaseDetail(id) {
  const [{ release: r, mods }, artifacts] = await Promise.all([
    api("releases/" + id),
    api("artifacts"),
  ]);
  const channels = await api("servers/" + enc(r.server_id) + "/channels");
  const live = channels.some(
    (c) => c.name === "stable" && c.active_release_id === r.id,
  );
  heading(
    r.name,
    `${serverName(r.server_id)} · ${published(r) ? "这一版已锁定，可以安全复用。" : "修改草稿不会影响玩家当前的环境。"}`,
    U.link(
      "返回版本列表",
      "/admin/releases?server=" + enc(r.server_id),
      "btn btn-outline-secondary",
    ),
  );
  content.append(
    U.box(
      "d-flex gap-2 mb-3",
      status(r),
      live
        ? U.badge("玩家正在使用 · 正式版", "success")
        : U.badge("尚未启用到正式版", "secondary"),
    ),
  );
  content.append(
    U.info(
      published(r)
        ? live
          ? "玩家已能获取这一版"
          : "最后一步：让玩家使用这一版"
        : "配置草稿",
      published(r)
        ? live
          ? "玩家下次检查更新时会获取当前正式版；已经运行的游戏不会被强制改变。"
          : "封存不会自动分发。点击“启用给玩家”，再把服务器地址发给玩家。"
        : "先添加 MOD，再检查依赖并封存。必装、自选和禁止规则都保存在这一版中。",
      live ? "success" : "",
    ),
  );
  const toolbar = U.box("section-toolbar");
  if (!published(r)) {
    toolbar.append(
      U.button("＋ 添加 MOD 或规则", () => modDialog(r, artifacts)),
      U.button("检查依赖", () => checkRelease(r), "btn-outline-secondary"),
      U.button(
        "完成配置并封存",
        async () => {
          if (
            !(await U.confirm(
              "封存这一版环境？",
              "封存后不能再修改 MOD 和运行环境；后续调整需要创建新版本。封存本身不会影响玩家，之后还需要启用到正式版。",
              "确认封存",
            ))
          )
            return;
          await api("releases/" + r.id + "/publish", "POST", {});
          await refresh("已封存。下一步：启用给玩家。");
        },
        "btn-outline-primary",
      ),
    );
  } else {
    toolbar.append(
      U.button(live ? "切换玩家使用的版本" : "启用给玩家", async () =>
        activateDialog(
          r.server_id,
          await api("servers/" + enc(r.server_id) + "/releases"),
          channels,
          r.id,
        ),
      ),
    );
    if (live)
      toolbar.append(
        U.link(
          "复制玩家地址 →",
          "/admin/servers/" + enc(r.server_id) + "#share",
          "btn btn-outline-secondary",
        ),
      );
    toolbar.append(
      U.button(
        "创建下一版",
        () => createRelease(r.server_id),
        "btn-outline-secondary",
      ),
    );
  }
  content.append(toolbar);
  const p = U.panel(
    "这一版的 MOD",
    "必须安装：自动同步；玩家自选：由玩家决定；禁止安装：检测到时提示处理。",
    content,
  );
  U.gridTable(
    p.body,
    mods,
    [
      {
        label: "MOD",
        render: (m) => U.textPair(m.display_name || m.mod_id, m.mod_id),
      },
      {
        label: "规则",
        render: (m) =>
          U.badge(
            requirement[m.requirement],
            m.requirement === "forbidden" ? "danger" : "primary",
          ),
      },
      {
        label: "文件版本",
        render: (m) =>
          m.requirement === "forbidden"
            ? m.version_constraint || "所有版本"
            : artifacts.find((a) => a.sha256 === m.artifact_sha256)
                ?.mod_version || "文件不可用",
      },
      {
        label: "默认选择",
        render: (m) =>
          m.requirement === "optional"
            ? m.default_selected
              ? "默认勾选"
              : "默认不选"
            : "—",
      },
      {
        label: "操作",
        render: (m) =>
          published(r)
            ? "已锁定"
            : U.button(
                "移除",
                async () => {
                  if (
                    await U.confirm(
                      "移除这条 MOD 规则？",
                      "只从当前草稿移除，不删除文件库中的文件。",
                      "确认移除",
                    )
                  ) {
                    await api(
                      `releases/${r.id}/mods/${enc(m.mod_id)}`,
                      "DELETE",
                    );
                    await refresh("已从草稿移除。");
                  }
                },
                "btn-sm btn-outline-danger",
              ),
      },
    ],
    "还没有添加 MOD，点击上方“添加 MOD 或规则”开始",
  );
  const runtime = U.advanced(
    "游戏与运行环境",
    U.kv([
      ["Minecraft", r.minecraft_version],
      ["Fabric Loader", r.loader_constraint],
      ["Java", r.java_constraint],
      ["同步客户端最低版本", r.min_client_version],
    ]),
  );
  if (!published(r))
    runtime.lastChild.append(
      U.button(
        "修改名称与运行环境",
        () => {
          const m = U.modal(
            "修改草稿信息",
            "这些要求会由玩家客户端检查，请与实际使用的游戏版本保持一致。",
          );
          U.form(
            m.body,
            [
              { name: "name", label: "环境版本名称", value: r.name },
              ...runtimeFields(r),
            ],
            "保存草稿信息",
            async (b) => {
              await api("releases/" + r.id, "PATCH", b);
              m.close();
              await refresh("草稿信息已保存。");
            },
          );
        },
        "btn-outline-secondary",
      ),
    );
  content.append(runtime);
}
function modDialog(r, artifacts) {
  const usable = artifacts.filter(
    (a) => a.mod_environment !== "server" && a.mod_id !== "mod-list-discovery",
  );
  const m = U.modal(
    "添加 MOD 或禁止规则",
    "从文件库按名称选择，无需填写文件校验码。同一个 MOD 再次添加会替换它在本草稿中的规则。",
  );
  const f = U.form(
    m.body,
    [
      {
        name: "requirement",
        label: "希望玩家如何处理？",
        choices: [
          ["required", "必须安装：玩家同步时自动安装"],
          ["optional", "玩家自选：玩家可以勾选是否安装"],
          ["forbidden", "禁止安装：发现这个 MOD 时提示移除"],
        ],
        value: "required",
      },
      {
        name: "artifact",
        label: "选择文件库中的 MOD",
        choices: [
          ["", "请选择一个 MOD 文件"],
          ...usable.map((a) => [
            a.sha256,
            `${a.mod_name || a.mod_id} · ${a.mod_version} · ${a.filename}`,
          ]),
        ],
        help: "只显示可用于客户端的文件。找不到文件时，请先在 MOD 文件库上传。",
      },
      {
        name: "modId",
        label: "禁止的 MOD 内部名称",
        placeholder: "例如：example_mod",
        help: "这里要填 Mod ID，不是文件名或显示名称。可在文件详情中查看。",
      },
      {
        name: "name",
        label: "给玩家看的名称",
        required: false,
        help: "选填，例如“与服务器冲突的地图 MOD”。",
      },
      {
        name: "versionConstraint",
        label: "禁止哪些版本？",
        value: "*",
        help: "默认 * 表示所有版本。熟悉版本表达式时也可填 <2.0.0。",
      },
      {
        name: "defaultSelected",
        label: "玩家首次看到时是否默认勾选？",
        choices: [
          ["true", "默认勾选"],
          ["false", "默认不选"],
        ],
        value: "true",
        help: "仅影响首次选择，玩家仍可自行修改。",
      },
    ],
    "保存到这一版",
    async (b) => {
      const a = usable.find((a) => a.sha256 === b.artifact);
      await api("releases/" + r.id + "/mods", "POST", {
        modId: b.requirement === "forbidden" ? b.modId : a.mod_id,
        name:
          b.requirement === "forbidden"
            ? b.name || b.modId
            : a.mod_name || a.mod_id,
        requirement: b.requirement,
        artifact: b.requirement === "forbidden" ? null : b.artifact,
        versionConstraint:
          b.requirement === "forbidden" ? b.versionConstraint : "*",
        defaultSelected:
          b.requirement === "optional" && b.defaultSelected === "true",
      });
      m.close();
      await refresh("规则已保存到草稿。完成后可以检查依赖并封存。");
    },
  );
  const sync = () => {
    for (const key of [
      "artifact",
      "modId",
      "name",
      "versionConstraint",
      "defaultSelected",
    ]) {
      const shown =
        key === "artifact"
          ? f.refs.requirement.value !== "forbidden"
          : key === "defaultSelected"
            ? f.refs.requirement.value === "optional"
            : f.refs.requirement.value === "forbidden";
      f.refs[key].disabled = !shown;
      f.refs[key].closest(".field").hidden = !shown;
    }
  };
  f.refs.requirement.addEventListener("change", sync);
  sync();
  if (!usable.length)
    m.body.prepend(
      U.alert(
        "文件库中还没有可用的客户端 MOD。你仍可添加禁止规则，或先关闭此窗口去上传文件。",
        "warning",
      ),
    );
  m.body.append(
    U.link("打开 MOD 文件库", "/admin/artifacts?server=" + enc(r.server_id)),
  );
}
function explainDependency(text) {
  return text
    .replace(
      /(.+) depends on (.+); verify bundled or user-installed dependencies/,
      "$1 需要 $2。请确认它已包含在 MOD 内，或由玩家另外安装。",
    )
    .replace(
      /(.+) dependency (.+) version mismatch/,
      "$1 需要的 $2 版本与当前选择不一致，请更换合适的文件。",
    )
    .replace(/(.+) conflicts with (.+)/, "$1 与 $2 声明存在冲突，请调整搭配。");
}
async function checkRelease(r) {
  const result = await api("releases/" + r.id + "/validate");
  const m = U.modal(
    "依赖检查结果",
    "检查 MOD 声明的依赖与冲突。文件存储、游戏版本等其他要求仍会在封存时检查；建议上线前实际启动游戏测试。",
  );
  m.body.append(
    U.alert(
      result.valid ? "基础依赖检查通过。" : "发现需要处理的问题。",
      result.valid ? "success" : "danger",
    ),
  );
  if (result.error) m.body.append(U.el("p", explainDependency(result.error)));
  for (const warning of result.warnings || [])
    m.body.append(U.alert(explainDependency(warning), "warning"));
  m.body.append(U.button("返回配置", m.close, "btn-outline-secondary"));
}
async function clientsView() {
  heading(
    "玩家设备与邀请",
    "私密服务器通过邀请码登记玩家设备；公开服务器通常无需邀请码。",
    U.button("＋ 创建邀请码", () => inviteDialog(query.get("server"))),
  );
  const [clients, codes] = await Promise.all([
    api("clients"),
    api("enrollment-codes"),
  ]);
  const filter = U.field({
    name: "server",
    label: "查看哪个服务器？",
    choices: [["", "全部服务器"], ...serverChoices()],
    value: query.get("server") || "",
    required: false,
  });
  content.append(
    filter.wrapper,
    U.info(
      "邀请码和设备授权的区别",
      "邀请码用于首次登记，可以限制使用次数和有效期。设备登记成功后使用自己的凭证；撤销邀请码不会踢掉已经登记的设备。",
    ),
  );
  const result = U.box("");
  content.append(result);
  const draw = () => {
    result.replaceChildren();
    const selected = filter.input.value;
    const devicePanel = U.panel(
      "已登记的玩家设备",
      "“最近联系”表示上次访问服务的时间，不代表玩家正在游戏中。",
      result,
    );
    U.gridTable(
      devicePanel.body,
      clients.filter((c) => !selected || c.server_id === selected),
      [
        {
          label: "设备",
          render: (c) =>
            U.textPair(c.name || "未命名设备", serverName(c.server_id)),
        },
        { label: "最近联系", render: (c) => U.date(c.last_seen_at) },
        {
          label: "授权状态",
          render: (c) =>
            U.badge(
              c.revoked_at ? "已撤销" : "已授权",
              c.revoked_at ? "secondary" : "success",
            ),
        },
        {
          label: "操作",
          render: (c) =>
            c.revoked_at
              ? "—"
              : U.button(
                  "撤销授权",
                  async () => {
                    if (
                      await U.confirm(
                        "撤销这个设备的授权？",
                        "该设备将无法继续获取私密清单。玩家需要新的邀请码重新登记；已经下载的文件不会被远程删除。",
                        "撤销设备授权",
                        true,
                      )
                    ) {
                      await api("clients/" + c.id + "/revoke", "POST", {});
                      await refresh("设备授权已撤销。");
                    }
                  },
                  "btn-sm btn-outline-danger",
                ),
        },
      ],
      "还没有玩家登记设备",
    );
    const invitations = U.panel(
      "邀请码记录",
      "出于安全考虑，这里只显示使用情况，完整邀请码仅在创建成功时显示。",
      result,
    );
    invitations.card.classList.add("mt-4");
    U.gridTable(
      invitations.body,
      codes.filter((c) => !selected || c.server_id === selected),
      [
        { label: "服务器", render: (c) => serverName(c.server_id) },
        {
          label: "使用情况",
          render: (c) => `${c.use_count} / ${c.max_uses} 次`,
        },
        { label: "到期时间", render: (c) => U.date(c.expires_at) },
        {
          label: "状态",
          render: (c) =>
            U.badge(
              c.revoked_at
                ? "已撤销"
                : Date.parse(c.expires_at) <= Date.now()
                  ? "已过期"
                  : c.use_count >= c.max_uses
                    ? "已用完"
                    : "可使用",
              !c.revoked_at &&
                Date.parse(c.expires_at) > Date.now() &&
                c.use_count < c.max_uses
                ? "success"
                : "secondary",
            ),
        },
        {
          label: "操作",
          render: (c) =>
            c.revoked_at
              ? "—"
              : U.button(
                  "撤销邀请码",
                  async () => {
                    if (
                      await U.confirm(
                        "撤销这个邀请码？",
                        "之后不能再用它登记新设备，已经登记的设备不受影响。",
                        "确认撤销",
                        true,
                      )
                    ) {
                      await api(
                        "enrollment-codes/" + c.id + "/revoke",
                        "POST",
                        {},
                      );
                      await refresh("邀请码已撤销。");
                    }
                  },
                  "btn-sm btn-outline-danger",
                ),
        },
      ],
      "还没有创建邀请码",
    );
  };
  filter.input.addEventListener("change", draw);
  draw();
}
function inviteDialog(serverId) {
  const privateServers = servers.filter((s) => s.visibility === "private");
  if (!privateServers.length) {
    const m = U.modal(
      "目前不需要邀请码",
      "公开服务器的玩家可以直接使用服务器地址。需要控制访问时，请在服务器信息中把访问方式改为“私密”。",
    );
    m.body.append(U.link("管理服务器", "/admin/servers", "btn btn-primary"));
    return;
  }
  const m = U.modal(
    "邀请玩家使用私密服务器",
    "邀请码用于 MOD 同步授权，不替代游戏服务器自身的白名单。",
  );
  const f = U.form(
    m.body,
    [
      {
        name: "serverId",
        label: "邀请到哪个服务器？",
        choices: serverChoices(true),
        value: privateServers.some((s) => s.id === serverId)
          ? serverId
          : privateServers[0].id,
      },
      {
        name: "maxUses",
        label: "最多允许登记几个设备？",
        type: "number",
        min: 1,
        max: 100000,
        value: 1,
        help: "通常一个玩家的一台电脑需要一次。多人共用时，请填写足够的次数。",
      },
      {
        name: "ttl",
        label: "多久内可以使用？",
        choices: [
          ["1h", "1 小时"],
          ["24h", "24 小时"],
          ["168h", "7 天"],
        ],
        value: "24h",
        help: "过期后不能再登记新设备，已经登记的设备不受影响。",
      },
    ],
    "创建邀请码",
    async (b) => {
      const code = await api(
        "servers/" + enc(b.serverId) + "/enrollment-codes",
        "POST",
        { maxUses: Number(b.maxUses), ttl: b.ttl },
      );
      f.node.hidden = true;
      m.body.append(
        U.alert(
          "请现在复制并妥善发送。关闭窗口后无法再次查看完整邀请码。",
          "warning",
        ),
        U.el("h3", "邀请码", { className: "fs-6" }),
        U.copy(code.code, "复制邀请码"),
        U.el("p", "有效期至 " + U.date(code.expiresAt), {
          className: "subtle mt-3",
        }),
        U.el("h3", "可以发给玩家的说明", { className: "fs-6" }),
        U.copy(
          `加入 ${serverName(b.serverId)} 的 MOD 环境：\n服务器地址：${serverURL(b.serverId)}\n邀请码：${code.code}\n在 Minecraft 标题界面打开 Mod List Discovery，添加这个地址并按提示输入邀请码，检查并同步 MOD 后重启游戏。`,
          "复制完整说明",
        ),
        U.button(
          "我已保存，关闭",
          async () => {
            await m.close();
            await refresh("邀请码已创建。可在列表查看使用情况。");
          },
          "btn-primary mt-3",
        ),
      );
    },
  );
}
async function settingsView() {
  heading(
    "系统设置",
    "了解服务地址、文件存储和最近操作。部署配置由服务端配置文件管理。",
  );
  const p = U.panel("服务信息", "修改部署配置后需要重启服务。", content);
  p.body.append(
    U.kv([
      ["当前程序版本", settings.version],
      ["玩家访问的基础地址", settings.publicBaseUrl],
      [
        "文件存储方式",
        settings.storage === "local" ? "本机磁盘" : settings.storage,
      ],
    ]),
    U.info(
      "玩家无法访问地址？",
      "检查配置文件中的 publicBaseUrl：它应当是玩家能访问的 HTTPS 地址，不能使用仅管理员电脑可访问的 localhost。",
    ),
  );
  content.append(
    U.advanced(
      "高级：清单签名公钥",
      U.el(
        "p",
        "客户端使用公钥核验环境清单，防止内容被篡改。这不是管理员密码或邀请码，无需发给普通玩家。",
        { className: "subtle" },
      ),
      U.kv([
        ["算法", settings.signingKey?.algorithm],
        ["密钥标识", settings.signingKey?.keyId],
      ]),
      U.copy(settings.signingKey?.publicKey || "", "复制公钥"),
    ),
  );
  const audit = await api("audit");
  section("最近操作记录", "用于核对更改，不代表玩家在线状态");
  const actions = {
    "server.create": "创建服务器",
    "server.update": "更新服务器",
    "server.delete": "删除服务器",
    "artifact.upload": "上传 MOD",
    "artifact.import": "导入 MOD",
    "artifact.delete": "删除文件",
    "release.create": "创建草稿",
    "release.update": "修改草稿",
    "release.publish": "封存环境版本",
    "release.mod.update": "保存 MOD 规则",
    "release.mod.delete": "移除 MOD 规则",
    "channel.change": "启用环境版本",
    "enrollment.create": "创建邀请码",
    "enrollment-codes.revoke": "撤销邀请码",
    "clients.revoke": "撤销设备授权",
    "admin.login.success": "管理员登录",
    "admin.login.failure": "登录失败",
    "admin.logout": "管理员退出",
  };
  const panel = U.box("card");
  content.append(panel);
  U.gridTable(
    panel,
    audit,
    [
      { label: "时间", render: (a) => U.date(a.created_at) },
      { label: "操作", render: (a) => actions[a.action] || a.action },
      {
        label: "对象",
        render: (a) => `${a.object_type || ""} ${a.object_id || ""}`,
      },
      {
        label: "操作者",
        render: (a) =>
          a.actor_type === "admin" ? "管理员" : a.actor_type || "系统",
      },
    ],
    "还没有操作记录",
  );
}
function guideView() {
  heading(
    "第一次使用？从这里开始",
    "这套工具帮你把服务器需要的 MOD 发给玩家，并检查他们的游戏环境是否一致。",
  );
  content.append(
    U.info(
      "先弄清楚这两个“服务器”",
      "Minecraft 游戏服务器负责联机；这里的管理服务负责分发 MOD 清单和文件。玩家填写的是本服务生成的分享地址，不是游戏服务器的 IP。",
    ),
  );
  const steps = [
    [
      "1",
      "创建服务器配置",
      "在“我的服务器”点击“创建服务器”。填写玩家能认出的名称和地址名称。朋友服可选私密，开放社区可选公开。创建后还不能立即同步，需要继续准备正式版。",
      "/admin/servers",
      "管理服务器",
    ],
    [
      "2",
      "上传玩家要安装的 MOD",
      "在“MOD 文件库”上传 Fabric JAR，系统自动识别名称和版本。上传的是玩家客户端需要的文件；只在游戏服务端运行的 MOD 不要放进玩家安装清单。每个依赖 MOD 也应按需要上传。",
      "/admin/artifacts",
      "打开文件库",
    ],
    [
      "3",
      "搭配并封存一版环境",
      "创建环境版本草稿，按名称选择 MOD：必装会自动同步，自选由玩家决定，禁止规则用于排除冲突。检查依赖并实际测试游戏。确认后封存；封存后的内容不可修改。",
      "/admin/releases",
      "搭配环境版本",
    ],
    [
      "4",
      "启用正式版并分享地址",
      "打开已封存版本，点击“启用给玩家”，选择正式版 stable。回到服务器详情复制地址。私密服务器还需要创建邀请码。以后更新也要创建草稿、封存、再次启用。",
      "/admin/servers",
      "获取分享地址",
    ],
  ];
  for (const [n, title, text, url, label] of steps) {
    const p = U.panel(title, "", content);
    p.card.classList.add("guide-step");
    p.header.prepend(U.el("span", n, { className: "step-number" }));
    p.body.append(U.el("p", text), U.link(label + " →", url));
  }
  const player = U.panel(
    "发给玩家：如何开始同步",
    "玩家端不需要管理员账号。",
    content,
  );
  const list = U.el("ol");
  for (const t of [
    "安装匹配的 Minecraft、Fabric Loader、Fabric API 和 Mod List Discovery 客户端 MOD。当前配套版本是 Minecraft 26.2、Java 25、Fabric Loader 0.19.0 或以上。",
    "启动游戏，在标题界面打开 Mod List Discovery，添加管理员提供的完整服务器地址。私密服务器按提示输入邀请码。",
    "检查环境，查看必装、自选、禁止及本地额外 MOD；按提示选择并同步。同步前先备份重要的本地自定义 MOD。",
    "按提示重启游戏后再次检查。MOD 不能在运行中的游戏里直接热加载。环境就绪后，再使用游戏服务器地址进行联机。",
  ])
    list.append(U.el("li", t));
  player.body.append(list);
  section("常见词，用普通话解释");
  const terms = [
    [
      "服务器配置",
      "以一个分享地址组织 MOD 环境和玩家授权，不会替你创建 Minecraft 游戏进程。",
    ],
    [
      "MOD 文件库（Artifact）",
      "上传后的 JAR 文件仓库。文件放在这里还不会自动发给玩家。",
    ],
    [
      "环境版本（Release）",
      "某一次搭配好的 MOD 清单，包含文件、安装规则和运行环境要求。",
    ],
    ["草稿（Draft）", "可以修改的准备状态。不会影响玩家正在使用的版本。"],
    [
      "封存（Published）",
      "锁定内容并生成签名，之后不能修改。还需要启用才会成为正式版。",
    ],
    [
      "正式版 / stable 通道",
      "告诉客户端“现在应该使用哪一版”的指向。可以切回之前封存的版本。",
    ],
    [
      "必须安装 / 玩家自选 / 禁止安装",
      "分别表示必装、可自由选择、以及不允许出现的 MOD。",
    ],
    [
      "Mod ID / 内部名称",
      "MOD 作者定义的机器可读名称，例如 fabric-api，可能和文件名、显示名不同。通常由系统读取，只有手写禁止规则时才需要。",
    ],
    [
      "SHA256 / 文件校验码",
      "系统计算的文件指纹，用来验证下载内容。普通配置无需手填或复制。",
    ],
    [
      "邀请码 / 设备凭证",
      "邀请码用于首次登记，设备凭证用于之后访问。撤销邀请码不会撤销已经登记的设备。",
    ],
    [
      "环境配置（Profile）",
      "客户端为一个服务器保存的地址和个人选择，方便在不同服务器之间切换。",
    ],
  ];
  const glossary = U.box("glossary-grid");
  for (const [term, text] of terms) {
    const p = U.panel(term, "", glossary);
    p.body.append(U.el("p", text, { className: "mb-0" }));
  }
  content.append(glossary);
  section("遇到问题时");
  for (const [q, a] of [
    [
      "上传成功，为什么玩家没变化？",
      "上传只进入文件库。还需要把文件加入草稿、封存，再启用到正式版；玩家也需要检查同步并重启。",
    ],
    [
      "封存后发现填错了怎么办？",
      "创建新的环境版本重新搭配并封存，再启用它。也可以把正式版切回之前已经封存的版本；不要直接改数据库。",
    ],
    [
      "依赖检查提示缺少 MOD？",
      "某些 MOD 需要其他 MOD 才能运行。查阅它的说明并上传对应依赖，再加入同一版本；如果依赖已打包在 JAR 内，核实后再继续。检查通过也不能代替实际启动测试。",
    ],
    [
      "私密服务器提示没有权限？",
      "检查邀请码是否过期、用完或撤销，以及设备授权是否已撤销。必要时创建新邀请码，让玩家重新登记。不要让玩家使用管理员密码。",
    ],
    [
      "玩家已经下载的 MOD，撤销授权能收回吗？",
      "不能。撤销只阻止之后访问私密清单和文件，无法远程删除玩家已有文件。",
    ],
    [
      "不知道游戏版本、Java 或签名是什么？",
      "搭配草稿时通常保留默认的高级设置即可。签名和文件校验由系统自动处理，普通玩家无需手动配置。",
    ],
  ])
    content.append(U.advanced(q, U.el("p", a)));
}
function loginView() {
  document.body.classList.add("login-mode");
  content.replaceChildren();
  const intro = U.box(
    "login-intro",
    U.el("span", "MOD LIST DISCOVERY", { className: "eyebrow" }),
    U.el("h1", "把 MOD 配好，\n让朋友轻松加入。"),
    U.el("p", "从文件上传到玩家同步，一个清晰的流程就够了。"),
    U.box(
      "login-steps",
      U.el("span", "01 · 上传 MOD"),
      U.el("span", "02 · 搭配并启用环境"),
      U.el("span", "03 · 分享给玩家"),
    ),
  );
  const card = U.box(
    "login-card",
    U.el("span", "管理员入口", { className: "eyebrow" }),
    U.el("h2", "登录环境管家"),
    U.el("p", "使用初始化服务时创建的管理员账号。", { className: "subtle" }),
  );
  U.form(
    card,
    [
      {
        name: "username",
        label: "管理员用户名",
        value: "admin",
        autocomplete: "username",
        help: "默认用户名为 admin。",
      },
      {
        name: "password",
        label: "管理员密码",
        type: "password",
        autocomplete: "current-password",
        help: "填写服务初始化时设置的密码，不是玩家邀请码。",
      },
    ],
    "登录管理台",
    async (b) => {
      await api("login", "POST", b);
      location.assign("/admin/");
    },
  );
  card.append(
    U.el(
      "p",
      "这是服主的管理入口。玩家请在游戏中的 Mod List Discovery 界面添加服务器地址。",
      { className: "login-note" },
    ),
  );
  content.append(U.box("login-layout", intro, card));
  content.setAttribute("aria-busy", "false");
  document.title = "管理员登录 · MOD 环境管家";
}
async function render() {
  content.setAttribute("aria-busy", "true");
  document.querySelector("#notice").hidden = true;
  content.replaceChildren(
    U.el("p", "正在加载管理数据…", { className: "loading-state" }),
  );
  try {
    const session = await api("session");
    csrf = session.csrfToken;
    [settings, servers] = await Promise.all([api("settings"), api("servers")]);
    const parts = location.pathname.split("/").filter(Boolean);
    const view = parts[1] || "dashboard";
    document.querySelector("#breadcrumb").textContent =
      labels[view] || "管理台";
    document.querySelector("#version").textContent = settings.version || "";
    document.title = (labels[view] || "管理台") + " · MOD 环境管家";
    document.querySelectorAll("[data-view]").forEach((a) => {
      const active = a.dataset.view === view;
      a.classList.toggle("active", active);
      if (active) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    });
    content.replaceChildren();
    if (view === "servers" && parts[2])
      await serverDetail(decodeURIComponent(parts[2]));
    else if (view === "releases" && parts[2])
      await releaseDetail(enc(parts[2]));
    else {
      const views = {
        dashboard,
        servers: serverList,
        artifacts: artifactsView,
        releases: releasesView,
        clients: clientsView,
        settings: settingsView,
        guide: guideView,
      };
      if (!views[view])
        throw new Error("没有找到这个页面，请通过左侧导航选择功能。");
      await views[view]();
    }
    const message = sessionStorage.getItem("mld.notice");
    if (message) {
      sessionStorage.removeItem("mld.notice");
      U.toast(message);
    }
    if (location.hash)
      document.getElementById(location.hash.slice(1))?.scrollIntoView();
  } catch (e) {
    U.report(e);
    content.replaceChildren(
      U.empty(
        "页面暂时无法加载",
        "请检查连接并重试，你已经保存的数据不会因此丢失。",
        U.button("重新加载", render),
      ),
    );
  } finally {
    content.setAttribute("aria-busy", "false");
  }
}
const mobileQuery = matchMedia("(max-width: 800px)");
const sidebar = document.querySelector("#sidebar");
const menuToggle = document.querySelector("#menu-toggle");
function setMenu(open) {
  document.body.classList.toggle("menu-open", open);
  menuToggle.setAttribute("aria-expanded", String(open));
  menuToggle.setAttribute("aria-label", open ? "收起导航" : "展开导航");
  sidebar.inert = mobileQuery.matches && !open;
}
menuToggle.addEventListener("click", () =>
  setMenu(!document.body.classList.contains("menu-open")),
);
mobileQuery.addEventListener("change", () => setMenu(false));
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && document.body.classList.contains("menu-open")) {
    setMenu(false);
    menuToggle.focus();
  }
});
document.addEventListener("click", (e) => {
  if (
    document.body.classList.contains("menu-open") &&
    !sidebar.contains(e.target) &&
    !menuToggle.contains(e.target)
  )
    setMenu(false);
});
setMenu(false);
document.querySelector("#logout").addEventListener("click", async () => {
  try {
    await api("logout", "POST", {});
    location.assign("/login");
  } catch (e) {
    U.report(e);
  }
});
if (location.pathname === "/login") loginView();
else render();
