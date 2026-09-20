import { describe, expect, test } from "bun:test"
import { chooseIconName } from "./file-icon"

describe("chooseIconName", () => {
  describe("known file names", () => {
    test("maps package.json to Nodejs icon", () => {
      expect(chooseIconName("package.json", "file", false)).toBe("Nodejs")
    })

    test("maps dockerfile to Docker icon", () => {
      expect(chooseIconName("Dockerfile", "file", false)).toBe("Docker")
    })

    test("maps .gitignore to Git icon", () => {
      expect(chooseIconName(".gitignore", "file", false)).toBe("Git")
    })

    test("maps tsconfig.json to Tsconfig icon", () => {
      expect(chooseIconName("tsconfig.json", "file", false)).toBe("Tsconfig")
    })

    test("maps go.mod to GoMod icon", () => {
      expect(chooseIconName("go.mod", "file", false)).toBe("GoMod")
    })

    test("matches file names case-insensitively", () => {
      expect(chooseIconName("PACKAGE.JSON", "file", false)).toBe("Nodejs")
      expect(chooseIconName("Makefile", "file", false)).toBe("Makefile")
    })

    test("maps readme.md to Readme icon", () => {
      expect(chooseIconName("README.md", "file", false)).toBe("Readme")
    })

    test("maps .env to Tune icon", () => {
      expect(chooseIconName(".env", "file", false)).toBe("Tune")
    })
  })

  describe("file extensions", () => {
    test("maps .ts to Typescript icon", () => {
      expect(chooseIconName("main.ts", "file", false)).toBe("Typescript")
    })

    test("maps .tsx to React_ts icon", () => {
      expect(chooseIconName("App.tsx", "file", false)).toBe("React_ts")
    })

    test("maps .js to Javascript icon", () => {
      expect(chooseIconName("index.js", "file", false)).toBe("Javascript")
    })

    test("maps .py to Python icon", () => {
      expect(chooseIconName("script.py", "file", false)).toBe("Python")
    })

    test("maps .go to Go icon", () => {
      expect(chooseIconName("main.go", "file", false)).toBe("Go")
    })

    test("maps .rs to Rust icon", () => {
      expect(chooseIconName("lib.rs", "file", false)).toBe("Rust")
    })

    test("maps .md to Markdown icon", () => {
      expect(chooseIconName("notes.md", "file", false)).toBe("Markdown")
    })

    test("maps .json to Json icon", () => {
      expect(chooseIconName("data.json", "file", false)).toBe("Json")
    })

    test("maps .css to Css icon", () => {
      expect(chooseIconName("styles.css", "file", false)).toBe("Css")
    })

    test("maps .html to Html icon", () => {
      expect(chooseIconName("index.html", "file", false)).toBe("Html")
    })

    test("maps .svg to Svg icon", () => {
      expect(chooseIconName("logo.svg", "file", false)).toBe("Svg")
    })
  })

  describe("test file extensions", () => {
    test("maps .test.ts to TestTs icon", () => {
      expect(chooseIconName("app.test.ts", "file", false)).toBe("TestTs")
    })

    test("maps .spec.ts to TestTs icon", () => {
      expect(chooseIconName("app.spec.ts", "file", false)).toBe("TestTs")
    })

    test("maps .test.jsx to TestJsx icon", () => {
      expect(chooseIconName("app.test.jsx", "file", false)).toBe("TestJsx")
    })

    test("maps .test.js to TestJs icon", () => {
      expect(chooseIconName("app.test.js", "file", false)).toBe("TestJs")
    })
  })

  describe("compound extensions (longest match first)", () => {
    test("maps .d.ts to TypescriptDef icon", () => {
      expect(chooseIconName("types.d.ts", "file", false)).toBe("TypescriptDef")
    })

    test("maps .js.map to JavascriptMap icon", () => {
      expect(chooseIconName("bundle.js.map", "file", false)).toBe("JavascriptMap")
    })
  })

  describe("fallback to default icon for unknown files", () => {
    test("returns Document for unknown file extension", () => {
      expect(chooseIconName("data.xyz", "file", false)).toBe("Document")
    })

    test("returns Document for file without extension", () => {
      expect(chooseIconName("LICENSE", "file", false)).toBe("Certificate")
    })
  })

  describe("directory icons", () => {
    test("maps src directory to FolderSrc icon", () => {
      expect(chooseIconName("src", "directory", false)).toBe("FolderSrc")
    })

    test("maps node_modules to FolderNode icon", () => {
      expect(chooseIconName("node_modules", "directory", false)).toBe("FolderNode")
    })

    test("maps test directory to FolderTest icon", () => {
      expect(chooseIconName("tests", "directory", false)).toBe("FolderTest")
    })

    test("maps .github directory to FolderGithub icon", () => {
      expect(chooseIconName(".github", "directory", false)).toBe("FolderGithub")
    })

    test("maps components directory to FolderComponents icon", () => {
      expect(chooseIconName("components", "directory", false)).toBe("FolderComponents")
    })

    test("returns default Folder icon for unknown directory", () => {
      expect(chooseIconName("mydir", "directory", false)).toBe("Folder")
    })
  })

  describe("expanded directory icons", () => {
    test("appends Open suffix when expanded", () => {
      expect(chooseIconName("src", "directory", true)).toBe("FolderSrcOpen")
    })

    test("uses FolderOpen for unknown expanded directory", () => {
      expect(chooseIconName("mydir", "directory", true)).toBe("FolderOpen")
    })

    test("appends Open to node_modules when expanded", () => {
      expect(chooseIconName("node_modules", "directory", true)).toBe("FolderNodeOpen")
    })
  })

  describe("path handling", () => {
    test("extracts basename from full path", () => {
      expect(chooseIconName("/home/user/project/main.go", "file", false)).toBe("Go")
    })

    test("handles backslash paths", () => {
      expect(chooseIconName("C:\\Users\\project\\index.ts", "file", false)).toBe("Typescript")
    })

    test("handles trailing slashes", () => {
      expect(chooseIconName("src/components/", "directory", false)).toBe("FolderComponents")
    })

    test("handles dotted directory names with underscores", () => {
      expect(chooseIconName("__tests__", "directory", false)).toBe("FolderTest")
    })
  })
})
