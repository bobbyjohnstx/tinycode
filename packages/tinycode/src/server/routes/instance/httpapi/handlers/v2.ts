import { Layer } from "effect"
import { layer as v2LocationLayer } from "../groups/v2/location"
import { modelHandlers } from "./v2/model"
import { providerHandlers } from "./v2/provider"

export const v2Handlers = Layer.mergeAll(modelHandlers, providerHandlers).pipe(Layer.provide(v2LocationLayer))
