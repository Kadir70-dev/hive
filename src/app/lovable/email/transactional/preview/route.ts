import * as React from 'react';
import { render } from '@react-email/render';
import { TEMPLATES } from '@/lib/email-templates/registry';
export const runtime='nodejs';
export async function POST(request:Request){
        const apiKey = process.env["LOVABLE_API_KEY"];
        if (!apiKey) {
          return Response.json({ error: "Server configuration error" }, { status: 500 });
        }

        // Verify the caller is authorized with LOVABLE_API_KEY
        const authHeader = request.headers.get("Authorization");
        const token = authHeader?.replace(/^Bearer\s+/i, "");
        if (token !== apiKey) {
          return Response.json({ error: "Unauthorized" }, { status: 401 });
        }

        const results: Array<{
          templateName: string;
          displayName: string;
          subject: string;
          html: string;
          status: "ready" | "preview_data_required" | "render_failed";
          errorMessage?: string;
        }> = [];

        for (const [name, entry] of Object.entries(TEMPLATES)) {
          const displayName = entry.displayName || name;

          if (!entry.previewData) {
            results.push({
              templateName: name,
              displayName,
              subject: "",
              html: "",
              status: "preview_data_required",
            });
            continue;
          }

          try {
            const html = await render(React.createElement(entry.component, entry.previewData));
            const resolvedSubject =
              typeof entry.subject === "function"
                ? entry.subject(entry.previewData)
                : entry.subject;

            results.push({
              templateName: name,
              displayName,
              subject: resolvedSubject,
              html,
              status: "ready",
            });
          } catch (err) {
            console.error("Failed to render template for preview", {
              template: name,
              error: err,
            });
            results.push({
              templateName: name,
              displayName,
              subject: "",
              html: "",
              status: "render_failed",
              errorMessage: err instanceof Error ? err.message : String(err),
            });
          }
        }

        return Response.json({ templates: results });

}
