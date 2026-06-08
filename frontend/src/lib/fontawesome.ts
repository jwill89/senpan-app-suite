/**
 * FontAwesome setup (replaces the hosted FontAwesome Kit script).
 *
 * Uses the SVG-core library with `dom.watch()` so the existing
 * `<i class="fa-solid fa-...">` markup throughout the templates is
 * auto-replaced with inline SVG — no need to convert every icon usage to a
 * <font-awesome-icon> component. Only the icons actually used are bundled.
 */
import { config, library, dom } from '@fortawesome/fontawesome-svg-core'
import {
  faBars,
  faChampagneGlasses,
  faCircleCheck,
  faCircleDot,
  faCirclePause,
  faClipboardList,
  faClock,
  faCopy,
  faDice,
  faDownload,
  faEraser,
  faFlagCheckered,
  faFolderOpen,
  faFont,
  faGamepad,
  faGear,
  faIdCard,
  faLink,
  faLock,
  faPalette,
  faPenToSquare,
  faPlus,
  faRotate,
  faTicket,
  faTrash,
  faTriangleExclamation,
  faTrophy,
  faUser,
  faVolumeHigh,
  faVolumeXmark,
} from '@fortawesome/free-solid-svg-icons'

// Don't auto-replace on import; we trigger dom.watch() explicitly after mount.
config.autoReplaceSvg = true
config.observeMutations = true // watch for icons added by Vue after initial render

library.add(
  faBars,
  faChampagneGlasses,
  faCircleCheck,
  faCircleDot,
  faCirclePause,
  faClipboardList,
  faClock,
  faCopy,
  faDice,
  faDownload,
  faEraser,
  faFlagCheckered,
  faFolderOpen,
  faFont,
  faGamepad,
  faGear,
  faIdCard,
  faLink,
  faLock,
  faPalette,
  faPenToSquare,
  faPlus,
  faRotate,
  faTicket,
  faTrash,
  faTriangleExclamation,
  faTrophy,
  faUser,
  faVolumeHigh,
  faVolumeXmark,
)

/**
 * Starts watching the DOM and replacing `<i class="fa-...">` placeholders with
 * inline SVG. `observeMutations` keeps icons rendered by Vue updates in sync.
 */
export function initFontAwesome(): void {
  dom.watch()
}
