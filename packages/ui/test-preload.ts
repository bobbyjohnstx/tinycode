// Minimal browser globals for testing modules that import browser-only packages
if (typeof globalThis.window === "undefined") {
  // @ts-expect-error - minimal stub
  globalThis.window = {
    history: {
      state: null as any,
      length: 1,
      pushState(data: any, _unused: string, _url?: string) { (globalThis as any).window.history.state = data },
      replaceState(data: any, _unused: string, _url?: string) { (globalThis as any).window.history.state = data },
      back() {},
      go() {},
      forward() {},
    },
    location: { pathname: "/", assign: () => {}, href: "", origin: "", search: "", hash: "" },
    addEventListener: () => {},
    removeEventListener: () => {},
    getComputedStyle: () => new Proxy({}, { get: () => "" }),
    innerWidth: 1024,
    innerHeight: 768,
    matchMedia: () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {} }),
    requestAnimationFrame: (cb: () => void) => setTimeout(cb, 0),
    cancelAnimationFrame: (id: number) => clearTimeout(id),
    scrollTo: () => {},
    dispatchEvent: () => true,
    CSS: undefined,
  }
}

if (typeof globalThis.document === "undefined") {
  const noop = () => {}
  const mockElement = () => ({
    style: new Proxy({}, { set: () => true, get: () => "" }),
    setAttribute: noop,
    removeAttribute: noop,
    getAttribute: () => null,
    appendChild: noop,
    removeChild: noop,
    remove: noop,
    contains: () => false,
    closest: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener: noop,
    removeEventListener: noop,
    getBoundingClientRect: () => ({ top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0 }),
    focus: noop,
    blur: noop,
    isConnected: false,
    dataset: {},
    innerHTML: "",
    textContent: "",
    tagName: "DIV",
    nodeType: 1,
    childNodes: [],
    children: [],
    parentElement: null,
  })

  // @ts-expect-error - minimal stub
  globalThis.document = {
    createElement: () => mockElement(),
    createDocumentFragment: () => ({ appendChild: noop }),
    createTreeWalker: () => ({ nextNode: () => null }),
    createRange: () => ({ setStart: noop, setEnd: noop, getClientRects: () => [], cloneRange: () => ({}) }),
    getElementById: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    body: mockElement(),
    head: mockElement(),
    documentElement: {
      ...mockElement(),
      style: new Proxy({}, { set: () => true, get: () => "" }),
      setProperty: noop,
      removeProperty: noop,
    },
    activeElement: null,
    execCommand: () => false,
    addEventListener: noop,
    removeEventListener: noop,
    createComment: () => ({ nodeType: 8 }),
    createTextNode: (text: string) => ({ nodeType: 3, data: text, textContent: text }),
    importNode: (node: any) => node,
  }
}

if (typeof globalThis.navigator === "undefined") {
  // @ts-expect-error - minimal stub
  globalThis.navigator = { clipboard: undefined, userAgent: "bun-test" }
}

if (typeof globalThis.MutationObserver === "undefined") {
  // @ts-expect-error - minimal stub
  globalThis.MutationObserver = class {
    observe() {}
    disconnect() {}
    takeRecords() { return [] }
  }
}

if (typeof globalThis.ResizeObserver === "undefined") {
  // @ts-expect-error - minimal stub
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

if (typeof globalThis.IntersectionObserver === "undefined") {
  // @ts-expect-error - minimal stub
  globalThis.IntersectionObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}
