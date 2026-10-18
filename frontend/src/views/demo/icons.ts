// Icons an agent can put on demo nodes, by the names show_demo documents.
import {
  BarChart3, Bell, Box, Braces, BrickWall, Clock, Cloud, Code, Cpu, CreditCard, Database, DatabaseZap, Earth, Eye, File, Globe, HardDrive,
  KeyRound, Laptop, Layers, Lock, Mail, MapPin, Megaphone, Network, Router, Rows3, Search, Server, Settings, Shield, Smartphone,
  SquareFunction, Tablet, User, Users, Webhook, Zap, type LucideIcon,
} from 'lucide-react';

export const ICONS: Record<string, LucideIcon> = {
  shield: Shield, dns: Network, network: Network, globe: Globe, cdn: Earth, cache: DatabaseZap, database: Database, server: Server,
  servers: Layers, cloud: Cloud, laptop: Laptop, phone: Smartphone, tablet: Tablet, user: User, users: Users, lock: Lock, key: KeyRound,
  queue: Rows3, function: SquareFunction, cpu: Cpu, file: File, mail: Mail, payment: CreditCard, bell: Bell, search: Search,
  chart: BarChart3, megaphone: Megaphone, pin: MapPin, layers: Layers, disk: HardDrive, router: Router, firewall: BrickWall,
  api: Webhook, code: Code, box: Box, clock: Clock, bolt: Zap, eye: Eye, settings: Settings, json: Braces,
};

/** Icons used when a node names none, by kind. */
const BY_KIND: Record<string, string> = { datastore: 'database', queue: 'queue', actor: 'user', external: 'cloud', function: 'function', ui: 'laptop', file: 'file' };

/** The icon for a node: its own, else its kind's (tiles always get one). */
export function iconFor(icon: string | undefined, kind: string | undefined, tile = false): LucideIcon | undefined {
  return ICONS[icon ?? ''] ?? (tile ? ICONS[BY_KIND[kind ?? ''] ?? 'box'] : undefined);
}

/** Edge and flow colors an agent may ask for. */
export const PALETTE: Record<string, string> = {
  blue: 'var(--accent)', green: 'var(--ok)', red: 'var(--err)', yellow: 'var(--warn)', purple: '#a78bfa', orange: '#fb923c', gray: 'var(--text-3)',
};
