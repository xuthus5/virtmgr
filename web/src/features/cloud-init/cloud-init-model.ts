import { parseDocument, stringify } from "yaml";

export const maxCloudInitBytes = 64 << 10;

export type CloudInitDraft = {
  packageUpdate: boolean;
  packageUpgrade: boolean;
  packageRebootIfRequired: boolean;
  timezone: string;
  locale: string;
  packages: string;
  runcmd: string;
  createUser: boolean;
  username: string;
  groups: string;
  shell: string;
  sshAuthorizedKeys: string;
};

export type CloudInitParseResult = {
  cloudConfig: boolean;
  syntaxValid: boolean;
  visualizable: boolean;
  error: string;
  unsupported: string[];
  draft: CloudInitDraft;
};

export const emptyCloudInitDraft: CloudInitDraft = {
  packageUpdate: false,
  packageUpgrade: false,
  packageRebootIfRequired: false,
  timezone: "",
  locale: "",
  packages: "",
  runcmd: "",
  createUser: false,
  username: "",
  groups: "sudo",
  shell: "/bin/bash",
  sshAuthorizedKeys: "",
};

export function defaultCloudInitDraft(): CloudInitDraft {
  return { ...emptyCloudInitDraft, packageUpdate: true };
}

export function defaultCloudInitUserData(): string {
  return serializeCloudInit(defaultCloudInitDraft());
}

function lines(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function commaValues(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function packagesFrom(value: string): string[] {
  return value
    .split(/[\r\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function serializeCloudInit(draft: CloudInitDraft): string {
  const config: Record<string, unknown> = {};
  if (draft.packageUpdate) config.package_update = true;
  if (draft.packageUpgrade) config.package_upgrade = true;
  if (draft.packageRebootIfRequired) config.package_reboot_if_required = true;
  if (draft.timezone.trim()) config.timezone = draft.timezone.trim();
  if (draft.locale.trim()) config.locale = draft.locale.trim();
  const packages = packagesFrom(draft.packages);
  if (packages.length) config.packages = packages;

  if (draft.createUser && draft.username.trim()) {
    const user: Record<string, unknown> = {
      name: draft.username.trim(),
      lock_passwd: true,
    };
    const groups = commaValues(draft.groups);
    if (groups.length) user.groups = groups;
    if (draft.shell.trim()) user.shell = draft.shell.trim();
    user.sudo = "ALL=(ALL) NOPASSWD:ALL";
    const keys = lines(draft.sshAuthorizedKeys);
    if (keys.length) user.ssh_authorized_keys = keys;
    config.users = [user];
  }

  const runcmd = lines(draft.runcmd);
  if (runcmd.length) config.runcmd = runcmd;
  if (!Object.keys(config).length) return "#cloud-config\n";
  return `#cloud-config\n${stringify(config, { lineWidth: 0 })}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function stringList(value: unknown, unsupported: string[], name: string): string[] {
  if (!Array.isArray(value)) {
    if (value !== undefined) unsupported.push(name);
    return [];
  }
  const values: string[] = [];
  for (const item of value) {
    if (typeof item === "string") values.push(item);
    else unsupported.push(name);
  }
  return values;
}

const knownTopLevel = new Set([
  "package_update",
  "package_upgrade",
  "package_reboot_if_required",
  "timezone",
  "locale",
  "packages",
  "users",
  "runcmd",
]);

const knownUserFields = new Set([
  "name",
  "groups",
  "shell",
  "sudo",
  "lock_passwd",
  "ssh_authorized_keys",
]);

export function parseCloudInitUserData(value: string): CloudInitParseResult {
  const text = value.trim();
  const result: CloudInitParseResult = {
    cloudConfig: false,
    syntaxValid: true,
    visualizable: false,
    error: "",
    unsupported: [],
    draft: { ...emptyCloudInitDraft },
  };
  if (!text) {
    result.error = "尚未填写 user-data。";
    return result;
  }
  if (!text.startsWith("#cloud-config")) {
    result.error = "当前内容不是 #cloud-config 格式，可使用 YAML 源码模式继续编辑脚本或高级配置。";
    return result;
  }
  result.cloudConfig = true;
  const document = parseDocument(text, { prettyErrors: true });
  if (document.errors.length) {
    result.syntaxValid = false;
    result.error = document.errors[0]?.message ?? "YAML 语法错误。";
    return result;
  }

  let data: unknown;
  try {
    data = document.toJS();
  } catch (cause) {
    result.syntaxValid = false;
    result.error = cause instanceof Error ? cause.message : "无法解析 YAML。";
    return result;
  }
  if (data == null) data = {};
  if (!isRecord(data)) {
    result.error = "cloud-config 顶层必须是 YAML 映射。";
    return result;
  }

  const draft: CloudInitDraft = {
    ...emptyCloudInitDraft,
    packageUpdate: data.package_update === true,
    packageUpgrade: data.package_upgrade === true,
    packageRebootIfRequired: data.package_reboot_if_required === true,
    timezone: stringValue(data.timezone),
    locale: stringValue(data.locale),
    packages: stringList(data.packages, result.unsupported, "packages 中的复杂条目").join("\n"),
    runcmd: stringList(data.runcmd, result.unsupported, "runcmd 中的复杂命令").join("\n"),
  };

  for (const key of Object.keys(data)) {
    if (!knownTopLevel.has(key)) result.unsupported.push(key);
  }

  if (data.users !== undefined) {
    if (!Array.isArray(data.users)) {
      result.unsupported.push("users 不是列表");
    } else if (data.users.length) {
      if (data.users.length > 1) result.unsupported.push("多个用户");
      const user = data.users[0];
      if (isRecord(user)) {
        draft.createUser = true;
        draft.username = stringValue(user.name);
        if (Array.isArray(user.groups)) draft.groups = stringList(user.groups, result.unsupported, "用户 groups 中的复杂条目").join(", ");
        else draft.groups = stringValue(user.groups);
        draft.shell = stringValue(user.shell);
        draft.sshAuthorizedKeys = stringList(user.ssh_authorized_keys, result.unsupported, "SSH 公钥中的复杂条目").join("\n");
        for (const key of Object.keys(user)) {
          if (!knownUserFields.has(key)) result.unsupported.push(`users.${key}`);
        }
      } else {
        result.unsupported.push("users 中的字符串用户");
      }
    }
  }

  result.draft = draft;
  result.visualizable = true;
  return result;
}

export function cloudInitDraftIssues(draft: CloudInitDraft): string[] {
  const issues: string[] = [];
  if (draft.createUser && !draft.username.trim()) issues.push("启用登录用户后必须填写用户名。");
  if (draft.createUser && draft.username.trim() && !/^[a-z_][a-z0-9_-]*$/i.test(draft.username.trim())) {
    issues.push("用户名只能包含字母、数字、下划线和短横线，且不能以数字开头。");
  }
  return issues;
}

export function cloudInitByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function cloudInitValidationError(value: string): string {
  if (!value.trim()) return "启用 Cloud-init 后必须填写用户数据。";
  if (cloudInitByteLength(value) > maxCloudInitBytes) return "Cloud-init 用户数据不能超过 64 KiB。";
  const parsed = parseCloudInitUserData(value);
  if (!parsed.cloudConfig) return "";
  if (!parsed.syntaxValid) return `Cloud-init YAML 语法错误：${parsed.error}`;
  const issues = cloudInitDraftIssues(parsed.draft);
  return issues[0] ?? "";
}
