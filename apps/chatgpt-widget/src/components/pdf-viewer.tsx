import { Camera, Expand, Minus, Plus, ScanLine } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { PDFDocumentProxy, PDFPageProxy, RenderTask } from "pdfjs-dist";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { callToolCompat, reportModelContext, sendSelectionFollowUp } from "@/lib/chatgpt";
import { capturePage, clamp, cropCanvas, findTextPage, getPDFLib, normalizeRect, relativePoint } from "@/lib/pdf";
import { cn } from "@/lib/utils";
import type { PDFSelection, PDFViewerDescriptor } from "@/types/openai";

type PDFViewerProps = {
  viewer: PDFViewerDescriptor;
  pdfUrl?: string;
};

type SelectionDraft = {
  page: number;
  startX: number;
  startY: number;
  currentX: number;
  currentY: number;
};

type ZoomAnchor = {
  contentX: number;
  contentY: number;
  viewX: number;
  viewY: number;
  ratio: number;
};

export function PDFViewer({ viewer, pdfUrl }: PDFViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasesRef = useRef(new Map<number, HTMLCanvasElement>());
  const wrappersRef = useRef(new Map<number, HTMLElement>());
  const saveTimer = useRef<number>(0);
  const zoomAnchorRef = useRef<ZoomAnchor | null>(null);
  const appliedTargetRef = useRef("");

  const [pdf, setPDF] = useState<PDFDocumentProxy | { error: string } | null>(null);
  const [pageCount, setPageCount] = useState(0);
  const [currentPage, setCurrentPage] = useState(Math.max(1, Number(viewer.target?.page || 1)));
  const [zoom, setZoom] = useState(clamp(Number(viewer.target?.zoom || 1), 0.5, 2.75));
  const [viewportWidth, setViewportWidth] = useState(900);
  const [renderedPages, setRenderedPages] = useState(new Set<number>());
  const [selectMode, setSelectMode] = useState(false);
  const [selectionDraft, setSelectionDraft] = useState<SelectionDraft | null>(null);
  const [selection, setSelection] = useState<PDFSelection | null>(null);

  const saveState = useCallback(
    async (override: { page?: number } = {}) => {
      const page = override.page || currentPage;
      await callToolCompat("save_pdf_view_state", {
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
        selectionHeight: Math.round(selection?.height || 0),
      });
    },
    [currentPage, pageCount, selection, viewer],
  );

  const queueSaveState = useCallback(
    (override: { page?: number } = {}) => {
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => void saveState(override), 300);
    },
    [saveState],
  );

  const scrollToPage = useCallback(
    (pageNo: number) => {
      const node = wrappersRef.current.get(pageNo);
      if (!node) return;
      setCurrentPage(pageNo);
      node.scrollIntoView({ behavior: "smooth", block: "start" });
      queueSaveState({ page: pageNo });
      reportModelContext(pageNo, pageCount, viewer.title || "PDF");
    },
    [pageCount, queueSaveState, viewer.title],
  );

  useEffect(() => {
    if (!pdfUrl) return;
    let cancelled = false;
    setPDF(null);
    setPageCount(0);
    setRenderedPages(new Set());
    getPDFLib()
      .getDocument({ url: pdfUrl })
      .promise.then((nextPDF) => {
        if (cancelled) return;
        setPDF(nextPDF);
        setPageCount(nextPDF.numPages);
      })
      .catch((error: unknown) => {
        if (!cancelled) setPDF({ error: error instanceof Error ? error.message : String(error) });
      });
    return () => {
      cancelled = true;
    };
  }, [pdfUrl]);

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
    if (!pdf || "error" in pdf || !pageCount || renderedPages.size < Math.min(pageCount, 3)) return;
    const target = viewer.target || {};
    const targetKey = JSON.stringify([viewer.courseId || "", viewer.resourceId || "", target.page || 1, target.query || ""]);
    if (appliedTargetRef.current === targetKey) return;
    appliedTargetRef.current = targetKey;

    void (async () => {
      if (target.query) {
        const page = await findTextPage(pdf, String(target.query));
        if (page) {
          scrollToPage(page);
          return;
        }
      }
      scrollToPage(Math.min(Math.max(1, Number(target.page || 1)), pageCount));
    })();
  }, [pdf, pageCount, renderedPages, scrollToPage, viewer.courseId, viewer.resourceId, viewer.target]);

  useEffect(() => {
    const node = containerRef.current;
    if (!node || !pageCount) return;

    const onScroll = () => {
      const containerTop = node.getBoundingClientRect().top;
      let best = currentPage;
      let bestDistance = Infinity;
      wrappersRef.current.forEach((pageNode, pageNo) => {
        const distance = Math.abs(pageNode.getBoundingClientRect().top - containerTop);
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

    node.addEventListener("scroll", onScroll, { passive: true });
    return () => node.removeEventListener("scroll", onScroll);
  }, [currentPage, pageCount, queueSaveState, viewer.title]);

  const setZoomFromInteraction = useCallback((nextZoom: number | ((previousZoom: number) => number), event?: WheelEvent) => {
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
          ratio: resolvedZoom / previousZoom,
        };
      }
      return resolvedZoom;
    });
  }, []);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) return;
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const factor = Math.exp(-event.deltaY * 0.003);
      setZoomFromInteraction((value) => value * factor, event);
    };
    node.addEventListener("wheel", onWheel, { passive: false });
    return () => node.removeEventListener("wheel", onWheel);
  }, [setZoomFromInteraction]);

  const onPageRendered = useCallback((pageNo: number, canvas: HTMLCanvasElement, wrapper: HTMLElement) => {
    canvasesRef.current.set(pageNo, canvas);
    wrappersRef.current.set(pageNo, wrapper);
    setRenderedPages((prev) => new Set(prev).add(pageNo));
  }, []);

  const beginSelection = useCallback(
    (pageNo: number, event: React.PointerEvent<HTMLElement>) => {
      if (!selectMode) return;
      const wrapper = wrappersRef.current.get(pageNo);
      if (!wrapper) return;
      event.preventDefault();
      wrapper.setPointerCapture?.(event.pointerId);
      const point = relativePoint(wrapper, event);
      setSelectionDraft({ page: pageNo, startX: point.x, startY: point.y, currentX: point.x, currentY: point.y });
    },
    [selectMode],
  );

  const updateSelection = useCallback(
    (pageNo: number, event: React.PointerEvent<HTMLElement>) => {
      if (!selectionDraft || selectionDraft.page !== pageNo) return;
      const wrapper = wrappersRef.current.get(pageNo);
      if (!wrapper) return;
      const point = relativePoint(wrapper, event);
      setSelectionDraft((draft) => (draft ? { ...draft, currentX: point.x, currentY: point.y } : null));
    },
    [selectionDraft],
  );

  const endSelection = useCallback(
    async (pageNo: number, event: React.PointerEvent<HTMLElement>) => {
      if (!selectionDraft || selectionDraft.page !== pageNo) return;
      const wrapper = wrappersRef.current.get(pageNo);
      const canvas = canvasesRef.current.get(pageNo);
      if (!wrapper || !canvas) return;
      const point = relativePoint(wrapper, event);
      const rect = normalizeRect(selectionDraft.startX, selectionDraft.startY, point.x, point.y);
      setSelectionDraft(null);
      setSelectMode(false);
      if (rect.width < 12 || rect.height < 12) return;

      const nextSelection = { ...rect, page: pageNo, dataURL: cropCanvas(canvas, wrapper, rect) };
      setSelection(nextSelection);
      await saveSelectionState(nextSelection, pageNo, viewer, pageCount, canvasesRef.current);
      await sendSelectionFollowUp(nextSelection);
    },
    [pageCount, selectionDraft, viewer],
  );

  if (!pdfUrl) return <PDFEmptyState title="PDF URL missing" description="The MCP tool did not provide a PDF URL." />;
  if (pdf && "error" in pdf) return <PDFEmptyState title="PDF render failed" description={pdf.error} />;

  const pages = pageCount ? Array.from({ length: pageCount }, (_, index) => index + 1) : [];
  const rendered = pageCount > 0 && renderedPages.size >= Math.min(pageCount, 3);

  return (
    <div className="grid h-full grid-rows-[auto_1fr] overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 border-b bg-background/95 px-3 py-2 backdrop-blur">
        <Badge variant="secondary" className="h-8 gap-1.5 rounded-md px-2.5 text-xs tabular-nums">
          <span className="font-medium">{rendered ? `${currentPage} / ${pageCount}` : "Loading"}</span>
          <span className="hidden text-muted-foreground sm:inline">Page</span>
        </Badge>
        <ZoomControl
          zoom={zoom}
          onZoomOut={() => setZoomFromInteraction((value) => value - 0.1)}
          onZoomIn={() => setZoomFromInteraction((value) => value + 0.1)}
          onReset={() => setZoomFromInteraction(1)}
        />
        <div className="hidden items-center gap-1 text-xs text-muted-foreground md:flex">
          <span>Pinch</span>
          <span>or</span>
          <kbd className="rounded border bg-background px-1.5 py-0.5 font-mono text-[10px] shadow-sm">ctrl</kbd>
          <span>+ scroll to zoom</span>
        </div>
        <div className="flex-1" />
        <Separator orientation="vertical" className="hidden h-5 sm:block" />
        <Button variant={selectMode ? "default" : "outline"} size="sm" title="Select an area to ask ChatGPT" onClick={() => setSelectMode((value) => !value)}>
          <ScanLine data-icon="inline-start" />
          {selectMode ? "Select area" : "Ask"}
        </Button>
        <Button variant="outline" size="sm" title="Save current page screenshot for ChatGPT" onClick={() => void saveState()}>
          <Camera data-icon="inline-start" />
          Screenshot
        </Button>
        <Button variant="outline" size="sm" title="Open fullscreen" onClick={() => void window.openai?.requestDisplayMode?.({ mode: "fullscreen" })}>
          <Expand data-icon="inline-start" />
          Fullscreen
        </Button>
      </div>
      <div ref={containerRef} className={cn("min-h-0 overflow-auto bg-muted/40 p-4 pdf-scrollbar sm:p-6", selectMode && "cursor-crosshair")}>
        {!pdf ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Loading embedded PDF...</div>
        ) : (
          <div className="mx-auto flex max-w-[1200px] flex-col items-center gap-5">
            {pages.map((pageNo) => (
              <PDFPage
                key={`${pageNo}-${zoom}-${viewportWidth}`}
                pdf={pdf}
                pageNo={pageNo}
                zoom={zoom}
                viewportWidth={viewportWidth}
                currentSelection={selectionDraft?.page === pageNo ? selectionDraft : null}
                savedSelection={selection?.page === pageNo ? selection : null}
                onRendered={onPageRendered}
                onPointerDown={beginSelection}
                onPointerMove={updateSelection}
                onPointerUp={endSelection}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// TODO: Add persistent visual annotations so ChatGPT can draw highlights on top of the rendered PDF canvas.

function ZoomControl({ zoom, onZoomIn, onZoomOut, onReset }: { zoom: number; onZoomIn: () => void; onZoomOut: () => void; onReset: () => void }) {
  return (
    <div className="flex items-center rounded-md border bg-background p-1 shadow-sm">
      <Button variant="ghost" size="icon" aria-label="Zoom out" title="Zoom out" onClick={onZoomOut}>
        <Minus data-icon="inline-start" />
      </Button>
      <button className="h-8 min-w-14 rounded-sm px-2 text-center text-xs font-medium tabular-nums hover:bg-accent" title="Reset zoom" onClick={onReset}>
        {Math.round(zoom * 100)}%
      </button>
      <Button variant="ghost" size="icon" aria-label="Zoom in" title="Zoom in" onClick={onZoomIn}>
        <Plus data-icon="inline-start" />
      </Button>
    </div>
  );
}

function PDFPage({
  pdf,
  pageNo,
  zoom,
  viewportWidth,
  currentSelection,
  savedSelection,
  onRendered,
  onPointerDown,
  onPointerMove,
  onPointerUp,
}: {
  pdf: PDFDocumentProxy;
  pageNo: number;
  zoom: number;
  viewportWidth: number;
  currentSelection: SelectionDraft | null;
  savedSelection: PDFSelection | null;
  onRendered: (pageNo: number, canvas: HTMLCanvasElement, wrapper: HTMLElement) => void;
  onPointerDown: (pageNo: number, event: React.PointerEvent<HTMLElement>) => void;
  onPointerMove: (pageNo: number, event: React.PointerEvent<HTMLElement>) => void;
  onPointerUp: (pageNo: number, event: React.PointerEvent<HTMLElement>) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapperRef = useRef<HTMLElement>(null);
  const [size, setSize] = useState<{ width: number; height: number } | null>(null);

  useEffect(() => {
    let cancelled = false;
    let renderTask: RenderTask | null = null;
    const render = async () => {
      const page: PDFPageProxy = await pdf.getPage(pageNo);
      const viewport = page.getViewport({ scale: 1 });
      const width = Math.min(Math.max(320, viewportWidth - 32), 1120);
      const scale = clamp((width / viewport.width) * zoom, 0.35, 4);
      const scaled = page.getViewport({ scale });
      const canvas = canvasRef.current;
      const wrapper = wrapperRef.current;
      if (!canvas || !wrapper || cancelled) return;
      const context = canvas.getContext("2d", { alpha: false });
      if (!context) return;
      canvas.width = Math.floor(scaled.width);
      canvas.height = Math.floor(scaled.height);
      setSize({ width: scaled.width, height: scaled.height });
      renderTask = page.render({ canvasContext: context, viewport: scaled });
      await renderTask.promise;
      if (!cancelled) onRendered(pageNo, canvas, wrapper);
    };

    void render().catch(() => {});
    return () => {
      cancelled = true;
      try {
        renderTask?.cancel();
      } catch (_) {
        // PDF.js can throw when the render task already finished.
      }
    };
  }, [pdf, pageNo, zoom, viewportWidth, onRendered]);

  const draftRect = currentSelection ? normalizeRect(currentSelection.startX, currentSelection.startY, currentSelection.currentX, currentSelection.currentY) : null;
  const selectionRect = draftRect || savedSelection;

  return (
    <article
      ref={wrapperRef}
      className="pdf-page-shadow relative w-fit max-w-full rounded-lg border bg-card p-2"
      data-page={pageNo}
      onPointerDown={(event) => onPointerDown(pageNo, event)}
      onPointerMove={(event) => onPointerMove(pageNo, event)}
      onPointerUp={(event) => onPointerUp(pageNo, event)}
    >
      <canvas ref={canvasRef} className="block max-w-full rounded-sm bg-white" style={size ? { width: size.width, height: size.height } : undefined} />
      {selectionRect ? <div className="selection-box" style={{ left: selectionRect.x, top: selectionRect.y, width: selectionRect.width, height: selectionRect.height }} /> : null}
      <div className="flex items-center justify-between gap-4 px-1 pt-2 text-xs text-muted-foreground">
        <span>Page {pageNo}</span>
        <span className="tabular-nums">{Math.round(zoom * 100)}%</span>
      </div>
    </article>
  );
}

function PDFEmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6 text-center">
      <div className="flex max-w-sm flex-col gap-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}

async function saveSelectionState(
  nextSelection: PDFSelection,
  pageNo: number,
  viewer: PDFViewerDescriptor,
  pageCount: number,
  canvases: Map<number, HTMLCanvasElement>,
) {
  await callToolCompat("save_pdf_view_state", {
    title: viewer.title || "PDF",
    courseId: viewer.courseId || "",
    resourceId: viewer.resourceId || "",
    page: pageNo,
    pageCount: pageCount || 1,
    screenshotDataURL: capturePage(canvases.get(pageNo)),
    selectionDataURL: nextSelection.dataURL,
    selectionPage: nextSelection.page,
    selectionX: Math.round(nextSelection.x),
    selectionY: Math.round(nextSelection.y),
    selectionWidth: Math.round(nextSelection.width),
    selectionHeight: Math.round(nextSelection.height),
  });
}
