import { expect, test, type Locator, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function login(page: Page) {
  await loginAs(page, "admin", "secret");
}

async function selectOptionContaining(select: Locator, text: string) {
  const option = select.locator("option").filter({ hasText: text }).first();
  const value = await option.getAttribute("value");
  if (!value) throw new Error(`option containing ${text} was not found`);
  await select.selectOption(value);
}

async function loginAs(page: Page, username: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

async function readVMXML(page: Page, uuid: string, format: "live" | "config" = "live") {
  return page.evaluate(async ({ uuid, format }) => {
    const response = await fetch(`/api/v1/vms/${uuid}/xml?format=${format}`);
    if (!response.ok) throw new Error(`read VM XML failed with ${response.status}`);
    const payload = await response.json() as { xml: string };
    return payload.xml;
  }, { uuid, format });
}

async function readVM(page: Page, uuid: string) {
  return page.evaluate(async (uuid) => {
    const response = await fetch(`/api/v1/vms/${uuid}`);
    if (!response.ok) throw new Error(`read VM failed with ${response.status}`);
    return response.json();
  }, uuid);
}

async function replaceEditorDocument(page: Page, editor: Locator, value: string) {
  await editor.click();
  await page.keyboard.press("Control+A");
  await page.keyboard.insertText(value);
}

type VMTabName = "overview" | "config" | "guest" | "snapshots" | "advanced";

async function openVMTab(page: Page, tab: VMTabName) {
  await page.getByTestId(`vm-tab-${tab}`).click();
  await expect(page.getByTestId(`vm-tab-${tab}`)).toHaveAttribute("aria-selected", "true");
}

test("可写模式完成启动和强制下电确认", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-legacy-01", exact: true }).click();

  const start = page.getByTestId("vm-action-start");
  await expect(start).toBeEnabled();
  await start.click();
  await expect(page.getByText("启动指令已提交，状态已刷新。", { exact: true })).toBeVisible();
  await expect(page.getByText("运行中", { exact: true }).first()).toBeVisible();
  await expect(page.getByTestId("vm-action-shutdown")).toBeVisible();

  await page.getByTestId("vm-action-destroy").click();
  await expect(page.getByRole("alertdialog")).toContainText("确认强制下电？");
  await page.getByRole("button", { name: "确认强制下电", exact: true }).click();
  await expect(page.getByText("强制下电指令已提交，状态已刷新。", { exact: true })).toBeVisible();
  await expect(page.getByText("已关机", { exact: true }).first()).toBeVisible();
  await expect(page.getByTestId("vm-action-start")).toBeEnabled();

  await openVMTab(page, "guest");
  await expect(page.getByTestId("guest-agent-state")).toHaveText("未配置通道");
  const addChannel = page.getByTestId("guest-agent-add-channel");
  await expect(addChannel).toBeEnabled();
  await addChannel.click();
  await expect(page.getByTestId("guest-agent-notice")).toContainText("已补充 QEMU Guest Agent 通道");
  await expect(page.getByTestId("guest-agent-state")).toHaveText("未连接");
  await page.getByTestId("vm-password-open").click();
  await expect(page.getByTestId("vm-password-dialog")).toBeVisible();
  await expect(page.getByTestId("vm-password-state")).toHaveText("Guest Agent 未连接");
  await expect(page.getByTestId("vm-password-submit")).toBeDisabled();
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
});

test("可写模式使用 Guest Agent 文件、密码和进程管理", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();

  await openVMTab(page, "guest");
  await expect(page.getByTestId("guest-agent-state")).toHaveText("已连接");
  await expect(page.getByTestId("guest-agent-controls")).toBeVisible();
  await page.getByLabel("客户机文件路径").fill("/etc/os-release");
  await page.getByTestId("guest-agent-file-read").click();
  await expect(page.getByTestId("guest-agent-file-preview")).toContainText("Ubuntu 24.04.1 LTS");
  await expect(page.getByTestId("guest-agent-notice")).toContainText("文件读取完成");

  await page.getByTestId("vm-password-open").click();
  await expect(page.getByTestId("vm-password-dialog")).toBeVisible();
  await expect(page.getByTestId("vm-password-state")).toHaveText("Guest Agent 已连接");
  await expect(page.getByTestId("vm-password-username")).toBeFocused();
  await page.getByTestId("vm-password-new").fill("temporary-e2e-secret");
  await page.getByTestId("vm-password-confirm").fill("temporary-e2e-secret");
  await page.getByTestId("vm-password-confirm-check").check();
  await page.getByTestId("vm-password-submit").click();
  await expect(page.getByTestId("vm-password-notice")).toContainText("密码修改任务已创建");
  await expect(page.getByTestId("vm-password-new")).toHaveValue("");
  await expect(page.getByTestId("vm-password-confirm")).toHaveValue("");
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();

  await page.getByTestId("guest-agent-process-load").click();
  const processRow = page.getByTestId("guest-agent-process-918");
  await expect(processRow).toContainText("nginx");
  await page.getByTestId("guest-agent-process-signal-918").click();
  await page.getByRole("alertdialog").getByRole("button", { name: "确认执行", exact: true }).click();
  await expect(page.getByTestId("guest-agent-notice")).toContainText("进程信号任务");
  await expect(processRow).toHaveCount(0);
});

test("可写模式 Guest Agent 服务诊断在桌面和移动端可读", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await openVMTab(page, "guest");
  await expect(page.getByTestId("guest-agent-state")).toHaveText("已连接");

  await page.getByTestId("guest-agent-service-load").click();
  await expect(page.getByTestId("guest-agent-notice")).toContainText("已读取 4 个 systemd 服务");
  await expect(page.getByTestId("guest-agent-service-qemu-guest-agent.service")).toContainText("active/running");
  await expect(page.getByTestId("guest-agent-service-debug-shell.service")).toContainText("inactive/dead");
  await expect(page.getByTestId("guest-agent-services")).toContainText("不支持启动、停止、重启或修改服务");

  await page.getByTestId("guest-agent-cloud-init-load").click();
  await expect(page.getByTestId("guest-agent-notice")).toContainText("Cloud-init 状态已读取：done");
  await expect(page.getByTestId("guest-agent-cloud-init-result")).toContainText("DataSourceNoCloud");
  await expect(page.getByTestId("guest-agent-cloud-init-log")).toContainText("Cloud-init v. 24.4");

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px Guest Agent 详情不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px Guest Agent 详情 body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    const service = page.getByTestId("guest-agent-service-qemu-guest-agent.service");
    await expect(service).toBeVisible();
    const box = await service.boundingBox();
    expect(box?.width ?? width + 1, `${width}px 服务行应适应视口`).toBeLessThanOrEqual(width);
    await expect(page.getByTestId("guest-agent-cloud-init")).toBeVisible();
  }
});

test("可写模式通过 Guest Agent 在线扩容文件系统并执行启动诊断", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await openVMTab(page, "guest");
  await expect(page.getByTestId("guest-agent-state")).toHaveText("已连接");

  await page.getByTestId("guest-agent-diagnostics-toggle").click();
  const diagnostics = page.getByTestId("guest-agent-diagnostics");
  await expect(diagnostics).toContainText("1 个服务失败", { timeout: 10_000 });
  await expect(diagnostics).toContainText("0 个 OOM 事件");
  await expect(diagnostics).toContainText("app-worker.service");
  await expect(diagnostics).toContainText("内核警告");
  await expect(diagnostics).toContainText("系统错误");
  await expect(diagnostics).toContainText("检查失败服务的 systemd 日志");

  await page.getByTestId("guest-filesystem-grow-/").click();
  const growModal = page.getByTestId("guest-filesystem-grow-modal");
  await expect(growModal).toBeVisible();
  await expect(growModal).toContainText("80 GiB");
  await page.getByTestId("guest-filesystem-grow-submit").click();
  await expect(page.getByTestId("guest-agent-notice")).toContainText("/ 已扩展至 82 GiB", { timeout: 10_000 });
  await expect(growModal).toBeHidden();

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px Guest Agent 诊断不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px Guest Agent 诊断 body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    const box = await diagnostics.boundingBox();
    expect(box?.width ?? width + 1, `${width}px 诊断区域应适应视口`).toBeLessThanOrEqual(width);
  }

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const action of ["vm.guest_agent.diagnostics", "vm.guest_agent.filesystem.grow"] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText(/共 [1-9]\d* 条审计记录/);
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("成功");
  }
});

test("可写模式批量暂停和恢复虚拟机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByTestId("vm-total-label")).toHaveText("共 4 台虚拟机");

  const webSelect = page.getByTestId("vm-select-11111111-1111-4111-8111-111111111111");
  const dbSelect = page.getByTestId("vm-select-22222222-2222-4222-8222-222222222222");
  const submit = page.getByTestId("vm-batch-submit");
  const action = page.getByTestId("vm-batch-action");
  await expect(submit).toBeDisabled();
  await webSelect.check();
  await dbSelect.check();
  await expect(page.getByTestId("vm-batch-selected")).toContainText("已选择 2 台虚拟机");
  await expect(submit).toBeEnabled();

  await action.selectOption("pause");
  await submit.click();
  await expect(page.getByTestId("vm-batch-result")).toContainText("批量暂停完成：成功 2 台，失败 0 台。");
  await expect(page.getByRole("row", { name: /demo-web-01/ })).toContainText("已暂停");
  await expect(page.getByRole("row", { name: /demo-db-01/ })).toContainText("已暂停");

  await webSelect.check();
  await dbSelect.check();
  await action.selectOption("resume");
  await submit.click();
  await expect(page.getByTestId("vm-batch-result")).toContainText("批量恢复完成：成功 2 台，失败 0 台。");
  await expect(page.getByRole("row", { name: /demo-web-01/ })).toContainText("运行中");
  await expect(page.getByRole("row", { name: /demo-db-01/ })).toContainText("运行中");
});

test("可写模式通过一次性凭证打开 VNC 控制台", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  const openConsole = page.getByTestId("vm-console-open");
  await expect(openConsole).toBeEnabled();
  await openConsole.click();

  await expect(page).toHaveURL(/\/console\/vms\/22222222-2222-4222-8222-222222222222$/);
  await expect(page.getByTestId("vnc-console-title")).toHaveText("demo-db-01");
  await expect(page.getByTestId("vnc-console-status")).toHaveText("已连接", { timeout: 15_000 });
  await expect(page.getByTestId("vnc-console-screen").locator("canvas")).toBeVisible();

  await page.getByRole("link", { name: "返回虚拟机详情" }).first().click();
  await expect(page).toHaveURL(/\/vms\/22222222-2222-4222-8222-222222222222$/);
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();
});

test("移动端 VNC 控制台可连接且头部自适应", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await page.getByTestId("vm-console-open").click();
  await expect(page.getByTestId("vnc-console-title")).toHaveText("demo-db-01");
  await expect(page.getByTestId("vnc-console-status")).toHaveText("已连接", { timeout: 15_000 });
  await expect(page.getByTestId("vnc-console-screen").locator("canvas")).toBeVisible();
  await expect(page.getByTestId("vnc-console-header")).toBeVisible();
  await expect(page.getByRole("button", { name: "重新连接控制台" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
});

test("虚拟机详情内嵌控制台支持发送按键、断开和重连", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  const panel = page.getByTestId("vm-console-panel");
  await expect(panel).toHaveAttribute("data-kind", "vnc");
  await expect(panel).toHaveAttribute("data-state", "connected", { timeout: 15_000 });
  await expect(page.getByTestId("vm-console-vnc-screen").locator("canvas")).toBeVisible();

  await page.getByTestId("vm-console-send-key").selectOption("ctrl-alt-del");
  await expect(page.getByText("已发送 Ctrl + Alt + Del", { exact: true })).toBeVisible();

  await page.getByTestId("vm-console-disconnect").click();
  await expect(panel).toHaveAttribute("data-state", "disconnected");
  await expect(page.getByTestId("vm-console-message")).toContainText("控制台连接已断开");

  await page.getByTestId("vm-console-connect").click();
  await expect(panel).toHaveAttribute("data-state", "connected", { timeout: 15_000 });
});

test("VNC 输入板支持逐字键入和远端剪贴板", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByTestId("vm-console-panel")).toHaveAttribute("data-state", "connected", { timeout: 15_000 });

  const trigger = page.getByTestId("vm-console-input-open");
  await trigger.click();
  const pad = page.getByTestId("vnc-input-pad");
  await expect(pad).toBeVisible();
  const input = page.getByTestId("vnc-input-textarea");
  await expect(input).toBeFocused();

  await input.fill("echo e2e-input");
  await page.getByTestId("vnc-input-type").click();
  await expect(page.getByTestId("vnc-input-notice")).toHaveText("已键入 14 个字符。", { timeout: 5_000 });

  await input.fill("echo 你好");
  await expect(page.getByTestId("vnc-input-unsupported")).toContainText("逐字键入仅支持 ASCII");
  await expect(page.getByTestId("vnc-input-type")).toBeDisabled();
  await page.getByTestId("vnc-input-send-clipboard").click();
  await expect(page.getByTestId("vnc-input-notice")).toContainText("已写入远端剪贴板");

  await page.getByRole("button", { name: "关闭输入板" }).click();
  await expect(pad).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("虚拟机详情内嵌串口控制台可连接并收发数据", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await page.getByTestId("console-kind-serial").click();
  const panel = page.getByTestId("vm-console-panel");
  await expect(panel).toHaveAttribute("data-kind", "serial");
  await expect(panel).toHaveAttribute("data-state", "connected", { timeout: 15_000 });
  const screen = page.getByTestId("vm-console-serial-screen");
  const terminal = screen.locator(".xterm");
  await expect(terminal).toBeVisible();
  await screen.scrollIntoViewIfNeeded();
  await expect(screen).toContainText("VirtMgr demo serial console");
  await expect(screen).toContainText("login:");

  await screen.locator("textarea").focus();
  await page.keyboard.type("e2e-serial");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("[demo] e2e-serial");
});

test("图形控制台可分离到独立窗口", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByTestId("vm-console-panel")).toHaveAttribute("data-state", "connected", { timeout: 15_000 });

  const popupPromise = page.waitForEvent("popup");
  await page.getByRole("button", { name: "分离", exact: true }).click();
  const popup = await popupPromise;
  await expect(popup).toHaveURL(/\/console\/vms\/22222222-2222-4222-8222-222222222222\?kind=vnc&detached=true$/);
  await expect(popup.getByTestId("vnc-console-title")).toHaveText("demo-db-01");
  await expect(popup.getByTestId("vnc-console-status")).toHaveText("已连接", { timeout: 15_000 });
  await popup.close();
});

