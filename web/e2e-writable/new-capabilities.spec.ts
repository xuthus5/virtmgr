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
