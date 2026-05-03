package chatgptapp

const widgetURI = "ui://widget/moodle-browser-v4.html"
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
      --background: 220 20% 97%;
      --foreground: 222 47% 11%;
      --card: 0 0% 100%;
      --card-foreground: 222 47% 11%;
      --primary: 222 47% 11%;
      --primary-foreground: 210 40% 98%;
      --secondary: 210 18% 92%;
      --secondary-foreground: 222 47% 11%;
      --muted: 210 18% 92%;
      --muted-foreground: 215 12% 44%;
      --accent: 212 92% 95%;
      --accent-foreground: 222 47% 11%;
      --destructive: 0 84% 60%;
      --destructive-foreground: 210 40% 98%;
      --border: 214 20% 88%;
      --input: 214 20% 88%;
      --ring: 222 47% 11%;
      --radius: 0.75rem;
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
  </style>
</head>
<body>
  <div id="root"></div>
  <script type="module">
    import React, { useCallback, useEffect, useMemo, useRef, useState } from "https://cdn.jsdelivr.net/npm/react@18.3.1/+esm";
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
        className: cn("inline-flex shrink-0 items-center justify-center gap-2 rounded-full font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50", variants[variant], sizes[size], className)
      }, children);
    }

    function Badge({ children, className = "" }) {
      return e("span", { className: cn("inline-flex items-center rounded-full bg-secondary px-2.5 py-1 text-xs font-medium text-secondary-foreground", className) }, children);
    }

    function Slider({ value, min, max, step, onChange, label }) {
      return e("label", { className: "flex min-w-40 items-center gap-2 text-xs text-muted-foreground" },
        e("span", { className: "w-10 tabular-nums" }, label),
        e("input", {
          className: "h-2 w-36 cursor-pointer accent-foreground",
          type: "range",
          min,
          max,
          step,
          value,
          "aria-label": "Zoom",
          onChange: (event) => onChange(Number(event.target.value))
        })
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

      return e("main", { className: "grid h-screen min-h-[420px] grid-rows-[auto_1fr] overflow-hidden bg-background text-foreground" },
        e(Header, { data: toolPayload }),
        e("section", { className: "min-h-0 overflow-hidden" }, e(Content, { data: toolPayload, metadata }))
      );
    }

    function Header({ data }) {
      const title = data?.viewer?.title || data?.document?.title || (Array.isArray(data?.courses) ? "Moodle courses" : "Moodle");
      const subtitle = data?.viewer?.fileType === "pdf" ? "PDF viewer" : "Moodle";
      return e("header", { className: "flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-3" },
        e("div", { className: "min-w-0" },
          e("h1", { className: "truncate text-base font-semibold tracking-normal" }, title),
          e("p", { className: "truncate text-xs text-muted-foreground" }, subtitle)
        ),
        e(Badge, null, "Moodle")
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
        if (!pdf || pdf.error || !pageCount || renderedPages.size < pageCount) return;
        const target = viewer.target || {};
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
      const zoomLabel = Math.round(zoom * 100) + "%";

      return e("div", { className: "grid h-full grid-rows-[auto_1fr] overflow-hidden" },
        e("div", { className: "flex flex-wrap items-center gap-2 border-b border-border bg-card px-3 py-2" },
          e(Badge, null, rendered ? currentPage + " / " + pageCount : "Loading"),
          e(Button, { variant: "secondary", size: "icon", "aria-label": "Zoom out", onClick: () => setZoom((value) => clamp(value - 0.1, 0.5, 2.75)) }, "−"),
          e(Slider, { value: zoom, min: 0.5, max: 2.75, step: 0.05, label: zoomLabel, onChange: setZoom }),
          e(Button, { variant: "secondary", size: "icon", "aria-label": "Zoom in", onClick: () => setZoom((value) => clamp(value + 0.1, 0.5, 2.75)) }, "+"),
          e("div", { className: "flex-1" }),
          e(Button, { variant: selectMode ? "default" : "secondary", onClick: () => setSelectMode((value) => !value) }, selectMode ? "Drag on PDF" : "Ask"),
          e(Button, { variant: "secondary", onClick: () => saveState() }, "Screenshot"),
          e(Button, { variant: "secondary", onClick: () => window.openai?.requestDisplayMode?.({ mode: "fullscreen" }) }, "Fullscreen")
        ),
        e("div", { ref: containerRef, className: cn("min-h-0 overflow-auto p-4 pdf-scrollbar", selectMode && "cursor-crosshair") },
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
        className: "relative w-fit max-w-full rounded-lg bg-card p-2 shadow-sm ring-1 ring-border",
        "data-page": pageNo,
        onPointerDown: (event) => onPointerDown(pageNo, event),
        onPointerMove: (event) => onPointerMove(pageNo, event),
        onPointerUp: (event) => onPointerUp(pageNo, event)
      },
        e("canvas", { ref: canvasRef, className: "block max-w-full rounded-md bg-white", style: size ? { width: size.width, height: size.height } : undefined }),
        selectionRect ? e("div", { className: "selection-box", style: { left: selectionRect.x, top: selectionRect.y, width: selectionRect.width, height: selectionRect.height } }) : null,
        e("div", { className: "px-1 pt-2 text-xs text-muted-foreground" }, "Page " + pageNo)
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
