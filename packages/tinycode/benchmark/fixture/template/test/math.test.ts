import { expect, test } from "bun:test"
import { add, multiply, divide } from "../src/math"

test("add returns sum", () => {
  expect(add(2, 3)).toBe(5)
})

test("multiply returns product", () => {
  expect(multiply(3, 4)).toBe(12)
})

test("divide returns quotient", () => {
  expect(divide(10, 2)).toBe(5)
})

test("divide by zero throws", () => {
  expect(() => divide(10, 0)).toThrow()  // FAILS: divide returns Infinity instead of throwing
})
