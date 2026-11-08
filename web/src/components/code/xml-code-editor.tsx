import { useEffect, useRef } from "react";
import { basicSetup } from "codemirror";
import { defaultHighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { oneDarkHighlightStyle } from "@codemirror/theme-one-dark";
import { EditorView } from "@codemirror/view";
import { xml } from "@codemirror/lang-xml";
import { useTheme } from "@/lib/theme";

type XMLCodeEditorProps = {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  ariaLabel: string;
  testId: string;
  format?: string;
  className?: string;
};

function editorTheme(dark: boolean) {
  return EditorView.theme({
    "&": {
      height: "100%",
      backgroundColor: "hsl(var(--card))",
      color: "hsl(var(--foreground))",
      fontSize: "12px",
    },
    ".cm-scroller": {
      fontFamily: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
      lineHeight: "1.65",
      overflow: "auto",
    },
    ".cm-content": {
      padding: "16px 0",
      caretColor: "hsl(var(--foreground))",
    },
    ".cm-line": {
      padding: "0 16px 0 8px",
    },
    ".cm-gutters": {
      backgroundColor: "hsl(var(--muted))",
      color: "hsl(var(--muted-foreground))",
      border: "0",
      borderRight: "1px solid hsl(var(--border))",
    },
    ".cm-activeLine": {
      backgroundColor: "hsl(var(--primary) / 0.06)",
    },
    ".cm-activeLineGutter": {
      backgroundColor: "hsl(var(--primary) / 0.1)",
      color: "hsl(var(--foreground))",
    },
    ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, .cm-content ::selection": {
      backgroundColor: "hsl(var(--primary) / 0.2) !important",
    },
    ".cm-cursor, .cm-dropCursor": {
      borderLeftColor: "hsl(var(--foreground))",
    },
    ".cm-foldPlaceholder": {
      backgroundColor: "hsl(var(--secondary))",
      border: "1px solid hsl(var(--border))",
      color: "hsl(var(--muted-foreground))",
    },
    ".cm-tooltip": {
      backgroundColor: "hsl(var(--popover))",
      border: "1px solid hsl(var(--border))",
      color: "hsl(var(--popover-foreground))",
    },
    ".cm-tooltip-autocomplete > ul > li[aria-selected]": {
      backgroundColor: "hsl(var(--accent))",
      color: "hsl(var(--accent-foreground))",
    },
    ".cm-panels": {
      backgroundColor: "hsl(var(--muted))",
      color: "hsl(var(--foreground))",
    },
    ".cm-searchMatch": {
      backgroundColor: "hsl(var(--warning) / 0.22)",
      outline: "1px solid hsl(var(--warning) / 0.55)",
    },
    ".cm-searchMatch.cm-searchMatch-selected": {
      backgroundColor: "hsl(var(--primary) / 0.28)",
    },
  }, { dark });
}

export function XMLCodeEditor({ value, onChange, readOnly = false, ariaLabel, testId, format, className }: XMLCodeEditorProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const { theme } = useTheme();

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    if (!hostRef.current) return;
    const view = new EditorView({
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          xml(),
          EditorView.lineWrapping,
          syntaxHighlighting(theme === "dark" ? oneDarkHighlightStyle : defaultHighlightStyle, { fallback: true }),
          editorTheme(theme === "dark"),
          EditorState.readOnly.of(readOnly),
          EditorView.editable.of(!readOnly),
          EditorView.contentAttributes.of({
            "aria-label": ariaLabel,
            "aria-readonly": String(readOnly),
            "data-testid": testId,
            ...(format ? { "data-format": format } : {}),
            spellcheck: "false",
            autocapitalize: "off",
            autocomplete: "off",
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) onChangeRef.current?.(update.state.doc.toString());
          }),
        ],
      }),
      parent: hostRef.current,
    });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, [ariaLabel, format, readOnly, testId, theme]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (current === value) return;
    view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
  }, [value]);

  return <div ref={hostRef} className={className} />;
}
