// The SeatHold palette, carried over from the web stylesheet so the two clients
// cannot drift apart. Only the dark set is here: the design is dark first, and
// app.json pins the interface style so the OS cannot ask for a theme that does
// not exist yet.
//
// Nothing outside this file names a colour.
export const color = {
  bg: '#120F0D',
  surface: '#1D1916',
  surfaceRaised: '#26211D',
  line: '#2E2723',
  lineStrong: '#3A322C',
  text: '#F5EEE7',
  textMuted: '#A8998B',

  primary: '#FF9F43',
  onPrimary: '#1A1004',
  danger: '#FF5B4F',
  warning: '#F2C14E',
  success: '#4CC38A',

  seatOpen: '#4B413A',
  seatHeldLine: '#B8A595',
  seatHeldFill: 'rgba(184, 165, 149, 0.12)',
  seatSold: '#3A302A',
  seatMine: '#FF9F43',

  dangerBg: '#2A1512',
  warningBg: '#231C0C',
  successBg: '#0F2419',
  infoBg: '#241A0D',
} as const;

// color-mix has no React Native equivalent, so the few blends the web stylesheet
// makes are resolved here by hand against the background they sit on.
export const blend = {
  primarySoft: 'rgba(255, 159, 67, 0.18)',
  primaryLine: 'rgba(255, 159, 67, 0.45)',
  dangerSoft: 'rgba(255, 91, 79, 0.15)',
  successSoft: 'rgba(76, 195, 138, 0.18)',
  onPoster: 'rgba(255, 255, 255, 0.18)',
} as const;

export const radius = {
  base: 14,
  lg: 20,
  pill: 999,
} as const;

export const space = {
  gutter: 16,
  tap: 52,
} as const;

// Two families on the web, neither of which ships with a phone. Until the files
// are bundled this follows the same rule the web does: fall back rather than
// load a third party, and keep one name for display and one for body so the
// swap is a single edit.
export const font = {
  display: undefined as string | undefined,
  body: undefined as string | undefined,
} as const;

export const type = {
  display: { fontSize: 30, lineHeight: 34, fontWeight: '700' },
  title: { fontSize: 28, lineHeight: 32, fontWeight: '700' },
  heading: { fontSize: 19, lineHeight: 24, fontWeight: '700' },
  body: { fontSize: 16, lineHeight: 22, fontWeight: '400' },
  label: { fontSize: 12, lineHeight: 16, fontWeight: '600', letterSpacing: 1.2 },
  caption: { fontSize: 13, lineHeight: 18, fontWeight: '400' },
} as const;
