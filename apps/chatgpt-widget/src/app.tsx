import { FileText } from "lucide-react";

import { AdminDashboard } from "@/components/admin-dashboard";
import { PDFViewer } from "@/components/pdf-viewer";
import { Badge } from "@/components/ui/badge";
import { useToolPayload } from "@/hooks/use-tool-payload";
import type { MoodleData, MoodleDocument, WidgetMetadata } from "@/types/openai";

export function App() {
  const { data, metadata } = useToolPayload();

  if (isAdminView()) {
    return <AdminDashboard />;
  }

  return (
    <main className="grid h-screen min-h-[420px] grid-rows-[auto_1fr] overflow-hidden bg-muted/40 text-foreground">
      <Header data={data} />
      <section className="min-h-0 overflow-hidden">
        <Content data={data} metadata={metadata} />
      </section>
    </main>
  );
}

function Header({ data }: { data: MoodleData | null }) {
  const title = data?.viewer?.title || data?.document?.title || (Array.isArray(data?.courses) ? "Moodle courses" : "Moodle");
  const subtitle = data?.viewer?.fileType === "pdf" ? "Scrollable PDF viewer" : "Moodle";

  return (
    <header className="flex h-14 items-center justify-between gap-3 border-b bg-background/95 px-4 backdrop-blur">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-card text-muted-foreground shadow-sm">
          <FileText data-icon="inline-start" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold tracking-normal">{title}</h1>
          <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
        </div>
      </div>
      <Badge variant="secondary" className="hidden sm:inline-flex">
        Moodle
      </Badge>
    </header>
  );
}

function Content({ data, metadata }: { data: MoodleData | null; metadata: WidgetMetadata }) {
  if (!data) return <EmptyState title="Waiting for data" description="Open a Moodle course or PDF from ChatGPT." />;
  if (data.viewer?.fileType === "pdf") return <PDFViewer viewer={data.viewer} pdfUrl={metadata.pdfUrl} />;
  if (Array.isArray(data.courses)) {
    return <ListView title="Courses" items={data.courses} renderItem={(course) => [course.fullname || course.shortname || "Course", [course.shortname, course.category].filter(Boolean).join(" · ")]} />;
  }
  if (Array.isArray(data.materials)) {
    return <ListView title="Materials" items={data.materials} renderItem={(item) => [item.name || "Material", [item.sectionName, item.fileType || item.type].filter(Boolean).join(" · ")]} />;
  }
  if (Array.isArray(data.events)) {
    return <ListView title="Calendar" items={data.events} renderItem={(event) => [event.summary || "Calendar event", formatDate(event.start) + (event.location ? ` · ${event.location}` : "")]} />;
  }
  if (data.document) return <DocumentView document={data.document} />;
  return <DocumentView document={{ title: "Moodle result", text: JSON.stringify(data, null, 2) }} />;
}

function ListView<T extends { id?: string | number }>({ title, items, renderItem }: { title: string; items: T[]; renderItem: (item: T) => [string, string] }) {
  return (
    <div className="h-full overflow-auto p-4 pdf-scrollbar">
      <div className="mx-auto flex max-w-5xl flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold">{title}</h2>
          <Badge variant="secondary">{items.length}</Badge>
        </div>
        {items.map((item, index) => {
          const [headline, subline] = renderItem(item);
          return (
            <article key={item.id || index} className="rounded-lg bg-card px-4 py-3 shadow-sm ring-1 ring-border">
              <p className="text-sm font-medium">{headline}</p>
              {subline ? <p className="mt-1 text-xs text-muted-foreground">{subline}</p> : null}
            </article>
          );
        })}
      </div>
    </div>
  );
}

function DocumentView({ document }: { document: MoodleDocument }) {
  return (
    <div className="h-full overflow-auto p-4 pdf-scrollbar">
      <article className="mx-auto max-w-5xl rounded-lg bg-card p-4 text-sm shadow-sm ring-1 ring-border">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 className="min-w-0 truncate text-base font-semibold">{document.title || "Document"}</h2>
          <Badge variant="secondary">{document.metadata?.fileType ? document.metadata.fileType.toUpperCase() : "Text"}</Badge>
        </div>
        <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-6">{document.text || ""}</pre>
      </article>
    </div>
  );
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="flex flex-col gap-1 text-center">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}

function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

function isAdminView() {
  const params = new URLSearchParams(window.location.search);
  return params.get("view") === "admin";
}
