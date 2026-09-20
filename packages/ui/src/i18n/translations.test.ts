import { describe, expect, test } from "bun:test"
import { dict as en } from "./en"
import { dict as ar } from "./ar"
import { dict as br } from "./br"
import { dict as bs } from "./bs"
import { dict as da } from "./da"
import { dict as de } from "./de"
import { dict as es } from "./es"
import { dict as fr } from "./fr"
import { dict as ja } from "./ja"
import { dict as ko } from "./ko"
import { dict as no } from "./no"
import { dict as pl } from "./pl"
import { dict as ru } from "./ru"
import { dict as th } from "./th"
import { dict as tr } from "./tr"
import { dict as uk } from "./uk"
import { dict as zh } from "./zh"
import { dict as zht } from "./zht"

const locales: Record<string, Record<string, string>> = {
  ar,
  br,
  bs,
  da,
  de,
  es,
  fr,
  ja,
  ko,
  no,
  pl,
  ru,
  th,
  tr,
  uk,
  zh,
  zht,
}

const enKeys = Object.keys(en).sort()

describe("i18n translation completeness", () => {
  test("english dictionary has keys", () => {
    expect(enKeys.length).toBeGreaterThan(0)
  })

  for (const [locale, dict] of Object.entries(locales)) {
    describe(`${locale} locale`, () => {
      test("covers at least 90% of english keys", () => {
        const localeKeys = new Set(Object.keys(dict))
        const missing = enKeys.filter((key) => !localeKeys.has(key))
        const coveragePercent = ((enKeys.length - missing.length) / enKeys.length) * 100
        expect(coveragePercent).toBeGreaterThanOrEqual(90)
      })

      test("has no extra keys beyond english", () => {
        const localeKeys = Object.keys(dict)
        const extra = localeKeys.filter((key) => !(key in en))
        expect(extra).toEqual([])
      })

      test("all values are strings", () => {
        for (const [key, value] of Object.entries(dict)) {
          expect(typeof value).toBe("string")
        }
      })

      test("preserves template placeholders from english for translated keys", () => {
        const placeholderPattern = /\{\{\s*(\w+)\s*\}\}/g
        for (const [key, enValue] of Object.entries(en)) {
          const enPlaceholders = [...enValue.matchAll(placeholderPattern)].map((m) => m[1]).sort()
          if (enPlaceholders.length === 0) continue
          const localeValue = dict[key]
          if (!localeValue) continue
          const localePlaceholders = [...localeValue.matchAll(placeholderPattern)].map((m) => m[1]).sort()
          expect(localePlaceholders).toEqual(enPlaceholders)
        }
      })
    })
  }
})
