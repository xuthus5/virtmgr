import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function login(page: Page) {
  await loginAs(page, "admin", "secret");
}

async function loginAs(page: Page, username: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("匿名访问重定向到登录页", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "登录 VirtMgr" })).toBeVisible();
  await expect(page.getByLabel("用户名")).toBeFocused();
  await expect(page).toHaveTitle("登录 · VirtMgr");
});

test("本地管理员登录后展示 libvirt 总览", async ({ page }) => {
  await login(page);
  await expect(page.getByRole("main").getByText("virtmgr-demo", { exact: true })).toBeVisible();
  await expect(page).toHaveTitle("总览 · VirtMgr");
  await expect(page.getByText("只读模式")).toBeVisible();
  await expect(page.getByTestId("metric-total-value")).toHaveText("4");
  await expect(page.getByTestId("metric-running-value")).toHaveText("2");
  await expect(page.getByText("demo-web-01")).toBeVisible();
});

test("错误密码显示可理解的登录错误", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("wrong");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("用户名或密码错误");
  await expect(page).toHaveURL(/\/login$/);
});

test("全局搜索支持快捷键、资源查找和页面跳转", async ({ page }) => {
  await login(page);

  await page.keyboard.press("Control+k");
  await expect(page.getByTestId("global-search-dialog")).toBeVisible();
  await page.getByTestId("global-search-input").fill("demo-db");
  const vmResult = page.getByTestId("global-search-vm-22222222-2222-4222-8222-222222222222");
  await expect(vmResult).toBeVisible();
  await expect(vmResult).toContainText("192.168.122.102");
  await vmResult.click();

  await expect(page).toHaveURL(/\/vms\/22222222-2222-4222-8222-222222222222$/);
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();

  await page.keyboard.press("Control+k");
  await page.getByTestId("global-search-input").fill("物理主机");
  const hostPage = page.getByTestId("global-search-page-host");
  await expect(hostPage).toBeVisible();
  await hostPage.click();

  await expect(page).toHaveURL(/\/host$/);
  await expect(page.getByRole("heading", { name: "物理主机", exact: true })).toBeVisible();

  await page.keyboard.press("Control+k");
  await expect(page.getByTestId("global-search-dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("global-search-dialog")).toHaveCount(0);
});

test("虚拟机列表支持搜索和状态筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page).toHaveURL(/\/vms$/);
  await expect(page.getByRole("heading", { name: "虚拟机", exact: true })).toBeVisible();
  await expect(page.getByTestId("vm-total-label")).toHaveText("共 4 台虚拟机");
  await page.getByLabel("搜索虚拟机").fill("db");
  await expect(page.getByTestId("vm-total-label")).toHaveText("共 1 台虚拟机");
  await expect(page.getByText("demo-db-01")).toBeVisible();
  await expect(page.getByText("192.168.122.102", { exact: true })).toBeVisible();
  await expect(page.getByText("demo-web-01")).toHaveCount(0);
  await page.getByLabel("搜索虚拟机").fill("");
  await page.getByLabel("状态筛选").selectOption("shutoff");
  await expect(page.getByTestId("vm-total-label")).toHaveText("共 1 台虚拟机");
  await expect(page.getByText("demo-legacy-01")).toBeVisible();
});

test("物理主机展示实时资源趋势", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "物理主机", exact: true }).click();
  await expect(page).toHaveURL(/\/host$/);
  await expect(page.getByRole("heading", { name: "物理主机", exact: true })).toBeVisible();
  await expect(page.getByTestId("host-metrics")).toBeVisible();
  await expect(page.getByTestId("host-metrics-memory")).toContainText("64 GiB");
  await expect(page.getByTestId("host-metrics-network")).toContainText("↓");
  await expect.poll(async () => Number(await page.getByTestId("host-metrics-cpu-chart").getAttribute("data-samples")), { timeout: 8_000 }).toBeGreaterThanOrEqual(2);
  await expect(page.getByTestId("host-metrics-cpu")).not.toContainText("0.0%");
});