test("可写模式创建并删除虚拟机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "新建虚拟机", exact: true }).click();
  await expect(page.getByRole("heading", { name: "新建虚拟机", exact: true })).toBeVisible();

  await page.getByLabel("名称", { exact: true }).fill("e2e-created-01");
  await page.getByLabel("vCPU", { exact: true }).fill("2");
  await page.getByLabel("内存 (MiB)", { exact: true }).fill("2048");
  await page.getByLabel("固件", { exact: true }).selectOption("uefi");
  await page.getByLabel("安全启动 (Secure Boot)", { exact: true }).check();
  await expect(page.getByLabel("启用 vTPM 2.0", { exact: true })).toBeEnabled();
  await page.getByLabel("启用 vTPM 2.0", { exact: true }).check();
  await page.getByLabel("启用 Cloud-init", { exact: true }).check();
  await expect(page.getByTestId("vm-cloud-init-mode-visual")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("vm-cloud-init-network-source")).toContainText("IPv6 fd00:122::1/64 · DHCP");
  await expect(page.getByTestId("vm-cloud-init-network-ipv6-mode")).toHaveValue("dhcpv6");
  await expect(page.getByTestId("vm-cloud-init-network-config")).toHaveValue(/dhcp6: true/);
  await expect(page.getByTestId("vm-cloud-init-network-config")).toHaveValue(/mtu: 1500/);
  await page.getByTestId("vm-cloud-init-runcmd").fill("echo e2e-cloud-init");
  await expect(page.getByTestId("vm-cloud-init-preview")).toContainText("runcmd:");
  await expect(page.getByTestId("vm-cloud-init-preview")).toContainText("echo e2e-cloud-init");
  await page.getByLabel("存储池", { exact: true }).selectOption("default");
  await page.getByLabel("存储卷", { exact: true }).selectOption("demo-db-01.qcow2");
  await expect(page.getByLabel("磁盘路径", { exact: true })).toHaveValue("/var/lib/libvirt/images/demo-db-01.qcow2");
  await expect(page.getByLabel("磁盘格式", { exact: true })).toHaveValue("qcow2");
  await page.getByLabel("虚拟网络", { exact: true }).selectOption("default");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page).toHaveURL(/\/vms\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "e2e-created-01", exact: true })).toBeVisible();
  await expect(page.getByText("已关机", { exact: true }).first()).toBeVisible();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-firmware")).toHaveText("UEFI");
  await expect(page.getByTestId("vm-secure-boot")).toHaveText("已启用");
  await expect(page.getByTestId("vm-tpm")).toHaveText("2.0 已启用");
  await expect(page.getByTestId("vm-cloud-init")).toHaveText("已配置");

  await openVMTab(page, "advanced");
  const deleteButton = page.getByTestId("vm-delete-button");
  await expect(deleteButton).toBeEnabled();
  await deleteButton.click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("删除虚拟机？");
  await dialog.getByLabel("输入虚拟机名称确认").fill("e2e-created-01");
  await dialog.getByTestId("confirm-delete-vm").click();

  await expect(page).toHaveURL(/\/vms$/);
  await expect(page.getByRole("link", { name: "e2e-created-01", exact: true })).toHaveCount(0);
});

test("可写模式通过存储池挂载、替换并弹出安装 ISO", async ({ page }) => {
  await login(page);
  await page.goto("/vms/new");
  await expect(page.getByRole("heading", { name: "新建虚拟机", exact: true })).toBeVisible();

  await page.getByLabel("名称", { exact: true }).fill("e2e-iso-attach-01");
  await page.getByLabel("存储池", { exact: true }).selectOption("default");
  await page.getByLabel("存储卷", { exact: true }).selectOption("demo-db-01.qcow2");
  await page.getByTestId("vm-iso-volume").selectOption("");
  await expect(page.getByLabel("ISO 绝对路径（可选）", { exact: true })).toHaveValue("");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page).toHaveURL(/\/vms\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "e2e-iso-attach-01", exact: true })).toBeVisible();
  await openVMTab(page, "config");
  await expect(page.locator('[data-testid^="vm-disk-row-"]').filter({ hasText: ".iso" })).toHaveCount(0);

  await page.getByTestId("vm-iso-attach-open").click();
  await expect(page.getByTestId("vm-iso-attach-modal")).toBeVisible();
  await page.getByTestId("vm-iso-attach-pool").selectOption("iso-store");
  await page.getByTestId("vm-iso-attach-volume").selectOption("debian-13-netinst.iso");
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px ISO 弹窗不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px ISO 弹窗 body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    await expect(page.getByTestId("vm-iso-attach-submit")).toBeVisible();
  }
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.getByTestId("vm-iso-attach-submit").click();
  await expect(page.getByTestId("vm-disk-resize-notice")).toContainText("安装 ISO debian-13-netinst.iso 已挂载");
  let isoRow = page.locator('[data-testid^="vm-disk-row-"]').filter({ hasText: "debian-13-netinst.iso" });
  await expect(isoRow).toBeVisible();

  await page.getByTestId("vm-iso-attach-open").click();
  await page.getByTestId("vm-iso-attach-pool").selectOption("iso-store");
  await page.getByTestId("vm-iso-attach-volume").selectOption("virtio-win.iso");
  await page.getByTestId("vm-iso-attach-submit").click();
  await expect(page.getByTestId("vm-disk-resize-notice")).toContainText("安装 ISO virtio-win.iso 已挂载");
  isoRow = page.locator('[data-testid^="vm-disk-row-"]').filter({ hasText: "virtio-win.iso" });
  await expect(isoRow).toBeVisible();
  await expect(page.locator('[data-testid^="vm-disk-row-"]').filter({ hasText: "debian-13-netinst.iso" })).toHaveCount(0);
  const ejectButtons = page.locator('[data-testid^="vm-iso-eject-"]');
  await expect(ejectButtons).toHaveCount(1);
  await ejectButtons.first().click();
  await expect(page.getByRole("alertdialog")).toContainText("弹出安装 ISO？");
  await page.getByTestId("vm-iso-eject-submit").click();
  await expect(page.getByText("安装 ISO 已弹出，主磁盘已设为第一启动设备。", { exact: true })).toBeVisible();
  await expect(page.locator('[data-testid^="vm-disk-row-"]').filter({ hasText: ".iso" })).toHaveCount(0);
  await expect(page.getByTestId("vm-disk-source").first()).toContainText("demo-db-01.qcow2");

  await openVMTab(page, "advanced");
  await page.getByTestId("vm-delete-button").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入虚拟机名称确认").fill("e2e-iso-attach-01");
  await dialog.getByTestId("confirm-delete-vm").click();
  await expect(page).toHaveURL(/\/vms$/);
});

test("可写模式创建存储卷并刷新列表", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "存储", exact: true }).click();
  await expect(page).toHaveURL(/\/storage$/);
  await expect(page.getByRole("heading", { name: "存储", exact: true })).toBeVisible();
  await expect(page.getByTestId("storage-pool-volumes-default")).toHaveText("4");

  const openCreate = page.getByTestId("storage-create-volume");
  await expect(openCreate).toBeEnabled();
  await openCreate.click();
  await expect(page.getByTestId("storage-volume-create-form")).toBeVisible();
  await page.getByLabel("卷名称", { exact: true }).fill("e2e-volume-01.qcow2");
  await page.getByLabel("卷格式", { exact: true }).selectOption("qcow2");
  await page.getByLabel("容量 (GiB)", { exact: true }).fill("21");
  await page.getByRole("button", { name: "创建存储卷", exact: true }).click();

  await expect(page.getByTestId("storage-create-success")).toContainText("e2e-volume-01.qcow2");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 5 个卷");
  await expect(page.getByText("e2e-volume-01.qcow2", { exact: true })).toBeVisible();
  await expect(page.getByTestId("storage-pool-volumes-default")).toHaveText("5");

  await openCreate.click();
  await page.getByLabel("卷名称", { exact: true }).fill("e2e-volume-01.qcow2");
  await page.getByRole("button", { name: "创建存储卷", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("资源状态已发生变化");
  await page.getByRole("button", { name: "取消", exact: true }).click();

  await page.getByTestId("delete-volume-e2e-volume-01.qcow2").click();
  const deleteDialog = page.getByRole("alertdialog");
  await expect(deleteDialog).toContainText("删除存储卷？");
  await deleteDialog.getByLabel("输入存储卷名称确认").fill("e2e-volume-01.qcow2");
  await deleteDialog.getByTestId("confirm-delete-volume").click();
  await expect(page.getByTestId("storage-create-success")).toContainText("e2e-volume-01.qcow2 已删除。");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 4 个卷");
  await expect(page.getByTestId("storage-volume-row-e2e-volume-01.qcow2")).toHaveCount(0);
  await expect(page.getByTestId("storage-pool-volumes-default")).toHaveText("4");

  await expect(page.getByTestId("storage-volume-usage-demo-web-01.qcow2")).toContainText("demo-web-01");
  await expect(page.getByTestId("delete-volume-demo-web-01.qcow2")).toBeDisabled();
  await expect(page.getByTestId("delete-volume-demo-web-01.qcow2")).toHaveAttribute("title", /demo-web-01.*不能删除/);
});

test("可写模式转换 qcow2 存储卷为 raw 并跟踪任务", async ({ page }) => {
  await login(page);
  await page.goto("/storage");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 4 个卷");

  await page.getByTestId("storage-create-volume").click();
  await page.getByLabel("卷名称", { exact: true }).fill("e2e-convert-01.qcow2");
  await page.getByLabel("卷格式", { exact: true }).selectOption("qcow2");
  await page.getByLabel("容量 (GiB)", { exact: true }).fill("8");
  await page.getByRole("button", { name: "创建存储卷", exact: true }).click();
  await expect(page.getByTestId("storage-create-success")).toContainText("e2e-convert-01.qcow2");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 5 个卷");

  await page.getByTestId("convert-volume-e2e-convert-01.qcow2").click();
  const modal = page.getByTestId("storage-convert-modal");
  await expect(modal).toBeVisible();
  await expect(modal).toContainText("e2e-convert-01.qcow2");
  await expect(modal).toContainText("qcow2");
  await expect(page.getByTestId("storage-convert-format")).toHaveValue("raw");
  await expect(page.getByTestId("storage-convert-name")).toHaveValue("e2e-convert-01.raw");
  await page.getByTestId("storage-convert-submit").click();

  const task = page.getByTestId("storage-convert-task");
  await expect(task).toHaveAttribute("data-status", "succeeded", { timeout: 10_000 });
  await expect(task).toContainText("100%");
  await expect(page.getByTestId("storage-convert-success")).toContainText("e2e-convert-01.qcow2 已转换为 raw");
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 6 个卷");
  const converted = page.getByTestId("storage-volume-row-e2e-convert-01.raw");
  await expect(converted).toContainText("raw");
  await expect(converted).toContainText(/8(\.0)? GiB/);

  for (const name of ["e2e-convert-01.raw", "e2e-convert-01.qcow2"]) {
    await page.getByTestId(`delete-volume-${name}`).click();
    const dialog = page.getByRole("alertdialog");
    await dialog.getByLabel("输入存储卷名称确认").fill(name);
    await dialog.getByTestId("confirm-delete-volume").click();
    await expect(page.getByTestId("storage-create-success")).toContainText(`${name} 已删除`);
  }
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 4 个卷");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("storage.volume.convert");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const auditRow = page.locator('[data-testid^="audit-row-"]').first();
  await expect(auditRow).toContainText("e2e-convert-01.qcow2");
  await expect(auditRow).toContainText("成功");
});

test("可写模式创建 NFS 和 LVM 存储池并约束 LVM 卷", async ({ page }) => {
  await login(page);
  await page.goto("/storage");
  await expect(page.getByRole("heading", { name: "存储", exact: true })).toBeVisible();

  await page.getByTestId("storage-create-pool").click();
  await expect(page.getByTestId("storage-pool-create-form")).toBeVisible();
  await page.getByTestId("storage-pool-name").fill("e2e-nfs-pool");
  await page.getByTestId("storage-pool-type").selectOption("netfs");
  await expect(page.getByTestId("storage-pool-source-host")).toBeVisible();
  await expect(page.getByTestId("storage-pool-source-path")).toBeVisible();
  await page.getByTestId("storage-pool-target-path").fill("/mnt/e2e-nfs");
  await page.getByTestId("storage-pool-source-host").fill("nfs.example.com");
  await page.getByTestId("storage-pool-source-path").fill("/export/vms");
  await page.getByTestId("storage-pool-create-submit").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-nfs-pool 已创建");
  const nfsRow = page.getByTestId("storage-pool-row-e2e-nfs-pool");
  await expect(nfsRow).toContainText("netfs");
  await expect(nfsRow).toContainText("活跃");

  await page.getByTestId("storage-create-pool").click();
  await page.getByTestId("storage-pool-name").fill("e2e-lvm-pool");
  await page.getByTestId("storage-pool-type").selectOption("logical");
  await expect(page.getByTestId("storage-pool-volume-group")).toBeVisible();
  await page.getByTestId("storage-pool-target-path").fill("/dev/vg0");
  await page.getByTestId("storage-pool-volume-group").fill("vg0");
  await page.getByTestId("storage-pool-create-submit").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-lvm-pool 已创建");
  const lvmRow = page.getByTestId("storage-pool-row-e2e-lvm-pool");
  await expect(lvmRow).toContainText("logical");
  await expect(lvmRow).toContainText("活跃");

  await expect(page.getByTestId("storage-upload-iso")).toBeDisabled();
  await expect(page.getByTestId("storage-upload-iso")).toHaveAttribute("title", /LVM logical 池不支持文件上传/);
  await page.getByTestId("storage-create-volume").click();
  await expect(page.getByTestId("storage-volume-create-form")).toContainText("LVM logical 池仅支持 .raw 块卷");
  await expect(page.getByLabel("卷格式", { exact: true })).toHaveValue("raw");
  await expect(page.getByLabel("卷格式", { exact: true }).locator('option[value="qcow2"]')).toBeDisabled();
  await page.getByRole("button", { name: "取消", exact: true }).last().click();

  for (const name of ["e2e-lvm-pool", "e2e-nfs-pool"]) {
    await page.getByTestId(`storage-pool-stop-${name}`).click();
    await expect(page.getByTestId("storage-pool-success")).toContainText(`存储池 ${name} 已停止`);
    await page.getByTestId(`storage-pool-delete-${name}`).click();
    const dialog = page.getByRole("alertdialog");
    await dialog.getByLabel("输入存储池名称确认").fill(name);
    await dialog.getByTestId("confirm-delete-pool").click();
    await expect(page.getByTestId("storage-pool-success")).toContainText(`存储池 ${name} 已删除`);
    await expect(page.getByTestId(`storage-pool-row-${name}`)).toHaveCount(0);
  }
});

