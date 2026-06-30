import { describe, expect, test } from "bun:test"
import { Slug } from "@/core/util/slug"

describe("Slug.create", () => {
  test("generates a slug with adjective-noun format", () => {
    const slug = Slug.create()
    expect(slug).toMatch(/^[a-z]+-[a-z]+$/)
    expect(slug.split("-").length).toBe(2)
  })

  test("generates different slugs on multiple calls", () => {
    const slugs = new Set()
    for (let i = 0; i < 50; i++) {
      slugs.add(Slug.create())
    }
    expect(slugs.size).toBeGreaterThan(1)
  })

  test("generates slugs from known word lists", () => {
    const validAdjectives = [
      "brave",
      "calm",
      "clever",
      "cosmic",
      "crisp",
      "curious",
      "eager",
      "gentle",
      "glowing",
      "happy",
      "hidden",
      "jolly",
      "kind",
      "lucky",
      "mighty",
      "misty",
      "neon",
      "nimble",
      "playful",
      "proud",
      "quick",
      "quiet",
      "shiny",
      "silent",
      "stellar",
      "sunny",
      "swift",
      "tidy",
      "witty",
    ]

    const validNouns = [
      "cabin",
      "cactus",
      "canyon",
      "circuit",
      "comet",
      "eagle",
      "engine",
      "falcon",
      "forest",
      "garden",
      "harbor",
      "island",
      "knight",
      "lagoon",
      "meadow",
      "moon",
      "mountain",
      "nebula",
      "orchid",
      "otter",
      "panda",
      "pixel",
      "planet",
      "river",
      "rocket",
      "sailor",
      "squid",
      "star",
      "tiger",
      "wizard",
      "wolf",
    ]

    for (let i = 0; i < 20; i++) {
      const slug = Slug.create()
      const [adjective, noun] = slug.split("-")
      expect(validAdjectives).toContain(adjective)
      expect(validNouns).toContain(noun)
    }
  })
})
