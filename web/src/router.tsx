import { Link, Outlet, createRootRoute, createRoute, createRouter, lazyRouteComponent, type ErrorComponentProps } from "@tanstack/react-router";
import { LoaderCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AppShell } from "@/components/layout/app-shell";

function RootComponent() {
  return <Outlet />;
}

function RoutePendingComponent() {
  return (
    <div className="grid min-h-64 place-items-center px-6 py-12" role="status" aria-live="polite">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="h-4 w-4 animate-spin" />
        正在加载页面…
      </div>
    </div>
  );
}

function RouteErrorComponent({ error }: ErrorComponentProps) {
  const message = error instanceof Error && error.message ? error.message : "页面资源加载失败";
  return (
    <div className="grid min-h-64 place-items-center px-6 py-12" role="alert">
      <div className="w-full max-w-lg rounded-xl border border-destructive/30 bg-card p-6 shadow-panel">
        <h1 className="text-lg font-semibold">页面暂时无法加载</h1>
        <p className="mt-2 text-sm text-muted-foreground">可能是页面资源更新或网络中断。重新加载后可获取最新资源。</p>
        <p className="mt-3 break-words rounded-md bg-muted/40 px-3 py-2 font-mono text-xs text-muted-foreground">{message}</p>
        <Button className="mt-5" onClick={() => window.location.reload()}>重新加载</Button>
      </div>
    </div>
  );
}

const LazyVNCConsolePage = lazyRouteComponent(() => import("@/features/console/vnc-console-page"), "VNCConsolePage");
const LazyVMDetailPage = lazyRouteComponent(() => import("@/features/vms/vm-detail-page"), "VMDetailPage");
const LazyLoginPage = lazyRouteComponent(() => import("@/features/auth/login-page"), "LoginPage");
const LazyForgotPasswordPage = lazyRouteComponent(() => import("@/features/auth/forgot-password-page"), "ForgotPasswordPage");
const LazyOverviewPage = lazyRouteComponent(() => import("@/features/overview/overview-page"), "OverviewPage");
const LazyVMsPage = lazyRouteComponent(() => import("@/features/vms/vms-page"), "VMsPage");
const LazyBackupsPage = lazyRouteComponent(() => import("@/features/backups/backups-page"), "BackupsPage");
const LazyVMCreatePage = lazyRouteComponent(() => import("@/features/vms/vm-create-page"), "VMCreatePage");
const LazyVMImportPage = lazyRouteComponent(() => import("@/features/vms/vm-import-page"), "VMImportPage");
const LazyTemplatesPage = lazyRouteComponent(() => import("@/features/templates/templates-page"), "TemplatesPage");
const LazyImagesPage = lazyRouteComponent(() => import("@/features/images/images-page"), "ImagesPage");
const LazyStoragePage = lazyRouteComponent(() => import("@/features/storage/storage-page"), "StoragePage");
const LazyNetworksPage = lazyRouteComponent(() => import("@/features/networks/networks-page"), "NetworksPage");
const LazyHostPage = lazyRouteComponent(() => import("@/features/host/host-page"), "HostPage");
const LazyTasksPage = lazyRouteComponent(() => import("@/features/tasks/tasks-page"), "TasksPage");
const LazyEventsPage = lazyRouteComponent(() => import("@/features/events/events-page"), "EventsPage");
const LazyAuditPage = lazyRouteComponent(() => import("@/features/audit/audit-page"), "AuditPage");
const LazyUsersPage = lazyRouteComponent(() => import("@/features/users/users-page"), "UsersPage");
const LazyAccountSecurityPage = lazyRouteComponent(() => import("@/features/account/account-security-page"), "AccountSecurityPage");
const LazySettingsPage = lazyRouteComponent(() => import("@/features/settings/settings-page"), "SettingsPage");

const rootRoute = createRootRoute({
  component: RootComponent,
  notFoundComponent: () => (
    <div className="grid min-h-screen place-items-center bg-background p-6">
      <div className="w-full max-w-md rounded-xl border bg-card p-8 text-center shadow-panel">
        <p className="text-5xl font-semibold tracking-tight text-primary">404</p>
        <h1 className="mt-4 text-lg font-semibold">页面不存在</h1>
        <p className="mt-2 text-sm text-muted-foreground">请求的页面可能已移动，或地址输入有误。</p>
        <Button asChild className="mt-6"><Link to="/">返回总览</Link></Button>
      </div>
    </div>
  ),
});

