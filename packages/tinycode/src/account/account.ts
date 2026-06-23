import { Effect, Layer, Option, Context } from "effect"
import { serviceUse } from "@/core/effect/service-use"

import type { AccountError, AccessToken, AccountID, Info, Org, OrgID, Login, PollResult } from "./schema"

export {
  AccountID,
  type AccountError,
  AccountRepoError,
  AccountServiceError,
  AccountTransportError,
  AccessToken,
  RefreshToken,
  DeviceCode,
  UserCode,
  Info,
  Org,
  OrgID,
  Login,
  PollSuccess,
  PollPending,
  PollSlow,
  PollExpired,
  PollDenied,
  PollError,
  PollResult,
} from "./schema"

export type AccountOrgs = {
  account: Info
  orgs: readonly Org[]
}

export type ActiveOrg = {
  account: Info
  org: Org
}

export interface Interface {
  readonly active: () => Effect.Effect<Option.Option<Info>, AccountError>
  readonly activeOrg: () => Effect.Effect<Option.Option<ActiveOrg>, AccountError>
  readonly list: () => Effect.Effect<Info[], AccountError>
  readonly orgsByAccount: () => Effect.Effect<readonly AccountOrgs[], AccountError>
  readonly remove: (accountID: AccountID) => Effect.Effect<void, AccountError>
  readonly use: (accountID: AccountID, orgID: Option.Option<OrgID>) => Effect.Effect<void, AccountError>
  readonly orgs: (accountID: AccountID) => Effect.Effect<readonly Org[], AccountError>
  readonly config: (
    accountID: AccountID,
    orgID: OrgID,
  ) => Effect.Effect<Option.Option<Record<string, unknown>>, AccountError>
  readonly token: (accountID: AccountID) => Effect.Effect<Option.Option<AccessToken>, AccountError>
  readonly login: (url: string) => Effect.Effect<Login, AccountError>
  readonly poll: (input: Login) => Effect.Effect<PollResult, AccountError>
}

export class Service extends Context.Service<Service, Interface>()("@tinycode/Account") {}

export const use = serviceUse(Service)

export const layer: Layer.Layer<Service> = Layer.succeed(
  Service,
  Service.of({
    active: () => Effect.succeed(Option.none<Info>()),
    activeOrg: () => Effect.succeed(Option.none<ActiveOrg>()),
    list: () => Effect.succeed([] as Info[]),
    orgsByAccount: () => Effect.succeed([] as AccountOrgs[]),
    remove: (_accountID) => Effect.void,
    use: (_accountID, _orgID) => Effect.void,
    orgs: (_accountID) => Effect.succeed([] as Org[]),
    config: (_accountID, _orgID) => Effect.succeed(Option.none<Record<string, unknown>>()),
    token: (_accountID) => Effect.succeed(Option.none<AccessToken>()),
    login: (_url) => Effect.die(new Error("Account system removed in tinycode")),
    poll: (_input) => Effect.die(new Error("Account system removed in tinycode")),
  }),
)

export const defaultLayer = layer

export * as Account from "./account"
