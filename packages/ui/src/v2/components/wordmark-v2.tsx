import { createUniqueId, type ComponentProps } from "solid-js"

export function WordmarkV2(props: Pick<ComponentProps<"svg">, "class">) {
  const filter = createUniqueId()
  const mask = createUniqueId()
  const maskGradient = createUniqueId()

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 720.002 129.001"
      fill="none"
      preserveAspectRatio="none"
      classList={{ [props.class ?? ""]: !!props.class }}
    >
      <g opacity="0.16" filter={`url(#${filter})`} mask={`url(#${mask})`}>
        <path
          opacity="0.7"
          d="M55.3845 36.8583H36.923V110.573H18.4615V36.8583H0V18.4297H55.3845Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M110.774 18.4297H129.236V36.8583H110.774Z M110.774 55.2868H129.236V110.573H110.774Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M240.002 36.8583H203.079V110.573H184.617V18.4297H240.002Z M258.463 110.573H240.002V36.8583H258.463Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M276.922 18.4297H295.383V55.2868H276.922Z M332.306 18.4297H350.768V55.2868H332.306Z M295.383 55.2868H332.306V73.7154H313.845V110.573H295.383Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M443.081 36.8583H387.696V92.144H443.081V110.573H369.234V18.4297H443.081V36.8583Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M516.924 36.8583H480.001V92.144H516.924V36.8583ZM535.385 110.573H461.539V18.4297H535.385V110.573Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M609.228 36.8571H572.305V92.1429H609.228V36.8571ZM627.69 110.571H553.844V18.4286H609.228V0H627.69V110.571Z"
          fill="currentColor"
        />
        <path
          opacity="0.7"
          d="M664.618 36.8583V55.2868H701.541V36.8583H664.618ZM720.002 73.7154H664.618V92.144H720.002V110.573H646.156V18.4297H720.002V73.7154Z"
          fill="currentColor"
        />
      </g>
      <defs>
        <mask id={mask} maskUnits="userSpaceOnUse" x="0" y="0" width="720" height="129">
          <rect width="720" height="129" fill={`url(#${maskGradient})`} />
        </mask>
        <linearGradient id={maskGradient} x1="360" y1="0" x2="360" y2="112" gradientUnits="userSpaceOnUse">
          <stop stop-color="white" stop-opacity="0.7" />
          <stop offset="1" stop-color="white" stop-opacity="0" />
        </linearGradient>
        <filter
          id={filter}
          x="0"
          y="0"
          width="720.002"
          height="130.001"
          filterUnits="userSpaceOnUse"
          color-interpolation-filters="sRGB"
        >
          <feFlood flood-opacity="0" result="BackgroundImageFix" />
          <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
          <feColorMatrix
            in="SourceAlpha"
            type="matrix"
            values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0"
            result="hardAlpha"
          />
          <feOffset dy="1" />
          <feGaussianBlur stdDeviation="1" />
          <feComposite in2="hardAlpha" operator="arithmetic" k2="-1" k3="1" />
          <feColorMatrix type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0" />
          <feBlend mode="normal" in2="shape" result="effect1_innerShadow_4938_16028" />
        </filter>
      </defs>
    </svg>
  )
}
