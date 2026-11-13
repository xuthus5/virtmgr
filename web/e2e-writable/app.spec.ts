import { expect, test, type Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("用户名").fill("admin");
  await page.getByLabel("密码").fill("secret");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "总览", exact: true })).toBeVisible();
}

test("管理员可批量导入并导出静态 DHCP 主机", async ({ page }) => {
  await login(page);
  await page.goto("/networks");
  await page.getByTestId("network-hosts-default").click();

  const csv = [
    "mac,ip,name",
    "52:54:00:12:34:56,192.168.122.101,demo-web-01",
    "52:54:00:AB:CD:EF,192.168.122.150,api-web-01",
  ].join("\n");
  await page.getByTestId("network-hosts-import-file-default").setInputFiles({
    name: "dhcp-hosts.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(csv),
  });

  await expect(page.getByText("静态主机导入完成：新增 1 条，跳过 1 条。")).toBeVisible();
  await expect(page.getByTestId("network-host-row-52:54:00:ab:cd:ef")).toContainText("192.168.122.150");

  const downloadPromise = page.waitForEvent("download");
  await page.getByTestId("network-hosts-export-default").click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("virtmgr-default-dhcp-hosts.csv");
});
