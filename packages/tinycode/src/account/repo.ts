import { Effect, Layer, Option, Context } from "effect"
import { serviceUse } from "@opencode-ai/core/effect/service-use"

import { AccountTable } from "./account.sql"
import { AccessToken, AccountID, AccountRepoError, Info, OrgID, RefreshToken } from "./schema"

export type AccountRow = (typeof AccountTable)["$inferSelect"]

export interface Interface {
  readonly active: () => Effect.Effect<Option.Option<Info>, AccountRepoError>
  readonly list: () => Effect.Effect<Info[], AccountRepoError>
  readonly remove: (accountID: AccountID) => Effect.Effect<void, AccountRepoError>
  readonly use: (accountID: AccountID, orgID: Option.Option<OrgID>) => Effect.Effect<void, AccountRepoError>
  readonly getRow: (accountID: AccountID) => Effect.Effect<Option.Option<AccountRow>, AccountRepoError>
  readonly persistToken: (input: {
    accountID: AccountID
    accessToken: AccessToken
    refreshToken: RefreshToken
    expiry: Option.Option<number>
  }) => Effect.Effect<void, AccountRepoError>
  readonly persistAccount: (input: {
    id: AccountID
    email: string
    url: string
    accessToken: AccessToken
    refreshToken: RefreshToken
    expiry: number
    orgID: Option.Option<OrgID>
  }) => Effect.Effect<void, AccountRepoError>
}

export class Service extends Context.Service<Service, Interface>()("@opencode/AccountRepo") {}

export const use = serviceUse(Service)

export const layer: Layer.Layer<Service> = Layer.succeed(
  Service,
  Service.of({
    active: () => Effect.succeed(Option.none<Info>()),
    list: () => Effect.succeed([] as Info[]),
    remove: (_accountID) => Effect.void,
    use: (_accountID, _orgID) => Effect.void,
    getRow: (_accountID) => Effect.succeed(Option.none<AccountRow>()),
    persistToken: (_input) => Effect.void,
    persistAccount: (_input) => Effect.void,
  }),
)

export * as AccountRepo from "./repo"
