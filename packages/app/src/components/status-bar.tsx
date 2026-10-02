import type { JSX } from "solid-js"
import { Show, createMemo } from "solid-js"

export interface StatusBarProps {
  agentName: string
  model: string
  provider: string
  effort: string
  contextUsed: number
  contextTotal: number
  isWorking: boolean
}

function formatTokens(n: number): string {
  return n.toLocaleString()
}

export function StatusBar(props: StatusBarProps): JSX.Element {
  const contextPct = createMemo(() => {
    if (props.contextTotal <= 0) return 0
    return Math.round((props.contextUsed / props.contextTotal) * 100)
  })

  const fillWidth = createMemo(() => {
    if (props.contextTotal <= 0) return "0%"
    return `${Math.min(100, (props.contextUsed / props.contextTotal) * 100)}%`
  })

  return (
    <>
      <style>{`
        @keyframes status-bar-spin {
          from { transform: rotate(0deg); }
          to { transform: rotate(360deg); }
        }
      `}</style>
      <div
        style={{
          width: "100%",
          position: "sticky",
          bottom: "0",
          background: "#111111",
          "border-top": "1px solid #1e1e1e",
          padding: "6px 24px",
          "font-family": "'JetBrains Mono', monospace",
          "font-size": "12px",
          display: "flex",
          "flex-direction": "row",
          "align-items": "center",
          "z-index": "10",
          "box-sizing": "border-box",
        }}
      >
        {/* Left section */}
        <div
          style={{
            display: "flex",
            "align-items": "center",
            gap: "0",
            "min-width": "0",
          }}
        >
          <span style={{ color: "#c87898", "font-weight": "500" }}>{props.agentName}</span>
          <span style={{ color: "#c8c8d0" }}>{" · "}</span>
          <span style={{ color: "#c8c8d0" }}>{props.model}</span>
          <span style={{ color: "#c8c8d0" }}>{" · "}</span>
          <span style={{ color: "#c8c8d0" }}>{props.provider}</span>
          <Show when={props.effort && props.effort !== "medium"}>
            <span style={{ color: "#c8c8d0" }}>{" · "}</span>
            <span style={{ color: "#c8c8d0" }}>{props.effort}</span>
          </Show>
          <span style={{ color: "#c8c8d0" }}>{" · "}</span>
          <span style={{ color: "#56b6c2" }}>{contextPct()}%</span>
          <span style={{ color: "#c8c8d0", "margin-left": "4px" }}>ctx</span>
        </div>

        {/* Right section */}
        <div
          style={{
            "margin-left": "auto",
            display: "flex",
            "align-items": "center",
            gap: "10px",
          }}
        >
          {/* Context meter */}
          <div
            style={{
              width: "48px",
              height: "4px",
              background: "#2a2a2a",
              "border-radius": "2px",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                width: fillWidth(),
                height: "100%",
                background: "#56b6c2",
                "border-radius": "2px",
                transition: "width 0.3s ease",
              }}
            />
          </div>

          {/* Token count */}
          <span style={{ color: "#484848", "font-size": "10px", "white-space": "nowrap" }}>
            {formatTokens(props.contextUsed)} / {formatTokens(props.contextTotal)}
          </span>

          {/* Working indicator */}
          <Show
            when={props.isWorking}
            fallback={
              <div
                style={{
                  width: "6px",
                  height: "6px",
                  "border-radius": "50%",
                  background: "#808080",
                  "flex-shrink": "0",
                }}
              />
            }
          >
            <div
              style={{
                width: "12px",
                height: "12px",
                border: "2px solid #2a2a2a",
                "border-top-color": "#c87898",
                "border-radius": "50%",
                animation: "status-bar-spin 0.8s linear infinite",
                "flex-shrink": "0",
              }}
            />
          </Show>
        </div>
      </div>
    </>
  )
}
