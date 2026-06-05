import { AnthropicPlugin } from "./anthropic"
import { DynamicProviderPlugin } from "./dynamic"
import { GooglePlugin } from "./google"
import { OpenAICompatiblePlugin } from "./openai-compatible"
import { OpenAIPlugin } from "./openai"

export const ProviderPlugins = [
  AnthropicPlugin,
  GooglePlugin,
  OpenAICompatiblePlugin,
  OpenAIPlugin,
  DynamicProviderPlugin,
]