test("存储页展示存储池和卷", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "存储", exact: true }).click();
  await expect(page).toHaveURL(/\/storage$/);
  await expect(page.getByRole("heading", { name: "存储", exact: true })).toBeVisible();
  await expect(page.getByTestId("storage-pool-total")).toHaveText("3");

  const poolsCard = await page.getByTestId("storage-pools-card").boundingBox();
  const volumesCard = await page.getByTestId("storage-volumes-card").boundingBox();
  expect(poolsCard).not.toBeNull();
  expect(volumesCard).not.toBeNull();
  expect(volumesCard!.y).toBeGreaterThan(poolsCard!.y + poolsCard!.height);

  await expect(page.getByTestId("storage-pool-active")).toHaveText("2");
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 4 个卷");
  await expect(page.getByText("demo-db-01.qcow2", { exact: true })).toBeVisible();
  await expect(page.getByTestId("storage-create-volume")).toBeDisabled();
  await expect(page.getByTestId("storage-upload-iso")).toBeDisabled();
  await expect(page.getByTestId("storage-create-pool")).toBeDisabled();
  await expect(page.getByTestId("storage-pool-start-archive")).toBeDisabled();
  await expect(page.getByTestId("storage-pool-start-archive")).toContainText("启动");
  await expect(page.getByTestId("storage-pool-refresh-default")).toBeDisabled();
  await expect(page.getByTestId("storage-pool-refresh-default")).toContainText("刷新");
  await expect(page.getByTestId("storage-pool-action-hint-default")).toContainText("只读模式");
  await expect(page.getByTestId("delete-volume-demo-build-01.qcow2")).toBeDisabled();

  await page.getByRole("button", { name: "iso-store", exact: true }).click();
  await expect(page.getByTestId("storage-volume-total")).toHaveText("共 2 个卷");
  await expect(page.getByText("debian-13-netinst.iso", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "archive", exact: true }).click();
  await expect(page.getByText("后端服务暂不可用", { exact: true })).toBeVisible();
});

test("网络页展示虚拟网络和 DHCP 配置", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "网络", exact: true }).click();
  await expect(page).toHaveURL(/\/networks$/);
  await expect(page.getByRole("heading", { name: "虚拟网络", exact: true })).toBeVisible();
  await expect(page.getByTestId("network-total")).toHaveText("3");
  await expect(page.getByTestId("network-active")).toHaveText("2");
  await expect(page.getByTestId("network-bridges")).toHaveText("3");
  await expect(page.getByTestId("network-nat")).toHaveText("1");

  const defaultRow = page.getByTestId("network-row-default");
  await expect(defaultRow).toContainText("virbr0");
  await expect(defaultRow).toContainText("192.168.122.1/24");
  await expect(defaultRow).toContainText("DHCP 192.168.122.2 - 192.168.122.254");
  await expect(page.getByTestId("network-row-lab-isolated")).toContainText("未激活");
  await expect(page.getByTestId("network-create-toggle")).toBeDisabled();
  await expect(page.getByTestId("network-start-lab-isolated")).toHaveCount(0);
  await expect(page.getByTestId("network-stop-default")).toHaveCount(0);
  await expect(page.getByTestId("network-delete-lab-isolated")).toHaveCount(0);
  await expect(page.getByTestId("network-edit-lab-isolated")).toHaveCount(0);
  await expect(page.getByTestId("network-action-hint-default")).toContainText("只读模式");
  await expect(page.getByTestId("network-leases-default")).toBeEnabled();
  await expect(page.getByTestId("network-hosts-default")).toBeEnabled();

  await page.getByTestId("network-leases-default").click();
  await expect(page.getByTestId("network-leases-card")).toContainText("DHCP 租约 · default");
  const leaseRows = page.locator('[data-testid^="network-lease-row-"]');
  await expect(leaseRows).toHaveCount(2);
  await expect(leaseRows.first()).toContainText("demo-web-01");
  await expect(leaseRows.first()).toContainText("192.168.122.101/24");
  await expect(leaseRows.first()).toContainText("52:54:00:12:34:56");

  await page.getByTestId("network-leases-lab-isolated").click();
  await expect(page.getByTestId("network-leases-card")).toContainText("DHCP 租约 · lab-isolated");
  await expect(page.getByTestId("network-leases-empty")).toContainText("网络未激活，或未配置 DHCP 地址池");

  await page.getByTestId("network-hosts-default").click();
  await expect(page.getByTestId("network-hosts-card")).toContainText("静态 DHCP 主机 · default");
  await expect(page.getByTestId("network-hosts-card")).toContainText("实时生效");
  await expect(page.getByTestId("network-host-row-52:54:00:12:34:56")).toContainText("demo-web-01");
  await expect(page.getByTestId("network-host-form")).toHaveCount(0);
  await expect(page.getByTestId("network-host-delete-52:54:00:12:34:56")).toBeDisabled();
});

