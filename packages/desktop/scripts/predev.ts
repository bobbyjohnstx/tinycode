import { $ } from "bun"

await $`bun ./scripts/copy-icons.ts ${process.env.TINYCODE_CHANNEL ?? "dev"}`

await $`cd ../tinycode && bun script/build-node.ts`
