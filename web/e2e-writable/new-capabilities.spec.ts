import { expect, test, type Locator, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

async function selectOptionContaining(select: Locator, text: string) {
  const option = select.locator("option").filter({ hasText: text }).first();
  const value = await option.getAttribute("value");
  if (!value) throw new Error(`option containing ${text} was not found`);
  await select.selectOption(value);
}

test("可写模式可下载并重新导入备份 ZIP 包", async ({ page }) => {
  await login(page);
  await page.goto("/backups");
  await page.getByTestId("backup-create-open").click();
  const createModal = page.getByTestId("backup-create-modal");
  await expect(createModal).toBeVisible();
  await selectOptionContaining(createModal.getByTestId("backup-create-vm"), "demo-legacy-01");
  await createModal.getByTestId("backup-create-name").fill("e2e-package-backup");
  await selectOptionContaining(createModal.getByTestId("backup-create-repository"), "default");
  await createModal.getByRole("button", { name: "开始备份", exact: true }).click();
  await expect(page.getByTestId("backup-notice")).toContainText("创建备份已完成。", { timeout: 30_000 });

  const sourceRow = page.locator('[data-testid^="backup-row-"]').filter({ hasText: "e2e-package-backup" });
  await expect(sourceRow).toContainText("已完成");
  const downloadPromise = page.waitForEvent("download");
  await sourceRow.getByRole("button", { name: "下载 e2e-package-backup 备份包", exact: true }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("e2e-package-backup.virtmgr-backup.zip");
  const downloadPath = await download.path();
  expect(downloadPath).not.toBeNull();

  await page.getByTestId("backup-import-open").click();
  const importModal = page.getByTestId("backup-import-modal");
  await expect(importModal).toBeVisible();
  await importModal.getByTestId("backup-import-file").setInputFiles(downloadPath!);
  await importModal.getByTestId("backup-import-name").fill("e2e-package-imported");
  await selectOptionContaining(importModal.getByTestId("backup-import-repository"), "default");
  await importModal.getByTestId("backup-import-submit").click();
  await expect(importModal).toBeHidden();
  await expect(page.getByTestId("backup-notice")).toContainText("备份包已导入为“e2e-package-imported”", { timeout: 30_000 });
  const importedRow = page.locator('[data-testid^="backup-row-"]').filter({ hasText: "e2e-package-imported" });
  await expect(importedRow).toContainText("已完成");
  await expect(importedRow).toContainText("未校验");
});

test("可写模式可创建 IPv6 入站端口转发并删除", async ({ page }) => {
  await login(page);
  await page.goto("/networks");
  await page.getByTestId("network-port-forwards-default").click();
  const card = page.getByTestId("network-port-forwards-card");
  await expect(card).toBeVisible();
  await page.getByTestId("port-forward-name").fill("e2e-ipv6-https");
  await page.getByTestId("port-forward-family").selectOption("ipv6");
  await page.getByTestId("port-forward-protocol").selectOption("tcp");
  await page.getByTestId("port-forward-host-port").fill("16443");
  await page.getByTestId("port-forward-guest-address").fill("fd00:122::50");
  await page.getByTestId("port-forward-guest-port").fill("443");
  await page.getByTestId("port-forward-submit").click();
  await expect(card).toContainText("已创建端口转发“e2e-ipv6-https”");
  const row = page.locator('[data-testid^="port-forward-row-"]').filter({ hasText: "e2e-ipv6-https" });
  await expect(row).toContainText("TCP");
  await expect(row).toContainText("IPV6");
  await expect(row).toContainText("[fd00:122::50]:443");

  await row.locator('[data-testid^="port-forward-delete-"]').click();
  await page.getByTestId("port-forward-delete-confirm").click();
  await expect(card).toContainText("已删除端口转发“e2e-ipv6-https”");
  await expect(row).toHaveCount(0);
});

test("可写模式可保存关机虚拟机 vCPU 绑定", async ({ page }) => {
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await page.getByTestId("vm-tab-config").click();
  await page.getByTestId("vm-advanced-config-open").click();
  const modal = page.getByRole("dialog");
  await expect(modal).toBeVisible();
  await modal.getByRole("button", { name: "添加绑定", exact: true }).click();
  await page.getByTestId("vm-placement-pin-vcpu-0").fill("0");
  await page.getByTestId("vm-placement-pin-cpus-0").fill("0");
  await page.getByTestId("vm-placement-save").click();
  await expect(page.getByTestId("vm-resources-notice")).toContainText("性能与拓扑已更新", { timeout: 15_000 });
});

test("可写模式可更新关机虚拟机的 CPU/内存配额与 RNG/watchdog", async ({ page }) => {
  await login(page);
  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await page.getByTestId("vm-tab-config").click();

  await page.getByTestId("vm-advanced-config-open").click();
  const advanced = page.getByTestId("vm-advanced-config-modal");
  await expect(advanced).toBeVisible();
  await page.getByTestId("vm-quota-shares").fill("1024");
  await page.getByTestId("vm-quota-quota").fill("50000");
  await page.getByTestId("vm-quota-period").fill("100000");
  await page.getByTestId("vm-quota-memory-hard").fill("3072");
  await page.getByTestId("vm-quota-memory-soft").fill("2560");
  await page.getByTestId("vm-quota-save").click();
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
  await expect(page.getByTestId("vm-resources-notice")).toContainText("CPU 与内存配额已更新", { timeout: 15_000 });
  await expect(page.getByTestId("vm-resource-quota")).toHaveText("已设置");

  await page.getByTestId("vm-advanced-config-open").click();
  await page.getByLabel("启用 RNG", { exact: true }).check();
  await page.getByLabel("启用 watchdog", { exact: true }).check();
  await page.getByTestId("vm-watchdog-model").selectOption("ib700");
  await page.getByTestId("vm-watchdog-action").selectOption("poweroff");
  await page.getByTestId("vm-hardware-save").click();
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();
  await expect(page.getByTestId("vm-resources-notice")).toContainText("硬件增强已更新：RNG · watchdog ib700/poweroff", { timeout: 15_000 });
  await expect(page.getByTestId("vm-rng")).toHaveText("virtio-rng 已启用");
  await expect(page.getByTestId("vm-watchdog")).toHaveText("ib700 / poweroff");

  const vm = await page.evaluate(async () => {
    const response = await fetch("/api/v1/vms/44444444-4444-4444-8444-444444444444", { credentials: "same-origin" });
    return response.json() as Promise<{
      resourceQuota?: Record<string, number>;
      rng?: boolean;
      watchdog?: { model: string; action: string };
    }>;
  });
  expect(vm.resourceQuota).toMatchObject({
    cpuShares: 1024,
    cpuQuota: 50000,
    cpuPeriod: 100000,
    memoryHardLimitMiB: 3072,
    memorySoftLimitMiB: 2560,
  });
  expect(vm.rng).toBe(true);
  expect(vm.watchdog).toEqual({ model: "ib700", action: "poweroff" });
});

test("可写模式可在创建虚拟机时设置资源配额和 RNG/watchdog", async ({ page }) => {
  await login(page);
  await page.goto("/vms/new");
  await expect(page.getByRole("heading", { name: "新建虚拟机", exact: true })).toBeVisible();
  await page.getByLabel("名称", { exact: true }).fill("e2e-quota-hardware");
  await page.getByLabel("vCPU", { exact: true }).fill("2");
  await page.getByLabel("内存 (MiB)", { exact: true }).fill("2048");
  await page.getByLabel("磁盘路径", { exact: true }).fill("/var/lib/libvirt/images/e2e-quota-hardware.qcow2");
  await expect(page.locator("#vm-network")).not.toHaveValue("");

  await page.getByTestId("vm-create-quota-shares").fill("512");
  await page.getByTestId("vm-create-quota-quota").fill("50000");
  await page.getByTestId("vm-create-quota-memory-hard").fill("3072");
  await page.getByLabel("创建时启用 RNG", { exact: true }).check();
  await page.getByLabel("创建时启用 watchdog", { exact: true }).check();
  await page.getByTestId("vm-create-watchdog-model").selectOption("ib700");
  await page.getByTestId("vm-create-watchdog-action").selectOption("shutdown");
  await page.getByRole("button", { name: "创建虚拟机", exact: true }).click();

  await expect(page.getByTestId("vm-detail-name")).toHaveText("e2e-quota-hardware", { timeout: 20_000 });
  await page.getByTestId("vm-tab-config").click();
  await expect(page.getByTestId("vm-resource-quota")).toHaveText("已设置");
  await expect(page.getByTestId("vm-rng")).toHaveText("virtio-rng 已启用");
  await expect(page.getByTestId("vm-watchdog")).toHaveText("ib700 / shutdown");
});

test("资源配额与硬件增强在移动端无横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);

  for (const path of ["/vms/new", "/vms/import"]) {
    await page.goto(path);
    await expect(page.getByTestId(path === "/vms/new" ? "vm-hardware-quota-card" : "vm-import-hardware-quota-card")).toBeVisible();
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 844 });
      const layout = await page.evaluate(() => ({
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        bodyWidth: document.body.scrollWidth,
      }));
      expect(layout.documentWidth, `${path} ${width}px document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
      expect(layout.bodyWidth, `${path} ${width}px body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    }
  }

  await page.setViewportSize({ width: 320, height: 844 });
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await page.getByTestId("vm-tab-config").click();
  await page.getByTestId("vm-advanced-config-open").click();
  await expect(page.getByTestId("vm-advanced-config-modal")).toBeVisible();
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => {
      const modal = document.querySelector('[data-testid="vm-advanced-config-modal"]') as HTMLElement;
      const quota = document.querySelector('[data-testid="vm-quota-shares"]') as HTMLElement;
      const modalBox = modal.getBoundingClientRect();
      const quotaBox = quota.getBoundingClientRect();
      return {
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        bodyWidth: document.body.scrollWidth,
        modalLeft: modalBox.left,
        modalRight: modalBox.right,
        quotaLeft: quotaBox.left,
        quotaRight: quotaBox.right,
      };
    });
    expect(layout.documentWidth, `${width}px 高级配置 document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 高级配置 body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.modalLeft, `${width}px 高级配置弹窗应位于视口内`).toBeGreaterThanOrEqual(0);
    expect(layout.modalRight, `${width}px 高级配置弹窗应位于视口内`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.quotaLeft, `${width}px 配额输入框应位于视口内`).toBeGreaterThanOrEqual(0);
    expect(layout.quotaRight, `${width}px 配额输入框应位于视口内`).toBeLessThanOrEqual(layout.viewportWidth);
  }
});
