<script lang="ts">
  let { size = 28 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
  <rect width="32" height="32" rx="8" fill="var(--accent)" />
  <path d="M10 22.5V9.5l12 13V9.5" fill="none" stroke="var(--accent-text)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
  <circle cx="10" cy="9.5" r="2.6" fill="var(--accent-text)" />
  <circle cx="22" cy="22.5" r="2.6" fill="var(--accent-text)" />
</svg>
