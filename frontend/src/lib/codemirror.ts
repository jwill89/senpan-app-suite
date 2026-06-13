/**
 * CodeMirror 6 configuration for the theme (custom CSS) editor.
 *
 * The editor uses its OWN fixed colour scheme (a dark "Palenight" palette and a
 * light "GitHub"-style palette) that is deliberately INDEPENDENT of the app's
 * current theme. The CSS editor is where admins author themes, so letting the
 * active theme recolour the editor itself caused readability conflicts (e.g. a
 * dark theme's text on a dark editor). Admins pick dark or light explicitly via
 * a toggle on the Themes tab; `cssEditorExtensions(mode)` returns the matching
 * extension set. Only structural sizing rules remain in app.css section 26.
 */
import { css } from '@codemirror/lang-css'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { tags as t } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'
import { colorPicker } from '@replit/codemirror-css-color-picker'

/** Which fixed colour scheme the CSS editor renders with. */
export type EditorColorMode = 'dark' | 'light'

const MONO = "'Consolas', 'Monaco', 'Courier New', monospace"

// ── Dark scheme (Material "Palenight", fixed) ───────────────────────────────
const darkTheme = EditorView.theme(
  {
    '&': { backgroundColor: '#292d3e', color: '#bfc7d5', fontSize: '.85rem' },
    '.cm-scroller': { overflow: 'auto' },
    '.cm-content': { fontFamily: MONO, lineHeight: '1.5', caretColor: '#ffcb6b' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#ffcb6b' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'rgba(113, 124, 180, .35)',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(255, 255, 255, .04)' },
    '.cm-gutters': {
      backgroundColor: '#232635',
      color: '#5b6273',
      border: 'none',
      borderRight: '1px solid #3a3f58',
    },
    '.cm-activeLineGutter': { backgroundColor: 'rgba(255, 255, 255, .05)' },
    '.cm-matchingBracket': { color: '#ffcb6b', fontWeight: '700' },
  },
  { dark: true },
)

const darkHighlight = HighlightStyle.define([
  { tag: t.keyword, color: '#c792ea' }, // @media, @import, etc.
  { tag: [t.atom, t.bool, t.number], color: '#f78c6c' }, // values/numbers
  { tag: [t.definition(t.variableName), t.definitionKeyword], color: '#82aaff' },
  { tag: t.variableName, color: '#bfc7d5' },
  { tag: t.propertyName, color: '#c3e88d' }, // CSS properties
  { tag: [t.className, t.attributeName], color: '#ffcb6b' }, // selectors/attrs
  { tag: [t.tagName, t.typeName], color: '#f07178' }, // tag names
  { tag: [t.function(t.variableName), t.standard(t.variableName)], color: '#ffcb6b' },
  { tag: [t.string, t.special(t.string)], color: '#c3e88d' },
  { tag: t.comment, color: '#676e95', fontStyle: 'italic' },
  { tag: [t.meta, t.documentMeta], color: '#ffcb6b' },
  { tag: t.invalid, color: '#ff5370' },
  { tag: t.color, color: '#f78c6c' },
])

// ── Light scheme (GitHub-style, fixed) ──────────────────────────────────────
const lightTheme = EditorView.theme(
  {
    '&': { backgroundColor: '#ffffff', color: '#24292e', fontSize: '.85rem' },
    '.cm-scroller': { overflow: 'auto' },
    '.cm-content': { fontFamily: MONO, lineHeight: '1.5', caretColor: '#24292e' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#24292e' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'rgba(3, 102, 214, .15)',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(0, 0, 0, .035)' },
    '.cm-gutters': {
      backgroundColor: '#f6f8fa',
      color: '#9aa0a6',
      border: 'none',
      borderRight: '1px solid #e1e4e8',
    },
    '.cm-activeLineGutter': { backgroundColor: 'rgba(0, 0, 0, .05)' },
    '.cm-matchingBracket': { color: '#0b7285', fontWeight: '700' },
  },
  { dark: false },
)

const lightHighlight = HighlightStyle.define([
  { tag: t.keyword, color: '#8200b3' }, // @media, @import, etc.
  { tag: [t.atom, t.bool, t.number], color: '#b35900' }, // values/numbers
  { tag: [t.definition(t.variableName), t.definitionKeyword], color: '#0b5cad' },
  { tag: t.variableName, color: '#24292e' },
  { tag: t.propertyName, color: '#116329' }, // CSS properties
  { tag: [t.className, t.attributeName], color: '#8a6d00' }, // selectors/attrs
  { tag: [t.tagName, t.typeName], color: '#b3261e' }, // tag names
  { tag: [t.function(t.variableName), t.standard(t.variableName)], color: '#8a6d00' },
  { tag: [t.string, t.special(t.string)], color: '#116329' },
  { tag: t.comment, color: '#6a737d', fontStyle: 'italic' },
  { tag: [t.meta, t.documentMeta], color: '#8a6d00' },
  { tag: t.invalid, color: '#b3261e' },
  { tag: t.color, color: '#b35900' },
])

// Shared, theme-agnostic extensions: the CSS language, line wrapping, and the
// inline colour-swatch picker (clicking a swatch opens a native picker).
const base: Extension[] = [css(), EditorView.lineWrapping, colorPicker]

/**
 * Returns the extensions for the custom-CSS theme editor in the chosen fixed
 * colour mode (dark/light), independent of the app's active theme.
 */
export function cssEditorExtensions(mode: EditorColorMode): Extension[] {
  return mode === 'light'
    ? [...base, lightTheme, syntaxHighlighting(lightHighlight)]
    : [...base, darkTheme, syntaxHighlighting(darkHighlight)]
}
