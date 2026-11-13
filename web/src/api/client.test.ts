import { afterEach, describe, expect, it, vi } from "vitest";
import { api, unauthorizedEvent } from "@/api/client";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("api client", () => {
  it("emits an unauthorized event when a protected request loses its session", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      error: { code: "UNAUTHORIZED", message: "用户名或密码错误" },
    }), { status: 401, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const dispatch = vi.spyOn(window, "dispatchEvent");

    await expect(api.overview()).rejects.toMatchObject({ status: 401, code: "UNAUTHORIZED" });

    expect(dispatch).toHaveBeenCalledTimes(1);
    expect((dispatch.mock.calls[0][0] as Event).type).toBe(unauthorizedEvent);
  });

  it("does not emit the global unauthorized event for the session endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      error: { code: "UNAUTHORIZED", message: "用户名或密码错误" },
    }), { status: 401, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const dispatch = vi.spyOn(window, "dispatchEvent");

    await expect(api.session()).rejects.toMatchObject({ status: 401 });

    expect(dispatch).not.toHaveBeenCalled();
  });

  it("stores the session CSRF token and sends it on VM actions", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        uuid: "uuid-a", name: "demo", state: "running", stateReason: 0, persistent: true,
        autostart: false, vcpus: 1, maxVcpus: 1, memoryBytes: 1, maxMemoryBytes: 1, cpuTimeNanos: 0,
        disks: [], interfaces: [], graphics: [], passthrough: [], allowedActions: [],
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    await api.vmAction("uuid-a", "start");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/actions/start");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    expect(init.method).toBe("POST");
  });

  it("encodes audit event filters and pagination", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      items: [], total: 0, page: 2, pageSize: 20,
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.auditEvents({ actor: "admin", action: "vm.start", targetKind: "vm", result: "success", page: 2, pageSize: 20 });

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/audit-events?actor=admin&action=vm.start&targetKind=vm&result=success&page=2&pageSize=20");
  });

  it("encodes audit time and target filters and downloads exports", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20 }), {
        status: 200, headers: { "Content-Type": "application/json" },
      }))
      .mockResolvedValueOnce(new Response("id,action\n1,vm.start\n", {
        status: 200,
        headers: { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="virtmgr-audit-test.csv"' },
      }));
    vi.stubGlobal("fetch", fetchMock);

    const filters = { targetId: "uuid-a", from: "2026-11-01T00:00:00.000Z", to: "2026-11-02T00:00:00.000Z", page: 1, pageSize: 20 };
    await api.auditEvents(filters);
    const result = await api.exportAuditEvents(filters, "csv");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/audit-events?targetId=uuid-a&from=2026-11-01T00%3A00%3A00.000Z&to=2026-11-02T00%3A00%3A00.000Z&page=1&pageSize=20");
    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/audit-events/export?targetId=uuid-a&from=2026-11-01T00%3A00%3A00.000Z&to=2026-11-02T00%3A00%3A00.000Z&format=csv");
    expect(result.filename).toBe("virtmgr-audit-test.csv");
    expect(result.blob.type).toContain("text/csv");
  });

  it("requests VM XML for the selected format", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      uuid: "uuid-a", name: "demo", format: "config", xml: "<domain/>",
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.vmXML("uuid-a", "config");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/vms/uuid-a/xml?format=config");
  });

  it("creates, refreshes and deletes storage pools with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-pool",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        name: "data-pool", uuid: "pool-uuid", type: "dir", state: "inactive", autostart: true,
        persistent: true, targetPath: "/srv/virtmgr/data", capacityBytes: 0, allocationBytes: 0,
        availableBytes: 0, volumeCount: 0,
      }), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const pool = await api.createStoragePool({ name: "data-pool", type: "dir", targetPath: "/srv/virtmgr/data", autostart: true, start: false });
    await api.storagePoolAction("data-pool", "refresh");
    await api.deleteStoragePool("data-pool");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/storage/pools");
    const createInit = fetchMock.mock.calls[1][1] as RequestInit;
    expect(createInit.method).toBe("POST");
    expect(createInit.body).toBe('{"name":"data-pool","type":"dir","targetPath":"/srv/virtmgr/data","autostart":true,"start":false}');
    expect((createInit.headers as Headers).get("X-CSRF-Token")).toBe("csrf-pool");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/storage/pools/data-pool/actions/refresh");
    expect((fetchMock.mock.calls[2][1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/storage/pools/data-pool");
    expect((fetchMock.mock.calls[3][1] as RequestInit).method).toBe("DELETE");
    expect(pool.state).toBe("inactive");
  });

  it("exports and imports static DHCP hosts as CSV", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-network",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response("mac,ip,name\n52:54:00:12:34:56,192.168.122.50,web-01\n", {
        status: 200,
        headers: { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="virtmgr-default-dhcp-hosts.csv"' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        network: { name: "default", active: true, persistent: true, autostart: false, addresses: [] },
        imported: 1,
        skipped: 0,
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const exported = await api.exportNetworkDHCPHosts("default");
    const imported = await api.importNetworkDHCPHosts("default", "mac,ip,name\n52:54:00:AB:CD:EF,192.168.122.60,db-01\n");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/networks/default/hosts/export");
    expect(exported.filename).toBe("virtmgr-default-dhcp-hosts.csv");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/networks/default/hosts/import");
    const init = fetchMock.mock.calls[2][1] as RequestInit;
    expect(init.method).toBe("POST");
    expect((init.headers as Headers).get("Content-Type")).toBe("text/csv; charset=utf-8");
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-network");
    expect(imported.imported).toBe(1);
  });

  it("creates an asynchronous VM clone task with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: "task-clone", kind: "vm.clone", targetKind: "vm", targetId: "uuid-a", status: "running",
        progress: 0, step: "准备克隆", actor: "admin", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const task = await api.cloneVM("uuid-a", { name: "api-clone", autostart: true, start: false });

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/clone");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(init.body).toBe('{"name":"api-clone","autostart":true,"start":false}');
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    expect(task.id).toBe("task-clone");
    expect(task.kind).toBe("vm.clone");
  });

  it("creates an asynchronous VM backup task with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: "task-backup", kind: "vm.backup", targetKind: "vm", targetId: "uuid-a", status: "running",
        progress: 0, step: "准备备份", actor: "admin", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const task = await api.backupVM("uuid-a", { name: "api-backup", targetPool: "default" });

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/backups");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(init.body).toBe('{"name":"api-backup","targetPool":"default"}');
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    expect(task.id).toBe("task-backup");
    expect(task.kind).toBe("vm.backup");
  });

  it("uploads ISO bytes with CSRF and an encoded file name", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        name: "install image.iso", path: "/var/lib/libvirt/iso/install image.iso", type: "file", format: "raw",
        capacityBytes: 1048576, allocationBytes: 1048576,
      }), { status: 201, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const file = new File([new Uint8Array(1024 * 1024)], "install image.iso", { type: "application/octet-stream" });

    await api.login("admin", "secret");
    const volume = await api.uploadStorageVolume("iso-store", file);

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/storage/pools/iso-store/volumes/upload?name=install%20image.iso");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(init.body).toBe(file);
    expect((init.headers as Headers).get("Content-Type")).toBe("application/octet-stream");
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    expect(volume.name).toBe("install image.iso");
  });

  it("lists, creates, reverts and deletes VM snapshots with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [{ name: "baseline", createdAt: 100, state: "shutoff", current: true }], total: 1 }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ name: "before-upgrade", createdAt: 200, state: "shutoff", current: true }), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const snapshots = await api.vmSnapshots("uuid-a");
    const created = await api.createVMSnapshot("uuid-a", { name: "before-upgrade", description: "stable" });
    await api.revertVMSnapshot("uuid-a", "before-upgrade");
    await api.deleteVMSnapshot("uuid-a", "before-upgrade");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/snapshots");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/vms/uuid-a/snapshots");
    expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/vms/uuid-a/snapshots/before-upgrade/revert");
    expect(fetchMock.mock.calls[4][0]).toBe("/api/v1/vms/uuid-a/snapshots/before-upgrade");
    expect(snapshots.items[0].name).toBe("baseline");
    expect(created.name).toBe("before-upgrade");
    for (const index of [2, 3, 4]) {
      const init = fetchMock.mock.calls[index][1] as RequestInit;
      expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    }
    const createInit = fetchMock.mock.calls[2][1] as RequestInit;
    expect(createInit.body).toBe('{"name":"before-upgrade","description":"stable"}');
    expect((fetchMock.mock.calls[4][1] as RequestInit).method).toBe("DELETE");
  });

  it("manages scheduled snapshot policies with CSRF", async () => {
    const policy = {
      id: 7,
      vmUuid: "uuid-a",
      name: "nightly",
      intervalMinutes: 60,
      retention: 3,
      enabled: true,
      nextRunAt: "2026-05-01T11:00:00Z",
      createdAt: "2026-05-01T10:00:00Z",
      updatedAt: "2026-05-01T10:00:00Z",
    };
    const updated = { ...policy, intervalMinutes: 120, retention: 5, enabled: false };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [policy], total: 1 }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify(policy), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify(updated), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        policy: { ...policy, lastSnapshot: "nightly-20260501T100000" },
        snapshot: { name: "nightly-20260501T100000", createdAt: 100, state: "shutoff", current: true },
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const list = await api.snapshotPolicies("uuid-a");
    const created = await api.createSnapshotPolicy("uuid-a", { name: "nightly", intervalMinutes: 60, scheduleMode: "interval", dailyTime: "02:00", retention: 3, enabled: true });
    const changed = await api.updateSnapshotPolicy(7, { name: "nightly", intervalMinutes: 120, scheduleMode: "interval", dailyTime: "02:00", retention: 5, enabled: false });
    const run = await api.runSnapshotPolicy(7);
    await api.deleteSnapshotPolicy(7);

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/snapshot-policies");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/vms/uuid-a/snapshot-policies");
    expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/snapshot-policies/7");
    expect(fetchMock.mock.calls[4][0]).toBe("/api/v1/snapshot-policies/7/run");
    expect(fetchMock.mock.calls[5][0]).toBe("/api/v1/snapshot-policies/7");
    expect(list.total).toBe(1);
    expect(created.name).toBe("nightly");
    expect(changed.enabled).toBe(false);
    expect(run.snapshot?.name).toBe("nightly-20260501T100000");
    for (const index of [2, 3, 4, 5]) {
      const init = fetchMock.mock.calls[index][1] as RequestInit;
      expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    }
    expect((fetchMock.mock.calls[3][1] as RequestInit).method).toBe("PATCH");
    expect((fetchMock.mock.calls[3][1] as RequestInit).body).toBe('{"name":"nightly","intervalMinutes":120,"scheduleMode":"interval","dailyTime":"02:00","retention":5,"enabled":false}');
    expect((fetchMock.mock.calls[5][1] as RequestInit).method).toBe("DELETE");
  });

  it("uses PATCH when updating a user", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      id: 2, username: "viewer", role: "operator", disabled: true, createdAt: "2026-01-01T00:00:00Z",
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.updateUser(2, { role: "operator", disabled: true });

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/users/2");
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe('{"role":"operator","disabled":true}');
  });

  it("sends network mutations with CSRF and encoded names", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-token",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ name: "lab/net", active: false, persistent: true, autostart: false, addresses: [] }), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    await api.createNetwork({ name: "lab/net", mode: "isolated", subnet: "10.99.0.0/24", autostart: false, start: false });
    await api.networkAction("lab/net", "stop");
    await api.deleteNetwork("lab/net");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/networks");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/networks/lab%2Fnet/actions/stop");
    expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/networks/lab%2Fnet");
    for (const index of [1, 2, 3]) {
      const init = fetchMock.mock.calls[index][1] as RequestInit;
      expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-token");
    }
  });

  it("loads DHCP leases for a network", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      network: "default",
      items: [{ family: "ipv4", mac: "52:54:00:12:34:56", ipAddress: "192.168.122.101", prefix: 24, hostname: "demo-web-01" }],
      total: 1,
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    const leases = await api.networkLeases("default");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/networks/default/leases");
    expect((fetchMock.mock.calls[0][1] as RequestInit | undefined)?.method).toBeUndefined();
    expect(leases.total).toBe(1);
    expect(leases.items[0].hostname).toBe("demo-web-01");
  });

  it("adds and deletes static DHCP hosts with CSRF and encoded names", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-host",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ name: "default", active: true, persistent: true, autostart: true, addresses: [] }), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ name: "default", active: true, persistent: true, autostart: true, addresses: [] }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    await api.addNetworkDHCPHost("default", { mac: "52:54:00:12:34:56", ip: "192.168.122.50", name: "web-01" });
    await api.deleteNetworkDHCPHost("default", "52:54:00:12:34:56");

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/networks/default/hosts");
    expect((fetchMock.mock.calls[1][1] as RequestInit).body).toBe('{"mac":"52:54:00:12:34:56","ip":"192.168.122.50","name":"web-01"}');
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/networks/default/hosts/52%3A54%3A00%3A12%3A34%3A56");
    for (const index of [1, 2]) {
      const init = fetchMock.mock.calls[index][1] as RequestInit;
      expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-host");
    }
  });

  it("creates, updates and deletes VM templates with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-template",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: 7, name: "linux-base", description: "", vcpus: 2, memoryMiB: 2048,
        diskFormat: "qcow2", network: "default", firmware: "bios", secureBoot: false,
        cloudInit: false, autostart: false, start: false,
        createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
      }), { status: 201, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: 7, name: "secure-web", description: "updated", vcpus: 4, memoryMiB: 4096,
        diskFormat: "qcow2", network: "default", firmware: "uefi", secureBoot: true,
        cloudInit: false, autostart: false, start: false,
        createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const base = { description: "", vcpus: 2, memoryMiB: 2048, diskFormat: "qcow2", network: "default", firmware: "bios" as const, secureBoot: false, cloudInit: false, autostart: false, start: false };
    const created = await api.createTemplate({ ...base, name: "linux-base" });
    await api.updateTemplate(created.id, { ...base, name: "secure-web", description: "updated", vcpus: 4, memoryMiB: 4096, firmware: "uefi", secureBoot: true });
    await api.deleteTemplate(created.id);

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/templates");
    expect((fetchMock.mock.calls[1][1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/templates/7");
    expect((fetchMock.mock.calls[2][1] as RequestInit).method).toBe("PATCH");
    expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/templates/7");
    expect((fetchMock.mock.calls[3][1] as RequestInit).method).toBe("DELETE");
    for (const index of [1, 2, 3]) {
      expect(((fetchMock.mock.calls[index][1] as RequestInit).headers as Headers).get("X-CSRF-Token")).toBe("csrf-template");
    }
  });

  it("updates VM CPU and memory with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-resources",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        uuid: "uuid-a", name: "api-01", state: "running", stateReason: 0, persistent: true,
        autostart: false, vcpus: 4, maxVcpus: 8, memoryBytes: 4294967296, maxMemoryBytes: 8589934592,
        cpuTimeNanos: 0, disks: [], interfaces: [], graphics: [], passthrough: [], allowedActions: [],
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const vm = await api.updateVMResources("uuid-a", { vcpus: 4, memoryMiB: 4096 });

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/resources");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe('{"vcpus":4,"memoryMiB":4096}');
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-resources");
    expect(vm.maxVcpus).toBe(8);
  });

  it("resizes a VM disk with CSRF", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        user: { id: 1, username: "admin", role: "admin", disabled: false, createdAt: "2026-01-01T00:00:00Z" },
        authMode: "local",
        readOnly: false,
        csrfToken: "csrf-disk",
        expiresAt: "2026-01-01T01:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        uuid: "uuid-a", name: "api-01", state: "running", stateReason: 0, persistent: true,
        autostart: false, vcpus: 4, maxVcpus: 8, memoryBytes: 4294967296, maxMemoryBytes: 8589934592,
        cpuTimeNanos: 0, disks: [{ device: "disk", target: "vda", bus: "virtio", format: "qcow2", source: "/disk.qcow2", readOnly: false, capacityBytes: 107374182400, allocationBytes: 42949672960 }],
        interfaces: [], graphics: [], passthrough: [], allowedActions: [],
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.login("admin", "secret");
    const vm = await api.resizeVMDisk("uuid-a", "vda", { capacityBytes: 100 * 1024 ** 3 });

    expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/vms/uuid-a/disks/vda");
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe('{"capacityBytes":107374182400}');
    expect((init.headers as Headers).get("X-CSRF-Token")).toBe("csrf-disk");
    expect(vm.disks[0].capacityBytes).toBe(100 * 1024 ** 3);
  });
});
