/**
 * Generate a secure encryption key for signing cookies
 * @returns A base64-encoded 32-byte key suitable for cookie signing
 */
export function generateEncryptionKey(): string {
  // Generate 32 random bytes using Web Crypto API
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);

  // Convert to base64
  const key = btoa(String.fromCharCode(...array));
  return key;
}