test("物理主机页展示资源、存储池、网络和虚拟机分配", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "物理主机", exact: true }).click();
  await expect(page).toHaveURL(/\/host$/);
  await expect(page.getByRole("heading", { name: "物理主机", exact: true })).toBeVisible();
  await expect(page.getByTestId("host-cpu")).toHaveText("16");
  await expect(page.getByTestId("host-memory")).toHaveText("64 GiB");
  await expect(page.getByTestId("host-pools")).toHaveText("3");
  await expect(page.getByTestId("host-networks")).toHaveText("3");
  await expect(page.getByTestId("host-pool-row-default")).toContainText("active");
  await expect(page.getByTestId("host-pool-row-archive")).toContainText("inactive");
  await expect(page.getByTestId("host-network-row-default")).toContainText("virbr0");
  await expect(page.getByTestId("host-vm-row-demo-db-01")).toContainText("8");
  await expect(page.getByTestId("host-device-total")).toHaveText("6");
  await expect(page.getByTestId("host-device-row-pci_0000_00_02_0")).toContainText("PCI");
  await expect(page.getByTestId("host-device-row-pci_0000_00_02_0")).toContainText("Intel Corporation");
  await expect(page.getByTestId("host-device-row-pci_0000_00_02_0")).toContainText("0000:00:02.0");
  await expect(page.getByTestId("host-device-row-pci_0000_00_02_0")).toContainText("i40e");
  await expect(page.getByTestId("host-device-row-pci_0000_00_02_0")).toContainText("在线");
  await expect(page.getByTestId("host-device-row-usb_device_046d_c52b_1_2")).toContainText("USB 设备");
  await expect(page.getByTestId("host-device-row-usb_device_046d_c52b_1_2")).toContainText("bus 1 · device 2");
  await expect(page.getByTestId("host-device-row-net_enp2s0")).toContainText("enp2s0");
  await expect(page.getByTestId("host-device-row-block_sda")).toContainText("/dev/sda");
  await expect(page.getByTestId("host-warnings")).toHaveCount(0);
});

test("事件页展示实时事件流并支持搜索和级别筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "事件", exact: true }).click();
  await expect(page).toHaveURL(/\/events$/);
  await expect(page.getByRole("heading", { name: "事件", exact: true })).toBeVisible();
  await expect(page.getByTestId("events-live-status")).toHaveText("实时连接");
  await expect(page.getByTestId("events-total")).toHaveText("3");
  const rows = page.locator('[data-testid^="event-row-"]');
  await expect(rows).toHaveCount(3);
  await expect(page.getByText("虚拟机已暂停", { exact: true })).toBeVisible();
  await expect(page.getByTestId("events-warning")).toHaveText("1");

  await page.getByLabel("搜索事件").fill("demo-build-01");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("虚拟机已暂停");
  await expect(rows.first().getByRole("link", { name: "demo-build-01", exact: true })).toHaveAttribute("href", /\/vms\/33333333-3333-4333-8333-333333333333$/);

  await page.getByLabel("搜索事件").fill("");
  await page.getByTestId("events-severity-filter").selectOption("critical");
  await expect(rows).toHaveCount(0);
  await expect(page.getByTestId("events-empty")).toContainText("暂无匹配事件");

  await page.getByTestId("events-severity-filter").selectOption("warning");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("虚拟机已暂停");
});

