import { Link, Outlet, createRootRoute, createRoute, createRouter, lazyRouteComponent } from "@tanstack/react-router";
import { LoaderCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AppShell } from "@/components/layout/app-shell";
import { AuditPage } from "@/features/audit/audit-page";
import { BackupsPage } from "@/features/backups/backups-page";
import { LoginPage } from "@/features/auth/login-page";
import { NetworksPage } from "@/features/networks/networks-page";
import { HostPage } from "@/features/host/host-page";
import { EventsPage } from "@/features/events/events-page";
import { OverviewPage } from "@/features/overview/overview-page";
import { StoragePage } from "@/features/storage/storage-page";
import { SettingsPage } from "@/features/settings/settings-page";
import { TasksPage } from "@/features/tasks/tasks-page";
import { TemplatesPage } from "@/features/templates/templates-page";
import { VMCreatePage } from "@/features/vms/vm-create-page";
import { VMsPage } from "@/features/vms/vms-page";
import { UsersPage } from "@/features/users/users-page";

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

const LazyVNCConsolePage = lazyRouteComponent(() => import("@/features/console/vnc-console-page"), "VNCConsolePage");
const LazyVMDetailPage = lazyRouteComponent(() => import("@/features/vms/vm-detail-page"), "VMDetailPage");

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

const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: "/login", component: LoginPage });
const appRoute = createRoute({ getParentRoute: () => rootRoute, id: "app", component: AppShell });
const overviewRoute = createRoute({ getParentRoute: () => appRoute, path: "/", component: OverviewPage });
const vmsRoute = createRoute({ getParentRoute: () => appRoute, path: "/vms", component: VMsPage });
const backupsRoute = createRoute({ getParentRoute: () => appRoute, path: "/backups", component: BackupsPage });
const vmCreateRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/vms/new",
  validateSearch: (search: Record<string, unknown>): { template?: number } => {
    const template = Number(search.template);
    return Number.isInteger(template) && template > 0 ? { template } : {};
  },
  component: VMCreatePage,
});
const templatesRoute = createRoute({ getParentRoute: () => appRoute, path: "/templates", component: TemplatesPage });
const storageRoute = createRoute({ getParentRoute: () => appRoute, path: "/storage", component: StoragePage });
const networksRoute = createRoute({ getParentRoute: () => appRoute, path: "/networks", component: NetworksPage });
const hostRoute = createRoute({ getParentRoute: () => appRoute, path: "/host", component: HostPage });
const tasksRoute = createRoute({ getParentRoute: () => appRoute, path: "/tasks", component: TasksPage });
const eventsRoute = createRoute({ getParentRoute: () => appRoute, path: "/events", component: EventsPage });
const auditRoute = createRoute({ getParentRoute: () => appRoute, path: "/audit", component: AuditPage });
const usersRoute = createRoute({ getParentRoute: () => appRoute, path: "/users", component: UsersPage });
const settingsRoute = createRoute({ getParentRoute: () => appRoute, path: "/settings", component: SettingsPage });
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
  component: function VNCConsoleRouteComponent() {
    const { uuid } = vncConsoleRoute.useParams();
    return <LazyVNCConsolePage uuid={uuid} />;
  },
});

export const routeTree = rootRoute.addChildren([
  loginRoute,
  vncConsoleRoute,
  appRoute.addChildren([overviewRoute, vmsRoute, backupsRoute, vmCreateRoute, vmDetailRoute, templatesRoute, storageRoute, networksRoute, hostRoute, tasksRoute, eventsRoute, auditRoute, usersRoute, settingsRoute]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPendingComponent: RoutePendingComponent,
  defaultPendingMs: 120,
  defaultPendingMinMs: 240,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
