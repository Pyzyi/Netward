<script lang="ts">
  let {
    checked = $bindable(false),
    label,
    hint = '',
    disabled = false,
    onchange,
  }: { checked?: boolean; label: string; hint?: string; disabled?: boolean; onchange?: (v: boolean) => void } = $props();
  const id = `t-${Math.random().toString(36).slice(2, 9)}`;
</script>

<div class="toggle-row">
  <div class="text">
    <label for={id}>{label}</label>
    {#if hint}<p class="hint">{hint}</p>{/if}
  </div>
  <button
    {id}
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label}
    {disabled}
    class="switch"
    onclick={() => {
      checked = !checked;
      onchange?.(checked);
    }}
  >
    <span class="knob"></span>
  </button>
</div>

<style>
  .toggle-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
  }
  .text {
    display: grid;
    gap: 3px;
  }
  label {
    font-weight: 600;
    cursor: pointer;
  }
  .switch {
    flex: none;
    position: relative;
    width: 40px;
    height: 22px;
    margin-top: 1px;
    border: 0;
    border-radius: 4px;
    background: var(--control-border);
    cursor: pointer;
    transition: background 150ms var(--ease);
  }
  .switch[aria-checked='true'] {
    background: var(--accent);
  }
  .switch:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .knob {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 2px;
    background: var(--surface);
    transition: transform 150ms var(--ease);
  }
  .switch[aria-checked='true'] .knob {
    transform: translateX(18px);
  }
</style>
