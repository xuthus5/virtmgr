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

test("镜像库、API Token 和任务中心在窄屏无页面级横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  for (const path of ["/images", "/settings", "/tasks", "/storage", "/networks", "/vms/new"]) {
    await page.goto(path);
    await expect(page.getByRole("main")).toBeVisible();
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
  await page.goto("/images");
  await expect(page.getByRole("heading", { name: "镜像库", exact: true })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "ubuntu-24.04-cloud-base" })).toBeVisible();
  await page.goto("/networks");
  await expect(page.getByTestId("network-row-br-prod")).toContainText("br0");
  await expect(page.getByTestId("network-leases-br-prod")).toBeDisabled();
});

test("虚拟机导入向导在移动端无页面级横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/vms/import");
  await expect(page.getByTestId("vm-import-page")).toBeVisible();
  await expect(page.getByTestId("vm-import-copy-notice")).toContainText("来源卷不会被修改或删除");

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `/vms/import ${width}px document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `/vms/import ${width}px body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
  }
});

test("虚拟机详情、Guest Agent 状态与 ISO 入口在窄屏保持可读", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();
  await expect(page.getByTestId("guest-agent-state")).toHaveText("已连接");
  await expect(page.getByTestId("vm-iso-attach-open")).toBeVisible();
  await expect(page.getByTestId("vm-iso-attach-open")).toBeDisabled();
  await expect(page.getByTestId("guest-agent-controls")).toHaveCount(0);

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px 虚拟机详情 document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 虚拟机详情 body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    const isoButton = await page.getByTestId("vm-iso-attach-open").boundingBox();
    expect(isoButton?.width ?? width + 1, `${width}px ISO 操作按钮应适应视口`).toBeLessThanOrEqual(width);
  }
});
