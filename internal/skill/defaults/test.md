---
name: test
description: Generate comprehensive test cases for a function, module, or feature — covering happy path, edge cases, and error conditions
---

# Test

Generate thorough test coverage for specified code. Analyze the implementation, identify all testable behaviors, and write tests that catch real bugs.

## When to Use

Use this skill when:
- The user says "test", "write tests", "add test coverage", "test this function"
- A function or module has no tests or inadequate coverage
- After implementing a feature, to add regression protection
- The user wants to understand what test cases are needed

## When Not to Use

- The user wants to run existing tests — use `verify`
- The user wants to fix a failing test — use `debug`
- The user wants a broad test strategy discussion — advise directly

## Workflow

1. **Read the code**: Understand the function/module being tested.
2. **Identify behaviors**: List all distinct behaviors including:
   - Happy path (normal inputs, expected outputs)
   - Edge cases (empty, nil/null, boundary values, max/min)
   - Error conditions (invalid input, failures, timeouts)
   - State transitions (if stateful)
3. **Check existing tests**: Read any existing test files to avoid duplication.
4. **Match test patterns**: Follow the project's existing test conventions (framework, naming, file location, assertion style).
5. **Write tests**: One test per behavior, with descriptive names.
6. **Run tests**: Verify all new tests pass.

## Rules
- Follow the project's existing test framework and patterns.
- Each test should be independent (no shared mutable state).
- Test names should describe the scenario and expected outcome.
- Use table-driven tests where the language/framework supports them.
- Mock external dependencies, not the code under test.
- Include both positive and negative test cases.

## Output Contract

Report on completion:
- **Target**: what was tested (function/module name and file)
- **Tests written**: count, with names listed
- **Coverage areas**: happy path, edge cases, error conditions
- **Test results**: all pass / N failures
- **Gaps**: any behaviors deliberately not tested (with reason)
