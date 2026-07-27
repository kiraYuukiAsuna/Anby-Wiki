let fallbackSequence = 0;

/**
 * clientUUID returns an RFC 4122 version 4 UUID in both secure and plain-HTTP
 * browser contexts. The identifiers are used for tracing, idempotency, and
 * collaboration framing; they never carry authentication or authorization.
 */
export function clientUUID(): string {
  const webCrypto = globalThis.crypto;
  if (typeof webCrypto?.randomUUID === "function") {
    return webCrypto.randomUUID();
  }

  const bytes = new Uint8Array(16);
  if (typeof webCrypto?.getRandomValues === "function") {
    webCrypto.getRandomValues(bytes);
  } else {
    // Last-resort compatibility for browsers without Web Crypto. Mix a
    // page-local sequence into Math.random so repeated calls remain distinct.
    fallbackSequence = (fallbackSequence + 1) % Number.MAX_SAFE_INTEGER;
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
    const view = new DataView(bytes.buffer);
    view.setUint32(0, Date.now() >>> 0);
    view.setUint32(4, fallbackSequence >>> 0);
  }

  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40;
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join("-");
}