test("任务中心展示持久化任务并支持状态筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page).toHaveURL(/\/tasks$/);
  await expect(page.getByRole("heading", { name: "任务中心", exact: true })).toBeVisible();
  await expect(page.getByTestId("tasks-total")).toHaveText("共 4 个任务");
  const rows = page.locator('[data-testid^="task-row-"]');
  await expect(rows).toHaveCount(4);

  const cloneRow = page.getByTestId("task-row-task-demo-clone");
  await expect(cloneRow).toContainText("克隆虚拟机");
  await expect(cloneRow).toContainText("执行中");
  await expect(cloneRow).toContainText("68%");
  await expect(cloneRow).toContainText("正在复制 demo-db-01.qcow2");
  await expect(cloneRow.getByRole("link", { name: "22222222-2222-4222-8222-222222222222" })).toHaveAttribute("href", /\/vms\/22222222-2222-4222-8222-222222222222$/);
  await expect(page.getByTestId("task-row-task-demo-iso-upload")).toContainText("已完成");

  await page.getByTestId("task-status-filter").selectOption("failed");
  await expect(page.getByTestId("tasks-total")).toHaveText("共 1 个任务");
  await expect(rows).toHaveCount(1);
  await expect(page.getByTestId("task-row-task-demo-snapshot-revert")).toContainText("失败");
  await expect(page.getByTestId("task-row-task-demo-snapshot-revert")).toContainText("libvirt: requested operation is not supported");

  await page.getByTestId("task-status-filter").selectOption("interrupted");
  await expect(page.getByTestId("tasks-total")).toHaveText("共 1 个任务");
  await expect(page.getByTestId("task-row-task-demo-network-interrupted")).toContainText("已中断");
  await expect(page.getByTestId("task-row-task-demo-network-interrupted")).toContainText("服务重启，任务已中断");
});

test("主题切换持久化到页面状态", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "切换深色主题" }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.getByRole("button", { name: "切换浅色主题" }).click();
  await expect(page.locator("html")).not.toHaveClass(/dark/);
});

test("退出登录清除会话并返回登录页", async ({ page }) => {
  await login(page);
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "登录 VirtMgr" })).toBeVisible();
  await page.goto("/vms");
  await expect(page).toHaveURL(/\/login$/);
});

test("移动端导航可打开并进入虚拟机列表", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await page.getByRole("button", { name: "打开导航" }).click();
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page).toHaveURL(/\/vms$/);
  await expect(page.getByRole("heading", { name: "虚拟机", exact: true })).toBeVisible();
});

test("桌面端侧边栏固定、可折叠并记忆状态", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 600 });
  await login(page);
  await page.goto("/host");
  await expect(page.getByRole("heading", { name: "物理主机", exact: true })).toBeVisible();

  const sidebar = page.getByTestId("app-sidebar");
  const header = page.getByTestId("app-header");
  await expect(sidebar).toHaveAttribute("data-collapsed", "false");
  const expandedBox = await sidebar.boundingBox();
  expect(expandedBox).not.toBeNull();

  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0);
  const scrolledBox = await sidebar.boundingBox();
  expect(scrolledBox).not.toBeNull();
  expect(scrolledBox?.y).toBe(expandedBox?.y);
  await expect(header).toBeVisible();
  expect((await header.boundingBox())?.y).toBe(0);

  await page.getByRole("button", { name: "收起侧边栏" }).click();
  await expect(sidebar).toHaveAttribute("data-collapsed", "true");
  await expect.poll(async () => (await sidebar.boundingBox())?.width ?? 0).toBeLessThan(expandedBox!.width);

  await page.reload();
  await expect(page.getByRole("heading", { name: "物理主机", exact: true })).toBeVisible();
  await expect(page.getByTestId("app-sidebar")).toHaveAttribute("data-collapsed", "true");
  await page.getByRole("button", { name: "展开侧边栏" }).click();
  await expect(page.getByTestId("app-sidebar")).toHaveAttribute("data-collapsed", "false");
});

test("键盘可跳过导航并聚焦主内容", async ({ page }) => {
  await login(page);
  await page.keyboard.press("Tab");
  const skipLink = page.getByRole("link", { name: "跳到主要内容" });
  await expect(skipLink).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.locator("#main-content")).toBeFocused();
});

