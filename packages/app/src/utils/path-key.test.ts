import { describe, expect, test } from "bun:test"
import { pathKey } from "./path-key"

describe("pathKey", () => {
  test("returns unix path unchanged", () => {
    expect(pathKey("/home/user/project")).toBe("/home/user/project")
  })

  test("strips trailing slashes from unix paths", () => {
    expect(pathKey("/home/user/project/")).toBe("/home/user/project")
    expect(pathKey("/home/user/project///")).toBe("/home/user/project")
  })

  test("normalizes root path to /", () => {
    expect(pathKey("/")).toBe("/")
  })

  test("converts Windows backslashes to forward slashes", () => {
    expect(pathKey("C:\\Users\\project")).toBe("C:/Users/project")
  })

  test("preserves drive letter with trailing slash", () => {
    expect(pathKey("C:")).toBe("C:/")
    expect(pathKey("D:")).toBe("D:/")
  })

  test("handles UNC Windows paths", () => {
    const result = pathKey("\\\\server\\share")
    expect(result).toBe("//server/share")
  })

  test("strips trailing slashes from Windows paths after normalization", () => {
    expect(pathKey("C:\\Users\\project\\")).toBe("C:/Users/project")
  })

  test("handles lowercase drive letters", () => {
    expect(pathKey("c:")).toBe("c:/")
  })
})
