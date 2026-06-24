# Performance Rules

> **Scope:** These guidelines apply to the performance of code being written. 

## Context Window Management

Avoid the last 20% of the context window for:
- Large-scale refactoring
- Feature implementation spanning multiple files
- Debugging complex interactions

Start a new conversation or compact context before hitting the limit.

## Algorithm Efficiency

Before implementing:
- [ ] Avoid O(n²) when O(n log n) is possible
- [ ] Use appropriate data structures
- [ ] Cache expensive computations

## [CUSTOMIZE] Project-Specific Performance

Add project-specific performance requirements here:
- Response time targets
- Bundle size limits
- Database query limits