test("可写模式展示审计记录并支持筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await expect(page).toHaveURL(/\/audit$/);
  await expect(page.getByRole("heading", { name: "审计日志", exact: true })).toBeVisible();
  await expect(page.getByTestId("audit-total")).toHaveText(/^共 \d+ 条审计记录$/);
  await expect(page.getByText("管理员", { exact: true }).first()).toBeVisible();

  await page.getByTestId("audit-action-filter").selectOption("vm.start");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const auditRow = page.locator('[data-testid^="audit-row-"]').first();
  await expect(auditRow).toContainText("启动虚拟机");
  await expect(auditRow).toContainText("成功");

  await page.getByTestId("audit-result-filter").selectOption("failure");
  await expect(page.getByTestId("audit-total")).toHaveText("共 0 条审计记录");
  await expect(page.getByTestId("audit-empty")).toBeVisible();

  await page.getByTestId("audit-action-filter").selectOption("");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  await expect(page.locator('[data-testid^="audit-row-"]').filter({ hasText: "失败" })).toHaveCount(1);
});

test("可写模式上传存储卷并刷新列表", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "存储", exact: true }).click();
  await page.getByRole("button", { name: "iso-store", exact: true }).click();
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 2 个卷");

  const uploadButton = page.getByTestId("storage-upload-iso");
  await expect(uploadButton).toBeEnabled();
  await uploadButton.click();
  await expect(page.getByTestId("storage-upload-form")).toBeVisible();
  await page.getByTestId("storage-upload-input").setInputFiles({
    name: "e2e-upload.iso",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc(1024 * 1024, 1),
  });
  await page.getByTestId("storage-upload-submit").click();

  await expect(page.getByTestId("storage-upload-success")).toContainText("e2e-upload.iso");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 3 个卷");
  await expect(page.getByTestId("storage-volume-row-e2e-upload.iso")).toContainText("raw");
  await expect(page.getByTestId("storage-pool-volumes-iso-store")).toHaveText("3");

  await page.getByTestId("delete-volume-e2e-upload.iso").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入存储卷名称确认").fill("e2e-upload.iso");
  await dialog.getByTestId("confirm-delete-volume").click();
  await expect(page.getByTestId("storage-create-success")).toContainText("e2e-upload.iso 已删除。");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 2 个卷");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("storage.volume.upload");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("e2e-upload.iso");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("成功");

  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByTestId("tasks-total")).toHaveText(/共 \d+ 个任务/);
  const uploadTask = page.locator('[data-testid^="task-row-"]').filter({ hasText: "e2e-upload.iso" }).first();
  await expect(uploadTask).toContainText("上传存储卷");
  await expect(uploadTask).toContainText("已完成");
  await expect(uploadTask).toContainText("100%");
  await expect(uploadTask).toContainText("存储卷上传完成");
});

test("可写模式管理存储池生命周期", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "存储", exact: true }).click();
  await expect(page.getByTestId("storage-pool-total")).toHaveText("3");

  const createPool = page.getByTestId("storage-create-pool");
  await expect(createPool).toBeEnabled();
  await createPool.click();
  await expect(page.getByTestId("storage-pool-create-form")).toBeVisible();
  await page.getByLabel("存储池名称", { exact: true }).fill("e2e-pool-01");
  await page.getByLabel("目标路径", { exact: true }).fill("/srv/virtmgr/e2e-pool-01");
  await page.getByLabel("启用开机自启动", { exact: true }).check();
  await page.getByLabel("创建后立即启动", { exact: true }).uncheck();
  await page.getByTestId("storage-pool-create-submit").click();

  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-pool-01 已创建。");
  await expect(page.getByTestId("storage-pool-total")).toHaveText("4");
  const row = page.getByTestId("storage-pool-row-e2e-pool-01");
  await expect(row).toContainText("未激活");
  await expect(page.getByTestId("storage-pool-start-e2e-pool-01")).toContainText("启动");
  await expect(page.getByTestId("storage-pool-delete-e2e-pool-01")).toContainText("删除");
  await expect(page.getByTestId("storage-pool-refresh-e2e-pool-01")).toHaveCount(0);
  await expect(row).toContainText("/srv/virtmgr/e2e-pool-01");

  await page.getByTestId("storage-pool-start-e2e-pool-01").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-pool-01 已启动。");
  await expect(row).toContainText("活跃");
  await expect(page.getByTestId("storage-pool-active")).toHaveText("3");
  await expect(page.getByTestId("storage-pool-refresh-e2e-pool-01")).toContainText("刷新");
  await expect(page.getByTestId("storage-pool-stop-e2e-pool-01")).toContainText("停止");
  await expect(page.getByTestId("storage-pool-delete-e2e-pool-01")).toHaveCount(0);

  await page.getByTestId("storage-pool-refresh-e2e-pool-01").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-pool-01 已刷新。");

  await page.getByTestId("storage-pool-stop-e2e-pool-01").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-pool-01 已停止。");
  await expect(row).toContainText("未激活");
  await expect(page.getByTestId("storage-pool-active")).toHaveText("2");

  await page.getByTestId("storage-pool-delete-e2e-pool-01").click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("删除存储池？");
  await dialog.getByLabel("输入存储池名称确认").fill("e2e-pool-01");
  await dialog.getByTestId("confirm-delete-pool").click();
  await expect(page.getByTestId("storage-pool-success")).toContainText("存储池 e2e-pool-01 已删除。");
  await expect(page.getByTestId("storage-pool-total")).toHaveText("3");
  await expect(page.getByTestId("storage-pool-row-e2e-pool-01")).toHaveCount(0);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const [action, label] of [
    ["storage.pool.create", "创建存储池"],
    ["storage.pool.start", "启动存储池"],
    ["storage.pool.refresh", "刷新存储池"],
    ["storage.pool.stop", "停止存储池"],
    ["storage.pool.delete", "删除存储池"],
  ] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    const auditRow = page
      .locator('[data-testid^="audit-row-"]')
      .filter({ hasText: "e2e-pool-01" })
      .first();
    await expect(auditRow).toBeVisible();
    await expect(auditRow).toContainText(label);
    await expect(auditRow).toContainText("e2e-pool-01");
    await expect(auditRow).toContainText("成功");
  }
});

test("可写模式克隆虚拟机并跟踪异步任务", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-legacy-01", exact: true }).click();
  await openVMTab(page, "advanced");

  const openClone = page.getByTestId("vm-clone-open");
  await expect(openClone).toBeEnabled();
  await openClone.click();
  await expect(page.getByTestId("vm-clone-form")).toBeVisible();
  await page.getByLabel("克隆名称", { exact: true }).fill("e2e-clone-01");
  await page.getByTestId("vm-clone-submit").click();

  const task = page.getByTestId("vm-clone-task");
  await expect(task).toHaveAttribute("data-status", "succeeded", { timeout: 10_000 });
  await expect(task).toContainText("虚拟机 e2e-clone-01 克隆完成");
  await expect(task).toContainText("100%");

  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  const cloneTask = page.locator('[data-testid^="task-row-"]').filter({ hasText: "e2e-clone-01" }).first();
  await expect(cloneTask).toContainText("克隆虚拟机");
  await expect(cloneTask).toContainText("已完成");
  await expect(cloneTask).toContainText("100%");

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByRole("link", { name: "e2e-clone-01", exact: true })).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.clone");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const auditRow = page.locator('[data-testid^="audit-row-"]').first();
  await expect(auditRow).toContainText("克隆虚拟机");
  await expect(auditRow).toContainText("成功");
  await expect(auditRow).toContainText("e2e-clone-01");
});

test("可写模式创建、回滚和删除虚拟机快照", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-legacy-01", exact: true }).click();
  await openVMTab(page, "snapshots");
  await expect(page.getByTestId("vm-snapshots-card")).toBeVisible();
  await expect(page.getByTestId("vm-snapshot-empty")).toContainText("暂无快照");

  await page.getByTestId("vm-snapshot-create-toggle").click();
  await expect(page.getByTestId("vm-snapshot-create-form")).toBeVisible();
  await page.getByLabel("快照名称", { exact: true }).fill("e2e-baseline");
  await page.getByLabel("描述（可选）", { exact: true }).fill("before e2e upgrade");
  await page.getByTestId("vm-snapshot-create-submit").click();

  await expect(page.getByTestId("vm-snapshot-notice")).toContainText("快照 e2e-baseline 已创建。");
  const row = page.getByTestId("vm-snapshot-row-e2e-baseline");
  await expect(row).toContainText("before e2e upgrade");
  await expect(row).toContainText("当前");
  await expect(row).toContainText("已关机");

  await page.getByTestId("vm-snapshot-revert-e2e-baseline").click();
  const revertDialog = page.getByTestId("vm-snapshot-revert-dialog");
  await expect(revertDialog).toContainText("回滚到快照？");
  await revertDialog.getByTestId("vm-snapshot-confirm-revert").click();
  await expect(page.getByTestId("vm-snapshot-notice")).toContainText("快照 e2e-baseline 已回滚");
  await expect(row).toContainText("当前");

  await page.getByTestId("vm-snapshot-delete-e2e-baseline").click();
  const deleteDialog = page.getByTestId("vm-snapshot-delete-dialog");
  await expect(deleteDialog).toContainText("删除快照？");
  await deleteDialog.getByTestId("vm-snapshot-confirm-delete").click();
  await expect(page.getByTestId("vm-snapshot-notice")).toContainText("快照 e2e-baseline 已删除。");
  await expect(page.getByTestId("vm-snapshot-row-e2e-baseline")).toHaveCount(0);
  await expect(page.getByTestId("vm-snapshot-empty")).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const [action, target] of [["vm.snapshot.create", "创建快照"], ["vm.snapshot.revert", "回滚快照"], ["vm.snapshot.delete", "删除快照"]] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText(target);
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("e2e-baseline");
  }
});

test("可写模式直通 PCI 和 USB 设备并写入域 XML", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-passthrough-card")).toBeVisible();
  await expect(page.getByTestId("vm-passthrough-empty")).toContainText("当前虚拟机还没有直通 PCI / USB 设备");
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const responsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/host/devices") && response.request().method() === "GET");
    await page.getByTestId("vm-passthrough-refresh").click();
    expect((await responsePromise).status()).toBe(200);
    await expect(page.getByText("加载失败", { exact: true })).toHaveCount(0);
  }

  await page.getByTestId("vm-passthrough-select").selectOption("pci_0000_3b_00_0");
  await page.getByTestId("vm-passthrough-attach").click();
  await expect(page.getByTestId("vm-passthrough-notice")).toContainText("设备 pci_0000_3b_00_0 已直通到虚拟机");
  await expect(page.getByTestId("vm-passthrough-total")).toHaveText("1 个设备");
  const pciRow = page.getByTestId("vm-passthrough-row-pci_0000_3b_00_0");
  await expect(pciRow).toContainText("PCI");
  await expect(pciRow).toContainText("NVIDIA Corporation");
  await expect(pciRow).toContainText("0000:3b:00.0");

  await page.getByTestId("vm-passthrough-select").selectOption("usb_device_046d_c52b_1_2");
  await page.getByTestId("vm-passthrough-attach").click();
  await expect(page.getByTestId("vm-passthrough-notice")).toContainText("设备 usb_device_046d_c52b_1_2 已直通到虚拟机");
  await expect(page.getByTestId("vm-passthrough-total")).toHaveText("2 个设备");
  const usbRow = page.getByTestId("vm-passthrough-row-usb_device_046d_c52b_1_2");
  await expect(usbRow).toContainText("USB");
  await expect(usbRow).toContainText("Logitech, Inc.");
  await expect(usbRow).toContainText("bus 1 · device 2");

  await openVMTab(page, "advanced");
  const xml = page.getByTestId("vm-xml-content");
  await expect(xml).toContainText("<hostdev mode='subsystem' type='pci'");
  const completeXML = await readVMXML(page, "22222222-2222-4222-8222-222222222222");
  expect(completeXML).toContain("<hostdev mode='subsystem' type='usb'");
  expect(completeXML).toContain("bus='0x3b'");
  expect(completeXML).toContain("vendor id='0x046d'");

  await openVMTab(page, "config");
  await page.getByTestId("vm-passthrough-detach-pci_0000_3b_00_0").click();
  const detachDialog = page.getByTestId("vm-passthrough-detach-dialog");
  await expect(detachDialog).toContainText("移除直通设备？");
  await detachDialog.getByTestId("vm-passthrough-confirm-detach").click();
  await expect(page.getByTestId("vm-passthrough-notice")).toContainText("设备 pci_0000_3b_00_0 已从虚拟机移除");
  await expect(page.getByTestId("vm-passthrough-row-pci_0000_3b_00_0")).toHaveCount(0);
  await expect(page.getByTestId("vm-passthrough-total")).toHaveText("1 个设备");

  await page.getByTestId("vm-passthrough-detach-usb_device_046d_c52b_1_2").click();
  await page.getByTestId("vm-passthrough-detach-dialog").getByTestId("vm-passthrough-confirm-detach").click();
  await expect(page.getByTestId("vm-passthrough-notice")).toContainText("设备 usb_device_046d_c52b_1_2 已从虚拟机移除");
  await expect(page.getByTestId("vm-passthrough-empty")).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const [action, target] of [["vm.device.attach", "pci_0000_3b_00_0"], ["vm.device.detach", "usb_device_046d_c52b_1_2"]] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText("共 2 条审计记录");
    const rows = page.locator('[data-testid^="audit-row-"]');
    await expect(rows.filter({ hasText: target })).toHaveCount(1);
    await expect(rows.filter({ hasText: target })).toContainText("成功");
  }
});

