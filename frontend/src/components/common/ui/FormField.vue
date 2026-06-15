<script setup lang="ts">
/**
 * One labelled form control. Wraps the control (default slot) with a label and
 * optional help text, giving every admin form the same field rhythm. Controls
 * placed in the default slot stretch full-width automatically (see the `.field`
 * rule in app.css), replacing the old per-input `.field-input-full` class.
 *
 * The label can be passed as the `label` prop or, when it needs markup, via the
 * `#label` slot. Help text likewise supports a `help` prop (plain string) or a
 * `#help` slot for markup such as links.
 */
defineProps<{
  label?: string
  /** Marks the field required: appends a `*` to the label. */
  required?: boolean
  /** Plain-text help shown beneath the control. Use the #help slot for markup. */
  help?: string
  /** Associates the label with a control via `for`/`id`. */
  htmlFor?: string
}>()
</script>

<template>
  <div class="field">
    <label v-if="label || $slots.label" class="field-label" :for="htmlFor">
      <slot name="label">{{ label }}</slot
      ><span v-if="required" class="req" aria-hidden="true">*</span>
    </label>
    <slot />
    <small v-if="$slots.help || help" class="field-help">
      <slot name="help">{{ help }}</slot>
    </small>
  </div>
</template>