const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: "/login", component: LazyLoginPage });
const forgotPasswordRoute = createRoute({ getParentRoute: () => rootRoute, path: "/forgot-password", component: LazyForgotPasswordPage });
const appRoute = createRoute({ getParentRoute: () => rootRoute, id: "app", component: AppShell });
const overviewRoute = createRoute({ getParentRoute: () => appRoute, path: "/", component: LazyOverviewPage });
const vmsRoute = createRoute({ getParentRoute: () => appRoute, path: "/vms", component: LazyVMsPage });
const backupsRoute = createRoute({ getParentRoute: () => appRoute, path: "/backups", component: LazyBackupsPage });
const vmCreateRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/vms/new",
  validateSearch: (search: Record<string, unknown>): { template?: number } => {
    const template = Number(search.template);
    return Number.isInteger(template) && template > 0 ? { template } : {};
  },
  component: LazyVMCreatePage,
});
const vmImportRoute = createRoute({ getParentRoute: () => appRoute, path: "/vms/import", component: LazyVMImportPage });
const templatesRoute = createRoute({ getParentRoute: () => appRoute, path: "/templates", component: LazyTemplatesPage });
const imagesRoute = createRoute({ getParentRoute: () => appRoute, path: "/images", component: LazyImagesPage });
const storageRoute = createRoute({ getParentRoute: () => appRoute, path: "/storage", component: LazyStoragePage });
const networksRoute = createRoute({ getParentRoute: () => appRoute, path: "/networks", component: LazyNetworksPage });
const hostRoute = createRoute({ getParentRoute: () => appRoute, path: "/host", component: LazyHostPage });
const tasksRoute = createRoute({ getParentRoute: () => appRoute, path: "/tasks", component: LazyTasksPage });
const eventsRoute = createRoute({ getParentRoute: () => appRoute, path: "/events", component: LazyEventsPage });
const auditRoute = createRoute({ getParentRoute: () => appRoute, path: "/audit", component: LazyAuditPage });
const usersRoute = createRoute({ getParentRoute: () => appRoute, path: "/users", component: LazyUsersPage });
const accountRoute = createRoute({ getParentRoute: () => appRoute, path: "/account", component: LazyAccountSecurityPage });
const settingsRoute = createRoute({ getParentRoute: () => appRoute, path: "/settings", component: LazySettingsPage });
const vmDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/vms/$uuid",
  component: function VMDetailRouteComponent() {
    const { uuid } = vmDetailRoute.useParams();
    return <LazyVMDetailPage uuid={uuid} />;
  },
});

const vncConsoleRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/console/vms/$uuid",
  validateSearch: (search: Record<string, unknown>): { kind?: "vnc" | "serial"; detached?: boolean } => {
    const kind = search.kind === "serial" ? "serial" : search.kind === "vnc" ? "vnc" : undefined;
    const detached = search.detached === true || search.detached === "1";
    return {
      ...(kind ? { kind } : {}),
      ...(detached ? { detached: true } : {}),
    };
  },
  component: function VNCConsoleRouteComponent() {
    const { uuid } = vncConsoleRoute.useParams();
    const { kind, detached } = vncConsoleRoute.useSearch();
    return <LazyVNCConsolePage uuid={uuid} initialKind={kind} detached={detached} />;
  },
});

export const routeTree = rootRoute.addChildren([
  loginRoute,
  forgotPasswordRoute,
  vncConsoleRoute,
  appRoute.addChildren([overviewRoute, vmsRoute, backupsRoute, vmCreateRoute, vmImportRoute, vmDetailRoute, templatesRoute, imagesRoute, storageRoute, networksRoute, hostRoute, tasksRoute, eventsRoute, auditRoute, usersRoute, accountRoute, settingsRoute]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPendingComponent: RoutePendingComponent,
  defaultErrorComponent: RouteErrorComponent,
  defaultPendingMs: 120,
  defaultPendingMinMs: 240,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
