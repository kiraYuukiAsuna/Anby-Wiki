import { safeHttpUrl } from "@/lib/http-url";

export type EmbedProvider = "youtube" | "vimeo" | "figma";

function safeToken(value: string): boolean {
  return /^[A-Za-z0-9_-]{6,32}$/.test(value);
}

export function interactiveEmbedUrl(
  provider: EmbedProvider | undefined,
  rawUrl: string,
): string | null {
  const safe = safeHttpUrl(rawUrl);
  if (!provider || !safe) return null;
  const url = new URL(safe);
  const host = url.hostname.toLowerCase().replace(/^www\./, "");
  if (provider === "youtube") {
    let videoId = "";
    if (host === "youtu.be") {
      videoId = url.pathname.split("/").filter(Boolean)[0] ?? "";
    } else if (host === "youtube.com" || host === "m.youtube.com") {
      videoId =
        url.searchParams.get("v") ??
        (url.pathname.startsWith("/embed/")
          ? url.pathname.slice("/embed/".length).split("/")[0]
          : "");
    }
    return safeToken(videoId)
      ? `https://www.youtube-nocookie.com/embed/${videoId}`
      : null;
  }
  if (provider === "vimeo" && (host === "vimeo.com" || host === "player.vimeo.com")) {
    const videoId = url.pathname.split("/").filter(Boolean).at(-1) ?? "";
    return /^\d+$/.test(videoId)
      ? `https://player.vimeo.com/video/${videoId}`
      : null;
  }
  if (
    provider === "figma" &&
    host === "figma.com" &&
    ["/design/", "/file/", "/proto/"].some((prefix) =>
      url.pathname.startsWith(prefix),
    )
  ) {
    return `https://www.figma.com/embed?embed_host=anby&url=${encodeURIComponent(url.toString())}`;
  }
  return null;
}

export function detectEmbedProvider(rawUrl: string): EmbedProvider | null {
  const safe = safeHttpUrl(rawUrl);
  if (!safe) return null;
  const url = new URL(safe);
  const host = url.hostname.toLowerCase().replace(/^www\./, "");
  if (
    host === "youtu.be" ||
    host === "youtube.com" ||
    host === "m.youtube.com"
  ) {
    return interactiveEmbedUrl("youtube", safe) ? "youtube" : null;
  }
  if (host === "vimeo.com" || host === "player.vimeo.com") {
    return interactiveEmbedUrl("vimeo", safe) ? "vimeo" : null;
  }
  if (host === "figma.com") {
    return interactiveEmbedUrl("figma", safe) ? "figma" : null;
  }
  return null;
}
