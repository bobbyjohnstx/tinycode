import type { JSX } from "solid-js"
import { Show, For } from "solid-js"

export interface GoalStep {
  label: string
  status: "done" | "active" | "pending"
}

export interface GoalTrackerProps {
  description: string
  steps: GoalStep[]
  iteration: number
  maxIterations: number
  isComplete: boolean
}

function StepIcon(props: { status: GoalStep["status"] }): JSX.Element {
  return (
    <>
      <Show when={props.status === "done"}>
        <span style={{ color: "#7fd88f" }}>{"✓"}</span>
      </Show>
      <Show when={props.status === "active"}>
        <span
          style={{
            color: "#f5a742",
            animation: "goal-tracker-pulse 1.5s ease-in-out infinite",
          }}
        >
          {"⚡"}
        </span>
      </Show>
      <Show when={props.status === "pending"}>
        <span style={{ color: "#484848" }}>{"○"}</span>
      </Show>
    </>
  )
}

export function GoalTracker(props: GoalTrackerProps): JSX.Element {
  const remainingIterations = () => props.maxIterations - props.iteration

  return (
    <>
      <style>{`
        @keyframes goal-tracker-pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.4; }
        }
      `}</style>
      <Show
        when={!props.isComplete}
        fallback={
          <div
            style={{
              border: "1px solid #c87898",
              background: "#0f0a0c",
              "border-radius": "6px",
              margin: "0 24px 8px",
              padding: "8px 14px",
              display: "flex",
              "align-items": "center",
              "font-family": "'JetBrains Mono', monospace",
              "font-size": "12px",
            }}
          >
            <span style={{ color: "#7fd88f", "margin-right": "8px" }}>{"✓"}</span>
            <span style={{ color: "#7fd88f", "font-weight": "600", "margin-right": "6px" }}>Goal:</span>
            <span style={{ color: "#eeeeee", "font-weight": "500" }}>{props.description}</span>
            <span
              style={{
                "margin-left": "auto",
                color: "#808080",
                "font-size": "11px",
              }}
            >
              {props.steps.filter((s) => s.status === "done").length} / {props.steps.length} steps
            </span>
          </div>
        }
      >
        <div
          style={{
            border: "1px solid #c87898",
            background: "#0f0a0c",
            "border-radius": "6px",
            margin: "0 24px 8px",
            "font-family": "'JetBrains Mono', monospace",
            "font-size": "12px",
          }}
        >
          {/* Header */}
          <div
            style={{
              display: "flex",
              "align-items": "center",
              padding: "8px 14px",
              "border-bottom": "1px solid rgba(200, 120, 152, 0.2)",
            }}
          >
            <div style={{ display: "flex", "align-items": "center", "min-width": "0" }}>
              <span style={{ color: "#c87898", "font-weight": "600", "margin-right": "6px" }}>Goal:</span>
              <span style={{ color: "#eeeeee", "font-weight": "500" }}>{props.description}</span>
            </div>
            <span
              style={{
                "margin-left": "auto",
                color: "#808080",
                "font-size": "11px",
                "white-space": "nowrap",
                "padding-left": "12px",
              }}
            >
              iteration {props.iteration} / {props.maxIterations}
            </span>
          </div>

          {/* Steps */}
          <div
            style={{
              display: "flex",
              "flex-wrap": "wrap",
              gap: "16px",
              padding: "8px 14px",
            }}
          >
            <For each={props.steps}>
              {(step) => (
                <div style={{ display: "flex", "align-items": "center", gap: "4px" }}>
                  <StepIcon status={step.status} />
                  <span
                    style={{
                      color: step.status === "active" ? "#eeeeee" : step.status === "done" ? "#808080" : "#484848",
                    }}
                  >
                    {step.label}
                  </span>
                </div>
              )}
            </For>
          </div>

          {/* Budget line */}
          <div
            style={{
              display: "flex",
              gap: "16px",
              padding: "4px 14px 8px",
              color: "#484848",
              "font-size": "11px",
            }}
          >
            <span>Remaining iterations: {remainingIterations()}</span>
          </div>
        </div>
      </Show>
    </>
  )
}
