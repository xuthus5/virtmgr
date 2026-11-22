import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("版本信息只在关于页面展示", async ({ page }) => {
  await login(page);
  await page.goto("/settings");
  await expect(page.getByTestId("settings-page")).toBeVisible();
  await expect(page.getByTestId("settings-system-info")).toHaveCount(0);
  await expect(page.getByTestId("settings-version")).toHaveCount(0);

  await page.goto("/about");
  await expect(page.getByTestId("about-version")).toBeVisible();
  await expect(page.getByText("当前运行版本", { exact: true })).toHaveCount(0);
});

test("当前账号改密表单只在账号安全页面展示", async ({ page }) => {
  await login(page);
  await page.goto("/users");
  await expect(page.getByRole("heading", { name: "用户管理", exact: true })).toBeVisible();
  await expect(page.getByTestId("change-password-card")).toHaveCount(0);

  await page.getByTestId("account-security-link").click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByTestId("change-password-card")).toBeVisible();
});

test("存储和网络页面只保留一条只读提示", async ({ page }) => {
  await login(page);
  await page.goto("/storage");
  await expect(page.getByTestId("storage-manage-disabled-notice")).toHaveCount(1);
  await expect(page.locator('[data-testid^="storage-pool-action-hint-"]')).toHaveCount(0);

  await page.goto("/networks");
  await expect(page.getByTestId("network-manage-disabled-notice")).toHaveCount(1);
  await expect(page.locator('[data-testid^="network-action-hint-"]')).toHaveCount(0);
});

test("虚拟机详情只保留一条只读状态说明", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByTestId("vm-read-only-notice")).toHaveCount(1);
  await expect(page.getByText("系统当前处于只读模式。", { exact: true })).toHaveCount(0);
});
