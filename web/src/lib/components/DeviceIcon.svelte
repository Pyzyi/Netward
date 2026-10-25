<script lang="ts">
  import { category, typeIcons } from '../devices';
  import type { DeviceType } from '../types';

  let { type, online = undefined, size = 40 }: { type: DeviceType; online?: boolean; size?: number } = $props();
  const Icon = $derived(typeIcons[type] ?? typeIcons.unknown);
</script>

<span class="icon cat-{category(type)}" style:width="{size}px" style:height="{size}px" aria-hidden="true">
  <Icon size={Math.round(size * 0.5)} strokeWidth={1.75} />
  {#if online !== undefined}
    <span class="dot" class:on={online}></span>
  {/if}
</span>

<style>
  .icon {
    position: relative;
    display: inline-grid;
    place-items: center;
    flex: none;
    border-radius: 50%;
    color: var(--cat);
    background: color-mix(in srgb, var(--cat) 12%, var(--surface));
  }
  .dot {
    position: absolute;
    right: -1px;
    bottom: -1px;
    width: 30%;
    height: 30%;
    max-width: 13px;
    max-height: 13px;
    border-radius: 50%;
    background: var(--off);
    border: 2.5px solid var(--surface);
  }
  .dot.on {
    background: var(--ok);
  }
</style>
