import { config } from '@vue/test-utils'

// <font-awesome-icon> is registered globally in main.ts (see app.component), so
// it is never imported by the components under test. Tests don't run main.ts, so
// stub it for every mount here - this avoids "Failed to resolve component"
// warnings and gives a stable `.fa-stub` element to assert against when a
// component renders an icon.
//
// `data-icon` carries the icon NAME (the second element of the `[prefix, name]`
// tuple), so a test can assert WHICH icon rendered - e.g. that a sort header shows
// `chevron-up` rather than `chevron-down` - without casting an untyped
// findComponent() wrapper to reach its props.
config.global.stubs = {
  ...config.global.stubs,
  'font-awesome-icon': {
    props: ['icon'],
    template: '<i class="fa-stub" :data-icon="iconName"></i>',
    computed: {
      iconName(): string {
        // Icons are passed as `[prefix, name]` (occasionally a bare name string).
        const icon = (this as unknown as { icon?: string | string[] }).icon
        if (Array.isArray(icon)) return icon[icon.length - 1] ?? ''
        return icon ?? ''
      },
    },
  },
}
