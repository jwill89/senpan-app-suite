/**
 * CodeMirror 6 configuration for the theme (custom CSS) editor.
 *
 * The original used CodeMirror 5 with a dark look applied via global CSS
 * (`.style-css-editor .CodeMirror*` / `.cm-keyword` rules). CodeMirror 6 has a
 * different DOM and styles syntax via a HighlightStyle, so we reproduce the
 * same Material-inspired palette here as a theme + highlight style. The
 * container chrome (border, sizing) is still provided by `.style-css-editor`
 * in app.css; the structural `.cm-*` rules were added to app.css section 26.
 */
import { css } from '@codemirror/lang-css'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { tags as t } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

// Editor chrome (background, text, cursor, selection, active line, gutters).
// Colors mirror the original CodeMirror 5 overrides, using the same CSS vars.
const editorTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'var(--surface)',
      color: 'var(--text)',
      fontSize: '.85rem',
    },
    '.cm-scroller': { overflow: 'auto' },
    '.cm-content': {
      fontFamily: "'Consolas', 'Monaco', 'Courier New', monospace",
      lineHeight: '1.5',
      caretColor: 'var(--text)',
    },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text)' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
      backgroundColor: 'rgba(107, 112, 92, .5)',
    },
    '.cm-activeLine': { backgroundColor: 'rgba(255, 255, 255, .04)' },
    '.cm-gutters': {
      backgroundColor: 'var(--surface2)',
      color: 'var(--text-dim)',
      border: 'none',
      borderRight: '1px solid var(--surface2)',
    },
    '.cm-activeLineGutter': { backgroundColor: 'rgba(255, 255, 255, .04)' },
    '.cm-matchingBracket': { color: 'var(--gold)', fontWeight: '700' },
  },
  { dark: true },
)

// Syntax token colors — same Material-inspired palette as the CM5 overrides.
const highlightStyle = HighlightStyle.define([
  { tag: t.keyword, color: '#c792ea' }, // @media, @import, etc.
  { tag: [t.atom, t.bool, t.number], color: '#f78c6c' }, // values/numbers
  { tag: [t.definition(t.variableName), t.definitionKeyword], color: '#82aaff' },
  { tag: t.variableName, color: 'var(--text)' },
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

/** Extensions for the custom-CSS theme editor (language + dark look). */
export const cssEditorExtensions: Extension[] = [
  css(),
  editorTheme,
  syntaxHighlighting(highlightStyle),
  EditorView.lineWrapping,
]
