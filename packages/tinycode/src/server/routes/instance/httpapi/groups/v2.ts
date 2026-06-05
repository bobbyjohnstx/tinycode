import { HttpApi, OpenApi } from "effect/unstable/httpapi"
import { ModelGroup } from "./v2/model"
import { ProviderGroup } from "./v2/provider"

export const V2Api = HttpApi.make("v2")
  .add(ModelGroup)
  .add(ProviderGroup)
  .annotateMerge(
    OpenApi.annotations({
      title: "tinycode experimental HttpApi",
      version: "0.0.1",
      description: "Experimental HttpApi surface for selected instance routes.",
    }),
  )
