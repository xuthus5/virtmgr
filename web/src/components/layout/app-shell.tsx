import { useEffect, useRef, useState } from "react";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Activity, Archive, ChevronsLeft, ChevronsRight, Cpu, HardDrive, LayoutDashboard, LayoutTemplate, ListChecks, LogOut, Menu, Moon, Network as NetworkIcon, ScrollText, Server, Settings2, Sun, Users, X, type LucideIcon } from "lucide-react";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/feedback/states";
import { useSessionQuery } from "@/features/auth/use-session";
import { useOverviewQuery } from "@/features/overview/use-overview-query";
import { GlobalSearch } from "@/features/search/global-search";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const navigation: Array<{ href: string; label: string; icon: LucideIcon; match: (path: string) => boolean; adminOnly?: boolean }> = [
  { href: "/", label: "总览", icon: LayoutDashboard, match: (path) => path === "/" },
  { href: "/vms", label: "虚拟机", icon: Server, match: (path) => path.startsWith("/vms") },
  { href: "/backups", label: "备份中心", icon: Archive, match: (path) => path.startsWith("/backups") },
  { href: "/templates", label: "虚拟机模板", icon: LayoutTemplate, match: (path) => path.startsWith("/templates") },
  { href: "/storage", label: "存储", icon: HardDrive, match: (path) => path.startsWith("/storage") },
  { href: "/networks", label: "网络", icon: NetworkIcon, match: (path) => path.startsWith("/networks") },
  { href: "/host", label: "物理主机", icon: Cpu, match: (path) => path.startsWith("/host") },
  { href: "/tasks", label: "任务中心", icon: ListChecks, match: (path) => path.startsWith("/tasks") },
  { href: "/events", label: "事件", icon: Activity, match: (path) => path.startsWith("/events") },
  { href: "/audit", label: "审计日志", icon: ScrollText, match: (path) => path.startsWith("/audit"), adminOnly: true },
  { href: "/users", label: "用户管理", icon: Users, match: (path) => path.startsWith("/users"), adminOnly: true },
  { href: "/settings", label: "系统设置", icon: Settings2, match: (path) => path.startsWith("/settings"), adminOnly: true },
];

const SIDEBAR_COLLAPSED_KEY = "virtmgr-sidebar-collapsed";

