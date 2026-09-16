import { Browser } from "@wailsio/runtime";

const DANGEROUS_SCHEMES = ["javascript:", "data:", "vbscript:"];

function resolve(href: string): URL | null {
  try {
    return new URL(href, window.location.href);
  } catch {
    return null;
  }
}

function isExternal(url: URL): boolean {
  return (url.protocol === "http:" || url.protocol === "https:") && url.origin !== window.location.origin;
}

/**
 * Keep the webview pinned to the app: external http(s) links open in the OS
 * browser, and script-bearing URL schemes are dropped. This is defense in
 * depth on top of the strict CSP for any user- or daemon-controlled content
 * that might end up rendered as a link.
 */
export function installLinkGuard(): void {
  document.addEventListener(
    "click",
    (event) => {
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = (event.target as Element | null)?.closest?.("a[href]");
      if (!anchor) return;
      const href = anchor.getAttribute("href") ?? "";
      if (href === "" || href.startsWith("#")) return;
      if (DANGEROUS_SCHEMES.some((scheme) => href.trim().toLowerCase().startsWith(scheme))) {
        event.preventDefault();
        return;
      }
      const url = resolve(href);
      if (!url || !isExternal(url)) return;
      event.preventDefault();
      void Browser.OpenURL(url.toString());
    },
    true,
  );

  const nativeOpen = window.open.bind(window);
  window.open = ((url?: string | URL, target?: string, features?: string) => {
    const href = url === undefined ? "" : String(url);
    const resolved = resolve(href);
    if (resolved && isExternal(resolved)) {
      void Browser.OpenURL(resolved.toString());
      return null;
    }
    if (DANGEROUS_SCHEMES.some((scheme) => href.trim().toLowerCase().startsWith(scheme))) {
      return null;
    }
    return nativeOpen(url, target, features);
  }) as typeof window.open;
}
