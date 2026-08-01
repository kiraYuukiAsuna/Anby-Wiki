/**
 * Reads the server's versioned RenderedPage projection.
 *
 * The HTML is not arbitrary page input: backend/internal/render escapes every
 * AST value and only permits in-process component renderers. Keeping the
 * injection in this one component makes that trust boundary easy to audit.
 */
export function ServerRenderedDocument({
  html,
  rendererVersion,
}: {
  html: string;
  rendererVersion: string;
}) {
  return (
    <div
      className="wiki-rendered"
      data-rendered-document=""
      data-renderer-version={rendererVersion}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
