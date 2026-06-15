import { config } from '@vue/test-utils'

// <font-awesome-icon> is registered globally in main.ts (see app.component), so
// it is never imported by the components under test. Tests don't run main.ts, so
// stub it for every mount here — this avoids "Failed to resolve component"
// warnings and gives a stable `.fa-stub` element to assert against when a
// component renders an icon.
config.global.stubs = {
  ...config.global.stubs,
  'font-awesome-icon': { props: ['icon'], template: '<i class="fa-stub"></i>' },
}
