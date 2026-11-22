import { expect, test, type Page } from "@playwright/test";

const expectedNavigation = [
  "总览",
  "虚拟机",
  "存储",
  "网络",
  "任务中心",
  "镜像库",
  "虚拟机模板",
  "备份中心",
  "物理主机",
  "事件",
  "审计日志",
  "数据面运维",
  "用户管理",
  "系统设置",
  "关于",
];

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

async function expectNavigationOrder(page: Page) {
  const links = page.getByTestId("app-sidebar").locator("nav a");
  await expect(links).toHaveText(expectedNavigation);
}

test("桌面端导航按高频能力优先排序", async ({ page }) => {
  await login(page);
  await expectNavigationOrder(page);
});

test("移动端抽屉导航保持相同顺序且无横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  await page.getByRole("button", { name: "打开导航" }).click();
  await expectNavigationOrder(page);

  const layout = await page.evaluate(() => ({
    viewportWidth: window.innerWidth,
    documentWidth: document.documentElement.scrollWidth,
    bodyWidth: document.body.scrollWidth,
  }));
  expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth);
  expect(layout.bodyWidth).toBeLessThanOrEqual(layout.viewportWidth);
});
