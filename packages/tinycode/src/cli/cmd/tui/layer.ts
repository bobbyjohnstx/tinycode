import { Layer } from "effect"
import { TuiConfig } from "./config/tui"
import { Npm } from "@/core/npm"
import { Observability } from "@/core/effect/observability"

export const CliLayer = Observability.layer.pipe(Layer.merge(TuiConfig.layer), Layer.provide(Npm.defaultLayer))
