# Go Style

- Return errors, don't panic in library code.
- Wrap errors with %w and context.
- Keep interfaces small; accept interfaces, return structs.
- Name things for the caller, not the implementation.