test("管理员创建、调整和禁用用户并验证访问控制", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "用户管理", exact: true }).click();
  await expect(page).toHaveURL(/\/users$/);
  await expect(page.getByTestId("users-total")).toHaveText("1");

  await page.getByTestId("user-create-toggle").click();
  await page.getByLabel("用户名", { exact: true }).fill("e2e-viewer");
  await page.getByLabel("初始密码", { exact: true }).fill("viewer-secret-123");
  await page.getByLabel("角色", { exact: true }).selectOption("viewer");
  await page.getByTestId("user-create-submit").click();

  await expect(page.getByTestId("user-action-success")).toContainText("用户 e2e-viewer 已创建。");
  await expect(page.getByTestId("users-total")).toHaveText("2");
  const viewerRow = page.getByTestId("user-row-e2e-viewer");
  await expect(viewerRow).toContainText("已启用");

  await page.getByLabel("修改 e2e-viewer 的角色").selectOption("operator");
  await expect(page.getByTestId("user-action-success")).toContainText("用户 e2e-viewer 已更新。");
  await expect(page.getByLabel("修改 e2e-viewer 的角色")).toHaveValue("operator");

  await page.getByTestId("user-toggle-e2e-viewer").click();
  await expect(viewerRow).toContainText("已禁用");

  await page.getByRole("button", { name: "退出登录" }).click();
  await page.goto("/login");
  await page.getByLabel("用户名").fill("e2e-viewer");
  await page.getByLabel("密码").fill("viewer-secret-123");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("用户名或密码错误");

  await login(page);
  await page.getByRole("link", { name: "用户管理", exact: true }).click();
  await expect(page.getByTestId("user-row-e2e-viewer")).toContainText("已禁用");
  await page.getByTestId("user-toggle-e2e-viewer").click();
  await expect(page.getByTestId("user-row-e2e-viewer")).toContainText("已启用");

  await page.getByRole("button", { name: "退出登录" }).click();
  await loginAs(page, "e2e-viewer", "viewer-secret-123");
  await expect(page.getByRole("link", { name: "用户管理", exact: true })).toHaveCount(0);
  await page.goto("/users");
  await expect(page.getByRole("heading", { name: "用户管理", exact: true })).toBeVisible();
  await expect(page.getByText("用户管理仅对管理员开放", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "前往账号安全", exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByTestId("change-password-card")).toBeVisible();
});

test("可写模式创建、启停和删除虚拟网络", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "网络", exact: true }).click();
  await expect(page).toHaveURL(/\/networks$/);
  await expect(page.getByTestId("network-total")).toHaveText("3");
  const activeRow = page.getByTestId("network-row-default");
  await expect(activeRow).toContainText("停止");
  await expect(page.getByTestId("network-start-default")).toHaveCount(0);
  await expect(page.getByTestId("network-edit-default")).toBeDisabled();
  await expect(page.getByTestId("network-delete-default")).toBeDisabled();
  await expect(page.getByTestId("network-action-hint-default")).toContainText("网络运行中");

  await page.getByTestId("network-create-toggle").click();
  await expect(page.getByTestId("network-create-form")).toBeVisible();
  await page.getByLabel("网络名称", { exact: true }).fill("e2e-net-01");
  await page.getByLabel("网络模式", { exact: true }).selectOption("isolated");
  await page.getByTestId("network-subnet").fill("10.99.0.0/24");
  await page.getByTestId("network-ipv6-subnet").fill("fd00:99::/64");
  await page.getByTestId("network-ipv6-dhcp-start").fill("fd00:99::100");
  await page.getByTestId("network-ipv6-dhcp-end").fill("fd00:99::1ff");
  await page.getByTestId("network-mtu").fill("1400");
  await expect(page.getByLabel("创建后立即启动", { exact: true })).not.toBeChecked();
  await page.getByTestId("network-create-submit").click();

  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-net-01 已创建。");
  await expect(page.getByTestId("network-total")).toHaveText("4");
  const row = page.getByTestId("network-row-e2e-net-01");
  await expect(row).toContainText("isolated");
  await expect(row).toContainText("未激活");
  await expect(row).toContainText("10.99.0.1/24");
  await expect(row).toContainText("fd00:99::1/64");
  await expect(row).toContainText("DHCP 10.99.0.2 - 10.99.0.254");
  await expect(row).toContainText("DHCP fd00:99::100 - fd00:99::1ff");
  await expect(row).toContainText("MTU 1400");
  await expect(row).toContainText("启动");
  await expect(page.getByTestId("network-stop-e2e-net-01")).toHaveCount(0);

  await page.getByTestId("network-edit-e2e-net-01").click();
  await expect(page.getByTestId("network-edit-form")).toBeVisible();
  await expect(page.getByLabel("网络名称", { exact: true }).last()).toHaveValue("e2e-net-01");
  await page.getByLabel("网络模式", { exact: true }).last().selectOption("nat");
  await page.getByTestId("network-edit-subnet").fill("10.99.1.0/24");
  await page.getByTestId("network-edit-mtu").fill("1500");
  await expect(page.getByTestId("network-edit-ipv6-subnet")).toHaveValue("fd00:99::1/64");
  await page.getByLabel("DHCP 起始地址（可选）", { exact: true }).last().fill("10.99.1.20");
  await page.getByLabel("DHCP 结束地址（可选）", { exact: true }).last().fill("10.99.1.120");
  await page.getByLabel("开机自动启动", { exact: true }).last().check();
  await page.getByTestId("network-edit-submit").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-net-01 配置已更新。");
  await expect(row).toContainText("nat");
  await expect(row).toContainText("10.99.1.1/24");
  await expect(row).toContainText("DHCP 10.99.1.20 - 10.99.1.120");
  await expect(row).toContainText("MTU 1500");
  await expect(row).toContainText("是");
  await expect(page.getByTestId("network-edit-form")).toHaveCount(0);

  await page.getByTestId("network-start-e2e-net-01").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-net-01 已启动。");
  await expect(row).toContainText("活跃");
  await expect(page.getByTestId("network-active")).toHaveText("3");

  await page.getByTestId("network-hosts-e2e-net-01").click();
  await expect(page.getByTestId("network-hosts-card")).toContainText("静态 DHCP / DHCPv6 主机 · e2e-net-01");
  await page.getByLabel("MAC 地址", { exact: true }).fill("52:54:00:aa:bb:cc");
  await page.getByLabel("IPv4 / IPv6 地址", { exact: true }).fill("10.99.1.50");
  await page.getByLabel("主机名（可选）", { exact: true }).fill("e2e-static-01");
  await page.getByTestId("network-host-submit").click();
  await expect(page.getByTestId("network-action-success")).toContainText("已立即生效");
  await expect(page.getByTestId("network-host-row-52:54:00:aa:bb:cc")).toContainText("e2e-static-01");
  await expect(page.getByTestId("network-host-row-52:54:00:aa:bb:cc")).toContainText("10.99.1.50");
  await page.getByTestId("network-host-delete-52:54:00:aa:bb:cc").click();
  const hostDialog = page.getByRole("alertdialog");
  await expect(hostDialog).toContainText("删除静态主机条目？");
  await hostDialog.getByTestId("confirm-delete-network-host").click();
  await expect(page.getByTestId("network-action-success")).toContainText("静态主机 52:54:00:aa:bb:cc 已删除");
  await expect(page.getByTestId("network-host-row-52:54:00:aa:bb:cc")).toHaveCount(0);

  await page.getByLabel("IPv4 / IPv6 地址", { exact: true }).fill("fd00:99::50");
  await expect(page.getByTestId("network-host-mac")).toBeDisabled();
  await page.getByLabel("DUID / Client ID（IPv6）", { exact: true }).fill("00:03:00:01:52:54:00:aa:bb:cd");
  await page.getByLabel("主机名（可选）", { exact: true }).fill("e2e-static-ipv6");
  await page.getByTestId("network-host-submit").click();
  await expect(page.getByTestId("network-host-row-00:03:00:01:52:54:00:aa:bb:cd")).toContainText("fd00:99::50");
  await expect(page.getByTestId("network-host-row-00:03:00:01:52:54:00:aa:bb:cd")).toContainText("e2e-static-ipv6");

  await page.getByTestId("network-dns-host-ip").fill("fd00:99::60");
  await page.getByTestId("network-dns-hostnames").fill("e2e-dns.lab.example, e2e-dns");
  await page.getByTestId("network-dns-host-submit").click();
  await expect(page.getByTestId("network-action-success")).toContainText("DNS 记录已添加");
  const dnsRow = page.getByTestId("network-dns-host-row-fd00:99::60");
  await expect(dnsRow).toContainText("e2e-dns.lab.example, e2e-dns");
  await expect(dnsRow).toContainText("AAAA");
  await page.getByTestId("network-dns-host-delete-fd00:99::60").click();
  const dnsDialog = page.getByRole("alertdialog");
  await expect(dnsDialog).toContainText("删除静态 DNS 记录？");
  await dnsDialog.getByTestId("confirm-delete-network-dns-host").click();
  await expect(page.getByTestId("network-action-success")).toContainText("DNS 记录 fd00:99::60 已删除");
  await expect(page.getByTestId("network-dns-host-row-fd00:99::60")).toHaveCount(0);

  await page.getByTestId("network-diagnostics-e2e-net-01").click();
  await expect(page.getByTestId("network-diagnostics-card")).toContainText("NAT66 已启用");
  await expect(page.getByTestId("network-diagnostic-active")).toContainText("libvirt 网络处于活跃状态");
  await expect(page.getByTestId("network-diagnostic-dhcpv6")).toContainText("fd00:99::100 - fd00:99::1ff");
  await expect(page.getByTestId("network-diagnostic-mtu")).toContainText("MTU 1500");

  await page.getByTestId("network-stop-e2e-net-01").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-net-01 已停止。");
  await expect(row).toContainText("未激活");
  await expect(page.getByTestId("network-active")).toHaveText("2");

  await page.getByTestId("network-delete-e2e-net-01").click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("删除虚拟网络？");
  await dialog.getByLabel("输入网络名称确认").fill("e2e-net-01");
  await dialog.getByTestId("confirm-delete-network").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-net-01 已删除。");
  await expect(page.getByTestId("network-total")).toHaveText("3");
  await expect(page.getByTestId("network-row-e2e-net-01")).toHaveCount(0);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await expect(page.getByTestId("audit-action-filter")).toContainText("创建虚拟网络");
  for (const [action, label] of [
    ["network.create", "创建虚拟网络"],
    ["network.update", "编辑虚拟网络"],
    ["network.dns.host.add", "添加静态 DNS 记录"],
    ["network.dns.host.delete", "删除静态 DNS 记录"],
    ["network.delete", "删除虚拟网络"],
  ] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    const auditRow = page
      .locator('[data-testid^="audit-row-"]')
      .filter({ hasText: "e2e-net-01" })
      .first();
    await expect(auditRow).toBeVisible();
    await expect(auditRow).toContainText(label);
    await expect(auditRow).toContainText("成功");
  }
});

test("可写模式创建 IPv6-only SLAAC 网络并完成诊断", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "网络", exact: true }).click();
  await expect(page.getByTestId("network-total")).toHaveText("3");

  await page.getByTestId("network-create-toggle").click();
  await expect(page.getByTestId("network-create-form")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  const layout = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
    bodyWidth: document.body.scrollWidth,
  }));
  expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth);
  expect(layout.bodyWidth).toBeLessThanOrEqual(layout.viewportWidth);

  await page.getByLabel("网络名称", { exact: true }).fill("e2e-v6-only");
  await page.getByLabel("网络模式", { exact: true }).selectOption("isolated");
  await page.getByTestId("network-subnet").fill("");
  await page.getByTestId("network-ipv6-subnet").fill("fd00:9a::/64");
  await page.getByTestId("network-ipv6-mode").selectOption("slaac");
  await page.getByTestId("network-dns-domain").fill("v6.example.com");
  await page.getByTestId("network-dns-servers").fill("2606:4700:4700::1111");
  await page.getByTestId("network-create-submit").click();

  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-v6-only 已创建。");
  await expect(page.getByTestId("network-total")).toHaveText("4");
  const row = page.getByTestId("network-row-e2e-v6-only");
  await expect(row).toContainText("fd00:9a::1/64");
  await expect(row).toContainText("slaac");
  await expect(row).toContainText("DNS v6.example.com · 2606:4700:4700::1111");

  await page.getByTestId("network-start-e2e-v6-only").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-v6-only 已启动。");
  await page.getByTestId("network-diagnostics-e2e-v6-only").click();
  await expect(page.getByTestId("network-diagnostics-card")).toContainText("IPv6 / 转发诊断 · e2e-v6-only");
  await expect(page.getByTestId("network-diagnostic-active")).toContainText("libvirt 网络处于活跃状态");
  await expect(page.getByTestId("network-diagnostic-dhcpv6")).toContainText("使用 RA/SLAAC");

  await page.getByTestId("network-stop-e2e-v6-only").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-v6-only 已停止。");
  await page.getByTestId("network-delete-e2e-v6-only").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入网络名称确认").fill("e2e-v6-only");
  await dialog.getByTestId("confirm-delete-network").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-v6-only 已删除。");
  await expect(page.getByTestId("network-total")).toHaveText("3");
});

test("可写模式将 DHCPv6 租约保留为静态主机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "网络", exact: true }).click();
  await page.getByTestId("network-leases-default").click();

  const leaseRow = page.locator('[data-testid^="network-lease-row-"]').filter({ hasText: "fd00:122::51" });
  await expect(leaseRow).toContainText("demo-app-01");
  await expect(leaseRow.getByTestId(/network-lease-reserve-/)).toBeEnabled();
  await leaseRow.getByTestId(/network-lease-reserve-/).click();
  await expect(page.getByTestId("network-action-success")).toContainText("已保留为网络 default 的静态主机");

  await page.getByTestId("network-hosts-default").click();
  const hostRow = page.getByTestId("network-host-row-0:3:0:1:0:16:3e:11:22:44");
  await expect(hostRow).toContainText("fd00:122::51");
  await expect(hostRow).toContainText("demo-app-01");
  await expect(page.getByTestId("network-host-row-0:3:0:1:0:16:3e:11:22:33")).toContainText("fd00:122::50");
});

