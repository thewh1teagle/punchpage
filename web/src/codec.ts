/** Base64 helpers and AES-GCM sealing used for signaling and body transfer. */

export function encode(bytes: Uint8Array | ArrayBuffer): string {
  let value = '';
  const array = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  for (let i = 0; i < array.length; i += 0x8000) {
    value += String.fromCharCode(...array.subarray(i, i + 0x8000));
  }
  return btoa(value);
}

export function decode(value: string | undefined): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(value || ''), character => character.charCodeAt(0));
}

export function encodeURL(bytes: Uint8Array): string {
  return encode(bytes).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

export function decodeURL(value: string): Uint8Array<ArrayBuffer> {
  return decode(value.replaceAll('-', '+').replaceAll('_', '/') + '='.repeat((4 - (value.length % 4)) % 4));
}

/** AES-GCM box keyed by the shared secret embedded in the page fragment. */
export class SecretBox {
  private constructor(private readonly key: CryptoKey) {}

  static async import(keyText: string): Promise<SecretBox> {
    const key = await crypto.subtle.importKey('raw', decodeURL(keyText), 'AES-GCM', false, ['encrypt', 'decrypt']);
    return new SecretBox(key);
  }

  /** Encrypts and returns URL-safe base64 of nonce || ciphertext. */
  async encrypt(plaintext: Uint8Array<ArrayBuffer>): Promise<string> {
    const nonce = crypto.getRandomValues(new Uint8Array(12));
    const ciphertext = new Uint8Array(await crypto.subtle.encrypt({name: 'AES-GCM', iv: nonce}, this.key, plaintext));
    const sealed = new Uint8Array(nonce.length + ciphertext.length);
    sealed.set(nonce);
    sealed.set(ciphertext, nonce.length);
    return encodeURL(sealed);
  }

  /** Decrypts URL-safe base64 of nonce || ciphertext. */
  async decrypt(encoded: string): Promise<Uint8Array<ArrayBuffer>> {
    const sealed = decodeURL(encoded);
    const nonce = sealed.slice(0, 12);
    return new Uint8Array(await crypto.subtle.decrypt({name: 'AES-GCM', iv: nonce}, this.key, sealed.slice(12)));
  }
}
