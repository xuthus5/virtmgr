import * as React from "react";
import { cn } from "@/lib/utils";

type TableProps = React.TableHTMLAttributes<HTMLTableElement> & {
  containerProps?: React.HTMLAttributes<HTMLDivElement> & Record<`data-${string}`, string | undefined>;
};

const Table = React.forwardRef<HTMLTableElement, TableProps>(
  ({ className, containerProps, ...props }, ref) => {
    const { className: containerClassName, ...restContainerProps } = containerProps ?? {};
    const containerRef = React.useRef<HTMLDivElement>(null);
    const [scrollable, setScrollable] = React.useState(false);

    React.useEffect(() => {
      const container = containerRef.current;
      if (!container) return;
      const update = () => setScrollable(container.scrollWidth > container.clientWidth + 1);
      update();
      const observer = new ResizeObserver(update);
      observer.observe(container);
      if (container.firstElementChild) observer.observe(container.firstElementChild);
      window.addEventListener("resize", update);
      return () => {
        observer.disconnect();
        window.removeEventListener("resize", update);
      };
    }, []);

    return (
      <div className="min-w-0">
        {scrollable ? <p className="px-3 pb-2 text-right text-[10px] text-muted-foreground lg:hidden" aria-hidden="true">左右滑动查看完整表格</p> : null}
        <div
          {...restContainerProps}
          ref={containerRef}
          data-table-scrollable={scrollable ? "true" : undefined}
          className={cn("w-full overflow-auto", containerClassName)}
        >
          <table ref={ref} className={cn("w-full min-w-[640px] caption-bottom text-sm", className)} {...props} />
        </div>
      </div>
    );
  },
);
Table.displayName = "Table";

const TableHeader = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(
  ({ className, ...props }, ref) => <thead ref={ref} className={cn("[&_tr]:border-b", className)} {...props} />,
);
TableHeader.displayName = "TableHeader";

const TableBody = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(
  ({ className, ...props }, ref) => <tbody ref={ref} className={cn("[&_tr:last-child]:border-0", className)} {...props} />,
);
TableBody.displayName = "TableBody";

const TableRow = React.forwardRef<HTMLTableRowElement, React.HTMLAttributes<HTMLTableRowElement>>(
  ({ className, ...props }, ref) => <tr ref={ref} className={cn("border-b transition-colors hover:bg-muted/35", className)} {...props} />,
);
TableRow.displayName = "TableRow";

const TableHead = React.forwardRef<HTMLTableCellElement, React.ThHTMLAttributes<HTMLTableCellElement>>(
  ({ className, scope = "col", ...props }, ref) => <th ref={ref} scope={scope} className={cn("h-10 px-3 text-left align-middle text-xs font-medium text-muted-foreground", className)} {...props} />,
);
TableHead.displayName = "TableHead";

const TableCell = React.forwardRef<HTMLTableCellElement, React.TdHTMLAttributes<HTMLTableCellElement>>(
  ({ className, ...props }, ref) => <td ref={ref} className={cn("p-3 align-middle", className)} {...props} />,
);
TableCell.displayName = "TableCell";

export { Table, TableHeader, TableBody, TableRow, TableHead, TableCell };
