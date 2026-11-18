import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("密码弹窗关闭后恢复触发按钮焦点", async ({ page }) => {
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");

  const trigger = page.getByTestId("vm-password-open");
  await trigger.click();
  const dialog = page.getByTestId("vm-password-dialog");
  await expect(dialog).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭对话框", exact: true })).toBeFocused();

  await page.keyboard.press("Shift+Tab");
  expect(await dialog.evaluate((element) => element.contains(document.activeElement))).toBe(true);

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("系统主题变化会同步更新 React 主题状态", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await login(page);
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.getByRole("button", { name: "切换浅色主题" })).toBeVisible();

  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).not.toHaveClass(/dark/);
  await expect(page.getByRole("button", { name: "切换深色主题" })).toBeVisible();
});

test("全局搜索将 Tab 焦点限制在对话框内", async ({ page }) => {
  await login(page);
  await page.keyboard.press("Control+k");
  await expect(page.getByTestId("global-search-dialog")).toBeVisible();

  const input = page.getByTestId("global-search-input");
  const dialog = page.getByTestId("global-search-dialog");
  await expect(input).toBeFocused();

  await page.keyboard.press("Shift+Tab");
  expect(await dialog.evaluate((element) => element.contains(document.activeElement))).toBe(true);

  await dialog.getByRole("button").last().focus();
  await page.keyboard.press("Tab");
  await expect(input).toBeFocused();
});

test("全局搜索在输入关键词前不请求资源接口", async ({ page }) => {
  await login(page);
  const resourcePaths = new Set<string>();
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (["/api/v1/vms", "/api/v1/networks", "/api/v1/storage/pools", "/api/v1/images", "/api/v1/templates", "/api/v1/tasks"].includes(path)) {
      resourcePaths.add(path);
    }
  });

  await page.keyboard.press("Control+k");
  await expect(page.getByTestId("global-search-dialog")).toBeVisible();
  await page.waitForTimeout(300);
  expect([...resourcePaths]).toEqual([]);

  await page.getByTestId("global-search-input").fill("demo-db");
  await expect(page.getByTestId("global-search-vm-22222222-2222-4222-8222-222222222222")).toBeVisible();
  expect(resourcePaths.has("/api/v1/vms")).toBe(true);
});

test("备份中心离开备份集页签后停止列表轮询", async ({ page }) => {
  await login(page);
  await page.goto("/backups");
  await expect(page.getByRole("heading", { name: "备份中心", exact: true })).toBeVisible();
  await page.getByTestId("backup-tab-schedules").click();
  await expect(page.getByTestId("backup-schedules-empty")).toBeVisible();

  let backupRequests = 0;
  page.on("request", (request) => {
    if (request.method() === "GET" && new URL(request.url()).pathname === "/api/v1/backups") backupRequests += 1;
  });
  await page.waitForTimeout(9_000);
  expect(backupRequests).toBe(0);
});

test("路由分块加载失败时提供重新加载入口", async ({ page }) => {
  await login(page);
  await page.route(/\/assets\/vms-page-[^/]+\.js$/, (route) => route.abort("failed"));
  await page.goto("/vms");

  await expect(page.getByRole("heading", { name: "页面暂时无法加载", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "重新加载", exact: true })).toBeVisible();
});