test("可写模式维护虚拟机文件夹和资源分组并支持列表筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-web-01", exact: true }).click();
  await openVMTab(page, "config");

  const metadataCard = page.getByTestId("vm-metadata-card");
  await expect(metadataCard).toBeVisible();
  await metadataCard.getByRole("button", { name: "编辑", exact: true }).click();
  await page.getByTestId("vm-metadata-folder").fill("E2E/生产/Web");
  await page.getByTestId("vm-metadata-group").fill("验收业务");
  await page.getByTestId("vm-metadata-save").click();
  await expect(metadataCard).toContainText("虚拟机组织信息已保存。");
  await expect(metadataCard).toContainText("E2E/生产/Web");
  await expect(metadataCard).toContainText("验收业务");

  await page.goto("/vms");
  await expect(page.getByTestId("vm-name-cell-11111111-1111-4111-8111-111111111111")).toContainText("E2E/生产/Web");
  await expect(page.getByTestId("vm-name-cell-11111111-1111-4111-8111-111111111111")).toContainText("验收业务");

  await page.getByLabel("文件夹筛选").fill("E2E/生产");
  await expect(page.getByTestId("vm-name-cell-11111111-1111-4111-8111-111111111111")).toBeVisible();
  await expect(page.getByTestId("vm-name-cell-22222222-2222-4222-8222-222222222222")).toHaveCount(0);

  await page.getByLabel("文件夹筛选").fill("");
  await page.getByLabel("资源分组筛选").fill("验收业务");
  await expect(page.getByTestId("vm-name-cell-11111111-1111-4111-8111-111111111111")).toBeVisible();
  await expect(page.getByTestId("vm-name-cell-22222222-2222-4222-8222-222222222222")).toHaveCount(0);
});

test("可写模式创建外部桥接网络并校验网桥切换", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "网络", exact: true }).click();
  await expect(page.getByTestId("network-total")).toHaveText("3");
  await expect(page.getByTestId("network-bridges")).toHaveText("1");

  await page.getByTestId("network-create-toggle").click();
  await page.getByLabel("网络名称", { exact: true }).fill("e2e-bridge-01");
  await page.getByLabel("网络模式", { exact: true }).selectOption("bridge");
  await expect(page.getByLabel("IPv4 子网", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("DHCP 起始地址（可选）", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("宿主机网桥", { exact: true })).toBeVisible();
  await page.getByLabel("宿主机网桥", { exact: true }).fill("br1");

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const mobileLayout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(mobileLayout.documentWidth, `${width}px 桥接网络表单不应横向溢出`).toBeLessThanOrEqual(mobileLayout.viewportWidth);
    expect(mobileLayout.bodyWidth, `${width}px 桥接网络表单 body 不应横向溢出`).toBeLessThanOrEqual(mobileLayout.viewportWidth);
    await expect(page.getByTestId("network-bridge")).toBeVisible();
  }
  await page.setViewportSize({ width: 1280, height: 720 });

  await page.getByTestId("network-create-submit").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-bridge-01 已创建。");
  await expect(page.getByTestId("network-total")).toHaveText("4");
  await expect(page.getByTestId("network-bridges")).toHaveText("2");
  const row = page.getByTestId("network-row-e2e-bridge-01");
  await expect(row).toContainText("bridge");
  await expect(row).toContainText("br1");
  await expect(row).toContainText("由上游网桥分配");
  await expect(page.getByTestId("network-leases-e2e-bridge-01")).toBeDisabled();
  await expect(page.getByTestId("network-hosts-e2e-bridge-01")).toBeDisabled();

  await page.getByTestId("network-edit-e2e-bridge-01").click();
  await expect(page.getByTestId("network-edit-form")).toBeVisible();
  await expect(page.getByLabel("IPv4 子网", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("宿主机网桥", { exact: true })).toHaveValue("br1");
  await page.getByLabel("宿主机网桥", { exact: true }).fill("br2");
  await page.getByTestId("network-edit-submit").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-bridge-01 配置已更新。");
  await expect(row).toContainText("br2");

  await page.getByTestId("network-start-e2e-bridge-01").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-bridge-01 已启动。");
  await expect(row).toContainText("活跃");
  await page.getByTestId("network-stop-e2e-bridge-01").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-bridge-01 已停止。");

  await page.getByTestId("network-delete-e2e-bridge-01").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入网络名称确认").fill("e2e-bridge-01");
  await dialog.getByTestId("confirm-delete-network").click();
  await expect(page.getByTestId("network-action-success")).toContainText("网络 e2e-bridge-01 已删除。");
  await expect(page.getByTestId("network-total")).toHaveText("3");
  await expect(page.getByTestId("network-bridges")).toHaveText("1");
});

test("可写模式切换虚拟机开机自启动", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-build-01", exact: true }).click();
  await openVMTab(page, "config");

  const toggle = page.getByTestId("vm-autostart-toggle");
  await expect(toggle).toBeEnabled();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await toggle.click();
  await expect(page.getByTestId("vm-autostart-notice")).toHaveText("开机自启动已启用。");
  await expect(toggle).toHaveAttribute("aria-checked", "true");

  await page.reload();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-autostart-toggle")).toHaveAttribute("aria-checked", "true");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.autostart");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const row = page.locator('[data-testid^="audit-row-"]').first();
  await expect(row).toContainText("更新开机自启动");
  await expect(row).toContainText("demo-build-01");
  await expect(row).toContainText("成功");
});

test("可写模式创建存储池使用率告警规则", async ({ page }) => {
  await login(page);
  await page.goto("/host");
  const card = page.getByTestId("host-alert-rules");
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "新建规则", exact: true }).click();
  const modal = page.getByTestId("alert-rule-modal");
  await expect(modal).toBeVisible();
  await page.getByTestId("alert-rule-name").fill("e2e-storage-alert");
  await page.getByLabel("指标", { exact: true }).selectOption("storage_pool_percent");
  await page.getByTestId("alert-rule-threshold").fill("1");
  modal.getByRole("button", { name: "保存规则", exact: true }).click();
  await expect(card.getByRole("status")).toContainText("告警规则 e2e-storage-alert 已保存");
  const row = card.getByRole("row").filter({ hasText: "e2e-storage-alert" });
  await expect(row).toContainText("存储池使用率（最高活跃池）");
  await card.getByRole("button", { name: "删除规则 e2e-storage-alert" }).click();
  await page.getByTestId("alert-rule-delete-modal").getByRole("button", { name: "确认删除", exact: true }).click();
  await expect(card.getByRole("status")).toContainText("告警规则 e2e-storage-alert 已删除");
});

test("管理员可运行时切换只读模式", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "系统设置", exact: true }).click();
  await expect(page.getByRole("heading", { name: "系统设置", exact: true })).toBeVisible();
  await expect(page.getByTestId("settings-readonly-status")).toHaveText("写操作已启用");

  await page.getByTestId("settings-toggle-readonly").click();
  await expect(page.getByTestId("settings-readonly-status")).toHaveText("只读模式已启用");
  await expect(page.getByTestId("settings-success")).toContainText("只读模式已启用");

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByTestId("vm-read-only-notice")).toBeVisible();
  await expect(page.getByTestId("vm-batch-toolbar")).toHaveCount(0);

  await page.getByRole("link", { name: "系统设置", exact: true }).click();
  await page.getByTestId("settings-toggle-readonly").click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("关闭只读模式？");
  await dialog.getByTestId("settings-confirm-write-enable").click();
  await expect(page.getByTestId("settings-readonly-status")).toHaveText("写操作已启用");

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByTestId("vm-read-only-notice")).toHaveCount(0);
  await expect(page.getByTestId("vm-batch-toolbar")).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("settings.readonly.update");
  await expect(page.getByTestId("audit-total")).toHaveText("共 2 条审计记录");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("更新只读模式");
});

test("事件页实时接收虚拟机生命周期事件", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "事件", exact: true }).click();
  await expect(page.getByTestId("events-live-status")).toHaveText("实时连接");
  const rows = page.locator('[data-testid^="event-row-"]');
  await expect(rows.first()).toBeVisible();
  const initialCount = await rows.count();

  const actionPage = await page.context().newPage();
  await actionPage.goto("/vms/44444444-4444-4444-8444-444444444444");
  await expect(actionPage.getByRole("heading", { name: "demo-legacy-01", exact: true })).toBeVisible();
  await actionPage.getByTestId("vm-action-start").click();
  await expect(actionPage.getByText("启动指令已提交，状态已刷新。", { exact: true })).toBeVisible();

  await expect(rows).toHaveCount(initialCount + 1);
  await expect(rows.first()).toContainText("虚拟机已启动");
  await expect(rows.first()).toContainText("demo-legacy-01");
  await actionPage.close();
});

test("可写模式管理虚拟机模板并用模板创建虚拟机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机模板", exact: true }).click();
  await expect(page).toHaveURL(/\/templates$/);
  await expect(page.getByTestId("template-total")).toHaveText("2");

  await page.getByTestId("template-create-toggle").click();
  await expect(page.getByTestId("template-form")).toBeVisible();
  await page.getByLabel("模板名称", { exact: true }).fill("e2e-template");
  await page.getByLabel("描述", { exact: true }).fill("Playwright template lifecycle");
  await page.getByLabel("vCPU", { exact: true }).fill("3");
  await page.getByLabel("内存 (MiB)", { exact: true }).fill("3072");
  await page.getByLabel("磁盘格式", { exact: true }).selectOption("raw");
  await page.getByLabel("网络名称", { exact: true }).fill("default");
  await page.getByLabel("固件", { exact: true }).selectOption("uefi");
  await page.getByLabel("模板安全启动", { exact: true }).check();
  await page.getByLabel("模板启用 Cloud-init", { exact: true }).check();
  await expect(page.getByTestId("template-cloud-init-mode-visual")).toHaveAttribute("aria-selected", "true");
  await page.getByTestId("template-cloud-init-packages").fill("curl\nvim");
  await page.getByTestId("template-cloud-init-runcmd").fill("echo e2e-template");
  await page.getByTestId("template-cloud-init-create-user").check();
  await page.getByTestId("template-cloud-init-username").fill("deploy");
  await page.getByTestId("template-cloud-init-ssh-keys").fill("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA e2e@example");
  await expect(page.getByTestId("template-cloud-init-preview")).toContainText("packages:");
  await expect(page.getByTestId("template-cloud-init-preview")).toContainText("- curl");
  await expect(page.getByTestId("template-cloud-init-preview")).toContainText("users:");
  await expect(page.getByTestId("template-cloud-init-preview")).toContainText("ssh_authorized_keys:");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByTestId("template-cloud-init-editor")).toBeVisible();
  await expect(page.getByTestId("template-cloud-init-preview")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
  await page.getByTestId("template-cloud-init-mode-source").click();
  const yamlEditor = page.getByTestId("template-cloud-init-yaml-editor");
  await expect(yamlEditor).toContainText("name: deploy");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
  await yamlEditor.fill("#cloud-config\ntimezone: UTC\npackages:\n  - htop\nruncmd:\n  - echo from-yaml\n");
  await page.getByTestId("template-cloud-init-mode-visual").click();
  await expect(page.getByTestId("template-cloud-init-timezone")).toHaveValue("UTC");
  await expect(page.getByTestId("template-cloud-init-packages")).toHaveValue("htop");
  await expect(page.getByTestId("template-cloud-init-runcmd")).toHaveValue("echo from-yaml");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByTestId("template-submit").click();
  await expect(page.getByTestId("template-success")).toContainText("模板 e2e-template 已创建。");
  await expect(page.getByTestId("template-total")).toHaveText("3");
  await expect(page.getByTestId("template-row-e2e-template")).toContainText("3 vCPU · 3072 MiB");
  await expect(page.getByTestId("template-row-e2e-template")).toContainText("UEFI · Secure Boot");

  await page.getByTestId("template-edit-e2e-template").click();
  await expect(page.getByLabel("模板名称", { exact: true })).toHaveValue("e2e-template");
  await page.getByLabel("内存 (MiB)", { exact: true }).fill("3584");
  await page.getByTestId("template-submit").click();
  await expect(page.getByTestId("template-success")).toContainText("模板 e2e-template 已更新。");
  await expect(page.getByTestId("template-row-e2e-template")).toContainText("3 vCPU · 3584 MiB");

  await page.getByTestId("template-use-e2e-template").click();
  await expect(page).toHaveURL(/\/vms\/new\?template=\d+$/);
  await expect(page.getByLabel("vCPU", { exact: true })).toHaveValue("3");
  await expect(page.getByLabel("内存 (MiB)", { exact: true })).toHaveValue("3584");
  await expect(page.getByLabel("磁盘格式", { exact: true })).toHaveValue("raw");
  await expect(page.getByLabel("固件", { exact: true })).toHaveValue("uefi");
  await expect(page.getByLabel("安全启动 (Secure Boot)", { exact: true })).toBeChecked();
  await expect(page.getByLabel("启用 Cloud-init", { exact: true })).toBeChecked();
  await page.getByLabel("名称", { exact: true }).fill("e2e-template-vm");
  await page.getByLabel("存储池", { exact: true }).selectOption("default");
  await page.getByLabel("存储卷", { exact: true }).selectOption("demo-db-01.qcow2");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page).toHaveURL(/\/vms\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "e2e-template-vm", exact: true })).toBeVisible();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-firmware")).toHaveText("UEFI");
  await expect(page.getByTestId("vm-secure-boot")).toHaveText("已启用");
  await expect(page.getByTestId("vm-cloud-init")).toHaveText("已配置");
  await openVMTab(page, "advanced");
  await page.getByTestId("vm-delete-button").click();
  const vmDialog = page.getByRole("alertdialog");
  await vmDialog.getByLabel("输入虚拟机名称确认").fill("e2e-template-vm");
  await vmDialog.getByTestId("confirm-delete-vm").click();
  await expect(page).toHaveURL(/\/vms$/);

  await page.getByRole("link", { name: "虚拟机模板", exact: true }).click();
  await page.getByTestId("template-delete-e2e-template").click();
  const templateDialog = page.getByRole("alertdialog");
  await expect(templateDialog).toContainText("删除虚拟机模板？");
  await templateDialog.getByLabel("输入模板名称确认").fill("e2e-template");
  await templateDialog.getByTestId("confirm-delete-template").click();
  await expect(page.getByTestId("template-success")).toContainText("模板 e2e-template 已删除。");
  await expect(page.getByTestId("template-total")).toHaveText("2");
  await expect(page.getByTestId("template-row-e2e-template")).toHaveCount(0);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const action of ["vm_template.create", "vm_template.update", "vm_template.delete"]) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("e2e-template");
  }
});