test("移动导航支持 Escape 关闭并恢复焦点", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  const trigger = page.getByRole("button", { name: "打开导航" });
  await trigger.click();
  await expect(page.getByRole("navigation", { name: "主导航" })).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭导航", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("navigation", { name: "主导航" })).toBeHidden();
  await expect(trigger).toBeFocused();
});

test("移动端虚拟机表格支持横向滚动和首列冻结", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await page.goto("/vms");
  const tableRegion = page.getByTestId("vm-table-scroll");
  await expect(tableRegion).toBeVisible();
  await expect(page.getByText("左右滑动查看完整表格")).toBeVisible();
  const dimensions = await tableRegion.evaluate((element) => ({ scrollWidth: element.scrollWidth, clientWidth: element.clientWidth }));
  expect(dimensions.scrollWidth).toBeGreaterThan(dimensions.clientWidth);
  await tableRegion.focus();
  await expect(tableRegion).toBeFocused();
  const position = await page.getByTestId("vm-name-cell-11111111-1111-4111-8111-111111111111").evaluate((element) => getComputedStyle(element).position);
  expect(position).toBe("sticky");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
});

test("移动端虚拟机详情保持关键操作可用", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();
  await expect(page.getByTestId("vm-console-open")).toBeVisible();
  await expect(page.getByTestId("vm-metrics-memory")).toBeVisible();
  await expect(page.getByRole("heading", { name: "生命周期操作", exact: true })).toBeVisible();
  await expect(page.getByTestId("vm-action-pause")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
});

test("虚拟机列表可进入详情页并展示硬件信息", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await expect(page).toHaveURL(/\/vms\/22222222-2222-4222-8222-222222222222$/);
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();
  await expect(page).toHaveTitle("demo-db-01 · VirtMgr");
  await expect(page.getByTestId("vm-detail-vcpus")).toHaveText("8");
  await expect(page.getByTestId("vm-detail-memory")).toHaveText("16 GiB");
  await expect(page.getByTestId("vm-detail-disks")).toHaveText("1");
  await expect(page.getByTestId("vm-hostname")).toHaveText("demo-db-01");
  await expect(page.getByTestId("vm-ip-addresses")).toHaveText("192.168.122.102");
  await expect(page.getByTestId("vm-agent-status")).toHaveText("已连接");
  await expect(page.getByTestId("vm-firmware")).toHaveText("UEFI");
  await expect(page.getByTestId("vm-secure-boot")).toHaveText("已启用");
  await expect(page.getByText("/var/lib/libvirt/images/demo-db-01.qcow2", { exact: true })).toBeVisible();
  await expect(page.getByText("VNC", { exact: true })).toBeVisible();
  await expect(page.getByText("127.0.0.1:5902", { exact: true })).toBeVisible();
});

test("虚拟机详情展示实时资源趋势", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await expect(page.getByTestId("vm-metrics")).toBeVisible();
  await expect(page.getByTestId("vm-metrics-memory")).toContainText("16 GiB");
  await expect(page.getByTestId("vm-metrics-network")).toContainText("↓");
  await expect.poll(async () => Number(await page.getByTestId("vm-metrics-cpu-chart").getAttribute("data-samples")), { timeout: 8_000 }).toBeGreaterThanOrEqual(2);
  await expect(page.getByTestId("vm-metrics-cpu")).not.toContainText("0.0%");
});

test("虚拟机详情磁盘数据源保持单行且不会纵向撑高表格", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  const source = page.getByTestId("vm-disk-source");
  await expect(source).toBeVisible();
  await expect(source).toHaveText("/var/lib/libvirt/images/demo-db-01.qcow2");
  await expect(source).toHaveAttribute("title", "/var/lib/libvirt/images/demo-db-01.qcow2");

  const desktop = await source.evaluate((element) => {
    const style = getComputedStyle(element);
    const row = element.closest("tr");
    return {
      whiteSpace: style.whiteSpace,
      textOverflow: style.textOverflow,
      width: element.getBoundingClientRect().width,
      rowHeight: row?.getBoundingClientRect().height ?? 0,
    };
  });
  expect(desktop.whiteSpace).toBe("nowrap");
  expect(desktop.textOverflow).toBe("ellipsis");
  expect(desktop.width).toBeGreaterThanOrEqual(320);
  expect(desktop.rowHeight).toBeLessThan(64);

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(source).toBeVisible();
  expect(await source.evaluate((element) => {
    const row = element.closest("tr");
    return row?.getBoundingClientRect().height ?? 0;
  })).toBeLessThan(64);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
});

