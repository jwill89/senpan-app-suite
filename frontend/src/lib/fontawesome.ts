/**
 * FontAwesome setup (replaces the hosted FontAwesome Kit script).
 *
 * Icons come from the Font Awesome Pro kit (`@awesome.me/kit-46204fb6f1`),
 * tree-shaken to only what's used, in two styles:
 *
 *   - DUOTONE (`fa-duotone`, prefix `fad`) for headers, sidebar sections/nav,
 *     and decorative icons — the two-tone look reads well at larger sizes.
 *   - SOLID (`fa-solid`, prefix `fas`) for action-button icons (Save, Delete,
 *     Upload, Copy, …), where the flat single-tone glyph stays crisp and clear.
 *
 * The SVG-core library with `dom.watch()` auto-replaces the `<i class="fa-…">`
 * markup throughout the templates with inline SVG; core's auto-injected CSS
 * supplies the duotone primary/secondary layer styling.
 *
 * To add an icon: import its `fa…` name from the matching style module below
 * (alias solid imports with the `Solid` suffix to avoid name clashes with the
 * duotone set), add it to `icons`, and reference it in markup with the right
 * style class.
 */
import { config, library, dom, type IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  faBallot,
  faBars,
  faBicep,
  faBook,
  faCalendarCircleExclamation,
  faCalendarClock,
  faCalendarDays,
  faChampagneGlasses,
  faCircleCheck,
  faCircleDot,
  faCirclePause,
  faClipboardList,
  faClock,
  faCloudArrowUp,
  faCompassDrafting,
  faFileAudio,
  faFlagCheckered,
  faFlowerDaffodil,
  faFolder,
  faFolderOpen,
  faFont,
  faGamepad,
  faGear,
  faGrid,
  faIdCard,
  faImage,
  faImages,
  faLayerGroup,
  faLink,
  faLocationDot,
  faLock,
  faMagnifyingGlass,
  faMegaphone,
  faPalette,
  faPenToSquare,
  faPlus,
  faTicket,
  faToriiGate,
  faTriangleExclamation,
  faTrophy,
  faFaceSmile,
  faUser,
} from '@awesome.me/kit-46204fb6f1/icons/duotone/solid'
import {
  faArrowLeft as faArrowLeftSolid,
  faArrowRightFromBracket as faArrowRightFromBracketSolid,
  faChevronDown as faChevronDownSolid,
  faChevronUp as faChevronUpSolid,
  faCircleCheck as faCircleCheckSolid,
  faCircleDot as faCircleDotSolid,
  faCopy as faCopySolid,
  faDice as faDiceSolid,
  faDownload as faDownloadSolid,
  faEraser as faEraserSolid,
  faEye as faEyeSolid,
  faForwardStep as faForwardStepSolid,
  faLink as faLinkSolid,
  faLock as faLockSolid,
  faMagnifyingGlass as faMagnifyingGlassSolid,
  faMoon as faMoonSolid,
  faPaperPlane as faPaperPlaneSolid,
  faPenToSquare as faPenToSquareSolid,
  faPlus as faPlusSolid,
  faRotate as faRotateSolid,
  faSliders as faSlidersSolid,
  faSun as faSunSolid,
  faTrash as faTrashSolid,
  faUpload as faUploadSolid,
  faVolumeHigh as faVolumeHighSolid,
  faVolumeXmark as faVolumeXmarkSolid,
} from '@awesome.me/kit-46204fb6f1/icons/classic/solid'
import { faDiscord } from '@fortawesome/free-brands-svg-icons'

// Render in "nest" mode: FontAwesome inserts the <svg> *inside* the existing
// <i class="fa-…"> wrapper instead of replacing it. This is essential for Vue
// compatibility — Vue keeps owning the stable <i> element while FA owns the
// nested <svg>, so reconciliation (re-renders, v-if/v-else icon swaps) never
// operates on an FA-detached node. The default `true` (replace) mode swaps the
// <i> for an <svg>, leaving Vue's vnode pointing at a detached element; the next
// patch then throws "Cannot read properties of null (insertBefore/emitsOptions)"
// and aborts (e.g. the admin last-drawn number silently stops updating).
config.autoReplaceSvg = 'nest'
config.observeMutations = true // watch for icons added by Vue after initial render

// The Pro kit ships icons typed against fontawesome-common-types v7 while the
// SVG core here is v6; the runtime icon shape is identical (verified: prefixes
// `fad`/`fas`, and core 6.x maps the `fa-duotone`/`fa-solid` classes → those
// prefixes), so the v7→v6 IconDefinition mismatch is a types-only concern we
// cast away.
const icons = [
  // Duotone — headers, sidebar sections/nav, decorative.
  faBallot,
  faBars,
  faBicep,
  faBook,
  faCalendarCircleExclamation,
  faCalendarClock,
  faCalendarDays,
  faChampagneGlasses,
  faCircleCheck,
  faCircleDot,
  faCirclePause,
  faClipboardList,
  faClock,
  faCloudArrowUp,
  faCompassDrafting,
  faFileAudio,
  faFlagCheckered,
  faFlowerDaffodil,
  faFolder,
  faFolderOpen,
  faFont,
  faGamepad,
  faGear,
  faGrid,
  faIdCard,
  faImage,
  faImages,
  faLayerGroup,
  faLink,
  faLocationDot,
  faLock,
  faMagnifyingGlass,
  faMegaphone,
  faPalette,
  faPenToSquare,
  faPlus,
  faTicket,
  faToriiGate,
  faTriangleExclamation,
  faTrophy,
  faFaceSmile,
  faUser,
  // Solid — action-button icons.
  faArrowLeftSolid,
  faArrowRightFromBracketSolid,
  faChevronDownSolid,
  faChevronUpSolid,
  faCircleCheckSolid,
  faCircleDotSolid,
  faCopySolid,
  faDiceSolid,
  faDownloadSolid,
  faEraserSolid,
  faEyeSolid,
  faForwardStepSolid,
  faLinkSolid,
  faLockSolid,
  faMagnifyingGlassSolid,
  faMoonSolid,
  faPaperPlaneSolid,
  faPenToSquareSolid,
  faPlusSolid,
  faRotateSolid,
  faSlidersSolid,
  faSunSolid,
  faTrashSolid,
  faUploadSolid,
  faVolumeHighSolid,
  faVolumeXmarkSolid,
  // Brands — social/external links.
  faDiscord,
] as unknown as IconDefinition[]

library.add(...icons)

/**
 * Starts watching the DOM and replacing `<i class="fa-...">` placeholders with
 * inline SVG. `observeMutations` keeps icons rendered by Vue updates in sync.
 */
export function initFontAwesome(): void {
  dom.watch()
}
