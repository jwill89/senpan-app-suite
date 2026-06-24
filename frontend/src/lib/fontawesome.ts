/**
 * FontAwesome icon registry.
 *
 * Icons come from the Font Awesome Pro kit (`@awesome.me/kit-46204fb6f1`),
 * tree-shaken to only what's used, in three styles:
 *
 *   - DUOTONE (prefix `fad`) for headers, sidebar sections/nav, and decorative
 *     icons — the two-tone look reads well at larger sizes.
 *   - SOLID (prefix `fas`) for action-button icons (Save, Delete, Upload, Copy,
 *     …), where the flat single-tone glyph stays crisp and clear.
 *   - BRANDS (prefix `fab`) for the Discord icon.
 *
 * Icons are added to the core `library` here and rendered in templates with the
 * `<font-awesome-icon :icon="[prefix, name]" />` component (registered globally
 * in main.ts) — e.g. `:icon="['fad', 'gear']"` / `:icon="['fas', 'trash']"` /
 * `:icon="['fab', 'discord']"`. Vue owns the rendered <svg> directly, so there
 * is no `dom.watch()` / MutationObserver (the old hosted-kit approach); core
 * auto-injects its CSS, which supplies the duotone primary/secondary layering.
 *
 * To add an icon: import its `fa…` name from the matching style module below
 * (alias solid imports with the `Solid` suffix to avoid name clashes with the
 * duotone set), add it to `icons`, and reference it by `[prefix, name]`.
 */
import { library, type IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  faAt,
  faBallot,
  faBars,
  faBicep,
  faBook,
  faBookOpenCover,
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
  faDice,
  faFileAudio,
  faFlagCheckered,
  faFlowerDaffodil,
  faFolder,
  faFolderOpen,
  faFont,
  faGameBoardSimple,
  faGamepad,
  faGear,
  faGears,
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
  faSliders,
  faTicket,
  faToriiGate,
  faTriangleExclamation,
  faTrophy,
  faFaceSmile,
  faUser,
  faUserPlus,
  faUsersGear,
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
  faVolumeLow as faVolumeLowSolid,
  faVolumeXmark as faVolumeXmarkSolid,
} from '@awesome.me/kit-46204fb6f1/icons/classic/solid'
import { faDiscord } from '@fortawesome/free-brands-svg-icons'

// The Pro kit ships icons typed against fontawesome-common-types v7 while the
// SVG core here is v6; the runtime icon shape is identical (verified: prefixes
// `fad`/`fas`, and core 6.x maps the `fa-duotone`/`fa-solid` classes → those
// prefixes), so the v7→v6 IconDefinition mismatch is a types-only concern we
// cast away.
const icons = [
  // Duotone — headers, sidebar sections/nav, decorative.
  faAt,
  faBallot,
  faBars,
  faBicep,
  faBook,
  faBookOpenCover,
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
  faDice,
  faFileAudio,
  faFlagCheckered,
  faFlowerDaffodil,
  faFolder,
  faFolderOpen,
  faFont,
  faGameBoardSimple,
  faGamepad,
  faGear,
  faGears,
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
  faSliders,
  faTicket,
  faToriiGate,
  faTriangleExclamation,
  faTrophy,
  faFaceSmile,
  faUser,
  faUserPlus,
  faUsersGear,
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
  faVolumeLowSolid,
  faVolumeXmarkSolid,
  // Brands — social/external links.
  faDiscord,
] as unknown as IconDefinition[]

library.add(...icons)