export function AppShell() {
  const session = useSessionQuery();
  const overview = useOverviewQuery(Boolean(session.data));
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const [mobileOpen, setMobileOpen] = useState(false);
  const [isDesktop, setIsDesktop] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const sidebarRef = useRef<HTMLElement>(null);
  const { theme, toggleTheme } = useTheme();

  useEffect(() => {
    if (session.isError) void navigate({ to: "/login", replace: true });
  }, [navigate, session.isError]);

  useEffect(() => {
    const media = window.matchMedia("(min-width: 1024px)");
    const update = () => setIsDesktop(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    setSidebarCollapsed(window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "true");
  }, []);

  useEffect(() => {
    if (!mobileOpen || isDesktop) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeButtonRef.current?.focus();

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        setMobileOpen(false);
        window.setTimeout(() => menuButtonRef.current?.focus(), 0);
        return;
      }
      if (event.key !== "Tab" || !sidebarRef.current) return;
      const focusable = Array.from(sidebarRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )).filter((element) => !element.hasAttribute("hidden"));
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && (document.activeElement === first || !sidebarRef.current.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
    };
  }, [isDesktop, mobileOpen]);

  function closeMobileNavigation(restoreFocus: boolean) {
    setMobileOpen(false);
    if (restoreFocus) window.setTimeout(() => menuButtonRef.current?.focus(), 0);
  }

  function toggleSidebar() {
    setSidebarCollapsed((current) => {
      const next = !current;
      window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(next));
      return next;
    });
  }

  async function logout() {
    try {
      await api.logout();
    } finally {
      queryClient.clear();
      await navigate({ to: "/login", replace: true });
    }
  }

  if (session.isLoading) return <LoadingState label="正在验证会话" className="min-h-screen" />;
  if (!session.data) return <LoadingState label="正在跳转登录" className="min-h-screen" />;

  const hostOnline = !overview.isError;
  const hostName = overview.data?.host.name ?? "libvirt 主机";
  const user = session.data.user;
  const visibleNavigation = navigation.filter((item) => !item.adminOnly || user.role === "admin");
  const drawerHidden = !isDesktop && !mobileOpen;
  const compactSidebar = isDesktop && sidebarCollapsed;

  return (
    <div className="min-h-screen w-full bg-background">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-[60] focus:rounded-md focus:bg-card focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:text-foreground focus:shadow-lg focus:outline-none focus:ring-2 focus:ring-ring"
        onClick={(event) => {
          event.preventDefault();
          const main = document.getElementById("main-content");
          main?.focus();
          main?.scrollIntoView();
        }}
      >跳到主要内容</a>
      <header data-testid="app-header" className="fixed inset-x-0 top-0 z-40 flex h-14 w-full items-center border-b bg-card/95 px-4 backdrop-blur md:px-5">
        <Button ref={menuButtonRef} variant="ghost" size="icon" className="mr-2 lg:hidden" onClick={() => setMobileOpen(true)} aria-label="打开导航" aria-expanded={mobileOpen} aria-controls="app-navigation">
          <Menu className="h-5 w-5" />
        </Button>
        <Link to="/" className="flex shrink-0 items-center gap-2.5">
          <div className="grid h-8 w-8 place-items-center rounded-lg bg-primary text-sm font-bold text-primary-foreground shadow-sm">V</div>
          <div className="hidden sm:block">
            <p className="text-sm font-semibold leading-4">VirtMgr Console</p>
            <p className="text-[10px] uppercase tracking-[0.16em] text-muted-foreground">Libvirt Operations</p>
          </div>
        </Link>
        <Button
          variant="ghost"
          size="icon-sm"
          className="ml-2 hidden lg:inline-flex"
          onClick={toggleSidebar}
          aria-label={compactSidebar ? "展开侧边栏" : "收起侧边栏"}
          aria-expanded={!compactSidebar}
          aria-controls="app-navigation"
          title={compactSidebar ? "展开侧边栏" : "收起侧边栏"}
          data-testid="sidebar-toggle"
        >
          {compactSidebar ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
        </Button>
        <div className="mx-5 hidden h-6 w-px bg-border lg:block" />
        <div className="hidden min-w-0 items-center gap-2 text-sm lg:flex">
          <span className={cn("h-2 w-2 rounded-full", hostOnline ? "bg-success" : "bg-destructive")} aria-hidden="true" />
          <span className="max-w-52 truncate font-medium">{hostName}</span>
          <span className="text-muted-foreground">/ {hostOnline ? "libvirt" : "连接异常"}</span>
        </div>
        <div className="ml-auto flex items-center gap-1 sm:gap-2">
          <GlobalSearch role={user.role} />
          {session.data.readOnly ? <Badge variant="secondary" className="hidden sm:inline-flex">只读模式</Badge> : null}
          <Button variant="ghost" size="icon" onClick={toggleTheme} aria-label={theme === "dark" ? "切换浅色主题" : "切换深色主题"}>
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
          <div className="hidden items-center gap-2 sm:flex">
            <span className="grid h-7 w-7 place-items-center rounded-full bg-primary/10 text-xs font-semibold text-primary">{user.username.slice(0, 1).toUpperCase()}</span>
            <span className="max-w-28 truncate text-sm">{user.username}</span>
          </div>
          <Button variant="ghost" size="sm" onClick={() => void logout()} aria-label="退出登录">
            <LogOut className="h-4 w-4" /><span className="hidden sm:inline">退出登录</span>
          </Button>
        </div>
      </header>

      <div className={cn("min-h-screen w-full pt-14 transition-[padding-left] duration-200", compactSidebar ? "lg:pl-[4.75rem]" : "lg:pl-60")}>
        <aside
          ref={sidebarRef}
          id="app-navigation"
          aria-label="应用导航"
          aria-hidden={drawerHidden || undefined}
          inert={drawerHidden || undefined}
          data-testid="app-sidebar"
          data-collapsed={compactSidebar ? "true" : "false"}
          className={cn(
            "fixed inset-y-0 left-0 z-50 flex w-60 flex-col border-r bg-card transition-[width,transform] duration-200 lg:bottom-0 lg:top-14 lg:z-30 lg:h-auto lg:translate-x-0",
            compactSidebar && "lg:w-[4.75rem]",
            mobileOpen ? "translate-x-0" : "-translate-x-full",
          )}
        >
          <div className="flex h-14 shrink-0 items-center justify-between border-b px-4 lg:hidden">
            <span className="font-semibold">导航</span>
            <Button ref={closeButtonRef} variant="ghost" size="icon" onClick={() => closeMobileNavigation(true)} aria-label="关闭导航"><X className="h-4 w-4" /></Button>
          </div>
          <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3" aria-label="主导航">
            <p className={cn("px-3 pb-2 pt-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground", compactSidebar && "lg:hidden")}>运行与资源</p>
            {visibleNavigation.map((item) => {
              const Icon = item.icon;
              const active = item.match(pathname);
              return (
                <Link
                  key={item.href}
                  to={item.href}
                  aria-current={active ? "page" : undefined}
                  aria-label={compactSidebar ? item.label : undefined}
                  title={compactSidebar ? item.label : undefined}
                  onClick={() => closeMobileNavigation(false)}
                  className={cn(
                    "flex h-9 items-center rounded-md text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    compactSidebar ? "lg:justify-center lg:px-0" : "gap-3 px-3",
                    active ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  <Icon className="h-4 w-4 shrink-0" />
                  {compactSidebar ? <span className="sr-only">{item.label}</span> : <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </nav>
          <div className={cn("shrink-0 border-t", compactSidebar ? "p-2" : "p-3")}>
            <div
              className={cn("rounded-md bg-muted/55 text-xs", compactSidebar ? "flex h-9 items-center justify-center" : "p-3")}
              aria-label={`宿主机连接状态：${hostOnline ? "libvirt 连接正常" : "libvirt 连接异常"}`}
              title={compactSidebar ? (hostOnline ? "libvirt 连接正常" : "libvirt 连接异常") : undefined}
            >
              <div className={cn("flex items-center", compactSidebar ? "justify-center" : "gap-2 font-medium")}>
                <span className={cn("h-2 w-2 shrink-0 rounded-full", hostOnline ? "bg-success" : "bg-destructive")} />
                {compactSidebar ? <span className="sr-only">{hostOnline ? "libvirt 连接正常" : "libvirt 连接异常"}</span> : (hostOnline ? "libvirt 连接正常" : "libvirt 连接异常")}
              </div>
              {compactSidebar ? null : <p className="mt-1 text-muted-foreground">{overview.data ? `libvirt ${overview.data.host.libvirtVersion}` : "正在读取宿主机状态"}</p>}
            </div>
          </div>
        </aside>
        {mobileOpen ? <button type="button" tabIndex={-1} aria-label="关闭导航菜单" className="fixed inset-0 z-40 bg-slate-950/30 lg:hidden" onClick={() => closeMobileNavigation(true)} /> : null}
        <main id="main-content" tabIndex={-1} aria-label="主要内容" className="min-w-0 w-full overflow-x-hidden p-4 outline-none md:p-6">
          <div className="mx-auto min-w-0 max-w-[1600px] animate-fade-in"><Outlet /></div>
        </main>
      </div>
    </div>
  );
}