test("虚拟机详情展示只读域 XML 并支持切换和复制", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"], { origin: "http://127.0.0.1:18080" });
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await expect(page.getByRole("heading", { name: "域 XML", exact: true })).toBeVisible();

  const xml = page.getByTestId("vm-xml-content");
  await expect(xml).toContainText("<domain type='kvm'>");
  await expect(xml).toContainText("<name>demo-db-01</name>");
  await expect(xml).toContainText("<os firmware='efi'>");
  await expect(xml).toContainText("<loader secure='yes'/>");
  await expect(page.getByTestId("vm-xml-format-badge")).toHaveText("运行态");

  await page.getByLabel("XML 类型").selectOption("config");
  await expect(xml).toHaveAttribute("data-format", "config");
  await expect(page.getByTestId("vm-xml-format-badge")).toHaveText("持久配置");
  await expect(xml).toContainText("demo-db-01");
  await expect(page.getByTestId("vm-xml-edit")).toHaveCount(0);

  await page.getByTestId("vm-xml-copy").click();
  await expect(page.getByTestId("vm-xml-copy-notice")).toHaveText("已复制到剪贴板");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toContain("demo-db-01");
});

test("只读模式可查看域 XML 版本历史但禁止回滚", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();

  await page.getByTestId("vm-config-versions-open").click();
  const dialog = page.getByTestId("vm-config-versions-dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("域 XML 版本历史");
  const versionItems = dialog.locator('[data-testid^="vm-config-version-item-"]');
  await expect(versionItems.first()).toBeVisible({ timeout: 15_000 });
  await expect(dialog.getByTestId("vm-config-version-current-xml")).toContainText("demo-db-01");
  await expect(dialog.getByTestId("vm-config-version-xml")).toContainText("demo-db-01");
  await expect(dialog.getByTestId("vm-config-version-rollback")).toBeDisabled();
  await expect(dialog.getByTestId("vm-config-version-rollback")).toHaveAttribute("title", /只读模式/);
  await expect(dialog.getByTestId("vm-config-version-pin")).toHaveCount(0);
  await expect(dialog.getByTestId("vm-config-version-download")).toBeEnabled();
});

test("只读模式禁用虚拟机生命周期操作", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-legacy-01", exact: true }).click();
  await expect(page.getByTestId("vm-read-only-notice")).toContainText("只读模式已开启");
  await expect(page.getByTestId("vm-action-start")).toBeDisabled();
  await expect(page.getByTestId("vm-console-open")).toBeDisabled();
  await expect(page.getByTestId("vm-snapshots-card")).toBeVisible();
  await expect(page.getByTestId("vm-snapshot-create-toggle")).toBeDisabled();
  await expect(page.getByTestId("vm-snapshot-empty")).toContainText("暂无快照");
  await expect(page.getByTestId("vm-autostart-toggle")).toBeDisabled();
  await expect(page.getByTestId("vm-resources-card")).toBeVisible();
  await expect(page.getByTestId("vm-resources-readonly")).toContainText("只读模式");
  await expect(page.getByTestId("vm-resources-save")).toHaveCount(0);
  await expect(page.getByTestId("vm-disks-card")).toBeVisible();
  await expect(page.getByTestId("vm-disk-capacity-vda")).toHaveText("20 GiB");
  await expect(page.getByTestId("vm-disk-resize-vda")).toBeDisabled();
  await expect(page.getByTestId("vm-clone-open")).toBeDisabled();
  await expect(page.getByTestId("vm-backup-card")).toBeVisible();
  await expect(page.getByTestId("vm-backup-open")).toBeDisabled();
  await expect(page.getByTestId("vm-backup-disabled")).toContainText("只读模式");
  await expect(page.getByTestId("vm-agent-status")).toHaveText("未连接");
  await expect(page.getByTestId("vm-ip-addresses")).toHaveText("-");
});

