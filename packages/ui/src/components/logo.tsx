import { type ComponentProps } from "solid-js"

export const Mark = (props: { class?: string }) => {
  return (
    <svg
      data-component="logo-mark"
      classList={{ [props.class ?? ""]: !!props.class }}
      viewBox="0 0 16 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path data-slot="logo-logo-mark-shadow" d="M12 16H4V8H12V16Z" fill="var(--icon-weak-base)" />
      <path data-slot="logo-logo-mark-o" d="M12 4H4V16H12V4ZM16 20H0V0H16V20Z" fill="var(--icon-strong-base)" />
    </svg>
  )
}

export const Splash = (props: Pick<ComponentProps<"svg">, "ref" | "class">) => {
  return (
    <svg
      ref={props.ref}
      data-component="logo-splash"
      classList={{ [props.class ?? ""]: !!props.class }}
      viewBox="0 0 40 80"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M30 4 L10 76"
        stroke="var(--icon-strong-base)"
        stroke-width="8"
        stroke-linecap="round"
      />
    </svg>
  )
}

// Diagonal split wordmark: "tiny" top-left (muted), ╲ separator, "code" bottom-right (strong).
// Pixel grid: each letter is 3×3 units, stroke width 2, gap 1.
export const Logo = (props: { class?: string }) => {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 280 80"
      width="280"
      height="80"
      fill="none"
      classList={{ [props.class ?? ""]: !!props.class }}
    >
      {/* tiny — muted, top-left */}
      <g fill="var(--icon-weak-base)">
        {/* t */}
        <rect x="2" y="4" width="14" height="4" />
        <rect x="7" y="4" width="4" height="24" />
        {/* i */}
        <rect x="22" y="4" width="4" height="4" />
        <rect x="22" y="12" width="4" height="16" />
        {/* n */}
        <rect x="32" y="4" width="4" height="24" />
        <rect x="32" y="4" width="16" height="4" />
        <rect x="44" y="4" width="4" height="24" />
        {/* y */}
        <rect x="54" y="4" width="4" height="14" />
        <rect x="64" y="4" width="4" height="14" />
        <rect x="54" y="14" width="14" height="4" />
        <rect x="59" y="18" width="4" height="10" />
      </g>
      {/* diagonal slash */}
      <line
        x1="82" y1="2"
        x2="118" y2="78"
        stroke="var(--icon-strong-base)"
        stroke-width="3"
        stroke-linecap="round"
      />
      {/* code — strong, bottom-right */}
      <g fill="var(--icon-strong-base)">
        {/* c */}
        <rect x="126" y="28" width="16" height="4" />
        <rect x="126" y="28" width="4" height="24" />
        <rect x="126" y="48" width="16" height="4" />
        {/* o */}
        <rect x="148" y="28" width="16" height="4" />
        <rect x="148" y="28" width="4" height="24" />
        <rect x="160" y="28" width="4" height="24" />
        <rect x="148" y="48" width="16" height="4" />
        {/* d */}
        <rect x="170" y="16" width="4" height="36" />
        <rect x="170" y="28" width="12" height="4" />
        <rect x="178" y="28" width="4" height="24" />
        <rect x="170" y="48" width="12" height="4" />
        {/* e */}
        <rect x="190" y="28" width="16" height="4" />
        <rect x="190" y="28" width="4" height="24" />
        <rect x="190" y="38" width="14" height="4" />
        <rect x="190" y="48" width="16" height="4" />
      </g>
    </svg>
  )
}
