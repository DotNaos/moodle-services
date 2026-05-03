import { useCallback, useEffect, useState } from "react";

import { normalizeToolPayload, subscribeToToolPayload } from "@/lib/chatgpt";
import type { MoodleData, ToolPayload, WidgetMetadata } from "@/types/openai";

export function useToolPayload() {
  const [data, setData] = useState<MoodleData | null>(null);
  const [metadata, setMetadata] = useState<WidgetMetadata>({});

  const renderToolPayload = useCallback((payload?: ToolPayload | MoodleData, fallbackMetadata?: WidgetMetadata) => {
    const normalized = normalizeToolPayload(payload, fallbackMetadata);
    if (normalized.data) {
      setData(normalized.data);
      setMetadata(normalized.metadata);
    }
  }, []);

  useEffect(() => {
    const unsubscribe = subscribeToToolPayload(renderToolPayload);
    const tryInitialRender = () => renderToolPayload(window.openai?.toolOutput, window.openai?.toolResponseMetadata);
    tryInitialRender();
    const timers = [50, 250, 1000].map((delay) => window.setTimeout(tryInitialRender, delay));

    return () => {
      unsubscribe();
      timers.forEach(window.clearTimeout);
    };
  }, [renderToolPayload]);

  return { data, metadata };
}
