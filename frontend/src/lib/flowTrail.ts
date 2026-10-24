import type { Flow } from './types';

/** One step into a closure: the node holding it and which closure. */
export interface TrailStep {
  nodeId: string;
  index: number;
}

/** A breadcrumb: its label and how many steps deep it is. */
export interface Crumb {
  label: string;
  depth: number;
}

/**
 * The path from a function's control flow into the closures nested in it.
 * Immutable: navigating returns a new trail, so it can live in React state.
 */
export class FlowTrail {
  /** Starts at root, optionally already some steps into its closures. */
  constructor(
    readonly root: Flow,
    private readonly steps: readonly TrailStep[] = [],
  ) {}

  /** The flows along the path, root first; stops at a step that no longer resolves. */
  private flows(): Flow[] {
    const out = [this.root];
    for (const st of this.steps) {
      const node = out[out.length - 1].nodes.find((n) => n.id === st.nodeId);
      const next = node?.closures?.[st.index];
      if (!next) break;
      out.push(next);
    }
    return out;
  }

  /** The flow to show: the innermost closure on the path, or the root. */
  current(): Flow {
    const fs = this.flows();
    return fs[fs.length - 1];
  }

  /** How many closures deep the trail is. */
  depth(): number {
    return this.flows().length - 1;
  }

  /** Enters closure index of node nodeId in the current flow; unchanged when it doesn't exist. */
  open(nodeId: string, index: number): FlowTrail {
    const node = this.current().nodes.find((n) => n.id === nodeId);
    if (!node?.closures?.[index]) return this;
    return new FlowTrail(this.root, [...this.steps.slice(0, this.depth()), { nodeId, index }]);
  }

  /** Returns to depth (0 is the root). */
  back(depth: number): FlowTrail {
    return new FlowTrail(this.root, this.steps.slice(0, Math.max(0, depth)));
  }

  /** Breadcrumbs for the path: the function, then each closure by line. */
  crumbs(): Crumb[] {
    return this.flows().map((f, depth) => ({ label: depth === 0 ? f.name : `closure L${f.line}`, depth }));
  }
}
