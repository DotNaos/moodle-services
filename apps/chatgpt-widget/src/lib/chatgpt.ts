import type { MoodleData, OpenAIGlobals, PDFSelection, ToolPayload, WidgetMetadata } from "@/types/openai";

type ToolResultNotification = {
  jsonrpc?: string;
  method?: string;
  params?: ToolPayload;
};

type SetGlobalsEvent = CustomEvent<{
  globals?: OpenAIGlobals;
}>;

export function normalizeToolPayload(payload?: ToolPayload | MoodleData, fallbackMetadata?: WidgetMetadata) {
  const data = isToolPayload(payload) ? payload.structuredContent : payload;
  const metadata = isToolPayload(payload)
    ? (payload._meta ?? fallbackMetadata ?? window.openai?.toolResponseMetadata ?? {})
    : (fallbackMetadata ?? window.openai?.toolResponseMetadata ?? {});

  return { data: data ?? null, metadata };
}

export function subscribeToToolPayload(onPayload: (payload?: ToolPayload | MoodleData, metadata?: WidgetMetadata) => void) {
  const receiveMessage = (event: MessageEvent<ToolResultNotification>) => {
    if (event.source !== window.parent) return;
    const message = event.data;
    if (!message || message.jsonrpc !== "2.0") return;
    if (message.method === "ui/notifications/tool-result") {
      onPayload(message.params);
    }
  };

  const receiveGlobals = (event: Event) => {
    const globals = (event as SetGlobalsEvent).detail?.globals ?? {};
    onPayload(globals.toolOutput, globals.toolResponseMetadata);
  };

  window.addEventListener("message", receiveMessage as EventListener, { passive: true });
  window.addEventListener("openai:set_globals", receiveGlobals, { passive: true });

  return () => {
    window.removeEventListener("message", receiveMessage as EventListener);
    window.removeEventListener("openai:set_globals", receiveGlobals);
  };
}

export async function callToolCompat(name: string, args: Record<string, unknown>) {
  const callTool = window.openai?.callTool;
  if (typeof callTool !== "function") return;

  try {
    return await callTool(name, args);
  } catch (_) {
    return await callTool({ name, arguments: args });
  }
}

export async function sendSelectionFollowUp(selection: PDFSelection) {
  const text =
    `I selected an area on page ${selection.page} of the Moodle PDF. ` +
    "Please use get_pdf_selection to inspect the selected screenshot and answer what is shown there.";
  const sender = window.openai?.sendFollowUpMessage;

  if (typeof sender === "function") {
    try {
      await sender({ prompt: text });
      return;
    } catch (_) {
      try {
        await sender(text);
        return;
      } catch (_) {
        // Fall back to model context below.
      }
    }
  }

  reportModelContext(selection.page, "?", "PDF", "Selected area is ready. Ask ChatGPT to use get_pdf_selection.");
}

export function reportModelContext(page: number, pageCount: number | "?", title: string, extra?: string) {
  const text = `Moodle PDF open: ${title || "PDF"}. Current visible page: ${page} of ${pageCount}.${extra ? ` ${extra}` : ""}`;
  if (window.openai?.updateModelContext) {
    window.openai.updateModelContext(text);
    return;
  }

  window.parent.postMessage(
    {
      jsonrpc: "2.0",
      id: `pdf-state-${Date.now()}`,
      method: "ui/update-model-context",
      params: { text },
    },
    "*",
  );
}

function isToolPayload(payload?: ToolPayload | MoodleData): payload is ToolPayload {
  return Boolean(payload && "structuredContent" in payload);
}