test("只读模式展示 PCI / USB 直通状态但禁用变更", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await expect(page.getByTestId("vm-passthrough-card")).toBeVisible();
  await expect(page.getByTestId("vm-passthrough-total")).toHaveText("0 个设备");
  await expect(page.getByTestId("vm-passthrough-empty")).toContainText("当前虚拟机还没有直通 PCI / USB 设备");
  await expect(page.getByTestId("vm-passthrough-readonly")).toContainText("只读模式");
  await expect(page.getByTestId("vm-passthrough-form")).toHaveCount(0);
  await expect(page.getByTestId("vm-passthrough-select")).toHaveCount(0);
});

test("只读模式展示定时快照策略但禁用变更", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await page.getByRole("link", { name: "demo-db-01", exact: true }).click();
  await expect(page.getByTestId("vm-snapshot-policy-card")).toBeVisible();
  await expect(page.getByTestId("vm-snapshot-policy-total")).toHaveText("0 个策略");
  await expect(page.getByTestId("vm-snapshot-policy-empty")).toContainText("暂无快照策略");
  await expect(page.getByTestId("vm-snapshot-policy-readonly")).toContainText("只读模式");
  await expect(page.getByTestId("vm-snapshot-policy-create-toggle")).toBeDisabled();
  await expect(page.getByTestId("vm-snapshot-policy-form")).toHaveCount(0);
});

test("只读模式禁用新建虚拟机", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机", exact: true }).click();
  await expect(page.getByRole("button", { name: "新建虚拟机", exact: true })).toBeDisabled();
});

test("只读模式展示备份中心并禁用写操作", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "备份中心", exact: true }).click();
  await expect(page).toHaveURL(/\/backups$/);
  await expect(page.getByRole("heading", { name: "备份中心", exact: true })).toBeVisible();
  await expect(page.getByTestId("backup-total")).toHaveText("0");
  await expect(page.getByTestId("backups-empty")).toContainText("还没有备份集");
  await expect(page.getByTestId("backup-create-open")).toBeDisabled();
  await expect(page.getByTestId("backup-create-open")).toHaveAttribute("title", /只读模式/);

  await page.getByTestId("backup-tab-schedules").click();
  await expect(page.getByTestId("backup-schedules-empty")).toContainText("还没有定时计划");
  await expect(page.getByTestId("backup-schedule-create")).toBeDisabled();

  await page.getByTestId("backup-tab-repositories").click();
  await expect(page.getByTestId("backup-repository-create")).toBeDisabled();
  await expect(page.getByTestId("backup-repository-create")).toHaveAttribute("title", /只读模式/);
  const defaultRepository = page.locator('[data-testid^="backup-repository-row-"]').filter({ hasText: "default" });
  await expect(defaultRepository).toBeVisible();
  await expect(defaultRepository).toContainText("默认");
});

test("管理员可查看空的审计日志并筛选", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await expect(page).toHaveURL(/\/audit$/);
  await expect(page.getByRole("heading", { name: "审计日志", exact: true })).toBeVisible();
  await expect(page.getByTestId("audit-total")).toHaveText("共 0 条审计记录");
  await expect(page.getByTestId("audit-empty")).toContainText("没有匹配的审计记录");

  await page.getByTestId("audit-action-filter").selectOption("vm.start");
  await expect(page.getByTestId("audit-total")).toHaveText("共 0 条审计记录");
  await page.getByTestId("audit-result-filter").selectOption("failure");
  await expect(page.getByTestId("audit-empty")).toBeVisible();
});

