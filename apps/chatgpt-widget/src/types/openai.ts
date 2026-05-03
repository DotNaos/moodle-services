export type ToolPayload = {
  structuredContent?: MoodleData;
  _meta?: WidgetMetadata;
};

export type WidgetMetadata = {
  pdfUrl?: string;
};

export type MoodleData = {
  courses?: MoodleCourse[];
  materials?: MoodleMaterial[];
  events?: MoodleEvent[];
  document?: MoodleDocument;
  viewer?: PDFViewerDescriptor;
};

export type MoodleCourse = {
  id?: string | number;
  fullname?: string;
  shortname?: string;
  category?: string;
};

export type MoodleMaterial = {
  id?: string | number;
  name?: string;
  sectionName?: string;
  fileType?: string;
  type?: string;
};

export type MoodleEvent = {
  id?: string | number;
  summary?: string;
  start?: string;
  location?: string;
};

export type MoodleDocument = {
  title?: string;
  text?: string;
  metadata?: {
    fileType?: string;
  };
};

export type PDFViewerDescriptor = {
  id?: string;
  title?: string;
  courseId?: string;
  resourceId?: string;
  sectionName?: string;
  fileType?: string;
  target?: {
    page?: number;
    query?: string;
    zoom?: number;
  };
};

export type PDFSelection = {
  page: number;
  x: number;
  y: number;
  width: number;
  height: number;
  dataURL: string;
};

export type OpenAIGlobals = {
  toolOutput?: ToolPayload | MoodleData;
  toolResponseMetadata?: WidgetMetadata;
  callTool?: unknown;
  sendFollowUpMessage?: unknown;
  updateModelContext?: (text: string) => void;
  requestDisplayMode?: (input: { mode: "inline" | "pip" | "fullscreen" }) => Promise<void> | void;
};

declare global {
  interface Window {
    openai?: OpenAIGlobals;
  }
}
