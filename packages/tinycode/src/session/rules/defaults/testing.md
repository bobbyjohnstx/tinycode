# Testing Rules

> **Scope:** Applies to application and service code. Scripts, prototypes, configuration-only repos, and docs-only repos are exempt. Override the defaults below under `[CUSTOMIZE]`.

## Minimum Test Coverage: 80% (default)

Test Types (required for application/service code):
1. **Unit Tests** - Individual functions, utilities, components
2. **Integration Tests** - API endpoints, database operations
3. **E2E Tests** - Critical user flows (only if the project has user-facing flows)

## Test-Driven Development

Recommended workflow for application code (mandatory for bug fixes — always write a reproducing test first):
1. Write test first (RED)
2. Run test - it should FAIL
3. Write minimal implementation (GREEN)
4. Run test - it should PASS
5. Refactor (IMPROVE)
6. Verify coverage meets project threshold (80% default)

> The git workflow in `git-workflow.md` references TDD as a step — this file is the single source of truth for the TDD process.

## Edge Cases to Test

For each function, test the applicable cases among:
- [ ] Null/undefined inputs (where the parameter is nullable)
- [ ] Empty arrays/strings (where a collection or string is accepted)
- [ ] Invalid types (where the input is untyped or externally sourced)
- [ ] Boundary values (min/max, where numeric or length-constrained)
- [ ] Error conditions (where the function can fail or throw)

## Test Quality Checklist

- [ ] Tests are independent (no shared state)
- [ ] Test names state the scenario and expected outcome (not just the function name)
- [ ] Mocks used for external dependencies
- [ ] Both happy path and error paths tested
- [ ] No flaky tests

## [CUSTOMIZE] Project-Specific Testing

Add project-specific testing requirements here:
- Test framework configuration
- Mock setup patterns
- E2E test scenarios
