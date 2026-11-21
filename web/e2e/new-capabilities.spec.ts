import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("新增能力入口在桌面端可发现且只读约束正确", async ({ page }) => {
  await login(page);

  await page.goto("/backups");
  await expect(page.getByTestId("backup-import-open")).toBeVisible();
  await expect(page.getByTestId("backup-import-open")).toBeDisabled();
  await expect(page.getByTestId("backup-import-open")).toHaveAttribute("title", /只读模式/);

  await page.goto("/networks");
  const portForwardButton = page.getByTestId("network-port-forwards-default");
  await expect(portForwardButton).toBeEnabled();
  await portForwardButton.click();
  const portForwardCard = page.getByTestId("network-port-forwards-card");
  await expect(portForwardCard).toBeVisible();
  await expect(portForwardCard).toContainText("入站端口转发 · default");
  await expect(portForwardCard).toContainText("NAT 网络");
  await expect(portForwardCard).toContainText("系统当前处于只读模式");
  await expect(page.getByTestId("network-port-forward-form")).toHaveCount(0);

  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await page.getByTestId("vm-tab-guest").click();
  const packages = page.getByTestId("guest-agent-packages");
  await expect(packages).toBeVisible();
  await packages.getByTestId("guest-agent-packages-toggle").click();
  await expect(packages).toContainText("openssl");
  await expect(packages).toContainText("3.0.13-1ubuntu1.1");
  await expect(packages).toContainText("1 个安全更新");
  await packages.getByTestId("guest-packages-search").fill("qemu-guest-agent");
  await expect(packages).toContainText("qemu-guest-agent");
  await expect(packages).not.toContainText("linux-image-generic");

  await page.goto("/vms/44444444-4444-4444-8444-444444444444");
  await page.getByTestId("vm-tab-config").click();
  const advanced = page.getByTestId("vm-advanced-config-open");
  await expect(advanced).toBeVisible();
  await expect(advanced).toBeDisabled();
  await expect(advanced).toHaveAttribute("title", /只读模式/);
});

test("新增能力在移动端不产生页面级横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/networks");
  await page.getByTestId("network-port-forwards-default").click();
  await expect(page.getByTestId("network-port-forwards-card")).toBeVisible();
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await page.getByTestId("vm-tab-guest").click();
  await page.getByTestId("guest-agent-packages-toggle").click();
  await expect(page.getByTestId("guest-agent-packages")).toContainText("openssl");

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px 新增能力页面不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 新增能力页面 body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
  }
});
