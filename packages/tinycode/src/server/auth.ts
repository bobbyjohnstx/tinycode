export * as ServerAuth from "./auth"

import { ConfigService } from "@/effect/config-service"
import { Flag } from "@/core/flag/flag"
import { Config as EffectConfig, Context, Option, Redacted } from "effect"
import { timingSafeEqual } from "crypto"

export type Credentials = {
  password?: string
  username?: string
}

export type DecodedCredentials = {
  readonly username: string
  readonly password: Redacted.Redacted
}

export class Config extends ConfigService.Service<Config>()("@tinycode/ServerAuthConfig", {
  password: EffectConfig.string("TINYCODE_SERVER_PASSWORD").pipe(EffectConfig.option),
  username: EffectConfig.string("TINYCODE_SERVER_USERNAME").pipe(EffectConfig.withDefault("tinycode")),
}) {}

export type Info = Context.Service.Shape<typeof Config>

export function required(config: Info) {
  return Option.isSome(config.password) && config.password.value !== ""
}

export function authorized(credentials: DecodedCredentials, config: Info) {
  if (!Option.isSome(config.password)) return false
  if (credentials.username !== config.username) return false

  const supplied = Buffer.from(Redacted.value(credentials.password))
  const expected = Buffer.from(config.password.value)
  if (supplied.length !== expected.length) return false

  return timingSafeEqual(supplied, expected)
}

export function header(credentials?: Credentials) {
  const password = credentials?.password ?? Flag.TINYCODE_SERVER_PASSWORD
  if (!password) return undefined

  const username = credentials?.username ?? Flag.TINYCODE_SERVER_USERNAME ?? "tinycode"
  return `Basic ${Buffer.from(`${username}:${password}`).toString("base64")}`
}

export function headers(credentials?: Credentials) {
  const authorization = header(credentials)
  if (!authorization) return undefined
  return { Authorization: authorization }
}
