<script lang="ts">
  let { points, height = 36, color = 'var(--signal)' }: { points: { t: number; ms: number }[]; height?: number; color?: string } = $props();

  const W = 240;
  const path = $derived.by(() => {
    const ok = points.filter((p) => p.ms >= 0);
    if (ok.length < 2) return { line: '', fails: [] };
    const t0 = points[0].t;
    const t1 = points[points.length - 1].t;
    const max = Math.max(...ok.map((p) => p.ms), 10) * 1.15;
    const x = (t: number) => ((t - t0) / Math.max(1, t1 - t0)) * W;
    const y = (ms: number) => height - 2 - (ms / max) * (height - 4);
    let d = '';
    let pen = false;
    for (const p of points) {
      if (p.ms < 0) {
        pen = false;
        continue;
      }
      d += `${pen ? 'L' : 'M'}${x(p.t).toFixed(1)} ${y(p.ms).toFixed(1)}`;
      pen = true;
    }
    // Runs of failed probes become one segment on the baseline, at least 3 wide.
    const fails: { x: number; w: number }[] = [];
    let start = -1;
    points.forEach((p, i) => {
      if (p.ms < 0 && start < 0) start = i;
      if (start >= 0 && (p.ms >= 0 || i === points.length - 1)) {
        const end = p.ms >= 0 ? i : i + 1;
        const x0 = x(points[start].t);
        const x1 = end < points.length ? x(points[end].t) : W;
        const w = Math.max(3, x1 - x0);
        fails.push({ x: Math.min(W - w, Math.max(0, x0)), w });
        start = -1;
      }
    });
    return { line: d, fails };
  });
</script>

<svg viewBox="0 0 {W} {height}" preserveAspectRatio="none" width="100%" {height} aria-hidden="true">
  {#each path.fails as f, i (i)}
    <rect x={f.x} y={height - 3} width={f.w} height="3" fill="#ff7b6e" />
  {/each}
  <path d={path.line} fill="none" stroke={color} stroke-width="1.5" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
</svg>
