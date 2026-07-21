#!/usr/bin/env php
<?php
/**
 * insert_styles.php — insert the Senpan theme set (14 concepts x light/dark = 28
 * styles) into the App Suite SQLite database.
 *
 * Every theme has been verified WCAG 2.1 AAA-compliant against the same token
 * pairs the app's theme editor / themetool check (7:1 normal text, 4.5:1 large).
 *
 * Each style is written as a row in the `styles` table with its design tokens
 * JSON-encoded into the `tokens` column (the same shape the Go server reads); the
 * applied stylesheet is generated from those tokens at runtime.
 *
 * Idempotent: a theme whose name already exists is skipped, so re-running is safe.
 * All inserts run in a single transaction (all-or-nothing).
 *
 * USAGE:
 *   php insert_styles.php /opt/senpan/data/database.sqlite
 *
 * Back up the DB first (e.g. cp database.sqlite database.sqlite.bak) — this writes
 * to the live styles table. Stop the service or run during a quiet window if you
 * want to avoid writing while the app holds a connection.
 */

// ── Embedded theme definitions (verified WCAG AAA) ──────────────────────────
const THEMES_JSON = <<<'JSON'
[
  {
    "name": "Summer Beachy Blues (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(6, 50, 63, .3)",
      "page-bg": "#a7d8ea",
      "panel-bg": "#e6f4fb",
      "panel-raised-bg": "#c7e7f2",
      "control-border": "#8ec6da",
      "input-bg": "#f5fbfe",
      "accent": "#4a1c0a",
      "accent-hover": "#3a1507",
      "accent-2": "#0b5570",
      "accent-2-hover": "#083f52",
      "highlight": "#06323f",
      "text": "#0a2b39",
      "text-muted": "#1c3d4a",
      "text-on-accent": "#f4fbfd",
      "text-on-fill": "#f4fbfd",
      "board-cell-bg": "#0f4a62",
      "board-cell-hover-bg": "#135d79",
      "board-free-bg": "#b0451c",
      "board-gradient-start": "#f3e4c6",
      "board-gradient-end": "#e6d0a2"
    }
  },
  {
    "name": "Summer Beachy Blues (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(132, 212, 232, .45)",
      "page-bg": "#08191f",
      "panel-bg": "#0e252e",
      "panel-raised-bg": "#123039",
      "control-border": "#244a5a",
      "input-bg": "#0a1f28",
      "accent": "#f1b48a",
      "accent-hover": "#f7c8a4",
      "accent-2": "#0b5570",
      "accent-2-hover": "#083f52",
      "highlight": "#84d4e8",
      "text": "#e9f5fa",
      "text-muted": "#b6d4de",
      "text-on-accent": "#06171f",
      "text-on-fill": "#f2fafd",
      "board-cell-bg": "#e7d8bc",
      "board-cell-hover-bg": "#f0e5cd",
      "board-free-bg": "#f0a878",
      "board-gradient-start": "#0e2833",
      "board-gradient-end": "#091d25"
    }
  },
  {
    "name": "Spring Sakura Pinks & Greens (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(106, 20, 54, .28)",
      "page-bg": "#f6d6e0",
      "panel-bg": "#fdeef3",
      "panel-raised-bg": "#f7dbe4",
      "control-border": "#e19db4",
      "input-bg": "#fef6f9",
      "accent": "#2c4a1a",
      "accent-hover": "#233c14",
      "accent-2": "#3a5a24",
      "accent-2-hover": "#2e491b",
      "highlight": "#6a1436",
      "text": "#3a1226",
      "text-muted": "#4a1a2e",
      "text-on-accent": "#f6f2e6",
      "text-on-fill": "#f4f8ec",
      "board-cell-bg": "#7a1540",
      "board-cell-hover-bg": "#8f1c4c",
      "board-free-bg": "#2f5220",
      "board-gradient-start": "#e7f0d4",
      "board-gradient-end": "#d6e6bd"
    }
  },
  {
    "name": "Spring Sakura Pinks & Greens (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(169, 224, 138, .4)",
      "page-bg": "#1c0f16",
      "panel-bg": "#281521",
      "panel-raised-bg": "#341b2b",
      "control-border": "#5a3247",
      "input-bg": "#22111c",
      "accent": "#f2a9c4",
      "accent-hover": "#f7bdd2",
      "accent-2": "#3a5a24",
      "accent-2-hover": "#2e491b",
      "highlight": "#a9e08a",
      "text": "#f7e2ea",
      "text-muted": "#dcb6c7",
      "text-on-accent": "#1a0a12",
      "text-on-fill": "#f3faec",
      "board-cell-bg": "#f0cad9",
      "board-cell-hover-bg": "#f7dbe6",
      "board-free-bg": "#bfe6a6",
      "board-gradient-start": "#2b1723",
      "board-gradient-end": "#1f0f18"
    }
  },
  {
    "name": "Autumn Leaves (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(106, 36, 8, .3)",
      "page-bg": "#f0cfa0",
      "panel-bg": "#faecd4",
      "panel-raised-bg": "#f0dab4",
      "control-border": "#dcb488",
      "input-bg": "#fdf5e8",
      "accent": "#5a1e08",
      "accent-hover": "#481806",
      "accent-2": "#7a3410",
      "accent-2-hover": "#5f280c",
      "highlight": "#6a2408",
      "text": "#3a1c08",
      "text-muted": "#4c2810",
      "text-on-accent": "#fdf3e2",
      "text-on-fill": "#fdf3e2",
      "board-cell-bg": "#7a2e0c",
      "board-cell-hover-bg": "#8f3a12",
      "board-free-bg": "#8a5a10",
      "board-gradient-start": "#f5e3c2",
      "board-gradient-end": "#ecd0a0"
    }
  },
  {
    "name": "Autumn Leaves (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(242, 180, 92, .42)",
      "page-bg": "#1a0f07",
      "panel-bg": "#26160b",
      "panel-raised-bg": "#321f10",
      "control-border": "#5a3a1e",
      "input-bg": "#1f1109",
      "accent": "#f0a850",
      "accent-hover": "#f6bd6e",
      "accent-2": "#7a3410",
      "accent-2-hover": "#5f280c",
      "highlight": "#f2b45c",
      "text": "#f7e6cc",
      "text-muted": "#dcbb8c",
      "text-on-accent": "#1a0d05",
      "text-on-fill": "#fdf1de",
      "board-cell-bg": "#eccfa0",
      "board-cell-hover-bg": "#f5dcb2",
      "board-free-bg": "#e08a3a",
      "board-gradient-start": "#2a1810",
      "board-gradient-end": "#1d0f08"
    }
  },
  {
    "name": "Winter Colors (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(16, 31, 58, .28)",
      "page-bg": "#cfe0ee",
      "panel-bg": "#eef4fb",
      "panel-raised-bg": "#dce8f4",
      "control-border": "#94b0cc",
      "input-bg": "#f7fafd",
      "accent": "#14284a",
      "accent-hover": "#0e1f3c",
      "accent-2": "#2a4c74",
      "accent-2-hover": "#1f3a5c",
      "highlight": "#101f3a",
      "text": "#101f36",
      "text-muted": "#1e3050",
      "text-on-accent": "#f2f7fc",
      "text-on-fill": "#f2f7fc",
      "board-cell-bg": "#1b3a64",
      "board-cell-hover-bg": "#234878",
      "board-free-bg": "#0f4a5a",
      "board-gradient-start": "#e4eef8",
      "board-gradient-end": "#cfe0ee"
    }
  },
  {
    "name": "Winter Colors (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(174, 224, 242, .42)",
      "page-bg": "#0a121c",
      "panel-bg": "#111c2a",
      "panel-raised-bg": "#182636",
      "control-border": "#33506e",
      "input-bg": "#0d1622",
      "accent": "#9fd0ee",
      "accent-hover": "#bce0f5",
      "accent-2": "#2a4c74",
      "accent-2-hover": "#1f3a5c",
      "highlight": "#aee0f2",
      "text": "#e8f2fb",
      "text-muted": "#b3ccdf",
      "text-on-accent": "#08111c",
      "text-on-fill": "#f0f7fc",
      "board-cell-bg": "#d4e6f2",
      "board-cell-hover-bg": "#e6f1f9",
      "board-free-bg": "#7ec4d6",
      "board-gradient-start": "#101d2c",
      "board-gradient-end": "#0a121c"
    }
  },
  {
    "name": "Mysticism & Magic (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(46, 14, 86, .3)",
      "page-bg": "#ddd0ee",
      "panel-bg": "#f1eafa",
      "panel-raised-bg": "#e4d8f3",
      "control-border": "#c3aede",
      "input-bg": "#f8f4fd",
      "accent": "#3a1466",
      "accent-hover": "#2e0f52",
      "accent-2": "#4a2a7a",
      "accent-2-hover": "#3a2062",
      "highlight": "#2e0e56",
      "text": "#26123f",
      "text-muted": "#361c54",
      "text-on-accent": "#f4eefc",
      "text-on-fill": "#f4eefc",
      "board-cell-bg": "#3a1a6a",
      "board-cell-hover-bg": "#472280",
      "board-free-bg": "#7a4a12",
      "board-gradient-start": "#ece2f6",
      "board-gradient-end": "#ddd0ee"
    }
  },
  {
    "name": "Mysticism & Magic (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(216, 180, 245, .45)",
      "page-bg": "#100a1e",
      "panel-bg": "#1a1030",
      "panel-raised-bg": "#241640",
      "control-border": "#453066",
      "input-bg": "#140c26",
      "accent": "#c4a6f2",
      "accent-hover": "#d4bcf7",
      "accent-2": "#4a2a7a",
      "accent-2-hover": "#3a2062",
      "highlight": "#d8b4f5",
      "text": "#ece2fb",
      "text-muted": "#c3adde",
      "text-on-accent": "#0f0820",
      "text-on-fill": "#f2ecfb",
      "board-cell-bg": "#ddccf5",
      "board-cell-hover-bg": "#eaddfb",
      "board-free-bg": "#e0b24e",
      "board-gradient-start": "#181030",
      "board-gradient-end": "#100a1e"
    }
  },
  {
    "name": "La Noscea (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(10, 48, 56, .3)",
      "page-bg": "#a8ddda",
      "panel-bg": "#e4f5f3",
      "panel-raised-bg": "#c8e9e6",
      "control-border": "#96d0cb",
      "input-bg": "#f4fcfb",
      "accent": "#0c3a3a",
      "accent-hover": "#092d2d",
      "accent-2": "#0f5060",
      "accent-2-hover": "#0b3f4c",
      "highlight": "#0a3038",
      "text": "#0a3030",
      "text-muted": "#164040",
      "text-on-accent": "#f2fbfa",
      "text-on-fill": "#f2fbfa",
      "board-cell-bg": "#0e4b4b",
      "board-cell-hover-bg": "#125e5e",
      "board-free-bg": "#9a5216",
      "board-gradient-start": "#eef0d8",
      "board-gradient-end": "#dce0bd"
    }
  },
  {
    "name": "La Noscea (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(134, 220, 206, .42)",
      "page-bg": "#071817",
      "panel-bg": "#0d2523",
      "panel-raised-bg": "#123130",
      "control-border": "#285450",
      "input-bg": "#091f1d",
      "accent": "#79d6c8",
      "accent-hover": "#98e2d7",
      "accent-2": "#0f5060",
      "accent-2-hover": "#0b3f4c",
      "highlight": "#86dcce",
      "text": "#e2f5f2",
      "text-muted": "#a9d2cb",
      "text-on-accent": "#051512",
      "text-on-fill": "#eefbf8",
      "board-cell-bg": "#d4e8c8",
      "board-cell-hover-bg": "#e4f2da",
      "board-free-bg": "#e0a24a",
      "board-gradient-start": "#0d2624",
      "board-gradient-end": "#071817"
    }
  },
  {
    "name": "Thanalan (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(94, 28, 6, .3)",
      "page-bg": "#f0c9a0",
      "panel-bg": "#fbe8d2",
      "panel-raised-bg": "#f2d4b2",
      "control-border": "#e0b184",
      "input-bg": "#fdf3e6",
      "accent": "#6a1c0a",
      "accent-hover": "#551607",
      "accent-2": "#8a2c10",
      "accent-2-hover": "#6e220c",
      "highlight": "#5e1c06",
      "text": "#3e1a0a",
      "text-muted": "#52240e",
      "text-on-accent": "#fdf1e0",
      "text-on-fill": "#fdf1e0",
      "board-cell-bg": "#7c2a0e",
      "board-cell-hover-bg": "#923614",
      "board-free-bg": "#8a5a0c",
      "board-gradient-start": "#f6e2bc",
      "board-gradient-end": "#eccf98"
    }
  },
  {
    "name": "Thanalan (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(242, 162, 78, .42)",
      "page-bg": "#1c0d06",
      "panel-bg": "#28140a",
      "panel-raised-bg": "#341d0f",
      "control-border": "#5e3820",
      "input-bg": "#210f07",
      "accent": "#f0b24e",
      "accent-hover": "#f6c66e",
      "accent-2": "#8a2c10",
      "accent-2-hover": "#6e220c",
      "highlight": "#f2a24e",
      "text": "#f9e6c8",
      "text-muted": "#deb888",
      "text-on-accent": "#1a0b05",
      "text-on-fill": "#fdefd8",
      "board-cell-bg": "#eccb98",
      "board-cell-hover-bg": "#f5daac",
      "board-free-bg": "#e07a2e",
      "board-gradient-start": "#2c160c",
      "board-gradient-end": "#1c0d06"
    }
  },
  {
    "name": "The Shroud (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(32, 58, 16, .3)",
      "page-bg": "#c3ddab",
      "panel-bg": "#e9f3da",
      "panel-raised-bg": "#d5e8bd",
      "control-border": "#b0cf94",
      "input-bg": "#f5faee",
      "accent": "#1e3a0e",
      "accent-hover": "#172e0a",
      "accent-2": "#3a5418",
      "accent-2-hover": "#2e4413",
      "highlight": "#203a10",
      "text": "#1c3010",
      "text-muted": "#2a3f18",
      "text-on-accent": "#f2f6e6",
      "text-on-fill": "#f2f6e6",
      "board-cell-bg": "#284d14",
      "board-cell-hover-bg": "#33601a",
      "board-free-bg": "#7a4a10",
      "board-gradient-start": "#eef2d6",
      "board-gradient-end": "#dce6bd"
    }
  },
  {
    "name": "The Shroud (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(176, 220, 132, .42)",
      "page-bg": "#0d1707",
      "panel-bg": "#16230d",
      "panel-raised-bg": "#1f3013",
      "control-border": "#354c20",
      "input-bg": "#101c0a",
      "accent": "#a8d878",
      "accent-hover": "#bfe494",
      "accent-2": "#3a5418",
      "accent-2-hover": "#2e4413",
      "highlight": "#b0dc84",
      "text": "#e8f4d6",
      "text-muted": "#bcd29c",
      "text-on-accent": "#0b1405",
      "text-on-fill": "#eff8e0",
      "board-cell-bg": "#d6e8bc",
      "board-cell-hover-bg": "#e6f2d0",
      "board-free-bg": "#d69a3e",
      "board-gradient-start": "#14210c",
      "board-gradient-end": "#0d1707"
    }
  },
  {
    "name": "Ishgard (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(32, 43, 60, .28)",
      "page-bg": "#cdd4de",
      "panel-bg": "#eef1f5",
      "panel-raised-bg": "#dce1e9",
      "control-border": "#adb8c8",
      "input-bg": "#f7f9fb",
      "accent": "#1e2c40",
      "accent-hover": "#172233",
      "accent-2": "#374a63",
      "accent-2-hover": "#2c3c52",
      "highlight": "#202b3c",
      "text": "#1a2432",
      "text-muted": "#2a3444",
      "text-on-accent": "#f1f4f8",
      "text-on-fill": "#f1f4f8",
      "board-cell-bg": "#28374e",
      "board-cell-hover-bg": "#33445e",
      "board-free-bg": "#5a3a1a",
      "board-gradient-start": "#e6e9ef",
      "board-gradient-end": "#cdd4de"
    }
  },
  {
    "name": "Ishgard (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(188, 208, 230, .42)",
      "page-bg": "#0e131b",
      "panel-bg": "#171e29",
      "panel-raised-bg": "#202936",
      "control-border": "#3a4658",
      "input-bg": "#111823",
      "accent": "#aec2dc",
      "accent-hover": "#c6d6ea",
      "accent-2": "#374a63",
      "accent-2-hover": "#2c3c52",
      "highlight": "#bcd0e6",
      "text": "#e6ecf4",
      "text-muted": "#b0bccd",
      "text-on-accent": "#0c1119",
      "text-on-fill": "#eef3f9",
      "board-cell-bg": "#d0dae8",
      "board-cell-hover-bg": "#e2e9f2",
      "board-free-bg": "#a87a4e",
      "board-gradient-start": "#141b26",
      "board-gradient-end": "#0e131b"
    }
  },
  {
    "name": "Sharlayan (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(10, 52, 58, .3)",
      "page-bg": "#dfead0",
      "panel-bg": "#f4f7e8",
      "panel-raised-bg": "#e8eed6",
      "control-border": "#b0cdb8",
      "input-bg": "#fbfcf4",
      "accent": "#0c3a40",
      "accent-hover": "#092d32",
      "accent-2": "#12565a",
      "accent-2-hover": "#0e4448",
      "highlight": "#0a343a",
      "text": "#123030",
      "text-muted": "#1e4040",
      "text-on-accent": "#f4faf4",
      "text-on-fill": "#f4faf4",
      "board-cell-bg": "#0f4c4c",
      "board-cell-hover-bg": "#135e5e",
      "board-free-bg": "#7a5810",
      "board-gradient-start": "#f2f0d4",
      "board-gradient-end": "#e2e2ba"
    }
  },
  {
    "name": "Sharlayan (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(224, 200, 120, .42)",
      "page-bg": "#0a1614",
      "panel-bg": "#10221f",
      "panel-raised-bg": "#162e2a",
      "control-border": "#2a524c",
      "input-bg": "#0c1c19",
      "accent": "#6fd4c4",
      "accent-hover": "#8ee0d3",
      "accent-2": "#12565a",
      "accent-2-hover": "#0e4448",
      "highlight": "#e0c878",
      "text": "#e4f4ee",
      "text-muted": "#a8d0c6",
      "text-on-accent": "#06140f",
      "text-on-fill": "#f0faf4",
      "board-cell-bg": "#e6e0bc",
      "board-cell-hover-bg": "#f0eccf",
      "board-free-bg": "#5ec6b4",
      "board-gradient-start": "#0f221f",
      "board-gradient-end": "#0a1614"
    }
  },
  {
    "name": "Doma & Hingashi (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(90, 16, 24, .3)",
      "page-bg": "#e2dcc6",
      "panel-bg": "#f5f1e2",
      "panel-raised-bg": "#e9e3cd",
      "control-border": "#c8b8a6",
      "input-bg": "#fbf9f0",
      "accent": "#1a2452",
      "accent-hover": "#141b40",
      "accent-2": "#7a1420",
      "accent-2-hover": "#5f0f19",
      "highlight": "#5a1018",
      "text": "#242018",
      "text-muted": "#3a2e1c",
      "text-on-accent": "#f6f2e2",
      "text-on-fill": "#f6f2e2",
      "board-cell-bg": "#1f2a5c",
      "board-cell-hover-bg": "#28356e",
      "board-free-bg": "#8a1420",
      "board-gradient-start": "#f2ecd6",
      "board-gradient-end": "#e2dcc0"
    }
  },
  {
    "name": "Doma & Hingashi (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(240, 154, 164, .4)",
      "page-bg": "#0e0f1c",
      "panel-bg": "#161829",
      "panel-raised-bg": "#1f2138",
      "control-border": "#3c3e5c",
      "input-bg": "#111220",
      "accent": "#e0b45a",
      "accent-hover": "#ecc878",
      "accent-2": "#7a1420",
      "accent-2-hover": "#5f0f19",
      "highlight": "#f09aa4",
      "text": "#eee6d4",
      "text-muted": "#c8b294",
      "text-on-accent": "#141006",
      "text-on-fill": "#f6efdc",
      "board-cell-bg": "#e6d4a8",
      "board-cell-hover-bg": "#f0e2bc",
      "board-free-bg": "#e05860",
      "board-gradient-start": "#151626",
      "board-gradient-end": "#0e0f1c"
    }
  },
  {
    "name": "Radz-at-Han (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(84, 16, 74, .3)",
      "page-bg": "#f2d8a4",
      "panel-bg": "#fbeed2",
      "panel-raised-bg": "#f4ddb4",
      "control-border": "#dcb87c",
      "input-bg": "#fdf6e6",
      "accent": "#5a1050",
      "accent-hover": "#480c40",
      "accent-2": "#0e5a5a",
      "accent-2-hover": "#0b4747",
      "highlight": "#54104a",
      "text": "#3a0e34",
      "text-muted": "#4c1c40",
      "text-on-accent": "#f8eef4",
      "text-on-fill": "#eefaf8",
      "board-cell-bg": "#6a1460",
      "board-cell-hover-bg": "#7e1a72",
      "board-free-bg": "#0e5a52",
      "board-gradient-start": "#f6e6c0",
      "board-gradient-end": "#eed09a"
    }
  },
  {
    "name": "Radz-at-Han (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(240, 160, 216, .42)",
      "page-bg": "#150a16",
      "panel-bg": "#201029",
      "panel-raised-bg": "#2c163a",
      "control-border": "#523a5e",
      "input-bg": "#190b20",
      "accent": "#f0b44e",
      "accent-hover": "#f6c86e",
      "accent-2": "#0e5a5a",
      "accent-2-hover": "#0b4747",
      "highlight": "#f0a0d8",
      "text": "#f6e2f0",
      "text-muted": "#d2b0c8",
      "text-on-accent": "#160a16",
      "text-on-fill": "#eefaf8",
      "board-cell-bg": "#f0c8e4",
      "board-cell-hover-bg": "#f6daee",
      "board-free-bg": "#3ec2b4",
      "board-gradient-start": "#1e0f26",
      "board-gradient-end": "#140a19"
    }
  },
  {
    "name": "Tuliyollal (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(12, 66, 64, .3)",
      "page-bg": "#f4d8a2",
      "panel-bg": "#fceed0",
      "panel-raised-bg": "#f6ddb0",
      "control-border": "#e6be8a",
      "input-bg": "#fef6e4",
      "accent": "#0e4a48",
      "accent-hover": "#0a3a38",
      "accent-2": "#7a3410",
      "accent-2-hover": "#612810",
      "highlight": "#0c4240",
      "text": "#123a34",
      "text-muted": "#1a3e36",
      "text-on-accent": "#f2fbf9",
      "text-on-fill": "#fdf1e2",
      "board-cell-bg": "#0f5450",
      "board-cell-hover-bg": "#136662",
      "board-free-bg": "#9a4212",
      "board-gradient-start": "#f8e6bc",
      "board-gradient-end": "#f0d296"
    }
  },
  {
    "name": "Tuliyollal (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(240, 184, 80, .42)",
      "page-bg": "#1a0f06",
      "panel-bg": "#26160a",
      "panel-raised-bg": "#331f0f",
      "control-border": "#5c3e28",
      "input-bg": "#201007",
      "accent": "#4ecdb8",
      "accent-hover": "#72dcca",
      "accent-2": "#7a3410",
      "accent-2-hover": "#612810",
      "highlight": "#f0b850",
      "text": "#f8e6c8",
      "text-muted": "#dab98a",
      "text-on-accent": "#041512",
      "text-on-fill": "#fdf1e0",
      "board-cell-bg": "#f0d29a",
      "board-cell-hover-bg": "#f8dfae",
      "board-free-bg": "#e07a2c",
      "board-gradient-start": "#2a1810",
      "board-gradient-end": "#1a0f06"
    }
  },
  {
    "name": "Solution Nine (Light)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .5)",
      "shadow": "rgba(0, 0, 0, .2)",
      "highlight-glow": "rgba(10, 52, 80, .28)",
      "page-bg": "#d2dbe4",
      "panel-bg": "#eef2f6",
      "panel-raised-bg": "#dee4ec",
      "control-border": "#b0b6c8",
      "input-bg": "#f8fafc",
      "accent": "#0c3a5a",
      "accent-hover": "#092c46",
      "accent-2": "#701e5a",
      "accent-2-hover": "#5a1748",
      "highlight": "#0a3450",
      "text": "#14202c",
      "text-muted": "#243240",
      "text-on-accent": "#eef7fc",
      "text-on-fill": "#fbeef6",
      "board-cell-bg": "#0e4666",
      "board-cell-hover-bg": "#12587e",
      "board-free-bg": "#8a1a6a",
      "board-gradient-start": "#e6eaf0",
      "board-gradient-end": "#d2dbe4"
    }
  },
  {
    "name": "Solution Nine (Dark)",
    "tokens": {
      "success": "#175020",
      "danger": "#8f2018",
      "warning": "#e0a82e",
      "modal-overlay": "rgba(0, 0, 0, .7)",
      "shadow": "rgba(0, 0, 0, .55)",
      "highlight-glow": "rgba(240, 112, 192, .42)",
      "page-bg": "#080b12",
      "panel-bg": "#0f1420",
      "panel-raised-bg": "#161d2c",
      "control-border": "#323e58",
      "input-bg": "#0a0e18",
      "accent": "#4cd6e8",
      "accent-hover": "#74e2f0",
      "accent-2": "#701e5a",
      "accent-2-hover": "#5a1748",
      "highlight": "#fba6dc",
      "text": "#e6f0f8",
      "text-muted": "#a6bcce",
      "text-on-accent": "#041016",
      "text-on-fill": "#fbeef6",
      "board-cell-bg": "#bfe6f2",
      "board-cell-hover-bg": "#d6f0f8",
      "board-free-bg": "#e858b8",
      "board-gradient-start": "#0d1220",
      "board-gradient-end": "#080b12"
    }
  }
]
JSON;

