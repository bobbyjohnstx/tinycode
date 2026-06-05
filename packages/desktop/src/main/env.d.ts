interface ImportMetaEnv {
  readonly TINYCODE_CHANNEL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module "virtual:tinycode-server" {
  export namespace Server {
    export const listen: typeof import("../../../tinycode/dist/types/src/node").Server.listen
    export type Listener = import("../../../tinycode/dist/types/src/node").Server.Listener
  }
  export namespace Config {
    export const get: typeof import("../../../tinycode/dist/types/src/node").Config.get
    export type Info = import("../../../tinycode/dist/types/src/node").Config.Info
  }
  export namespace Log {
    export const init: typeof import("../../../tinycode/dist/types/src/node").Log.init
  }
  export namespace Database {
    export const getPath: typeof import("../../../tinycode/dist/types/src/node").Database.getPath
    export const Client: typeof import("../../../tinycode/dist/types/src/node").Database.Client
  }
  export namespace JsonMigration {
    export type Progress = import("../../../tinycode/dist/types/src/node").JsonMigration.Progress
    export const run: typeof import("../../../tinycode/dist/types/src/node").JsonMigration.run
  }
  export const bootstrap: typeof import("../../../tinycode/dist/types/src/node").bootstrap
}
