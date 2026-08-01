import {SimplePool} from 'nostr-tools/pool';
import {finalizeEvent, generateSecretKey, getPublicKey} from 'nostr-tools/pure';
import type {SecretBox} from './codec';
import type {Signal, SignalEnvelope} from './protocol';

const SIGNAL_KIND = 24242;

export interface SignalingOptions {
  relays: string[];
  room: string;
  peer: string;
  box: SecretBox;
}

/** Publishes and receives encrypted WebRTC signals through Nostr relays. */
export class Signaling {
  private readonly pool = new SimplePool({enableReconnect: true});
  private readonly secret = generateSecretKey();
  private readonly pubkey = getPublicKey(this.secret);
  private readonly seen = new Set<string>();

  constructor(private readonly options: SignalingOptions) {}

  /** Subscribes to host signals addressed to this peer. */
  subscribe(onSignal: (signal: Signal) => Promise<void> | void): void {
    const {relays, room, peer, box} = this.options;
    this.pool.subscribeMany(relays, {kinds: [SIGNAL_KIND], '#d': [room], since: Math.floor(Date.now() / 1000) - 30}, {
      onevent: async event => {
        if (event.pubkey === this.pubkey || this.seen.has(event.id)) return;
        this.seen.add(event.id);
        try {
          const payload = JSON.parse(new TextDecoder().decode(await box.decrypt(event.content))) as SignalEnvelope;
          if (payload.role === 'host' && payload.peer === peer) await onSignal(payload.signal);
        } catch {
          // Ignore events we cannot decrypt or parse; they are not for us.
        }
      }
    });
  }

  /** Encrypts and publishes a signal; resolves once any relay accepts it. */
  async publish(signal: Signal): Promise<void> {
    const {relays, room, peer, box} = this.options;
    const envelope: SignalEnvelope = {role: 'browser', peer, signal};
    const content = await box.encrypt(new TextEncoder().encode(JSON.stringify(envelope)));
    const event = finalizeEvent(
      {kind: SIGNAL_KIND, created_at: Math.floor(Date.now() / 1000), tags: [['d', room]], content},
      this.secret
    );
    await Promise.any(this.pool.publish(relays, event));
  }
}
