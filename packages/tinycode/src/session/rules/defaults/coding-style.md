# Coding Style Rules

## Immutability (CRITICAL for application state)

In shared state and data flow, prefer new objects over in-place mutation. Local accumulators, performance-critical loops, and builder patterns are exempt.

```javascript
// WRONG: Mutation
function updateUser(user, name) {
  user.name = name  // MUTATION!
  return user
}

// CORRECT: Immutability
function updateUser(user, name) {
  return { ...user, name }
}
```

## File Organization

MANY SMALL FILES > FEW LARGE FILES:
- High cohesion, low coupling
- 200-400 lines typical, 800 max
- Extract utilities from large components
- Organize by feature/domain, not by type

## Error Handling

ALWAYS handle errors comprehensively:

```typescript
try {
  const result = await riskyOperation()
  return result
} catch (error) {
  console.error('Operation failed:', error)
  throw new Error('User-friendly error message')
}
```

## Input Validation

ALWAYS validate user input at system boundaries:

```typescript
import { z } from 'zod'

const schema = z.object({
  email: z.string().email(),
  age: z.number().int().min(0).max(150)
})

const validated = schema.parse(input)
```

> **Note:** The `zod` example is TypeScript/JavaScript-specific. Use the equivalent validation library for your stack (e.g., `pydantic` for Python, `validator` for Go), or add project-specific patterns under `[CUSTOMIZE]` below.

## Code Quality Checklist

Before marking work complete:
- [ ] Functions are small (<50 lines)
- [ ] Files are focused (<800 lines)
- [ ] No deep nesting (>4 levels)
- [ ] Proper error handling at boundaries
- [ ] No debug/console.log statements left in
- [ ] No hardcoded values that should be config
- [ ] Immutable patterns used

## [CUSTOMIZE] Project-Specific Style

Add project-specific coding style rules here:
- Naming conventions
- File structure requirements
- Framework-specific patterns