if (PHP_SAPI !== 'cli') {
    fwrite(STDERR, "Run this from the command line.\n");
    exit(2);
}
if ($argc < 2) {
    fwrite(STDERR, "usage: php insert_styles.php <path-to-database.sqlite>\n");
    exit(2);
}
$dbPath = $argv[1];
if (!is_file($dbPath)) {
    fwrite(STDERR, "error: database not found: {$dbPath}\n");
    exit(1);
}

$themes = json_decode(THEMES_JSON, true);
if (!is_array($themes)) {
    fwrite(STDERR, "error: embedded theme JSON failed to parse\n");
    exit(1);
}

try {
    $db = new PDO('sqlite:' . $dbPath);
    $db->setAttribute(PDO::ATTR_ERRMODE, PDO::ERRMODE_EXCEPTION);
} catch (Throwable $e) {
    fwrite(STDERR, "error: cannot open database: " . $e->getMessage() . "\n");
    exit(1);
}

$hasTable = $db->query(
    "SELECT name FROM sqlite_master WHERE type='table' AND name='styles'"
)->fetchColumn();
if ($hasTable === false) {
    fwrite(STDERR, "error: 'styles' table not found — is this the App Suite database?\n");
    exit(1);
}

$exists = $db->prepare("SELECT 1 FROM styles WHERE name = ?");
$insert = $db->prepare(
    "INSERT INTO styles (name, tokens, board_flourish, number_flourish) VALUES (?, ?, '', '')"
);

$inserted = 0;
$skipped  = 0;
$db->beginTransaction();
try {
    foreach ($themes as $t) {
        $name = $t['name'];
        $exists->execute([$name]);
        if ($exists->fetchColumn() !== false) {
            fwrite(STDOUT, "  skip (already present): {$name}\n");
            $skipped++;
            continue;
        }
        $tokens = json_encode($t['tokens'], JSON_UNESCAPED_SLASHES);
        $insert->execute([$name, $tokens]);
        fwrite(STDOUT, "  inserted: {$name}\n");
        $inserted++;
    }
    $db->commit();
} catch (Throwable $e) {
    $db->rollBack();
    fwrite(STDERR, "error during insert (rolled back, no changes made): " . $e->getMessage() . "\n");
    exit(1);
}

fwrite(STDOUT, "\nDone. Inserted {$inserted}, skipped {$skipped} (already present).\n");
