import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("Cloud-init 编辑器在移动端保持可读且不产生横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/templates");
  await page.getByTestId("template-use-secure-uefi").click();

  const editor = page.getByTestId("vm-cloud-init-editor");
  await expect(editor).toBeVisible();

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    await expect(page.getByRole("tab", { name: "可视化", exact: true })).toBeVisible();

    const layout = await editor.evaluate((element) => {
      const header = element.firstElementChild as HTMLElement;
      const summary = header.firstElementChild as HTMLElement;
      const tabs = header.lastElementChild as HTMLElement;
      const summaryBox = summary.getBoundingClientRect();
      const tabsBox = tabs.getBoundingClientRect();
      const body = document.body;
      return {
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        bodyWidth: body.scrollWidth,
        summaryWidth: summaryBox.width,
        tabsWidth: tabsBox.width,
        stacked: tabsBox.top >= summaryBox.bottom - 1,
      };
    });

    expect(layout.documentWidth, `${width}px: 页面不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px: body 不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.summaryWidth, `${width}px: 标题和说明应保留可读宽度`).toBeGreaterThan(200);
    expect(layout.tabsWidth, `${width}px: 模式切换器应占满内容宽度`).toBeCloseTo(layout.summaryWidth, 0);
    expect(layout.stacked, `${width}px: 模式切换器应位于标题区下方`).toBe(true);
  }

  await expect(page.getByTestId("vm-cloud-init-preview")).toBeVisible();
  await expect(page.getByTestId("vm-cloud-init-mode-source")).toBeDisabled();
});
