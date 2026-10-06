"use strict";
// Shared Bootstrap-based components. Text from the API is always inserted as textContent.
window.UI = (() => {
  let serial = 0;
  function el(tag, text, attrs = {}) {
    const node = document.createElement(tag);
    if (text !== undefined && text !== null) node.textContent = text;
    Object.assign(node, attrs);
    return node;
  }
  function box(cls, ...children) {
    const node = el("div", null, { className: cls });
    node.append(...children.filter(Boolean));
    return node;
  }
  function link(text, href, cls = "") {
    return el("a", text, { href, className: cls });
  }
  function errorMessage(e) {
    const messages = {
      invalid_credentials: "用户名或密码不正确，请重新输入。",
      authentication_required: "登录已过期，请重新登录。",
      csrf_failed:
        "页面凭证已过期。请刷新页面后重试，已填写的内容可先复制保存。",
      rate_limited: "操作过于频繁，请等待一分钟再试。",
      server_exists: "这个地址名称已经被使用，请换一个。",
      invalid_server:
        "请检查服务器名称和地址名称。地址名称只能使用小写字母、数字和短横线。",
      invalid_mod:
        "MOD 设置不完整，请选择文件；禁止某个 MOD 时需要填写它的内部名称。",
      artifact_rejected:
        "文件未被接受。请确认它是有效的 Fabric MOD JAR，包含 fabric.mod.json，且没有超过上传限制。",
      artifact_referenced:
        "这个文件还在某个环境版本中使用。请先从草稿移除；已封存版本引用的文件不能删除。",
      cannot_delete_published_releases:
        "这个服务器已有封存版本，为保留历史记录，暂时不能删除。",
      import_failed:
        "导入失败。请确认链接能直接下载 JAR、使用 HTTPS，且目标不是受限制的内网地址。也可以下载到电脑后上传。",
      invalid_release: "请选择同一服务器下已经封存的环境版本。",
      operation_rejected:
        "操作未完成。内容可能已被其他页面修改或封存，请刷新后检查当前状态。",
      publish_rejected:
        "无法封存这一版。请检查 MOD 是否可用于客户端、文件是否仍存在，以及依赖是否冲突。",
      storage_unavailable:
        "文件存储暂时不可用，请检查服务端存储配置或稍后重试。",
      database_error:
        "服务暂时无法保存数据，请稍后重试；若持续发生，请检查服务端日志。",
      not_found: "没有找到这条记录，它可能已被删除。",
      release_not_found: "没有找到环境版本，请返回列表重新选择。",
      invalid_json: "表单内容不符合要求，请检查输入后重试。",
      invalid_max_uses: "邀请码可使用次数必须大于 0。",
    };
    return messages[e.code] || e.message || "操作未完成，请稍后再试。";
  }
  function alert(text, kind = "danger") {
    const node = el("div", text, { className: `alert alert-${kind}` });
    node.setAttribute("role", kind === "danger" ? "alert" : "status");
    return node;
  }
  function report(e, parent = document.querySelector("#notice")) {
    parent.replaceChildren(alert(errorMessage(e)));
    parent.hidden = false;
    if (e.code === "publish_rejected" && e.detail) {
      const d = el("details");
      d.append(
        el("summary", "展开检查详情"),
        el("p", e.detail, { className: "mono mt-2" }),
      );
      parent.firstChild.append(d);
    }
    parent.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }
  function button(text, fn, cls = "btn-primary") {
    const node = el("button", text, {
      type: "button",
      className: `btn ${cls}`,
    });
    node.addEventListener("click", async () => {
      if (node.disabled) return;
      node.disabled = true;
      try {
        await fn(node);
      } catch (e) {
        report(
          e,
          node.closest(".modal")?.querySelector(".modal-notice") || undefined,
        );
      } finally {
        if (node.isConnected) node.disabled = false;
      }
    });
    return node;
  }
  function toast(text) {
    const node = box("toast", box("toast-body", el("span", text)));
    node.setAttribute("role", "status");
    const close = button("×", () => instance.hide(), "btn-sm btn-light");
    close.setAttribute("aria-label", "关闭提示");
    node.firstChild.append(close);
    document.querySelector("#toast-region").append(node);
    const instance = new bootstrap.Toast(node, { delay: 5000 });
    node.addEventListener("hidden.bs.toast", () => {
      instance.dispose();
      node.remove();
    });
    instance.show();
  }
  function modal(title, description = "") {
    const id = `dialog-${++serial}`;
    const root = el("div", null, { className: "modal fade", tabIndex: -1 });
    root.setAttribute("aria-labelledby", id);
    const titleNode = el("h2", title, { className: "modal-title", id });
    const close = el("button", null, {
      className: "btn-close",
      type: "button",
    });
    close.setAttribute("aria-label", "关闭");
    close.dataset.bsDismiss = "modal";
    const notice = box("modal-notice");
    notice.hidden = true;
    const body = box("modal-body", notice);
    if (description)
      body.append(el("p", description, { className: "subtle mb-4" }));
    root.append(
      box(
        "modal-dialog modal-dialog-centered modal-lg",
        box("modal-content", box("modal-header", titleNode, close), body),
      ),
    );
    document.body.append(root);
    const instance = new bootstrap.Modal(root, { backdrop: "static" });
    const opener = document.activeElement;
    let shown = false;
    root.addEventListener("shown.bs.modal", () => {
      shown = true;
    });
    root.addEventListener("hide.bs.modal", (e) => {
      if (root.querySelector('form[aria-busy="true"]')) e.preventDefault();
    });
    root.addEventListener("hidden.bs.modal", () => {
      instance.dispose();
      root.remove();
      if (opener?.isConnected) opener.focus();
    });
    root.addEventListener("shown.bs.modal", () =>
      root
        .querySelector(
          'input:not([type="hidden"]),select,button[type="submit"]',
        )
        ?.focus(),
    );
    instance.show();
    return {
      body,
      root,
      close: () =>
        new Promise((resolve) => {
          root
            .querySelectorAll("form")
            .forEach((f) => f.setAttribute("aria-busy", "false"));
          root.addEventListener("hidden.bs.modal", resolve, { once: true });
          if (shown) instance.hide();
          else
            root.addEventListener("shown.bs.modal", () => instance.hide(), {
              once: true,
            });
        }),
    };
  }
  function confirm(title, text, action = "确认", danger = false) {
    return new Promise((resolve) => {
      const m = modal(title, text);
      let accepted = false;
      m.root.addEventListener("hidden.bs.modal", () => resolve(accepted), {
        once: true,
      });
      m.body.append(
        box(
          "form-actions",
          button("取消", m.close, "btn-outline-secondary"),
          button(
            action,
            () => {
              accepted = true;
              m.close();
            },
            danger ? "btn-danger" : "btn-primary",
          ),
        ),
      );
    });
  }
  function field({
    name,
    label,
    help = "",
    type = "text",
    value = "",
    placeholder = "",
    required = true,
    choices,
    min,
    max,
    pattern,
    accept,
    autocomplete,
  }) {
    const id = `field-${name}-${++serial}`;
    const wrapper = box("field");
    const labelNode = el("label", label, {
      className: "form-label",
      htmlFor: id,
    });
    if (required) {
      const star = el("span", "*", { className: "required-mark" });
      star.setAttribute("aria-hidden", "true");
      labelNode.append(star);
    }
    let input;
    if (choices) {
      input = el("select", null, { className: "form-select" });
      for (const c of choices) {
        const [v, t] = Array.isArray(c) ? c : [c, c];
        input.append(el("option", t, { value: String(v) }));
      }
    } else if (type === "textarea") {
      input = el("textarea", null, { className: "form-control", rows: 3 });
    } else input = el("input", null, { type, className: "form-control" });
    Object.assign(input, { id, name, required, placeholder });
    if (type !== "file") input.value = String(value);
    if (min !== undefined) input.min = String(min);
    if (max !== undefined) input.max = String(max);
    if (pattern) input.pattern = pattern;
    if (accept) input.accept = accept;
    if (autocomplete) input.autocomplete = autocomplete;
    wrapper.append(labelNode, input);
    if (help) {
      const hint = el("div", help, {
        id: id + "-help",
        className: "form-text",
      });
      input.setAttribute("aria-describedby", hint.id);
      wrapper.append(hint);
    }
    return { wrapper, input };
  }
  function form(parent, fields, label, submit) {
    const node = el("form");
    const notice = box("form-notice");
    notice.hidden = true;
    const container = el("fieldset");
    const refs = {};
    for (const def of fields) {
      const fieldNode = field(def);
      container.append(fieldNode.wrapper);
      refs[def.name] = fieldNode.input;
    }
    const submitButton = el("button", label, {
      type: "submit",
      className: "btn btn-primary",
    });
    const actions = box("form-actions", submitButton);
    node.append(notice, container, actions);
    node.addEventListener(
      "invalid",
      (e) => {
        const details = e.target.closest("details");
        if (details) details.open = true;
      },
      true,
    );
    node.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (node.getAttribute("aria-busy") === "true") return;
      const body = Object.fromEntries(new FormData(node));
      notice.hidden = true;
      node.setAttribute("aria-busy", "true");
      container.disabled = true;
      const otherButtons = [...actions.querySelectorAll("button[type=button]")];
      otherButtons.forEach((b) => (b.disabled = true));
      submitButton.disabled = true;
      submitButton.textContent = "正在处理…";
      try {
        await submit(body, node);
      } catch (error) {
        report(error, notice);
      } finally {
        node.setAttribute("aria-busy", "false");
        container.disabled = false;
        otherButtons.forEach((b) => (b.disabled = false));
        submitButton.disabled = false;
        submitButton.textContent = label;
      }
    });
    parent.append(node);
    return { node, refs, container, actions, submitButton };
  }
  function advanced(title, ...nodes) {
    const detail = el("details", null, { className: "advanced" });
    detail.append(el("summary", title), box("", ...nodes));
    return detail;
  }
  function panel(title, description = "", parent) {
    const card = box("card");
    const header = box("card-header", el("h2", title));
    if (description) header.append(el("p", description));
    const body = box("card-body");
    card.append(header, body);
    if (parent) parent.append(card);
    return { card, body, header };
  }
  function info(title, text, kind = "") {
    return box(`info-box ${kind}`, el("strong", title), el("span", text));
  }
  function badge(text, kind = "secondary") {
    return el("span", text, { className: `badge badge-soft-${kind}` });
  }
  function empty(title, text, action) {
    return box(
      "empty-state",
      box("empty-icon", el("span", "▦")),
      el("h3", title),
      el("p", text),
      action,
    );
  }
  function gridTable(parent, rows, columns, emptyMessage = "暂无记录") {
    if (!rows.length) {
      parent.append(
        empty(emptyMessage, "完成前面的步骤后，相关记录会显示在这里。"),
      );
      return;
    }
    const table = el("table", null, { className: "table align-middle" });
    const head = el("thead"),
      tr = el("tr");
    for (const c of columns) {
      const th = el("th", c.label);
      th.scope = "col";
      tr.append(th);
    }
    head.append(tr);
    const body = el("tbody");
    for (const row of rows) {
      const line = el("tr");
      for (const c of columns) {
        const cell = el("td");
        const result = c.render(row);
        cell.append(
          result instanceof Node
            ? result
            : document.createTextNode(String(result ?? "—")),
        );
        line.append(cell);
      }
      body.append(line);
    }
    table.append(head, body);
    parent.append(box("table-responsive", table));
  }
  function searchTable(
    parent,
    rows,
    columns,
    searchText,
    emptyTitle = "暂无记录",
    emptyText = "记录会显示在这里。",
    action,
  ) {
    if (!rows.length) {
      parent.append(empty(emptyTitle, emptyText, action));
      return;
    }
    const input = el("input", null, {
      type: "search",
      className: "form-control search-box",
      placeholder: "搜索名称、版本或标识…",
    });
    input.setAttribute("aria-label", "搜索列表");
    const count = el("span", `${rows.length} 条记录`, { className: "subtle" });
    const results = box("card");
    parent.append(box("section-toolbar", input, count), results);
    const draw = () => {
      const q = input.value.trim().toLowerCase();
      const filtered = rows.filter((r) =>
        searchText(r).toLowerCase().includes(q),
      );
      count.textContent = `${filtered.length} / ${rows.length} 条记录`;
      results.replaceChildren();
      gridTable(results, filtered, columns, "没有找到匹配记录");
    };
    input.addEventListener("input", draw);
    draw();
  }
  function kv(values) {
    const node = el("dl", null, { className: "detail-list" });
    for (const [label, value] of values)
      node.append(el("dt", label), el("dd", value));
    return node;
  }
  function copy(text, label = "复制") {
    const input = el("code", text);
    const node = box(
      "copy-field",
      input,
      button(
        label,
        async () => {
          try {
            await navigator.clipboard.writeText(text);
            toast("已复制，可以粘贴给玩家。");
          } catch {
            const range = document.createRange();
            range.selectNodeContents(input);
            const sel = window.getSelection();
            sel.removeAllRanges();
            sel.addRange(range);
            toast("浏览器未允许自动复制，已选中文字，请手动复制。");
          }
        },
        "btn-sm btn-outline-secondary",
      ),
    );
    return node;
  }
  const date = (s) =>
    s
      ? new Date(s).toLocaleString("zh-CN", {
          year: "numeric",
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
        })
      : "尚无记录";
  const bytes = (n) =>
    n >= 1048576
      ? (n / 1048576).toFixed(1) + " MB"
      : Math.ceil(n / 1024) + " KB";
  const textPair = (title, subtitle) =>
    box("", el("strong", title), el("small", subtitle));
  return {
    el,
    box,
    link,
    button,
    alert,
    report,
    toast,
    modal,
    confirm,
    field,
    form,
    advanced,
    panel,
    info,
    badge,
    empty,
    gridTable,
    searchTable,
    kv,
    copy,
    date,
    bytes,
    textPair,
  };
})();
