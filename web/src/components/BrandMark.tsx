// Original Variya interface mark, 2026-09-11.
export default function BrandMark({ size = 30 }: { size?: number }) {
  return <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true" focusable="false">
    <rect width="32" height="32" rx="9" fill="#5966A6" />
    <path d="M8 9L15.5 24L24 9H20.5L15.6 18.4L11.3 9H8Z" fill="white" />
    <path d="M21.8 22.5H25" stroke="white" strokeWidth="2" strokeLinecap="round" />
  </svg>;
}
