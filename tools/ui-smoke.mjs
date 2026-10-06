// Real-browser UI regression: node tools/ui-smoke.mjs
// Requires Node 22+, Python 3, Chrome, and `make server`. Uses isolated temporary data.
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawn, execFileSync } from "node:child_process";
await fs.mkdir('.tools/ui-smoke', {recursive:true});
const temp = await fs.mkdtemp(path.join(os.tmpdir(), "mld-ui-"));
const origin = "http://127.0.0.1:18775";
const config = path.join(temp, "config.yaml");
await fs.writeFile(
  config,
  `server:\n  listen: 127.0.0.1:18775\n  public_base_url: ${origin}\n  data_dir: ${temp}/data\n`,
);
const bin = path.resolve("dist/mod-list-discovery-server");
execFileSync(bin, ["init", "--config", config], {
  env: { ...process.env, MLD_ADMIN_PASSWORD: "ui-test-password-only" },
});
execFileSync("python3", [
  "-c",
  `import zipfile,json,sys
with zipfile.ZipFile(sys.argv[1],'w') as z:
 z.writestr('fabric.mod.json',json.dumps({'schemaVersion':1,'id':'ui_fixture','version':'1.0.0','name':'界面测试 MOD','environment':'*','depends':{'minecraft':'26.2'}}))`,
  path.join(temp, "fixture.jar"),
]);
const server = spawn(bin, ["serve", "--config", config], { stdio: "ignore" });
const chrome = spawn(
  process.env.CHROME || "google-chrome",
  [
    "--headless",
    "--disable-gpu",
    "--no-first-run",
    "--no-default-browser-check",
    "--remote-debugging-port=18776",
    "--user-data-dir=" + temp + "/chrome",
    "about:blank",
  ],
  { stdio: "ignore" },
);
const pause = (ms) => new Promise((r) => setTimeout(r, ms));
let ws;
try {
  let targets;
  for (let i = 0; i < 100; i++) {
    try {
      targets = await (await fetch("http://127.0.0.1:18776/json/list")).json();
      if (targets.length) break;
    } catch {}
    await pause(100);
  }
  ws = new WebSocket(
    targets.find((t) => t.type === "page").webSocketDebuggerUrl,
  );
  await new Promise((r) => (ws.onopen = r));
  let seq = 0;
  const pending = new Map(),
    errors = [],
    external = [];
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data);
    if (m.id) {
      const p = pending.get(m.id);
      pending.delete(m.id);
      m.error ? p.reject(m.error) : p.resolve(m.result);
    } else if (m.method === "Runtime.exceptionThrown")
      errors.push(JSON.stringify(m.params.exceptionDetails));
    else if (
      m.method === "Log.entryAdded" &&
      m.params.entry.text.includes("Content Security Policy")
    )
      errors.push(m.params.entry.text);
    else if (
      m.method === "Network.requestWillBeSent" &&
      !m.params.request.url.startsWith(origin) &&
      !m.params.request.url.startsWith("data:")
    )
      external.push(m.params.request.url);
  };
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++seq;
      pending.set(id, { resolve, reject });
      ws.send(JSON.stringify({ id, method, params }));
    });
  const ev = async (expression) => {
    const r = await call("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true,
    });
    if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails));
    return r.result.value;
  };
  const wait = async (expression) => {
    for (let i = 0; i < 100; i++) {
      if (await ev(expression)) return;
      await pause(100);
    }
    throw Error(
      "UI timeout: " +
        expression +
        "\n" +
        (await ev("document.body.innerText")),
    );
  };
  const click = async (text) => {
    await ev(
      `(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)}&&!b.disabled);if(!b)throw Error('button missing: '+${JSON.stringify(text)});b.click()})()`,
    );
  };
  const fill = async (values) => {
    await ev(
      `(()=>{for(const [k,v]of Object.entries(${JSON.stringify(values)})){const n=document.querySelector('.modal.show [name="'+k+'"]')||document.querySelector('[name="'+k+'"]');n.value=v;n.dispatchEvent(new Event('change',{bubbles:true}));}})()`,
    );
  };
  const submit = () =>
    ev(
      `(document.querySelector('.modal.show form')||document.querySelector('form')).requestSubmit()`,
    );
  const closed = () => wait('!document.querySelector(".modal")');
  const nav = async (route) => {
    await call("Page.navigate", { url: origin + route });
    await wait(
      'document.querySelector("#content")?.getAttribute("aria-busy")==="false"',
    );
    if (await ev('document.body.innerText.includes("页面暂时无法加载")'))
      throw Error(await ev("document.body.innerText"));
  };
  const modal = () =>
    wait(
      '!!document.querySelector(".modal.show") && !document.querySelector(".modal.show")?.classList.contains("hiding")',
    );
  const shot = async (name) => {
    await pause(350);
    const s = await call("Page.captureScreenshot", { format: "png" });
    await fs.writeFile(
      ".tools/ui-smoke/ui-" + name + ".png",
      Buffer.from(s.data, "base64"),
    );
  };
  await call("Runtime.enable");
  await call("Page.enable");
  await call("Log.enable");
  await call("Network.enable");
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 1000,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await nav("/login");
  await shot("login");
  await fill({ password: "wrong-password" });
  await submit();
  await wait('document.body.innerText.includes("用户名或密码不正确")');
  await fill({ password: "ui-test-password-only" });
  await submit();
  await wait(
    'location.pathname==="/admin/" && !!document.querySelector(".stat-card")',
  );
  await shot("dashboard-empty");
  await click("创建第一个服务器");
  await modal();
  await pause(350);
  await fill({
    id: "friends",
    displayName: "朋友们的生存服",
    visibility: "private",
  });
  await submit();
  await wait(
    'location.pathname==="/admin/servers/friends" && document.body.innerText.includes("暂时还没有可用环境")',
  );
  await nav("/admin/artifacts");
  await click("＋ 上传 MOD 文件");
  await modal();
  await pause(350);
  let doc = await call("DOM.getDocument");
  let file = await call("DOM.querySelector", {
    nodeId: doc.root.nodeId,
    selector: "input[type=file]",
  });
  await call("DOM.setFileInputFiles", {
    nodeId: file.nodeId,
    files: [path.join(temp, "fixture.jar")],
  });
  await submit();
  await closed();
  await wait(
    'document.querySelector("table")?.textContent.includes("界面测试 MOD")',
  );
  await nav("/admin/releases?server=friends");
  await click("＋ 创建环境版本");
  await modal();
  await pause(350);
  await fill({ name: "第一次开放" });
  await submit();
  await wait(
    'location.pathname.startsWith("/admin/releases/") && document.body.innerText.includes("添加 MOD 或规则")',
  );
  const releasePath = await ev("location.pathname");
  await click("＋ 添加 MOD 或规则");
  await modal();
  await pause(350);
  await ev(`document.querySelector('[name=artifact]').selectedIndex=1`);
  await submit();
  await closed();
  await wait(
    'document.querySelector("table")?.textContent.includes("界面测试 MOD")',
  );
  // Exercise conditional fields: optional rules and forbidden rules must submit only applicable values.
  await click("＋ 添加 MOD 或规则");
  await modal();
  await pause(350);
  await fill({ requirement: "optional", defaultSelected: "false" });
  await ev("document.querySelector('[name=artifact]').selectedIndex=1");
  await submit();
  await closed();
  await wait(
    'document.querySelector("table")?.textContent.includes("默认不选")',
  );
  await click("＋ 添加 MOD 或规则");
  await modal();
  await pause(350);
  await fill({
    requirement: "forbidden",
    modId: "blocked_mod",
    name: "冲突示例 MOD",
  });
  await submit();
  await closed();
  await wait(
    'document.querySelector("table")?.textContent.includes("禁止安装")',
  );
  await click("检查依赖");
  await modal();
  await wait('document.body.innerText.includes("基础依赖检查通过")');
  await pause(350);
  await click("返回配置");
  await closed();
  await shot("release-draft");
  await click("完成配置并封存");
  await modal();
  await pause(350);
  await click("确认封存");
  await closed();
  await wait('document.body.innerText.includes("最后一步：让玩家使用这一版")');
  await click("启用给玩家");
  await modal();
  await pause(350);
  await submit();
  await wait(
    'location.pathname==="/admin/servers/friends" && document.body.innerText.includes("第一次开放")',
  );
  await shot("server-ready");
  await nav("/admin/clients?server=friends");
  await click("＋ 创建邀请码");
  await modal();
  await pause(350);
  await submit();
  await wait('document.body.innerText.includes("请现在复制")');
  const code = await ev(
    'document.querySelector(".modal .copy-field code").textContent',
  );
  if (
    !code ||
    (await ev(
      'sessionStorage.getItem("mld.notice")?.includes(' +
        JSON.stringify(code) +
        ")",
    ))
  )
    throw Error("Invite display/storage failed");
  await click("我已保存，关闭");
  await closed();
  await wait('document.querySelector("table")?.textContent.includes("0 / 1")');
  // Register a fixture device through the public API, then verify revocation in UI.
  const enrolled = await fetch(origin + "/api/v1/servers/friends/enroll", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code, deviceName: "测试玩家电脑" }),
  });
  if (!enrolled.ok)
    throw Error("Enrollment failed: " + (await enrolled.text()));
  await nav("/admin/clients?server=friends");
  await wait('document.body.innerText.includes("已授权")');
  await click("撤销授权");
  await modal();
  await pause(350);
  await click("撤销设备授权");
  await closed();
  await wait('document.body.innerText.includes("已撤销")');
  await nav("/admin/guide");
  await shot("guide");
  await nav("/admin/settings");
  await wait('document.body.innerText.includes("管理员登录")');
  await nav("/admin/");
  await shot("dashboard");
  await call("Emulation.setDeviceMetricsOverride", {
    width: 390,
    height: 844,
    deviceScaleFactor: 1,
    mobile: true,
  });
  await pause(200);
  await ev('document.querySelector("#menu-toggle").click()');
  await wait(
    'document.querySelector("#sidebar").getBoundingClientRect().left===0',
  );
  await ev(
    'document.dispatchEvent(new KeyboardEvent("keydown",{key:"Escape"}))',
  );
  await shot("mobile");
  if (await ev("document.documentElement.scrollWidth>innerWidth"))
    throw Error("Mobile page overflows horizontally");
  await nav(releasePath);
  if (await ev("document.documentElement.scrollWidth>innerWidth"))
    throw Error("Mobile release overflows");
  await nav("/admin/servers/friends");
  await click("编辑信息");
  await modal();
  await pause(350);
  const hostileName = "<img src=x onerror=alert(1)> 纯文本名称";
  await fill({ displayName: hostileName });
  await call("Network.emulateNetworkConditions", {
    offline: true,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1,
  });
  await submit();
  await wait(
    'document.querySelector(".modal")?.textContent.includes("无法连接管理服务")',
  );
  if (
    (await ev('document.querySelector("[name=displayName]").value')) !==
    hostileName
  )
    throw Error("Failed submission lost form values");
  await call("Network.emulateNetworkConditions", {
    offline: false,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1,
  });
  await submit();
  await wait(
    '!document.querySelector(".modal") && document.querySelector("h1")?.textContent === ' +
      JSON.stringify(hostileName),
  );
  if (await ev('!!document.querySelector("#content img")'))
    throw Error("Server name was rendered as HTML");
  await nav("/admin/artifacts");
  await ev(
    'document.querySelector("input[type=search]").value="不存在的文件"; document.querySelector("input[type=search]").dispatchEvent(new Event("input"))',
  );
  await wait('document.body.innerText.includes("没有找到匹配记录")');
  await nav("/admin/clients");
  await click("撤销邀请码");
  await modal();
  await pause(350);
  await click("确认撤销");
  await closed();
  if (errors.length) throw Error("Browser errors: " + errors.join("\n"));
  if (external.length)
    throw Error("External runtime requests: " + external.join("\n"));
  console.log(
    "UI smoke passed: login errors, empty state, create private server, JAR upload, named MOD selection, optional/forbidden rules, dependency check, publish, activate, invitation, enroll/revoke, guide/settings, desktop/mobile, failed-form recovery, XSS-safe text, list filtering, CSP and no external runtime requests.",
  );
} finally {
  ws?.close();
  server.kill();
  chrome.kill();
  await pause(500);
  await fs.rm(temp, { recursive: true, force: true, maxRetries: 3 });
}
