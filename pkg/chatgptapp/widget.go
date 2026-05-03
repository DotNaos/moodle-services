package chatgptapp

const widgetURI = "ui://widget/moodle-browser-v5.html"
const resourceMimeType = "text/html;profile=mcp-app"
const widgetDomain = "https://moodle-services.vercel.app"

const widgetHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Moodle</title>
  <script>
    window.tailwind = {
      theme: {
        extend: {
          colors: {
            border: "hsl(var(--border))",
            input: "hsl(var(--input))",
            ring: "hsl(var(--ring))",
            background: "hsl(var(--background))",
            foreground: "hsl(var(--foreground))",
            primary: { DEFAULT: "hsl(var(--primary))", foreground: "hsl(var(--primary-foreground))" },
            secondary: { DEFAULT: "hsl(var(--secondary))", foreground: "hsl(var(--secondary-foreground))" },
            muted: { DEFAULT: "hsl(var(--muted))", foreground: "hsl(var(--muted-foreground))" },
            accent: { DEFAULT: "hsl(var(--accent))", foreground: "hsl(var(--accent-foreground))" },
            destructive: { DEFAULT: "hsl(var(--destructive))", foreground: "hsl(var(--destructive-foreground))" },
            card: { DEFAULT: "hsl(var(--card))", foreground: "hsl(var(--card-foreground))" }
          },
          borderRadius: { lg: "var(--radius)", md: "calc(var(--radius) - 2px)", sm: "calc(var(--radius) - 4px)" }
        }
      }
    };
  </script>
  <script src="https://cdn.tailwindcss.com"></script>
  <style>
    :root {
      --background: 0 0% 100%;
      --foreground: 222.2 84% 4.9%;
      --card: 0 0% 100%;
      --card-foreground: 222.2 84% 4.9%;
      --primary: 222.2 47.4% 11.2%;
      --primary-foreground: 210 40% 98%;
      --secondary: 210 40% 96.1%;
      --secondary-foreground: 222.2 47.4% 11.2%;
      --muted: 210 40% 96.1%;
      --muted-foreground: 215.4 16.3% 46.9%;
      --accent: 210 40% 96.1%;
      --accent-foreground: 222.2 47.4% 11.2%;
      --destructive: 0 84% 60%;
      --destructive-foreground: 210 40% 98%;
      --border: 214.3 31.8% 91.4%;
      --input: 214.3 31.8% 91.4%;
      --ring: 222.2 84% 4.9%;
      --radius: 0.5rem;
      color: hsl(var(--foreground));
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: hsl(var(--background));
    }
    * { box-sizing: border-box; }
    html, body, #root { height: 100%; margin: 0; }
    button, input { font: inherit; }
    .pdf-scrollbar::-webkit-scrollbar { width: 10px; height: 10px; }
    .pdf-scrollbar::-webkit-scrollbar-track { background: transparent; }
    .pdf-scrollbar::-webkit-scrollbar-thumb { background: hsl(var(--border)); border-radius: 999px; border: 2px solid hsl(var(--background)); }
    .selection-box { position: absolute; border: 2px solid hsl(var(--primary)); background: color-mix(in oklab, hsl(var(--primary)) 12%, transparent); pointer-events: none; }
    .pdf-page-shadow { box-shadow: 0 1px 2px rgba(15, 23, 42, .06), 0 18px 48px rgba(15, 23, 42, .10); }
  </style>
