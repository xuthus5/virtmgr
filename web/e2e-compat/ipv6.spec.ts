import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

async function login(page: Page) {
  await page.goto("/login");
  await expect(page).toHaveURL(/\[::1\]:18082\/login$/);
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("IPv6 literal access renders networks and diagnostics", async ({ page }) => {
  await login(page);
  expect(new URL(page.url()).hostname).toBe("[::1]");

  await page.getByRole("link", { name: "网络", exact: true }).click();
  await expect(page).toHaveURL(/\[::1\]:18082\/networks$/);
  await expect(page.getByTestId("network-row-default")).toContainText("fd00:122::1/64");
  await page.getByTestId("network-diagnostics-default").click();

  const card = page.getByTestId("network-diagnostics-card");
  await expect(card).toContainText("NAT66 已启用");
  await expect(card).toContainText("DHCPv6 运行");
  await expect(card).toContainText("RA 运行");
  await expect(page.getByTestId("network-diagnostic-compatibility")).toContainText("满足受支持基线");

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
  }
});
