// The standard CSS rules, in the .vue files' style blocks as well. Prettier formats;
// this catches what formatting cannot (an unknown property, a rule shadowed by one
// before it, an @import browsers would ignore). The project's own CSS conventions -
// naming, the scoped-CSS ratchet, the theme tokens - are scripts/check-conventions.mjs.
// A rule is turned off here with its reason, or at the one line it does not fit with
// the reason beside it.
export default {
  extends: ['stylelint-config-standard', 'stylelint-config-recommended-vue'],
  rules: {
    // app.css imports its parts by string, which Vite resolves and bundles.
    'import-notation': 'string',
    // AGENTS.md "CSS conventions": kebab-case, a part joined with `-`, a modifier
    // with `--`, and no BEM `__`. The libraries the stylesheets reskin keep their
    // own names: vue-color's BEM classes (`vc-input__input`) and ProseMirror's.
    'selector-class-pattern': [
      '^(?:vc-[a-z0-9-]+(?:__[a-z0-9-]+)?|ProseMirror(?:-[a-z]+)*|[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?:--[a-z0-9]+(?:-[a-z0-9]+)*)?)$',
      { message: 'Name classes in kebab-case, a modifier after --, no __ (AGENTS.md)' },
    ],
    'custom-property-pattern': [
      '^[a-z][a-z0-9]*(-[a-z0-9]+)*$',
      { message: 'Name custom properties in kebab-case' },
    ],
    // Off: the stylesheets are ordered by object and domain on purpose (app.css sets
    // the cascade between files), and this rule pairs selectors from unrelated parts
    // of the app that merely end on the same element - a login form's `input` and a
    // raffle form's `input`. Satisfying it would mean ordering by specificity rather
    // than by what each rule describes.
    'no-descending-specificity': null,
    // The theme's tokens sit in groups, a blank line between one group and the next.
    'custom-property-empty-line-before': null,
    // A typeface's name is written as the foundry writes it.
    'value-keyword-case': ['lower', { ignoreProperties: ['/^--font-/', 'font-family'] }],
  },
}