</head>
<body>
  <div id="root"></div>
  <script type="module">
    import React, { useCallback, useEffect, useRef, useState } from "https://cdn.jsdelivr.net/npm/react@18.3.1/+esm";
    import { createRoot } from "https://cdn.jsdelivr.net/npm/react-dom@18.3.1/client/+esm";

    const e = React.createElement;
    const root = createRoot(document.getElementById("root"));

    const cn = (...parts) => parts.flat().filter(Boolean).join(" ");
    const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

    function Button({ variant = "default", size = "sm", className = "", children, ...props }) {
      const variants = {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        outline: "border border-input bg-background hover:bg-accent hover:text-accent-foreground"
      };
      const sizes = {
        sm: "h-8 px-3 text-xs",
        icon: "size-8",
        default: "h-9 px-4 text-sm"
      };
      return e("button", {
        ...props,
        className: cn("inline-flex shrink-0 items-center justify-center gap-2 rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50", variants[variant], sizes[size], className)
      }, children);
    }

    function Badge({ children, className = "" }) {
      return e("span", { className: cn("inline-flex items-center rounded-md border border-transparent bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground", className) }, children);
    }

    function Icon({ name }) {
      const icons = {
        minus: e("path", { d: "M5 12h14" }),
        plus: e("path", { d: "M12 5v14M5 12h14" }),
        scan: e(React.Fragment, null, e("path", { d: "M8 3H5a2 2 0 0 0-2 2v3" }), e("path", { d: "M21 8V5a2 2 0 0 0-2-2h-3" }), e("path", { d: "M3 16v3a2 2 0 0 0 2 2h3" }), e("path", { d: "M16 21h3a2 2 0 0 0 2-2v-3" }), e("path", { d: "M7 12h10" })),
        camera: e(React.Fragment, null, e("path", { d: "M14.5 4h-5L8 6H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-3l-1.5-2Z" }), e("circle", { cx: "12", cy: "13", r: "3" })),
        expand: e(React.Fragment, null, e("path", { d: "M15 3h6v6" }), e("path", { d: "m21 3-7 7" }), e("path", { d: "M9 21H3v-6" }), e("path", { d: "m3 21 7-7" })),
        file: e(React.Fragment, null, e("path", { d: "M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" }), e("path", { d: "M14 2v4a2 2 0 0 0 2 2h4" }))
      };
      return e("svg", { "aria-hidden": "true", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: "2", strokeLinecap: "round", strokeLinejoin: "round", className: "size-4" }, icons[name]);
    }

    function Separator() {
      return e("div", { className: "h-5 w-px bg-border" });
    }

    function Kbd({ children }) {
      return e("kbd", { className: "rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground shadow-sm" }, children);
    }

    function ZoomControl({ zoom, onZoomIn, onZoomOut, onReset }) {
      return e("div", { className: "flex items-center rounded-md border border-border bg-background p-1 shadow-sm" },
        e(Button, { variant: "ghost", size: "icon", "aria-label": "Zoom out", title: "Zoom out", onClick: onZoomOut }, e(Icon, { name: "minus" })),
        e("button", {
          className: "h-8 min-w-14 rounded-sm px-2 text-center text-xs font-medium tabular-nums text-foreground hover:bg-accent",
          title: "Reset zoom",
          onClick: onReset
        }, Math.round(zoom * 100) + "%"),
        e(Button, { variant: "ghost", size: "icon", "aria-label": "Zoom in", title: "Zoom in", onClick: onZoomIn }, e(Icon, { name: "plus" }))
      );
    }

    function App() {
      const [toolPayload, setToolPayload] = useState(null);
      const [metadata, setMetadata] = useState({});

      const renderToolPayload = useCallback((payload, fallbackMetadata) => {
        const data = payload?.structuredContent ?? payload;
        const meta = payload?._meta ?? fallbackMetadata ?? window.openai?.toolResponseMetadata ?? {};
        if (data) {
          setToolPayload(data);
          setMetadata(meta);
        }
      }, []);

      useEffect(() => {
        const receiveMessage = (event) => {
          if (event.source !== window.parent) return;
          const message = event.data;
          if (!message || message.jsonrpc !== "2.0") return;
          if (message.method === "ui/notifications/tool-result") renderToolPayload(message.params);
        };
        const receiveGlobals = (event) => {
          const globals = event.detail?.globals || {};
          renderToolPayload(globals.toolOutput, globals.toolResponseMetadata);
        };
        window.addEventListener("message", receiveMessage, { passive: true });
        window.addEventListener("openai:set_globals", receiveGlobals, { passive: true });
        const tryInitialRender = () => renderToolPayload(window.openai?.toolOutput, window.openai?.toolResponseMetadata);
        tryInitialRender();
        const timers = [50, 250, 1000].map((delay) => window.setTimeout(tryInitialRender, delay));
        return () => {
          window.removeEventListener("message", receiveMessage);
          window.removeEventListener("openai:set_globals", receiveGlobals);
          timers.forEach(window.clearTimeout);
        };
      }, [renderToolPayload]);

      return e("main", { className: "grid h-screen min-h-[420px] grid-rows-[auto_1fr] overflow-hidden bg-muted/40 text-foreground" },
        e(Header, { data: toolPayload }),
        e("section", { className: "min-h-0 overflow-hidden" }, e(Content, { data: toolPayload, metadata }))
      );
    }

    function Header({ data }) {
      const title = data?.viewer?.title || data?.document?.title || (Array.isArray(data?.courses) ? "Moodle courses" : "Moodle");
      const subtitle = data?.viewer?.fileType === "pdf" ? "Scrollable PDF viewer" : "Moodle";
      return e("header", { className: "flex h-14 items-center justify-between gap-3 border-b border-border bg-background/95 px-4 backdrop-blur" },
        e("div", { className: "flex min-w-0 items-center gap-3" },
          e("div", { className: "flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-card text-muted-foreground shadow-sm" }, e(Icon, { name: "file" })),
          e("div", { className: "min-w-0" },
            e("h1", { className: "truncate text-sm font-semibold tracking-normal" }, title),
            e("p", { className: "truncate text-xs text-muted-foreground" }, subtitle)
          )
        ),
        e(Badge, { className: "hidden sm:inline-flex" }, "Moodle")
      );
    }

    function Content({ data, metadata }) {
      if (!data) return e(EmptyState, { title: "Waiting for data", description: "Open a Moodle course or PDF from ChatGPT." });
      if (data.viewer?.fileType === "pdf") return e(PDFViewer, { viewer: data.viewer, pdfUrl: metadata?.pdfUrl });
      if (Array.isArray(data.courses)) return e(ListView, { title: "Courses", items: data.courses, renderItem: (course) => [course.fullname || course.shortname || "Course", [course.shortname, course.category].filter(Boolean).join(" · ")] });
      if (Array.isArray(data.materials)) return e(ListView, { title: "Materials", items: data.materials, renderItem: (item) => [item.name || "Material", [item.sectionName, item.fileType || item.type].filter(Boolean).join(" · ")] });
      if (Array.isArray(data.events)) return e(ListView, { title: "Calendar", items: data.events, renderItem: (event) => [event.summary || "Calendar event", formatDate(event.start) + (event.location ? " · " + event.location : "")] });
      if (data.document) return e(DocumentView, { document: data.document });
      return e(DocumentView, { document: { title: "Moodle result", text: JSON.stringify(data, null, 2) } });
    }

    function ListView({ title, items, renderItem }) {
      return e("div", { className: "h-full overflow-auto p-4 pdf-scrollbar" },
        e("div", { className: "mx-auto flex max-w-5xl flex-col gap-3" },
          e("div", { className: "flex items-center justify-between" },
            e("h2", { className: "text-sm font-semibold" }, title),
            e(Badge, null, String(items.length))
          ),
          items.map((item, index) => {
            const [headline, subline] = renderItem(item);
            return e("article", { key: item.id || index, className: "rounded-lg bg-card px-4 py-3 shadow-sm ring-1 ring-border" },
              e("p", { className: "text-sm font-medium" }, headline),
              subline ? e("p", { className: "mt-1 text-xs text-muted-foreground" }, subline) : null
            );
          })
        )
      );
    }

    function DocumentView({ document }) {
      return e("div", { className: "h-full overflow-auto p-4 pdf-scrollbar" },
        e("article", { className: "mx-auto max-w-5xl rounded-lg bg-card p-4 text-sm shadow-sm ring-1 ring-border" },
          e("div", { className: "mb-3 flex items-center justify-between gap-3" },
            e("h2", { className: "min-w-0 truncate text-base font-semibold" }, document.title || "Document"),
            e(Badge, null, document.metadata?.fileType ? document.metadata.fileType.toUpperCase() : "Text")
          ),
          e("pre", { className: "whitespace-pre-wrap break-words font-mono text-sm leading-6" }, document.text || "")
        )
      );
    }

    function EmptyState({ title, description }) {
      return e("div", { className: "flex h-full items-center justify-center p-6" },
        e("div", { className: "text-center" },
          e("p", { className: "text-sm font-medium" }, title),
          e("p", { className: "mt-1 text-sm text-muted-foreground" }, description)
        )
      );
    }

    function PDFViewer({ viewer, pdfUrl }) {
      const containerRef = useRef(null);
      const canvasesRef = useRef(new Map());
      const wrappersRef = useRef(new Map());
      const [pdfjsLib, setPDFJSLib] = useState(null);
      const [pdf, setPDF] = useState(null);
      const [pageCount, setPageCount] = useState(0);
      const [currentPage, setCurrentPage] = useState(Math.max(1, Number(viewer.target?.page || 1)));
      const [zoom, setZoom] = useState(clamp(Number(viewer.target?.zoom || 1), 0.5, 2.75));
      const [viewportWidth, setViewportWidth] = useState(900);
      const [renderedPages, setRenderedPages] = useState(new Set());
      const [selectMode, setSelectMode] = useState(false);
      const [selectionDraft, setSelectionDraft] = useState(null);
      const [selection, setSelection] = useState(null);
      const saveTimer = useRef(0);
      const zoomAnchorRef = useRef(null);
      const appliedTargetRef = useRef("");

      useEffect(() => {
        let cancelled = false;
        loadPDFJS().then((lib) => !cancelled && setPDFJSLib(lib));
        return () => { cancelled = true; };
      }, []);

      useEffect(() => {
        if (!pdfjsLib || !pdfUrl) return;
        let cancelled = false;
        setPDF(null);
        setPageCount(0);
        setRenderedPages(new Set());
        pdfjsLib.getDocument({ url: pdfUrl }).promise.then((nextPDF) => {
          if (cancelled) return;
          setPDF(nextPDF);
          setPageCount(nextPDF.numPages);
        }).catch((error) => {
          if (!cancelled) setPDF({ error: error.message || String(error) });
        });
        return () => { cancelled = true; };
      }, [pdfjsLib, pdfUrl]);

      useEffect(() => {
        if (!containerRef.current) return;
        const observer = new ResizeObserver((entries) => {
          const width = entries[0]?.contentRect?.width;
          if (width) setViewportWidth(width);
        });
        observer.observe(containerRef.current);
        return () => observer.disconnect();
      }, []);

      useEffect(() => {
        const targetZoom = Number(viewer.target?.zoom || 0);
        if (targetZoom > 0) setZoom(clamp(targetZoom, 0.5, 2.75));
      }, [viewer.target?.zoom]);

      useEffect(() => {
        setRenderedPages(new Set());
      }, [zoom, viewportWidth]);

      useEffect(() => {
        const anchor = zoomAnchorRef.current;
        const container = containerRef.current;
        if (!anchor || !container) return;
        zoomAnchorRef.current = null;
        window.requestAnimationFrame(() => {
          container.scrollLeft = anchor.contentX * anchor.ratio - anchor.viewX;
          container.scrollTop = anchor.contentY * anchor.ratio - anchor.viewY;
        });
      }, [zoom]);

      useEffect(() => {
        if (!pdf || pdf.error || !pageCount || renderedPages.size < pageCount) return;
        const target = viewer.target || {};
        const targetKey = JSON.stringify([viewer.courseId || "", viewer.resourceId || "", target.page || 1, target.query || ""]);
        if (appliedTargetRef.current === targetKey) return;
        appliedTargetRef.current = targetKey;
        const go = async () => {
          if (target.query) {
            const page = await findTextPage(pdf, String(target.query));
            if (page) {
              scrollToPage(page);
              return;
            }
          }
          scrollToPage(Math.min(Math.max(1, Number(target.page || 1)), pageCount));
        };
        go();
      }, [pdf, pageCount, renderedPages, viewer.target?.page, viewer.target?.query]);

      useEffect(() => {
        if (!containerRef.current || !pageCount) return;
        const onScroll = () => {
          const containerTop = containerRef.current.getBoundingClientRect().top;
          let best = currentPage;
          let bestDistance = Infinity;
          wrappersRef.current.forEach((node, pageNo) => {
            const distance = Math.abs(node.getBoundingClientRect().top - containerTop);
            if (distance < bestDistance) {
              best = pageNo;
              bestDistance = distance;
            }
          });
          if (best !== currentPage) {
            setCurrentPage(best);
            queueSaveState({ page: best });
            reportModelContext(best, pageCount, viewer.title || "PDF");
          }
        };
        const node = containerRef.current;
        node.addEventListener("scroll", onScroll, { passive: true });
        return () => node.removeEventListener("scroll", onScroll);
      }, [currentPage, pageCount, viewer.title, selection]);

      const setZoomFromInteraction = useCallback((nextZoom, event) => {
        const container = containerRef.current;
        setZoom((previousZoom) => {
          const resolvedZoom = clamp(typeof nextZoom === "function" ? nextZoom(previousZoom) : nextZoom, 0.5, 3);
          if (container && event && resolvedZoom !== previousZoom) {
            const rect = container.getBoundingClientRect();
            zoomAnchorRef.current = {
              contentX: event.clientX - rect.left + container.scrollLeft,
              contentY: event.clientY - rect.top + container.scrollTop,
              viewX: event.clientX - rect.left,
              viewY: event.clientY - rect.top,
              ratio: resolvedZoom / previousZoom
            };
          }
          return resolvedZoom;
        });
      }, []);

      useEffect(() => {
        const node = containerRef.current;
        if (!node) return;
        const onWheel = (event) => {
          if (!event.ctrlKey && !event.metaKey) return;
          event.preventDefault();
          const factor = Math.exp(-event.deltaY * 0.003);
          setZoomFromInteraction((value) => value * factor, event);
        };
        node.addEventListener("wheel", onWheel, { passive: false });
        return () => node.removeEventListener("wheel", onWheel);
      }, [setZoomFromInteraction]);

      const onPageRendered = useCallback((pageNo, canvas, wrapper) => {
        canvasesRef.current.set(pageNo, canvas);
        wrappersRef.current.set(pageNo, wrapper);
        setRenderedPages((prev) => new Set(prev).add(pageNo));
      }, []);

      const scrollToPage = useCallback((pageNo) => {
        const node = wrappersRef.current.get(pageNo);
        if (!node) return;
        setCurrentPage(pageNo);
        node.scrollIntoView({ behavior: "smooth", block: "start" });
        queueSaveState({ page: pageNo });
        reportModelContext(pageNo, pageCount, viewer.title || "PDF");
      }, [pageCount, viewer.title, selection]);

      const queueSaveState = useCallback((override = {}) => {
        window.clearTimeout(saveTimer.current);
        saveTimer.current = window.setTimeout(() => saveState(override), 300);
      }, [currentPage, pageCount, selection, viewer]);

      const saveState = useCallback(async (override = {}) => {
        if (!window.openai?.callTool) return;
        const page = override.page || currentPage;
        const payload = {
          title: viewer.title || "PDF",
          courseId: viewer.courseId || "",
          resourceId: viewer.resourceId || "",
          page,
          pageCount: pageCount || 1,
          screenshotDataURL: capturePage(canvasesRef.current.get(page)),
          selectionDataURL: selection?.dataURL || "",
          selectionPage: selection?.page || 0,
          selectionX: Math.round(selection?.x || 0),
          selectionY: Math.round(selection?.y || 0),
          selectionWidth: Math.round(selection?.width || 0),
          selectionHeight: Math.round(selection?.height || 0)
        };
        await callToolCompat("save_pdf_view_state", payload);
      }, [currentPage, pageCount, selection, viewer]);

      const beginSelection = useCallback((pageNo, event) => {
        if (!selectMode) return;
        const wrapper = wrappersRef.current.get(pageNo);
        if (!wrapper) return;
        event.preventDefault();
        wrapper.setPointerCapture?.(event.pointerId);
        const point = relativePoint(wrapper, event);
        setSelectionDraft({ page: pageNo, startX: point.x, startY: point.y, currentX: point.x, currentY: point.y });
      }, [selectMode]);

      const updateSelection = useCallback((pageNo, event) => {
        if (!selectionDraft || selectionDraft.page !== pageNo) return;
        const wrapper = wrappersRef.current.get(pageNo);
        if (!wrapper) return;
        const point = relativePoint(wrapper, event);
        setSelectionDraft((draft) => draft ? { ...draft, currentX: point.x, currentY: point.y } : null);
      }, [selectionDraft]);

      const endSelection = useCallback(async (pageNo, event) => {
        if (!selectionDraft || selectionDraft.page !== pageNo) return;
        const wrapper = wrappersRef.current.get(pageNo);
        const canvas = canvasesRef.current.get(pageNo);
        if (!wrapper || !canvas) return;
        const point = relativePoint(wrapper, event);
        const rect = normalizeRect(selectionDraft.startX, selectionDraft.startY, point.x, point.y);
        setSelectionDraft(null);
        setSelectMode(false);
        if (rect.width < 12 || rect.height < 12) return;
        const dataURL = cropCanvas(canvas, wrapper, rect);
        const nextSelection = { ...rect, page: pageNo, dataURL };
        setSelection(nextSelection);
        await saveSelectionState(nextSelection, pageNo);
        await sendFollowUp(nextSelection);
      }, [selectionDraft, viewer, pageCount]);

      const saveSelectionState = useCallback(async (nextSelection, pageNo) => {
        if (!window.openai?.callTool) return;
        const payload = {
          title: viewer.title || "PDF",
          courseId: viewer.courseId || "",
          resourceId: viewer.resourceId || "",
          page: pageNo,
          pageCount: pageCount || 1,
          screenshotDataURL: capturePage(canvasesRef.current.get(pageNo)),
          selectionDataURL: nextSelection.dataURL,
          selectionPage: nextSelection.page,
          selectionX: Math.round(nextSelection.x),
          selectionY: Math.round(nextSelection.y),
          selectionWidth: Math.round(nextSelection.width),
          selectionHeight: Math.round(nextSelection.height)
        };
        await callToolCompat("save_pdf_view_state", payload);
      }, [viewer, pageCount]);

      if (!pdfUrl) return e(EmptyState, { title: "PDF URL missing", description: "The MCP tool did not provide a PDF URL." });
      if (pdf?.error) return e(EmptyState, { title: "PDF render failed", description: pdf.error });

      const pages = pageCount ? Array.from({ length: pageCount }, (_, index) => index + 1) : [];
      const rendered = pageCount > 0 && renderedPages.size >= pageCount;
      return e("div", { className: "grid h-full grid-rows-[auto_1fr] overflow-hidden" },
        e("div", { className: "flex flex-wrap items-center gap-2 border-b border-border bg-background/95 px-3 py-2 backdrop-blur" },
          e("div", { className: "flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs shadow-sm" },
            e("span", { className: "font-medium tabular-nums" }, rendered ? currentPage + " / " + pageCount : "Loading"),
            e("span", { className: "hidden text-muted-foreground sm:inline" }, "Page")
          ),
          e(ZoomControl, {
            zoom,
            onZoomOut: () => setZoomFromInteraction((value) => value - 0.1),
            onZoomIn: () => setZoomFromInteraction((value) => value + 0.1),
            onReset: () => setZoomFromInteraction(1)
          }),
          e("div", { className: "hidden items-center gap-1 text-xs text-muted-foreground md:flex" },
            e("span", null, "Pinch"),
            e("span", null, "or"),
            e(Kbd, null, "ctrl"),
            e("span", null, "+ scroll to zoom")
          ),
          e("div", { className: "flex-1" }),
          e(Separator),
          e(Button, { variant: selectMode ? "default" : "outline", title: "Select an area to ask ChatGPT", onClick: () => setSelectMode((value) => !value) }, e(Icon, { name: "scan" }), selectMode ? "Select area" : "Ask"),
          e(Button, { variant: "outline", title: "Save current page screenshot for ChatGPT", onClick: () => saveState() }, e(Icon, { name: "camera" }), "Screenshot"),
          e(Button, { variant: "outline", title: "Open fullscreen", onClick: () => window.openai?.requestDisplayMode?.({ mode: "fullscreen" }) }, e(Icon, { name: "expand" }), "Fullscreen")
        ),
        e("div", { ref: containerRef, className: cn("min-h-0 overflow-auto bg-muted/40 p-4 pdf-scrollbar sm:p-6", selectMode && "cursor-crosshair") },
          !pdf ? e("div", { className: "flex h-full items-center justify-center text-sm text-muted-foreground" }, "Loading embedded PDF...") :
          e("div", { className: "mx-auto flex max-w-[1200px] flex-col items-center gap-5" },
            pages.map((pageNo) => e(PDFPage, {
              key: pageNo + "-" + zoom + "-" + viewportWidth,
              pdf,
              pageNo,
              zoom,
              viewportWidth,
              currentSelection: selectionDraft?.page === pageNo ? selectionDraft : null,
              savedSelection: selection?.page === pageNo ? selection : null,
              onRendered: onPageRendered,
              onPointerDown: beginSelection,
              onPointerMove: updateSelection,
              onPointerUp: endSelection
            }))
          )
        )
      );
    }

    // TODO: Add persistent visual annotations so ChatGPT can draw highlights on top of the rendered PDF canvas.

    function PDFPage({ pdf, pageNo, zoom, viewportWidth, currentSelection, savedSelection, onRendered, onPointerDown, onPointerMove, onPointerUp }) {
      const canvasRef = useRef(null);
      const wrapperRef = useRef(null);
      const [size, setSize] = useState(null);

      useEffect(() => {
        let cancelled = false;
        let renderTask = null;
        const render = async () => {
          const page = await pdf.getPage(pageNo);
          const viewport = page.getViewport({ scale: 1 });
          const width = Math.min(Math.max(320, viewportWidth - 32), 1120);
          const scale = clamp((width / viewport.width) * zoom, 0.35, 4);
          const scaled = page.getViewport({ scale });
          const canvas = canvasRef.current;
          const wrapper = wrapperRef.current;
          if (!canvas || !wrapper || cancelled) return;
          const context = canvas.getContext("2d", { alpha: false });
          canvas.width = Math.floor(scaled.width);
          canvas.height = Math.floor(scaled.height);
          setSize({ width: scaled.width, height: scaled.height });
          renderTask = page.render({ canvasContext: context, viewport: scaled });
          await renderTask.promise;
          if (!cancelled) onRendered(pageNo, canvas, wrapper);
        };
        render().catch(() => {});
        return () => {
          cancelled = true;
          try { renderTask?.cancel?.(); } catch (_) {}
        };
      }, [pdf, pageNo, zoom, viewportWidth, onRendered]);

      const draftRect = currentSelection ? normalizeRect(currentSelection.startX, currentSelection.startY, currentSelection.currentX, currentSelection.currentY) : null;
      const selectionRect = draftRect || savedSelection;

      return e("article", {
        ref: wrapperRef,
        className: "pdf-page-shadow relative w-fit max-w-full rounded-lg border border-border bg-card p-2",
        "data-page": pageNo,
        onPointerDown: (event) => onPointerDown(pageNo, event),
        onPointerMove: (event) => onPointerMove(pageNo, event),
        onPointerUp: (event) => onPointerUp(pageNo, event)
      },
        e("canvas", { ref: canvasRef, className: "block max-w-full rounded-sm bg-white", style: size ? { width: size.width, height: size.height } : undefined }),
        selectionRect ? e("div", { className: "selection-box", style: { left: selectionRect.x, top: selectionRect.y, width: selectionRect.width, height: selectionRect.height } }) : null,
        e("div", { className: "flex items-center justify-between px-1 pt-2 text-xs text-muted-foreground" },
          e("span", null, "Page " + pageNo),
          e("span", { className: "tabular-nums" }, Math.round(zoom * 100) + "%")
        )
      );
    }

    let pdfjsLibPromise = null;
    async function loadPDFJS() {
      if (!pdfjsLibPromise) {
        pdfjsLibPromise = import("https://cdn.jsdelivr.net/npm/pdfjs-dist@4.10.38/build/pdf.mjs").then((lib) => {
          lib.GlobalWorkerOptions.workerSrc = "https://cdn.jsdelivr.net/npm/pdfjs-dist@4.10.38/build/pdf.worker.mjs";
          return lib;
        });
      }
      return pdfjsLibPromise;
    }

    async function findTextPage(pdf, query) {
      if (!pdf || !query.trim()) return 0;
      const needle = query.trim().toLowerCase();
      for (let pageNo = 1; pageNo <= pdf.numPages; pageNo++) {
        const page = await pdf.getPage(pageNo);
        const text = await page.getTextContent();
        const haystack = text.items.map((item) => item.str || "").join(" ").toLowerCase();
        if (haystack.includes(needle)) return pageNo;
      }
      return 0;
    }

    function capturePage(canvas) {
      if (!canvas) return "";
      const maxWidth = 1100;
      const scale = Math.min(1, maxWidth / canvas.width);
      if (scale >= 1) return canvas.toDataURL("image/jpeg", 0.76);
      const thumbnail = document.createElement("canvas");
      thumbnail.width = Math.floor(canvas.width * scale);
      thumbnail.height = Math.floor(canvas.height * scale);
      thumbnail.getContext("2d").drawImage(canvas, 0, 0, thumbnail.width, thumbnail.height);
      return thumbnail.toDataURL("image/jpeg", 0.76);
    }

    function cropCanvas(canvas, wrapper, rect) {
      const canvasRect = canvas.getBoundingClientRect();
      const wrapperRect = wrapper.getBoundingClientRect();
      const offsetX = canvasRect.left - wrapperRect.left;
      const offsetY = canvasRect.top - wrapperRect.top;
      const x = clamp(rect.x - offsetX, 0, canvasRect.width);
      const y = clamp(rect.y - offsetY, 0, canvasRect.height);
      const width = clamp(rect.width, 1, canvasRect.width - x);
      const height = clamp(rect.height, 1, canvasRect.height - y);
      const scaleX = canvas.width / canvasRect.width;
      const scaleY = canvas.height / canvasRect.height;
      const target = document.createElement("canvas");
      target.width = Math.max(1, Math.floor(width * scaleX));
      target.height = Math.max(1, Math.floor(height * scaleY));
      target.getContext("2d").drawImage(canvas, Math.floor(x * scaleX), Math.floor(y * scaleY), target.width, target.height, 0, 0, target.width, target.height);
      return target.toDataURL("image/jpeg", 0.82);
    }

    function relativePoint(node, event) {
      const rect = node.getBoundingClientRect();
      return {
        x: clamp(event.clientX - rect.left, 0, rect.width),
        y: clamp(event.clientY - rect.top, 0, rect.height)
      };
    }

    function normalizeRect(x1, y1, x2, y2) {
      return { x: Math.min(x1, x2), y: Math.min(y1, y2), width: Math.abs(x2 - x1), height: Math.abs(y2 - y1) };
    }

    async function callToolCompat(name, args) {
      try {
        return await window.openai.callTool(name, args);
      } catch (error) {
        return await window.openai.callTool({ name, arguments: args });
      }
    }

    async function sendFollowUp(selection) {
      const text = "I selected an area on page " + selection.page + " of the Moodle PDF. Please use get_pdf_selection to inspect the selected screenshot and answer what is shown there.";
      if (window.openai?.sendFollowUpMessage) {
        try {
          await window.openai.sendFollowUpMessage({ prompt: text });
          return;
        } catch (_) {
          try {
            await window.openai.sendFollowUpMessage(text);
            return;
          } catch (_) {}
        }
      }
      reportModelContext(selection.page, "?", "PDF", "Selected area is ready. Ask ChatGPT to use get_pdf_selection.");
    }

    function reportModelContext(page, pageCount, title, extra) {
      const text = "Moodle PDF open: " + (title || "PDF") + ". Current visible page: " + page + " of " + pageCount + "." + (extra ? " " + extra : "");
      if (window.openai?.updateModelContext) {
        window.openai.updateModelContext(text);
        return;
      }
      window.parent.postMessage({ jsonrpc: "2.0", id: "pdf-state-" + Date.now(), method: "ui/update-model-context", params: { text } }, "*");
    }

    function formatDate(value) {
      const date = new Date(value);
      return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
    }

    root.render(e(App));
  </script>
</body>
</html>`