test("可写模式管理定时快照策略并立即执行", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await openVMTab(page, "snapshots");
  await expect(page.getByTestId("vm-snapshot-policy-card")).toBeVisible();
  await expect(page.getByTestId("vm-snapshot-policy-total")).toHaveText("0 个策略");
  await expect(page.getByTestId("vm-snapshot-policy-empty")).toContainText("暂无快照策略");

  await page.getByTestId("vm-snapshot-policy-create-toggle").click();
  await expect(page.getByTestId("vm-snapshot-policy-form")).toBeVisible();
  await page.getByLabel("策略名称", { exact: true }).fill("e2e-nightly");
  await page.getByLabel("间隔（分钟）", { exact: true }).fill("60");
  await page.getByLabel("保留快照数", { exact: true }).fill("2");
  await expect(page.getByLabel("启用策略", { exact: true })).toBeChecked();
  await page.getByTestId("vm-snapshot-policy-submit").click();
  await expect(page.getByTestId("vm-snapshot-policy-notice")).toContainText("策略 e2e-nightly 已创建。");
  await expect(page.getByTestId("vm-snapshot-policy-total")).toHaveText("1 个策略");
  const row = page.getByTestId("vm-snapshot-policy-row-e2e-nightly");
  await expect(row).toContainText("60 分钟");
  await expect(row).toContainText("2 份");
  await expect(row).toContainText("已启用");

  await page.getByTestId("vm-snapshot-policy-run-e2e-nightly").click();
  await expect(page.getByTestId("vm-snapshot-policy-notice")).toContainText("策略 e2e-nightly 已执行，快照 e2e-nightly-");
  await expect(row).toContainText("e2e-nightly-");
  const snapshotRows = page.locator('[data-testid^="vm-snapshot-row-"]');
  await expect(snapshotRows).toHaveCount(1);
  await expect(snapshotRows.first()).toContainText("e2e-nightly-");

  await page.getByTestId("vm-snapshot-policy-edit-e2e-nightly").click();
  await expect(page.getByLabel("策略名称", { exact: true })).toHaveValue("e2e-nightly");
  await page.getByLabel("间隔（分钟）", { exact: true }).fill("120");
  await page.getByLabel("保留快照数", { exact: true }).fill("1");
  await page.getByLabel("启用策略", { exact: true }).uncheck();
  await page.getByTestId("vm-snapshot-policy-submit").click();
  await expect(page.getByTestId("vm-snapshot-policy-notice")).toContainText("策略 e2e-nightly 已更新。");
  await expect(row).toContainText("120 分钟");
  await expect(row).toContainText("1 份");
  await expect(row).toContainText("已禁用");

  await page.getByTestId("vm-snapshot-policy-toggle-e2e-nightly").click();
  await expect(page.getByTestId("vm-snapshot-policy-notice")).toContainText("策略 e2e-nightly 已启用。");
  await expect(row).toContainText("已启用");

  await page.getByTestId("vm-snapshot-policy-delete-e2e-nightly").click();
  const dialog = page.getByTestId("vm-snapshot-policy-delete-dialog");
  await expect(dialog).toContainText("删除快照策略？");
  await dialog.getByTestId("vm-snapshot-policy-confirm-delete").click();
  await expect(page.getByTestId("vm-snapshot-policy-notice")).toContainText("策略 e2e-nightly 已删除。");
  await expect(page.getByTestId("vm-snapshot-policy-total")).toHaveText("0 个策略");
  await expect(page.getByTestId("vm-snapshot-policy-empty")).toBeVisible();
  await expect(snapshotRows).toHaveCount(1);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const [action, label, count] of [
    ["vm.snapshot_policy.create", "创建快照策略", "共 1 条审计记录"],
    ["vm.snapshot_policy.update", "更新快照策略", "共 2 条审计记录"],
    ["vm.snapshot.schedule", "执行快照策略", "共 1 条审计记录"],
    ["vm.snapshot_policy.delete", "删除快照策略", "共 1 条审计记录"],
  ] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText(count);
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText(label);
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("e2e-nightly");
  }
});

