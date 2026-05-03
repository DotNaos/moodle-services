import * as pdfjsLib from "pdfjs-dist";
import type { PDFDocumentProxy } from "pdfjs-dist";

pdfjsLib.GlobalWorkerOptions.workerSrc = "https://cdn.jsdelivr.net/npm/pdfjs-dist@4.10.38/build/pdf.worker.mjs";

export type PDFLib = typeof pdfjsLib;

export function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

export function getPDFLib() {
  return pdfjsLib;
}

export async function findTextPage(pdf: PDFDocumentProxy, query: string) {
  if (!query.trim()) return 0;
  const needle = query.trim().toLowerCase();

  for (let pageNo = 1; pageNo <= pdf.numPages; pageNo += 1) {
    const page = await pdf.getPage(pageNo);
    const text = await page.getTextContent();
    const haystack = text.items.map((item) => ("str" in item ? item.str : "")).join(" ").toLowerCase();
    if (haystack.includes(needle)) return pageNo;
  }

  return 0;
}

export function capturePage(canvas?: HTMLCanvasElement) {
  if (!canvas) return "";
  const maxWidth = 1100;
  const scale = Math.min(1, maxWidth / canvas.width);
  if (scale >= 1) return canvas.toDataURL("image/jpeg", 0.76);

  const thumbnail = document.createElement("canvas");
  thumbnail.width = Math.floor(canvas.width * scale);
  thumbnail.height = Math.floor(canvas.height * scale);
  thumbnail.getContext("2d")?.drawImage(canvas, 0, 0, thumbnail.width, thumbnail.height);
  return thumbnail.toDataURL("image/jpeg", 0.76);
}

export function cropCanvas(canvas: HTMLCanvasElement, wrapper: HTMLElement, rect: DOMRectShape) {
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
  target
    .getContext("2d")
    ?.drawImage(canvas, Math.floor(x * scaleX), Math.floor(y * scaleY), target.width, target.height, 0, 0, target.width, target.height);
  return target.toDataURL("image/jpeg", 0.82);
}

export function relativePoint(node: HTMLElement, event: React.PointerEvent | PointerEvent) {
  const rect = node.getBoundingClientRect();
  return {
    x: clamp(event.clientX - rect.left, 0, rect.width),
    y: clamp(event.clientY - rect.top, 0, rect.height),
  };
}

export function normalizeRect(x1: number, y1: number, x2: number, y2: number) {
  return {
    x: Math.min(x1, x2),
    y: Math.min(y1, y2),
    width: Math.abs(x2 - x1),
    height: Math.abs(y2 - y1),
  };
}

export type DOMRectShape = {
  x: number;
  y: number;
  width: number;
  height: number;
};