test("只读模式仍可导出虚拟机配置并记录审计", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByTestId("vm-read-only-notice")).toBeVisible();
  const exportButton = page.getByTestId("vm-export-button");
  await expect(exportButton).toBeEnabled();

  const downloadPromise = page.waitForEvent("download");
  await exportButton.click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("demo-db-01-config-export.json");
  const path = await download.path();
  expect(path).not.toBeNull();
  const exported = JSON.parse(await readFile(path!, "utf8")) as {
    formatVersion: string;
    vm: { name: string; uuid: string };
    configXml: { format: string; xml: string };
    liveXml?: { format: string; xml: string };
    snapshots: { total: number };
    disclaimer: string;
  };
  expect(exported.formatVersion).toBe("virtmgr.vm-export/v1");
  expect(exported.vm).toMatchObject({ name: "demo-db-01", uuid: "22222222-2222-4222-8222-222222222222" });
  expect(exported.configXml.format).toBe("config");
  expect(exported.configXml.xml).toContain("<domain type='kvm'>");
  expect(exported.liveXml?.format).toBe("live");
  expect(exported.snapshots.total).toBe(0);
  expect(exported.disclaimer).not.toBe("");
  await expect(page.getByTestId("vm-export-notice")).toContainText("已导出 demo-db-01");

  await page.getByRole("link", { name: "审计日志", exact: true }).click();
  await page.getByTestId("audit-action-filter").selectOption("vm.export");
  await expect(page.getByTestId("audit-total")).toHaveText("共 1 条审计记录");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("导出虚拟机配置");
  await expect(page.locator('[data-testid^="audit-row-"]').first()).toContainText("成功");
});

test("只读模式禁用用户管理写操作", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "用户管理", exact: true }).click();
  await expect(page).toHaveURL(/\/users$/);
  await expect(page.getByRole("heading", { name: "用户管理", exact: true })).toBeVisible();
  await expect(page.getByTestId("users-total")).toHaveText("1");
  await expect(page.getByTestId("user-create-toggle")).toBeDisabled();
  await expect(page.getByText("系统处于只读模式，账号写操作已禁用。", { exact: true })).toBeVisible();
  await expect(page.getByTestId("user-toggle-admin")).toBeDisabled();
});

test("只读模式展示虚拟机模板并支持预填新建表单", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "虚拟机模板", exact: true }).click();
  await expect(page).toHaveURL(/\/templates$/);
  await expect(page.getByRole("heading", { name: "虚拟机模板", exact: true })).toBeVisible();
  await expect(page.getByTestId("template-total")).toHaveText("2");
  await expect(page.getByTestId("template-row-linux-base")).toBeVisible();
  await expect(page.getByTestId("template-row-secure-uefi")).toContainText("UEFI · Secure Boot");
  await expect(page.getByTestId("template-read-only-notice")).toContainText("只读模式");
  await expect(page.getByTestId("template-create-toggle")).toBeDisabled();
  await expect(page.getByTestId("template-edit-linux-base")).toBeDisabled();
  await expect(page.getByTestId("template-delete-linux-base")).toBeDisabled();

  await page.getByTestId("template-use-linux-base").click();
  await expect(page).toHaveURL(/\/vms\/new\?template=\d+$/);
  await expect(page.getByRole("heading", { name: "新建虚拟机", exact: true })).toBeVisible();
  await expect(page.getByLabel("虚拟机模板", { exact: true })).not.toHaveValue("");
  await expect(page.getByLabel("vCPU", { exact: true })).toHaveValue("2");
  await expect(page.getByLabel("内存 (MiB)", { exact: true })).toHaveValue("2048");
  await expect(page.getByLabel("固件", { exact: true })).toHaveValue("bios");

  await page.getByLabel("虚拟机模板", { exact: true }).selectOption({ label: "secure-uefi · 4 vCPU · 4096 MiB" });
  await expect(page.getByLabel("vCPU", { exact: true })).toHaveValue("4");
  await expect(page.getByLabel("内存 (MiB)", { exact: true })).toHaveValue("4096");
  await expect(page.getByLabel("固件", { exact: true })).toHaveValue("uefi");
  await expect(page.getByLabel("安全启动 (Secure Boot)", { exact: true })).toBeChecked();
  await expect(page.getByLabel("启用 Cloud-init", { exact: true })).toBeChecked();
  await expect(page.getByLabel("Cloud-init 用户数据", { exact: true })).toHaveValue(/#cloud-config/);
  await expect(page.getByRole("button", { name: "创建虚拟机", exact: true })).toBeDisabled();
});
