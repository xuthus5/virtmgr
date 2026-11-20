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

test("Cloud-init IPv6 网络配置在移动端可读且不产生横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/templates");
  await page.getByTestId("template-use-secure-uefi").click();

  const editor = page.getByTestId("vm-cloud-init-editor");
  const source = page.getByTestId("vm-cloud-init-network-source");
  const ipv6Mode = page.getByTestId("vm-cloud-init-network-ipv6-mode");
  await expect(editor).toBeVisible();
  await expect(source).toContainText("IPv6 fd00:122::1/64 · DHCP");
  await expect(ipv6Mode).toHaveValue("dhcpv6");

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => {
      const source = document.querySelector('[data-testid="vm-cloud-init-network-source"]') as HTMLElement;
      const ipv6 = document.querySelector('[data-testid="vm-cloud-init-network-ipv6-mode"]') as HTMLElement;
      const sourceBox = source.getBoundingClientRect();
      const ipv6Box = ipv6.getBoundingClientRect();
      return {
        viewportWidth: window.innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        sourceLeft: sourceBox.left,
        sourceRight: sourceBox.right,
        ipv6Left: ipv6Box.left,
        ipv6Right: ipv6Box.right,
      };
    });
    expect(layout.documentWidth, `${width}px: 页面不应横向溢出`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.sourceLeft, `${width}px: 网络来源卡片应留在视口内`).toBeGreaterThanOrEqual(0);
    expect(layout.sourceRight, `${width}px: 网络来源卡片应留在视口内`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.ipv6Left, `${width}px: IPv6 选择器应留在视口内`).toBeGreaterThanOrEqual(0);
    expect(layout.ipv6Right, `${width}px: IPv6 选择器应留在视口内`).toBeLessThanOrEqual(layout.viewportWidth);
  }
});

test("镜像库、API Token 和任务中心在窄屏无页面级横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  for (const path of ["/images", "/settings", "/tasks", "/storage", "/networks", "/vms/new", "/dataplane"]) {
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

test("IPv6 MTU 与静态 DNS 记录卡片在移动端可读且不产生页面级溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/networks");
  await expect(page.getByTestId("network-row-default")).toContainText("MTU 1500");
  await page.getByTestId("network-hosts-default").click();

  const dnsCard = page.getByTestId("network-dns-hosts-card");
  await expect(dnsCard).toBeVisible();
  await expect(dnsCard).toContainText("demo-db-01.lab.example");
  await expect(page.getByTestId("network-dns-host-delete-fd00:122::50")).toBeDisabled();

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px 网络 DNS 页面 document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 网络 DNS 页面 body overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    const box = await dnsCard.boundingBox();
    expect(box?.width ?? width + 1, `${width}px DNS 卡片应适应视口`).toBeLessThanOrEqual(width);
  }
});

test("移动端账号安全入口可访问且无横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  const accountLink = page.getByTestId("account-security-link");
  await expect(accountLink).toBeVisible();
  await accountLink.click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByRole("heading", { name: "账号安全", exact: true })).toBeVisible();
  for (const width of [320, 390]) {
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

test("虚拟机导入向导在移动端无页面级横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/vms/import");
  await expect(page.getByTestId("vm-import-page")).toBeVisible();
  await expect(page.getByTestId("vm-import-copy-notice")).toContainText("来源卷不会被修改或删除");
  await expect(page.getByLabel("启用 vTPM 2.0", { exact: true })).toBeVisible();
  await expect(page.getByLabel("启用 vTPM 2.0", { exact: true })).toBeDisabled();

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

test("vTPM、磁盘迁移和在线备份控件在移动端保持可读且无横向溢出", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);

  for (const path of ["/vms/new", "/vms/import"]) {
    await page.goto(path);
    await expect(page.getByLabel("启用 vTPM 2.0", { exact: true })).toBeVisible();
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
  const migrate = page.getByTestId("vm-disk-migrate-vda");
  await expect(migrate).toBeVisible();
  await expect(migrate).toBeDisabled();
  await expect(page.getByTestId("vm-network-model-52:54:00:01:00:02")).toBeVisible();
  await expect(page.getByTestId("vm-network-model-52:54:00:01:00:02")).toBeDisabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);

  await page.getByTestId("vm-tab-snapshots").click();
  await expect(page.getByTestId("vm-backup-card")).toContainText("在线备份");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
});

test("虚拟机详情、Guest Agent 状态与 ISO 入口在窄屏保持可读", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  await login(page);
  await page.goto("/vms/22222222-2222-4222-8222-222222222222");
  await expect(page.getByRole("heading", { name: "demo-db-01", exact: true })).toBeVisible();
  await expect(page.getByTestId("vm-agent-status")).toHaveText("QGA 已连接");
  await expect(page.getByTestId("vm-guest-agent-card")).toHaveCount(0);
  await expect(page.getByTestId("vm-iso-attach-open")).toHaveCount(0);

  await page.getByTestId("vm-password-open").click();
  await expect(page.getByTestId("vm-password-dialog")).toBeVisible();
  await expect(page.getByTestId("vm-password-state")).toHaveText("Guest Agent 已连接");
  await expect(page.getByTestId("vm-password-username")).toBeDisabled();
  await expect(page.getByTestId("vm-password-submit")).toBeDisabled();
  const dialog = await page.getByTestId("vm-password-dialog").boundingBox();
  expect(dialog?.width ?? 321, "320px 客户机密码弹窗应适应视口").toBeLessThanOrEqual(320);
  await page.getByRole("button", { name: "关闭对话框", exact: true }).click();

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    await page.getByTestId("vm-tab-overview").click();
    const layout = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(layout.documentWidth, `${width}px 虚拟机详情 document overflow`).toBeLessThanOrEqual(layout.viewportWidth);
    expect(layout.bodyWidth, `${width}px 虚拟机详情 body overflow`).toBeLessThanOrEqual(layout.viewportWidth);

    await page.getByTestId("vm-tab-config").click();
    await expect(page.getByTestId("vm-iso-attach-open")).toBeVisible();
    await expect(page.getByTestId("vm-iso-attach-open")).toBeDisabled();
    const isoButton = await page.getByTestId("vm-iso-attach-open").boundingBox();
    expect(isoButton?.width ?? width + 1, `${width}px ISO 操作按钮应适应视口`).toBeLessThanOrEqual(width);

    await page.getByTestId("vm-tab-guest").click();
    await expect(page.getByTestId("guest-agent-state")).toHaveText("已连接");
    await expect(page.getByTestId("guest-agent-controls")).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
  }
});