test("管理员校验并保存虚拟机持久 XML", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-legacy-01", exact: true }).click();
  await openVMTab(page, "advanced");
  await page.getByLabel("XML 类型").selectOption("config");
  const xml = page.getByTestId("vm-xml-content");
  await expect(xml).toContainText("<vcpu>2</vcpu>");

  const editButton = page.getByTestId("vm-xml-edit");
  await expect(editButton).toBeEnabled();
  await editButton.click();
  const editor = page.getByTestId("vm-xml-editor");
  await expect(editor).toBeVisible();
  const original = await readVMXML(page, "44444444-4444-4444-8444-444444444444", "config");
  const edited = original
    .replace("<vcpu>2</vcpu>", "<vcpu>3</vcpu>")
    .replace(/<topology([^>]*?)cores=(["'])2\2/, (_match, prefix: string, quote: string) => `<topology${prefix}cores=${quote}3${quote}`);
  await replaceEditorDocument(page, editor, edited);
  await page.getByTestId("vm-xml-save").click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("保存持久配置 XML？");
  await dialog.getByTestId("confirm-vm-xml-save").click();

  await expect(page.getByTestId("vm-xml-edit-notice")).toContainText("持久配置 XML 已保存");
  await expect(page.getByTestId("vm-xml-content")).toContainText("<vcpu>3</vcpu>");
  expect(await readVMXML(page, "44444444-4444-4444-8444-444444444444", "config")).toContain("<vcpu>3</vcpu>");
  await page.reload();
  await openVMTab(page, "advanced");
  await page.getByLabel("XML 类型").selectOption("config");
  await expect(page.getByTestId("vm-xml-content")).toContainText("<vcpu>3</vcpu>");
  expect(await readVMXML(page, "44444444-4444-4444-8444-444444444444", "config")).toMatch(/cores=["']3["']/);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.xml.update");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("更新虚拟机 XML");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("demo-legacy-01");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("成功");
});

test("可写模式通过备份中心完成创建、校验、恢复、调度与清理", async ({ page }) => {
  test.setTimeout(120_000);
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await expect(page.getByTestId("vm-detail-name")).toHaveText("demo-legacy-01");

  const shutdown = page.getByTestId("vm-action-shutdown");
  if (await shutdown.isVisible()) {
    await shutdown.click();
    await expect(page.getByText("正常关机指令已提交，状态已刷新。", { exact: true })).toBeVisible();
  }
  await expect(page.getByTestId("vm-action-start")).toBeEnabled();

  await page.goto("/backups");
  await expect(page).toHaveURL(/\/backups$/);
  await expect(page.getByRole("heading", { name: "备份中心", exact: true })).toBeVisible();
  await expect(page.getByTestId("backups-empty")).toContainText("还没有备份集");

  await page.getByTestId("backup-create-open").click();
  const createModal = page.getByTestId("backup-create-modal");
  await expect(createModal).toBeVisible();
  await selectOptionContaining(createModal.getByTestId("backup-create-vm"), "demo-legacy-01");
  await createModal.getByTestId("backup-create-name").fill("e2e-backup-01");
  await selectOptionContaining(createModal.getByTestId("backup-create-repository"), "default");
  await createModal.getByRole("button", { name: "开始备份", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("创建备份已完成。", { timeout: 30_000 });

  const backupRow = page.locator('[data-testid^="backup-row-"]').filter({ hasText: "e2e-backup-01" });
  await expect(backupRow).toContainText("已完成");
  await expect(backupRow).toContainText("未校验");

  await page.getByRole("button", { name: "校验 e2e-backup-01", exact: true }).click();
  const verifyModal = page.getByTestId("backup-verify-modal");
  await expect(verifyModal).toBeVisible();
  await verifyModal.getByRole("checkbox").check();
  await verifyModal.getByRole("button", { name: "开始校验", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("校验备份已完成。", { timeout: 30_000 });
  await expect(backupRow).toContainText("校验通过");

  await page.getByRole("button", { name: "恢复 e2e-backup-01", exact: true }).click();
  const restoreModal = page.getByTestId("backup-restore-modal");
  await expect(restoreModal).toBeVisible();
  await restoreModal.getByTestId("restore-name").fill("e2e-restored-01");
  await selectOptionContaining(restoreModal.getByTestId("restore-pool"), "default");
  await restoreModal.getByRole("button", { name: "预检并恢复", exact: true }).click();
  await expect(restoreModal).toContainText("即将创建 e2e-restored-01");
  await restoreModal.getByRole("button", { name: "确认恢复", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("恢复虚拟机已完成。", { timeout: 30_000 });

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByRole("link", { name: "e2e-restored-01", exact: true })).toBeVisible();

  await page.goto("/backups");
  await page.getByTestId("backup-tab-schedules").click();
  await expect(page.getByTestId("backup-schedules-empty")).toContainText("还没有定时计划");
  await page.getByTestId("backup-schedule-create").click();
  const scheduleModal = page.getByTestId("backup-schedule-modal");
  await expect(scheduleModal).toBeVisible();
  await selectOptionContaining(scheduleModal.getByTestId("backup-schedule-vm"), "demo-legacy-01");
  await scheduleModal.getByTestId("backup-schedule-name").fill("e2e-nightly-backup");
  await selectOptionContaining(scheduleModal.getByTestId("backup-schedule-repository"), "default");
  await scheduleModal.getByTestId("backup-schedule-interval").fill("12");
  await scheduleModal.getByTestId("backup-schedule-retention").fill("3");
  await scheduleModal.getByRole("button", { name: "创建计划", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("定时计划已创建。");
  const scheduleRow = page.locator('[data-testid^="backup-schedule-row-"]').filter({ hasText: "e2e-nightly-backup" });
  await expect(scheduleRow).toContainText("每 12 小时");
  await expect(scheduleRow).toContainText("已启用");
  await scheduleRow.getByRole("button", { name: "暂停", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("计划已暂停。");
  await expect(scheduleRow).toContainText("已暂停");
  await scheduleRow.getByRole("button", { name: "启用", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("计划已启用。");
  await expect(scheduleRow).toContainText("已启用");

  await scheduleRow.getByRole("button", { name: "立即执行", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("执行定时备份已完成。", { timeout: 30_000 });
  await scheduleRow.getByRole("button", { name: "删除", exact: true }).click();
  await page.getByTestId("backup-schedule-delete-confirm").click();
  await expect(page.getByTestId("backup-notice")).toContainText("定时计划已删除。");

  await page.getByTestId("backup-tab-backups").click();
  await expect(backupRow).toBeVisible();
  await page.getByRole("button", { name: "固定 e2e-backup-01", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("已固定保留");
  await expect(page.getByRole("button", { name: "取消固定 e2e-backup-01", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "删除 e2e-backup-01", exact: true }).click();
  const deleteModal = page.getByTestId("backup-delete-modal");
  await expect(deleteModal).toBeVisible();
  await deleteModal.getByTestId("backup-delete-name").fill("e2e-backup-01");
  await deleteModal.getByTestId("backup-delete-confirm").click();
  await expect(page.getByTestId("backup-notice")).toContainText("删除备份已完成。", { timeout: 30_000 });
  await expect(backupRow).toHaveCount(0);

  await page.getByTestId("backup-tab-repositories").click();
  const repositoryRow = page.locator('[data-testid^="backup-repository-row-"]').filter({ hasText: "iso-store" });
  await expect(repositoryRow).toBeVisible();
  await repositoryRow.getByRole("button", { name: "编辑", exact: true }).click();
  const repositoryModal = page.getByTestId("backup-repository-modal");
  await expect(repositoryModal).toBeVisible();
  await repositoryModal.getByTestId("backup-repository-name").fill("e2e-backup-repo");
  await repositoryModal.getByRole("button", { name: "保存修改", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("备份存储库已更新。");
  const updatedRepositoryRow = page.locator('[data-testid^="backup-repository-row-"]').filter({ hasText: "e2e-backup-repo" });
  await expect(updatedRepositoryRow).toBeVisible();
  await updatedRepositoryRow.getByRole("button", { name: "删除", exact: true }).click();
  await page.getByTestId("backup-repository-delete-confirm").click();
  await expect(page.getByTestId("backup-notice")).toContainText("备份存储库已删除。");
  await expect(updatedRepositoryRow).toHaveCount(0);

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const [action, label] of [
    ["backup.create", "创建备份"],
    ["backup.verify", "校验备份"],
    ["backup.restore", "恢复备份"],
    ["backup.schedule.create", "创建备份计划"],
    ["backup.schedule.run", "立即执行备份计划"],
    ["backup.schedule.delete", "删除备份计划"],
    ["backup.pin", "更新备份保留"],
    ["backup.delete", "删除备份"],
    ["backup.repository.update", "更新备份存储库"],
    ["backup.repository.delete", "删除备份存储库"],
  ] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText(/共 [1-9]\d* 条审计记录/);
    const auditRow = page.locator('[data-testid^="audit-row-"]').first();
    await expect(auditRow).toContainText(label);
    await expect(auditRow).toContainText("成功");
  }
});

test("可写模式通过备份中心在线备份运行中虚拟机并展示一致性信息", async ({ page }) => {
  test.setTimeout(120_000);
  await login(page);
  await page.goto("/backups");
  await expect(page.getByRole("heading", { name: "备份中心", exact: true })).toBeVisible();

  await page.getByTestId("backup-create-open").click();
  const createModal = page.getByTestId("backup-create-modal");
  await expect(createModal).toBeVisible();
  await selectOptionContaining(createModal.getByTestId("backup-create-vm"), "demo-web-01");
  await createModal.getByTestId("backup-create-name").fill("e2e-online-backup-01");
  await selectOptionContaining(createModal.getByTestId("backup-create-repository"), "default");
  await expect(createModal.getByLabel("运行中在线备份", { exact: true })).toBeChecked();
  await createModal.getByLabel("Guest Agent 静默", { exact: true }).check();
  await createModal.getByRole("button", { name: "开始备份", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("创建备份已完成。", { timeout: 30_000 });

  const backupRow = page.locator('[data-testid^="backup-row-"]').filter({ hasText: "e2e-online-backup-01" });
  await expect(backupRow).toContainText("已完成");
  await page.getByRole("button", { name: "查看 e2e-online-backup-01 详情", exact: true }).click();
  const detail = page.getByTestId("backup-detail-modal");
  await expect(detail).toContainText("在线 · Agent 静默");
  await expect(detail).toContainText("磁盘快照一致性");
  await expect(detail).toContainText("未使用");
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();

  await page.goto("/vms/11111111-1111-4111-8111-111111111111");
  await openVMTab(page, "snapshots");
  await expect(page.getByTestId("vm-backup-card")).toContainText("在线备份");
  await expect(page.getByTestId("vm-backup-open")).toBeEnabled();
});

test("可写模式调整运行中虚拟机的 CPU 和内存并写入审计", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await openVMTab(page, "config");

  await expect(page.getByTestId("vm-resources-card")).toBeVisible();
  await expect(page.getByTestId("vm-resources-form")).toBeVisible();
  await expect(page.getByTestId("vm-resources-vcpus")).toHaveValue("8");
  await expect(page.getByTestId("vm-resources-memory")).toHaveValue("16384");

  await page.getByTestId("vm-resources-vcpus").fill("10");
  await page.getByTestId("vm-resources-memory").fill("18432");
  await page.getByTestId("vm-resources-save").click();

  await expect(page.getByTestId("vm-resources-notice")).toContainText("CPU 与内存已更新为 10 vCPU / 18 GiB");
  await expect(page.getByTestId("vm-resources-current-vcpus")).toHaveText("10");
  await expect(page.getByTestId("vm-resources-current-memory")).toHaveText("18 GiB");
  await openVMTab(page, "overview");
  await expect(page.getByTestId("vm-detail-vcpus")).toHaveText("10");
  await expect(page.getByTestId("vm-detail-memory")).toHaveText("18 GiB");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.resources.update");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const row = page.locator('[data-testid^="audit-row-"]').first();
  await expect(row).toContainText("调整虚拟机资源");
  await expect(row).toContainText("成功");
  await expect(row).toContainText("vcpus: 10");
});

test("可写模式修改启动顺序、CPU 拓扑和磁盘网卡限速", async ({ page }) => {
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await expect(page.getByTestId("vm-detail-name")).toHaveText("demo-legacy-01");
  await openVMTab(page, "config");

  await page.getByTestId("vm-advanced-config-open").click();
  const advanced = page.getByTestId("vm-advanced-config-modal");
  await expect(advanced).toBeVisible();
  await page.getByTestId("vm-boot-device-0").selectOption("network");
  await page.getByTestId("vm-boot-device-1").selectOption("hd");
  await page.getByTestId("vm-boot-save").click();
  await expect(page.getByTestId("vm-resources-notice")).toContainText("启动顺序已更新为 网络 → 硬盘");

  const vcpuSummary = await advanced.getByText(/CPU 拓扑乘积必须等于当前 \d+ 个 vCPU。/).textContent();
  const currentVCPUs = Number(vcpuSummary?.match(/当前 (\d+) 个 vCPU/)?.[1]);
  expect(Number.isSafeInteger(currentVCPUs)).toBeTruthy();
  await page.getByTestId("vm-cpu-model").fill("EPYC");
  await page.getByTestId("vm-cpu-sockets").fill("1");
  await page.getByTestId("vm-cpu-cores").fill(String(currentVCPUs + 1));
  await page.getByTestId("vm-cpu-threads").fill("1");
  await page.getByTestId("vm-cpu-save").click();
  await expect(advanced).toContainText(`CPU 拓扑乘积必须等于当前 ${currentVCPUs} 个 vCPU`);
  await page.getByTestId("vm-cpu-cores").fill(String(currentVCPUs));
  await page.getByTestId("vm-cpu-save").click();
  await expect(page.getByTestId("vm-resources-notice")).toContainText(`CPU 型号与拓扑已更新，共 ${currentVCPUs} 个 vCPU`);
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();

  const vm = await readVM(page, "44444444-4444-4444-8444-444444444444") as {
    bootDevices: string[];
    cpuModel: string;
    cpuSockets: number;
    cpuCores: number;
    cpuThreads: number;
  };
  expect(vm.bootDevices).toEqual(["network", "hd"]);
  expect(vm.cpuModel).toBe("EPYC");
  expect([vm.cpuSockets, vm.cpuCores, vm.cpuThreads]).toEqual([1, currentVCPUs, 1]);

  await page.getByTestId("vm-disk-qos-vda").click();
  const diskModal = page.getByTestId("vm-disk-qos-modal");
  await expect(diskModal).toBeVisible();
  await page.getByTestId("vm-disk-qos-total").fill("64");
  await page.getByTestId("vm-disk-qos-read").fill("32");
  await page.getByTestId("vm-disk-qos-write").fill("16");
  await page.getByTestId("vm-disk-qos-total-iops").fill("1000");
  await page.getByTestId("vm-disk-qos-read-iops").fill("600");
  await page.getByTestId("vm-disk-qos-write-iops").fill("400");
  await page.getByTestId("vm-disk-qos-submit").click();
  await expect(page.getByTestId("vm-disk-resize-notice")).toContainText("vda 的磁盘 I/O 限制已更新", { timeout: 10_000 });

  await page.getByTestId("vm-network-qos-52:54:00:01:00:04").click();
  const networkModal = page.getByTestId("vm-network-qos-modal");
  await expect(networkModal).toBeVisible();
  await page.getByTestId("vm-network-qos-in").fill("20");
  await page.getByTestId("vm-network-qos-in-peak").fill("30");
  await page.getByTestId("vm-network-qos-in-burst").fill("2");
  await page.getByTestId("vm-network-qos-out").fill("10");
  await page.getByTestId("vm-network-qos-out-peak").fill("15");
  await page.getByTestId("vm-network-qos-out-burst").fill("1");
  await page.getByTestId("vm-network-qos-submit").click();
  await expect(page.getByTestId("vm-network-interfaces-card")).toContainText("网卡 52:54:00:01:00:04 的带宽限制已更新", { timeout: 10_000 });

  const configured = await readVM(page, "44444444-4444-4444-8444-444444444444") as {
    disks: Array<{ target: string; qos?: Record<string, number> }>;
    interfaces: Array<{ mac: string; qos?: Record<string, number> }>;
  };
  expect(configured.disks.find((disk) => disk.target === "vda")?.qos).toMatchObject({
    totalBytesSec: 64 * 1024 * 1024,
    readBytesSec: 32 * 1024 * 1024,
    writeBytesSec: 16 * 1024 * 1024,
    totalIops: 1000,
    readIops: 600,
    writeIops: 400,
  });
  expect(configured.interfaces.find((item) => item.mac === "52:54:00:01:00:04")?.qos).toMatchObject({
    inboundAverageBytes: 20 * 1024 * 1024,
    inboundPeakBytes: 30 * 1024 * 1024,
    inboundBurstBytes: 2 * 1024 * 1024,
    outboundAverageBytes: 10 * 1024 * 1024,
    outboundPeakBytes: 15 * 1024 * 1024,
    outboundBurstBytes: 1 * 1024 * 1024,
  });

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px 高级配置不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 高级配置 body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
  }

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  for (const action of ["vm.boot.update", "vm.cpu.update", "vm.disk.qos.update", "vm.network.qos.update"] as const) {
    await page.getByTestId("audit-action-filter").selectOption(action);
    await expect(page.getByTestId("audit-total")).toHaveText(/共 [1-9]\d* 条审计记录/);
    await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("成功");
  }
});

test("可写模式扩容 qcow2 磁盘并写入审计", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await openVMTab(page, "config");

  await expect(page.getByTestId("vm-disks-card")).toBeVisible();
  await expect(page.getByTestId("vm-disk-capacity-vda")).toHaveText("80 GiB");
  const resize = page.getByTestId("vm-disk-resize-vda");
  await expect(resize).toBeEnabled();
  await resize.click();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("当前容量 80 GiB");
  await expect(page.getByTestId("vm-disk-resize-input")).toHaveValue("90");
  await page.getByTestId("vm-disk-resize-input").fill("100");
  await page.getByTestId("vm-disk-resize-submit").click();

  await expect(page.getByTestId("vm-disk-resize-notice")).toContainText("vda 已扩容至 100 GiB");
  await expect(page.getByTestId("vm-disk-capacity-vda")).toHaveText("100 GiB");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.disk.resize");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const row = page.locator('[data-testid^="audit-row-"]').first();
  await expect(row).toContainText("扩容虚拟机磁盘");
  await expect(row).toContainText("成功");
  await expect(row).toContainText("target: vda");
  await expect(row).toContainText("capacityBytes: 107374182400");
});

test("管理员固定并回滚域 XML 版本且记录审计", async ({ page }) => {
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await expect(page.getByTestId("vm-detail-name")).toHaveText("demo-legacy-01");
  await openVMTab(page, "advanced");

  await page.getByTestId("vm-config-versions-open").click();
  const dialog = page.getByTestId("vm-config-versions-dialog");
  await expect(dialog).toBeVisible();
  const baseline = dialog.getByTestId("vm-config-version-item-1");
  await expect(baseline).toBeVisible({ timeout: 15_000 });
  await expect(baseline).toContainText("初始基线");
  await baseline.click();

  await expect(dialog.getByTestId("vm-config-version-xml")).toContainText("<vcpu>2</vcpu>");
  await expect(dialog.getByTestId("vm-config-version-current-xml")).toContainText("<vcpu>3</vcpu>");

  await dialog.getByTestId("vm-config-version-pin").click();
  await expect(dialog.getByTestId("vm-config-version-notice")).toContainText("已固定");
  await expect(dialog.getByTestId("vm-config-version-item-1")).toContainText("已固定");

  await dialog.getByTestId("vm-config-version-rollback").click();
  const confirmation = page.getByTestId("vm-config-version-rollback-dialog");
  await expect(confirmation).toContainText("回滚持久配置到 v1");
  await confirmation.getByTestId("vm-config-version-rollback-note").fill("playwright restore baseline");
  await confirmation.getByTestId("vm-config-version-rollback-confirm").click();

  await expect(dialog.getByTestId("vm-config-version-notice")).toContainText("已回滚到 v1，并自动创建版本 v");
  await expect(dialog.getByTestId("vm-config-version-current-xml")).toContainText("<vcpu>2</vcpu>");
  await expect(dialog.locator('[data-testid^="vm-config-version-item-"]').first()).toContainText("XML 版本回滚");
  await expect(dialog.getByTestId("vm-config-version-rollback")).toBeDisabled();

  await dialog.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.config.rollback");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const row = page.locator('[data-testid^="audit-row-"]').first();
  await expect(row).toContainText("回滚域 XML 版本");
  await expect(row).toContainText("成功");
  await expect(row).toContainText("sourceVersion: 1");
  await expect(row).toContainText("playwright restore baseline");
});

test("可写模式登记、刷新并移除镜像条目", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "镜像库", exact: true }).click();
  await expect(page).toHaveURL(/\/images$/);
  await expect(page.getByTestId("image-total")).toHaveText("1");
  await page.getByTestId("image-create-toggle").click();
  await expect(page.getByTestId("image-form")).toBeVisible();
  await page.getByTestId("image-pool").selectOption("default");
  await page.getByTestId("image-volume").selectOption("demo-db-01.qcow2");
  await page.getByTestId("image-name").fill("e2e-image");
  await page.getByLabel("OS 名称", { exact: true }).fill("E2E Linux");
  await page.getByTestId("image-submit").click();

  await expect(page.getByTestId("image-notice")).toContainText("镜像 e2e-image 已登记");
  await expect(page.getByTestId("image-total")).toHaveText("2");
  const row = page.getByRole("row").filter({ hasText: "e2e-image" });
  await expect(row).toContainText("demo-db-01.qcow2");
  await expect(row).toContainText("未计算");
  await row.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(page.getByTestId("image-notice")).toContainText("SHA-256 已刷新");
  await expect(row).toContainText("已计算");
  await expect(row).not.toContainText("未计算");

  await row.getByRole("button", { name: "移除", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("不会删除存储池中的卷");
  await dialog.getByTestId("confirm-delete-image").click();
  await expect(page.getByTestId("image-notice")).toContainText("底层磁盘卷未被删除");
  await expect(page.getByTestId("image-total")).toHaveText("1");
  await expect(page.getByRole("row").filter({ hasText: "e2e-image" })).toHaveCount(0);

  await page.getByRole("link", { name: "存储", exact: true }).click();
  await expect(page.getByText("demo-db-01.qcow2", { exact: true }).first()).toBeVisible();
});

test("可写模式上传 qcow2 镜像并自动登记", async ({ page }) => {
  await login(page);
  await page.goto("/images");
  await expect(page.getByTestId("image-total")).toHaveText("1");
  await page.getByTestId("image-upload-toggle").click();
  await expect(page.getByTestId("image-form")).toBeVisible();
  await page.getByTestId("image-name").fill("e2e-uploaded-image");
  await page.getByTestId("image-upload-file").setInputFiles({
    name: "e2e-uploaded-image.qcow2",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc(1024 * 1024, 2),
  });
  await expect(page.getByTestId("image-upload-file")).toBeVisible();
  await page.getByTestId("image-submit").click();
  await expect(page.getByTestId("image-notice")).toContainText("已上传并登记");
  await expect(page.getByTestId("image-total")).toHaveText("2");
  const row = page.getByRole("row").filter({ hasText: "e2e-uploaded-image" });
  await expect(row).toContainText("e2e-uploaded-image.qcow2");

  await row.getByRole("button", { name: "移除", exact: true }).click();
  await page.getByTestId("confirm-delete-image").click();
  await expect(page.getByTestId("image-notice")).toContainText("底层磁盘卷未被删除");
  await expect(page.getByTestId("image-total")).toHaveText("1");

  await page.goto("/storage");
  await page.getByRole("button", { name: "default", exact: true }).click();
  await page.getByTestId("delete-volume-e2e-uploaded-image.qcow2").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入存储卷名称确认").fill("e2e-uploaded-image.qcow2");
  await dialog.getByTestId("confirm-delete-volume").click();
  await expect(page.getByTestId("storage-volume-row-e2e-uploaded-image.qcow2")).toHaveCount(0);
});

test("模板绑定镜像并从镜像全量克隆创建虚拟机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机模板", exact: true }).click();
  await page.getByTestId("template-create-toggle").click();
  await page.getByLabel("模板名称", { exact: true }).fill("e2e-image-template");
  await page.getByLabel("描述", { exact: true }).fill("Image clone template");
  const imageOption = page.getByTestId("template-image").locator("option").filter({ hasText: "ubuntu-24.04-cloud-base" });
  await expect(imageOption).toHaveCount(1);
  const imageId = await imageOption.getAttribute("value");
  if (!imageId) throw new Error("演示镜像缺少选项值");
  await page.getByTestId("template-image").selectOption(imageId);
  await page.getByTestId("template-submit").click();
  await expect(page.getByTestId("template-success")).toContainText("模板 e2e-image-template 已创建");
  await expect(page.getByTestId("template-row-e2e-image-template")).toContainText("镜像 · ubuntu-24.04-cloud-base");

  await page.getByTestId("template-use-e2e-image-template").click();
  await expect(page).toHaveURL(/\/vms\/new\?template=\d+$/);
  await expect(page.getByTestId("vm-disk-source-image")).toBeChecked();
  await expect(page.getByTestId("vm-image").locator("option:checked")).toContainText("ubuntu-24.04-cloud-base");
  await page.getByLabel("名称", { exact: true }).fill("e2e-image-vm");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page).toHaveURL(/\/vms\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "e2e-image-vm", exact: true })).toBeVisible();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-disk-source")).toHaveText("/var/lib/libvirt/images/e2e-image-vm-disk.qcow2");

  await openVMTab(page, "advanced");
  await page.getByTestId("vm-delete-button").click();
  const vmDialog = page.getByRole("alertdialog");
  await vmDialog.getByLabel("输入虚拟机名称确认").fill("e2e-image-vm");
  await vmDialog.getByTestId("confirm-delete-vm").click();
  await expect(page).toHaveURL(/\/vms$/);

  await page.getByRole("link", { name: "虚拟机模板", exact: true }).click();
  await page.getByTestId("template-delete-e2e-image-template").click();
  const templateDialog = page.getByRole("alertdialog");
  await templateDialog.getByLabel("输入模板名称确认").fill("e2e-image-template");
  await templateDialog.getByTestId("confirm-delete-template").click();
  await expect(page.getByTestId("template-row-e2e-image-template")).toHaveCount(0);
});

test("同存储池 qcow2 镜像可创建链接克隆虚拟机", async ({ page }) => {
  await login(page);
  await page.goto("/vms/new");
  await page.getByLabel("名称", { exact: true }).fill("e2e-linked-vm");
  await page.getByTestId("vm-disk-source-image").check();
  await selectOptionContaining(page.getByTestId("vm-image"), "ubuntu-24.04-cloud-base");
  await page.getByLabel("目标存储池", { exact: true }).selectOption("default");
  await expect(page.getByTestId("vm-image-clone-mode").locator("option[value=linked]")).toBeEnabled();
  await page.getByTestId("vm-image-clone-mode").selectOption("linked");
  await expect(page.getByTestId("vm-image-clone-mode")).toHaveValue("linked");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page).toHaveURL(/\/vms\/[0-9a-f-]+$/);
  await expect(page.getByRole("heading", { name: "e2e-linked-vm", exact: true })).toBeVisible();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-disk-source")).toHaveText("/var/lib/libvirt/images/e2e-linked-vm-disk.qcow2");

  await openVMTab(page, "advanced");
  await page.getByTestId("vm-delete-button").click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByLabel("输入虚拟机名称确认").fill("e2e-linked-vm");
  await dialog.getByTestId("confirm-delete-vm").click();
  await expect(page).toHaveURL(/\/vms$/);
});

test("任务取消确认交互在安全检查点生效", async ({ page }) => {
  await page.route("**/api/v1/tasks/task-demo-clone/cancel", async (route) => {
    await route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({
        id: "task-demo-clone", kind: "vm.clone", targetKind: "vm", targetId: "22222222-2222-4222-8222-222222222222",
        status: "cancelled", progress: 68, step: "任务已取消", actor: "admin", cancellable: false,
        createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
      }),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  const cancel = page.getByTestId("task-cancel-task-demo-clone");
  await expect(cancel).toBeEnabled();
  await cancel.click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("取消任务？");
  await expect(dialog).toContainText("下一个安全检查点停止");
  await dialog.getByTestId("confirm-cancel-task").click();
  await expect(page.getByTestId("tasks-notice")).toContainText("已请求取消任务 task-demo-clone");
  await expect(page.getByTestId("tasks-notice")).toContainText("安全检查点");
});

test("可写模式导入 QCOW2 磁盘并保留来源卷", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByTestId("vm-import-link").click();
  await expect(page).toHaveURL(/\/vms\/import$/);
  await expect(page.getByRole("heading", { name: "导入虚拟机", exact: true })).toBeVisible();

  await page.getByLabel("来源存储池", { exact: true }).selectOption("default");
  await page.getByLabel("来源磁盘或 ISO", { exact: true }).selectOption("demo-db-01.qcow2");
  await page.getByLabel("目标存储池", { exact: true }).selectOption("default");
  await page.getByLabel("名称", { exact: true }).fill("e2e-import-01");
  await expect(page.getByLabel("启用 vTPM 2.0", { exact: true })).toBeEnabled();
  await page.getByLabel("启用 vTPM 2.0", { exact: true }).check();
  await expect(page.getByTestId("vm-import-copy-notice")).toContainText("来源卷不会被修改或删除");
  await expect(page.getByText("default/e2e-import-01-disk.qcow2", { exact: true })).toBeVisible();
  await page.getByTestId("vm-import-submit").click();
  await expect(page.getByTestId("vm-import-result")).toContainText("导入任务已创建");

  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  const importTask = page.locator('[data-testid^="task-row-"]').filter({ hasText: "导入虚拟机" }).first();
  await expect(importTask).toContainText("已完成", { timeout: 10_000 });
  await expect(importTask).toContainText("e2e-import-01");

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "e2e-import-01", exact: true }).click();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-disk-source").first()).toContainText("e2e-import-01-disk.qcow2");
  await expect(page.getByTestId("vm-tpm")).toHaveText("2.0 已启用");

  await page.getByRole("link", { name: "存储", exact: true }).click();
  await expect(page.getByText("demo-db-01.qcow2", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("e2e-import-01-disk.qcow2", { exact: true }).first()).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.import");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  const auditRow = page.locator('[data-testid^="audit-row-"]').first();
  await expect(auditRow).toContainText("导入虚拟机");
  await expect(auditRow).toContainText("成功");
  await expect(auditRow).toContainText("e2e-import-01");
});

test("可写模式使用 ISO 安装介质导入并创建空白系统盘", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByTestId("vm-import-link").click();
  await expect(page).toHaveURL(/\/vms\/import$/);

  await page.getByLabel("来源存储池", { exact: true }).selectOption("iso-store");
  await page.getByLabel("来源磁盘或 ISO", { exact: true }).selectOption("debian-13-netinst.iso");
  await page.getByLabel("目标存储池", { exact: true }).selectOption("default");
  await page.getByLabel("名称", { exact: true }).fill("e2e-iso-import-01");
  await expect(page.getByTestId("vm-import-iso-disk-capacity")).toBeVisible();
  await page.getByLabel("系统盘容量 (GiB)", { exact: true }).fill("24");
  await expect(page.getByTestId("vm-import-copy-notice")).toContainText("新建空白系统盘");
  await expect(page.getByText("default/e2e-iso-import-01-disk.qcow2", { exact: true })).toBeVisible();
  await page.getByTestId("vm-import-submit").click();
  await expect(page.getByTestId("vm-import-result")).toContainText("导入任务已创建");

  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  const importTask = page.locator('[data-testid^="task-row-"]').filter({ hasText: "导入虚拟机" }).first();
  await expect(importTask).toContainText("已完成", { timeout: 10_000 });
  await expect(importTask).toContainText("e2e-iso-import-01");

  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "e2e-iso-import-01", exact: true }).click();
  await openVMTab(page, "config");
  await expect(page.getByTestId("vm-disk-source").first()).toContainText("e2e-iso-import-01-disk.qcow2");
  await expect(page.getByTestId("vm-disk-source").nth(1)).toContainText("debian-13-netinst.iso");
});

test("非管理员也可从顶部进入账号安全", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "用户管理", exact: true }).click();
  await page.getByTestId("user-create-toggle").click();
  await page.getByLabel("用户名", { exact: true }).fill("account-viewer");
  await page.getByLabel("初始密码", { exact: true }).fill("viewer-secret-123");
  await page.getByTestId("user-create-form").getByLabel("角色").selectOption("viewer");
  await page.getByTestId("user-create-submit").click();
  await expect(page.getByTestId("user-action-success")).toContainText("account-viewer");

  await page.getByRole("button", { name: "退出登录", exact: true }).click();
  await loginAs(page, "account-viewer", "viewer-secret-123");
  await expect(page.getByRole("link", { name: "用户管理", exact: true })).toHaveCount(0);
  await page.getByTestId("account-security-link").click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByTestId("change-password-card")).toBeVisible();
  await expect(page.getByRole("heading", { name: "修改我的密码", exact: true })).toBeVisible();

  await page.getByLabel("当前密码", { exact: true }).fill("viewer-secret-123");
  await page.getByLabel("新密码", { exact: true }).fill("viewer-secret-456");
  await page.getByLabel("确认新密码", { exact: true }).fill("viewer-secret-456");
  await page.getByTestId("change-password-submit").click();
  await expect(page.getByText("密码已更新，其他设备上的登录会话已撤销。", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "退出登录", exact: true }).click();
  await loginAs(page, "account-viewer", "viewer-secret-456");
});

test("可写模式将空闲链接克隆展平为独立卷", async ({ page }) => {
  await login(page);
  const created = await page.evaluate(async () => {
    const session = await fetch("/api/v1/session").then((response) => response.json()) as { csrfToken: string };
    const call = async (path: string, method: string, body?: unknown) => {
      const response = await fetch(path, {
        method,
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${await response.text()}`);
      return response.status === 204 ? null : response.json();
    };
    await call("/api/v1/storage/pools/default/volumes", "POST", {
      name: "e2e-flatten-base.qcow2", format: "qcow2", capacityBytes: 8 * 1024 ** 3,
    });
    const image = await call("/api/v1/images", "POST", {
      name: "e2e-flatten-image", description: "flatten e2e", pool: "default", volume: "e2e-flatten-base.qcow2",
      osFamily: "linux", osName: "E2E", architecture: "x86_64",
    }) as { id: number };
    const vm = await call("/api/v1/vms", "POST", {
      name: "e2e-flatten-vm", vcpus: 2, memoryMiB: 2048, imageId: image.id, imageCloneMode: "linked",
      targetPool: "default", network: "default", firmware: "bios", secureBoot: false, guestAgent: true,
      autostart: false, start: false,
    }) as { uuid: string };
    await call(`/api/v1/vms/${vm.uuid}`, "DELETE");
    return { imageID: image.id };
  });

  await page.getByRole("link", { name: "存储", exact: true }).click();
  const row = page.getByTestId("storage-volume-row-e2e-flatten-vm-disk.qcow2");
  await expect(row).toContainText("链接克隆");
  const flatten = page.getByTestId("flatten-volume-e2e-flatten-vm-disk.qcow2");
  await expect(flatten).toBeEnabled();
  await flatten.click();
  const modal = page.getByTestId("storage-flatten-modal");
  await expect(modal).toBeVisible();
  await expect(page.getByTestId("storage-flatten-name")).toHaveValue("e2e-flatten-vm-disk-flat.qcow2");
  await page.getByTestId("storage-flatten-submit").click();
  const task = page.getByTestId("storage-flatten-task");
  await expect(task).toHaveAttribute("data-status", "succeeded", { timeout: 10_000 });
  await expect(page.getByTestId("storage-volume-row-e2e-flatten-vm-disk-flat.qcow2")).toBeVisible();
  await expect(page.getByTestId("storage-volume-row-e2e-flatten-vm-disk-flat.qcow2")).not.toContainText("链接克隆");

  await page.evaluate(async ({ imageID }) => {
    const session = await fetch("/api/v1/session").then((response) => response.json()) as { csrfToken: string };
    await fetch(`/api/v1/images/${imageID}`, {
      method: "DELETE", credentials: "same-origin", headers: { "X-CSRF-Token": session.csrfToken },
    });
  }, created);
});

test("可写模式跨池迁移关机虚拟机磁盘并切换持久引用", async ({ page }) => {
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await expect(page.getByTestId("vm-detail-name")).toHaveText("demo-legacy-01");
  await openVMTab(page, "config");

  const migrate = page.getByTestId("vm-disk-migrate-vda");
  await expect(migrate).toBeEnabled();
  await migrate.click();
  const modal = page.getByTestId("vm-disk-migrate-modal");
  await expect(modal).toBeVisible();
  await expect(page.getByTestId("vm-disk-migrate-pool")).toContainText("iso-store");
  await page.getByTestId("vm-disk-migrate-pool").selectOption("iso-store");
  await page.getByTestId("vm-disk-migrate-format").selectOption("raw");
  await page.getByTestId("vm-disk-migrate-name").fill("e2e-migrated-vda.raw");
  await page.getByTestId("vm-disk-migrate-submit").click();
  await expect(page.getByTestId("vm-disk-resize-notice")).toContainText("磁盘已迁移并切换虚拟机引用；旧卷保留在源存储池。", { timeout: 20_000 });
  await expect(page.getByTestId("vm-disk-source").first()).toContainText("/var/lib/libvirt/iso/e2e-migrated-vda.raw");

  await page.getByRole("link", { name: "存储", exact: true }).click();
  await page.getByRole("button", { name: "iso-store", exact: true }).click();
  await expect(page.getByRole("heading", { name: "存储卷 / iso-store", exact: true })).toBeVisible();
  await expect(page.getByTestId("storage-volume-row-e2e-migrated-vda.raw")).toBeVisible();

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.disk.migrate");
  const row = page.locator('[data-testid^="audit-row-"]').first();
  await expect(row).toContainText("跨池迁移虚拟机磁盘");
  await expect(row).toContainText("成功");
  await expect(row).toContainText("demo-legacy-01");
});
